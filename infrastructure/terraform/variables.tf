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

variable "cluster_name" {
  description = "GKE cluster name"
  type        = string
  default     = "enterprise-portal-cluster"
}

variable "node_count" {
  description = "Number of nodes per zone in the GKE node pool"
  type        = number
  default     = 2
}

variable "machine_type" {
  description = "GKE node machine type"
  type        = string
  default     = "e2-standard-4"
}

variable "min_node_count" {
  description = "Minimum nodes per zone for autoscaling"
  type        = number
  default     = 2
}

variable "max_node_count" {
  description = "Maximum nodes per zone for autoscaling"
  type        = number
  default     = 10
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

variable "auth0_client_secret" {
  description = "Auth0 client secret"
  type        = string
  sensitive   = true
  default     = ""
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
  default     = ["https://portal.yourdomain.com", "http://localhost:3000"]
}

variable "alert_email" {
  description = "Email address for Cloud Monitoring alerts"
  type        = string
  default     = "admin@yourdomain.com"
}

variable "jenkins_iap_members" {
  description = "IAM members allowed to access Jenkins via IAP tunnel"
  type        = list(string)
  default     = ["allAuthenticatedUsers"]
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
