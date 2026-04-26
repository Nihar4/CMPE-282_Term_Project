# Presentation Outline — Enterprise Knowledge Portal

A slide-by-slide script for the CMPE-282 term project demo. Each slide has a
title, a short bullet list of talking points, and the asset you should show.

> Recommended length: **15 slides, 12-minute talk + 3-minute demo**.

---

## Slide 1 — Title

- **Title:** Enterprise Knowledge Portal
- **Subtitle:** A Cloud-Native PoC on GCP with Okta SSO, Kubernetes,
  Jenkins, and AI Document Intelligence
- **Author:** Nihar (`@Nihar4`) — CMPE-282, Spring 2026

> Visual: project logo / hero shot of the Dashboard.

---

## Slide 2 — The Ask

- Migrate an enterprise database to the cloud.
- Build a production-grade PoC that auditors won't reject.
- Cover **identity, data, portal, GitHub, Jenkins, security, modern stack**.

> Visual: bullet list of course requirements.

---

## Slide 3 — High-Level Architecture (One Picture)

- Browser → LB → GKE Ingress → API Gateway → 6 microservices → managed data.
- Identity flows: Okta → GitHub, Jenkins, GCP, Portal.
- IaC + GitOps powering everything.

> Visual: the big architecture diagram from `ARCHITECTURE.md §2`.

---

## Slide 4 — Tech Stack

- **Frontend:** React 18, MUI, TypeScript.
- **Backend:** Go 1.21 (six services) + Python FastAPI (parser).
- **Data:** Cloud SQL Postgres 15, Memorystore Redis 7, Cloud Storage.
- **Identity:** Okta (with Auth0 federation).
- **Orchestration:** GKE Regional + HPA + Workload Identity.
- **CI/CD:** Cloud Jenkins on GKE.
- **AI:** NVIDIA NIM Kimi K2 Thinking.

> Visual: logo grid.

---

## Slide 5 — Identity & SSO (Okta)

- Okta = master IdP, federates AD.
- Single click signs you into Portal, GitHub, Jenkins, and GCP.
- Internal JWT issued after Okta validation; role-based gateway middleware.

> Visual: SSO sequence diagram from `ARCHITECTURE.md §6.1`.

---

## Slide 6 — Cloud Database

- Cloud SQL Postgres 15 (regional HA, PITR, private IP).
- Migrated schema: `enterprise_employees`, `_products`, `_sales`,
  `_inventory`, `_departments`.
- 200+ rows of seed data; FTS index on document chunks.
- Read replicas + Memorystore caching for analytics workloads.

> Visual: ER diagram or `\dt` from psql.

---

## Slide 7 — Microservices (Live!)

- 7 services × independent images × independent rollouts.
- API Gateway = the only public entrypoint.
- Each service: `livenessProbe`, `readinessProbe`, HPA, structured logs.

> Visual: `kubectl get pods -n enterprise-portal`.

---

## Slide 8 — Document Repository + AI

- Drag-drop PDF/DOCX/CSV/TXT.
- Python FastAPI parses → chunks → indexed in Postgres FTS.
- AI Chat asks Kimi K2 to answer **grounded on those chunks**, with citations.
- NL → SQL mode produces safe parameterized SQL against the live DB.

> Visual: GIF of upload + AI Q&A.

---

## Slide 9 — GitHub + Okta

- GitHub org with **SAML SSO via Okta** + SCIM provisioning.
- Same identity used in PRs, Jenkins, GCP IAM.
- Branch protection on `main`: signed commits, required reviews, required CI.

> Visual: GitHub repo settings showing SSO + branch protection.

---

## Slide 10 — Cloud Jenkins on GKE

- Jenkins controller installed via Helm into GKE.
- Login via Okta OIDC (oic-auth plugin).
- 12-stage pipeline: test → build → scan → push → deploy → verify → rollback.

> Visual: Jenkins pipeline graph with all stages green.

---

## Slide 11 — CI/CD Pipeline Walkthrough

- Push triggers webhook → multibranch pipeline.
- Parallel Go tests; parallel Docker builds.
- Trivy scan on critical images.
- `kubectl set image` rollout; `rollout status` gate; auto-rollback in
  `post.failure`.

> Visual: side-by-side stage timeline.

---

## Slide 12 — Layered Security

- Edge: HTTPS LB + Cloud Armor.
- Gateway: CORS allow-list, JWT, rate-limit, header strip.
- Workload: distroless, non-root, Workload Identity.
- Data: TLS, private IP, Secret Manager, masked PII columns.
- Supply chain: branch protection, Trivy, signed commits.

> Visual: layered "onion" diagram.

---

## Slide 13 — Modern / Serverless Mindset

- Cloud SQL, Memorystore, GCS, Artifact Registry — all managed.
- Workload Identity replaces SA keys.
- Managed Prometheus instead of self-hosted scraping.
- HPA + auto node scaling = pay-per-use compute.
- Cloud Run for parser is a 1-day swap if we want fully serverless.

> Visual: managed-vs-self-managed table.

---

## Slide 14 — Demo (Live)

1. Login via Okta.
2. Browse Data → filter Sales by region.
3. Upload `sample.pdf` → see status icon turn green.
4. Ask AI: *"Top 5 reps in Q2 2024"* → table appears.
5. Ask AI: *"Summarize the file I just uploaded"* → grounded answer.
6. Trigger a Jenkins build → watch it deploy live.

> Visual: full screen of the running app.

---

## Slide 15 — Q&A / Future Work

- **Future:** pgvector, Cloud Run, Salesforce/Slack connectors, full SCIM.
- **Repo:** github.com/Nihar4/CMPE-282_Term_Project
- **Docs:** README + 6 supporting markdown docs, PROJECT_REPORT.md for
  full write-up.

> Visual: QR code → repo URL.

---

## Speaker Notes (cheat sheet)

- Open with **the rubric**, then walk through how every bullet maps to a
  shipped feature.
- When showing the Jenkins pipeline, scroll the `Jenkinsfile` briefly — call
  out `post.failure { rollout undo }` as the "auditor's friend".
- When demoing AI, paste a question that **shouldn't** generate destructive
  SQL (e.g., "delete all sales") and show the AST validator rejecting it.
- Close with the **modern stack table** (slide 13) and the cost estimate
  ("~$80/mo at demo scale, ~$510/mo at production scale").
