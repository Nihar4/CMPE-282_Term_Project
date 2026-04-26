#!/usr/bin/env bash
# ============================================================
# Enterprise Knowledge Portal — One-Command GCP Setup
# Run this ONCE to provision all infrastructure.
# Usage: ./infrastructure/scripts/setup-gcp.sh
# ============================================================
set -euo pipefail

PROJECT_ID="enterprise-portal-48689"
REGION="us-central1"
TFSTATE_BUCKET="enterprise-portal-tfstate-${PROJECT_ID}"

echo "================================================================"
echo " Enterprise Portal — GCP Infrastructure Setup"
echo " Project: ${PROJECT_ID}"
echo " Region:  ${REGION}"
echo "================================================================"

# ── 1. Verify prerequisites ─────────────────────────────────────────────────
command -v gcloud   >/dev/null 2>&1 || { echo "ERROR: gcloud not installed"; exit 1; }
command -v terraform >/dev/null 2>&1 || { echo "ERROR: terraform not installed"; exit 1; }
command -v kubectl  >/dev/null 2>&1 || { echo "ERROR: kubectl not installed"; exit 1; }
command -v docker   >/dev/null 2>&1 || { echo "ERROR: docker not installed"; exit 1; }

# ── 2. Authenticate with GCP ────────────────────────────────────────────────
echo ""
echo ">>> Step 1: GCP Authentication"
gcloud auth login --quiet || true
gcloud config set project "${PROJECT_ID}"
gcloud config set compute/region "${REGION}"

# ── 3. Create Terraform state bucket (must exist before terraform init) ─────
echo ""
echo ">>> Step 2: Creating Terraform state bucket"
gsutil mb -p "${PROJECT_ID}" -c STANDARD -l "${REGION}" \
  "gs://${TFSTATE_BUCKET}" 2>/dev/null || echo "  (bucket already exists)"
gsutil versioning set on "gs://${TFSTATE_BUCKET}"

# ── 4. Run Terraform ────────────────────────────────────────────────────────
echo ""
echo ">>> Step 3: Running Terraform"
cd "$(dirname "$0")/../../infrastructure/terraform"

terraform init -input=false

echo ""
echo "Enter secrets (will be stored in GCP Secret Manager):"
read -rsp "DB Password [portal_P@ssw0rd_Secure_2024!]: " DB_PASS
DB_PASS="${DB_PASS:-portal_P@ssw0rd_Secure_2024!}"
read -rsp "JWT Secret: " JWT_SECRET
JWT_SECRET="${JWT_SECRET:-enterprise-portal-super-secret-jwt-key-2024}"
read -rsp "NVIDIA API Key: " NVIDIA_KEY
read -rsp "Auth0 Client Secret: " AUTH0_SECRET
echo ""

terraform apply -auto-approve \
  -var="db_password=${DB_PASS}" \
  -var="jwt_secret=${JWT_SECRET}" \
  -var="nvidia_api_key=${NVIDIA_KEY}" \
  -var="auth0_client_secret=${AUTH0_SECRET}"

# ── 5. Get Terraform outputs ─────────────────────────────────────────────────
echo ""
echo ">>> Step 4: Collecting infrastructure outputs"
REDIS_HOST=$(terraform output -raw redis_host 2>/dev/null || echo "")
SQL_CONN=$(terraform output -raw cloud_sql_connection_name 2>/dev/null || echo "")
AR_URL=$(terraform output -raw artifact_registry_url 2>/dev/null || echo "")

echo "  Redis Host : ${REDIS_HOST}"
echo "  SQL Conn   : ${SQL_CONN}"
echo "  AR URL     : ${AR_URL}"

# Update configmap with real Redis host
if [ -n "${REDIS_HOST}" ]; then
  cd "$(dirname "$0")/../.."
  sed -i.bak "s|REPLACE_WITH_MEMORYSTORE_IP|${REDIS_HOST}|g" \
    infrastructure/k8s/configmap.yaml
  echo "  Updated configmap.yaml with Memorystore IP"
fi

# ── 6. Configure kubectl ─────────────────────────────────────────────────────
echo ""
echo ">>> Step 5: Configuring kubectl"
gcloud container clusters get-credentials enterprise-portal-cluster \
  --region "${REGION}" --project "${PROJECT_ID}"

# ── 7. Apply K8s manifests ───────────────────────────────────────────────────
echo ""
echo ">>> Step 6: Deploying to GKE"
kubectl apply -f infrastructure/k8s/namespace.yaml
kubectl apply -f infrastructure/k8s/configmap.yaml
kubectl apply -f infrastructure/k8s/api-gateway.yaml
kubectl apply -f infrastructure/k8s/frontend.yaml
kubectl apply -f infrastructure/k8s/services.yaml
kubectl apply -f infrastructure/k8s/ingress.yaml
kubectl apply -f infrastructure/k8s/pdb.yaml

echo ""
echo "================================================================"
echo " Setup complete!"
echo ""
terraform output setup_summary 2>/dev/null || true
echo ""
echo " Next: Push Docker images and trigger Jenkins pipeline."
echo " Jenkins access: gcloud compute ssh jenkins-server --tunnel-through-iap --zone ${REGION}-a -- -L 8080:localhost:8080"
echo "================================================================"
