# CI/CD

The project uses this deployment flow:

```text
GitHub -> Jenkins -> Cloud Build -> Artifact Registry -> Cloud Run
```

## Jenkins Role

Jenkins is the CI/CD orchestrator.

It does:

- Receives GitHub webhooks
- Checks out the repository
- Authenticates to GCP
- Runs backend tests
- Calls Cloud Build with `cloudbuild-serverless.yaml`

It does not deploy with Kubernetes or `kubectl`.

## Cloud Build Role

Cloud Build is the deployment executor.

It does:

- Build backend service images
- Build parser-service
- Build frontend with Cloud Run/Okta environment values
- Push all images to Artifact Registry
- Run Terraform
- Deploy/update Cloud Run services

## Jenkinsfile

Root file:

```text
Jenkinsfile
```

Main stages:

1. Checkout
2. GCP Auth
3. Test Backend
4. Cloud Build -> Cloud Run

The deployment command is:

```bash
gcloud builds submit \
  --config cloudbuild-serverless.yaml \
  --substitutions "_PROJECT_ID=${GCP_PROJECT_ID},_REGION=${GCP_REGION},_KAFKA_BROKERS=${KAFKA_BROKERS},_REACT_APP_API_URL=${REACT_APP_API_URL},_OKTA_ISSUER=${OKTA_ISSUER},_OKTA_CLIENT_ID=${OKTA_CLIENT_ID},_OKTA_REDIRECT_URI=${OKTA_REDIRECT_URI},_OKTA_LOGOUT_REDIRECT_URI=${OKTA_LOGOUT_REDIRECT_URI}"
```

## Required Jenkins Credentials

| ID | Type | Purpose |
| --- | --- | --- |
| `github-token` | username/token | Checkout private GitHub repo if needed |
| `GCP_SERVICE_ACCOUNT_KEY` | file | GCP service account JSON for `gcloud auth` |

## Required Jenkins Plugins

| Plugin | Purpose |
| --- | --- |
| Pipeline / Workflow Aggregator | Declarative Jenkinsfile support |
| Git | Git checkout |
| GitHub | Webhook integration |
| Credentials Binding | Inject GCP key file |
| SAML 2.0 | Okta SAML SSO for Jenkins admin login |
| Timestamper | Better build logs |

## GitHub Webhook

In GitHub:

```text
Settings -> Webhooks -> Add webhook
```

Use:

```text
Payload URL: http://<jenkins-url>/github-webhook/
Content type: application/json
Events: Just the push event
```

## Okta SSO For Jenkins

Jenkins uses Okta as SAML IdP.

Jenkins values copied into Okta:

```text
Single sign-on URL / ACS URL:
http://<jenkins-url>/securityRealm/finishLogin

Audience URI / SP Entity ID:
http://<jenkins-url>/
```

Okta values copied back into Jenkins:

```text
IdP Metadata URL
```

or manually:

```text
IdP SSO URL
IdP Issuer
X.509 certificate
```

## Cloud Run Rollback

Rollback is handled by Cloud Run revisions:

```bash
gcloud run services update-traffic api-gateway \
  --region us-central1 \
  --to-revisions <previous-revision>=100
```

## Why This Matches The Requirement

- GitHub is the source trigger.
- Jenkins is the controlled CI/CD entry point.
- Cloud Build handles container builds and Terraform deployment.
- Cloud Run is the only application runtime.
- Okta protects Jenkins and the portal.
