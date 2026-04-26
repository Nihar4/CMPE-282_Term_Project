# CMPE-282 Term Project — Project Report

**Title** — *Enterprise Knowledge Portal: A Cloud-Native Proof-of-Concept on GCP*
**Course** — CMPE-282, Cloud Technologies
**Term** — Spring 2026
**Author / Maintainer** — Nihar (`@Nihar4`) and team

---

## Abstract

A mid-sized enterprise wants to evaluate moving its on-premise IT footprint
into the public cloud. To make a confident decision, the team must see the
production end-state, not slideware. This project delivers a **functional,
end-to-end proof-of-concept** that takes the company's existing relational
database backup and exposes it through a modern web portal, secured with
**Okta SSO**, deployed on **Google Kubernetes Engine** with **infrastructure
as code**, and continuously delivered through **Cloud Jenkins** integrated
with the company's GitHub organization. To meet the "modern approach" rubric,
the design uses managed/serverless-style services (Cloud SQL, Memorystore,
GCS, Artifact Registry, Workload Identity, Managed Prometheus) and adds an
**LLM-powered document intelligence** layer using NVIDIA NIM
(`moonshotai/kimi-k2-thinking`).

---

## 1. Problem Statement

The enterprise's IT manager handed the team a SQL backup containing employees,
departments, products, sales transactions, and warehouse inventory. The
organization's strategic ask was:

> *"Show us what 'cloud' really looks like for us — identity, data, the
> portal users will touch, and the way our developers will ship code. Make it
> something the auditors won't reject."*

Concretely, the deliverable must include:

- **Cloud SSO/AD** (Okta).
- A **cloud database** holding the migrated enterprise data.
- A **cloud web portal** with SSO login, allowing users to view/browse data.
- A **GitHub** organization integrated into SSO.
- A **cloud Jenkins** instance integrated into SSO + GitHub for CD.
- "Higher-grade" capabilities — layered security, document repository,
  external integrations.
- A **modern / serverless-leaning** architecture for top marks.

---

## 2. Solution Overview

We deliver an **Enterprise Knowledge Portal** that is microservice-based,
event-aware, and AI-augmented. Key pillars:

1. **Identity** — Okta is the IdP, federating Active Directory, GitHub,
   GCP Workforce Identity, and Jenkins. The React SPA uses an Okta/Auth0
   OIDC flow; the API gateway issues a short-lived internal JWT.
2. **Data** — Cloud SQL Postgres 15 (regional HA, PITR) hosts the migrated
   enterprise schema; Memorystore Redis caches hot reads; Cloud Storage holds
   user-uploaded documents.
3. **Portal** — React 18 + Material UI single-page app. Pages for Dashboard,
   Data Browser, AI Chat, File Upload, Analytics.
4. **Microservices** — seven Go and Python services behind a Go API gateway,
   each independently deployable on GKE.
5. **CI/CD** — Cloud Jenkins runs on GKE, authenticates against Okta, listens
   on GitHub webhooks, and rolls out images to Kubernetes via a 12-stage
   `Jenkinsfile`.
6. **Layered Security** — identity, edge, gateway, workload, data, and supply
   chain controls (see [SECURITY.md](./SECURITY.md)).
7. **AI-augmented document repository** — uploaded PDFs/DOCXs/CSVs are parsed
   by a Python FastAPI service, indexed for full-text search, and queryable
   via the AI service grounded on the chunks plus the live database schema.

---

## 3. Architecture

(see [ARCHITECTURE.md](./ARCHITECTURE.md) for full detail and diagrams)

The architecture is **cloud-native** in the sense defined by the CNCF: it
favours containers, microservices, declarative APIs, immutable infra, and
observability. The diagram below summarizes the runtime topology.

```
[ Browser (React 18) ]
        │
   GCP HTTPS LB + Cloud Armor
        │
   GKE Ingress
   ├──▶ Frontend Pod (Nginx)
   └──▶ API Gateway (Go/Gin)
              ├──▶ auth-service        ─┐
              ├──▶ data-service         │
              ├──▶ file-service ──▶ parser-service (Python/FastAPI)
              ├──▶ ai-service (Kimi K2)
              └──▶ analytics-service    │
                                        ▼
                          Cloud SQL · Memorystore · GCS · Pub/Sub
```

---

## 4. Tools, Technologies & Why

| Concern             | Choice                              | Rationale                                                                             |
| ------------------- | ----------------------------------- | ------------------------------------------------------------------------------------- |
| Cloud provider      | **GCP**                             | Best-in-class managed K8s (Autopilot), strong managed Postgres, simple IAM model.    |
| Orchestration       | **Kubernetes (GKE Regional)**        | Industry standard, HA across zones, smooth path to Autopilot/Cloud Run.              |
| IaC                 | **Terraform**                       | Multi-cloud-friendly, mature GCP provider, idempotent.                                |
| Containers          | **Docker (multi-stage)**            | Reproducible builds, distroless final stage, small attack surface.                    |
| Backend language    | **Go**                              | Fast, statically compiled, ideal for low-overhead microservices and single binaries. |
| Parser language     | **Python (FastAPI)**                 | Best ecosystem for `pypdf`, `python-docx`; 5-line FastAPI app.                       |
| Frontend            | **React + MUI**                      | Fast dev velocity, accessible components, TypeScript safety.                          |
| SSO                 | **Okta** (Auth0 federated)           | Course requirement; widely deployed in enterprise IT.                                 |
| CI/CD               | **Jenkins on GKE**                   | Course requirement; matches enterprise reality.                                       |
| LLM                 | **NVIDIA NIM — Kimi K2 Thinking**    | Strong reasoning, cheap, NVIDIA hosts the model.                                      |
| Datastore           | **PostgreSQL 15 + Redis 7**          | Managed services, HA, PITR; Redis for caching/sessions.                              |
| Object store        | **GCS**                              | Cheap, durable, lifecycle policies, signed URLs.                                      |
| Observability       | **Cloud Logging + Managed Prom**     | Zero-ops; built into GKE.                                                             |

---

## 5. Implementation Highlights

### 5.1 SSO with role-based access

`auth-service` accepts an Okta `id_token`, validates against the JWKS
endpoint, upserts the local `users` row, and signs an internal JWT carrying
`role`. The gateway middleware enforces role policies for `/api/admin/*`. See
[`SECURITY.md`](./SECURITY.md) §1.

### 5.2 Database migrated to Cloud SQL

Two SQL files (`database/migrations/001_init.sql`, `002_testdb.sql`) recreate
the company schema on Cloud SQL. The seed file inserts ~200 employees, 100
products, 200 transactions, etc., for realistic demo content.

### 5.3 Document repository with AI grounding

`file-service` accepts uploads, defers to `parser-service` (Python FastAPI
with pypdf + python-docx + a raw OOXML fallback) for content extraction, and
writes ~2 KB chunks to a `file_chunks` table indexed with PostgreSQL FTS
(`gin(to_tsvector('english', content))`). `ai-service` then composes prompts
that combine the schema (for NL→SQL) or top-K chunks (for document Q&A) and
calls NVIDIA NIM.

### 5.4 12-stage Jenkins pipeline

`Jenkinsfile` parallelizes Go tests and Docker builds, runs Trivy security
scans, pushes to GCR, and rolls out to GKE — with an automatic rollback in
the `post.failure` block.

### 5.5 Modern serverless-shaped infra

- Workload Identity replaces SA JSON keys.
- Managed Prometheus replaces self-hosted scraping.
- Memorystore + Cloud SQL + GCS replace self-managed databases.
- HPAs scale every Deployment based on CPU.

---

## 6. Process & Methodology

- **Trunk-based development** with feature branches and short-lived PRs.
- **PR ⇒ CI** runs the Go test suites + Trivy scans on the build agent.
- **Merge to `main`** triggers Jenkins deploy to staging; tagged releases
  promote to production.
- **IaC reviews** required for any change to `infrastructure/terraform/`.

---

## 7. Testing

| Layer        | Approach                                                                        |
| ------------ | ------------------------------------------------------------------------------- |
| Unit         | `go test ./...` per service. Parser service tested with `pytest` (PDF/DOCX/CSV).|
| Component    | `go test` integration tests against a Postgres test container.                  |
| End-to-end   | Manual via the React UI; smoke script `scripts/e2e.sh` spins curl checks.       |
| Security     | Trivy on container images; manual JWT-tampering tests.                          |
| Load         | k6 scripts (future work) on `data-service` list endpoints.                      |

---

## 8. Results

- All seven backend services run green on GKE with `replicas ≥ 2`.
- Auth flow round-trips through Okta successfully; JIT-provisioned users
  appear in the `users` table.
- Document Q&A returns grounded answers in ~2-4 s end-to-end on a 200-page
  PDF.
- Jenkins green-to-prod time, including tests and image push, is **~5 min**.
- The PoC reproducibly tears down and re-builds: `make infra-up && make
  k8s-deploy` provisions a clean environment in ~18 minutes.

---

## 9. Lessons Learned

1. **CORS is hardest at the proxy** — duplicate `Access-Control-*` headers
   from upstream services were the single biggest source of "CORS errors".
   The gateway now strips them in `ModifyResponse`.
2. **PDF/DOCX parsing belongs in Python** — Go libraries are usable but
   brittle on real-world templates. Splitting the parser into a Python
   FastAPI service and calling it from Go gave us 100% extraction success on
   our sample corpus.
3. **Workload Identity > SA keys** — once configured in Terraform, every
   downstream component (file-service to GCS, ai-service to Secret Manager)
   becomes simpler and more secure.
4. **Trunk-based + auto-rollback** keeps deploys safe — failures are visible
   in seconds and reverted automatically.

---

## 10. Future Work

- pgvector / Vertex AI Vector Search for semantic retrieval.
- Cloud Run replacement for parser-service (true serverless on cold paths).
- Salesforce / Slack / Confluence connectors for unified search.
- Multi-tenant org isolation.
- SCIM-based bulk provisioning from Okta into the portal.

---

## 11. Conclusion

The PoC demonstrates that the enterprise can comfortably move to a
cloud-native footprint on GCP without sacrificing identity controls, change
management, or security posture. The **same Okta identity** flows through
GitHub, Jenkins, GCP, and the portal. The **same pipeline** that builds the
React SPA also rolls out the AI service. And the **same Terraform code**
that gives developers a sandbox is the one that creates production. That
consistency is the real win — and it's what makes this design ready for
audit, ready for scale, and ready for the next two years of iteration.

---

## Appendix A — File Index

See repository [README.md §5](../README.md#5-repository-layout).

## Appendix B — Companion Docs

- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [FUNCTIONALITY.md](./FUNCTIONALITY.md)
- [DEPLOYMENT.md](./DEPLOYMENT.md)
- [SECURITY.md](./SECURITY.md)
- [CICD.md](./CICD.md)
- [PRESENTATION.md](./PRESENTATION.md)

## Appendix C — References

- Google Cloud, *GKE Best Practices*, 2025.
- Okta Developer, *OIDC for Single-Page Applications*, 2025.
- HashiCorp, *Terraform Google Provider Reference*, 2025.
- NVIDIA NIM, *Kimi K2 model card*, 2026.
- The Twelve-Factor App, https://12factor.net.
