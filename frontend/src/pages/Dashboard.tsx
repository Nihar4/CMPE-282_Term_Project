import React, { useEffect, useState } from 'react';
import {
  Box, Grid, Card, CardContent, Typography, CircularProgress,
  Divider, List, ListItem, ListItemText, Avatar, Alert, Chip, Button,
} from '@mui/material';
import {
  People, AttachMoney, BusinessCenter, TrendingUp,
} from '@mui/icons-material';
import { analyticsApi, dataApi } from '../services/api';

interface StatCardProps {
  label: string;
  value: string | number;
  icon: React.ReactNode;
  color: string;
  sub?: string;
}

function StatCard({ label, value, icon, color, sub }: StatCardProps) {
  return (
    <Card>
      <CardContent>
        <Box display="flex" justifyContent="space-between" alignItems="flex-start">
          <Box>
            <Typography variant="body2" color="text.secondary" gutterBottom>{label}</Typography>
            <Typography variant="h4" fontWeight={700}>{value}</Typography>
            {sub && <Typography variant="caption" color="text.secondary">{sub}</Typography>}
          </Box>
          <Box sx={{ p: 1.2, bgcolor: color + '20', borderRadius: 2 }}>
            <Box sx={{ color }}>{icon}</Box>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
}

export default function Dashboard() {
  const [overview, setOverview] = useState<any>(null);
  const [dash, setDash] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;

    const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

    const loadDashboard = async () => {
      setLoading(true);
      setError('');

      let lastError: any = null;

      for (let attempt = 1; attempt <= 3; attempt += 1) {
        try {
          const [ov, d] = await Promise.all([dataApi.getOverview(), analyticsApi.dashboard()]);
          if (cancelled) return;
          setOverview(ov.data ?? ov);
          setDash(d);
          setLoading(false);
          return;
        } catch (e: any) {
          lastError = e;
          const isNetworkError = e?.message === 'Network Error';
          if (attempt < 3 && isNetworkError) {
            await sleep(600 * attempt);
            continue;
          }
          break;
        }
      }

      if (!cancelled) {
        setError(lastError?.message || 'Failed to load dashboard');
        setLoading(false);
      }
    };

    loadDashboard();

    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  if (loading) return (
    <Box display="flex" justifyContent="center" alignItems="center" minHeight={300}>
      <CircularProgress />
    </Box>
  );

  if (error) {
    return (
      <Alert
        severity="warning"
        action={
          <Button color="inherit" size="small" onClick={() => setReloadKey(k => k + 1)}>
            Retry
          </Button>
        }
      >
        {error} — temporary startup issue while loading dashboard data.
      </Alert>
    );
  }

  const deptData: any[] = dash?.headcount_by_dept ?? [];
  const salaryData: any[] = dash?.salary_by_dept ?? [];
  const titleData: any[] = dash?.title_dist ?? [];
  const genderData: any[] = dash?.gender_dist ?? [];

  return (
    <Box>
      <Typography variant="h5" fontWeight={700} mb={0.5}>Dashboard Overview</Typography>
      <Typography variant="body2" color="text.secondary" mb={3}>
        Enterprise workforce analytics — powered by test_db (datacharmer/test_db)
      </Typography>

      {/* KPI Cards */}
      <Grid container spacing={2} mb={3}>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard label="Total Employees" value={(overview?.total_employees ?? 0).toLocaleString()}
            icon={<People />} color="#1a237e"
            sub={`${overview?.male_count ?? 0}M / ${overview?.female_count ?? 0}F`} />
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard label="Avg Salary (Current)" value={`$${(overview?.avg_salary ?? 0).toLocaleString()}`}
            icon={<AttachMoney />} color="#2e7d32"
            sub={`Max: $${(overview?.max_salary ?? 0).toLocaleString()}`} />
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard label="Departments" value={overview?.total_departments ?? 0}
            icon={<BusinessCenter />} color="#0288d1" />
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard label="Most Common Title" value={overview?.most_common_title ?? '—'}
            icon={<TrendingUp />} color="#f57c00" />
        </Grid>
      </Grid>

      <Grid container spacing={2}>
        {/* Department Headcount */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" fontWeight={600} mb={2}>Headcount by Department</Typography>
              <List dense disablePadding>
                {deptData.map((d: any) => {
                  const maxCount = Math.max(...deptData.map((x: any) => x.count));
                  return (
                    <React.Fragment key={d.department}>
                      <ListItem disablePadding sx={{ py: 0.75 }}>
                        <ListItemText
                          primary={d.department}
                          primaryTypographyProps={{ variant: 'body2' }}
                        />
                        <Box display="flex" alignItems="center" gap={1}>
                          <Box sx={{
                            width: 80, height: 6, bgcolor: '#e8eaf6', borderRadius: 3, overflow: 'hidden',
                          }}>
                            <Box sx={{
                              height: '100%', borderRadius: 3,
                              background: 'linear-gradient(90deg, #1a237e, #0288d1)',
                              width: `${(d.count / maxCount) * 100}%`,
                            }} />
                          </Box>
                          <Typography variant="body2" fontWeight={700} sx={{ minWidth: 28 }}>
                            {d.count}
                          </Typography>
                        </Box>
                      </ListItem>
                      <Divider />
                    </React.Fragment>
                  );
                })}
              </List>
            </CardContent>
          </Card>
        </Grid>

        {/* Salary by Department */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" fontWeight={600} mb={2}>Avg Salary by Department</Typography>
              <List dense disablePadding>
                {salaryData.map((s: any) => {
                  const max = Math.max(...salaryData.map((x: any) => x.avg_salary));
                  return (
                    <React.Fragment key={s.department}>
                      <ListItem disablePadding sx={{ py: 0.75 }}>
                        <ListItemText
                          primary={s.department}
                          primaryTypographyProps={{ variant: 'body2' }}
                        />
                        <Box display="flex" alignItems="center" gap={1}>
                          <Box sx={{ width: 80, height: 6, bgcolor: '#e8eaf6', borderRadius: 3, overflow: 'hidden' }}>
                            <Box sx={{
                              height: '100%', borderRadius: 3,
                              background: 'linear-gradient(90deg, #2e7d32, #66bb6a)',
                              width: `${(s.avg_salary / max) * 100}%`,
                            }} />
                          </Box>
                          <Typography variant="body2" fontWeight={700} sx={{ minWidth: 60 }}>
                            ${Math.round(s.avg_salary).toLocaleString()}
                          </Typography>
                        </Box>
                      </ListItem>
                      <Divider />
                    </React.Fragment>
                  );
                })}
              </List>
            </CardContent>
          </Card>
        </Grid>

        {/* Title Distribution */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" fontWeight={600} mb={2}>Title Distribution</Typography>
              <Box display="flex" flexWrap="wrap" gap={1}>
                {titleData.map((t: any) => (
                  <Chip key={t.title} label={`${t.title} (${t.count})`}
                    color="primary" variant="outlined" size="small" />
                ))}
              </Box>
            </CardContent>
          </Card>
        </Grid>

        {/* Gender Distribution */}
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" fontWeight={600} mb={2}>Gender Distribution</Typography>
              {genderData.map((g: any) => {
                const total = genderData.reduce((s: number, x: any) => s + x.count, 0);
                const pct = total ? Math.round((g.count / total) * 100) : 0;
                return (
                  <Box key={g.gender} mb={2}>
                    <Box display="flex" justifyContent="space-between" mb={0.5}>
                      <Typography variant="body2">{g.gender === 'M' ? 'Male' : 'Female'}</Typography>
                      <Typography variant="body2" fontWeight={600}>{g.count} ({pct}%)</Typography>
                    </Box>
                    <Box sx={{ height: 10, bgcolor: '#e8eaf6', borderRadius: 5, overflow: 'hidden' }}>
                      <Box sx={{
                        height: '100%', borderRadius: 5,
                        bgcolor: g.gender === 'M' ? '#1a237e' : '#e91e63',
                        width: `${pct}%`, transition: 'width 0.5s',
                      }} />
                    </Box>
                  </Box>
                );
              })}
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
}
