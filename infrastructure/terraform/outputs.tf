output "gke_cluster_name" {
  value       = google_container_cluster.portal_cluster.name
  description = "GKE cluster name"
}

output "gke_endpoint" {
  value       = google_container_cluster.portal_cluster.endpoint
  description = "GKE cluster endpoint"
  sensitive   = true
}

output "get_credentials_command" {
  value       = "gcloud container clusters get-credentials ${google_container_cluster.portal_cluster.name} --region ${var.region} --project ${var.project_id}"
  description = "Command to configure kubectl"
}

output "cloud_sql_connection_name" {
  value       = google_sql_database_instance.portal_db.connection_name
  description = "Cloud SQL connection name (for Cloud SQL Proxy)"
}

output "cloud_sql_private_ip" {
  value       = google_sql_database_instance.portal_db.private_ip_address
  description = "Private IP of Cloud SQL instance"
  sensitive   = true
}

output "redis_host" {
  value       = google_redis_instance.portal_redis.host
  description = "Cloud Memorystore Redis host"
  sensitive   = true
}

output "redis_auth_string" {
  value       = google_redis_instance.portal_redis.auth_string
  description = "Cloud Memorystore Redis auth string"
  sensitive   = true
}

output "gcs_files_bucket" {
  value       = google_storage_bucket.portal_files.name
  description = "GCS bucket for file uploads"
}

output "artifact_registry_url" {
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/enterprise-portal"
  description = "Artifact Registry URL prefix for Docker images"
}

output "jenkins_vm_name" {
  value       = google_compute_instance.jenkins.name
  description = "Jenkins GCE instance name"
}

output "jenkins_iap_command" {
  value       = "gcloud compute ssh jenkins-server --tunnel-through-iap --zone ${var.zone} -- -L 8080:localhost:8080"
  description = "IAP tunnel command to access Jenkins UI"
}

output "pubsub_file_events_topic" {
  value       = google_pubsub_topic.file_events.name
  description = "Cloud Pub/Sub topic for file processing events"
}

output "pubsub_ai_events_topic" {
  value       = google_pubsub_topic.ai_events.name
  description = "Cloud Pub/Sub topic for AI query events"
}

output "dns_name_servers" {
  value       = google_dns_managed_zone.portal_zone.name_servers
  description = "NS records to set at your domain registrar"
}

output "secret_db_password_id" {
  value       = google_secret_manager_secret.db_password.secret_id
  description = "Secret Manager ID for DB password"
}

output "cloud_armor_policy" {
  value       = google_compute_security_policy.portal_armor.name
  description = "Cloud Armor WAF policy name"
}

output "cloud_run_parser_url" {
  value       = google_cloud_run_v2_service.parser_service.uri
  description = "Cloud Run URL for parser-service (serverless burst)"
}

output "kms_keyring" {
  value       = google_kms_key_ring.portal_keyring.id
  description = "Cloud KMS key ring"
}

output "bigquery_dataset" {
  value       = google_bigquery_dataset.portal_analytics.dataset_id
  description = "BigQuery dataset for analytics events"
}

output "static_bucket" {
  value       = google_storage_bucket.portal_static.name
  description = "GCS bucket hosting the React build (served via Cloud CDN)"
}

output "lb_ip" {
  value       = google_compute_global_address.portal_lb_ip.address
  description = "Global LB IP — set this as A record at your DNS registrar"
}

output "cloud_build_main_trigger" {
  value       = google_cloudbuild_trigger.main_branch.name
  description = "Cloud Build trigger that runs on push to main"
}

output "tasks_queue" {
  value       = google_cloud_tasks_queue.ai_rerank.name
  description = "Cloud Tasks queue for AI re-ranking jobs"
}

output "cloud_function_file_ingest" {
  value       = google_cloudfunctions2_function.file_ingest.name
  description = "Cloud Function (Gen2) reacting to GCS finalize"
}

output "scheduler_nightly_summary" {
  value       = google_cloud_scheduler_job.nightly_summary.name
  description = "Cloud Scheduler job for nightly summary refresh"
}

output "setup_summary" {
  value       = <<-EOT
  =========================================================
   Enterprise Portal — GCP Infrastructure Ready
  =========================================================
   GKE Cluster  : ${google_container_cluster.portal_cluster.name} (${var.region})
   Cloud SQL     : ${google_sql_database_instance.portal_db.connection_name}
   Redis         : ${google_redis_instance.portal_redis.host}
   Artifact Reg  : ${var.region}-docker.pkg.dev/${var.project_id}/enterprise-portal
   Files Bucket  : ${google_storage_bucket.portal_files.name}
   Jenkins VM    : ${google_compute_instance.jenkins.name} (${var.zone})

   Next steps:
   1. gcloud container clusters get-credentials ${google_container_cluster.portal_cluster.name} --region ${var.region}
   2. kubectl apply -f infrastructure/k8s/
   3. Access Jenkins: gcloud compute ssh jenkins-server --tunnel-through-iap --zone ${var.zone} -- -L 8080:localhost:8080
  =========================================================
  EOT
  description = "Summary of deployed infrastructure"
}
