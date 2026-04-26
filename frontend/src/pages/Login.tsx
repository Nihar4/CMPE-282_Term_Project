import React from 'react';
import { useAuth0 } from '@auth0/auth0-react';
import { Navigate, useLocation } from 'react-router-dom';
import {
  Box, Button, Typography, Card, CardContent, Divider,
  CircularProgress, Stack, Chip,
} from '@mui/material';
import {
  Login as LoginIcon, Security as SecurityIcon,
  Storage as StorageIcon, SmartToy as AIIcon,
  BarChart as ChartIcon, Business as BusinessIcon,
} from '@mui/icons-material';

const FEATURES = [
  { icon: <StorageIcon />, label: 'Enterprise Data Browser',   desc: 'Employees, products, sales & inventory' },
  { icon: <AIIcon />,      label: 'AI-Powered Query Engine',   desc: 'Ask questions in plain English' },
  { icon: <ChartIcon />,   label: 'Real-time Analytics',       desc: 'Dashboards, trends & KPI reports' },
  { icon: <SecurityIcon />,label: 'Secure SSO via Auth0',      desc: 'Enterprise-grade authentication' },
];

export default function Login() {
  const { loginWithRedirect, isAuthenticated, isLoading } = useAuth0();
  const location = useLocation();

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh"
           sx={{ background: 'linear-gradient(135deg, #1a237e 0%, #0288d1 100%)' }}>
        <CircularProgress size={56} sx={{ color: '#fff' }} />
      </Box>
    );
  }

  if (isAuthenticated) return <Navigate to="/dashboard" replace />;

  return (
    <Box sx={{
      minHeight: '100vh',
      background: 'linear-gradient(135deg, #1a237e 0%, #0288d1 100%)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      p: 2,
    }}>
      <Box sx={{ display: 'flex', gap: 4, alignItems: 'center', maxWidth: 960, width: '100%', flexWrap: 'wrap' }}>

        {/* Left — branding */}
        <Box sx={{ flex: 1, minWidth: 280, color: '#fff' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
            <Box sx={{
              width: 56, height: 56, borderRadius: 3,
              bgcolor: 'rgba(255,255,255,0.2)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
              <BusinessIcon sx={{ fontSize: 32, color: '#fff' }} />
            </Box>
            <Box>
              <Typography variant="h5" fontWeight={700}>Enterprise</Typography>
              <Typography variant="body2" sx={{ opacity: 0.85 }}>Knowledge Portal</Typography>
            </Box>
          </Box>

          <Typography variant="h4" fontWeight={700} mb={1}>
            AI-Powered Data Access
          </Typography>
          <Typography variant="body1" sx={{ opacity: 0.85, mb: 4, lineHeight: 1.7 }}>
            Query your enterprise data using natural language, upload documents for
            AI analysis, and explore real-time business insights.
          </Typography>

          <Stack spacing={2}>
            {FEATURES.map(f => (
              <Box key={f.label} sx={{ display: 'flex', alignItems: 'flex-start', gap: 1.5 }}>
                <Box sx={{ color: 'rgba(255,255,255,0.8)', mt: 0.3 }}>{f.icon}</Box>
                <Box>
                  <Typography variant="body2" fontWeight={600}>{f.label}</Typography>
                  <Typography variant="caption" sx={{ opacity: 0.75 }}>{f.desc}</Typography>
                </Box>
              </Box>
            ))}
          </Stack>
        </Box>

        {/* Right — login card */}
        <Card sx={{ width: 380, borderRadius: 3, p: 1 }}>
          <CardContent>
            <Typography variant="h5" fontWeight={700} mb={0.5} color="primary.main">
              Sign In
            </Typography>
            <Typography variant="body2" color="text.secondary" mb={3}>
              Authenticate with your enterprise credentials via Auth0 SSO.
            </Typography>

            <Chip label="Secured by Auth0" size="small" color="primary" variant="outlined"
              icon={<SecurityIcon />} sx={{ mb: 3 }} />

            <Button
              fullWidth
              variant="contained"
              size="large"
              startIcon={<LoginIcon />}
              onClick={() => loginWithRedirect({ appState: { returnTo: `${location.pathname}${location.search}` } })}
              sx={{
                py: 1.5,
                fontSize: 15,
                background: 'linear-gradient(135deg, #1a237e, #0288d1)',
                '&:hover': { background: 'linear-gradient(135deg, #283593, #0277bd)' },
              }}
            >
              Sign in with Auth0 SSO
            </Button>

            <Divider sx={{ my: 2 }}>
              <Typography variant="caption" color="text.secondary">or</Typography>
            </Divider>

            <Button
              fullWidth
              variant="outlined"
              size="large"
              onClick={() => loginWithRedirect({
                authorizationParams: { screen_hint: 'signup' },
                appState: { returnTo: `${location.pathname}${location.search}` },
              })}
              sx={{ py: 1.5, fontSize: 14 }}
            >
              Create Account
            </Button>

            <Typography variant="caption" color="text.secondary" display="block" textAlign="center" mt={3}>
              By signing in you agree to the enterprise usage policy.
              All activity is logged and monitored.
            </Typography>
          </CardContent>
        </Card>

      </Box>
    </Box>
  );
}
