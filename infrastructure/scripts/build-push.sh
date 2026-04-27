#!/usr/bin/env bash
# ============================================================
# Build and push all Docker images to Artifact Registry.
# Run this manually or from Jenkins.
# Usage: ./infrastructure/scripts/build-push.sh [image-tag]
# ============================================================
set -euo pipefail

PROJECT_ID="enterprise-portal-48689"
REGION="us-central1"
AR_REPO="${REGION}-docker.pkg.dev/${PROJECT_ID}/enterprise-portal"
TAG="${1:-$(git rev-parse --short HEAD)-local}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

echo "Building tag: ${TAG}"
echo "Registry: ${AR_REPO}"

# Authenticate
gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

SERVICES=("api-gateway" "auth-service" "data-service" "file-service" "ai-service" "analytics-service" "parser-service")

for svc in "${SERVICES[@]}"; do
  echo ""
  echo ">>> Building ${svc}..."
  docker build \
    --tag "${AR_REPO}/${svc}:${TAG}" \
    --tag "${AR_REPO}/${svc}:latest" \
    --cache-from "${AR_REPO}/${svc}:latest" \
    "${ROOT}/backend/${svc}"

  echo ">>> Pushing ${svc}..."
  docker push "${AR_REPO}/${svc}:${TAG}"
  docker push "${AR_REPO}/${svc}:latest"
done

# Frontend
echo ""
echo ">>> Building frontend..."
docker build \
  --build-arg REACT_APP_API_URL="https://portal.yourdomain.com" \
  --build-arg REACT_APP_OKTA_ISSUER="${OKTA_ISSUER:-https://dev-example.okta.com/oauth2/default}" \
  --build-arg REACT_APP_OKTA_CLIENT_ID="${OKTA_CLIENT_ID:-your-okta-client-id}" \
  --build-arg REACT_APP_OKTA_REDIRECT_URI="${OKTA_REDIRECT_URI:-https://portal.yourdomain.com/authorization-code/callback}" \
  --build-arg REACT_APP_OKTA_LOGOUT_REDIRECT_URI="${OKTA_LOGOUT_REDIRECT_URI:-https://portal.yourdomain.com}" \
  --tag "${AR_REPO}/frontend:${TAG}" \
  --tag "${AR_REPO}/frontend:latest" \
  --cache-from "${AR_REPO}/frontend:latest" \
  "${ROOT}/frontend"

docker push "${AR_REPO}/frontend:${TAG}"
docker push "${AR_REPO}/frontend:latest"

echo ""
echo "All images pushed with tag: ${TAG}"
