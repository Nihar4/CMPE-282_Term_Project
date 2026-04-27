# ============================================================
# Cloud Run — full serverless portal topology
# ============================================================

locals {
  cloud_run_image_base = "${var.region}-docker.pkg.dev/${var.project_id}/enterprise-portal"
  cloud_run_image_tag  = var.image_tag
  db_private_host      = google_sql_database_instance.portal_db.private_ip_address
  redis_url            = "${google_redis_instance.portal_redis.host}:6379"
  notification_topic   = "portal.notifications"
  ai_requests_topic    = "portal.ai.requests"
}

resource "google_vpc_access_connector" "serverless" {
  count         = var.deploy_serverless ? 1 : 0
  name          = "portal-run-vpc"
  region        = var.region
  network       = google_compute_network.portal_vpc.name
  ip_cidr_range = "10.8.0.0/28"
  min_instances = 2
  max_instances = 3

  depends_on = [google_project_service.apis]
}

resource "google_cloud_run_v2_service" "auth_service" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "auth-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "300s"

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    vpc_access {
      connector = google_vpc_access_connector.serverless[0].id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = "${local.cloud_run_image_base}/auth-service:${local.cloud_run_image_tag}"

      ports {
        container_port = 8081
      }

      env {
        name  = "DB_HOST"
        value = local.db_private_host
      }
      env {
        name  = "DB_PORT"
        value = "5432"
      }
      env {
        name  = "DB_NAME"
        value = var.db_name
      }
      env {
        name  = "DB_USER"
        value = var.db_user
      }
      env {
        name  = "REDIS_URL"
        value = local.redis_url
      }
      env {
        name  = "REDIS_AUTH"
        value = google_redis_instance.portal_redis.auth_string
      }
      env {
        name  = "DEV_MODE"
        value = "false"
      }
      env {
        name  = "OKTA_DOMAIN"
        value = var.okta_domain
      }
      env {
        name  = "OKTA_ISSUER"
        value = var.okta_issuer
      }
      env {
        name  = "OKTA_CLIENT_ID"
        value = var.okta_client_id
      }
      env {
        name  = "OKTA_REDIRECT_URI"
        value = var.okta_redirect_uri
      }
      env {
        name  = "OKTA_LOGOUT_REDIRECT_URI"
        value = var.okta_logout_redirect_uri
      }
      env {
        name  = "FRONTEND_URL"
        value = var.okta_logout_redirect_uri
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "OKTA_CLIENT_SECRET"
        value_source {
          secret_key_ref {
            secret  = data.google_secret_manager_secret.okta_client_secret.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service" "data_service" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "data-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "300s"

    scaling {
      min_instance_count = 1
      max_instance_count = 2
    }

    vpc_access {
      connector = google_vpc_access_connector.serverless[0].id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = "${local.cloud_run_image_base}/data-service:${local.cloud_run_image_tag}"

      ports {
        container_port = 8082
      }

      env {
        name  = "DB_HOST"
        value = local.db_private_host
      }
      env {
        name  = "DB_PORT"
        value = "5432"
      }
      env {
        name  = "DB_NAME"
        value = var.db_name
      }
      env {
        name  = "DB_USER"
        value = var.db_user
      }
      env {
        name  = "REDIS_URL"
        value = local.redis_url
      }
      env {
        name  = "REDIS_AUTH"
        value = google_redis_instance.portal_redis.auth_string
      }
      env {
        name  = "KAFKA_BROKERS"
        value = var.kafka_brokers
      }
      env {
        name  = "NOTIFICATIONS_TOPIC"
        value = local.notification_topic
      }
      env {
        name  = "NOTIFICATION_CONSUMER_GROUP"
        value = "portal-notification-service"
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service" "file_service" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "file-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "300s"

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    vpc_access {
      connector = google_vpc_access_connector.serverless[0].id
      egress    = "ALL_TRAFFIC"
    }

    containers {
      image = "${local.cloud_run_image_base}/file-service:${local.cloud_run_image_tag}"

      resources {
        limits = {
          cpu    = "1"
          memory = "1Gi"
        }
      }

      ports {
        container_port = 8083
      }

      env {
        name  = "DB_HOST"
        value = local.db_private_host
      }
      env {
        name  = "DB_PORT"
        value = "5432"
      }
      env {
        name  = "DB_NAME"
        value = var.db_name
      }
      env {
        name  = "DB_USER"
        value = var.db_user
      }
      env {
        name  = "USE_LOCAL_STORAGE"
        value = "false"
      }
      env {
        name  = "GCS_BUCKET"
        value = google_storage_bucket.portal_files.name
      }
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "KAFKA_BROKERS"
        value = var.kafka_brokers
      }
      env {
        name  = "NOTIFICATIONS_TOPIC"
        value = local.notification_topic
      }
      env {
        name  = "PARSER_SERVICE_URL"
        value = google_cloud_run_v2_service.parser_service[0].uri
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service" "ai_service" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "ai-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "900s"

    scaling {
      min_instance_count = var.serverless_ai_min_instances
      max_instance_count = 2
    }

    vpc_access {
      connector = google_vpc_access_connector.serverless[0].id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = "${local.cloud_run_image_base}/ai-service:${local.cloud_run_image_tag}"

      resources {
        limits = {
          cpu    = "2"
          memory = "2Gi"
        }
        cpu_idle = false
      }

      ports {
        container_port = 8084
      }

      env {
        name  = "DB_HOST"
        value = local.db_private_host
      }
      env {
        name  = "DB_PORT"
        value = "5432"
      }
      env {
        name  = "DB_NAME"
        value = var.db_name
      }
      env {
        name  = "DB_USER"
        value = var.db_user
      }
      env {
        name  = "REDIS_URL"
        value = local.redis_url
      }
      env {
        name  = "REDIS_AUTH"
        value = google_redis_instance.portal_redis.auth_string
      }
      env {
        name  = "KAFKA_BROKERS"
        value = var.kafka_brokers
      }
      env {
        name  = "AI_REQUESTS_TOPIC"
        value = local.ai_requests_topic
      }
      env {
        name  = "NOTIFICATIONS_TOPIC"
        value = local.notification_topic
      }
      env {
        name  = "AI_CONSUMER_GROUP"
        value = "portal-ai-workers"
      }
      env {
        name  = "AI_WORKER_ENABLED"
        value = "true"
      }
      env {
        name  = "NVIDIA_BASE_URL"
        value = "https://integrate.api.nvidia.com/v1"
      }
      env {
        name  = "NVIDIA_MODEL"
        value = "openai/gpt-oss-120b"
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "NVIDIA_API_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.nvidia_api_key.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service" "analytics_service" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "analytics-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "300s"

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    vpc_access {
      connector = google_vpc_access_connector.serverless[0].id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = "${local.cloud_run_image_base}/analytics-service:${local.cloud_run_image_tag}"

      ports {
        container_port = 8085
      }

      env {
        name  = "DB_HOST"
        value = local.db_private_host
      }
      env {
        name  = "DB_PORT"
        value = "5432"
      }
      env {
        name  = "DB_NAME"
        value = var.db_name
      }
      env {
        name  = "DB_USER"
        value = var.db_user
      }

      env {
        name = "DB_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.db_password.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service" "api_gateway" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "api-gateway"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "900s"

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    vpc_access {
      connector = google_vpc_access_connector.serverless[0].id
      egress    = "ALL_TRAFFIC"
    }

    containers {
      image = "${local.cloud_run_image_base}/api-gateway:${local.cloud_run_image_tag}"

      ports {
        container_port = 8080
      }

      env {
        name  = "AUTH_SERVICE_URL"
        value = google_cloud_run_v2_service.auth_service[0].uri
      }
      env {
        name  = "DATA_SERVICE_URL"
        value = google_cloud_run_v2_service.data_service[0].uri
      }
      env {
        name  = "FILE_SERVICE_URL"
        value = google_cloud_run_v2_service.file_service[0].uri
      }
      env {
        name  = "AI_SERVICE_URL"
        value = google_cloud_run_v2_service.ai_service[0].uri
      }
      env {
        name  = "ANALYTICS_SERVICE_URL"
        value = google_cloud_run_v2_service.analytics_service[0].uri
      }
      env {
        name = "ALLOWED_ORIGINS"
        # Always include the deployed frontend URL plus any extra origins from the variable.
        value = join(",", distinct(concat(
          ["https://frontend-ogukkf7z3q-uc.a.run.app", "http://localhost:3000"],
          var.allowed_origins,
        )))
      }

      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service" "frontend" {
  count    = var.deploy_serverless ? 1 : 0
  name     = "frontend"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.cloud_run_sa[0].email
    timeout         = "300s"

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = "${local.cloud_run_image_base}/frontend:${local.cloud_run_image_tag}"

      ports {
        container_port = 8080
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}

resource "google_cloud_run_v2_service_iam_member" "public_frontend" {
  count    = var.deploy_serverless ? 1 : 0
  location = google_cloud_run_v2_service.frontend[0].location
  name     = google_cloud_run_v2_service.frontend[0].name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_v2_service_iam_member" "public_gateway" {
  count    = var.deploy_serverless ? 1 : 0
  location = google_cloud_run_v2_service.api_gateway[0].location
  name     = google_cloud_run_v2_service.api_gateway[0].name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_v2_service_iam_member" "internal_services_invoker" {
  for_each = var.deploy_serverless ? {
    auth      = google_cloud_run_v2_service.auth_service[0].name
    data      = google_cloud_run_v2_service.data_service[0].name
    file      = google_cloud_run_v2_service.file_service[0].name
    ai        = google_cloud_run_v2_service.ai_service[0].name
    analytics = google_cloud_run_v2_service.analytics_service[0].name
    parser    = google_cloud_run_v2_service.parser_service[0].name
  } : {}

  location = var.region
  name     = each.value
  role     = "roles/run.invoker"
  # Only the Cloud Run service account (used by api-gateway) may invoke internal services.
  # The gateway fetches a Google identity token and presents it as Authorization header.
  member = "serviceAccount:${google_service_account.cloud_run_sa[0].email}"
}
