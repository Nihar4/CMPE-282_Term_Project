# Security

The Enterprise Knowledge Portal applies a **defense-in-depth** approach across
identity, network, workload, data, and supply-chain layers.

---

## 1. Identity Layer — Okta as the Source of Truth

### 1.1 SSO Topology

```
                  ┌────────────────────┐
                  │   Active Directory │
                  │  (corp on-prem)    │
                  └──────────┬─────────┘
                             │ SAML
                             ▼
                  ┌────────────────────┐
                  │       Okta         │  ← primary IdP (MFA, groups, lifecycle)
                  └─┬──────────┬──────┬─┘
              SAML  │     OIDC │      │ SCIM
                    ▼          ▼      ▼
            ┌────────────┐ ┌───────────┐ ┌──────────────┐
            │   GitHub   │ │  Portal   │ │ GCP Workforce│
            │ (org SSO)  │ │ (Auth0/OIDC)│ │  Identity   │
            └────────────┘ └───────────┘ └──────────────┘
                              │
                              ▼
                       ┌─────────────┐
                       │   Jenkins   │ (oic-auth plugin → Okta)
                       └─────────────┘
```

- **Okta** is the master directory; **AD** federates into it.
- **Auth0** is used as a thin OIDC layer for the React app — but the upstream
  identity is still Okta. Setting `auth-service` env to `OKTA_DOMAIN` directly
  makes Okta the OIDC issuer end-to-end.
- **GitHub Enterprise org** uses Okta SAML SSO + SCIM provisioning, so every
  developer commits with their Okta-managed identity.
- **Jenkins** authenticates via the `oic-auth` plugin against the same Okta
  tenant (configured in `DEPLOYMENT.md §5.4`).
- **GCP Workforce Identity Federation** lets `gcloud` and IAM consume Okta
  tokens, removing long-lived service account keys for human users.

### 1.2 Token flow

1. User → portal → Okta hosted login → Okta returns `id_token`.
2. `auth-service` validates the JWT against Okta JWKS (`exp`, `iss`, `aud`,
   `nonce`).
3. `auth-service` mints an internal **HS256 JWT** (`exp = 24h`, `aud=portal`)
   carrying `sub`, `email`, `role`, `dept`. The signing secret is in **Secret
   Manager** and mounted via `valueFrom.secretKeyRef`.
4. Frontend stores it in memory (with optional `localStorage` fallback) and
   refreshes silently.

### 1.3 Authorization

- `role` claim: `user`, `analyst`, `admin`.
- Gateway middleware:
  - `RequireAuth()` — verifies signature + expiry.
  - `RequireRole("admin")` — gate for `/api/admin/*`.
- DB-level: `data-service` connects as a read-only user with `pg_read_all_data`;
  AI-generated SQL therefore cannot mutate data even if validation is bypassed.

### 1.4 Session & token hygiene

- 24 h JWT lifetime; revocation list in Redis (`jwt:blocked:<jti>`).
- Logout: client deletes JWT and pushes the `jti` to the blocklist with TTL.
- `sub` is always the Okta `sub`; emails are mutable and never used as a primary
  key.

---

## 2. Network Layer

### 2.1 Edge

- GCP **HTTPS Load Balancer** terminates TLS with a Google-managed certificate.
- **Cloud Armor** policy attached to the LB:
  - Rate-limit: 100 req / IP / minute on `/api/auth/*`.
  - WAF rules: SQLi, XSS, RFI based on the OWASP CRS preset.
  - GeoIP block list: configurable per environment.

### 2.2 Cluster

- **Private GKE cluster** (no node public IPs).
- Subnet 10.0.0.0/16 with named secondary ranges (`pods`, `services`).
- East-west traffic stays inside the VPC.

### 2.3 Database & cache

- Cloud SQL **private IP only** (no public endpoint).
- Memorystore Redis with **AUTH** + transit encryption
  (`SERVER_AUTHENTICATION`).
- App connects via the cluster's VPC peering — credentials in Secret Manager.

---

## 3. Application Layer

### 3.1 API Gateway hardening

- **CORS allow-list** is explicit: only configured origins are reflected.
- The gateway **strips upstream Access-Control-* headers** (`ModifyResponse`)
  to prevent duplicates that browsers treat as CORS errors.
- **JWT middleware** rejects:
  - missing/expired tokens,
  - tokens signed with the wrong secret,
  - tokens with an `aud` mismatch.
- Per-IP **token-bucket rate limiter** (default 60 rps, burst 120).
- Sensitive endpoints (`/api/auth/*`) get a tighter limiter (10 rps).

### 3.2 Input validation

- All Gin handlers use `c.ShouldBindJSON` with struct tags
  (`binding:"required,email"` etc.).
- File upload: server-side MIME sniff + extension whitelist + 50 MB cap.

### 3.3 SQL safety

- Hand-written queries always parameterized via GORM.
- AI-generated SQL is parsed (`pg_query_go`) and rejected if the AST contains
  any of: `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `GRANT`,
  `REVOKE`, multi-statement.

### 3.4 Output encoding

- React escapes by default; `dangerouslySetInnerHTML` is forbidden by an ESLint
  rule.
- API responses set `Content-Type: application/json; charset=utf-8`.

---

## 4. Workload & Runtime

### 4.1 Container hardening

- **Multi-stage Docker builds** → final stage on `gcr.io/distroless/static`
  (Go) or `python:3.11-slim` (parser).
- All containers run as a **non-root** user.
- No shell in the Go images (distroless).
- `readOnlyRootFilesystem: true` (where feasible) and `allowPrivilegeEscalation:
  false` in PodSpecs.

### 4.2 Workload Identity

- Each Deployment uses a Kubernetes ServiceAccount that is bound to a GCP
  Service Account via `iam.workloadIdentityUser`. **No JSON key files** are
  shipped inside containers.
- file-service's KSA → GCP SA with `roles/storage.objectAdmin` on the bucket.
- ai-service's KSA → GCP SA with `roles/secretmanager.secretAccessor`.

### 4.3 Secret Management

- **GCP Secret Manager** holds long-lived secrets (`JWT_SECRET`, `DB_PASSWORD`,
  `NVIDIA_API_KEY`, `AUTH0_CLIENT_SECRET`).
- The K8s `Secret` resource is *generated* from Secret Manager via the
  `external-secrets-operator` (recommended). For the demo, a static `Secret`
  is shipped as `infrastructure/k8s/configmap.yaml` and rotated via
  `kubectl create secret … --dry-run=client -o yaml | kubectl apply -f -`.

### 4.4 Resource limits

- Every container has `requests` + `limits` set, and an HPA tracking CPU.
- PodDisruptionBudget keeps `minAvailable: 1` for each service during drains.

---

## 5. Data Layer

### 5.1 Encryption

- **At rest**: Cloud SQL, Memorystore, GCS, and Artifact Registry all encrypt
  with Google-managed keys by default; CMEK can be enabled per resource.
- **In transit**: TLS 1.2+ at the LB; Cloud SQL connections use the Cloud SQL
  Auth Proxy with mTLS; Redis uses TLS.

### 5.2 Backup & retention

- Cloud SQL: PITR enabled, 30 days of automated backups, daily 02:00 UTC.
- GCS: object versioning + 365-day lifecycle delete.
- Audit logs: 400 days retention in Cloud Logging.

### 5.3 PII & access control

- `enterprise_employees.salary` is masked for non-admin roles by the
  `data-service` projection (`SELECT … CASE WHEN role='admin' THEN salary
  ELSE NULL END`).
- Read-only DB user for AI service ensures NL → SQL cannot mutate data.

---

## 6. Supply Chain

### 6.1 Source

- **GitHub branch protection** on `main`:
  - Require PR review (≥1).
  - Require passing CI checks.
  - Require **signed commits** (Sigstore / GPG).
  - Linear history (no merge commits).
- **Dependabot** alerts on Go, npm, pip ecosystems.

### 6.2 Build

- **Trivy** scans every image for HIGH/CRITICAL CVEs in the `Security Scan`
  Jenkins stage. The pipeline currently runs in advisory mode (`--exit-code 0
  || true`); flip to `--exit-code 1` to block.
- GCR / Artifact Registry **vulnerability scanning** is enabled.
- Images are tagged with the **commit SHA**, providing immutable provenance.

### 6.3 Deploy

- Jenkins is the **only principal** allowed to push to `gcr.io/<proj>/`.
- Jenkins SA has `roles/container.developer` only — it can roll out, but not
  modify the cluster's IAM bindings or NodePool config.

---

## 7. Auditing & Compliance

| Concern                 | How it's met                                             |
| ----------------------- | -------------------------------------------------------- |
| Who logged in?          | Okta system log; portal `users.last_login_at`.           |
| What did they query?    | `query_history` table + Cloud Logging `data-service`.    |
| What did they upload?   | `uploaded_files` row + GCS audit logs (Data Access).     |
| Who changed prod?       | Jenkins build URL stored as deployment annotation.       |
| Image provenance        | GCR digest + commit SHA tag.                             |

---

## 8. Threats & Mitigations (STRIDE-lite)

| Threat (STRIDE)       | Example                                                       | Mitigation                                                                                  |
| --------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Spoofing              | Replay an old `id_token`                                       | OIDC `nonce`/`exp` checks, JWKS rotation, internal JWT short-lived.                         |
| Tampering             | Modify SQL after AI generates it                               | AST validation, allow-list of statements, parameterized exec.                               |
| Repudiation           | Admin denies user mgmt action                                  | Audit log of admin endpoints, immutable Cloud Logging, Okta event log.                      |
| Information disclosure| Leak salary data                                                | Role-based projection, encryption at rest, masked logs (`zap`/`zerolog` field redaction).   |
| Denial of service     | Brute-force login                                              | Cloud Armor + gateway rate limit + Okta brute-force protection.                             |
| Elevation of privilege| Attempt to call `/api/admin/*`                                 | Gateway middleware checks `role==admin`, deny by default.                                   |

---

## 9. Responsible Disclosure

If you discover a vulnerability in this educational project, please open a
private security advisory via GitHub (Security tab → Report a vulnerability)
rather than a public issue.
