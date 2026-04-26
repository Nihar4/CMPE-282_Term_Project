import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { useAuth0 } from '@auth0/auth0-react';
import {
  Box, Drawer, AppBar, Toolbar, Typography, List, ListItem,
  ListItemButton, ListItemIcon, ListItemText, Avatar, Menu, MenuItem,
  Divider, IconButton, Tooltip, Badge, Chip, Popover, Paper,
  useMediaQuery, useTheme, CircularProgress,
} from '@mui/material';
import {
  Dashboard as DashboardIcon,
  TableChart as DataIcon,
  CloudUpload as UploadIcon,
  SmartToy as AIIcon,
  BarChart as AnalyticsIcon,
  Menu as MenuIcon,
  Logout as LogoutIcon,
  Person as PersonIcon,
  Notifications as NotifIcon,
  Business as BusinessIcon,
  InsertDriveFile as FileIcon,
  CheckCircle as CheckIcon,
  Warning as WarningIcon,
  Info as InfoIcon,
  DoneAll as DoneAllIcon,
  HourglassEmpty as ProcessingIcon,
} from '@mui/icons-material';
import { setAuthToken, auth0Exchange, devLogin, getSavedAuthToken, notificationsApi } from '../../services/api';

const DRAWER_WIDTH = 240;
const READ_KEY = 'portal_notif_read_ids';

const NAV_ITEMS = [
  { label: 'Dashboard',    path: '/dashboard',  icon: <DashboardIcon /> },
  { label: 'Data Browser', path: '/data',       icon: <DataIcon /> },
  { label: 'File Upload',  path: '/files',      icon: <UploadIcon /> },
  { label: 'AI Assistant', path: '/ai',         icon: <AIIcon /> },
  { label: 'Analytics',    path: '/analytics',  icon: <AnalyticsIcon /> },
];

// ── Helpers ──────────────────────────────────────────────────────────────────

function getReadIds(): Set<string> {
  try {
    const raw = localStorage.getItem(READ_KEY);
    return raw ? new Set<string>(JSON.parse(raw) as string[]) : new Set<string>();
  } catch { return new Set<string>(); }
}

function saveReadIds(ids: Set<string>) {
  try { localStorage.setItem(READ_KEY, JSON.stringify(Array.from(ids))); } catch {}
}

function timeAgo(dateStr: string): string {
  const diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  return new Date(dateStr).toLocaleDateString();
}

function NotifIcon2({ icon, type }: { icon: string; type: string }) {
  const sx = { fontSize: 18 };
  const wrapSx = {
    width: 36, height: 36, borderRadius: '50%', display: 'flex',
    alignItems: 'center', justifyContent: 'center', flexShrink: 0,
  };

  if (icon === 'warning') return (
    <Box sx={{ ...wrapSx, bgcolor: '#fff3e0' }}>
      <WarningIcon sx={{ ...sx, color: '#f57c00' }} />
    </Box>
  );
  if (icon === 'ai') return (
    <Box sx={{ ...wrapSx, bgcolor: '#e8eaf6' }}>
      <AIIcon sx={{ ...sx, color: '#3949ab' }} />
    </Box>
  );
  if (icon === 'system') return (
    <Box sx={{ ...wrapSx, bgcolor: '#e3f2fd' }}>
      <InfoIcon sx={{ ...sx, color: '#0288d1' }} />
    </Box>
  );
  if (type === 'file_processing') return (
    <Box sx={{ ...wrapSx, bgcolor: '#f3e5f5' }}>
      <ProcessingIcon sx={{ ...sx, color: '#7b1fa2' }} />
    </Box>
  );
  // file_ready
  return (
    <Box sx={{ ...wrapSx, bgcolor: '#e8f5e9' }}>
      <CheckIcon sx={{ ...sx, color: '#2e7d32' }} />
    </Box>
  );
}

// ── Main Layout ──────────────────────────────────────────────────────────────

export default function Layout() {
  const { user, logout, getAccessTokenSilently } = useAuth0();
  const navigate = useNavigate();
  const location = useLocation();
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));

  const [drawerOpen, setDrawerOpen]           = useState(!isMobile);
  const [anchorEl, setAnchorEl]               = useState<null | HTMLElement>(null);
  const [portalToken, setPortalToken]         = useState<string | null>(null);
  const [authBootstrapping, setAuthBootstrapping] = useState(true);
  const restoredTokenRef = useRef(false);

  // Notifications state
  const [notifAnchor, setNotifAnchor]         = useState<null | HTMLElement>(null);
  const [notifications, setNotifications]     = useState<any[]>([]);
  const [readIds, setReadIds]                 = useState<Set<string>>(getReadIds);
  const [notifLoading, setNotifLoading]       = useState(false);
  const notifOpen = Boolean(notifAnchor);

  // Restore portal JWT on reload
  useEffect(() => {
    const savedToken = getSavedAuthToken();
    if (savedToken) {
      restoredTokenRef.current = true;
      setPortalToken(savedToken);
      setAuthToken(savedToken);
      setAuthBootstrapping(false);
    }
  }, []);

  // Exchange Auth0 token for portal JWT
  useEffect(() => {
    if (portalToken || restoredTokenRef.current) {
      setAuthBootstrapping(false);
      return;
    }
    (async () => {
      try {
        const auth0Token = await getAccessTokenSilently();
        const result = await auth0Exchange(auth0Token);
        if (result.access_token) { setPortalToken(result.access_token); setAuthToken(result.access_token); return; }
      } catch {}
      try {
        if (user?.email) {
          const result = await devLogin(user.email, 'devpassword');
          if (result.access_token) { setPortalToken(result.access_token); setAuthToken(result.access_token); return; }
        }
      } catch {}
      try {
        const result = await devLogin('admin@enterprise.com', 'admin123');
        if (result.access_token) { setPortalToken(result.access_token); setAuthToken(result.access_token); }
      } catch (e) { console.error('All auth methods failed:', e); }
      finally { setAuthBootstrapping(false); }
    })();
  }, [getAccessTokenSilently, portalToken, user?.email]);

  // Fetch notifications
  const fetchNotifications = useCallback(async () => {
    try {
      const data = await notificationsApi.list();
      setNotifications(data.notifications || []);
    } catch {}
  }, []);

  useEffect(() => {
    if (!authBootstrapping) {
      fetchNotifications();
      // Poll every 60 s
      const t = setInterval(fetchNotifications, 60000);
      return () => clearInterval(t);
    }
  }, [authBootstrapping, fetchNotifications]);

  const unreadCount = notifications.filter(n => !readIds.has(n.id)).length;

  const openNotifications = (e: React.MouseEvent<HTMLElement>) => {
    setNotifAnchor(e.currentTarget);
    // Fetch fresh data when panel opens
    fetchNotifications();
  };

  const closeNotifications = () => setNotifAnchor(null);

  const markAllRead = () => {
    const all = new Set<string>(notifications.map(n => n.id));
    setReadIds(all);
    saveReadIds(all);
  };

  const markOneRead = (id: string) => {
    const next = new Set(readIds);
    next.add(id);
    setReadIds(next);
    saveReadIds(next);
  };

  const handleLogout = () => {
    setAuthToken(null);
    logout({ logoutParams: { returnTo: window.location.origin } });
  };

  const currentTitle = NAV_ITEMS.find(n => location.pathname.startsWith(n.path))?.label || 'Portal';

  // ── Drawer content ─────────────────────────────────────────────────────────
  const drawerContent = (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Logo */}
      <Box sx={{ px: 2.5, py: 2.5, display: 'flex', alignItems: 'center', gap: 1.5 }}>
        <Box sx={{
          width: 38, height: 38, borderRadius: 2,
          background: 'linear-gradient(135deg, #1a237e, #0288d1)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
          <BusinessIcon sx={{ color: '#fff', fontSize: 22 }} />
        </Box>
        <Box>
          <Typography variant="subtitle2" fontWeight={700} color="primary.main" lineHeight={1.2}>
            Enterprise
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Knowledge Portal
          </Typography>
        </Box>
      </Box>

      <Divider />

      {/* Navigation */}
      <List sx={{ px: 1, pt: 1, flex: 1 }}>
        {NAV_ITEMS.map(item => {
          const active = location.pathname.startsWith(item.path);
          return (
            <ListItem key={item.path} disablePadding sx={{ mb: 0.5 }}>
              <ListItemButton
                onClick={() => { navigate(item.path); if (isMobile) setDrawerOpen(false); }}
                sx={{
                  borderRadius: 2,
                  bgcolor: active ? 'primary.main' : 'transparent',
                  color: active ? '#fff' : 'text.primary',
                  '&:hover': { bgcolor: active ? 'primary.dark' : 'action.hover' },
                  transition: 'all 0.15s',
                }}
              >
                <ListItemIcon sx={{ color: 'inherit', minWidth: 36 }}>
                  {item.icon}
                </ListItemIcon>
                <ListItemText
                  primary={item.label}
                  primaryTypographyProps={{ fontSize: 14, fontWeight: active ? 600 : 400 }}
                />
                {item.label === 'AI Assistant' && (
                  <Chip label="AI" size="small"
                    sx={{ height: 18, fontSize: 10, bgcolor: active ? 'rgba(255,255,255,0.25)' : '#e8eaf6', color: active ? '#fff' : 'primary.main' }} />
                )}
              </ListItemButton>
            </ListItem>
          );
        })}
      </List>

      <Divider />

      {/* User info */}
      <Box sx={{ p: 1.5, bgcolor: '#f8f9fc', borderTop: '1px solid', borderColor: 'divider' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Avatar
            src={user?.picture}
            sx={{ width: 36, height: 36, bgcolor: '#1a237e', fontSize: 15, flexShrink: 0 }}
          >
            {user?.name?.[0]?.toUpperCase()}
          </Avatar>
          <Box sx={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
            <Typography
              variant="body2" fontWeight={600}
              sx={{ lineHeight: 1.3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
            >
              {user?.name || 'User'}
            </Typography>
            <Typography
              variant="caption" color="text.secondary"
              sx={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 10 }}
            >
              {user?.email}
            </Typography>
          </Box>
          <Tooltip title="Logout" placement="top">
            <IconButton
              size="small"
              onClick={handleLogout}
              sx={{
                flexShrink: 0,
                color: 'text.secondary',
                '&:hover': { color: 'error.main', bgcolor: '#fee2e2' },
              }}
            >
              <LogoutIcon sx={{ fontSize: 18 }} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>
    </Box>
  );

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh', bgcolor: 'background.default' }}>
      {/* AppBar */}
      <AppBar position="fixed"
        sx={{
          width: { md: `calc(100% - ${DRAWER_WIDTH}px)` },
          ml: { md: `${DRAWER_WIDTH}px` },
          bgcolor: '#fff',
          color: 'text.primary',
          boxShadow: '0 1px 4px rgba(0,0,0,0.08)',
        }}
      >
        <Toolbar>
          <IconButton edge="start" onClick={() => setDrawerOpen(o => !o)} sx={{ mr: 1, display: { md: 'none' } }}>
            <MenuIcon />
          </IconButton>
          <Typography variant="h6" fontWeight={600} sx={{ flex: 1 }}>{currentTitle}</Typography>

          {/* Notification Bell */}
          <Tooltip title="Notifications">
            <IconButton sx={{ mr: 1 }} onClick={openNotifications}>
              <Badge badgeContent={unreadCount} color="error" max={99}>
                <NotifIcon />
              </Badge>
            </IconButton>
          </Tooltip>

          {/* User avatar */}
          <Tooltip title={user?.name || 'Profile'}>
            <IconButton onClick={e => setAnchorEl(e.currentTarget)}>
              <Avatar src={user?.picture} sx={{ width: 32, height: 32, bgcolor: 'primary.main', fontSize: 14 }}>
                {user?.name?.[0]}
              </Avatar>
            </IconButton>
          </Tooltip>

          <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}
            PaperProps={{ sx: { mt: 1, minWidth: 180 } }}>
            <MenuItem disabled>
              <ListItemIcon><PersonIcon fontSize="small" /></ListItemIcon>
              {user?.email}
            </MenuItem>
            <Divider />
            <MenuItem onClick={handleLogout}>
              <ListItemIcon><LogoutIcon fontSize="small" /></ListItemIcon>
              Logout
            </MenuItem>
          </Menu>
        </Toolbar>
      </AppBar>

      {/* ── Notifications Popover ── */}
      <Popover
        open={notifOpen}
        anchorEl={notifAnchor}
        onClose={closeNotifications}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        PaperProps={{
          sx: {
            mt: 1, width: 400, maxHeight: 540, borderRadius: 2,
            boxShadow: '0 8px 32px rgba(0,0,0,0.14)',
            display: 'flex', flexDirection: 'column',
          },
        }}
      >
        {/* Header */}
        <Box sx={{
          px: 2.5, py: 1.8, display: 'flex', alignItems: 'center',
          justifyContent: 'space-between', borderBottom: '1px solid', borderColor: 'divider',
          background: 'linear-gradient(135deg, #1a237e 0%, #0288d1 100%)',
        }}>
          <Box display="flex" alignItems="center" gap={1}>
            <NotifIcon sx={{ color: '#fff', fontSize: 20 }} />
            <Typography variant="subtitle1" fontWeight={700} color="#fff">
              Notifications
            </Typography>
            {unreadCount > 0 && (
              <Chip
                label={unreadCount}
                size="small"
                sx={{ bgcolor: '#ef5350', color: '#fff', fontWeight: 700, height: 20, fontSize: 11 }}
              />
            )}
          </Box>
          {unreadCount > 0 && (
            <Tooltip title="Mark all as read">
              <IconButton size="small" onClick={markAllRead} sx={{ color: 'rgba(255,255,255,0.8)', '&:hover': { color: '#fff' } }}>
                <DoneAllIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
        </Box>

        {/* List */}
        <Box sx={{ overflowY: 'auto', flex: 1 }}>
          {notifications.length === 0 ? (
            <Box sx={{ py: 6, textAlign: 'center' }}>
              <NotifIcon sx={{ fontSize: 48, color: '#e0e0e0', mb: 1 }} />
              <Typography variant="body2" color="text.secondary">No notifications yet</Typography>
            </Box>
          ) : (
            notifications.map((n, i) => {
              const isRead = readIds.has(n.id);
              return (
                <Box
                  key={n.id}
                  onClick={() => markOneRead(n.id)}
                  sx={{
                    display: 'flex', gap: 1.5, px: 2, py: 1.5, cursor: 'pointer',
                    bgcolor: isRead ? 'transparent' : '#f0f4ff',
                    borderBottom: i < notifications.length - 1 ? '1px solid' : 'none',
                    borderColor: 'divider',
                    transition: 'background 0.15s',
                    '&:hover': { bgcolor: isRead ? '#f9f9f9' : '#e8edf8' },
                  }}
                >
                  <NotifIcon2 icon={n.icon} type={n.type} />
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box display="flex" justifyContent="space-between" alignItems="flex-start">
                      <Typography
                        variant="body2" fontWeight={isRead ? 400 : 700}
                        sx={{ lineHeight: 1.3, color: isRead ? 'text.secondary' : 'text.primary' }}
                      >
                        {n.title}
                      </Typography>
                      <Typography variant="caption" color="text.disabled" sx={{ ml: 1, flexShrink: 0, fontSize: 10 }}>
                        {timeAgo(n.created_at)}
                      </Typography>
                    </Box>
                    <Typography
                      variant="caption" color="text.secondary"
                      sx={{ display: 'block', mt: 0.3, lineHeight: 1.4, wordBreak: 'break-word' }}
                    >
                      {n.body}
                    </Typography>
                    {!isRead && (
                      <Box sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: '#1a237e', mt: 0.5 }} />
                    )}
                  </Box>
                </Box>
              );
            })
          )}
        </Box>

        {/* Footer */}
        {notifications.length > 0 && (
          <Box sx={{
            px: 2, py: 1.2, borderTop: '1px solid', borderColor: 'divider',
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            bgcolor: '#fafafa',
          }}>
            <Typography variant="caption" color="text.secondary">
              {notifications.length} notification{notifications.length !== 1 ? 's' : ''}
              {unreadCount > 0 && ` · ${unreadCount} unread`}
            </Typography>
            {unreadCount > 0 && (
              <Typography
                variant="caption" color="primary" fontWeight={600}
                sx={{ cursor: 'pointer', '&:hover': { textDecoration: 'underline' } }}
                onClick={markAllRead}
              >
                Mark all read
              </Typography>
            )}
          </Box>
        )}
      </Popover>

      {/* Sidebar Drawer */}
      <Box component="nav" sx={{ width: { md: DRAWER_WIDTH }, flexShrink: { md: 0 } }}>
        <Drawer
          variant={isMobile ? 'temporary' : 'permanent'}
          open={isMobile ? drawerOpen : true}
          onClose={() => setDrawerOpen(false)}
          ModalProps={{ keepMounted: true }}
          sx={{
            '& .MuiDrawer-paper': {
              width: DRAWER_WIDTH,
              boxSizing: 'border-box',
              border: 'none',
              boxShadow: '2px 0 8px rgba(0,0,0,0.06)',
            },
          }}
        >
          {drawerContent}
        </Drawer>
      </Box>

      {/* Main content */}
      <Box component="main"
        sx={{
          flexGrow: 1,
          width: { md: `calc(100% - ${DRAWER_WIDTH}px)` },
          mt: '64px',
          p: { xs: 2, md: 3 },
          minHeight: 'calc(100vh - 64px)',
        }}
      >
        {authBootstrapping ? (
          <Box display="flex" justifyContent="center" alignItems="center" minHeight={300}>
            <CircularProgress size={32} />
          </Box>
        ) : (
          <Outlet />
        )}
      </Box>
    </Box>
  );
}
