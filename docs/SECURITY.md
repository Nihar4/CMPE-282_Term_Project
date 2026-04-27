# Security

The project uses a layered security model for the fully serverless Cloud Run deployment.

## Identity

- Okta OIDC protects the portal.
- Okta SAML protects Jenkins.
- The backend issues a portal JWT after successful Okta login.
- API gateway validates JWTs before proxying protected `/api/*` routes.

## Secrets

Secrets are not hardcoded.

Stored in Secret Manager:

- DB password
- JWT secret
- NVIDIA API key
- Okta client secret

Local `.env` is for development only and must not be committed.

## Network

- `frontend` and `api-gateway` are public Cloud Run services.
- Backend services are internal Cloud Run services.
- Cloud SQL uses private IP.
- Redis is private through Memorystore.
- Cloud Run reaches private services through Serverless VPC Access.

## Workloads

- Containers use multi-stage Docker builds.
- Cloud Run services use a dedicated service account.
- Backend services receive only required environment variables and secrets.
- Images are stored in Artifact Registry.

## CI/CD

```text
GitHub -> Jenkins -> Cloud Build -> Cloud Run
```

Jenkins uses a GCP service account to trigger Cloud Build. Cloud Build has IAM permissions for Artifact Registry, Terraform, Cloud Run, and Secret Manager access.

## Application Controls

- CORS is enforced at the API gateway.
- Rate limiting is implemented at the gateway.
- SQL generation is restricted to read-only queries.
- File uploads are parsed and chunked before AI usage.
- AI document mode caps context to top 5 chunks.
- AI jobs are idempotent through Redis job state.

## Break-Glass Jenkins Recovery

Keep one local Jenkins admin user available while testing SAML. Test Okta SAML login in an incognito window before logging out of the current admin session.

If SAML locks you out, temporarily switch Jenkins back to local user database in `/var/jenkins_home/config.xml`.
