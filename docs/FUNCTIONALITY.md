# Functionality

End-to-end walk-through of every user-facing feature in the Enterprise Knowledge
Portal, the underlying API contracts, and the data they touch.

---

## 1. Personas

| Persona            | Typical actions                                                                |
| ------------------ | ------------------------------------------------------------------------------ |
| **Employee**       | Browse data, ask AI questions, upload reference docs.                          |
| **Analyst**        | Ad-hoc SQL via natural language, build custom analytics reports.               |
| **Admin / IT**     | Manage users (via Okta), monitor system health, configure data sources.       |

All three log in with the same Okta / Auth0 SSO; **role** (`user`, `analyst`,
`admin`) is sourced from the IdP claim and stored in `users.role`.

---

## 2. Pages & Components (Frontend)

| Route          | Component         | Purpose                                                |
| -------------- | ----------------- | ------------------------------------------------------ |
| `/login`       | `Login.tsx`       | Auth0 / Okta universal login redirect, dev-login fallback. |
| `/`            | `Dashboard.tsx`   | Top-line KPIs, recent uploads, "Ask AI" quick prompt.  |
| `/data`        | `DataBrowser.tsx` | Paginated, sortable, filterable enterprise tables.    |
| `/files`       | `FileUpload.tsx`  | Drag-and-drop upload, list, delete, chunk preview.    |
| `/ai`          | `AIChat.tsx`      | NL→SQL chat + document Q&A with file context.         |
| `/analytics`   | `Analytics.tsx`   | Pre-built reports + custom report builder.            |

All routes are guarded by an `<AuthGuard>` wrapper that redirects to `/login` if
no JWT is in `localStorage`.

---

## 3. Authentication & SSO

### 3.1 What the user sees

1. Visits `https://portal.example.com`.
2. Redirected to Okta / Auth0 hosted login.
3. (Optional) AD/social/MFA challenge depending on tenant policy.
4. Auth0 redirects back to `/callback` with an `id_token`.
5. Frontend exchanges it at `POST /api/auth/exchange` for an internal JWT.
6. JWT stored in memory + `localStorage`, refreshed silently every 15 min.

### 3.2 What happens server-side

- `auth-service` validates the IdP token against the JWKS endpoint.
- A row is upserted in `users` keyed on `okta_id` / `email`.
- An internal JWT (HS256, `exp = 24h`) is signed with the `JWT_SECRET` from
  Secret Manager.
- The internal JWT carries: `sub`, `email`, `role`, `dept`, `iat`, `exp`.

### 3.3 Dev mode

When `DEV_MODE=true`, `POST /api/auth/dev-login` returns a JWT for a synthetic
admin user. This is **disabled in production** by the Helm/K8s config map.

---

## 4. Data Browser

- `GET /api/data/employees?limit=50&offset=0&q=jane`
- `GET /api/data/products?category=Electronics&sort=-unit_price`
- `GET /api/data/sales?from=2024-01-01&to=2024-03-31&region=NA`
- `GET /api/data/inventory?warehouse=US-WEST`
- `GET /api/data/departments`

Each endpoint returns:

```jsonc
{
  "items": [ /* rows */ ],
  "total": 1234,
  "limit": 50,
  "offset": 0
}
```

Frontend renders rows in MUI `DataGrid` with column toggles, density switch,
quick filter, and CSV export.

---

## 5. AI Chat

### 5.1 Modes

- **NL → SQL** — over the migrated enterprise data.
- **Document Q&A** — over a selected uploaded file.
- **General** — fallback chat.

### 5.2 API

`POST /api/ai/query`

```jsonc
{
  "mode": "nl_to_sql" | "document_qa" | "general",
  "query": "Top 5 sales reps in Q1 2024",
  "context_file_ids": ["uuid"],   // for document_qa
  "history": [ /* prior messages */ ]
}
```

Response:

```jsonc
{
  "answer": "Here are the top 5 reps …",
  "generated_sql": "SELECT … FROM enterprise_sales s JOIN enterprise_employees e …",
  "rows": [ … ],
  "rows_count": 5,
  "execution_time_ms": 412,
  "sources": [{ "file_id": "uuid", "chunk_index": 3 }]
}
```

### 5.3 Safety

- AI-generated SQL is **parsed** and rejected if it contains `UPDATE`, `DELETE`,
  `INSERT`, `DROP`, `ALTER`, or multi-statement blocks.
- Queries run as a read-only DB user with `pg_read_all_data`.

---

## 6. File Upload & Document Repository

### 6.1 Supported formats

| Type | MIME                                                                                  | Parser                              |
| ---- | ------------------------------------------------------------------------------------- | ----------------------------------- |
| CSV  | `text/csv`                                                                            | Python `csv.reader`                 |
| PDF  | `application/pdf`                                                                     | `pypdf.PdfReader`                   |
| DOCX | `application/vnd.openxmlformats-officedocument.wordprocessingml.document`             | `python-docx` + OOXML fallback      |
| TXT  | `text/plain`                                                                          | UTF-8 decode + chunker              |

Max upload size: **50 MB** (enforced both client- and server-side).

### 6.2 Upload flow

1. Drag & drop or click-to-browse in `FileUpload.tsx`.
2. Multipart `POST /api/files` to gateway → `file-service`.
3. `file-service` writes the blob to GCS / local volume, inserts
   `uploaded_files` with `status='processing'`.
4. `file-service` POSTs to `parser-service:/parse`.
5. `parser-service` extracts text → returns chunked array.
6. `file-service` writes chunks to `file_chunks`, sets `status='ready'`.
7. UI polls `GET /api/files` every 3 s while any file is `processing`.

### 6.3 Endpoints

- `POST /api/files` — multipart upload.
- `GET /api/files` — list user's files.
- `GET /api/files/:id` — metadata for one file.
- `GET /api/files/:id/chunks` — extracted chunks.
- `DELETE /api/files/:id` — single delete.
- `DELETE /api/files/` — **bulk delete** (used by the "Delete All" UI button).

### 6.4 UX details

- Status indicator shown as **icon-only** (check / hourglass / error). The
  textual "ready / processing / error" label was removed in favour of clean
  iconography.
- "Delete All" button confirms with a native dialog before bulk-deleting.

---

## 7. Analytics

### 7.1 Pre-built reports

| Report               | Endpoint                              | Visual         |
| -------------------- | ------------------------------------- | -------------- |
| Sales summary        | `GET /api/analytics/sales-summary`    | line + KPI    |
| Top products         | `GET /api/analytics/top-products`     | bar chart      |
| Headcount by dept    | `GET /api/analytics/headcount`        | pie chart      |
| Inventory health     | `GET /api/analytics/inventory`        | gauges + table |

### 7.2 Custom report builder

`POST /api/analytics/custom`

```jsonc
{
  "title": "Q3 NA Top Customers",
  "table": "enterprise_sales",
  "filters": { "region": "NA", "sale_date": { "$gte": "2024-07-01" } },
  "group_by": ["customer_name"],
  "aggregates": [{ "fn": "SUM", "column": "total_amount", "as": "revenue" }],
  "order_by": [{ "column": "revenue", "desc": true }],
  "limit": 25
}
```

Response includes `rows`, recommended `chart_type`, and a saveable `report_id`.

---

## 8. Admin Surface

- `GET /api/admin/users` (role=admin) — list IdP-provisioned users.
- `PATCH /api/admin/users/:id` — toggle `is_active`, change role.
- `GET /api/admin/queries?limit=50` — recent NL queries with cost/latency.
- `GET /api/admin/files` — system-wide file index.

> Role enforcement is in the gateway: `RequireRole("admin")` middleware applied
> on the `/api/admin/*` route group.

---

## 9. Health & Observability Endpoints

| Endpoint               | Purpose                                              |
| ---------------------- | ---------------------------------------------------- |
| `/health`              | 200 OK if process is alive (used by liveness probe). |
| `/ready`               | 200 OK only when DB + Redis dependencies pass.       |
| `/metrics` (Prom)      | RED metrics + Go runtime metrics.                    |
| `/version`             | Build SHA + image tag.                               |

---

## 10. End-to-End User Journeys

### 10.1 New employee onboarding

1. IT adds the user in Okta → Okta SCIM-syncs into Auth0/portal.
2. User logs in for the first time → JIT row in `users`.
3. Dashboard shows Welcome card, links to Data Browser and AI Chat.

### 10.2 Analyst answers an exec question

1. Analyst opens AI Chat → asks "Which 3 products had the largest YoY revenue
   drop in 2024?"
2. `ai-service` introspects schema, generates SQL, executes, returns rows.
3. Analyst saves the result as a custom report; it now appears under
   `/analytics`.

### 10.3 Document-grounded Q&A

1. Sales rep uploads `Q3_Forecast.pdf`.
2. After ~3 s the file shows the green check icon.
3. They open AI Chat, switch to "Document mode", select the file, ask
   "Summarize the key risks called out in the forecast."
4. The answer is returned with citations linking back to chunks.

---

## 11. Out-of-Scope (Future)

- Salesforce sync (read CRM accounts/opportunities into the same UI).
- Slack/Teams chatbot bridge to the same AI service.
- pgvector / Vector Search for semantic retrieval (beyond FTS).
- Multi-tenant org isolation (one DB / org).
