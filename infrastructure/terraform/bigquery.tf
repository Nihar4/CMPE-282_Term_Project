# ============================================================
# BigQuery — destination for analytics events streamed from the
# analytics-service via Pub/Sub → BQ subscription.
# ============================================================

resource "google_bigquery_dataset" "portal_analytics" {
  dataset_id    = "enterprise_portal_analytics"
  friendly_name = "Enterprise Portal Analytics"
  description   = "Streamed analytics events for dashboards"
  location      = var.region

  default_table_expiration_ms = 7776000000 # 90 days

  labels = {
    environment = var.environment
    app         = "enterprise-portal"
  }

  depends_on = [google_project_service.apis]
}

resource "google_bigquery_table" "events" {
  dataset_id          = google_bigquery_dataset.portal_analytics.dataset_id
  table_id            = "events"
  deletion_protection = false

  time_partitioning {
    type  = "DAY"
    field = "event_time"
  }

  schema = jsonencode([
    { name = "event_id", type = "STRING", mode = "REQUIRED" },
    { name = "event_time", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "user_id", type = "STRING", mode = "NULLABLE" },
    { name = "event_type", type = "STRING", mode = "REQUIRED" },
    { name = "service", type = "STRING", mode = "REQUIRED" },
    { name = "payload", type = "JSON", mode = "NULLABLE" },
  ])
}

# Pub/Sub → BigQuery subscription (no Dataflow required)
resource "google_pubsub_subscription" "ai_events_to_bq" {
  name  = "enterprise-portal-ai-events-bq"
  topic = google_pubsub_topic.ai_events.name

  bigquery_config {
    table            = "${data.google_project.current.project_id}.${google_bigquery_dataset.portal_analytics.dataset_id}.${google_bigquery_table.events.table_id}"
    use_table_schema = true
    write_metadata   = true
  }

  ack_deadline_seconds = 60

  depends_on = [
    google_bigquery_table.events,
    google_project_iam_member.pubsub_bq,
  ]
}

# Pub/Sub service agent needs BQ data editor
resource "google_project_iam_member" "pubsub_bq" {
  project = var.project_id
  role    = "roles/bigquery.dataEditor"
  member  = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_project_iam_member" "pubsub_bq_metadata" {
  project = var.project_id
  role    = "roles/bigquery.metadataViewer"
  member  = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
