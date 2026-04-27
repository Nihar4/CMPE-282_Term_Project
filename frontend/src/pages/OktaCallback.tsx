import React, { useEffect, useState } from 'react';
import { Box, CircularProgress, Typography } from '@mui/material';
import { api, API_BASE, saveAuthToken } from '../services/api';

const TOKEN_KEY = 'portal_auth_token';
const USER_KEY = 'portal_auth_user';

export default function OktaCallback() {
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const session = params.get('session');

    if (!session) {
      window.location.replace(`${API_BASE}/api/auth/callback${window.location.search}`);
      return;
    }

    (async () => {
      try {
        const resp = await api.get('/api/auth/session', { params: { session } });
        const { access_token, user } = resp.data;
        const portalUser = {
          email: user.email,
          name: user.name || user.email,
          picture: user.avatar_url || undefined,
          role: user.role,
          sub: user.id,
        };

        localStorage.setItem(TOKEN_KEY, access_token);
        localStorage.setItem(USER_KEY, JSON.stringify(portalUser));
        saveAuthToken(access_token);
        window.location.replace('/dashboard');
      } catch (err: any) {
        setError(err?.response?.data?.error || err?.message || 'Could not complete login');
      }
    })();
  }, []);

  return (
    <Box display="flex" flexDirection="column" gap={2} justifyContent="center" alignItems="center" minHeight="100vh">
      {error ? null : <CircularProgress size={48} />}
      <Typography color={error ? 'error' : 'text.secondary'}>
        {error || 'Completing Okta sign-in...'}
      </Typography>
    </Box>
  );
}
