# Project Report

## Summary

The Enterprise Knowledge Portal is a proof-of-concept cloud IT infrastructure for a modern enterprise data portal. It uses GCP managed services, Okta SSO, Kafka-based asynchronous processing, AI document analysis, and a fully serverless Cloud Run runtime.

## Final Deployment Architecture

```text
GitHub -> Jenkins -> Cloud Build -> Artifact Registry -> Cloud Run
```

All application services run on Cloud Run:

- frontend
- api-gateway
- auth-service
- data-service
- file-service
- parser-service
- ai-service
- analytics-service

Stateful services are managed:

- Cloud SQL PostgreSQL
- Memorystore Redis
- Cloud Storage
- Managed Kafka / Confluent Cloud
- Secret Manager

## Identity

Okta is used for SSO:

- Portal login uses Okta OIDC Authorization Code flow.
- Jenkins admin login uses Okta SAML.
- User profile data is stored in PostgreSQL after login.
- Dashboard/API routes are protected by the portal JWT.

## AI And Documents

AI requests are queued through Kafka topic `portal.ai.requests`.

The document pipeline uses RAG:

- Uploaded files are parsed into chunks.
- Chunks are stored in PostgreSQL.
- AI document mode uses hybrid search.
- Database querying and document querying run separately.
- The final LLM context is capped to the top 5 document chunks.

## CI/CD

Jenkins is connected to GitHub with a webhook. On push, Jenkins checks out the repo, authenticates to GCP, runs tests, and triggers Cloud Build.

Cloud Build builds all images, pushes them to Artifact Registry, and runs Terraform to deploy Cloud Run revisions.

## GCP Services Used

| Service | Purpose |
| --- | --- |
| Cloud Run | Serverless app runtime |
| Cloud Build | Build/deploy executor |
| Artifact Registry | Docker image storage |
| Cloud SQL | PostgreSQL database |
| Memorystore | Redis session/job/notification state |
| Cloud Storage | Uploaded files |
| Secret Manager | Runtime secrets |
| Serverless VPC Access | Private SQL/Redis connectivity |
| Cloud Logging | Central logs |
| Cloud Monitoring | Metrics and alerts |
| GCE | Jenkins controller VM |

## Result

The final project is cloud-native and serverless-first. Kubernetes/GKE has been removed from the deployment path so the team has one clean production model: Cloud Run.
