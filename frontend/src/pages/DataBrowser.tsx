import React, { useEffect, useState, useCallback } from 'react';
import {
  Box, Tabs, Tab, Typography, TextField, InputAdornment,
  CircularProgress, Alert, Chip, Select, MenuItem, FormControl,
  InputLabel, Grid, Card, CardContent, Avatar, Pagination,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Paper,
} from '@mui/material';
import { Search as SearchIcon, Person, Business, AttachMoney } from '@mui/icons-material';
import { dataApi } from '../services/api';

function TabPanel({ children, value, index }: any) {
  return value === index ? <Box pt={2}>{children}</Box> : null;
}

// ─── Employees Tab ──────────────────────────────────────────────────────────

function EmployeesTab() {
  const [result, setResult] = useState<any>({ data: [], pagination: { total: 0, pages: 1 } });
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [deptFilter, setDeptFilter] = useState('');
  const [genderFilter, setGenderFilter] = useState('');
  const [departments, setDepartments] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    dataApi.getDepartments().then((r: any) => setDepartments(r.data || []));
  }, []);

  const load = useCallback(() => {
    setLoading(true);
    const params: any = { page, limit: 12 };
    if (search) params.search = search;
    if (deptFilter) params.dept = deptFilter;
    if (genderFilter) params.gender = genderFilter;
    dataApi.getEmployees(params).then(setResult).finally(() => setLoading(false));
  }, [page, search, deptFilter, genderFilter]);

  useEffect(() => { load(); }, [load]);

  const employees = result.data ?? [];
  const pagination = result.pagination ?? { total: 0, pages: 1 };

  return (
    <Box>
      <Grid container spacing={2} mb={2}>
        <Grid item xs={12} sm={5}>
          <TextField fullWidth size="small" placeholder="Search by name or title…"
            value={search} onChange={e => { setSearch(e.target.value); setPage(1); }}
            InputProps={{ startAdornment: <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment> }} />
        </Grid>
        <Grid item xs={12} sm={3}>
          <FormControl fullWidth size="small">
            <InputLabel>Department</InputLabel>
            <Select value={deptFilter} label="Department" onChange={e => { setDeptFilter(e.target.value); setPage(1); }}>
              <MenuItem value="">All</MenuItem>
              {departments.map((d: any) => <MenuItem key={d.dept_no} value={d.dept_no}>{d.dept_name}</MenuItem>)}
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={12} sm={2}>
          <FormControl fullWidth size="small">
            <InputLabel>Gender</InputLabel>
            <Select value={genderFilter} label="Gender" onChange={e => { setGenderFilter(e.target.value); setPage(1); }}>
              <MenuItem value="">All</MenuItem>
              <MenuItem value="M">Male</MenuItem>
              <MenuItem value="F">Female</MenuItem>
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={12} sm={2} display="flex" alignItems="center">
          <Chip label={`${pagination.total} total`} size="small" color="primary" variant="outlined" />
        </Grid>
      </Grid>

      {loading ? <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box> : (
        <>
          <Grid container spacing={2}>
            {employees.map((emp: any) => (
              <Grid item xs={12} sm={6} md={4} key={emp.emp_no}>
                <Card sx={{ '&:hover': { boxShadow: 4 }, transition: 'box-shadow 0.2s' }}>
                  <CardContent>
                    <Box display="flex" alignItems="center" gap={1.5} mb={1}>
                      <Avatar sx={{ bgcolor: emp.gender === 'M' ? '#1a237e' : '#e91e63', width: 38, height: 38 }}>
                        <Person />
                      </Avatar>
                      <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="body1" fontWeight={600} noWrap>
                          {emp.first_name} {emp.last_name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          #{emp.emp_no} · {emp.gender === 'M' ? 'Male' : 'Female'}
                        </Typography>
                      </Box>
                    </Box>
                    <Box display="flex" flexWrap="wrap" gap={0.5}>
                      {emp.title && <Chip label={emp.title} size="small" color="primary" variant="outlined" />}
                      {emp.department && <Chip label={emp.department} size="small" />}
                      {emp.salary > 0 && (
                        <Chip label={`$${emp.salary.toLocaleString()}`} size="small"
                          sx={{ bgcolor: '#e8f5e9', color: '#2e7d32' }} />
                      )}
                    </Box>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                      Hired: {emp.hire_date}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            ))}
          </Grid>
          {pagination.pages > 1 && (
            <Box display="flex" justifyContent="center" mt={3}>
              <Pagination count={pagination.pages} page={page} onChange={(_, v) => setPage(v)} color="primary" />
            </Box>
          )}
        </>
      )}
    </Box>
  );
}

// ─── Departments Tab ─────────────────────────────────────────────────────────

function DepartmentsTab() {
  const [departments, setDepartments] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    dataApi.getDepartments()
      .then((r: any) => setDepartments(r.data || []))
      .catch((e: any) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box>;
  if (error) return <Alert severity="error">{error}</Alert>;

  return (
    <Grid container spacing={2}>
      {departments.map((d: any) => (
        <Grid item xs={12} sm={6} md={4} key={d.dept_no}>
          <Card sx={{ '&:hover': { boxShadow: 4 }, transition: 'box-shadow 0.2s' }}>
            <CardContent>
              <Box display="flex" alignItems="center" gap={1.5} mb={1}>
                <Avatar sx={{ bgcolor: '#0288d1' }}>
                  <Business />
                </Avatar>
                <Box>
                  <Typography variant="body1" fontWeight={600}>{d.dept_name}</Typography>
                  <Typography variant="caption" color="text.secondary">Code: {d.dept_no}</Typography>
                </Box>
              </Box>
              <Box display="flex" gap={1} flexWrap="wrap">
                <Chip label={`${d.employee_count} employees`} size="small" />
                {d.current_manager && (
                  <Chip label={`Mgr: ${d.current_manager}`} size="small" color="primary" variant="outlined" />
                )}
              </Box>
            </CardContent>
          </Card>
        </Grid>
      ))}
    </Grid>
  );
}

// ─── Salaries Tab ────────────────────────────────────────────────────────────

function SalariesTab() {
  const [data, setData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    dataApi.getSalaries()
      .then((r: any) => setData(r.data || []))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box>;

  return (
    <TableContainer component={Paper} sx={{ borderRadius: 2 }}>
      <Table size="small">
        <TableHead>
          <TableRow sx={{ bgcolor: '#f5f6fa' }}>
            <TableCell><Typography variant="body2" fontWeight={600}>Department</Typography></TableCell>
            <TableCell align="right"><Typography variant="body2" fontWeight={600}>Avg Salary</Typography></TableCell>
            <TableCell align="right"><Typography variant="body2" fontWeight={600}>Min</Typography></TableCell>
            <TableCell align="right"><Typography variant="body2" fontWeight={600}>Max</Typography></TableCell>
            <TableCell align="right"><Typography variant="body2" fontWeight={600}>Employees</Typography></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {data.map((row: any) => (
            <TableRow key={row.dept_no} hover>
              <TableCell>
                <Box display="flex" alignItems="center" gap={1}>
                  <AttachMoney fontSize="small" color="action" />
                  {row.department}
                </Box>
              </TableCell>
              <TableCell align="right">
                <Chip label={`$${Math.round(row.avg_salary).toLocaleString()}`}
                  size="small" sx={{ bgcolor: '#e8f5e9', color: '#2e7d32', fontWeight: 700 }} />
              </TableCell>
              <TableCell align="right">${row.min_salary.toLocaleString()}</TableCell>
              <TableCell align="right">${row.max_salary.toLocaleString()}</TableCell>
              <TableCell align="right">{row.employee_count}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

// ─── Main Component ──────────────────────────────────────────────────────────

export default function DataBrowser() {
  const [tab, setTab] = useState(0);
  const [searchGlobal, setSearchGlobal] = useState('');
  const [globalResults, setGlobalResults] = useState<any[]>([]);
  const [searching, setSearching] = useState(false);

  const runGlobalSearch = () => {
    if (!searchGlobal.trim()) return;
    setSearching(true);
    dataApi.search(searchGlobal)
      .then((r: any) => setGlobalResults(r.data || []))
      .finally(() => setSearching(false));
  };

  return (
    <Box>
      <Typography variant="h5" fontWeight={700} mb={0.5}>Data Browser</Typography>
      <Typography variant="body2" color="text.secondary" mb={2}>
        Browse enterprise employee data from datacharmer/test_db
      </Typography>

      {/* Global Search */}
      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Box display="flex" gap={1}>
            <TextField
              fullWidth size="small"
              placeholder="Search across all employees, departments, titles…"
              value={searchGlobal}
              onChange={e => setSearchGlobal(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && runGlobalSearch()}
              InputProps={{ startAdornment: <InputAdornment position="start"><SearchIcon /></InputAdornment> }}
            />
          </Box>
          {searching && <CircularProgress size={20} sx={{ mt: 1 }} />}
          {globalResults.length > 0 && (
            <Box mt={1.5} display="flex" flexWrap="wrap" gap={1}>
              {globalResults.slice(0, 10).map((e: any) => (
                <Chip key={e.emp_no} size="small"
                  label={`${e.first_name} ${e.last_name} · ${e.title || '—'} · ${e.department || '—'}`}
                  color="primary" variant="outlined" />
              ))}
              {globalResults.length > 10 && (
                <Chip size="small" label={`+${globalResults.length - 10} more`} />
              )}
            </Box>
          )}
        </CardContent>
      </Card>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 1 }}>
        <Tab label="Employees" />
        <Tab label="Departments" />
        <Tab label="Salaries" />
      </Tabs>

      <TabPanel value={tab} index={0}><EmployeesTab /></TabPanel>
      <TabPanel value={tab} index={1}><DepartmentsTab /></TabPanel>
      <TabPanel value={tab} index={2}><SalariesTab /></TabPanel>
    </Box>
  );
}
