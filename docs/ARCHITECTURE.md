# Architecture

This document describes the technical architecture of the **Enterprise Knowledge
Portal** — the components, the data flow, the deployment topology, and the
non-functional concerns (scalability, availability, security, observability).

---

## 1. Architectural Style

The system is a **cloud-native microservice architecture** that follows these
principles:

- **API-first** — every backend capability is exposed through a stable REST API.
- **Single-purpose services** — seven focused services, each independently
  deployable, scalable, and observable.
- **API Gateway pattern** — a single Go gateway terminates auth/CORS/rate-limit
  and reverse-proxies to internal services.
- **Polyglot where it matters** — Go for low-latency Gin services; Python/FastAPI
  for the parser microservice (best ecosystem for `pypdf`, `python-docx`).
- **Stateless compute, managed state** — services are stateless containers; state
  lives in Cloud SQL, Memorystore, GCS, and Pub/Sub.
- **Infrastructure as Code** — every cloud resource is declared in Terraform and
  every workload in Kubernetes manifests.

---

## 2. Component Diagram

```
┌────────────────────────────────────────────────────────────────────────────┐
│                              CLIENT (Browser)                              │
│   React 18 SPA · MUI · React Router · Axios · Auth0 SDK                    │
└────────────────────────────┬───────────────────────────────────────────────┘
                             │ HTTPS
                             ▼
┌────────────────────────────────────────────────────────────────────────────┐
│                       GCP HTTPS Load Balancer + Cloud Armor                │
└────────────────────────────┬───────────────────────────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────────────────────────┐
│                          GKE Ingress (NGINX/GCE)                           │
└────────────────────────────┬───────────────────────────────────────────────┘
                             │
            ┌────────────────┼────────────────┐
            │                │                │
            ▼                ▼                ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │   Frontend   │  │ API Gateway  │  │   Jenkins    │
    │ (Nginx pod)  │  │  (Go/Gin)    │  │ (controller) │
    └──────┬───────┘  └──────┬───────┘  └──────────────┘
           │                 │
           │ static          │ JWT-protected /api/*
           ▼                 ▼
                ┌──────────────────────────┐
                │   Internal Service Mesh  │
                │      (cluster DNS)       │
                └────┬─────┬─────┬─────┬───┘
                     │     │     │     │
                     ▼     ▼     ▼     ▼
            ┌──────┐ ┌─────┐ ┌─────┐ ┌──────────┐
            │ Auth │ │Data │ │File │ │Analytics │
            └───┬──┘ └──┬──┘ └──┬──┘ └────┬─────┘
                │       │       │         │
                │       │       ▼         │
                │       │   ┌─────────┐   │
                │       │   │ Parser  │   │
                │       │   │ (FastAPI)│  │
                │       │   └────┬────┘   │
                │       │        │        │
                │       └───┐    │   ┌────┘
                │           ▼    ▼   ▼
                │      ┌──────────────────────┐
                │      │    AI Service        │
                │      │ (Kimi K2 / NVIDIA)   │
                │      └──────────────────────┘
                ▼
   ┌──────────────────────────────────────────────────────┐
   │  Cloud SQL Postgres (HA) · Memorystore Redis (HA)    │
   │  Cloud Storage (files) · Kafka / Pub/Sub (events)    │
   └──────────────────────────────────────────────────────┘
```

---

## 3. Service Catalog

### 3.1 `api-gateway` (Go/Gin, port 8080)

- Single entry-point for `/api/*` from the SPA.
- Implements **CORS allow-listing** with a regex fallback for local dev origins,
  and **strips upstream Access-Control-* headers** to prevent duplication.
- **JWT middleware** — validates the bearer token, populates `c.Set("userID")`,
  enforces `role` claims when needed.
- **Rate limiter** — token-bucket per IP, configurable via env.
- **Reverse proxy** to internal services using `httputil.NewSingleHostReverseProxy`.

### 3.2 `auth-service` (Go, port 8081)

- Exchanges Auth0/Okta `id_token` for our internal JWT.
- Persists the user (JIT provisioning) into `users` with `role`, `okta_id`,
  `last_login_at`.
- Provides `/dev-login` in `DEV_MODE` for the demo.

### 3.3 `data-service` (Go, port 8082)

- Read-only API over the migrated enterprise database
  (`enterprise_employees`, `enterprise_products`, `enterprise_sales`,
  `enterprise_inventory`, `enterprise_departments`).
- Cursor pagination, `?q=` full-text filter, sortable columns.
- Caching layer in Redis for hot queries (configurable TTL).

### 3.4 `file-service` (Go, port 8083)

- Receives multipart uploads, stores blobs in **Cloud Storage** (or local volume
  in dev), creates a `uploaded_files` row with `status=processing`.
- Calls `parser-service` to extract text → persists chunks in `file_chunks`.
- Falls back to a built-in Go parser if Python service is unreachable.
- Endpoints: `POST /api/files`, `GET /api/files`, `GET /api/files/:id/chunks`,
  `DELETE /api/files/:id`, `DELETE /api/files/` (bulk).

### 3.5 `parser-service` (Python 3.11 / FastAPI, port 8090)

- High-fidelity extractors:
  - **PDF** → `pypdf.PdfReader`
  - **DOCX** → `python-docx` with **OOXML fallback** (raw `word/document.xml` via
    `xml.etree.ElementTree`) for unusual templates
  - **CSV** → `csv.reader`
  - **TXT** → safe UTF-8 decode + chunker
- Chunking: ~2000-char windows with paragraph boundaries preserved.
- Returns `{ "chunks": [...], "row_count": int }`.

### 3.6 `ai-service` (Go, port 8084)

- Two query modes:
  - **NL → SQL**: builds a system prompt that contains the live database schema
    (introspected via `INFORMATION_SCHEMA`) plus row-count hints, then asks
    the LLM to emit safe parameterized SQL → executes → returns table.
  - **Document Q&A**: pulls top-N relevant chunks from `file_chunks` (FTS index),
    composes a prompt, asks the LLM to answer with citations.
- Provider: NVIDIA NIM, model `moonshotai/kimi-k2-thinking`.
- Stores every query in `query_history`.

### 3.7 `analytics-service` (Go, port 8085)

- Pre-built KPIs (sales summary, top reps, headcount by dept, inventory levels).
- Custom report builder — accepts a JSON spec, returns paginated rows + chart
  hints.

---

## 4. Data Architecture

### 4.1 PostgreSQL Schema

| Table                     | Purpose                                                            |
| ------------------------- | ------------------------------------------------------------------ |
| `users`                   | Federated identity record (Okta `sub` → internal UUID).            |
| `enterprise_departments`  | Migrated dept master.                                              |
| `enterprise_employees`    | Headcount, salary, manager, hire date.                             |
| `enterprise_products`     | SKU master with prices/stock/supplier.                             |
| `enterprise_sales`        | Transactions, region, customer.                                    |
| `enterprise_inventory`    | Per-warehouse stock.                                               |
| `uploaded_files`          | File metadata (status, size, path, mime).                          |
| `file_chunks`             | 2 KB chunks with **GIN FTS index** on `content`.                   |
| `query_history`           | Every NL query with generated SQL & latency.                       |
| `analytics_reports`       | Saved report configurations + last result.                         |

Extensions enabled: `pgcrypto`, `pg_trgm`.

### 4.2 Caching

- Redis 7 caches:
  - Read-through for hot data-service lookups (5–60s TTL).
  - Session blocklist for revoked JWTs.
  - Per-IP token bucket counters.

### 4.3 Object Storage

- Cloud Storage bucket `${gcs_bucket_name}-${project_id}`.
- **Uniform bucket-level access**, **versioning on**, **lifecycle delete > 365d**.
- CORS limited to portal origin and localhost.

---

## 5. Deployment Topology (GCP)

```
                            ┌─────────────────────────┐
                            │      Project IAM         │
                            │ Workload-Identity-bound  │
                            └─────────────┬────────────┘
                                          │
            ┌─────────────────────────────┴──────────────────────────────┐
            │                       VPC (10.0.0.0/16)                    │
            │ ┌────────────────────────────────────────────────────────┐ │
            │ │                Subnet 10.0.0.0/16                      │ │
            │ │ ┌───────────────────────┐  ┌──────────────────────┐    │ │
            │ │ │ GKE Regional Cluster  │  │ Cloud SQL Postgres   │    │ │
            │ │ │  · 3 zones, autoscale │  │  Private IP, REGIONAL│    │ │
            │ │ │  · Workload Identity  │  └──────────────────────┘    │ │
            │ │ │  · HPA, managed Prom. │                              │ │
            │ │ └───────────────────────┘  ┌──────────────────────┐    │ │
            │ │                            │ Memorystore Redis 7  │    │ │
            │ │                            │ STANDARD_HA, TLS     │    │ │
            │ │                            └──────────────────────┘    │ │
            │ └────────────────────────────────────────────────────────┘ │
            └────────────────────────────────────────────────────────────┘
            ┌──────────────┐  ┌──────────────────────┐  ┌──────────────┐
            │ Cloud Storage│  │ Artifact Registry    │  │ Secret Mgr   │
            │  files+PITR  │  │ enterprise-portal    │  │ JWT/DB/Keys  │
            └──────────────┘  └──────────────────────┘  └──────────────┘
```

### 5.1 Cluster

- **Regional GKE** in `us-central1` for zonal redundancy.
- Custom node pool (`e2-standard-4`, autoscale 1→100) — Autopilot mode is
  toggleable via `enable_autopilot` in `infrastructure/terraform/main.tf`.
- **Workload Identity** on (`workload_pool = "${project_id}.svc.id.goog"`).
- Logging + Managed Prometheus turned on by default.

### 5.2 Ingress

- Single GKE Ingress fronts the React SPA (`/`) and the gateway (`/api/*`).
- Google-managed TLS certs for `portal.yourdomain.com`.
- Cloud Armor policy (rate limit + WAF rules) attached to the LB.

### 5.3 Pod Topology

| Deployment        | Replicas | HPA           |
| ----------------- | -------- | ------------- |
| `frontend`        | 2        | n/a           |
| `api-gateway`     | 2        | yes (cpu 65%) |
| `auth-service`    | 2        | yes           |
| `data-service`    | 3        | yes (3→50)    |
| `file-service`    | 2        | yes           |
| `parser-service`  | 2        | yes           |
| `ai-service`      | 2        | yes           |
| `analytics-service`| 2       | yes           |
| `postgres`*       | managed  | n/a (Cloud SQL) |
| `redis`*          | managed  | n/a (Memorystore) |
| `kafka`*          | optional | replaceable by Pub/Sub |

\* In Kubernetes manifests (`infrastructure/k8s/postgres.yaml`, etc.) we ship
in-cluster fallbacks for environments without managed services.

---

## 6. Sequence Diagrams

### 6.1 SSO Login

```
Browser            Auth0 / Okta            api-gateway          auth-service           Postgres
   │                     │                       │                     │                  │
   │  GET /login         │                       │                     │                  │
   ├────────────────────▶│ (Universal Login)     │                     │                  │
   │◀────── id_token ────┤                       │                     │                  │
   │                     │                       │                     │                  │
   │  POST /api/auth/exchange  ───────────────▶  │ proxy ─────────────▶│                  │
   │                                              │                     │ verify id_token   │
   │                                              │                     │ ─────▶ JWKS       │
   │                                              │                     │ upsert user       │
   │                                              │                     │ ────────────────▶│
   │                                              │                     │ ◀────────────────│
   │                                              │ ◀── { jwt, user } ──│                  │
   │ ◀──────────── 200 OK ─────────────────────── │                     │                  │
```

### 6.2 File Upload + AI Q&A

```
Browser           api-gateway        file-service     parser-service     ai-service     Postgres
   │ POST /api/files ─────────▶ │ proxy ──▶ │            │                  │              │
   │                           │            │ → GCS PUT │                  │              │
   │                           │            │ ─ POST /parse ─────▶          │              │
   │                           │            │ ◀──── chunks ────────         │              │
   │                           │            │ INSERT chunks ──────────────────────────────▶│
   │ ◀── 201 (file id) ────────┤            │                                              │
   │                                                                                        │
   │ POST /api/ai/query (file_id, q)                                                        │
   │  ───────▶ │ proxy ──▶ ai-service        │                                              │
   │                       │ load top-K chunks via FTS ─────────────────────────────────▶  │
   │                       │ ◀───── chunks ─────────────────────────────────────────────  │
   │                       │ → NVIDIA NIM (kimi-k2-thinking)                                │
   │                       │ ◀── answer + citations                                         │
   │ ◀── 200 { answer, sources } ────────────────────────────────────────────────────────  │
```

### 6.3 NL → SQL

```
Browser            api-gateway          ai-service             Postgres
   │ POST /api/ai/sql {q}  ─────▶ │ proxy ──▶ │                       │
   │                              │ introspect schema ─────────────▶ │
   │                              │ build prompt with schema          │
   │                              │ → NVIDIA Kimi-K2                  │
   │                              │ ◀ SQL                             │
   │                              │ validate (read-only, no DDL)      │
   │                              │ EXECUTE prepared stmt ─────────▶ │
   │                              │ ◀ rows                            │
   │                              │ INSERT query_history ─────────▶  │
   │ ◀── 200 { sql, rows, ms } ───┤                                   │
```

---

## 7. Non-Functional Concerns

### 7.1 Scalability

- Stateless services scale horizontally via HPA on CPU + custom metrics
  (request latency).
- Cloud SQL HA scales vertically; read replicas can be added for analytics load.
- Object storage and Memorystore are managed and elastic.

### 7.2 Availability

- **Regional** GKE & Cloud SQL with cross-zone failover.
- Each Deployment specifies `replicas ≥ 2` and a **PodDisruptionBudget** (added
  in `services.yaml`) to survive node drains.
- `livenessProbe` + `readinessProbe` on every container.

### 7.3 Performance

- p95 latency budgets (target):
  - `data-service` list endpoint < 150 ms.
  - `ai-service` NL→SQL < 4 s end-to-end.
  - `file-service` upload (≤ 50 MB) < 6 s.

### 7.4 Observability

- **Logs**: stdout → GKE → Cloud Logging.
- **Metrics**: managed Prometheus on the cluster, scrapes `/metrics` (Gin
  middleware exposes RED metrics).
- **Tracing**: OpenTelemetry-ready (env vars wired, exporter swappable).

### 7.5 Cost optimization

- Autoscaler `min_node_count = 1` for non-prod.
- Lifecycle rule deletes file blobs after 365 days.
- Trivy scans gated to HIGH/CRITICAL severities to keep CI fast.

---

## 8. Why this is a "Modern" Solution

- Managed services everywhere (Cloud SQL, Memorystore, Artifact Registry, GCS,
  Cloud Logging, Cloud Armor). No undifferentiated database admin.
- Workload Identity replaces SA JSON keys.
- Trunk-based GitOps style: pipeline runs on every push, no manual deploy.
- IaC + GitOps means the entire platform can be **destroyed and rebuilt** by
  running `make infra-up && make k8s-deploy`.
- The architecture is **serverless-shaped**: all stateful services are managed
  and elastic, all compute is autoscaling containers. Replacing GKE with Cloud
  Run is a 1-day swap if the org wants pure serverless.

---

## 9. Future Enhancements

- Replace Kafka with Cloud Pub/Sub end-to-end.
- Move parsing to **Cloud Run** to make it true serverless on cold paths.
- Add a vector database (pgvector / Vertex AI Vector Search) for semantic
  retrieval instead of plain FTS.
- Extend SSO with **SCIM provisioning** for org-wide AD sync.
- Add **Salesforce / Slack / Confluence connectors** for unified search.
