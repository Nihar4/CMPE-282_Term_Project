# ============================================================
# Cloud KMS — Customer-Managed Encryption Keys (CMEK)
# Used to encrypt: GCS buckets, Cloud SQL backups, Pub/Sub messages,
# Secret Manager (envelope) and any disk we wish to wrap with CMEK.
# ============================================================

resource "google_kms_key_ring" "portal_keyring" {
  name       = "enterprise-portal-keyring"
  location   = var.region
  depends_on = [google_project_service.apis]
}

resource "google_kms_crypto_key" "portal_data_key" {
  name            = "portal-data-key"
  key_ring        = google_kms_key_ring.portal_keyring.id
  rotation_period = "7776000s" # 90 days
  purpose         = "ENCRYPT_DECRYPT"

  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = "SOFTWARE"
  }

  lifecycle {
    prevent_destroy = false
  }
}

resource "google_kms_crypto_key" "portal_secret_key" {
  name            = "portal-secret-key"
  key_ring        = google_kms_key_ring.portal_keyring.id
  rotation_period = "2592000s" # 30 days for secrets
  purpose         = "ENCRYPT_DECRYPT"
}

# Allow GCS service to use the data key
data "google_storage_project_service_account" "gcs_account" {}

resource "google_kms_crypto_key_iam_member" "gcs_kms_use" {
  crypto_key_id = google_kms_crypto_key.portal_data_key.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${data.google_storage_project_service_account.gcs_account.email_address}"
}

# Allow Cloud SQL service to use the data key
data "google_project" "current" {}

resource "google_kms_crypto_key_iam_member" "sql_kms_use" {
  count         = 0 # Enable once Cloud SQL service identity is verified in this project
  crypto_key_id = google_kms_crypto_key.portal_data_key.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-cloud-sql.iam.gserviceaccount.com"
}

# Allow Pub/Sub service to use the data key
resource "google_kms_crypto_key_iam_member" "pubsub_kms_use" {
  crypto_key_id = google_kms_crypto_key.portal_data_key.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
