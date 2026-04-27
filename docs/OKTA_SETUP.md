# Okta OIDC and Jenkins SSO Setup

## Portal Okta OIDC

The portal uses Okta OpenID Connect Authorization Code flow:

1. Browser opens `/api/auth/login`.
2. `auth-service` redirects to Okta `/authorize`.
3. Okta redirects back to the React route `/authorization-code/callback`.
4. The React callback forwards `code` and `state` to `/api/auth/callback`.
5. `auth-service` exchanges the code for tokens using `OKTA_CLIENT_SECRET`.
6. The portal stores an HttpOnly session cookie and serves `/api/auth/me`.

### Localhost Okta App Settings

Use these values in the Okta OIDC application:

- Sign-in redirect URI: `http://localhost:3000/authorization-code/callback`
- Sign-out redirect URI: `http://localhost:3000`
- Grant types: `Authorization Code`, `Refresh Token`
- Application type: `Web`

### Local `.env`

Set these values in `.env`. Keep the secret out of committed files.

```bash
OKTA_CLIENT_ID=your-okta-client-id
OKTA_CLIENT_SECRET=your-okta-client-secret
OKTA_ISSUER=https://your-okta-domain.okta.com/oauth2/default
OKTA_REDIRECT_URI=http://localhost:3000/authorization-code/callback
OKTA_LOGOUT_REDIRECT_URI=http://localhost:3000
FRONTEND_URL=http://localhost:3000
```

For Cloud Run, use the deployed frontend URL:

```bash
OKTA_REDIRECT_URI=https://<frontend-cloud-run-url>/authorization-code/callback
OKTA_LOGOUT_REDIRECT_URI=https://<frontend-cloud-run-url>
FRONTEND_URL=https://<frontend-cloud-run-url>
```

Store `OKTA_CLIENT_SECRET` in Secret Manager as `portal-okta-client-secret`.

## Jenkins Okta SAML

The CI/CD flow is:

```text
GitHub -> Jenkins -> Cloud Build -> Cloud Run
```

For Jenkins SAML, the two Okta fields below come from Jenkins, not from Okta:

- Single sign-on URL: copy Jenkins `ACS URL` / `SSO URL`
- Audience URI / SP Entity ID: copy Jenkins `Entity ID`

Setup order:

1. Open Jenkins.
2. Go to `Manage Jenkins`.
3. Install the Jenkins `SAML` plugin.
4. Open Jenkins SAML configuration.
5. Copy `ACS URL` / `SSO URL`.
6. Copy `Entity ID`.
7. In Okta, create or edit the Jenkins SAML app.
8. Paste Jenkins `ACS URL` into Okta `Single sign-on URL`.
9. Paste Jenkins `Entity ID` into Okta `Audience URI / SP Entity ID`.
