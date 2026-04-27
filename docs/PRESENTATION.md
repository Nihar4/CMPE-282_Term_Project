# Presentation Outline

## Slide 1 — Title

Enterprise Knowledge Portal: Serverless Cloud IT Infrastructure on GCP

## Slide 2 — Problem

The organization needs a secure cloud portal for enterprise data, file uploads, AI-assisted querying, SSO, and CI/CD.

## Slide 3 — Final Architecture

```text
Browser -> Cloud Run frontend -> Cloud Run api-gateway -> internal Cloud Run services
```

Managed state:

- Cloud SQL
- Memorystore Redis
- Cloud Storage
- Kafka
- Secret Manager

## Slide 4 — Serverless Runtime

Every application service runs on Cloud Run:

- frontend
- api-gateway
- auth-service
- data-service
- file-service
- parser-service
- ai-service
- analytics-service

## Slide 5 — Identity

- Okta OIDC for portal login
- Okta SAML for Jenkins admin login
- Portal JWT protects API routes
- Secrets stored in Secret Manager

## Slide 6 — AI Flow

```text
User question -> Kafka job -> AI worker -> Cloud SQL + top 5 doc chunks -> NVIDIA LLM -> Redis result
```

## Slide 7 — Document RAG

- Files uploaded to Cloud Storage
- Parser extracts text
- Chunks stored in PostgreSQL
- Hybrid search retrieves top 5 chunks
- LLM receives capped context

## Slide 8 — CI/CD

```text
GitHub -> Jenkins -> Cloud Build -> Artifact Registry -> Cloud Run
```

## Slide 9 — Infrastructure As Code

Terraform manages:

- Cloud Run
- Cloud SQL
- Redis
- GCS
- Secret Manager
- Artifact Registry
- Jenkins VM
- Monitoring/logging

## Slide 10 — Security

- Public access only to frontend and API gateway
- Internal Cloud Run services
- Private SQL/Redis networking
- Least-privilege service accounts
- Okta SSO

## Slide 11 — Demo

Show:

1. Okta login
2. Dashboard
3. File upload
4. AI query with docs
5. Jenkins build
6. Cloud Run services in GCP Console

## Slide 12 — Result

The final project is fully serverless on GCP and uses one deployment path: Cloud Run.
