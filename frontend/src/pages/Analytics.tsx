import React, { useEffect, useState } from 'react';
import {
  Box, Grid, Card, CardContent, Typography, CircularProgress,
  Alert, Tabs, Tab, Chip,
  Table, TableBody, TableCell, TableHead, TableRow, TableContainer, Paper, Avatar,
} from '@mui/material';
import { Person, AttachMoney } from '@mui/icons-material';
import { analyticsApi } from '../services/api';

function TabPanel({ children, value, index }: any) {
  return value === index ? <Box pt={2}>{children}</Box> : null;
}

function BarChart({ data, labelKey, valueKey, color = '#1a237e', prefix = '', suffix = '' }: {
  data: any[]; labelKey: string; valueKey: string;
  color?: string; prefix?: string; suffix?: string;
}) {
  if (!data?.length) return <Typography variant="body2" color="text.secondary">No data</Typography>;
  const max = Math.max(...data.map(d => d[valueKey] || 0));
  return (
    <Box>
      {data.map((row: any, i: number) => (
        <Box key={i} mb={1.5}>
          <Box display="flex" justifyContent="space-between" mb={0.4}>
            <Typography variant="body2" color="text.secondary">{row[labelKey]}</Typography>
            <Typography variant="body2" fontWeight={600}>
              {prefix}{typeof row[valueKey] === 'number' ? row[valueKey].toLocaleString() : row[valueKey]}{suffix}
            </Typography>
          </Box>
          <Box sx={{ height: 8, bgcolor: '#e8eaf6', borderRadius: 4, overflow: 'hidden' }}>
            <Box sx={{
              height: '100%', borderRadius: 4, bgcolor: color,
              width: `${max ? (row[valueKey] / max) * 100 : 0}%`, transition: 'width 0.5s',
            }} />
          </Box>
        </Box>
      ))}
    </Box>
  );
}

export default function Analytics() {
  const [tab, setTab] = useState(0);
  const [dash, setDash] = useState<any>(null);
  const [salaryDist, setSalaryDist] = useState<any[]>([]);
  const [tenure, setTenure] = useState<any[]>([]);
  const [topEarners, setTopEarners] = useState<any[]>([]);
  const [salaryByGender, setSalaryByGender] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    Promise.all([
      analyticsApi.dashboard(),
      analyticsApi.salaryDistribution(),
      analyticsApi.tenure(),
      analyticsApi.topEarners(),
      analyticsApi.salaryByGender(),
    ])
      .then(([d, s, t, te, sg]) => {
        setDash(d);
        setSalaryDist(s.data || []);
        setTenure(t.data || []);
        setTopEarners(te.data || []);
        setSalaryByGender(sg.data || []);
      })
      .catch(e => setError(e.message || 'Failed to load analytics'))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <Box display="flex" justifyContent="center" alignItems="center" minHeight={300}><CircularProgress /></Box>;
  if (error) return <Alert severity="warning">{error}</Alert>;

  const headcount: any[] = dash?.headcount_by_dept ?? [];
  const salaryByDept: any[] = dash?.salary_by_dept ?? [];
  const titleDist: any[] = dash?.title_dist ?? [];
  const stats: any = dash?.stats ?? {};

  return (
    <Box>
      <Typography variant="h5" fontWeight={700} mb={0.5}>Analytics</Typography>
      <Typography variant="body2" color="text.secondary" mb={3}>
        Workforce analytics from the datacharmer/test_db dataset
      </Typography>

      {/* Summary cards */}
      <Grid container spacing={2} mb={3}>
        {[
          { label: 'Total Employees', value: (stats.total_employees ?? 0).toLocaleString(), color: '#1a237e' },
          { label: 'Average Salary',  value: `$${Math.round(stats.avg_salary ?? 0).toLocaleString()}`, color: '#2e7d32' },
          { label: 'Highest Salary',  value: `$${(stats.max_salary ?? 0).toLocaleString()}`, color: '#f57c00' },
          { label: 'Departments',     value: headcount.length, color: '#0288d1' },
        ].map(c => (
          <Grid item xs={6} md={3} key={c.label}>
            <Card>
              <CardContent sx={{ textAlign: 'center', py: 2 }}>
                <Typography variant="h4" fontWeight={700} sx={{ color: c.color }}>{c.value}</Typography>
                <Typography variant="caption" color="text.secondary">{c.label}</Typography>
              </CardContent>
            </Card>
          </Grid>
        ))}
      </Grid>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 1 }} variant="scrollable" scrollButtons="auto">
        <Tab label="Headcount" />
        <Tab label="Salaries" />
        <Tab label="Titles" />
        <Tab label="Salary Buckets" />
        <Tab label="Tenure" />
        <Tab label="Top Earners" />
        <Tab label="Salary by Gender" />
      </Tabs>

      {/* Headcount */}
      <TabPanel value={tab} index={0}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={2}>Headcount by Department</Typography>
          <BarChart data={headcount} labelKey="department" valueKey="count" color="#1a237e" />
        </CardContent></Card>
      </TabPanel>

      {/* Salaries */}
      <TabPanel value={tab} index={1}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={2}>Average Salary by Department</Typography>
          <BarChart data={salaryByDept} labelKey="department" valueKey="avg_salary" color="#2e7d32" prefix="$" />
        </CardContent></Card>
      </TabPanel>

      {/* Titles */}
      <TabPanel value={tab} index={2}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={2}>Title Distribution (Current)</Typography>
          <TableContainer component={Paper} elevation={0}>
            <Table size="small">
              <TableHead>
                <TableRow sx={{ bgcolor: '#f5f6fa' }}>
                  <TableCell>Title</TableCell>
                  <TableCell align="right">Count</TableCell>
                  <TableCell>Distribution</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {titleDist.map((t: any) => {
                  const total = titleDist.reduce((s: number, x: any) => s + x.count, 0);
                  const pct = total ? Math.round((t.count / total) * 100) : 0;
                  return (
                    <TableRow key={t.title} hover>
                      <TableCell><Chip label={t.title} size="small" color="primary" variant="outlined" /></TableCell>
                      <TableCell align="right"><strong>{t.count}</strong></TableCell>
                      <TableCell>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Box sx={{ flex: 1, height: 6, bgcolor: '#e8eaf6', borderRadius: 3, overflow: 'hidden' }}>
                            <Box sx={{ height: '100%', bgcolor: '#1a237e', width: `${pct}%`, borderRadius: 3 }} />
                          </Box>
                          <Typography variant="caption">{pct}%</Typography>
                        </Box>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent></Card>
      </TabPanel>

      {/* Salary Buckets */}
      <TabPanel value={tab} index={3}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={2}>Salary Distribution Buckets</Typography>
          <BarChart data={salaryDist} labelKey="bucket" valueKey="count" color="#9c27b0" />
        </CardContent></Card>
      </TabPanel>

      {/* Tenure */}
      <TabPanel value={tab} index={4}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={2}>Employee Tenure</Typography>
          <BarChart data={tenure} labelKey="years_range" valueKey="count" color="#0288d1" />
        </CardContent></Card>
      </TabPanel>

      {/* Top Earners */}
      <TabPanel value={tab} index={5}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={2}>Top 10 Highest Paid Employees</Typography>
          <TableContainer component={Paper} elevation={0}>
            <Table size="small">
              <TableHead>
                <TableRow sx={{ bgcolor: '#f5f6fa' }}>
                  <TableCell>Rank</TableCell>
                  <TableCell>Employee</TableCell>
                  <TableCell>Department</TableCell>
                  <TableCell>Title</TableCell>
                  <TableCell align="right">Salary</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {topEarners.map((e: any, i: number) => (
                  <TableRow key={e.emp_no} hover>
                    <TableCell>
                      <Chip label={`#${i + 1}`} size="small"
                        sx={{ bgcolor: i === 0 ? '#ffd700' : i === 1 ? '#c0c0c0' : i === 2 ? '#cd7f32' : '#f5f6fa', fontWeight: 700, fontSize: 11 }} />
                    </TableCell>
                    <TableCell>
                      <Box display="flex" alignItems="center" gap={1}>
                        <Avatar sx={{ width: 28, height: 28, bgcolor: '#1a237e', fontSize: 12 }}>
                          <Person sx={{ fontSize: 14 }} />
                        </Avatar>
                        <Box>
                          <Typography variant="body2" fontWeight={600}>{e.first_name} {e.last_name}</Typography>
                          <Typography variant="caption" color="text.secondary">#{e.emp_no}</Typography>
                        </Box>
                      </Box>
                    </TableCell>
                    <TableCell><Chip label={e.department} size="small" /></TableCell>
                    <TableCell><Chip label={e.title} size="small" color="primary" variant="outlined" /></TableCell>
                    <TableCell align="right">
                      <Chip
                        label={`$${e.salary.toLocaleString()}`}
                        size="small"
                        icon={<AttachMoney sx={{ fontSize: '14px !important' }} />}
                        sx={{ bgcolor: '#e8f5e9', color: '#2e7d32', fontWeight: 700 }}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent></Card>
      </TabPanel>

      {/* Salary by Gender */}
      <TabPanel value={tab} index={6}>
        <Card><CardContent>
          <Typography variant="h6" fontWeight={600} mb={3}>Salary Analysis by Gender</Typography>
          <Grid container spacing={2}>
            {salaryByGender.map((g: any) => {
              const label = g.gender === 'M' ? 'Male' : 'Female';
              const color = g.gender === 'M' ? '#1a237e' : '#e91e63';
              return (
                <Grid item xs={12} sm={6} key={g.gender}>
                  <Card variant="outlined" sx={{ borderColor: color + '40' }}>
                    <CardContent>
                      <Box display="flex" alignItems="center" gap={1} mb={2}>
                        <Avatar sx={{ bgcolor: color, width: 36, height: 36 }}>
                          <Person />
                        </Avatar>
                        <Box>
                          <Typography variant="h6" fontWeight={700}>{label}</Typography>
                          <Typography variant="caption" color="text.secondary">{g.count} employees</Typography>
                        </Box>
                      </Box>
                      {[
                        { label: 'Avg Salary', value: `$${Math.round(g.avg_salary).toLocaleString()}` },
                        { label: 'Max Salary', value: `$${g.max_salary.toLocaleString()}` },
                        { label: 'Min Salary', value: `$${g.min_salary.toLocaleString()}` },
                      ].map(row => (
                        <Box key={row.label} display="flex" justifyContent="space-between" mb={0.5}>
                          <Typography variant="body2" color="text.secondary">{row.label}</Typography>
                          <Typography variant="body2" fontWeight={600}>{row.value}</Typography>
                        </Box>
                      ))}
                    </CardContent>
                  </Card>
                </Grid>
              );
            })}
          </Grid>
        </CardContent></Card>
      </TabPanel>
    </Box>
  );
}
