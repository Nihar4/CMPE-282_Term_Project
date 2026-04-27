# CMPE-282 Term Project — Enterprise Knowledge Portal

> A modern, fully cloud-native Proof-of-Concept that migrates an enterprise database to the cloud
> and exposes it through an SSO-protected web portal with AI-powered querying, document
> intelligence, real-time analytics, and end-to-end CI/CD on **GCP Cloud Run + Docker + Jenkins**.

[![GCP](https://img.shields.io/badge/Cloud-GCP-4285F4?logo=googlecloud&logoColor=white)](https://cloud.google.com)
[![Cloud Run](https://img.shields.io/badge/Runtime-Cloud%20Run-4285F4?logo=googlecloud&logoColor=white)](https://cloud.google.com/run)
[![Docker](https://img.shields.io/badge/Container-Docker-2496ED?logo=docker&logoColor=white)](https://www.docker.com)
[![Jenkins](https://img.shields.io/badge/CI%2FCD-Jenkins-D24939?logo=jenkins&logoColor=white)](https://www.jenkins.io)
[![Terraform](https://img.shields.io/badge/IaC-Terraform-7B42BC?logo=terraform&logoColor=white)](https://www.terraform.io)
[![React](https://img.shields.io/badge/Frontend-React-61DAFB?logo=react&logoColor=black)](https://react.dev)
[![Go](https://img.shields.io/badge/Backend-Go-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![FastAPI](https://img.shields.io/badge/Parser-FastAPI-009688?logo=fastapi&logoColor=white)](https://fastapi.tiangolo.com)
[![PostgreSQL](https://img.shields.io/badge/DB-PostgreSQL-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![Okta](https://img.shields.io/badge/SSO-Okta-00297A?logo=okta&logoColor=white)](https://www.okta.com)

---

## 1. Project Overview

Your team works for a company that wants to explore moving their business to the cloud. The
manager has provided a backup of the company's enterprise database and asks for a
**Proof-of-Concept cloud-based IT infrastructure** that includes:

- Okta cloud Single Sign-On (SSO) and AD federation
- Cloud-based database / datastore backend
- Cloud-based web portal for viewing/browsing sample enterprise data (with SSO login)
- GitHub integrated into SSO for all project code
- Cloud-based Jenkins, integrated into SSO and the GitHub repo with continuous deployment
- Additional integrations / capabilities for higher grade (layered security, document
  repository, AI integration, observability, autoscaling, etc.)
- For top marks, the solution should use a **modern serverless / managed approach**.

This repo delivers all of the above on **Google Cloud Platform** with a microservice
architecture, a managed PostgreSQL backend, an SSO-protected React UI, an AI document
Q&A engine, a Jenkins-driven CI/CD pipeline, and infrastructure-as-code (Terraform).

A live demo URL, screenshots, and the full project report are referenced in
[`docs/PROJECT_REPORT.md`](./docs/PROJECT_REPORT.md).

---

## 2. High-Level Architecture

```
                       ┌──────────────────────────────────────────────┐
                       │                 Okta IdP                     │
                       │   (SSO, AD federation, MFA, GitHub Apps)     │
                       └───────────────┬──────────────────────────────┘
                                       │ OIDC
                                       ▼
 ┌──────────┐     HTTPS / OIDC     ┌─────────────────┐    REST/JWT
 │  React   │ ───────────────────▶ │  API Gateway    │ ─────────────────┐
 │ (MUI)    │                      │  (Go / Gin)     │                  │
 └──────────┘                      │  CORS · Rate    │                  │
       ▲                           │  limit · JWT    │                  │
       │ HTTPS                     └────────┬────────┘                  │
       │                                    │                           │
       │                  ┌─────────────────┼─────────────────┐         │
       │                  ▼                 ▼                 ▼         ▼
       │           ┌────────────┐    ┌────────────┐    ┌────────────┐ ┌────────────┐
       │           │   Auth     │    │   Data     │    │   File     │ │ Analytics  │
       │           │  Service   │    │  Service   │    │  Service   │ │  Service   │
       │           └─────┬──────┘    └─────┬──────┘    └─────┬──────┘ └─────┬──────┘
       │                 │                 │                 │              │
       │                 ▼                 ▼                 ▼              ▼
       │           ┌──────────────────────────────────────────────────────────────┐
       │           │  Cloud SQL (PostgreSQL 15, HA, PITR, private VPC)            │
       │           │  Cloud Memorystore (Redis 7, HA, TLS)                        │
       │           │  Cloud Storage (uploaded files, lifecycle, versioning)       │
       │           │  Pub/Sub or Kafka (events)                                   │
       │           └──────────────────────────────────────────────────────────────┘
       │
       │           ┌────────────┐    ┌────────────┐
       │           │    AI      │    │   Parser   │
       │           │  Service   │ ◀─▶│  Service   │  (FastAPI: pdf/docx/csv/txt)
       │           │  (Kimi K2) │    │   Python   │
       │           └────────────┘    └────────────┘

   GitHub  ──▶  Jenkins  ──▶  Cloud Build  ──▶  Artifact Registry  ──▶  Cloud Run
```

### Current Serverless Flow

- The portal now targets **Cloud Run for every application service**: frontend, API gateway, auth, data, file, AI, analytics, and parser.
- AI chat requests are submitted to Kafka topic `portal.ai.requests`; Cloud Run `ai-service` instances consume as the `portal-ai-workers` consumer group, which load-balances jobs across replicas.
- Redis stores AI job state/results under `ai_job:<job_id>`, so the React chat can enqueue once and poll until complete.
- Kafka topic `portal.notifications` carries file and AI events; `data-service` consumes those events and caches the latest notifications in Redis for the UI bell.

A more detailed view (sequence diagrams, flows, and IAM boundaries) lives in
[`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md).

---

## 3. Cloud Components Mapped to Course Requirements

| Requirement                                              | Implementation                                                                                                  |
| -------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| Okta-based Cloud SSO / AD                                | Okta OIDC tenant. Frontend redirects through Okta; auth-service exchanges the code and issues a portal JWT.     |
| Cloud database / datastore                               | Cloud SQL for PostgreSQL 15 (regional HA), Cloud Memorystore Redis 7, Cloud Storage for files.                  |
| Cloud Web Portal w/ SSO                                  | React 18 + MUI portal served by Cloud Run frontend, all routes guarded by Okta SSO login.                       |
| GitHub integrated into SSO                               | GitHub org enrolled in the Okta tenant (OIDC + SCIM). Same identity used for repo, Jenkins, GCP IAM.            |
| Cloud Jenkins, integrated into SSO + GitHub              | Jenkins controller on GCE, Okta SAML for login, GitHub webhooks for build triggers.                             |
| Continuous Deployment                                    | Jenkins triggers `cloudbuild-serverless.yaml`, which builds, pushes, and deploys Cloud Run.                     |
| Layered security                                         | Identity (SSO/JWT), network (private VPC/internal Cloud Run), Cloud Run service account, Secret Manager.        |
| Document repository                                      | File upload (PDF/DOCX/CSV/TXT), Python parser-service, vector-style chunking, AI Q&A on documents.              |
| Modern / serverless approach                             | Cloud Run, Cloud SQL, Memorystore, GCS, Artifact Registry, serverless VPC access, Secret Manager, Cloud Build. |

---

## 4. Tech Stack

| Layer                     | Technology                                                                                                    |
| ------------------------- | ------------------------------------------------------------------------------------------------------------- |
| Frontend                  | React 18, TypeScript, Material UI, Axios, React Router, react-dropzone                                        |
| API Gateway               | Go 1.21, Gin, JWT-Go, Gin-CORS, custom rate limiter, reverse-proxy with `httputil`                            |
| Microservices (Go)        | Gin, GORM (PostgreSQL), JWT, Okta OIDC, segmentio/kafka-go                                                    |
| Parser microservice       | Python 3.11, FastAPI, Uvicorn, pypdf, python-docx, csv, lxml fallback                                         |
| AI                        | NVIDIA NIM (`openai/gpt-oss-120b`), NL→SQL prompt engineering, document Q&A with retrieval            |
| Datastore                 | PostgreSQL 15 (Cloud SQL), Redis 7 (Memorystore), GCS for file blobs, Kafka for events                        |
| IaC                       | Terraform 1.5+ (Google + Google-beta providers), Cloud Run / Cloud SQL / GCS / Redis / Artifact Registry      |
| Runtime                   | Cloud Run for all application containers                                                                      |
| Build / CI                | Docker multi-stage builds, Jenkins on GCE, Cloud Build                                                        |
| CD                        | Jenkinsfile or `cloudbuild-serverless.yaml` → Artifact Registry → Cloud Run revisions                         |
| Identity                  | Okta, GitHub SAML/OIDC, GCP IAM, Cloud Run service accounts                                                   |
| Observability             | GCP Cloud Logging, Cloud Monitoring, Cloud Run metrics, Trivy image scans                                      |

---

## 5. Repository Layout

```
CMPE-282_Term_Project/
├── README.md                    ← you are here
├── Jenkinsfile                  ← cloud Jenkins pipeline (build → push → deploy → verify)
├── Makefile                     ← one-command dev workflow
├── docker-compose.yml           ← local stack (8 containers)
├── .env.example                 ← env-var contract (no secrets)
├── docs/
│   ├── ARCHITECTURE.md          ← system + sequence diagrams
│   ├── FUNCTIONALITY.md         ← feature matrix, screenshots, user journeys
│   ├── DEPLOYMENT.md            ← step-by-step GCP / Cloud Run / Docker / Jenkins setup
│   ├── SECURITY.md              ← Okta, JWT, IAM, encryption, layered defenses
│   ├── OKTA_SETUP.md            ← Portal OIDC + Jenkins SAML setup values
│   ├── CICD.md                  ← Jenkins + GitHub pipeline deep dive
│   ├── PROJECT_REPORT.md        ← academic-style write-up (intro→results→learnings)
│   └── PRESENTATION.md          ← slide-deck script
├── frontend/                    ← React + MUI portal
├── backend/
│   ├── api-gateway/             ← Go/Gin gateway: CORS, JWT, rate limit, proxy
│   ├── auth-service/            ← Okta exchange, user mgmt, JWT mint
│   ├── data-service/            ← Browse enterprise tables (employees, sales, …)
│   ├── file-service/            ← Uploads, GCS, parsing orchestration, chunks
│   ├── parser-service/          ← Python FastAPI parser (pdf/docx/csv/txt)
│   ├── ai-service/              ← NL→SQL & document Q&A via NVIDIA NIM
│   └── analytics-service/       ← KPI dashboards & report generation
├── database/
│   ├── migrations/              ← 001_init.sql, 002_testdb.sql
│   └── seeds/                   ← mock_data.sql, 002_testdb_sample.sql
└── infrastructure/
    └── terraform/               ← GCP IaC (Cloud Run, Cloud SQL, GCS, Memorystore, Artifact Reg.)
```

---

## 6. Quick Start (Local — Docker Compose)

Prerequisites: Docker Desktop, GNU make, ~6 GB free RAM.

```bash
git clone https://github.com/Nihar4/CMPE-282_Term_Project.git
cd CMPE-282_Term_Project
cp .env.example .env       # then edit .env with your own secrets

make up                    # docker compose up -d
make seed                  # load sample employees/sales/products
```

Open the portal:

| URL                                                  | Purpose                       |
| ---------------------------------------------------- | ----------------------------- |
| http://localhost:3000                                | React portal (SSO login)      |
| http://localhost:8080                                | API gateway                   |
| http://localhost:8090/docs                           | Parser-service Swagger        |
| http://localhost:5432                                | PostgreSQL (portal_user)      |

Helpful commands: `make logs`, `make ps`, `make down`, `make clean`, `make test`.

---

## 7. Quick Start (Cloud — GCP Cloud Run + Jenkins)

1. **Provision** GCP infra with Terraform — `make infra-up` (creates VPC, Cloud SQL,
   GCS, Memorystore, Artifact Registry, Jenkins VM, and Cloud Run services).
2. **Deploy** — trigger Jenkins or run `make cloud-run-deploy`.
3. **Connect SSO** — add the Cloud Run frontend callback/logout URLs to Okta, then
   update Jenkins env/substitutions and rerun the pipeline.

The exact step-by-step is documented in [`docs/DEPLOYMENT.md`](./docs/DEPLOYMENT.md).

---

## 8. Microservice Catalog

| Service             | Port | Lang   | Responsibility                                                            |
| ------------------- | ---- | ------ | ------------------------------------------------------------------------- |
| `api-gateway`       | 8080 | Go     | Single ingress for the SPA, CORS/JWT/rate-limit, reverse-proxy            |
| `auth-service`      | 8081 | Go     | Okta token exchange, profile sync, JWT issuance                           |
| `data-service`      | 8082 | Go     | Read-only enterprise data API (employees, products, sales, inventory)    |
| `file-service`      | 8083 | Go     | Upload, persist to GCS, orchestrate parsing, expose chunks                |
| `parser-service`    | 8090 | Python | Robust extraction for PDF / DOCX / CSV / TXT, returns ordered chunks     |
| `ai-service`        | 8084 | Go     | NL→SQL on enterprise data, document Q&A on uploaded files                |
| `analytics-service` | 8085 | Go     | KPIs, trends, executive dashboards                                        |

---

## 9. Security Highlights (Layered Defense)

1. **Identity**: Okta (primary IdP) federates AD, GitHub, GCP, and Jenkins.
2. **Edge**: GCP HTTPS Load Balancer + Cloud Armor (rate limit, geo block).
3. **Gateway**: CORS allow-list, JWT verification, per-IP rate limiter, response header
   stripping (no upstream CORS leakage).
4. **Workload**: Each microservice runs as a non-root Cloud Run container with a
   least-privilege Cloud Run service account.
5. **Data**: Cloud SQL private IP, TLS at rest + in transit, Secret Manager for DB pwd /
   JWT secret / NVIDIA key. Redis with auth + TLS.
6. **Supply chain**: Trivy image scan in Jenkins/Cloud Build, Artifact Registry scanning, signed
   commits required on `main`.

Full details in [`docs/SECURITY.md`](./docs/SECURITY.md).

---

## 10. CI/CD Pipeline

Trigger: every push or PR to `main` fires a GitHub webhook → Jenkins. The pipeline
([`Jenkinsfile`](./Jenkinsfile)) runs:

1. Checkout
2. GCP auth (service account → `gcloud auth`)
3. Backend tests (parallel `go test ./...`)
4. `gcloud builds submit --config cloudbuild-serverless.yaml`
5. Cloud Build builds images, pushes Artifact Registry, and runs Terraform apply
6. Terraform deploys/updates Cloud Run revisions

See [`docs/CICD.md`](./docs/CICD.md) for the full pipeline anatomy plus the GitHub +
Okta + Jenkins SSO wiring.

---

## 11. Functionality (User-Facing)

- **SSO Login** via Okta; JIT user provisioning into PostgreSQL `users` table.
- **Dashboard** — KPIs sourced from `analytics-service` (sales, headcount, inventory).
- **Data Browser** — paginated, filterable views over `enterprise_*` tables.
- **AI Chat** — NL→SQL ("show me top 5 reps in Q2"), grounded document Q&A
  ("summarize the offer letter PDF I just uploaded"), and chat history.
- **File Upload** — drag-and-drop CSV/PDF/DOCX/TXT, status tracking, per-file delete,
  bulk delete, and chunk preview.
- **Analytics** — pre-built reports + custom report builder.

See [`docs/FUNCTIONALITY.md`](./docs/FUNCTIONALITY.md) for screenshots and feature flows.

---

## 12. Documentation Index

| Doc                                        | Audience                                                |
| ------------------------------------------ | ------------------------------------------------------- |
| [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) | Architects, reviewers — system + sequence diagrams      |
| [`docs/FUNCTIONALITY.md`](./docs/FUNCTIONALITY.md) | Stakeholders — features, screenshots, journeys         |
| [`docs/DEPLOYMENT.md`](./docs/DEPLOYMENT.md)     | DevOps — GCP / Cloud Run / Docker / Jenkins step-by-step |
| [`INFRASTRUCTURE.md`](./INFRASTRUCTURE.md)         | DevOps — every GCP service used and why (single source of truth) |
| [`docs/SECURITY.md`](./docs/SECURITY.md)         | Security reviewers — Okta, layered defenses             |
| [`docs/OKTA_SETUP.md`](./docs/OKTA_SETUP.md)     | DevOps — portal Okta OIDC and Jenkins SAML setup        |
| [`docs/CICD.md`](./docs/CICD.md)                 | DevOps — Jenkins pipeline + GitHub + Okta SSO           |
| [`docs/PROJECT_REPORT.md`](./docs/PROJECT_REPORT.md) | Faculty — academic project report                       |
| [`docs/PRESENTATION.md`](./docs/PRESENTATION.md) | Presenters — slide-by-slide outline                     |

---

## 13. Team & Course

- **Course**: CMPE-282 — Cloud Technologies (Spring 2026)
- **Term project**: Cloud-based Enterprise IT Infrastructure (Proof-of-Concept)
- **Group**: **Serverless Squad** (4 students)

| Member                          | Focus area                                                  |
| ------------------------------- | ----------------------------------------------------------- |
| Aditya Govind Shahari           | Frontend & UX, AI integration                               |
| Mohsen Minai                    | Backend (Go), security, CORS / JWT / IAM                    |
| Nihar Dharmeshkumar Patel       | Cloud architecture, GCP / Terraform / Cloud Run, CI/CD     |
| Tamizh Selvan Manivannan        | Data engineering, parser-service (Python), analytics        |

See [`TEAM.md`](./TEAM.md) for contribution breakdown.

## 14. License

Released under the MIT License. See [`LICENSE`](./LICENSE) (add one if not present).
