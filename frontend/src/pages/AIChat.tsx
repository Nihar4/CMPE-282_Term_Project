import React, { useState, useRef, useEffect, useCallback } from 'react';
import {
  Box, Card, CardContent, Typography, TextField, IconButton,
  Avatar, CircularProgress, Chip,
  Divider, Tooltip, Button,
  Collapse, Switch, FormControlLabel,
} from '@mui/material';
import {
  Send as SendIcon, SmartToy as AIIcon, Person as UserIcon,
  Storage as DBIcon, Description as DocIcon,
  ExpandMore, ExpandLess, ContentCopy, Clear,
} from '@mui/icons-material';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { fileApi, api } from '../services/api';

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  reasoning?: string;
  loading?: boolean;
  error?: boolean;
  includedDocs?: boolean;
}

const EXAMPLE_QUERIES = [
  'Show me the top 5 highest paid employees',
  'How many employees are in each department?',
  'What is the average salary by department?',
  'List all Senior Engineers with their salary',
  'Who are the current department managers?',
  'Which employees were hired before 1990?',
  'Show gender distribution across all departments',
  'What titles does the Development department have?',
];

const markdownComponents = {
  table: ({ children, ...props }: any) => (
    <Box sx={{ overflowX: 'auto', my: 1 }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }} {...props}>
        {children}
      </table>
    </Box>
  ),
  thead: ({ children, ...props }: any) => (
    <thead style={{ backgroundColor: '#f0f4ff' }} {...props}>{children}</thead>
  ),
  th: ({ children, ...props }: any) => (
    <th style={{ border: '1px solid #d0d7ff', padding: '6px 12px', textAlign: 'left', fontWeight: 600 }} {...props}>
      {children}
    </th>
  ),
  td: ({ children, ...props }: any) => (
    <td style={{ border: '1px solid #e2e8f0', padding: '5px 12px' }} {...props}>{children}</td>
  ),
  tr: ({ children, ...props }: any) => (
    <tr style={{ borderBottom: '1px solid #e2e8f0' }} {...props}>{children}</tr>
  ),
  code: ({ children, ...props }: any) => (
    <code style={{ backgroundColor: '#f0f0f0', padding: '1px 5px', borderRadius: 4, fontSize: 12, fontFamily: 'monospace' }} {...props}>
      {children}
    </code>
  ),
  pre: ({ children, ...props }: any) => (
    <pre style={{ backgroundColor: '#1e1e1e', color: '#ce9178', padding: '10px 14px', borderRadius: 8, overflowX: 'auto', fontSize: 12, margin: '8px 0' }} {...props}>
      {children}
    </pre>
  ),
  p: ({ children }: any) => <p style={{ margin: '4px 0' }}>{children}</p>,
  ul: ({ children }: any) => <ul style={{ margin: '4px 0', paddingLeft: 20 }}>{children}</ul>,
  ol: ({ children }: any) => <ol style={{ margin: '4px 0', paddingLeft: 20 }}>{children}</ol>,
  li: ({ children }: any) => <li style={{ marginBottom: 2 }}>{children}</li>,
  h2: ({ children }: any) => <h2 style={{ fontSize: 15, fontWeight: 700, margin: '10px 0 4px' }}>{children}</h2>,
  h3: ({ children }: any) => <h3 style={{ fontSize: 14, fontWeight: 600, margin: '8px 0 4px' }}>{children}</h3>,
  strong: ({ children }: any) => <strong style={{ fontWeight: 700 }}>{children}</strong>,
};

export default function AIChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([
    {
      role: 'assistant',
      content: `Hello! I'm your **Enterprise AI Assistant**.

I can answer questions about your employee data and uploaded documents. Toggle **Include Documents** to also search your uploaded files.

**Try asking:**
- *Show me the top 5 highest paid employees*
- *What is the average salary by department?*
- *Who manages the Development department?*`,
    },
  ]);
  const [input, setInput] = useState('');
  const [includeDocs, setIncludeDocs] = useState(false);
  const [files, setFiles] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [showThinking, setShowThinking] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => {
    fileApi.list()
      .then(r => setFiles((r.files || []).filter((f: any) => f.status === 'ready')))
      .catch(() => {});
  }, []);

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).catch(() => {});
  };

  const sendMessage = useCallback(async (question: string) => {
    if (!question.trim() || loading) return;

    const userMsg: ChatMessage = { role: 'user', content: question, includedDocs: includeDocs };
    const aiPlaceholder: ChatMessage = { role: 'assistant', content: '', loading: true };

    setMessages(prev => [...prev, userMsg, aiPlaceholder]);
    setInput('');
    setLoading(true);

    let accumulated = '';
    let thinking = '';

    try {
      const response = await fetch(
        `${process.env.REACT_APP_API_URL || 'http://localhost:8080'}/api/ai/stream`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': api.defaults.headers.common['Authorization'] as string || '',
          },
          body: JSON.stringify({ question, include_docs: includeDocs }),
        }
      );

      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      if (!response.body) throw new Error('No response body');

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const text = decoder.decode(value, { stream: true });
        for (const line of text.split('\n')) {
          if (!line.startsWith('data: ')) continue;
          const data = line.slice(6);
          if (data === '[DONE]') continue;
          try {
            const parsed = JSON.parse(data);
            if (parsed.thinking) thinking += parsed.thinking;
            if (parsed.content) accumulated += parsed.content;
            if (parsed.error) throw new Error(parsed.error);

            setMessages(prev => {
              const updated = [...prev];
              const last = updated[updated.length - 1];
              if (last.role === 'assistant') {
                updated[updated.length - 1] = {
                  ...last,
                  content: accumulated,
                  reasoning: thinking,
                  loading: false,
                  includedDocs: includeDocs,
                };
              }
              return updated;
            });
          } catch {}
        }
      }

      if (!accumulated) {
        setMessages(prev => {
          const updated = [...prev];
          updated[updated.length - 1] = {
            role: 'assistant',
            content: 'No response received. Please try again.',
            error: true,
          };
          return updated;
        });
      }
    } catch (e: any) {
      setMessages(prev => {
        const updated = [...prev];
        updated[updated.length - 1] = {
          role: 'assistant',
          content: `Error: ${e.message}`,
          error: true,
        };
        return updated;
      });
    }

    setLoading(false);
    inputRef.current?.focus();
  }, [loading, includeDocs]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage(input);
    }
  };

  return (
    <Box sx={{ display: 'flex', gap: 2, height: 'calc(100vh - 130px)' }}>
      {/* Main Chat */}
      <Card sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>

        {/* Header */}
        <Box sx={{
          px: 2, py: 1.5, borderBottom: 1, borderColor: 'divider',
          display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap',
          bgcolor: '#fafbff',
        }}>
          <AIIcon color="primary" />
          <Typography variant="h6" fontWeight={600} sx={{ flex: 1 }}>AI Assistant</Typography>

          {/* Include Docs toggle */}
          <Tooltip title={includeDocs
            ? `Searching database + ${files.length} uploaded document${files.length !== 1 ? 's' : ''}`
            : 'Searching database only — toggle to include uploaded documents'
          }>
            <FormControlLabel
              control={
                <Switch
                  checked={includeDocs}
                  onChange={e => setIncludeDocs(e.target.checked)}
                  size="small"
                  color="primary"
                />
              }
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  <DocIcon sx={{ fontSize: 15, color: includeDocs ? 'primary.main' : 'text.disabled' }} />
                  <Typography variant="caption" fontWeight={includeDocs ? 600 : 400}
                    color={includeDocs ? 'primary.main' : 'text.secondary'}>
                    Include Docs {files.length > 0 ? `(${files.length})` : ''}
                  </Typography>
                </Box>
              }
              sx={{ mr: 0, ml: 'auto' }}
            />
          </Tooltip>

          <Tooltip title="Clear chat">
            <IconButton size="small" onClick={() => setMessages([{
              role: 'assistant',
              content: 'Chat cleared. How can I help you?',
            }])}>
              <Clear fontSize="small" />
            </IconButton>
          </Tooltip>

        </Box>

        {/* Doc context chips (when include_docs is on) */}
        {includeDocs && files.length > 0 && (
          <Box sx={{ px: 2, py: 0.75, borderBottom: 1, borderColor: 'divider', bgcolor: '#f0f4ff', display: 'flex', flexWrap: 'wrap', gap: 0.5, alignItems: 'center' }}>
            <Typography variant="caption" color="text.secondary" mr={0.5}>
              Searching across:
            </Typography>
            {files.map((f: any) => (
              <Chip key={f.id} label={f.original_name} size="small" icon={<DocIcon />}
                color="primary" variant="outlined" sx={{ fontSize: 10, height: 20 }} />
            ))}
          </Box>
        )}
        {includeDocs && files.length === 0 && (
          <Box sx={{ px: 2, py: 0.75, borderBottom: 1, borderColor: 'divider', bgcolor: '#fff8e1' }}>
            <Typography variant="caption" color="warning.dark">
              No uploaded documents found. Upload files in the <strong>File Upload</strong> page first.
            </Typography>
          </Box>
        )}

        {/* Messages */}
        <Box sx={{ flex: 1, overflowY: 'auto', p: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {messages.map((msg, i) => (
            <Box key={i} sx={{
              display: 'flex', gap: 1.5, alignItems: 'flex-start',
              flexDirection: msg.role === 'user' ? 'row-reverse' : 'row',
            }}>
              <Avatar sx={{
                width: 30, height: 30, flexShrink: 0,
                bgcolor: msg.role === 'user' ? '#1a237e' : '#7c3aed',
                fontSize: 14,
              }}>
                {msg.role === 'user' ? <UserIcon sx={{ fontSize: 16 }} /> : <AIIcon sx={{ fontSize: 16 }} />}
              </Avatar>

              <Box sx={{ maxWidth: '82%' }}>
                {msg.loading ? (
                  <Box sx={{
                    display: 'flex', alignItems: 'center', gap: 1.5, px: 2, py: 1.5,
                    bgcolor: '#f8f9ff', borderRadius: 2, border: '1px solid #e0e7ff',
                  }}>
                    <CircularProgress size={14} />
                    <Typography variant="caption" color="text.secondary">
                      Thinking… querying database{includeDocs && files.length > 0 ? ' + documents' : ''}
                    </Typography>
                  </Box>
                ) : (
                  <Box sx={{
                    px: 2, py: 1.5, borderRadius: 2,
                    bgcolor: msg.role === 'user' ? '#1a237e' : msg.error ? '#fff0f0' : '#ffffff',
                    color: msg.role === 'user' ? '#fff' : 'text.primary',
                    border: '1px solid',
                    borderColor: msg.role === 'user' ? '#1a237e' : msg.error ? '#ffcdd2' : '#e8eaf6',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.06)',
                  }}>
                    {/* Reasoning toggle */}
                    {msg.reasoning && (
                      <Box mb={1}>
                        <Button size="small"
                          onClick={() => setShowThinking(s => !s)}
                          endIcon={showThinking ? <ExpandLess /> : <ExpandMore />}
                          sx={{ fontSize: 10, p: 0, color: 'text.disabled', minWidth: 0 }}>
                          {showThinking ? 'Hide' : 'Show'} reasoning
                        </Button>
                        <Collapse in={showThinking}>
                          <Box sx={{ mt: 0.5, p: 1, bgcolor: '#f5f5f5', borderRadius: 1, maxHeight: 120, overflowY: 'auto' }}>
                            <Typography variant="caption" sx={{ fontStyle: 'italic', whiteSpace: 'pre-wrap', color: 'text.secondary', fontSize: 11 }}>
                              {msg.reasoning}
                            </Typography>
                          </Box>
                        </Collapse>
                        <Divider sx={{ my: 1 }} />
                      </Box>
                    )}

                    {/* Markdown content */}
                    {msg.role === 'user' ? (
                      <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', color: 'inherit' }}>
                        {msg.content}
                      </Typography>
                    ) : (
                      <Box sx={{ fontSize: 14, lineHeight: 1.6 }}>
                        <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                          {msg.content}
                        </ReactMarkdown>
                      </Box>
                    )}

                    {/* Footer */}
                    {msg.role === 'assistant' && !msg.loading && (
                      <Box display="flex" alignItems="center" gap={0.5} mt={0.75} pt={0.5}
                        sx={{ borderTop: '1px solid', borderColor: 'divider' }}>
                        {msg.includedDocs && (
                          <Chip label="DB + Docs" size="small" icon={<DocIcon />}
                            sx={{ fontSize: 9, height: 16 }} variant="outlined" color="primary" />
                        )}
                        {!msg.includedDocs && !msg.error && (
                          <Chip label="Database" size="small" icon={<DBIcon />}
                            sx={{ fontSize: 9, height: 16 }} variant="outlined" />
                        )}
                        <Box sx={{ flex: 1 }} />
                        <Tooltip title="Copy">
                          <IconButton size="small" onClick={() => copyToClipboard(msg.content)} sx={{ p: 0.3 }}>
                            <ContentCopy sx={{ fontSize: 12, color: 'action.disabled' }} />
                          </IconButton>
                        </Tooltip>
                      </Box>
                    )}
                  </Box>
                )}
              </Box>
            </Box>
          ))}
          <div ref={messagesEndRef} />
        </Box>

        {/* Example queries */}
        {messages.length === 1 && (
          <Box sx={{ px: 2, pb: 1, display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
            {EXAMPLE_QUERIES.map(q => (
              <Chip key={q} label={q} size="small" variant="outlined"
                sx={{ cursor: 'pointer', fontSize: 11, '&:hover': { bgcolor: '#f0f4ff' } }}
                icon={<DBIcon sx={{ fontSize: '13px !important' }} />}
                onClick={() => sendMessage(q)} />
            ))}
          </Box>
        )}

        {/* Input */}
        <Box sx={{ px: 2, pb: 2, pt: 1, borderTop: 1, borderColor: 'divider', bgcolor: '#fafbff' }}>
          <Box display="flex" gap={1}>
            <TextField
              fullWidth multiline maxRows={4} size="small"
              placeholder={includeDocs
                ? 'Ask anything — searches database + your uploaded documents…'
                : 'Ask about employee data, salaries, departments…'}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={loading}
              inputRef={inputRef}
              sx={{ '& .MuiOutlinedInput-root': { borderRadius: 3, bgcolor: '#fff' } }}
            />
            <IconButton
              onClick={() => sendMessage(input)}
              disabled={!input.trim() || loading}
              sx={{
                bgcolor: 'primary.main', color: '#fff', borderRadius: 2, px: 1.5,
                '&:hover': { bgcolor: 'primary.dark' },
                '&:disabled': { bgcolor: 'action.disabledBackground' },
              }}>
              {loading ? <CircularProgress size={20} sx={{ color: '#fff' }} /> : <SendIcon />}
            </IconButton>
          </Box>
          <Typography variant="caption" color="text.disabled" mt={0.5} display="block">
            Enter to send · Shift+Enter for new line · Always streaming · {includeDocs ? 'DB + Docs mode' : 'Database mode'}
          </Typography>
        </Box>
      </Card>

    </Box>
  );
}
