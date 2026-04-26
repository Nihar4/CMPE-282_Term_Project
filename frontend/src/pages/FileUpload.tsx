import React, { useCallback, useEffect, useState } from 'react';
import {
  Box, Card, CardContent, Typography, Button, LinearProgress,
  Grid, Chip, Alert, IconButton, List, ListItem, ListItemText,
  ListItemIcon, Tooltip, Divider,
} from '@mui/material';
import {
  CloudUpload, Description, TableChart, PictureAsPdf,
  CheckCircle, Error as ErrorIcon, HourglassEmpty,
  Delete as DeleteIcon, DeleteSweep as DeleteSweepIcon, Visibility as ViewIcon,
} from '@mui/icons-material';
import { useDropzone } from 'react-dropzone';
import { fileApi } from '../services/api';

const FILE_ICONS: Record<string, React.ReactNode> = {
  csv:  <TableChart color="success" />,
  pdf:  <PictureAsPdf color="error" />,
  docx: <Description color="primary" />,
  txt:  <Description color="action" />,
  default: <Description />,
};

const STATUS_ICONS: Record<string, React.ReactNode> = {
  ready:      <CheckCircle color="success" fontSize="small" />,
  error:      <ErrorIcon color="error" fontSize="small" />,
  processing: <HourglassEmpty color="warning" fontSize="small" />,
};

function formatBytes(bytes: number): string {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1048576).toFixed(1) + ' MB';
}

interface UploadingFile {
  name: string;
  progress: number;
  status: 'uploading' | 'done' | 'error';
  error?: string;
}

export default function FileUpload() {
  const [files, setFiles] = useState<any[]>([]);
  const [uploading, setUploading] = useState<UploadingFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [selectedFile, setSelectedFile] = useState<any | null>(null);
  const [chunks, setChunks] = useState<any[]>([]);

  const loadFiles = useCallback(() => {
    setLoading(true);
    fileApi.list()
      .then(r => setFiles(r.files || []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { loadFiles(); }, [loadFiles]);

  // Poll for processing files
  useEffect(() => {
    const processing = files.filter(f => f.status === 'processing');
    if (processing.length === 0) return;
    const interval = setInterval(loadFiles, 3000);
    return () => clearInterval(interval);
  }, [files, loadFiles]);

  const onDrop = useCallback(async (acceptedFiles: File[]) => {
    setError('');
    for (const file of acceptedFiles) {
      const entry: UploadingFile = { name: file.name, progress: 0, status: 'uploading' };
      setUploading(prev => [...prev, entry]);
      try {
        await fileApi.upload(file, pct => {
          setUploading(prev => prev.map(u => u.name === file.name ? { ...u, progress: pct } : u));
        });
        setUploading(prev => prev.map(u => u.name === file.name ? { ...u, status: 'done', progress: 100 } : u));
        setSuccess(`"${file.name}" uploaded successfully and is being processed.`);
        loadFiles();
      } catch (e: any) {
        setUploading(prev => prev.map(u => u.name === file.name ? { ...u, status: 'error', error: e.message } : u));
        setError(`Failed to upload "${file.name}": ${e.message}`);
      }
    }
  }, [loadFiles]);

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: {
      'text/csv': ['.csv'],
      'application/pdf': ['.pdf'],
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document': ['.docx'],
      'text/plain': ['.txt'],
    },
    maxSize: 50 * 1024 * 1024,
  });

  const handleDelete = async (id: string, name: string) => {
    if (!window.confirm(`Delete "${name}"?`)) return;
    try {
      await fileApi.delete(id);
      setFiles(prev => prev.filter(f => f.id !== id));
      if (selectedFile?.id === id) setSelectedFile(null);
    } catch (e: any) {
      setError(e.message);
    }
  };

  const handleDeleteAll = async () => {
    if (files.length === 0) return;
    if (!window.confirm(`Delete all ${files.length} uploaded files? This cannot be undone.`)) return;
    try {
      await fileApi.deleteAll();
      setFiles([]);
      setSelectedFile(null);
      setChunks([]);
      setSuccess('All uploaded files were deleted.');
      setError('');
      loadFiles();
    } catch (e: any) {
      setError(e.message);
    }
  };

  const handleView = async (file: any) => {
    setSelectedFile(file);
    if (file.status === 'ready') {
      fileApi.getChunks(file.id).then(r => setChunks(r.chunks || []));
    }
  };

  return (
    <Box>
      <Typography variant="h5" fontWeight={700} mb={0.5}>File Upload & Analysis</Typography>
      <Typography variant="body2" color="text.secondary" mb={3}>
        Upload CSV, PDF, or Word documents to enable AI-powered querying on your data
      </Typography>

      {error   && <Alert severity="error"   onClose={() => setError('')}   sx={{ mb: 2 }}>{error}</Alert>}
      {success && <Alert severity="success" onClose={() => setSuccess('')} sx={{ mb: 2 }}>{success}</Alert>}

      <Grid container spacing={2}>
        <Grid item xs={12} md={7}>
          {/* Drop Zone */}
          <Card>
            <CardContent>
              <Box
                {...getRootProps()}
                sx={{
                  border: '2px dashed',
                  borderColor: isDragActive ? 'primary.main' : 'divider',
                  borderRadius: 3,
                  p: 5,
                  textAlign: 'center',
                  cursor: 'pointer',
                  bgcolor: isDragActive ? 'primary.50' : 'background.default',
                  transition: 'all 0.2s',
                  '&:hover': { borderColor: 'primary.main', bgcolor: '#f3f4ff' },
                }}
              >
                <input {...getInputProps()} />
                <CloudUpload sx={{ fontSize: 56, color: isDragActive ? 'primary.main' : 'action.disabled', mb: 2 }} />
                <Typography variant="h6" fontWeight={600} mb={0.5}>
                  {isDragActive ? 'Drop files here…' : 'Drag & drop files here'}
                </Typography>
                <Typography variant="body2" color="text.secondary" mb={2}>
                  or click to browse. Supports CSV, PDF, DOCX, TXT up to 50MB
                </Typography>
                <Button variant="contained" size="small" sx={{ pointerEvents: 'none' }}>
                  Browse Files
                </Button>
              </Box>

              {/* Upload Progress */}
              {uploading.filter(u => u.status === 'uploading').length > 0 && (
                <Box mt={2}>
                  {uploading.filter(u => u.status === 'uploading').map(u => (
                    <Box key={u.name} mb={1}>
                      <Box display="flex" justifyContent="space-between">
                        <Typography variant="caption">{u.name}</Typography>
                        <Typography variant="caption">{u.progress}%</Typography>
                      </Box>
                      <LinearProgress variant="determinate" value={u.progress} sx={{ borderRadius: 2 }} />
                    </Box>
                  ))}
                </Box>
              )}
            </CardContent>
          </Card>

          {/* File List */}
          <Card sx={{ mt: 2 }}>
            <CardContent>
              <Box display="flex" alignItems="center" justifyContent="space-between" mb={1}>
                <Typography variant="h6" fontWeight={600}>
                  Uploaded Files ({files.length})
                </Typography>
                <Button
                  variant="outlined"
                  color="error"
                  size="small"
                  startIcon={<DeleteSweepIcon />}
                  onClick={handleDeleteAll}
                  disabled={files.length === 0}
                >
                  Delete All
                </Button>
              </Box>
              {loading ? (
                <Box py={3} textAlign="center"><Typography variant="body2" color="text.secondary">Loading…</Typography></Box>
              ) : files.length === 0 ? (
                <Box py={3} textAlign="center">
                  <CloudUpload sx={{ fontSize: 40, color: 'action.disabled', mb: 1 }} />
                  <Typography variant="body2" color="text.secondary">No files uploaded yet.</Typography>
                </Box>
              ) : (
                <List dense disablePadding>
                  {files.map((f: any, i: number) => (
                    <React.Fragment key={f.id}>
                      {i > 0 && <Divider />}
                      <ListItem disablePadding sx={{
                        py: 1, cursor: 'pointer', borderRadius: 1,
                        bgcolor: selectedFile?.id === f.id ? 'action.selected' : 'transparent',
                        '&:hover': { bgcolor: 'action.hover' },
                      }} onClick={() => handleView(f)}>
                        <ListItemIcon sx={{ minWidth: 36 }}>
                          {FILE_ICONS[f.file_type] || FILE_ICONS.default}
                        </ListItemIcon>
                        <ListItemText
                          primary={f.original_name}
                          secondary={`${formatBytes(f.file_size)} · ${f.file_type.toUpperCase()} · ${new Date(f.created_at).toLocaleDateString()}`}
                          primaryTypographyProps={{ variant: 'body2', fontWeight: 500 }}
                          secondaryTypographyProps={{ variant: 'caption' }}
                        />
                        <Box display="flex" alignItems="center" ml={1} mr={1}>
                          {STATUS_ICONS[f.status] || STATUS_ICONS.processing}
                        </Box>
                        <Box display="flex" alignItems="center">
                          <Tooltip title="View content">
                            <IconButton size="small" onClick={() => handleView(f)}><ViewIcon fontSize="small" /></IconButton>
                          </Tooltip>
                          <Tooltip title="Delete">
                            <IconButton size="small" color="error" onClick={() => handleDelete(f.id, f.original_name)}>
                              <DeleteIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        </Box>
                      </ListItem>
                    </React.Fragment>
                  ))}
                </List>
              )}
            </CardContent>
          </Card>
        </Grid>

        {/* File Preview */}
        <Grid item xs={12} md={5}>
          <Card sx={{ height: '100%', minHeight: 400 }}>
            <CardContent>
              {selectedFile ? (
                <>
                  <Typography variant="h6" fontWeight={600} mb={0.5}>{selectedFile.original_name}</Typography>
                  <Box display="flex" gap={1} mb={2} flexWrap="wrap">
                    <Chip label={selectedFile.file_type.toUpperCase()} size="small" color="primary" variant="outlined" />
                    <Chip label={formatBytes(selectedFile.file_size)} size="small" variant="outlined" />
                    {selectedFile.row_count > 0 && <Chip label={`${selectedFile.row_count} rows`} size="small" color="success" variant="outlined" />}
                    <Box display="flex" alignItems="center">
                      {STATUS_ICONS[selectedFile.status] || STATUS_ICONS.processing}
                    </Box>
                  </Box>

                  {selectedFile.status === 'processing' && (
                    <Box>
                      <LinearProgress sx={{ mb: 1, borderRadius: 2 }} />
                      <Typography variant="caption" color="text.secondary">
                        Processing file… This may take a moment.
                      </Typography>
                    </Box>
                  )}

                  {selectedFile.status === 'error' && (
                    <Alert severity="error" sx={{ mb: 2 }}>{selectedFile.error_message}</Alert>
                  )}

                  {chunks.length > 0 && (
                    <Box>
                      <Typography variant="subtitle2" fontWeight={600} mb={1}>
                        Extracted Content ({chunks.length} chunks)
                      </Typography>
                      <Box sx={{ maxHeight: 420, overflowY: 'auto', pr: 1 }}>
                        {chunks.slice(0, 5).map((c: any) => (
                          <Box key={c.id} mb={1.5} p={1.5} bgcolor="background.default" borderRadius={2}
                               sx={{ border: '1px solid', borderColor: 'divider' }}>
                            <Typography variant="caption" color="text.secondary" display="block" mb={0.5}>
                              Chunk {c.chunk_index + 1}
                            </Typography>
                            <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', fontSize: 12, maxHeight: 120, overflow: 'hidden' }}>
                              {c.content}
                            </Typography>
                          </Box>
                        ))}
                        {chunks.length > 5 && (
                          <Typography variant="caption" color="text.secondary">
                            +{chunks.length - 5} more chunks — use AI Chat to query this document
                          </Typography>
                        )}
                      </Box>
                    </Box>
                  )}

                  {selectedFile.status === 'ready' && chunks.length === 0 && (
                    <Box py={3} textAlign="center">
                      <Typography variant="body2" color="text.secondary">
                        File is ready. Use AI Chat to query its contents.
                      </Typography>
                    </Box>
                  )}
                </>
              ) : (
                <Box display="flex" flexDirection="column" alignItems="center" justifyContent="center" height={300}>
                  <ViewIcon sx={{ fontSize: 48, color: 'action.disabled', mb: 2 }} />
                  <Typography variant="body2" color="text.secondary">
                    Select a file to preview its content
                  </Typography>
                </Box>
              )}
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
}
