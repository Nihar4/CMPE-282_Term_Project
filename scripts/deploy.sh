#!/usr/bin/env bash
# =============================================================
# scripts/deploy.sh
# One-shot deploy:
#   1. terraform apply (idempotent)
#   2. Build + push 8 images via Cloud Build
#   3. Apply Kubernetes manifests
#   4. Smoke-test the gateway
# Re-runnable: each step is a no-op if already up-to-date.
# =============================================================
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-enterprise-portal-48689}"
REGION="${REGION:-us-central1}"
CLUSTER="${CLUSTER:-enterprise-portal-cluster}"
NAMESPACE="${NAMESPACE:-enterprise-portal}"
SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null || echo "deploy")"

bold(){ printf "\033[1m%s\033[0m\n" "$*"; }
ok(){ printf "  \033[32m✔\033[0m %s\n" "$*"; }
warn(){ printf "  \033[33m!\033[0m %s\n" "$*"; }

bold "▶ 1/4  Terraform apply"
(
  cd "$(dirname "$0")/../infrastructure/terraform"
  terraform init -upgrade
  terraform apply -auto-approve
)
ok "infrastructure ready"

bold "▶ 2/4  Cloud Build — build & push 8 images"
gcloud builds submit \
  --config=cloudbuild.yaml \
  --substitutions=SHORT_SHA="$SHORT_SHA",_REGION="$REGION",_CLUSTER="$CLUSTER",_AR_REPO=enterprise-portal,_NAMESPACE="$NAMESPACE",_DOMAIN="${DOMAIN:-portal.example.com}" \
  --project "$PROJECT_ID"
ok "images pushed: $SHORT_SHA"

bold "▶ 3/4  kubectl apply"
gcloud container clusters get-credentials "$CLUSTER" --region "$REGION" --project "$PROJECT_ID"
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "$(dirname "$0")/../infrastructure/k8s/" -n "$NAMESPACE"
kubectl rollout status deploy --timeout=10m -n "$NAMESPACE" || warn "some rollouts still progressing"
ok "kubernetes applied"

bold "▶ 4/4  Smoke test"
sleep 20
INGRESS_IP="$(kubectl get svc -n "$NAMESPACE" api-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
if [[ -n "${INGRESS_IP:-}" ]]; then
  curl -fsS "http://$INGRESS_IP/api/health" && ok "gateway healthy at $INGRESS_IP"
else
  warn "no ingress IP yet — check 'kubectl get svc -n $NAMESPACE'"
fi

cat <<EOF

🎉  Deploy complete.
   GKE cluster   : $CLUSTER
   Namespace     : $NAMESPACE
   Image tag     : $SHORT_SHA
   AR repo       : ${REGION}-docker.pkg.dev/$PROJECT_ID/enterprise-portal

Next:
  • Point your domain's A-record at the LB IP shown by:
       terraform output -raw lb_ip
  • Open Jenkins via:
       gcloud compute ssh jenkins-server --tunnel-through-iap \\
         --zone us-central1-a -- -L 8080:localhost:8080
EOF
