#!/usr/bin/env bash
# =============================================================
# scripts/deploy.sh
# One-shot serverless deploy:
#   1. Terraform apply (idempotent base infra)
#   2. Build, push, and deploy all images via Cloud Build -> Cloud Run
#   3. Print Cloud Run service URLs
# Re-runnable: each step is a no-op if already up-to-date.
# =============================================================
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-enterprise-portal-48689}"
REGION="${REGION:-us-central1}"
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo "deploy")"

bold(){ printf "\033[1m%s\033[0m\n" "$*"; }
ok(){ printf "  \033[32m✔\033[0m %s\n" "$*"; }
warn(){ printf "  \033[33m!\033[0m %s\n" "$*"; }

# Load local environment values (API keys, auth secrets) when present.
if [[ -f "$(dirname "$0")/../.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$(dirname "$0")/../.env"
  set +a
fi

export TF_VAR_db_password="${TF_VAR_db_password:-${DB_PASSWORD:-}}"
export TF_VAR_jwt_secret="${TF_VAR_jwt_secret:-${JWT_SECRET:-}}"
export TF_VAR_nvidia_api_key="${TF_VAR_nvidia_api_key:-${NVIDIA_API_KEY:-}}"
export TF_VAR_okta_client_secret="${TF_VAR_okta_client_secret:-${OKTA_CLIENT_SECRET:-}}"
export TF_VAR_enable_cloud_build_triggers="${TF_VAR_enable_cloud_build_triggers:-false}"

ensure_terraform() {
  if command -v terraform >/dev/null 2>&1; then
    echo "$(command -v terraform)"
    return 0
  fi

  warn "terraform not found in PATH; downloading a local binary"
  local os arch tf_version tf_zip tf_url tmp_dir
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    arm64|aarch64) arch="arm64" ;;
    x86_64|amd64) arch="amd64" ;;
    *)
      echo "Unsupported architecture: $arch" >&2
      return 1
      ;;
  esac

  tf_version="${TF_VERSION:-1.9.8}"
  tf_zip="terraform_${tf_version}_${os}_${arch}.zip"
  tf_url="https://releases.hashicorp.com/terraform/${tf_version}/${tf_zip}"
  tmp_dir="$(mktemp -d)"
  curl -fsSL -o "${tmp_dir}/${tf_zip}" "$tf_url"
  unzip -q -o "${tmp_dir}/${tf_zip}" -d "$tmp_dir"
  chmod +x "${tmp_dir}/terraform"
  echo "${tmp_dir}/terraform"
}

TF_BIN="$(ensure_terraform)"
export TF_BIN

bold "▶ 1/3  Terraform apply"
(
  cd "$(dirname "$0")/../infrastructure/terraform"
  "$TF_BIN" init -upgrade
  "$TF_BIN" apply -auto-approve \
    -var="project_id=${PROJECT_ID}" \
    -var="region=${REGION}" \
    -var="deploy_serverless=true"
)
ok "infrastructure ready"

bold "▶ 2/3  Cloud Build — build, push, and deploy to Cloud Run"
gcloud builds submit \
  --config=cloudbuild-serverless.yaml \
  --substitutions="_PROJECT_ID=${PROJECT_ID},_REGION=${REGION},_KAFKA_BROKERS=${KAFKA_BROKERS:-REPLACE_WITH_MANAGED_KAFKA_BOOTSTRAP:9092},_REACT_APP_API_URL=${REACT_APP_API_URL:-http://localhost:8080},_OKTA_ISSUER=${OKTA_ISSUER:-https://trial-5413467.okta.com/oauth2/default},_OKTA_CLIENT_ID=${OKTA_CLIENT_ID:-0oa12cfmwjeBVrl0I698},_OKTA_REDIRECT_URI=${OKTA_REDIRECT_URI:-http://localhost:3000/authorization-code/callback},_OKTA_LOGOUT_REDIRECT_URI=${OKTA_LOGOUT_REDIRECT_URI:-http://localhost:3000},_IMAGE_TAG=${SHORT_SHA}" \
  --project "$PROJECT_ID"
ok "Cloud Build submitted and Cloud Run deployment completed"

bold "▶ 3/3  Cloud Run URLs"
FRONTEND_URL="$(gcloud run services describe frontend --region "$REGION" --project "$PROJECT_ID" --format='value(status.url)' 2>/dev/null || true)"
GATEWAY_URL="$(gcloud run services describe api-gateway --region "$REGION" --project "$PROJECT_ID" --format='value(status.url)' 2>/dev/null || true)"
[[ -n "$FRONTEND_URL" ]] && ok "frontend: $FRONTEND_URL" || warn "frontend URL not available yet"
[[ -n "$GATEWAY_URL" ]] && ok "api-gateway: $GATEWAY_URL" || warn "api-gateway URL not available yet"

cat <<EOF

Deploy complete.
   Runtime       : Cloud Run
   Image tag     : $SHORT_SHA
   AR repo       : ${REGION}-docker.pkg.dev/$PROJECT_ID/enterprise-portal
   Frontend      : ${FRONTEND_URL:-not available}
   API Gateway   : ${GATEWAY_URL:-not available}

Next:
  • Add the Cloud Run frontend URL to Okta sign-in/sign-out redirect URIs.
  • Open Jenkins via:
       gcloud compute ssh jenkins-server --tunnel-through-iap \\
         --zone us-central1-a -- -L 8080:localhost:8080
EOF
