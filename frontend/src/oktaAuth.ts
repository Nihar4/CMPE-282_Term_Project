import { OktaAuth } from '@okta/okta-auth-js';

export const oktaAuth = new OktaAuth({
  issuer: process.env.REACT_APP_OKTA_ISSUER || 'https://trial-5413467.okta.com/oauth2/default',
  clientId: process.env.REACT_APP_OKTA_CLIENT_ID || '0oa12cfmwjeBVrl0I698',
  redirectUri: process.env.REACT_APP_OKTA_REDIRECT_URI || `${window.location.origin}/authorization-code/callback`,
  postLogoutRedirectUri: process.env.REACT_APP_OKTA_LOGOUT_REDIRECT_URI || window.location.origin,
  scopes: ['openid', 'profile', 'email'],
  pkce: true,
});
