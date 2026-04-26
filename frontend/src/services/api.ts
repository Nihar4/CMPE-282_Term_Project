import axios from 'axios';

const API_BASE = process.env.REACT_APP_API_URL || 'http://localhost:8080';
const PORTAL_TOKEN_KEY = 'portal_auth_token';

function canUseStorage() {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

export function getSavedAuthToken() {
  if (!canUseStorage()) return null;
  return window.localStorage.getItem(PORTAL_TOKEN_KEY);
}

export const api = axios.create({
  baseURL: API_BASE,
  withCredentials: true,
  timeout: 60000,
});

// Ensure first-page requests after reload still include a persisted token.
api.interceptors.request.use((config) => {
  const hasAuthHeader = Boolean((config.headers as any)?.Authorization);
  if (!hasAuthHeader) {
    const savedToken = getSavedAuthToken();
    if (savedToken) {
      config.headers = config.headers ?? {};
      (config.headers as any).Authorization = `Bearer ${savedToken}`;
    }
  }
  return config;
});

// Attach JWT token to every request
export function setAuthToken(token: string | null) {
  if (token) {
    api.defaults.headers.common['Authorization'] = `Bearer ${token}`;
    if (canUseStorage()) {
      window.localStorage.setItem(PORTAL_TOKEN_KEY, token);
    }
  } else {
    delete api.defaults.headers.common['Authorization'];
    if (canUseStorage()) {
      window.localStorage.removeItem(PORTAL_TOKEN_KEY);
    }
  }
}

// ─── Dev Login (when not using Auth0) ────────────────────────────────────────

export async function devLogin(email: string, password: string) {
  const res = await api.post('/api/auth/dev-login', { email, password });
  return res.data;
}

// ─── Auth0 Exchange ───────────────────────────────────────────────────────────

export async function auth0Exchange(auth0Token: string) {
  const res = await api.post('/api/auth/auth0-exchange', { access_token: auth0Token });
  return res.data;
}

// ─── Current User ─────────────────────────────────────────────────────────────

export async function getMe() {
  const res = await api.get('/api/auth/me');
  return res.data;
}

// ─── Data Service (test_db schema) ────────────────────────────────────────────

export const dataApi = {
  getOverview:    ()                      => api.get('/api/data/overview').then(r => r.data),
  search:         (q: string)             => api.get('/api/data/search', { params: { q } }).then(r => r.data),

  getDepartments: ()                      => api.get('/api/data/departments').then(r => r.data),

  getEmployees:   (p?: object)            => api.get('/api/data/employees', { params: p }).then(r => r.data),
  getEmployee:    (id: string | number)   => api.get(`/api/data/employees/${id}`).then(r => r.data),

  getSalaries:    (p?: object)            => api.get('/api/data/salaries', { params: p }).then(r => r.data),
  getTitles:      ()                      => api.get('/api/data/titles').then(r => r.data),
};

// ─── File Service ─────────────────────────────────────────────────────────────

export const fileApi = {
  upload: (file: File, onProgress?: (pct: number) => void) => {
    const form = new FormData();
    form.append('file', file);
    return api.post('/api/files/upload', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: e => {
        if (onProgress && e.total) onProgress(Math.round((e.loaded / e.total) * 100));
      },
    }).then(r => r.data);
  },
  list:       ()             => api.get('/api/files/').then(r => r.data),
  get:        (id: string)  => api.get(`/api/files/${id}`).then(r => r.data),
  getChunks:  (id: string)  => api.get(`/api/files/${id}/chunks`).then(r => r.data),
  search:     (q: string, fileId?: string) =>
    api.get('/api/files/search', { params: { q, file_id: fileId } }).then(r => r.data),
  delete:     (id: string)  => api.delete(`/api/files/${id}`).then(r => r.data),
  deleteAll:  ()            => api.delete('/api/files/').then(r => r.data),
};

// ─── AI Service ───────────────────────────────────────────────────────────────

export const aiApi = {
  query: (question: string, fileIds?: string[], mode?: string) =>
    api.post('/api/ai/query', { question, file_ids: fileIds, mode }).then(r => r.data),

  history: () => api.get('/api/ai/history').then(r => r.data),
  schema:  () => api.get('/api/ai/schema').then(r => r.data),

  // Streaming — returns EventSource-compatible URL and payload
  streamUrl: () => `${API_BASE}/api/ai/stream`,
};

// ─── Notifications ───────────────────────────────────────────────────────────

export const notificationsApi = {
  list: () => api.get('/api/data/notifications').then(r => r.data),
};

// ─── Analytics Service (test_db analytics) ────────────────────────────────────

export const analyticsApi = {
  dashboard:          () => api.get('/api/analytics/dashboard').then(r => r.data),
  salaryDistribution: () => api.get('/api/analytics/salary-distribution').then(r => r.data),
  tenure:             () => api.get('/api/analytics/tenure').then(r => r.data),
  topEarners:         () => api.get('/api/analytics/top-earners').then(r => r.data),
  salaryByGender:     () => api.get('/api/analytics/salary-by-gender').then(r => r.data),
  reports:            () => api.get('/api/analytics/reports').then(r => r.data),
};
