# ============================================================
# Static-asset bucket for the React build (served via Cloud CDN).
# Note: this is separate from `google_storage_bucket.portal_files`
# defined in main.tf which is used for user uploads.
# ============================================================

resource "google_storage_bucket" "portal_static" {
  name          = "${var.gcs_bucket_name}-static-${var.project_id}"
  location      = var.region
  force_destroy = true

  uniform_bucket_level_access = true

  website {
    main_page_suffix = "index.html"
    not_found_page   = "index.html"
  }

  cors {
    origin          = ["*"]
    method          = ["GET", "HEAD"]
    response_header = ["*"]
    max_age_seconds = 86400
  }
}

resource "google_storage_bucket_iam_member" "portal_static_public" {
  bucket = google_storage_bucket.portal_static.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}
