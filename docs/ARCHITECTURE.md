# Architecture

The Enterprise Knowledge Portal is now a fully serverless GCP application. All application containers run on Cloud Run; state is handled by managed GCP services.

## Runtime Topology

```text
Browser
  -> Cloud Run frontend
  -> Cloud Run api-gateway
  -> internal Cloud Run services
       auth-service
       data-service
       file-service
       ai-service
       analytics-service
       parser-service

Managed state:
  Cloud SQL PostgreSQL
  Memorystore Redis
  Cloud Storage
  Managed Kafka / Confluent Cloud
  Secret Manager
```

Public services:

- `frontend`
- `api-gateway`

Internal services:

- `auth-service`
- `data-service`
- `file-service`
- `ai-service`
- `analytics-service`
- `parser-service`

The internal services use Cloud Run service URLs and private VPC connectivity where needed.

## CI/CD Flow

```text
GitHub push
  -> Jenkins webhook
  -> Jenkinsfile
  -> gcloud builds submit cloudbuild-serverless.yaml
  -> Cloud Build
  -> Artifact Registry
  -> Terraform apply
  -> Cloud Run revisions
```

Jenkins is the orchestrator. Cloud Build is the build and deployment executor.

## Identity

Okta is the primary identity provider.

- Frontend redirects users to Okta OIDC.
- Auth service exchanges the authorization code for tokens.
- Auth service stores the session securely and issues a portal JWT.
- API gateway protects `/api/*` routes with JWT validation.
- Jenkins uses Okta SAML SSO for administrator login.

## Data Flow

Database queries:

```text
Frontend -> api-gateway -> ai-service -> Cloud SQL -> NVIDIA LLM -> response
```

Document upload:

```text
Frontend -> api-gateway -> file-service -> Cloud Storage
file-service -> parser-service -> file_chunks table
file-service -> Kafka notification -> data-service -> Redis notification cache
```

AI chat with documents:

```text
Frontend -> api-gateway -> ai-service job endpoint
ai-service -> Kafka topic portal.ai.requests
Cloud Run ai-service worker -> Cloud SQL + file_chunks hybrid retrieval
Cloud Run ai-service worker -> NVIDIA LLM
Cloud Run ai-service worker -> Redis job result
Frontend polls job result
```

Document retrieval is RAG-based:

- Documents are stored as chunks in `file_chunks`.
- The database query and document query run separately.
- Document search uses hybrid retrieval across PostgreSQL full-text search and token matching.
- The final LLM context is capped to the top 5 chunks.

## GCP Services

| Service | Purpose |
| --- | --- |
| Cloud Run | Runs every application service serverlessly |
| Cloud SQL | PostgreSQL enterprise data and app metadata |
| Memorystore Redis | Sessions, AI job state, notifications |
| Cloud Storage | Uploaded files |
| Artifact Registry | Docker image repository |
| Cloud Build | Build, push, deploy execution |
| Secret Manager | DB, JWT, NVIDIA, Okta secrets |
| Serverless VPC Access | Cloud Run private connectivity to SQL/Redis |
| Cloud Logging | Central logs |
| Cloud Monitoring | Metrics and alerts |
| GCE | Jenkins controller VM |

## Security Boundaries

- Only `frontend` and `api-gateway` are public.
- Backend Cloud Run services are internal.
- Secrets are injected from Secret Manager.
- Cloud SQL and Redis use private networking.
- Jenkins uses a GCP service account to trigger Cloud Build.
- Cloud Build deploys using IAM-scoped service account permissions.

## Scalability

- Cloud Run scales services independently.
- `ai-service` has warm minimum instances for Kafka worker consumption.
- Kafka consumer groups distribute AI jobs across active workers.
- Redis stores idempotent job state so duplicate Kafka delivery does not double-process completed jobs.
- Cloud SQL and Memorystore provide managed high availability.

## Removed Deployment Path

Kubernetes/GKE manifests and rollout scripts have been removed. The project now has one deployment path: Cloud Run.
