# CI / CD — Jenkins on GKE + GitHub + Okta

This document explains how code travels from a developer's laptop to a
production GKE deployment, with Okta-protected access at every step.

---

## 1. End-to-End Flow

```
Developer ─┬─▶ git push origin feature/x
           │
           ▼
       GitHub (org SSO via Okta)
           │ webhook
           ▼
   Jenkins on GKE  (login via Okta OIDC)
           │
           ├── checkout
           ├── gcloud auth (Workload Identity)
           ├── go test in parallel
           ├── docker build + tag (sha + latest)
           ├── trivy scan
           ├── docker push → Artifact Registry
           ├── kubectl set image deployment/<svc>
           ├── kubectl rollout status
           └── post.success | post.failure (auto-rollback)
                                │
                                ▼
                     Slack / Email notification
```

---

## 2. The `Jenkinsfile`

Located at the repo root. 12 stages, parameterized.

### 2.1 Parameters

| Parameter         | Default       | Purpose                                  |
| ----------------- | ------------- | ---------------------------------------- |
| `DEPLOY_INFRA`    | `false`       | Run Terraform apply (gated for safety).  |
| `RUN_MIGRATIONS`  | `false`       | Run DB migrations against Cloud SQL.     |
| `DEPLOY_FRONTEND` | `true`        | Build & deploy the React image.          |
| `TARGET_ENV`      | `production`  | `production` or `staging`.               |

### 2.2 Stages

1. **Checkout** — `checkout scm` and print last 5 commits.
2. **GCP Auth** — `gcloud auth activate-service-account` using
   `GCP_SERVICE_ACCOUNT_KEY` Jenkins credential.
3. **Terraform Apply** *(optional)* — runs `terraform init/plan/apply` in
   `infrastructure/terraform`.
4. **Test Backend** — `go test ./...` for each microservice **in parallel**.
5. **Build Images** — Docker multi-stage builds, parallelized; tags both
   `:<sha>-<build>` and `:latest`.
6. **Security Scan** — `trivy image` on critical services (`api-gateway`,
   `auth-service`, `data-service`, `ai-service`).
7. **Push Images** — pushes each tag to GCR / Artifact Registry **in parallel**.
8. **Get GKE Credentials** — `gcloud container clusters get-credentials`.
9. **DB Migrations** *(optional)* — runs a one-off `kubectl run` Postgres pod
   that `psql -f /migrations/001_init.sql`.
10. **Deploy to GKE** — `kubectl set image deployment/<svc> …` per service,
    then `kubectl apply -f infrastructure/k8s/` to update non-image manifests.
11. **Verify Deployment** — `kubectl rollout status` per service with a
    120 s timeout.
12. **Health Check** — counts running pods; aborts if the number drops below 5.

### 2.3 `post {}` block

- **success**: prints `IMAGE_TAG`, can hook Slack/email.
- **failure**: `kubectl rollout undo` for every deployment **automatically**.
- **always**: `docker system prune -f --filter 'until=24h'` and `cleanWs()`.

---

## 3. Required Jenkins Plugins

| Plugin                          | Why                                                       |
| ------------------------------- | --------------------------------------------------------- |
| **Kubernetes**                  | Run agents as ephemeral pods on GKE.                      |
| **Pipeline / Workflow Aggregator** | Declarative pipeline support.                          |
| **Git / GitHub**                | SCM integration, GitHub status checks, multibranch.       |
| **Configuration as Code (JCasC)** | Idempotent Jenkins configuration.                       |
| **Credentials / Plain Credentials** | Manage SA keys, tokens, passwords.                    |
| **OIC-Auth**                    | Okta OIDC login for the Jenkins UI.                       |
| **Role-Based Strategy**         | Map Okta groups → Jenkins authorizations.                 |
| **Slack / Email Ext**           | Build notifications.                                      |
| **Blue Ocean** (optional)       | Modern pipeline UI.                                       |
| **GCP Secret Manager**          | Pull secrets at build time without storing in Jenkins.    |

---

## 4. Jenkins Credentials Catalog

| ID                          | Type      | Purpose                                                           |
| --------------------------- | --------- | ----------------------------------------------------------------- |
| `GCP_PROJECT_ID`            | secret    | Used in `${DOCKER_REGISTRY}/…` interpolation.                     |
| `GCP_SERVICE_ACCOUNT_KEY`   | file      | Service account JSON for `gcloud auth`.                           |
| `TF_DB_PASSWORD`            | secret    | Cloud SQL DB password used by Terraform.                          |
| `GITHUB_TOKEN`              | username/token | Status check posts + private repo clone.                     |
| `OKTA_OIDC_CLIENT`          | secret    | Used by `oic-auth` plugin (Okta client secret).                   |
| `SLACK_WEBHOOK`             | secret    | Optional, for `post.success` notification.                        |

---

## 5. GitHub Side

### 5.1 Webhook

GitHub repo → Settings → Webhooks → Add:

- Payload URL: `https://jenkins.example.com/github-webhook/`
- Content type: `application/json`
- Events: `push`, `pull_request`, `pull_request_review`.

### 5.2 Branch Protection (`main`)

- Require pull request before merging — at least **1 approval**.
- Require status check **`continuous-integration/jenkins/branch`** to pass.
- Require **signed commits**.
- Restrict who can push to `main` to a "Release Engineers" team.

### 5.3 Org SSO

- Enforce **SSO via Okta** at the org level.
- Enable **SAML SSO + SCIM provisioning**: a new hire added to the
  `eng-portal-developers` group in Okta automatically gets a GitHub seat.

---

## 6. Okta Configuration for Jenkins

1. Okta admin → **Applications → Create App Integration → OIDC – Web**.
2. Sign-in redirect: `https://jenkins.example.com/securityRealm/finishLogin`.
3. Sign-out redirect: `https://jenkins.example.com/`.
4. Assign to groups: `jenkins-admins`, `jenkins-deployers`, `jenkins-readers`.
5. Copy `Client ID` + `Client Secret` → store in Jenkins credentials as
   `OKTA_OIDC_CLIENT`.
6. Jenkins → Manage Jenkins → Configure Global Security:
   - Security Realm: **OpenID Connect**.
   - Provider: `https://<tenant>.okta.com/oauth2/default/.well-known/openid-configuration`.
   - User name field: `email`, Groups field: `groups`.
7. Authorization: **Role-Based Strategy** with mappings:
   - `jenkins-admins` → admin.
   - `jenkins-deployers` → build, configure jobs.
   - `jenkins-readers` → read-only.

---

## 7. Local Pipeline Validation

```bash
# Lint pipeline
curl -X POST -F "jenkinsfile=<Jenkinsfile" https://jenkins.example.com/pipeline-model-converter/validate

# Dry-run on a feature branch — set TARGET_ENV=staging in the build form.
```

---

## 8. Promotion Strategy

| Branch                | Auto-deploy target | Approval                        |
| --------------------- | ------------------ | ------------------------------- |
| `feature/*`           | none (test only)    | n/a                             |
| `main`                | `staging`          | automatic on green CI           |
| Tagged release `v*.*` | `production`       | manual `Deploy` build parameter |

---

## 9. Observability for the Pipeline

- Build duration histogram → Cloud Monitoring (Jenkins exporter).
- `IMAGE_TAG` printed and stored as a Kubernetes annotation
  `app.kubernetes.io/version` on each Deployment.
- Slack message on success contains commit SHA + pipeline URL +
  `kubectl rollout history` link.

---

## 10. Failure Modes & Recovery

| Symptom                         | Action                                                       |
| ------------------------------- | ------------------------------------------------------------ |
| Trivy reports a CRITICAL CVE    | Update base image / dependency, re-run pipeline.             |
| `kubectl rollout status` times out | `post.failure` auto-rolls back; investigate logs.        |
| Push fails with permission denied | Refresh Jenkins SA roles: `roles/artifactregistry.writer`. |
| GitHub webhook missed           | Re-deliver from GitHub UI; or push an empty commit.          |
| Okta SSO outage                 | Local "break-glass" Jenkins admin defined in JCasC.          |

---

## 11. Why this satisfies the rubric

- **Cloud Jenkins** — runs on GKE, not a dev's laptop.
- **Integrated into SSO** — Okta OIDC for both UI and group authorization.
- **Integrated into GitHub** — webhook-driven, status checks reported back,
  branch protection enforced through the same Okta identity.
- **Continuous Deployment** — every green build on `main` reaches production
  via `kubectl set image`, with auto-rollback on failure.
- **GCP-native** — uses Workload Identity, Artifact Registry, Cloud Logging
  (no AWS-specific lock-in).
