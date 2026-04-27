/**
 * AuthContext — portal session state for Okta OIDC and local dev login.
 * Stores JWT token in localStorage when the backend returns one.
 */
import React, { createContext, useContext, useState, useCallback, useEffect } from 'react';
import { api, API_BASE, saveAuthToken, clearAuthToken, getMe } from '../services/api';
import { oktaAuth } from '../oktaAuth';

const TOKEN_KEY = 'portal_auth_token';
const USER_KEY  = 'portal_auth_user';

function decodeJwtClaims(token: string): any {
  try {
    const payload = token.split('.')[1];
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(window.atob(normalized));
  } catch {
    return {};
  }
}

export interface PortalUser {
  email:      string;
  name:       string;
  picture?:   string;
  role?:      string;
  sub?:       string;
}

interface AuthContextValue {
  isAuthenticated: boolean;
  isLoading:       boolean;
  user:            PortalUser | null;
  loginWithEmail:  (email: string, password: string) => Promise<void>;
  loginWithOkta:   () => void;
  logout:          () => void;
  error:           string | null;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [isLoading, setIsLoading]           = useState(true);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [user, setUser]                     = useState<PortalUser | null>(null);
  const [error, setError]                   = useState<string | null>(null);

  // Restore a local JWT or an HttpOnly cookie-backed Okta session on mount.
  useEffect(() => {
    (async () => {
      try {
        const token = localStorage.getItem(TOKEN_KEY);
        const raw   = localStorage.getItem(USER_KEY);
        if (token && raw) {
          const u = JSON.parse(raw) as PortalUser;
          saveAuthToken(token);
          setUser(u);
          setIsAuthenticated(true);
          return;
        }
        const profile = await getMe();
        const portalUser: PortalUser = {
          email:   profile.email,
          name:    profile.name || profile.email,
          picture: profile.avatar_url || undefined,
          role:    profile.role,
          sub:     profile.id,
        };
        localStorage.setItem(USER_KEY, JSON.stringify(portalUser));
        setUser(portalUser);
        setIsAuthenticated(true);
      } catch {
        clearAuthToken();
        localStorage.removeItem(TOKEN_KEY);
        localStorage.removeItem(USER_KEY);
      } finally {
        setIsLoading(false);
      }
    })();
  }, []);

  const loginWithEmail = useCallback(async (email: string, password: string) => {
    setError(null);
    setIsLoading(true);
    try {
      const resp = await api.post('/api/auth/dev-login', { email, password });
      const { access_token, user: u } = resp.data;

      const portalUser: PortalUser = {
        email:   u.email   || email,
        name:    u.name    || u.email || email,
        picture: u.avatar_url || undefined,
        role:    u.role,
        sub:     u.id,
      };

      localStorage.setItem(TOKEN_KEY, access_token);
      localStorage.setItem(USER_KEY,  JSON.stringify(portalUser));
      saveAuthToken(access_token);

      setUser(portalUser);
      setIsAuthenticated(true);
    } catch (err: any) {
      const msg = err?.response?.data?.error || err?.message || 'Login failed';
      setError(msg);
      throw new Error(msg);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const loginWithOkta = useCallback(() => {
    window.location.href = `${API_BASE}/api/auth/login`;
  }, []);

  const logout = useCallback(async () => {
    let idTokenHint: string | undefined;
    try {
      const resp = await api.get('/api/auth/logout-token');
      idTokenHint = resp.data?.id_token;
    } catch {
      // Dev-login sessions do not have an Okta ID token.
    }

    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    clearAuthToken();
    setUser(null);
    setIsAuthenticated(false);

    try {
      await api.post('/api/auth/logout', {});
    } catch {
      // Continue client-side logout even if the server session is already gone.
    }

    const postLogoutRedirectUri = process.env.REACT_APP_OKTA_LOGOUT_REDIRECT_URI || window.location.origin;
    if (!idTokenHint) {
      window.location.href = '/login';
      return;
    }

    const claims = decodeJwtClaims(idTokenHint);
    await oktaAuth.tokenManager.clear();
    await oktaAuth.tokenManager.add('idToken', {
      idToken: idTokenHint,
      claims,
      expiresAt: claims.exp || Math.floor(Date.now() / 1000) + 300,
      scopes: ['openid', 'profile', 'email'],
      authorizeUrl: `${process.env.REACT_APP_OKTA_ISSUER || 'https://trial-5413467.okta.com/oauth2/default'}/v1/authorize`,
      issuer: process.env.REACT_APP_OKTA_ISSUER || 'https://trial-5413467.okta.com/oauth2/default',
      clientId: process.env.REACT_APP_OKTA_CLIENT_ID || '0oa12cfmwjeBVrl0I698',
    } as any);

    try {
      await oktaAuth.signOut({
        postLogoutRedirectUri,
        revokeAccessToken: false,
        revokeRefreshToken: false,
      });
    } catch {
      window.location.href = `${API_BASE}/api/auth/logout`;
    }
  }, []);

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, user, loginWithEmail, loginWithOkta, logout, error }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
