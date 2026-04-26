# Infrastructure Reference — GCP Services Used

> Cloud-native deployment topology for the **Enterprise Knowledge Portal**
> (CMPE-282 term project, Serverless Squad).
> This document is the single source of truth for **every GCP service** the
> project provisions, **why** we chose it, **how** it is wired, and the
> **Terraform / Helm / kubectl** entrypoints that manage it.

---

## 0. At-a-glance topology

```
                      ┌──────────────────────────────────────────────┐
                      │            Users (browsers / API)            │
                      └───────────────┬──────────────────────────────┘
                                      │ HTTPS (TLS 1.3)
                                      ▼
              ┌───────────────────────────────────────────────────────┐
              │ Cloud DNS  →  Cert Manager (managed SSL)              │
              │ Global HTTPS LB + Cloud CDN + Cloud Armor (WAF/DDoS)  │
              └───────────────────────────┬───────────────────────────┘
                              ┌───────────┴────────────┐
                              ▼                        ▼
                  ┌──────────────────────┐    ┌─────────────────────┐
                  │   GCS (static React) │    │   GKE Ingress       │
                  │   serves /index.html │    │   (Internal LB)     │
                  └──────────────────────┘    └────────┬────────────┘
                                                       ▼
                            ┌────────────────────────────────────────┐
                            │            GKE Regional Cluster        │
                            │   (private nodes, Workload Identity)   │
                            │  ─ api-gateway / auth / data / file    │
                            │  ─ ai / analytics / parser  (HPA)      │
                            └─────────┬─────────────┬────────────────┘
                                      │             │
                                      ▼             ▼
                            ┌──────────────┐ ┌──────────────────┐
                            │  Cloud SQL   │ │ Cloud Memorystore│
                            │  PostgreSQL  │ │     Redis 7      │
                            │  (private)   │ │     (private)    │
                            └──────────────┘ └──────────────────┘
                                      │             ▲
                                      ▼             │
                            ┌──────────────────────────────────┐
                            │ Cloud Pub/Sub  (file/ai/notify)  │
                            └─────────────┬────────────────────┘
                ┌───────────────┬─────────┴──────────┬──────────────────┐
                ▼               ▼                    ▼                  ▼
        ┌───────────┐  ┌────────────────┐  ┌──────────────────┐  ┌──────────┐
        │ Cloud Run │  │ Cloud Functions│  │ Cloud Tasks      │  │ BigQuery │
        │ parser-svc│  │ file-ingest    │  │ ai-rerank queue  │  │ analytics│
        └───────────┘  └────────────────┘  └──────────────────┘  └──────────┘

   Cross-cutting: Cloud KMS · Secret Manager · IAP · Cloud Logging
                  Cloud Trace · Cloud Profiler · Cloud Monitoring · IAM
   CI/CD:         Jenkins (GCE or GKE) · Cloud Build (alt) · Artifact Reg
```

---

## 1. Compute & containers

| Service                 | Resource (TF)                              | Purpose                                                        |
| ----------------------- | ------------------------------------------ | -------------------------------------------------------------- |
| **GKE Regional Cluster**| `google_container_cluster.portal_cluster`  | Runs all 7 microservices + ingress controllers; private nodes  |
| **GKE Node Pool**       | `google_container_node_pool.portal_nodes`  | `e2-standard-4`, autoscale `2 → 10` per zone, surge upgrades   |
| **Cloud Run (Gen2)**    | `google_cloud_run_v2_service.parser_service` | Serverless burst tier for parser-service (scale-to-zero)     |
| **Cloud Functions Gen2**| `google_cloudfunctions2_function.file_ingest` | Reacts to GCS `object.finalized` → publishes Pub/Sub        |
| **GCE (Jenkins VM)**    | `google_compute_instance.jenkins`          | Single-node CI/CD on Debian 12 + Java 17 + Docker + kubectl    |

### Why GKE *and* Cloud Run?
- **GKE** for the long-running stateful services where we need fine control
  (sidecars, custom probes, service mesh), Workload Identity, and predictable
  cost.
- **Cloud Run** for the **parser-service** because parsing PDFs / DOCXs is
  spiky — we want scale-to-zero and don't want spike traffic to deplete GKE
  CPU quotas. A push subscription on `file_events` delivers work to it.
- **Cloud Functions** for the trivial GCS ➜ Pub/Sub fan-out. No need to keep
  a pod alive for it.

### GKE highlights
- **Regional** cluster — control plane replicated across 3 zones.
- **Private nodes** — pods have no public IPs; egress via Cloud NAT.
- **Workload Identity** — pods authenticate to GCP APIs as
  `enterprise-portal-gke-sa@…` without JSON keys.
- **HPA** + **Managed Prometheus** for autoscaling on CPU + custom metrics.
- **Binary Authorization** stub (set to `DISABLED` for demo, flip on for prod).

---

## 2. Data plane

| Service                       | Resource                                          | Notes                                              |
| ----------------------------- | ------------------------------------------------- | -------------------------------------------------- |
| **Cloud SQL (PostgreSQL 15)** | `google_sql_database_instance.portal_db`         | Regional HA, PITR backups, **private IP only**     |
| **Cloud Memorystore (Redis 7)** | `google_redis_instance.portal_redis`           | `STANDARD_HA`, TLS-ready, AUTH-enabled             |
| **GCS — uploads bucket**      | `google_storage_bucket.portal_files`             | CMEK, versioning, lifecycle to NEARLINE @ 90 d     |
| **GCS — static bucket**       | `google_storage_bucket.portal_static`            | Hosts the React build, public read, CDN-fronted    |
| **GCS — logs bucket**         | `google_storage_bucket.logs_bucket`              | Sink target for `enterprise-portal` namespace logs |
| **GCS — TF state**            | `google_storage_bucket.tfstate`                  | Versioned state bucket (`backend "gcs"`)           |
| **BigQuery dataset**          | `google_bigquery_dataset.portal_analytics`       | 90-day partitioned `events` table                  |

### Cloud SQL specifics
- `availability_type = "REGIONAL"` → automatic failover replica.
- `point_in_time_recovery_enabled = true` (7-day WAL retention).
- `ipv4_enabled = false` — only reachable inside the VPC.
- Insights enabled (`record_application_tags`, `record_client_address`).
- Read-only DB user (`portal_user_ro`) used by `ai-service` for NL→SQL safety.

### Memorystore specifics
- `STANDARD_HA` tier (failover replica in another zone).
- AUTH enabled, transit-encryption optional (set
  `SERVER_AUTHENTICATION` for prod).
- LRU policy for cache-friendly eviction.

### BigQuery wiring
- A **Pub/Sub-to-BigQuery** subscription (`ai_events_to_bq`) writes every AI
  event row into `events`. No Dataflow needed.
- Schema: `event_id, event_time, user_id, event_type, service, payload`.
- Partitioned by `event_time` (DAY) — keeps queries cheap.

---

## 3. Messaging & event-driven glue

| Resource                                       | Description                                              |
| ---------------------------------------------- | -------------------------------------------------------- |
| `google_pubsub_topic.file_events`              | Emitted on file uploads / deletes                        |
| `google_pubsub_subscription.file_events_sub`   | Pull subscription for Go file-service                    |
| `google_pubsub_subscription.parser_push`       | **Push** sub → Cloud Run parser-service                  |
| `google_pubsub_topic.ai_events`                | Emitted on every AI query / response                     |
| `google_pubsub_subscription.ai_events_sub`     | Pull for analytics-service                               |
| `google_pubsub_subscription.ai_events_to_bq`   | BigQuery direct subscription                             |
| `google_pubsub_topic.notification_events`      | Reserved for future SMS / email fan-out                  |
| `google_cloud_tasks_queue.ai_rerank`           | Bounded async queue for delayed AI re-ranking            |
| `google_cloud_scheduler_job.nightly_summary`   | Cron 03:00 PT — POSTs `/analytics/refresh`               |
| `google_cloud_scheduler_job.weekly_backup_check` | Mon 04:00 PT — Pub/Sub `notification_events`           |

---

## 4. Networking, security & SSL

| Service                      | Purpose                                                                    |
| ---------------------------- | -------------------------------------------------------------------------- |
| **VPC + Subnet**             | `enterprise-portal-vpc` with `pods` & `services` secondary ranges          |
| **Jenkins subnet**           | Isolated `/24` for the Jenkins VM                                          |
| **Cloud NAT**                | Outbound internet for private nodes                                        |
| **VPC Peering (private services)** | Cloud SQL + Memorystore reachable on `10.x.x.x`                      |
| **Firewall rules**           | Internal `10.0.0.0/8`, IAP `35.235.240.0/20`, GCP LB health checks         |
| **Cloud Armor**              | XSS-stable, SQLi-stable preconfigured WAF rules + 1k req/min IP rate-limit |
| **Cloud Armor Adaptive Protection** | L7 DDoS auto-mitigation                                             |
| **Identity-Aware Proxy (IAP)** | Tunnels SSH and Jenkins UI without public IP                            |
| **Cloud DNS**                | Managed zone for `portal.yourdomain.com` (DNSSEC on)                       |
| **Cert Manager (managed SSL)** | `google_compute_managed_ssl_certificate.portal_ssl`                      |
| **Global HTTPS LB**          | Anycast IP, HTTPS only, HTTP→HTTPS 301 redirect                            |
| **Cloud CDN**                | Edge cache for the static React bundle                                     |
| **Cloud KMS**                | CMEK for GCS, Cloud SQL, Pub/Sub, Secret envelope                          |
| **Secret Manager**           | `db_password`, `jwt_secret`, `nvidia_api_key`, `auth0_client_secret`       |
| **IAM Service Accounts**     | One SA per workload (least privilege)                                      |

### Service Account inventory (least-privilege)

| SA                             | Used by                | Roles (subset)                                              |
| ------------------------------ | ---------------------- | ----------------------------------------------------------- |
| `enterprise-portal-gke-sa`     | GKE nodes              | logging.logWriter, monitoring.metricWriter, artifactregistry.reader, storage.objectAdmin, secretmanager.secretAccessor, pubsub.{publisher,subscriber}, cloudtrace.agent |
| `enterprise-portal-jenkins`    | Jenkins VM             | container.developer, artifactregistry.writer, storage.objectAdmin, secretmanager.secretAccessor, cloudsql.client |
| `enterprise-portal-build`      | Cloud Build            | artifactregistry.writer, storage.objectAdmin, container.developer, run.developer, iam.serviceAccountUser |
| `enterprise-portal-run-sa`     | Cloud Run + Functions  | storage.objectAdmin, secretmanager.secretAccessor, pubsub.subscriber, cloudtrace.agent |

---

## 5. Observability

| Service               | What we collect                                                            |
| --------------------- | -------------------------------------------------------------------------- |
| **Cloud Logging**     | All container stdout/stderr + Cloud SQL error logs + Cloud NAT errors      |
| **Log sink → GCS**    | `enterprise-portal-log-sink` archives namespace logs for compliance        |
| **Cloud Monitoring**  | GKE / Cloud SQL / Memorystore default dashboards                           |
| **Managed Prometheus**| Scrapes pod `/metrics` — used by HPA custom metrics                        |
| **Cloud Trace**       | Each Go service & parser-service emits OpenTelemetry spans                 |
| **Cloud Profiler**    | Continuous CPU/heap profiles for backend services                          |
| **Alert Policies**    | `high_cpu`, `pod_restarts`, `sql_connections` → email channel              |
| **BigQuery events**   | Long-term funnel queries on usage                                          |

---

## 6. CI / CD

The project ships **two** equivalent pipelines so the team can pick whichever
is simpler in context. Both push to the same Artifact Registry and roll out
to the same GKE cluster.

### 6a — Jenkins (primary)
- **Hosting options**:
  - **Option A (simpler, default)**: GCE VM via `google_compute_instance.jenkins`
    with startup-script bootstrapping Java 17 + Jenkins LTS + Docker + kubectl
    + gcloud + Trivy + Terraform.
  - **Option B (cloud-native)**: Helm chart on the same GKE cluster — see
    `infrastructure/jenkins/values.yaml` and `infrastructure/jenkins/jcasc-config.yaml`.
    Configured via JCasC, with Okta SSO via the `oic-auth` plugin.
- **Pipeline file**: `Jenkinsfile` at the repo root.
- **Stages**: Checkout → SAST (SonarQube hook) → Unit tests (Go + Python +
  React) → Build images → Trivy scan → Push to Artifact Registry → Deploy to
  GKE → Smoke test → Slack/Email notify.
- **Authentication**: Okta OIDC; only `enterprise-portal-devs` group can run
  builds, only `…-admins` can manage Jenkins.

### 6b — Cloud Build (alternative / supplemental)
- **Triggers** (managed by Terraform):
  - `google_cloudbuild_trigger.main_branch` → push-to-main
  - `google_cloudbuild_trigger.pr_validation` → every PR
- **Pipeline files**: `cloudbuild.yaml` (deploy) and `cloudbuild-pr.yaml` (CI).
- Uses the `enterprise-portal-build` SA, machine type `E2_HIGHCPU_8`.
- Builds 8 images in parallel, runs Trivy, renders manifests, applies them to
  GKE, then smoke-tests `https://${DOMAIN}/api/health`.

### 6c — Artifact Registry
- Single `enterprise-portal` Docker repo in `${REGION}`.
- Cleanup policy: keep last 10 versions per image.
- Both Jenkins SA and Cloud Build SA have `artifactregistry.writer`.

---

## 7. AI services

| Service              | Role                                                                     |
| -------------------- | ------------------------------------------------------------------------ |
| **NVIDIA NIM API**   | External LLM provider; key stored in Secret Manager                      |
| **Vertex AI**        | (API enabled — placeholder for future Gemini Pro integration / Search)   |

The **`ai-service`** uses NVIDIA `moonshotai/kimi-k2-thinking` for chat &
NL→SQL. We pre-provisioned the `aiplatform.googleapis.com` API so a future
PR can swap to Vertex AI / Gemini / Model Garden without an infra change.

---

## 8. End-to-end deploy flow

The `Makefile` and `scripts/deploy.sh` automate everything below.

```bash
# 0. Pre-reqs (one-time per machine)
gcloud auth login
gcloud auth application-default login
gcloud config set project enterprise-portal-48689
brew install terraform helm kubectl    # macOS

# 1. Bootstrap state bucket (one-time per project)
PROJECT_ID=enterprise-portal-48689 ./scripts/bootstrap.sh

# 2. Provision *everything* on GCP
cd infrastructure/terraform
cp terraform.tfvars.example terraform.tfvars   # fill in secrets
terraform init
terraform plan -out=plan.bin
terraform apply plan.bin                       # ~15 min cold provision

# 3. Build + push images (Cloud Build OR locally)
gcloud builds submit --config cloudbuild.yaml --substitutions=SHORT_SHA=$(git rev-parse --short HEAD)

# 4. Connect kubectl
gcloud container clusters get-credentials enterprise-portal-cluster \
  --region us-central1

# 5. Deploy K8s manifests
kubectl apply -f infrastructure/k8s/

# 6. Verify
kubectl get pods -n enterprise-portal
curl https://portal.yourdomain.com/api/health
```

One-liner: `./scripts/deploy.sh` — wraps all of the above.

---

## 9. Cost ballpark (USD / month, us-central1)

| Component                                  | Tier                              | ~Cost  |
| ------------------------------------------ | --------------------------------- | ------ |
| GKE management (regional)                  | $0.10/hour                        | $73    |
| 6 × `e2-standard-4` worker nodes           | sustained-use discount            | $410   |
| Cloud SQL `db-g1-small` HA + 50 GB SSD     | regional                          | $90    |
| Memorystore Redis 2 GB STANDARD_HA         |                                   | $50    |
| Cloud Run (parser-svc)                     | scale-to-zero, ~10k requests      | $5     |
| Cloud Functions (file-ingest)              | <100k invocations                 | <$1    |
| Cloud Build                                | 120 free build-min/day            | $0     |
| Artifact Registry                          | 10 GB                             | $1     |
| Cloud Storage (uploads + static + logs)    | 100 GB STANDARD                   | $3     |
| Cloud DNS                                  | 1 zone + 1 M queries              | $0.40  |
| Cloud Armor                                | preconfigured rules               | $5     |
| Cloud KMS (2 keys + ops)                   |                                   | $1     |
| Secret Manager                             | 4 secrets, 100k access            | <$1    |
| BigQuery storage + queries                 | 5 GB stored + 10 GB scanned       | $1     |
| Cloud Logging / Monitoring / Trace          | within free tier                 | $0     |
| Jenkins GCE VM (option A)                  | `e2-standard-4` 24×7              | $100   |
| **Total**                                   |                                   | **~$740** |

> Trim with **Spot** node pools or **Cloud Run only** for `<100 RPS` demos.

---

## 10. Disaster recovery & SLOs

- **RPO**: 5 minutes (Cloud SQL PITR + GCS object versioning).
- **RTO**: 15 minutes (regional Cloud SQL failover + GKE regional control plane).
- **Backups**:
  - Cloud SQL automated daily, 30-day retention.
  - GCS object versioning + lifecycle to `NEARLINE` after 90 d.
- **Multi-region** (out-of-scope for the demo): change `region` to a multi-region
  bucket location, enable Cloud SQL cross-region replicas, and add a second
  GKE cluster behind the same global LB.

---

## 11. File map

```
infrastructure/
├── terraform/
│   ├── main.tf            # APIs, VPC, GKE, SQL, Storage, Redis, Pub/Sub,
│   │                      #   Secret Manager, Jenkins VM, Cloud Armor,
│   │                      #   DNS, Logging sink, alert policies
│   ├── cloud_run.tf       # Cloud Run parser-service + push subscription
│   ├── kms.tf             # Cloud KMS key ring + 2 keys + IAM
│   ├── cloud_build.tf     # Cloud Build SA + GitHub triggers
│   ├── serverless.tf      # Cloud Functions, Scheduler, Tasks
│   ├── cdn_lb.tf          # Global HTTPS LB + Cloud CDN + DNS A-record
│   ├── bigquery.tf        # BQ dataset/table + Pub/Sub→BQ subscription
│   ├── static_bucket.tf   # Public GCS bucket for React build
│   ├── variables.tf       # All inputs (sensitive defaults blank)
│   ├── outputs.tf         # All cluster / SQL / Redis / SA outputs
│   ├── functions/         # Cloud Function source (Python 3.11)
│   └── terraform.tfvars(.example)
├── jenkins/
│   ├── values.yaml        # Helm values for Jenkins on GKE
│   └── jcasc-config.yaml  # Configuration-as-Code (Okta + jobs)
├── k8s/                   # All Kubernetes manifests
│   ├── configmap.yaml
│   ├── api-gateway.yaml, auth-service.yaml, …
│   └── ingress.yaml
└── ../cloudbuild.yaml          # Main-branch CI/CD
└── ../cloudbuild-pr.yaml       # PR validation
```

---

## 12. References

- [GKE best-practices](https://cloud.google.com/kubernetes-engine/docs/best-practices)
- [Cloud Run pricing](https://cloud.google.com/run/pricing)
- [Cloud Build pipelines](https://cloud.google.com/build/docs/build-config-file-schema)
- [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity)
- [Cloud Armor preconfigured rules](https://cloud.google.com/armor/docs/rule-tuning)

---
*Authors: Serverless Squad — Aditya, Mohsen, Nihar, Tamizh.*
*See [`TEAM.md`](./TEAM.md).*
