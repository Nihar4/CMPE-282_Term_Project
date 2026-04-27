# Infrastructure

The project is now fully serverless on GCP. Kubernetes/GKE is not part of the active infrastructure.

## Deployment Model

```text
GitHub -> Jenkins -> Cloud Build -> Artifact Registry -> Cloud Run
```

Jenkins receives GitHub webhooks and starts Cloud Build. Cloud Build builds images, pushes them to Artifact Registry, and runs Terraform to update Cloud Run services.

## Terraform Entry Point

```text
infrastructure/terraform/
```

Main Terraform files:

| File | Purpose |
| --- | --- |
| `main.tf` | APIs, VPC, Cloud SQL, Redis, Storage, Secret Manager, Artifact Registry, Jenkins VM, monitoring |
| `cloud_run.tf` | Shared Cloud Run service account and parser-service |
| `cloud_run_services.tf` | Cloud Run services for frontend, gateway, and all backend services |
| `cloud_build.tf` | Cloud Build IAM and optional Cloud Build triggers |
| `variables.tf` | Inputs for project, region, Okta, Kafka, Cloud Run, and secrets |
| `outputs.tf` | Cloud Run URLs, Jenkins access command, storage, Redis, SQL outputs |

## GCP Resources

| Resource | Use |
| --- | --- |
| Cloud Run | Serverless runtime for all services |
| Cloud SQL PostgreSQL | Enterprise data, users, file metadata, chunks, query history |
| Memorystore Redis | Session data, AI job state, notification cache |
| Cloud Storage | Uploaded files and long-term object storage |
| Artifact Registry | Container image registry |
| Secret Manager | DB password, JWT secret, NVIDIA API key, Okta client secret |
| Serverless VPC Access | Private access from Cloud Run to SQL/Redis |
| Cloud Build | Build and deployment executor |
| GCE | Jenkins controller VM |
| Cloud Monitoring | Alerts and metrics |
| Cloud Logging | Centralized logs |

## Cloud Run Services

Public:

- `frontend`
- `api-gateway`

Internal:

- `auth-service`
- `data-service`
- `file-service`
- `ai-service`
- `analytics-service`
- `parser-service`

`api-gateway` proxies browser `/api/*` requests to the internal services.

## Messaging

Kafka is used for runtime queueing:

- `portal.ai.requests` queues AI jobs.
- `portal.notifications` carries AI and file notifications.
- Cloud Run `ai-service` instances consume as `portal-ai-workers`.
- `data-service` consumes notifications and caches the latest events in Redis.

Use a managed Kafka provider for Cloud Run, such as Confluent Cloud or GCP Managed Service for Apache Kafka. Set:

```text
KAFKA_BROKERS=<bootstrap-hosts>
```

## Jenkins

Jenkins runs on a GCE VM, not Kubernetes. It needs:

- GitHub credentials for checkout/webhook builds
- GCP service account credentials
- Okta SAML SSO for administrator login
- `gcloud` installed to trigger Cloud Build

Open Jenkins through IAP:

```bash
gcloud compute ssh jenkins-server \
  --tunnel-through-iap \
  --zone us-central1-a \
  -- -L 8080:localhost:8080
```

Then open:

```text
http://localhost:8080
```

## Deployment Command

Jenkins runs:

```bash
gcloud builds submit \
  --config cloudbuild-serverless.yaml \
  --substitutions "_PROJECT_ID=${GCP_PROJECT_ID},_REGION=${GCP_REGION},_KAFKA_BROKERS=${KAFKA_BROKERS},_REACT_APP_API_URL=${REACT_APP_API_URL},_OKTA_ISSUER=${OKTA_ISSUER},_OKTA_CLIENT_ID=${OKTA_CLIENT_ID},_OKTA_REDIRECT_URI=${OKTA_REDIRECT_URI},_OKTA_LOGOUT_REDIRECT_URI=${OKTA_LOGOUT_REDIRECT_URI}"
```

## Removed Infrastructure

The following Kubernetes/GKE pieces were removed:

- GKE cluster and node pool Terraform resources
- GKE node service account and Workload Identity binding
- Kubernetes YAML manifests
- Helm-based Jenkins-on-GKE files
- `kubectl` deployment scripts
- GKE rollout steps in Cloud Build and Makefile

The project now has one supported production runtime: Cloud Run.
