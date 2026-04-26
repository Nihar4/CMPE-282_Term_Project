# ============================================================
# Cloud Build — Alternative / complementary CI to Jenkins.
# Builds a service image from a GitHub trigger, pushes to
# Artifact Registry, then triggers GKE rollout. This complements
# Jenkins so the team can pick whichever is simpler per-PR.
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
    "roles/container.developer",
    "roles/run.developer",
    "roles/secretmanager.secretAccessor",
    "roles/iam.serviceAccountUser",
    "roles/logging.logWriter",
  ])
  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${google_service_account.cloud_build_sa.email}"
}

# Trigger: rebuild on push to main
resource "google_cloudbuild_trigger" "main_branch" {
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

  filename = "cloudbuild.yaml"

  included_files = [
    "backend/**",
    "frontend/**",
    "infrastructure/k8s/**",
    "cloudbuild.yaml",
  ]

  depends_on = [google_project_service.apis]
}

# Trigger: PR validation
resource "google_cloudbuild_trigger" "pr_validation" {
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
