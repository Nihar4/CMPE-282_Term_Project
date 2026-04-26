# ============================================================
# Cloud Run — serverless deployment of the Python parser-service
#
# We run the parser-service on Cloud Run (in addition to GKE) so we can
# burst to it during heavy parsing without scaling the GKE cluster.
# Cloud Run is event-driven (Pub/Sub-pushed) and scales to zero.
# ============================================================

# Build the parser-service image with Cloud Build first, then deploy:
#   gcloud builds submit --tag REGION-docker.pkg.dev/$PROJECT/enterprise-portal/parser-service:latest backend/parser-service

# Service account used only by Cloud Run revisions
resource "google_service_account" "cloud_run_sa" {
  account_id   = "enterprise-portal-run-sa"
  display_name = "Enterprise Portal Cloud Run SA"
}

resource "google_project_iam_member" "cloud_run_sa_roles" {
  for_each = toset([
    "roles/storage.objectAdmin",
    "roles/secretmanager.secretAccessor",
    "roles/pubsub.subscriber",
    "roles/cloudtrace.agent",
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
  ])
  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${google_service_account.cloud_run_sa.email}"
}

# Parser-service on Cloud Run (serverless burst tier)
resource "google_cloud_run_v2_service" "parser_service" {
  name     = "parser-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  template {
    service_account = google_service_account.cloud_run_sa.email

    scaling {
      min_instance_count = 0
      max_instance_count = 20
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/enterprise-portal/parser-service:latest"

      resources {
        limits = {
          cpu    = "2"
          memory = "2Gi"
        }
        cpu_idle = true
      }

      ports {
        container_port = 8000
      }

      env {
        name  = "GCS_BUCKET"
        value = google_storage_bucket.portal_files.name
      }
      env {
        name  = "PUBSUB_TOPIC"
        value = google_pubsub_topic.file_events.name
      }
      env {
        name  = "PROJECT_ID"
        value = var.project_id
      }
    }

    timeout = "300s"

    annotations = {
      "autoscaling.knative.dev/maxScale"     = "20"
      "run.googleapis.com/cpu-throttling"    = "false"
      "run.googleapis.com/startup-cpu-boost" = "true"
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }

  depends_on = [
    google_project_service.apis,
    google_artifact_registry_repository.portal_repo,
  ]
}

# Pub/Sub push subscription that delivers file_events → Cloud Run parser
resource "google_pubsub_subscription" "parser_push" {
  name  = "parser-service-file-events-push"
  topic = google_pubsub_topic.file_events.name

  ack_deadline_seconds = 300

  push_config {
    push_endpoint = google_cloud_run_v2_service.parser_service.uri
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa.email
    }
    attributes = {
      "x-goog-version" = "v1"
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}
