# Deployment Guide

This project now uses one production deployment path:

```text
GitHub -> Jenkins -> Cloud Build -> Artifact Registry -> Cloud Run
```

There is no Kubernetes/GKE deployment path in the project anymore.

## 1. Local Test

Run the full local stack with Docker Compose:

```bash
cp .env.example .env
docker compose up -d --build \
  postgres redis zookeeper kafka \
  api-gateway auth-service data-service parser-service \
  file-service ai-service ai-worker analytics-service
```

Run the frontend locally:

```bash
cd frontend
npm install --legacy-peer-deps
npm start
```

Local URLs:

```text
Frontend:     http://localhost:3000
API Gateway:  http://localhost:8080
Parser docs:  http://localhost:8090/docs
```

## 2. GCP Infrastructure

Terraform provisions the serverless platform:

- Cloud Run for frontend, API gateway, auth, data, file, AI, analytics, and parser services
- Cloud SQL PostgreSQL
- Memorystore Redis
- Cloud Storage file bucket
- Artifact Registry Docker repository
- Secret Manager for DB/JWT/NVIDIA/Okta secrets
- Serverless VPC Access for private Cloud SQL and Redis access
- Jenkins VM on GCE for GitHub webhook driven CI/CD
- Cloud Monitoring and Cloud Logging

Initialize and apply:

```bash
cd infrastructure/terraform
terraform init
terraform apply -auto-approve \
  -var="deploy_serverless=true" \
  -var="project_id=enterprise-portal-48689" \
  -var="region=us-central1"
```

## 3. Jenkins CI/CD

Jenkins receives the GitHub webhook and triggers Cloud Build.

Required Jenkins credentials:

```text
github-token
GCP_SERVICE_ACCOUNT_KEY
```

Required Jenkins environment values in `Jenkinsfile`:

```text
GCP_PROJECT_ID=enterprise-portal-48689
GCP_REGION=us-central1
KAFKA_BROKERS=<managed kafka bootstrap servers>
REACT_APP_API_URL=<Cloud Run api-gateway URL>
OKTA_ISSUER=https://trial-5413467.okta.com/oauth2/default
OKTA_CLIENT_ID=<Okta OIDC client id>
OKTA_REDIRECT_URI=<Cloud Run frontend URL>/authorization-code/callback
OKTA_LOGOUT_REDIRECT_URI=<Cloud Run frontend URL>
```

GitHub webhook:

```text
Payload URL: http://<jenkins-host>/github-webhook/
Content type: application/json
Events: push
```

## 4. Cloud Build

Jenkins runs:

```bash
gcloud builds submit \
  --config cloudbuild-serverless.yaml \
  --substitutions "_PROJECT_ID=${GCP_PROJECT_ID},_REGION=${GCP_REGION},_KAFKA_BROKERS=${KAFKA_BROKERS},_REACT_APP_API_URL=${REACT_APP_API_URL},_OKTA_ISSUER=${OKTA_ISSUER},_OKTA_CLIENT_ID=${OKTA_CLIENT_ID},_OKTA_REDIRECT_URI=${OKTA_REDIRECT_URI},_OKTA_LOGOUT_REDIRECT_URI=${OKTA_LOGOUT_REDIRECT_URI}"
```

Cloud Build then:

- Builds all backend images
- Builds parser-service
- Builds frontend with Okta and API URL build args
- Pushes all images to Artifact Registry
- Runs Terraform with `deploy_serverless=true`
- Updates Cloud Run services to the latest images

## 5. Manual Deploy

You can bypass Jenkins for testing:

```bash
make cloud-run-deploy
```

Or:

```bash
./scripts/deploy.sh
```

## 6. Get Cloud Run URLs

```bash
gcloud run services list --region us-central1
gcloud run services describe frontend --region us-central1 --format='value(status.url)'
gcloud run services describe api-gateway --region us-central1 --format='value(status.url)'
```

After the first deployment, update Okta:

```text
Sign-in redirect URI:
<frontend-url>/authorization-code/callback

Sign-out redirect URI:
<frontend-url>
```

Then update Jenkins `OKTA_REDIRECT_URI`, `OKTA_LOGOUT_REDIRECT_URI`, and `REACT_APP_API_URL`, and rerun the pipeline.

## 7. Rollback

Cloud Run keeps revisions. Roll back from the console:

```text
Cloud Run -> frontend/api-gateway/service -> Revisions -> Manage Traffic
```

Or with `gcloud`:

```bash
gcloud run services update-traffic api-gateway \
  --region us-central1 \
  --to-revisions <previous-revision>=100
```

## 8. Tear Down

```bash
cd infrastructure/terraform
terraform destroy
```

Cloud SQL has deletion protection enabled. Disable it intentionally before destroying a production environment.
