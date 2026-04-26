# Deployment Guide

This guide walks through every deployment surface of the Enterprise Knowledge
Portal — local Docker Compose, single-host Kubernetes (kind/minikube), and the
**production target on GCP with GKE + Jenkins**.

> TL;DR (production target):
>
> ```bash
> make infra-up        # Terraform: VPC, GKE, Cloud SQL, GCS, Memorystore, AR
> make docker-push     # Build & push all images to GCR
> make k8s-deploy      # kubectl apply all manifests
> make k8s-status      # verify rollout
> ```

---

## 1. Prerequisites

| Tool                | Version    | Notes                                          |
| ------------------- | ---------- | ---------------------------------------------- |
| Docker Desktop      | 24+        | with at least 6 GB RAM allotted                |
| Docker Compose v2   | bundled    | invoked as `docker compose`                    |
| Go                  | 1.21       | only if rebuilding services locally            |
| Node.js             | 20.x LTS   | for `npm start` against the React app          |
| Python              | 3.11       | only if running `parser-service` natively      |
| `gcloud` CLI        | latest     | authenticated to your GCP project              |
| `kubectl`           | 1.28+      | matches GKE control plane                      |
| Terraform           | 1.5+       |                                                |
| Make                | any        | optional but recommended                       |

GCP APIs that **must** be enabled (Terraform enables them automatically):

```
container.googleapis.com
sqladmin.googleapis.com
storage.googleapis.com
servicenetworking.googleapis.com
redis.googleapis.com
artifactregistry.googleapis.com
iam.googleapis.com
cloudresourcemanager.googleapis.com
compute.googleapis.com
```

---

## 2. Local — Docker Compose

`docker-compose.yml` boots the entire stack: Postgres, Redis, Kafka/Zookeeper,
all 6 backend services, the Python parser, and the React UI.

```bash
cp .env.example .env       # fill in JWT_SECRET, AUTH0_*, NVIDIA_API_KEY, …

make build                 # docker compose build --parallel
make up                    # detached
make seed                  # mock + sample data into postgres

open http://localhost:3000
```

Useful follow-ups:

```bash
make logs                  # tail -f everything
docker compose logs -f api-gateway
docker compose exec postgres psql -U portal_user -d enterprise_portal
make down                  # stop
make clean                 # stop + delete volumes + images
```

### 2.1 Hot-reloading frontend

```bash
cd frontend
npm install --legacy-peer-deps
npm start                  # CRA dev server on :3000 → proxy to API at :8080
```

### 2.2 Running parser-service natively (debugging)

```bash
cd backend/parser-service
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
uvicorn app:app --port 8090 --reload
```

---

## 3. Single-Node Kubernetes (kind / minikube)

```bash
kind create cluster --name portal --config infrastructure/k8s/kind-config.yaml
kubectl config use-context kind-portal

# Build images directly into kind's containerd
kind load docker-image cloud_final_project-api-gateway:latest --name portal
# … repeat per service …

kubectl apply -f infrastructure/k8s/namespace.yaml
kubectl apply -f infrastructure/k8s/configmap.yaml
kubectl apply -f infrastructure/k8s/postgres.yaml
kubectl apply -f infrastructure/k8s/redis.yaml
kubectl apply -f infrastructure/k8s/api-gateway.yaml
kubectl apply -f infrastructure/k8s/services.yaml
kubectl apply -f infrastructure/k8s/frontend.yaml
kubectl apply -f infrastructure/k8s/ingress.yaml
```

Forward the gateway:

```bash
kubectl port-forward -n enterprise-portal svc/api-gateway 8080:8080
```

---

## 4. GCP — Production Target

### 4.1 Configure Terraform

```bash
cd infrastructure/terraform
cp terraform.tfvars.example terraform.tfvars
# edit:
#   project_id      = "enterprise-portal-48689"
#   region          = "us-central1"
#   cluster_name    = "enterprise-portal-cluster"
#   db_password     = "<strong>"
#   gcs_bucket_name = "enterprise-portal-files"

terraform init
terraform plan
terraform apply             # ~10–15 min on a clean project
```

What this provisions:

- VPC + subnet (`10.0.0.0/16`) with secondary ranges for pods/services.
- **GKE Regional cluster** with Workload Identity, HPA, managed Prometheus.
- **Cloud SQL Postgres 15** REGIONAL HA, PITR, private IP.
- **Memorystore Redis 7** STANDARD_HA, TLS.
- **Cloud Storage bucket** with versioning + lifecycle rule.
- **Artifact Registry** Docker repo `enterprise-portal`.

### 4.2 Authenticate kubectl

```bash
gcloud container clusters get-credentials enterprise-portal-cluster \
    --region us-central1 \
    --project $GCP_PROJECT_ID
```

### 4.3 Push images

Either via Make:

```bash
make docker-push
```

…or via Jenkins (recommended — see [`CICD.md`](./CICD.md)).

The script tags each image as both `:latest` and `:<git-sha>-<build>` and
pushes to `gcr.io/$GCP_PROJECT_ID/enterprise-portal-<service>`.

### 4.4 Apply Kubernetes manifests

```bash
make k8s-deploy
# = kubectl apply -f infrastructure/k8s/namespace.yaml
#   kubectl apply -f infrastructure/k8s/configmap.yaml
#   kubectl apply -f infrastructure/k8s/
#   kubectl rollout status deployment -n enterprise-portal
```

What gets created:

| Object                                  | Purpose                              |
| --------------------------------------- | ------------------------------------ |
| `Namespace enterprise-portal`           | Isolation                            |
| `ConfigMap portal-config`               | Non-secret env (URLs, models, …)     |
| `Secret portal-secrets`                 | DB pwd, JWT, NVIDIA key, Auth0 secret|
| `Deployment / Service` × 7 microservices| Workloads                            |
| `HorizontalPodAutoscaler` per service   | CPU-based scaling                    |
| `Deployment / Service frontend`         | Nginx + React build                  |
| `Ingress portal-ingress`                | Single LB, TLS, path routing         |

### 4.5 Configure DNS + TLS

1. Reserve a global IP:
   ```bash
   gcloud compute addresses create portal-ip --global
   ```
2. Update your DNS A record (`portal.yourdomain.com`) to that IP.
3. Edit `infrastructure/k8s/ingress.yaml` so `host:` matches your domain.
4. Apply — Google-managed cert provisions in 10–15 min.

### 4.6 Database migrations on the cluster

```bash
kubectl run migration-job-$(date +%s) \
  --image=postgres:15-alpine --restart=Never \
  --namespace=enterprise-portal \
  --env="PGPASSWORD=$(kubectl get secret portal-secrets \
        -n enterprise-portal -o jsonpath='{.data.DB_PASSWORD}' | base64 -d)" \
  --command -- psql -h portal-postgres -U portal_user -d enterprise_portal \
                    -f /migrations/001_init.sql
```

In Jenkins this is gated behind `RUN_MIGRATIONS=true`.

### 4.7 Smoke test

```bash
INGRESS_IP=$(kubectl -n enterprise-portal get ingress portal-ingress \
            -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
curl -k https://$INGRESS_IP/api/health
```

---

## 5. Jenkins on GKE (Cloud Jenkins)

### 5.1 Install via Helm

```bash
kubectl create namespace jenkins
helm repo add jenkinsci https://charts.jenkins.io
helm install jenkins jenkinsci/jenkins -n jenkins \
  --set controller.installPlugins='{kubernetes,workflow-aggregator,git,configuration-as-code,oic-auth,blueocean,gcp-secret-manager}' \
  --set persistence.size=20Gi \
  --set controller.serviceType=LoadBalancer
```

### 5.2 Required credentials (Jenkins → Manage Credentials)

| ID                          | Type      | Value                                                    |
| --------------------------- | --------- | -------------------------------------------------------- |
| `GCP_PROJECT_ID`            | secret    | `enterprise-portal-48689`                                |
| `GCP_SERVICE_ACCOUNT_KEY`   | file      | JSON key for `jenkins-deployer@<proj>.iam.gserviceaccount.com` |
| `TF_DB_PASSWORD`            | secret    | DB password (matches Cloud SQL user)                     |
| `GITHUB_TOKEN`              | username/token | Used for status checks & repo cloning if private    |
| `OKTA_OIDC_CLIENT`          | secret    | Used by `oic-auth` for SSO login                         |

### 5.3 Pipeline job

- "New Item" → "Multibranch Pipeline" → Branch source: GitHub
  → Repo: `Nihar4/CMPE-282_Term_Project`.
- Discover branches: All.
- Build configuration: by `Jenkinsfile`.
- Webhook: GitHub → Jenkins URL `/github-webhook/`.

The pipeline is described in detail in [`CICD.md`](./CICD.md).

### 5.4 Connecting Okta SSO to Jenkins

1. Create an Okta OIDC application of type **Web Application** with:
   - Sign-in redirect: `https://jenkins.example.com/securityRealm/finishLogin`
   - Sign-out redirect: `https://jenkins.example.com/`
2. In Jenkins → Configure Global Security → SAML / OIDC:
   - Use the `oic-auth` plugin.
   - Provider: Okta `https://<tenant>.okta.com`.
   - Group claim: `groups` → assign Jenkins roles via the Role Strategy plugin.
3. Disable "Jenkins's own user database" once SSO works.

---

## 6. Rolling Update & Rollback

### 6.1 Update a single service

```bash
kubectl set image deployment/data-service \
  data-service=gcr.io/$GCP_PROJECT_ID/enterprise-portal-data-service:abc123 \
  -n enterprise-portal --record
kubectl rollout status deployment/data-service -n enterprise-portal
```

### 6.2 Rollback

```bash
kubectl rollout undo deployment/data-service -n enterprise-portal
```

### 6.3 Auto-rollback in pipeline

Already wired in `Jenkinsfile`'s `post.failure` block — every deployment is
undone on pipeline failure.

---

## 7. Backup & Disaster Recovery

| Asset            | Strategy                                                          |
| ---------------- | ----------------------------------------------------------------- |
| Cloud SQL DB     | PITR + 30 days of automated backups (`backup_retention_settings`). |
| GCS bucket       | Object versioning enabled; lifecycle deletes after 365 days.       |
| Cluster state    | All workloads are idempotent (manifests in Git). Re-`kubectl apply` from `main`. |
| Terraform state  | Optional GCS backend (uncomment in `main.tf`) for shared state.    |

---

## 8. Cost Estimate (rough)

| Resource                              | Approx monthly cost (USD) |
| ------------------------------------- | -------------------------: |
| GKE Regional, 3 e2-standard-4 nodes   | ~$220                      |
| Cloud SQL HA `db-custom-2-7680`       | ~$200                      |
| Memorystore 2 GB STANDARD_HA          | ~$70                       |
| Cloud Storage 100 GB + ops            | ~$3                        |
| Cloud Logging + Managed Prom          | ~$15                       |
| Egress (educational demo)             | ~$5                        |
| **Total**                             | **~$510/mo**               |

For a class demo, scaling node pool down to 1 e2-medium and Cloud SQL to a
shared-core tier brings the total under **$80/mo**.

---

## 9. Tear-down

```bash
make k8s-status                # confirm what's running
kubectl delete ns enterprise-portal
make infra-down                # terraform destroy (force-delete protection off)
```

> Cloud SQL has `deletion_protection = true`. Set it to `false`, re-apply, then
> destroy.
