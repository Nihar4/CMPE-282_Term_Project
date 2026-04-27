# ============================================================
# Cloud Run — serverless deployment of the Python parser-service
#
# Cloud Run is event-driven (Pub/Sub-pushed) and scales to zero.
# ============================================================

# Build the parser-service image with Cloud Build first, then deploy:
#   gcloud builds submit --tag REGION-docker.pkg.dev/$PROJECT/enterprise-portal/parser-service:latest backend/parser-service

# Service account used only by Cloud Run revisions
resource "google_service_account" "cloud_run_sa" {
  count        = var.deploy_serverless ? 1 : 0
  account_id   = "enterprise-portal-run-sa"
  display_name = "Enterprise Portal Cloud Run SA"
}

resource "google_project_iam_member" "cloud_run_sa_roles" {
  for_each = var.deploy_serverless ? toset([
    "roles/storage.objectAdmin",
    "roles/secretmanager.secretAccessor",
    "roles/cloudsql.client",
    "roles/vpcaccess.user",
    "roles/run.invoker",
    "roles/pubsub.subscriber",
    "roles/cloudtrace.agent",
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
  ]) : toset([])
  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${google_service_account.cloud_run_sa[0].email}"
}

# Parser-service on Cloud Run (serverless burst tier)
resource "google_cloud_run_v2_service" "parser_service" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "parser-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  template {
    service_account = google_service_account.cloud_run_sa[0].email

    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/enterprise-portal/parser-service:${var.image_tag}"

      resources {
        limits = {
          cpu    = "2"
          memory = "2Gi"
        }
        cpu_idle = true
      }

      ports {
        container_port = 8090
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
      "autoscaling.knative.dev/maxScale"     = "1"
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
  count = var.deploy_serverless ? 1 : 0
  name  = "parser-service-file-events-push"
  topic = google_pubsub_topic.file_events.name

  ack_deadline_seconds = 300

  push_config {
    push_endpoint = google_cloud_run_v2_service.parser_service[0].uri
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa[0].email
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
