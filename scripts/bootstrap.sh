#!/usr/bin/env bash
# =============================================================
# scripts/bootstrap.sh
# One-time GCP project bootstrap:
#   1. Enable bare-minimum APIs needed by Terraform itself.
#   2. Create the Terraform-state GCS bucket.
#   3. (Optional) zip + upload Cloud Function source.
# Run this BEFORE `terraform init`.
# =============================================================
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-enterprise-portal-48689}"
REGION="${REGION:-us-central1}"
TFSTATE_BUCKET="enterprise-portal-tfstate-${PROJECT_ID#enterprise-portal-}"  # matches main.tf backend
FUNCTION_DIR="$(cd "$(dirname "$0")"/../infrastructure/terraform/functions && pwd)"

echo "▶ Project: $PROJECT_ID   Region: $REGION"
gcloud config set project "$PROJECT_ID" >/dev/null

echo "▶ Enabling bootstrap APIs (cheap to call repeatedly)…"
gcloud services enable \
  cloudresourcemanager.googleapis.com \
  iam.googleapis.com \
  serviceusage.googleapis.com \
  storage.googleapis.com \
  cloudfunctions.googleapis.com \
  cloudbuild.googleapis.com \
  --project "$PROJECT_ID"

echo "▶ Creating Terraform state bucket: gs://$TFSTATE_BUCKET (if needed)…"
if ! gsutil ls -b "gs://$TFSTATE_BUCKET" >/dev/null 2>&1; then
  gsutil mb -p "$PROJECT_ID" -l "$REGION" -b on "gs://$TFSTATE_BUCKET"
  gsutil versioning set on "gs://$TFSTATE_BUCKET"
fi

echo "▶ Packaging Cloud Function (file-ingest-fn)…"
(
  cd "$FUNCTION_DIR"
  rm -f file-ingest-fn.zip
  zip -q -r file-ingest-fn.zip main.py requirements.txt
  echo "  → $(pwd)/file-ingest-fn.zip"
)

cat <<EOF

✅ Bootstrap complete.
Next:
  cd infrastructure/terraform
  cp terraform.tfvars.example terraform.tfvars
  # fill in db_password / nvidia_api_key / auth0_client_secret
  terraform init
  terraform plan -out=plan.bin
  terraform apply plan.bin
EOF
