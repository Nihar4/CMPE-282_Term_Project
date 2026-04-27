variable "project_id" {
  description = "GCP Project ID"
  type        = string
  default     = "enterprise-portal-48689"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone (for Jenkins VM)"
  type        = string
  default     = "us-central1-a"
}

variable "db_tier" {
  description = "Cloud SQL instance tier"
  type        = string
  default     = "db-g1-small"
}

variable "db_name" {
  description = "PostgreSQL database name"
  type        = string
  default     = "enterprise_portal"
}

variable "db_user" {
  description = "PostgreSQL user"
  type        = string
  default     = "portal_user"
}

variable "db_password" {
  description = "PostgreSQL password"
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT signing secret"
  type        = string
  sensitive   = true
  default     = "enterprise-portal-super-secret-jwt-key-2024"
}

variable "nvidia_api_key" {
  description = "NVIDIA NIM API key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "okta_domain" {
  description = "Okta tenant domain for optional Okta OIDC login"
  type        = string
  default     = "trial-5413467.okta.com"
}

variable "okta_issuer" {
  description = "Okta OIDC issuer URL, usually https://<tenant>.okta.com/oauth2/default"
  type        = string
  default     = "https://trial-5413467.okta.com/oauth2/default"
}

variable "okta_client_id" {
  description = "Okta OIDC client id"
  type        = string
  default     = "0oa12cfmwjeBVrl0I698"
}

variable "okta_client_secret" {
  description = "Okta OIDC client secret"
  type        = string
  sensitive   = true
  default     = ""
}

variable "okta_redirect_uri" {
  description = "Okta callback URL. Okta sends the auth code to this URL — must point to the api-gateway /api/auth/callback endpoint."
  type        = string
  default     = "https://api-gateway-ogukkf7z3q-uc.a.run.app/api/auth/callback"
}

variable "okta_logout_redirect_uri" {
  description = "Okta post-logout redirect URL — the frontend Cloud Run URL."
  type        = string
  default     = "https://frontend-ogukkf7z3q-uc.a.run.app"
}

variable "gcs_bucket_name" {
  description = "GCS bucket prefix for file uploads"
  type        = string
  default     = "enterprise-portal-files"
}

variable "environment" {
  description = "Deployment environment (production | staging)"
  type        = string
  default     = "production"
}

variable "domain_name" {
  description = "Primary domain name for the portal (e.g. portal.example.com)"
  type        = string
  default     = "portal.yourdomain.com"
}

variable "allowed_origins" {
  description = "Allowed CORS origins"
  type        = list(string)
  default     = ["https://frontend-ogukkf7z3q-uc.a.run.app", "http://localhost:3000"]
}

variable "alert_email" {
  description = "Email address for Cloud Monitoring alerts"
  type        = string
  default     = "admin@yourdomain.com"
}

variable "jenkins_iap_members" {
  description = "IAM members allowed to access Jenkins via IAP tunnel"
  type        = list(string)
  default     = ["user:niharpatel718@gmail.com"]
}

variable "github_owner" {
  description = "GitHub org / user that owns the repo (for Cloud Build trigger)"
  type        = string
  default     = "Nihar4"
}

variable "github_repo" {
  description = "GitHub repository name (for Cloud Build trigger)"
  type        = string
  default     = "CMPE-282_Term_Project"
}

variable "deploy_serverless" {
  description = "Set false to skip Cloud Run / Functions / Tasks etc. (e.g. on first apply)"
  type        = bool
  default     = true
}

variable "kafka_brokers" {
  description = "Comma-separated Kafka bootstrap brokers for Cloud Run services (Confluent Cloud or GCP Managed Service for Apache Kafka)."
  type        = string
  default     = "REPLACE_WITH_MANAGED_KAFKA_BOOTSTRAP:9092"
}

variable "serverless_ai_min_instances" {
  description = "Minimum warm Cloud Run AI worker instances for Kafka consumption."
  type        = number
  default     = 1
}

variable "image_tag" {
  description = "Container image tag deployed to Cloud Run."
  type        = string
  default     = "latest"
}

variable "enable_cloud_build_triggers" {
  description = "Create Cloud Build GitHub triggers (requires manual repository connection first)"
  type        = bool
  default     = false
}

variable "enable_cloud_armor" {
  description = "Create Cloud Armor policy (requires non-zero SECURITY_POLICIES quota)"
  type        = bool
  default     = false
}
