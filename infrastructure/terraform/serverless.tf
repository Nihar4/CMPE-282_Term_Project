# ============================================================
# Serverless add-ons:
#   - Cloud Functions (Gen 2) for asynchronous file ingestion hook
#   - Cloud Scheduler for nightly DB summary refresh
#   - Cloud Tasks for delayed AI re-ranking jobs
# ============================================================

# ─── Cloud Functions (Gen 2) ──────────────────────────────────────────────────

resource "google_storage_bucket" "functions_source" {
  name          = "enterprise-portal-fn-src-${var.project_id}"
  location      = var.region
  force_destroy = true

  uniform_bucket_level_access = true
}

# Placeholder: package zip created by deploy script and uploaded as gs://...
# We declare the function so terraform manages it; deploy.sh uploads the zip.
resource "google_storage_bucket_object" "fn_archive" {
  name   = "file-ingest-fn-${formatdate("YYYYMMDDhhmmss", timestamp())}.zip"
  bucket = google_storage_bucket.functions_source.name
  source = "${path.module}/functions/file-ingest-fn.zip"

  lifecycle {
    ignore_changes = [name, source]
  }
}

resource "google_cloudfunctions2_function" "file_ingest" {
  name        = "file-ingest-fn"
  location    = var.region
  description = "Triggered by GCS finalize → publishes file_events Pub/Sub message"

  build_config {
    runtime     = "python311"
    entry_point = "main"
    source {
      storage_source {
        bucket = google_storage_bucket.functions_source.name
        object = google_storage_bucket_object.fn_archive.name
      }
    }
  }

  service_config {
    max_instance_count = 50
    min_instance_count = 0
    available_memory   = "256M"
    timeout_seconds    = 60
    environment_variables = {
      PUBSUB_TOPIC = google_pubsub_topic.file_events.name
      PROJECT_ID   = var.project_id
    }
    ingress_settings      = "ALLOW_INTERNAL_ONLY"
    service_account_email = google_service_account.cloud_run_sa.email
  }

  event_trigger {
    trigger_region = var.region
    event_type     = "google.cloud.storage.object.v1.finalized"
    retry_policy   = "RETRY_POLICY_RETRY"
    event_filters {
      attribute = "bucket"
      value     = google_storage_bucket.portal_files.name
    }
  }

  depends_on = [
    google_project_service.apis,
    google_storage_bucket_object.fn_archive,
  ]
}

# ─── Cloud Scheduler ─────────────────────────────────────────────────────────

resource "google_cloud_scheduler_job" "nightly_summary" {
  name        = "enterprise-portal-nightly-summary"
  description = "Nightly: refresh AI/analytics caches"
  schedule    = "0 3 * * *"
  time_zone   = "America/Los_Angeles"
  region      = var.region

  http_target {
    http_method = "POST"
    uri         = "https://${var.domain_name}/api/analytics/refresh"
    headers = {
      "Content-Type" = "application/json"
    }
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa.email
    }
    body = base64encode(jsonencode({ task = "nightly-refresh" }))
  }

  retry_config {
    retry_count          = 3
    min_backoff_duration = "30s"
    max_backoff_duration = "300s"
  }

  depends_on = [google_project_service.apis]
}

resource "google_cloud_scheduler_job" "weekly_backup_check" {
  name        = "enterprise-portal-backup-check"
  description = "Weekly: verify Cloud SQL backups exist"
  schedule    = "0 4 * * 1"
  time_zone   = "America/Los_Angeles"
  region      = var.region

  pubsub_target {
    topic_name = google_pubsub_topic.notification_events.id
    data       = base64encode(jsonencode({ event = "backup_check" }))
  }
}

# ─── Cloud Tasks queue ────────────────────────────────────────────────────────

resource "google_cloud_tasks_queue" "ai_rerank" {
  name     = "enterprise-portal-ai-rerank"
  location = var.region

  rate_limits {
    max_concurrent_dispatches = 10
    max_dispatches_per_second = 50
  }

  retry_config {
    max_attempts       = 5
    max_retry_duration = "600s"
    min_backoff        = "10s"
    max_backoff        = "120s"
    max_doublings      = 4
  }

  depends_on = [google_project_service.apis]
}
