#!/usr/bin/env bash
# ============================================================
# Deploy latest images to GKE.
# Usage: ./infrastructure/scripts/deploy-k8s.sh [image-tag]
# ============================================================
set -euo pipefail

PROJECT_ID="enterprise-portal-48689"
REGION="us-central1"
CLUSTER="enterprise-portal-cluster"
NAMESPACE="enterprise-portal"
AR_REPO="${REGION}-docker.pkg.dev/${PROJECT_ID}/enterprise-portal"
TAG="${1:-latest}"

# Get kubectl credentials
gcloud container clusters get-credentials "${CLUSTER}" \
  --region "${REGION}" --project "${PROJECT_ID}"

echo "Deploying tag ${TAG} to ${CLUSTER}/${NAMESPACE}"

# Apply base resources
kubectl apply -f infrastructure/k8s/namespace.yaml
kubectl apply -f infrastructure/k8s/configmap.yaml

# Update images
SERVICES=("api-gateway" "auth-service" "data-service" "file-service" "ai-service" "analytics-service" "frontend")
for svc in "${SERVICES[@]}"; do
  echo "  Updating ${svc}..."
  kubectl set image "deployment/${svc}" \
    "${svc}=${AR_REPO}/${svc}:${TAG}" \
    -n "${NAMESPACE}" 2>/dev/null || true
done

# Apply all manifests
kubectl apply -f infrastructure/k8s/api-gateway.yaml
kubectl apply -f infrastructure/k8s/frontend.yaml
kubectl apply -f infrastructure/k8s/services.yaml
kubectl apply -f infrastructure/k8s/ingress.yaml
kubectl apply -f infrastructure/k8s/pdb.yaml

# Wait for rollout
for svc in "${SERVICES[@]}"; do
  echo "  Waiting for ${svc}..."
  kubectl rollout status "deployment/${svc}" -n "${NAMESPACE}" --timeout=120s || true
done

echo ""
echo "=== Deployment status ==="
kubectl get pods -n "${NAMESPACE}"
kubectl get svc  -n "${NAMESPACE}"
kubectl get ingress -n "${NAMESPACE}"
