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

output "setup_summary" {
  value = <<-EOT
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
