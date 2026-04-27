# ============================================================
# Cloud Build — serverless deployment runner invoked by Jenkins.
# Jenkins receives GitHub webhooks, then runs `gcloud builds submit`
# with cloudbuild-serverless.yaml. Optional triggers stay disabled by
# default and use the same Cloud Run-only config if enabled later.
# ============================================================

# Cloud Build service account (separate from default)
resource "google_service_account" "cloud_build_sa" {
  account_id   = "enterprise-portal-build"
  display_name = "Enterprise Portal Cloud Build SA"
}

resource "google_project_iam_member" "cloud_build_sa_roles" {
  for_each = toset([
    "roles/artifactregistry.writer",
    "roles/storage.objectAdmin",
    "roles/run.admin",
    "roles/cloudsql.admin",
    "roles/redis.admin",
    "roles/compute.networkAdmin",
    "roles/pubsub.admin",
    "roles/secretmanager.secretAccessor",
    "roles/logging.configWriter",
    "roles/resourcemanager.projectIamAdmin",
    "roles/iam.serviceAccountUser",
    "roles/logging.logWriter",
  ])
  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${google_service_account.cloud_build_sa.email}"
}

resource "google_project_iam_member" "cloud_build_default_sa_roles" {
  for_each = toset([
    "roles/artifactregistry.writer",
    "roles/storage.admin",
    "roles/run.admin",
    "roles/cloudsql.admin",
    "roles/redis.admin",
    "roles/compute.networkAdmin",
    "roles/pubsub.admin",
    "roles/secretmanager.admin",
    "roles/iap.admin",
    "roles/logging.configWriter",
    "roles/resourcemanager.projectIamAdmin",
    "roles/iam.serviceAccountAdmin",
    "roles/iam.serviceAccountUser",
    "roles/cloudbuild.builds.builder",
    "roles/logging.logWriter",
  ])
  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${data.google_project.current.number}@cloudbuild.gserviceaccount.com"
}

# Trigger: rebuild on push to main
resource "google_cloudbuild_trigger" "main_branch" {
  count       = var.enable_cloud_build_triggers ? 1 : 0
  name        = "enterprise-portal-main-build"
  description = "Build & deploy on push to main branch"
  location    = var.region

  service_account = google_service_account.cloud_build_sa.id

  github {
    owner = var.github_owner
    name  = var.github_repo
    push {
      branch = "^main$"
    }
  }

  filename = "cloudbuild-serverless.yaml"

  included_files = [
    "backend/**",
    "frontend/**",
    "infrastructure/terraform/**",
    "cloudbuild-serverless.yaml",
  ]

  depends_on = [google_project_service.apis]
}

# Trigger: PR validation
resource "google_cloudbuild_trigger" "pr_validation" {
  count       = var.enable_cloud_build_triggers ? 1 : 0
  name        = "enterprise-portal-pr-validation"
  description = "Run tests + lint on every PR"
  location    = var.region

  service_account = google_service_account.cloud_build_sa.id

  github {
    owner = var.github_owner
    name  = var.github_repo
    pull_request {
      branch          = "^main$"
      comment_control = "COMMENTS_ENABLED_FOR_EXTERNAL_CONTRIBUTORS_ONLY"
    }
  }

  filename = "cloudbuild-pr.yaml"
}
