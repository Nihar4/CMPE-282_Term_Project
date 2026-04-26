package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─── Models ──────────────────────────────────────────────────────────────────

type QueryHistory struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`
	QueryText      string    `json:"query_text"`
	QueryType      string    `json:"query_type"`
	GeneratedSQL   string    `json:"generated_sql,omitempty"`
	ResultSummary  string    `json:"result_summary,omitempty"`
	ResultCount    int       `json:"result_count"`
	ExecutionTime  int       `json:"execution_time"`
	Status         string    `gorm:"default:success" json:"status"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func (QueryHistory) TableName() string { return "query_history" }

type FileChunk struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	FileID     uuid.UUID `gorm:"type:uuid" json:"file_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
}

func (FileChunk) TableName() string { return "file_chunks" }

// ─── Config ──────────────────────────────────────────────────────────────────

type Config struct {
	Port          string
	DBDSN         string
	NvidiaAPIKey  string
	NvidiaBaseURL string
	NvidiaModel   string
}

func loadConfig() *Config {
	return &Config{
		Port: getEnv("PORT", "8084"),
		DBDSN: fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
			getEnv("DB_HOST", "localhost"),
			getEnv("DB_PORT", "5432"),
			getEnv("DB_USER", "portal_user"),
			getEnv("DB_PASSWORD", "portal_password"),
			getEnv("DB_NAME", "enterprise_portal"),
		),
		NvidiaAPIKey:  getEnv("NVIDIA_API_KEY", ""),
		NvidiaBaseURL: getEnv("NVIDIA_BASE_URL", "https://integrate.api.nvidia.com/v1"),
		NvidiaModel:   getEnv("NVIDIA_MODEL", "moonshotai/kimi-k2-instruct"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ─── NVIDIA / OpenAI-compatible API client ────────────────────────────────────

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	Temperature float64                `json:"temperature"`
	TopP        float64                `json:"top_p"`
	MaxTokens   int                    `json:"max_tokens"`
	Stream      bool                   `json:"stream"`
	ExtraBody   map[string]interface{} `json:"extra_body,omitempty"`
}

type ChatChoice struct {
	Delta   Message `json:"delta"`
	Message Message `json:"message"`
	Index   int     `json:"index"`
}

type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
}

type NvidiaClient struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

func newNvidiaClient(cfg *Config) *NvidiaClient {
	return &NvidiaClient{
		BaseURL: cfg.NvidiaBaseURL,
		APIKey:  cfg.NvidiaAPIKey,
		Model:   cfg.NvidiaModel,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// ChatComplete sends a request and returns the full response (non-streaming)
func (c *NvidiaClient) ChatComplete(messages []Message, enableThinking bool) (string, string, error) {
	req := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: 0.6,
		TopP:        1.0,
		MaxTokens:   16384,
		Stream:      false,
	}
	if enableThinking {
		req.ExtraBody = map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"enable_thinking": true,
				"clear_thinking":  false,
			},
		}
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("nvidia API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("nvidia API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", "", fmt.Errorf("response parse error: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content,
		chatResp.Choices[0].Message.ReasoningContent, nil
}

// StreamChat streams the response via SSE
func (c *NvidiaClient) StreamChat(messages []Message, w io.Writer, flusher http.Flusher) error {
	req := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: 0.7,
		TopP:        1.0,
		MaxTokens:   16384,
		Stream:      true,
		ExtraBody: map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"enable_thinking": true,
				"clear_thinking":  false,
			},
		},
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := (&http.Client{Timeout: 300 * time.Second}).Do(httpReq)
	if err != nil {
		return fmt.Errorf("stream request failed: %w", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		event := map[string]interface{}{}
		if delta.ReasoningContent != "" {
			event["thinking"] = delta.ReasoningContent
		}
		if delta.Content != "" {
			event["content"] = delta.Content
		}
		if len(event) > 0 {
			eventJSON, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", eventJSON)
			flusher.Flush()
		}
	}
	return scanner.Err()
}

// ─── Database Schema String (for SQL generation) ──────────────────────────────

const dbSchemaContext = `
Enterprise Database Schema (datacharmer/test_db — PostgreSQL):

Table: employees
  - emp_no (int, PK), birth_date (date), first_name (varchar), last_name (varchar),
    gender (char: M/F), hire_date (date)

Table: departments
  - dept_no (char(4), PK e.g. 'd001'), dept_name (varchar)
  - Values: d001=Marketing, d002=Finance, d003=Human Resources, d004=Production,
    d005=Development, d006=Quality Management, d007=Sales, d008=Research, d009=Customer Service

Table: dept_emp  (employee ↔ department assignments)
  - emp_no (int, FK→employees), dept_no (char(4), FK→departments),
    from_date (date), to_date (date)
  - Current assignments: to_date = '9999-01-01'

Table: dept_manager  (department managers)
  - emp_no (int, FK→employees), dept_no (char(4), FK→departments),
    from_date (date), to_date (date)
  - Current managers: to_date = '9999-01-01'

Table: titles  (job titles history)
  - emp_no (int, FK→employees), title (varchar), from_date (date), to_date (date)
  - Example titles: Engineer, Senior Engineer, Staff, Senior Staff, Manager,
    Technique Leader, Assistant Engineer
  - Current titles: to_date = '9999-01-01'

Table: salaries  (salary history)
  - emp_no (int, FK→employees), salary (int), from_date (date), to_date (date)
  - Current salary: to_date = '9999-01-01'

View: current_dept_emp  (only current dept assignments)
  - emp_no, dept_no, from_date, to_date

Useful query patterns:
  -- Current employees in a department:
     SELECT e.* FROM employees e JOIN dept_emp de ON e.emp_no=de.emp_no WHERE de.dept_no='d005' AND de.to_date='9999-01-01'
  -- Employee with current title and salary:
     SELECT e.first_name, e.last_name, t.title, s.salary FROM employees e
     JOIN titles t ON e.emp_no=t.emp_no AND t.to_date='9999-01-01'
     JOIN salaries s ON e.emp_no=s.emp_no AND s.to_date='9999-01-01'
  -- Average salary by department:
     SELECT d.dept_name, AVG(s.salary) FROM departments d
     JOIN dept_emp de ON d.dept_no=de.dept_no AND de.to_date='9999-01-01'
     JOIN salaries s ON de.emp_no=s.emp_no AND s.to_date='9999-01-01'
     GROUP BY d.dept_name ORDER BY AVG(s.salary) DESC

Table: uploaded_files
  - id (uuid), original_name (varchar), file_type (varchar: csv/pdf/docx/txt),
    status (varchar: processing/ready/error), row_count (integer), created_at (timestamp)

Table: file_chunks
  - id (uuid), file_id (uuid FK→uploaded_files), chunk_index (int), content (text)
`

// ─── AI Handler ───────────────────────────────────────────────────────────────

type AIHandler struct {
	db     *gorm.DB
	nvidia *NvidiaClient
	sqlDB  *sql.DB
}

// POST /api/ai/query — natural language → SQL → results (non-streaming)
func (h *AIHandler) Query(c *gin.Context) {
	var req struct {
		Question    string   `json:"question" binding:"required"`
		FileIDs     []string `json:"file_ids"`
		Mode        string   `json:"mode"` // "nl_to_sql" | "document_qa" | "general"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start := time.Now()
	userID := c.GetHeader("X-User-ID")

	mode := req.Mode
	if mode == "" {
		mode = "nl_to_sql"
		if len(req.FileIDs) > 0 {
			mode = "document_qa"
		}
	}

	var answer, generatedSQL, reasoning string
	var resultCount int
	var queryErr error

	switch mode {
	case "nl_to_sql":
		answer, generatedSQL, resultCount, reasoning, queryErr = h.handleNLToSQL(req.Question)
	case "document_qa":
		answer, reasoning, queryErr = h.handleDocumentQA(req.Question, req.FileIDs)
	default:
		answer, reasoning, queryErr = h.handleGeneral(req.Question)
	}

	elapsed := int(time.Since(start).Milliseconds())
	status := "success"
	errMsg := ""
	if queryErr != nil {
		status = "error"
		errMsg = queryErr.Error()
		if answer == "" {
			answer = "I encountered an error processing your question. " + errMsg
		}
	}

	// Save history
	var uid *uuid.UUID
	if userID != "" {
		id, _ := uuid.Parse(userID)
		uid = &id
	}
	h.db.Create(&QueryHistory{
		UserID:        uid,
		QueryText:     req.Question,
		QueryType:     mode,
		GeneratedSQL:  generatedSQL,
		ResultSummary: answer,
		ResultCount:   resultCount,
		ExecutionTime: elapsed,
		Status:        status,
		ErrorMessage:  errMsg,
	})

	c.JSON(http.StatusOK, gin.H{
		"answer":         answer,
		"reasoning":      reasoning,
		"generated_sql":  generatedSQL,
		"result_count":   resultCount,
		"mode":           mode,
		"execution_time": elapsed,
	})
}

// handleNLToSQL implements a 2-step agent loop with timing logs:
//   Step 1 — LLM Call 1: generate SQL
//   Step 2 — Tool: execute SQL against PostgreSQL
//   Step 3 — LLM Call 2: format results into clean business answer
func (h *AIHandler) handleNLToSQL(question string) (string, string, int, string, error) {
	model := h.nvidia.Model

	// ── Agent Step 1: LLM Call 1 — SQL Generation ────────────────────────────
	t0 := time.Now()
	sqlPrompt := []Message{
		{
			Role: "system",
			Content: `You are a PostgreSQL expert. Your ONLY job is to output a single valid SQL SELECT query.

` + dbSchemaContext + `

STRICT RULES:
- Output ONLY the SQL query. Zero explanation, zero markdown, zero text before or after.
- Start your response with SELECT or WITH — nothing else.
- Never use DROP, DELETE, UPDATE, INSERT, ALTER, TRUNCATE.
- Always filter current records with to_date='9999-01-01' on dept_emp, titles, salaries.
- Use LIMIT 50 unless the user asks for more.
- PostgreSQL syntax only.`,
		},
		{Role: "user", Content: question},
	}

	sqlRaw, reasoning, err := h.nvidia.ChatComplete(sqlPrompt, false)
	llm1Ms := time.Since(t0).Milliseconds()
	log.Printf("[AGENT] LLM Call 1 (SQL gen) | model=%s | %.2fs", model, float64(llm1Ms)/1000)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("SQL generation failed: %w", err)
	}

	generatedSQL := cleanSQL(sqlRaw)
	if generatedSQL == "" {
		// Retry with stricter prompt
		t1 := time.Now()
		retryPrompt := []Message{
			{Role: "system", Content: "Output ONLY a SQL SELECT query for PostgreSQL. No explanation. Start with SELECT."},
			{Role: "user", Content: question + "\nSchema: " + dbSchemaContext},
		}
		sqlRaw2, _, _ := h.nvidia.ChatComplete(retryPrompt, false)
		log.Printf("[AGENT] LLM Call 1 retry | model=%s | %.2fs", model, float64(time.Since(t1).Milliseconds())/1000)
		generatedSQL = cleanSQL(sqlRaw2)
		if generatedSQL == "" {
			return "I couldn't generate a valid SQL query for this question. Please try rephrasing.", "", 0, reasoning, nil
		}
	}

	// ── Agent Step 2: Tool — Execute SQL ─────────────────────────────────────
	t2 := time.Now()
	rows, err := h.sqlDB.Query(generatedSQL)
	if err != nil {
		log.Printf("[AGENT] Tool (SQL exec) FAILED | %.2fs | err: %v", float64(time.Since(t2).Milliseconds())/1000, err)
		// Auto-fix: send error back to LLM
		tf := time.Now()
		fixPrompt := []Message{
			{Role: "system", Content: "Fix the PostgreSQL SQL error. Output ONLY the corrected SQL, no explanation."},
			{Role: "user", Content: fmt.Sprintf("Failed SQL:\n%s\n\nError: %v\n\nCorrected SQL:", generatedSQL, err)},
		}
		fixedRaw, _, _ := h.nvidia.ChatComplete(fixPrompt, false)
		log.Printf("[AGENT] LLM Call 1 fix | model=%s | %.2fs", model, float64(time.Since(tf).Milliseconds())/1000)
		if fixed := cleanSQL(fixedRaw); fixed != "" {
			rows2, err2 := h.sqlDB.Query(fixed)
			if err2 == nil {
				rows = rows2
				generatedSQL = fixed
				err = nil
			}
		}
		if err != nil {
			return fmt.Sprintf("I couldn't execute the database query: %v", err), generatedSQL, 0, reasoning, nil
		}
	}

	cols, _ := rows.Columns()
	var results []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		row := make(map[string]interface{})
		for i, col := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[col] = v
		}
		results = append(results, row)
	}
	rows.Close()
	toolMs := time.Since(t2).Milliseconds()
	log.Printf("[AGENT] Tool (SQL exec) | %.2fs | rows=%d", float64(toolMs)/1000, len(results))

	resultCount := len(results)
	resultJSON, _ := json.Marshal(results)

	// ── Agent Step 3: LLM Call 2 — Format results as clean business answer ───
	t3 := time.Now()
	formatPrompt := []Message{
		{
			Role: "system",
			Content: `You are a senior business analyst. Given a question and raw database results, write a clear professional answer.
- Lead with the direct answer
- Use bullet points or markdown tables for lists
- Highlight key numbers and insights
- Be concise — no filler phrases like "Based on the results..."
- Use $ for currency, format numbers with commas
- If results are empty, clearly state no matching data was found`,
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Question: %s\n\nDatabase returned %d row(s):\n%s\n\nWrite a clear business answer.", question, resultCount, string(resultJSON)),
		},
	}

	answer, _, _ := h.nvidia.ChatComplete(formatPrompt, true)
	llm2Ms := time.Since(t3).Milliseconds()
	log.Printf("[AGENT] LLM Call 2 (format) | model=%s | %.2fs", model, float64(llm2Ms)/1000)
	log.Printf("[AGENT] TOTAL | llm1=%.2fs | tool=%.2fs | llm2=%.2fs | total=%.2fs",
		float64(llm1Ms)/1000, float64(toolMs)/1000, float64(llm2Ms)/1000,
		float64(llm1Ms+toolMs+llm2Ms)/1000)

	if answer == "" {
		if resultCount == 0 {
			answer = "No matching records found in the database for your query."
		} else {
			answer = fmt.Sprintf("Query returned %d result(s).", resultCount)
		}
	}

	return answer, generatedSQL, resultCount, reasoning, nil
}

func (h *AIHandler) handleDocumentQA(question string, fileIDs []string) (string, string, error) {
	// Retrieve relevant chunks
	var chunks []FileChunk
	q := h.db.Model(&FileChunk{})
	if len(fileIDs) > 0 {
		q = q.Where("file_id IN ?", fileIDs)
	}
	// Full-text search within chunks
	q = q.Where("to_tsvector('english', content) @@ plainto_tsquery('english', ?)", question)
	q.Order("chunk_index").Limit(8).Find(&chunks)

	if len(chunks) == 0 {
		// Fallback: just get first few chunks
		h.db.Model(&FileChunk{}).Where("file_id IN ?", fileIDs).
			Order("chunk_index").Limit(5).Find(&chunks)
	}

	var contextBuilder strings.Builder
	for _, ch := range chunks {
		contextBuilder.WriteString(ch.Content)
		contextBuilder.WriteString("\n\n---\n\n")
	}
	context := contextBuilder.String()
	if context == "" {
		context = "No document content available."
	}

	messages := []Message{
		{
			Role: "system",
			Content: `You are a helpful enterprise knowledge assistant. Answer questions based strictly on the provided document context.
If the answer is not in the context, say so clearly. Be precise and cite relevant data when available.`,
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Document Context:\n\n%s\n\nQuestion: %s", context, question),
		},
	}

	answer, reasoning, err := h.nvidia.ChatComplete(messages, true)
	return answer, reasoning, err
}

func (h *AIHandler) handleGeneral(question string) (string, string, error) {
	messages := []Message{
		{
			Role: "system",
			Content: `You are an intelligent enterprise knowledge assistant for a company portal.
Help users understand business data, answer general business questions, and provide analytical insights.
You have access to employee, product, sales, and inventory data.`,
		},
		{Role: "user", Content: question},
	}
	return h.nvidia.ChatComplete(messages, true)
}

// POST /api/ai/stream — unified agent: always database + optional docs, always streaming
// Request: { question, include_docs: bool }
// Agent loop:
//   1. LLM Call 1 → generate SQL
//   2. Tool → execute SQL → get DB results
//   3. (if include_docs) RAG → search all ready file chunks
//   4. LLM Call 2 → stream clean markdown answer combining DB + doc context
func (h *AIHandler) StreamQuery(c *gin.Context) {
	var req struct {
		Question    string `json:"question" binding:"required"`
		IncludeDocs bool   `json:"include_docs"`
		// Legacy fields kept for backward compat
		FileIDs []string `json:"file_ids"`
		Mode    string   `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Legacy compat: if file_ids sent or mode=document_qa, treat as include_docs=true
	if len(req.FileIDs) > 0 || req.Mode == "document_qa" {
		req.IncludeDocs = true
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	sendErr := func(msg string) {
		ev, _ := json.Marshal(map[string]string{"error": msg})
		fmt.Fprintf(c.Writer, "data: %s\n\n", ev)
		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		flusher.Flush()
	}

	model := h.nvidia.Model
	totalStart := time.Now()

	// ── Step 1: LLM Call 1 — Generate SQL ────────────────────────────────────
	t0 := time.Now()
	sqlPrompt := []Message{
		{
			Role: "system",
			Content: `You are a PostgreSQL expert. Output ONLY a single SQL SELECT query — no explanation, no markdown, no text before or after.
Start with SELECT or WITH. Always use to_date='9999-01-01' for current records in dept_emp, titles, salaries tables.
` + dbSchemaContext,
		},
		{Role: "user", Content: req.Question},
	}
	sqlRaw, _, err := h.nvidia.ChatComplete(sqlPrompt, false)
	log.Printf("[AGENT] LLM Call 1 (SQL gen) | model=%s | %.2fs", model, time.Since(t0).Seconds())
	if err != nil {
		sendErr("SQL generation failed: " + err.Error())
		return
	}

	generatedSQL := cleanSQL(sqlRaw)
	if generatedSQL == "" {
		t1 := time.Now()
		retryPrompt := []Message{
			{Role: "system", Content: "Output ONLY a SQL SELECT query for PostgreSQL. No explanation. Start with SELECT."},
			{Role: "user", Content: req.Question + "\nSchema hint: " + dbSchemaContext},
		}
		raw2, _, _ := h.nvidia.ChatComplete(retryPrompt, false)
		log.Printf("[AGENT] LLM Call 1 retry | %.2fs", time.Since(t1).Seconds())
		generatedSQL = cleanSQL(raw2)
	}
	if generatedSQL == "" {
		sendErr("Could not generate a valid SQL query. Try rephrasing your question.")
		return
	}

	// ── Step 2: Tool — Execute SQL ────────────────────────────────────────────
	t2 := time.Now()
	rows, err := h.sqlDB.Query(generatedSQL)
	if err != nil {
		log.Printf("[AGENT] Tool (SQL exec) FAILED | %.2fs | %v", time.Since(t2).Seconds(), err)
		tf := time.Now()
		fixPrompt := []Message{
			{Role: "system", Content: "Fix the PostgreSQL SQL. Output ONLY the corrected SQL."},
			{Role: "user", Content: fmt.Sprintf("Failed SQL:\n%s\n\nError: %v\n\nCorrected SQL:", generatedSQL, err)},
		}
		fixedRaw, _, _ := h.nvidia.ChatComplete(fixPrompt, false)
		log.Printf("[AGENT] LLM Call 1 fix | %.2fs", time.Since(tf).Seconds())
		if fixed := cleanSQL(fixedRaw); fixed != "" {
			rows, err = h.sqlDB.Query(fixed)
			if err == nil {
				generatedSQL = fixed
			}
		}
		if err != nil {
			sendErr(fmt.Sprintf("Database query failed: %v", err))
			return
		}
	}

	cols, _ := rows.Columns()
	var dbResults []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		row := make(map[string]interface{})
		for i, col := range cols {
			v := vals[i]
			if b, ok2 := v.([]byte); ok2 {
				v = string(b)
			}
			row[col] = v
		}
		dbResults = append(dbResults, row)
	}
	rows.Close()
	log.Printf("[AGENT] Tool (SQL exec) | %.2fs | rows=%d", time.Since(t2).Seconds(), len(dbResults))

	resultJSON, _ := json.Marshal(dbResults)

	// ── Step 3: RAG — search all ready file chunks (if include_docs) ──────────
	var docContext string
	if req.IncludeDocs {
		t3 := time.Now()
		var fileIDs []string
		h.db.Raw(`SELECT id::text FROM uploaded_files WHERE status = 'ready'`).Scan(&fileIDs)

		if len(fileIDs) > 0 {
			var chunks []FileChunk

			// Try FTS first
			h.db.Model(&FileChunk{}).
				Where("file_id::text IN ?", fileIDs).
				Where("to_tsvector('english', content) @@ plainto_tsquery('english', ?)", req.Question).
				Order("chunk_index").Limit(10).Find(&chunks)

			// Fallback: keyword ILIKE search
			if len(chunks) == 0 {
				words := strings.Fields(req.Question)
				if len(words) > 0 {
					keyword := words[0]
					if len(words) > 1 {
						keyword = words[1]
					}
					h.db.Model(&FileChunk{}).
						Where("file_id::text IN ?", fileIDs).
						Where("content ILIKE ?", "%"+keyword+"%").
						Order("chunk_index").Limit(10).Find(&chunks)
				}
			}

			// Last fallback: newest chunks
			if len(chunks) == 0 {
				h.db.Model(&FileChunk{}).
					Where("file_id::text IN ?", fileIDs).
					Order("created_at DESC, chunk_index ASC").Limit(8).Find(&chunks)
			}

			var sb strings.Builder
			for _, ch := range chunks {
				sb.WriteString(ch.Content)
				sb.WriteString("\n\n")
			}
			docContext = strings.TrimSpace(sb.String())
			log.Printf("[AGENT] RAG search | %.2fs | chunks=%d", time.Since(t3).Seconds(), len(chunks))
		} else {
			log.Printf("[AGENT] RAG: no ready files found")
		}
	}

	// ── Step 4: LLM Call 2 — Stream formatted markdown answer ─────────────────
	systemPrompt := `You are a senior business intelligence assistant. Format ALL responses in clean Markdown.

Rules:
- Use **bold** for key numbers and important data points
- Use markdown tables (| Header | Header | \n|---|---|\n| val | val |) for any list of data with 2+ columns
- Use bullet points (- item) for simple lists
- Use ## for section headings when there are multiple sections
- Use $ for currency values and format large numbers with commas (e.g., $71,534)
- Start directly with the answer — no preamble like "Based on the data..."
- If database returned 0 rows, clearly state no matching records were found
- Be concise, insightful, and professional`

	var userContent string
	if docContext != "" {
		userContent = fmt.Sprintf(
			"Question: %s\n\n## Database Results (%d rows)\n```json\n%s\n```\n\n## Document Context\n%s\n\nAnswer the question using both the database results and document context. Combine insights from both sources when relevant.",
			req.Question, len(dbResults), string(resultJSON), docContext,
		)
	} else {
		userContent = fmt.Sprintf(
			"Question: %s\n\n## Database Results (%d rows)\n```json\n%s\n```\n\nAnswer the question based on the database results.",
			req.Question, len(dbResults), string(resultJSON),
		)
	}

	formatMessages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	// Save history async
	go h.db.Create(&QueryHistory{
		QueryText:    req.Question,
		QueryType:    "unified",
		GeneratedSQL: generatedSQL,
		ResultCount:  len(dbResults),
		Status:       "success",
	})

	t4 := time.Now()
	if err := h.nvidia.StreamChat(formatMessages, c.Writer, flusher); err != nil {
		sendErr(err.Error())
		return
	}
	log.Printf("[AGENT] LLM Call 2 (stream format) | model=%s | %.2fs", model, time.Since(t4).Seconds())
	log.Printf("[AGENT] TOTAL | %.2fs | question=%q | docs=%v | rows=%d",
		time.Since(totalStart).Seconds(), req.Question, req.IncludeDocs, len(dbResults))
}

// GET /api/ai/history
func (h *AIHandler) History(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	role := c.GetHeader("X-User-Role")

	q := h.db.Model(&QueryHistory{}).Order("created_at desc").Limit(50)
	if role != "admin" && userID != "" {
		id, _ := uuid.Parse(userID)
		q = q.Where("user_id = ?", id)
	}

	var history []QueryHistory
	q.Find(&history)
	c.JSON(http.StatusOK, gin.H{"history": history, "count": len(history)})
}

// GET /api/ai/schema — Return the DB schema for frontend display
func (h *AIHandler) Schema(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"schema": dbSchemaContext})
}

// ─── SQL Cleaner ──────────────────────────────────────────────────────────────

// cleanSQL extracts a safe SELECT query from LLM output that may contain
// markdown fences, explanation text, or thinking tokens.
func cleanSQL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// 1. Strip thinking/reasoning blocks (Kimi K2 sometimes emits <think>…</think>)
	for {
		start := strings.Index(strings.ToLower(raw), "<think>")
		end := strings.Index(strings.ToLower(raw), "</think>")
		if start >= 0 && end > start {
			raw = raw[:start] + raw[end+8:]
		} else {
			break
		}
	}

	// 2. Extract from ```sql … ``` fence first (most reliable)
	lower := strings.ToLower(raw)
	if idx := strings.Index(lower, "```sql"); idx >= 0 {
		rest := raw[idx+6:]
		if end := strings.Index(rest, "```"); end >= 0 {
			raw = strings.TrimSpace(rest[:end])
		}
	} else if idx := strings.Index(lower, "```"); idx >= 0 {
		rest := raw[idx+3:]
		// Skip optional language tag on same line
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			candidate := strings.TrimSpace(rest[:nl])
			if !strings.HasPrefix(strings.ToUpper(candidate), "SELECT") &&
				!strings.HasPrefix(strings.ToUpper(candidate), "WITH") {
				rest = rest[nl+1:]
			}
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			raw = strings.TrimSpace(rest[:end])
		}
	}

	// 3. Find the first SELECT or WITH anywhere in the text
	upper := strings.ToUpper(raw)
	for _, kw := range []string{"SELECT", "WITH"} {
		if idx := strings.Index(upper, kw); idx >= 0 {
			raw = strings.TrimSpace(raw[idx:])
			upper = strings.ToUpper(raw)
			break
		}
	}

	// 4. Must start with SELECT or WITH
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return ""
	}

	// 5. Block any dangerous DML/DDL — read-only enforcement
	for _, dangerous := range []string{"DROP ", "DELETE ", "UPDATE ", "INSERT ", "ALTER ", "TRUNCATE ", "GRANT ", "REVOKE ", "CREATE "} {
		if strings.Contains(upper, dangerous) {
			return ""
		}
	}

	return raw
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()

	db, err := gorm.Open(postgres.Open(cfg.DBDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}

	rawDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	rawDB.SetMaxOpenConns(25)
	rawDB.SetMaxIdleConns(10)

	// We need raw *sql.DB for executing AI-generated SQL queries
	directDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "portal_user"),
		getEnv("DB_PASSWORD", "portal_password"),
		getEnv("DB_NAME", "enterprise_portal"),
	)
	directDB, err := sql.Open("pgx", directDSN)
	if err != nil {
		log.Printf("Warning: could not open raw DB connection: %v", err)
	}

	h := &AIHandler{
		db:     db,
		nvidia: newNvidiaClient(cfg),
		sqlDB:  directDB,
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:  []string{"*"},
		ExposeHeaders: []string{"Content-Type"},
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ai-service", "model": cfg.NvidiaModel})
	})

	api := r.Group("/api/ai")
	{
		api.POST("/query", h.Query)
		api.POST("/stream", h.StreamQuery)
		api.GET("/history", h.History)
		api.GET("/schema", h.Schema)
	}

	log.Printf("AI Service listening on :%s (model: %s)", cfg.Port, cfg.NvidiaModel)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 300 * time.Second, // generous for streaming
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
