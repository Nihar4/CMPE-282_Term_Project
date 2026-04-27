terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 5.0"
    }
  }
  # Remote state in GCS — create this bucket manually FIRST, then uncomment
  backend "gcs" {
    bucket = "enterprise-portal-tfstate-48689"
    prefix = "terraform/state"
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
}

# ─── Enable All Required GCP APIs ────────────────────────────────────────────

resource "google_project_service" "apis" {
  for_each = toset([
    # Core
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com",
    "compute.googleapis.com",           # GCE (Jenkins VM)
    "servicenetworking.googleapis.com", # Private networking
    "networkmanagement.googleapis.com",

    # Containers / Build
    "artifactregistry.googleapis.com", # Artifact Registry
    "cloudbuild.googleapis.com",       # Cloud Build deployment runner

    # Data
    "sqladmin.googleapis.com", # Cloud SQL
    "redis.googleapis.com",    # Memorystore
    "storage.googleapis.com",  # Cloud Storage
    "bigquery.googleapis.com", # BigQuery (analytics)

    # Messaging
    "pubsub.googleapis.com", # Cloud Pub/Sub

    # Serverless
    "run.googleapis.com",       # Cloud Run
    "vpcaccess.googleapis.com", # Serverless VPC Access for Cloud Run -> private SQL/Redis

    # Security
    "secretmanager.googleapis.com", # Secret Manager
    "cloudkms.googleapis.com",      # Cloud KMS

    # Networking & SSL
    "dns.googleapis.com",                # Cloud DNS
    "iap.googleapis.com",                # Identity-Aware Proxy
    "certificatemanager.googleapis.com", # Cert Manager

    # Observability
    "monitoring.googleapis.com",    # Cloud Monitoring
    "logging.googleapis.com",       # Cloud Logging
    "cloudtrace.googleapis.com",    # Cloud Trace
    "cloudprofiler.googleapis.com", # Cloud Profiler

    # AI
    "aiplatform.googleapis.com", # Vertex AI
  ])
  service            = each.key
  disable_on_destroy = false
}

# ─── VPC Network ─────────────────────────────────────────────────────────────

resource "google_compute_network" "portal_vpc" {
  name                    = "enterprise-portal-vpc"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.apis]
}

resource "google_compute_subnetwork" "portal_subnet" {
  name                     = "enterprise-portal-subnet"
  ip_cidr_range            = "10.0.0.0/16"
  region                   = var.region
  network                  = google_compute_network.portal_vpc.id
  private_ip_google_access = true # Allow access to Google APIs without public IP

}

# Jenkins subnet (separate for isolation)
resource "google_compute_subnetwork" "jenkins_subnet" {
  name          = "jenkins-subnet"
  ip_cidr_range = "10.10.0.0/24"
  region        = var.region
  network       = google_compute_network.portal_vpc.id
}

# ─── Cloud NAT (outbound internet for private serverless/VPC workloads) ───────

resource "google_compute_router" "portal_router" {
  name    = "enterprise-portal-router"
  region  = var.region
  network = google_compute_network.portal_vpc.id
}

resource "google_compute_router_nat" "portal_nat" {
  name                               = "enterprise-portal-nat"
  router                             = google_compute_router.portal_router.name
  region                             = var.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}

# ─── Firewall Rules ───────────────────────────────────────────────────────────

# Allow internal cluster communication
resource "google_compute_firewall" "allow_internal" {
  name    = "enterprise-portal-allow-internal"
  network = google_compute_network.portal_vpc.name

  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "icmp"
  }

  source_ranges = ["10.0.0.0/8"]
}

# Allow SSH to Jenkins via IAP
resource "google_compute_firewall" "allow_ssh_iap" {
  name    = "allow-ssh-iap"
  network = google_compute_network.portal_vpc.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
  # IAP IP range
  source_ranges = ["35.235.240.0/20"]
  target_tags   = ["jenkins"]
}

# Allow Jenkins port 8080 via IAP
resource "google_compute_firewall" "allow_jenkins_iap" {
  name    = "allow-jenkins-iap"
  network = google_compute_network.portal_vpc.name

  allow {
    protocol = "tcp"
    ports    = ["8080"]
  }
  source_ranges = ["35.235.240.0/20"]
  target_tags   = ["jenkins"]
}

# Allow health checks from GCP LB
resource "google_compute_firewall" "allow_health_checks" {
  name    = "allow-gcp-health-checks"
  network = google_compute_network.portal_vpc.name

  allow {
    protocol = "tcp"
  }
  source_ranges = ["130.211.0.0/22", "35.191.0.0/16"]
}

# ─── Cloud SQL (PostgreSQL 15) ────────────────────────────────────────────────

resource "google_compute_global_address" "sql_private_ip" {
  name          = "enterprise-portal-sql-private-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.portal_vpc.id
}

resource "google_service_networking_connection" "sql_private_connection" {
  network                 = google_compute_network.portal_vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.sql_private_ip.name]
}

resource "google_sql_database_instance" "portal_db" {
  name             = "enterprise-portal-pg"
  database_version = "POSTGRES_15"
  region           = var.region

  deletion_protection = true

  settings {
    tier              = var.db_tier
    availability_type = "REGIONAL" # HA with automatic failover replica

    disk_size             = 50
    disk_type             = "PD_SSD"
    disk_autoresize       = true
    disk_autoresize_limit = 500

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      start_time                     = "02:00"
      transaction_log_retention_days = 7
      backup_retention_settings {
        retained_backups = 30
        retention_unit   = "COUNT"
      }
    }

    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = google_compute_network.portal_vpc.id
      enable_private_path_for_google_cloud_services = true
    }

    database_flags {
      name  = "max_connections"
      value = "500"
    }

    database_flags {
      name  = "log_checkpoints"
      value = "on"
    }

    database_flags {
      name  = "log_connections"
      value = "on"
    }

    insights_config {
      query_insights_enabled  = true
      record_application_tags = true
      record_client_address   = true
      query_string_length     = 1024
    }

    maintenance_window {
      day          = 7 # Sunday
      hour         = 2 # 2 AM UTC
      update_track = "stable"
    }
  }

  depends_on = [
    google_service_networking_connection.sql_private_connection,
    google_project_service.apis,
  ]
}

resource "google_sql_database" "portal_database" {
  name     = var.db_name
  instance = google_sql_database_instance.portal_db.name
}

resource "google_sql_user" "portal_user" {
  name     = var.db_user
  instance = google_sql_database_instance.portal_db.name
  password = var.db_password
}

# ─── Cloud Secret Manager ─────────────────────────────────────────────────────

resource "google_secret_manager_secret" "db_password" {
  secret_id = "portal-db-password"
  replication {
    auto {}
  }
  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = var.db_password
}

resource "google_secret_manager_secret" "jwt_secret" {
  secret_id = "portal-jwt-secret"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "jwt_secret" {
  secret      = google_secret_manager_secret.jwt_secret.id
  secret_data = var.jwt_secret
}

resource "google_secret_manager_secret" "nvidia_api_key" {
  secret_id = "portal-nvidia-api-key"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "nvidia_api_key" {
  secret      = google_secret_manager_secret.nvidia_api_key.id
  secret_data = var.nvidia_api_key
}

data "google_secret_manager_secret" "okta_client_secret" {
  secret_id = "portal-okta-client-secret"
}

# ─── Cloud Storage (File Uploads + Static Assets) ────────────────────────────

resource "google_storage_bucket" "portal_files" {
  name          = "${var.gcs_bucket_name}-${var.project_id}"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true
  storage_class               = "STANDARD"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition { age = 365 }
    action { type = "Delete" }
  }

  lifecycle_rule {
    condition {
      age                   = 90
      matches_storage_class = ["STANDARD"]
    }
    action {
      type          = "SetStorageClass"
      storage_class = "NEARLINE"
    }
  }

  cors {
    origin          = var.allowed_origins
    method          = ["GET", "POST", "DELETE", "HEAD", "PUT"]
    response_header = ["*"]
    max_age_seconds = 3600
  }
}

# GCS bucket for Terraform state
resource "google_storage_bucket" "tfstate" {
  name          = "enterprise-portal-tfstate-${var.project_id}"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true
  versioning { enabled = true }
}

# ─── Cloud Memorystore (Redis 7) ──────────────────────────────────────────────

resource "google_redis_instance" "portal_redis" {
  name           = "enterprise-portal-redis"
  tier           = "STANDARD_HA"
  memory_size_gb = 2
  region         = var.region

  authorized_network      = google_compute_network.portal_vpc.id
  connect_mode            = "PRIVATE_SERVICE_ACCESS"
  redis_version           = "REDIS_7_0"
  display_name            = "Enterprise Portal Redis"
  auth_enabled            = true
  transit_encryption_mode = "DISABLED" # Enable for production: SERVER_AUTHENTICATION

  redis_configs = {
    "maxmemory-policy" = "allkeys-lru"
  }

  maintenance_policy {
    weekly_maintenance_window {
      day = "SUNDAY"
      start_time {
        hours   = 2
        minutes = 0
        seconds = 0
        nanos   = 0
      }
    }
  }

  depends_on = [
    google_service_networking_connection.sql_private_connection,
    google_project_service.apis,
  ]
}

# ─── Cloud Pub/Sub (Event Streaming) ─────────────────────────────────────────

# Topic: file processing events
resource "google_pubsub_topic" "file_events" {
  name = "enterprise-portal-file-events"

  message_retention_duration = "86600s" # 24 hours

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_subscription" "file_events_sub" {
  name  = "enterprise-portal-file-events-sub"
  topic = google_pubsub_topic.file_events.name

  ack_deadline_seconds       = 60
  message_retention_duration = "86400s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = "86400s"
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "300s"
  }
}

# Topic: AI query events
resource "google_pubsub_topic" "ai_events" {
  name                       = "enterprise-portal-ai-events"
  message_retention_duration = "86600s"
}

resource "google_pubsub_subscription" "ai_events_sub" {
  name  = "enterprise-portal-ai-events-sub"
  topic = google_pubsub_topic.ai_events.name

  ack_deadline_seconds       = 120
  message_retention_duration = "86600s"

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}

# Topic: notification events
resource "google_pubsub_topic" "notification_events" {
  name                       = "enterprise-portal-notifications"
  message_retention_duration = "86600s"
}

# ─── Artifact Registry ────────────────────────────────────────────────────────

resource "google_artifact_registry_repository" "portal_repo" {
  location      = var.region
  repository_id = "enterprise-portal"
  format        = "DOCKER"
  description   = "Enterprise Portal container images"

  docker_config {
    immutable_tags = false
  }

  cleanup_policies {
    id     = "keep-minimum-versions"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }

  depends_on = [google_project_service.apis]
}

# IAM: Jenkins SA can push images
resource "google_artifact_registry_repository_iam_member" "jenkins_push" {
  location   = google_artifact_registry_repository.portal_repo.location
  repository = google_artifact_registry_repository.portal_repo.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.jenkins_sa.email}"
}

# ─── Jenkins on GCE ───────────────────────────────────────────────────────────

resource "google_service_account" "jenkins_sa" {
  account_id   = "enterprise-portal-jenkins"
  display_name = "Jenkins CI/CD Service Account"
}

resource "google_project_iam_member" "jenkins_roles" {
  for_each = toset([
    "roles/cloudbuild.builds.editor",     # Trigger Cloud Build
    "roles/artifactregistry.writer",      # Push/read Docker images when needed
    "roles/storage.objectAdmin",          # Access GCS
    "roles/secretmanager.secretAccessor", # Read secrets
    "roles/cloudsql.client",              # Connect to Cloud SQL
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
  ])
  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${google_service_account.jenkins_sa.email}"
}

resource "google_compute_instance" "jenkins" {
  name         = "jenkins-server"
  machine_type = "e2-standard-4" # 4 vCPU, 16 GB RAM
  zone         = var.zone
  tags         = ["jenkins"]

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
      size  = 50
      type  = "pd-standard"
    }
  }

  network_interface {
    network    = google_compute_network.portal_vpc.id
    subnetwork = google_compute_subnetwork.jenkins_subnet.id
    # No access_config = no public IP (access via IAP)
  }

  service_account {
    email  = google_service_account.jenkins_sa.email
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }

  metadata = {
    enable-oslogin = "TRUE"
  }

  metadata_startup_script = <<-EOT
    #!/bin/bash
    set -e
    apt-get update -y

    # ── Java 21 ──────────────────────────────────────────
    apt-get install -y fontconfig openjdk-17-jre wget curl gnupg2 apt-transport-https ca-certificates

    # ── Jenkins ──────────────────────────────────────────
    curl -fsSL https://pkg.jenkins.io/debian-stable/jenkins.io-2023.key | gpg --dearmor -o /usr/share/keyrings/jenkins-keyring.gpg
    echo "deb [signed-by=/usr/share/keyrings/jenkins-keyring.gpg] https://pkg.jenkins.io/debian-stable binary/" > /etc/apt/sources.list.d/jenkins.list
    apt-get update -y
    apt-get install -y jenkins

    # ── Docker ───────────────────────────────────────────
    curl -fsSL https://get.docker.com | bash
    usermod -aG docker jenkins

    # ── gcloud SDK ───────────────────────────────────────
    echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" > /etc/apt/sources.list.d/google-cloud-sdk.list
    curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg
    apt-get update -y && apt-get install -y google-cloud-sdk

    # ── Trivy (security scanner) ─────────────────────────
    wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor | tee /usr/share/keyrings/trivy.gpg > /dev/null
    echo "deb [signed-by=/usr/share/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb generic main" > /etc/apt/sources.list.d/trivy.list
    apt-get update -y && apt-get install -y trivy

    # ── Terraform ────────────────────────────────────────
    wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" > /etc/apt/sources.list.d/hashicorp.list
    apt-get update -y && apt-get install -y terraform

    # ── Start Jenkins ─────────────────────────────────────
    systemctl enable jenkins
    systemctl start jenkins

    # Configure Docker credential helper for Artifact Registry
    gcloud auth configure-docker ${var.region}-docker.pkg.dev --quiet || true

    echo "Jenkins setup complete" >> /var/log/jenkins-setup.log
  EOT

  shielded_instance_config {
    enable_secure_boot          = true
    enable_integrity_monitoring = true
    enable_vtpm                 = true
  }

  depends_on = [google_project_service.apis]
}

# IAP tunnel access to Jenkins
resource "google_iap_tunnel_instance_iam_binding" "jenkins_iap" {
  instance = google_compute_instance.jenkins.name
  zone     = var.zone
  role     = "roles/iap.tunnelResourceAccessor"
  members  = var.jenkins_iap_members
}

# ─── Cloud Armor (WAF + DDoS Protection) ─────────────────────────────────────

resource "google_compute_security_policy" "portal_armor" {
  count = var.enable_cloud_armor ? 1 : 0
  name  = "enterprise-portal-armor"

  # Block known malicious IPs (geo-restrict if needed)
  rule {
    action   = "deny(403)"
    priority = 1000
    match {
      expr {
        expression = "evaluatePreconfiguredExpr('xss-stable')"
      }
    }
    description = "Block XSS attacks"
  }

  rule {
    action   = "deny(403)"
    priority = 1001
    match {
      expr {
        expression = "evaluatePreconfiguredExpr('sqli-stable')"
      }
    }
    description = "Block SQL injection attacks"
  }

  rule {
    action   = "throttle"
    priority = 2000
    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }
    description = "Rate limit: 1000 req/min per IP"
    rate_limit_options {
      conform_action = "allow"
      exceed_action  = "deny(429)"
      enforce_on_key = "IP"
      rate_limit_threshold {
        count        = 1000
        interval_sec = 60
      }
    }
  }

  # Default allow
  rule {
    action   = "allow"
    priority = 2147483647
    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }
    description = "Default allow"
  }

  adaptive_protection_config {
    layer_7_ddos_defense_config {
      enable = true
    }
  }

  depends_on = [google_project_service.apis]
}

# ─── Cloud Monitoring — Alert Policies ───────────────────────────────────────

resource "google_monitoring_notification_channel" "email" {
  display_name = "Enterprise Portal Alerts"
  type         = "email"
  labels = {
    email_address = var.alert_email
  }
  depends_on = [google_project_service.apis]
}

resource "google_monitoring_alert_policy" "high_cpu" {
  display_name = "Portal — Cloud Run High CPU Usage"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "Cloud Run CPU > 80% for 5 minutes"
    condition_threshold {
      filter          = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/container/cpu/utilizations\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0.8
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_PERCENTILE_95"
      }
    }
  }

  notification_channels = [google_monitoring_notification_channel.email.name]
  depends_on            = [google_project_service.apis]
}

resource "google_monitoring_alert_policy" "cloud_run_errors" {
  display_name = "Portal — Cloud Run 5xx Errors"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "5xx responses > 10 in 10 min"
    condition_threshold {
      filter          = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/request_count\" AND metric.labels.response_code_class=\"5xx\""
      duration        = "600s"
      comparison      = "COMPARISON_GT"
      threshold_value = 10
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_SUM"
      }
    }
  }

  notification_channels = [google_monitoring_notification_channel.email.name]
}

resource "google_monitoring_alert_policy" "sql_connections" {
  display_name = "Portal — Cloud SQL High Connections"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "SQL connections > 400"
    condition_threshold {
      filter          = "resource.type=\"cloudsql_database\" AND metric.type=\"cloudsql.googleapis.com/database/postgresql/num_backends\""
      duration        = "120s"
      comparison      = "COMPARISON_GT"
      threshold_value = 400
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }
    }
  }

  notification_channels = [google_monitoring_notification_channel.email.name]
}

# ─── Cloud DNS ────────────────────────────────────────────────────────────────

resource "google_dns_managed_zone" "portal_zone" {
  name        = "enterprise-portal-zone"
  dns_name    = "${var.domain_name}."
  description = "Enterprise Portal DNS Zone"

  dnssec_config {
    state = "on"
  }

  depends_on = [google_project_service.apis]
}

# DNS record can point at the frontend Cloud Run URL or a serverless NEG/load balancer.

# ─── Cloud Logging — Log Sink ─────────────────────────────────────────────────

resource "google_storage_bucket" "logs_bucket" {
  name          = "enterprise-portal-logs-${var.project_id}"
  location      = var.region
  force_destroy = false

  lifecycle_rule {
    condition { age = 90 }
    action { type = "Delete" }
  }
  uniform_bucket_level_access = true
}

resource "google_logging_project_sink" "portal_sink" {
  name        = "enterprise-portal-log-sink"
  destination = "storage.googleapis.com/${google_storage_bucket.logs_bucket.name}"
  filter      = "resource.type=\"cloud_run_revision\" OR resource.type=\"cloudsql_database\" OR resource.type=\"redis_instance\""

  unique_writer_identity = true
}

resource "google_storage_bucket_iam_member" "log_sink_writer" {
  bucket = google_storage_bucket.logs_bucket.name
  role   = "roles/storage.objectCreator"
  member = google_logging_project_sink.portal_sink.writer_identity
}
