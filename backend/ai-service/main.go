package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
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

type AIJobRequest struct {
	JobID       string    `json:"job_id"`
	UserID      string    `json:"user_id,omitempty"`
	Question    string    `json:"question"`
	FileIDs     []string  `json:"file_ids,omitempty"`
	Mode        string    `json:"mode,omitempty"`
	IncludeDocs bool      `json:"include_docs,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type AIJobResult struct {
	JobID        string    `json:"job_id"`
	Status       string    `json:"status"`
	Question     string    `json:"question"`
	Answer       string    `json:"answer,omitempty"`
	Reasoning    string    `json:"reasoning,omitempty"`
	GeneratedSQL string    `json:"generated_sql,omitempty"`
	ResultCount  int       `json:"result_count"`
	Mode         string    `json:"mode"`
	Error        string    `json:"error,omitempty"`
	ExecutionMS  int       `json:"execution_time"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type NotificationEvent struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Icon      string    `json:"icon"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Config ──────────────────────────────────────────────────────────────────

type Config struct {
	Port              string
	DBDSN             string
	NvidiaAPIKey      string
	NvidiaBaseURL     string
	NvidiaModel       string
	RedisURL          string
	RedisAuth         string
	KafkaBrokers      []string
	AIRequestsTopic   string
	NotificationsTopic string
	AIConsumerGroup   string
	AIWorkerEnabled   bool
}

func loadConfig() *Config {
	brokers := strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	return &Config{
		Port: getEnv("PORT", "8084"),
		DBDSN: fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
			getEnv("DB_HOST", "localhost"),
			getEnv("DB_PORT", "5432"),
			getEnv("DB_USER", "portal_user"),
			getEnv("DB_PASSWORD", "portal_password"),
			getEnv("DB_NAME", "enterprise_portal"),
		),
		NvidiaAPIKey:      getEnv("NVIDIA_API_KEY", ""),
		NvidiaBaseURL:     getEnv("NVIDIA_BASE_URL", "https://integrate.api.nvidia.com/v1"),
		NvidiaModel:       getEnv("NVIDIA_MODEL", "moonshotai/kimi-k2-instruct"),
		RedisURL:          getEnv("REDIS_URL", "localhost:6379"),
		RedisAuth:         getEnv("REDIS_AUTH", ""),
		KafkaBrokers:      brokers,
		AIRequestsTopic:   getEnv("AI_REQUESTS_TOPIC", "portal.ai.requests"),
		NotificationsTopic: getEnv("NOTIFICATIONS_TOPIC", "portal.notifications"),
		AIConsumerGroup:   getEnv("AI_CONSUMER_GROUP", "portal-ai-workers"),
		AIWorkerEnabled:   getEnv("AI_WORKER_ENABLED", "true") == "true",
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
	db                 *gorm.DB
	nvidia             *NvidiaClient
	sqlDB              *sql.DB
	rdb                *redis.Client
	cfg                *Config
	aiWriter           *kafka.Writer
	notificationWriter *kafka.Writer
}

func (h *AIHandler) jobKey(jobID string) string {
	return "ai_job:" + jobID
}

func (h *AIHandler) saveJobResult(ctx context.Context, result AIJobResult) {
	if h.rdb == nil {
		return
	}
	result.UpdatedAt = time.Now().UTC()
	payload, err := json.Marshal(result)
	if err != nil {
		log.Printf("[AI] could not marshal job result: %v", err)
		return
	}
	if err := h.rdb.Set(ctx, h.jobKey(result.JobID), payload, 24*time.Hour).Err(); err != nil {
		log.Printf("[AI] could not save job %s to Redis: %v", result.JobID, err)
	}
}

func (h *AIHandler) getJobResult(ctx context.Context, jobID string) (*AIJobResult, error) {
	if h.rdb == nil {
		return nil, fmt.Errorf("Redis is not configured")
	}
	payload, err := h.rdb.Get(ctx, h.jobKey(jobID)).Bytes()
	if err != nil {
		return nil, err
	}
	var result AIJobResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (h *AIHandler) publishNotification(ctx context.Context, event NotificationEvent) {
	if h.notificationWriter == nil {
		return
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("[AI] could not marshal notification: %v", err)
		return
	}
	if err := h.notificationWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.ID),
		Value: payload,
		Time:  event.CreatedAt,
	}); err != nil {
		log.Printf("[AI] notification publish failed: %v", err)
	}
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

	result := h.runAIRequest(AIJobRequest{
		JobID:     uuid.NewString(),
		UserID:    c.GetHeader("X-User-ID"),
		Question:  req.Question,
		FileIDs:   req.FileIDs,
		Mode:      req.Mode,
		CreatedAt: time.Now().UTC(),
	})

	c.JSON(http.StatusOK, gin.H{
		"answer":         result.Answer,
		"reasoning":      result.Reasoning,
		"generated_sql":  result.GeneratedSQL,
		"result_count":   result.ResultCount,
		"mode":           result.Mode,
		"execution_time": result.ExecutionMS,
		"status":         result.Status,
		"error":          result.Error,
	})
}

func (h *AIHandler) runAIRequest(req AIJobRequest) AIJobResult {
	start := time.Now()
	mode := req.Mode
	if mode == "" {
		mode = "nl_to_sql"
		if len(req.FileIDs) > 0 || req.IncludeDocs {
			mode = "db_and_docs"
		}
	}

	var answer, generatedSQL, reasoning string
	var resultCount int
	var queryErr error

	switch mode {
	case "nl_to_sql":
		answer, generatedSQL, resultCount, reasoning, queryErr = h.handleNLToSQL(req.Question)
	case "db_and_docs":
		answer, generatedSQL, resultCount, reasoning, queryErr = h.handleDBAndFullDocs(req.Question, req.FileIDs)
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
	if req.UserID != "" {
		if id, err := uuid.Parse(req.UserID); err == nil {
			uid = &id
		}
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

	return AIJobResult{
		JobID:        req.JobID,
		Status:       status,
		Question:     req.Question,
		Answer:       answer,
		Reasoning:    reasoning,
		GeneratedSQL: generatedSQL,
		ResultCount:  resultCount,
		Mode:         mode,
		Error:        errMsg,
		ExecutionMS:  elapsed,
		UpdatedAt:    time.Now().UTC(),
	}
}

// POST /api/ai/jobs — enqueue AI work into Kafka and return a Redis-backed job id.
func (h *AIHandler) QueueQuery(c *gin.Context) {
	if h.aiWriter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Kafka AI queue is not configured"})
		return
	}

	var req struct {
		Question    string   `json:"question" binding:"required"`
		FileIDs     []string `json:"file_ids"`
		Mode        string   `json:"mode"`
		IncludeDocs bool     `json:"include_docs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := AIJobRequest{
		JobID:       uuid.NewString(),
		UserID:      c.GetHeader("X-User-ID"),
		Question:    req.Question,
		FileIDs:     req.FileIDs,
		Mode:        req.Mode,
		IncludeDocs: req.IncludeDocs,
		CreatedAt:   time.Now().UTC(),
	}

	h.saveJobResult(c.Request.Context(), AIJobResult{
		JobID:     job.JobID,
		Status:    "queued",
		Question:  job.Question,
		Mode:      job.Mode,
		UpdatedAt: time.Now().UTC(),
	})

	payload, _ := json.Marshal(job)
	if err := h.aiWriter.WriteMessages(c.Request.Context(), kafka.Message{
		Key:   []byte(job.JobID),
		Value: payload,
		Time:  job.CreatedAt,
	}); err != nil {
		log.Printf("[AI] Kafka enqueue failed for job %s: %v; processing locally", job.JobID, err)
		go h.processAIJob(context.Background(), job, "local-fallback")
		c.JSON(http.StatusAccepted, gin.H{
			"job_id":     job.JobID,
			"status":     "queued",
			"status_url": "/api/ai/jobs/" + job.JobID,
			"message":    "Kafka queue unavailable; processing with local Cloud Run fallback",
		})
		return
	}

	h.publishNotification(c.Request.Context(), NotificationEvent{
		UserID: job.UserID,
		Type:   "ai_queued",
		Title:  "AI request queued",
		Body:   fmt.Sprintf("%q is waiting for an AI worker", truncate(job.Question, 80)),
		Icon:   "ai",
	})

	go func() {
		// Give the Kafka consumer first chance; local fallback keeps dev reliable
		// when Kafka is slow or a worker is not scaled up.
		time.Sleep(2 * time.Second)
		h.processAIJob(context.Background(), job, "local-fallback")
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"job_id":     job.JobID,
		"status":     "queued",
		"status_url": "/api/ai/jobs/" + job.JobID,
		"message":    "AI request queued in Kafka",
	})
}

// GET /api/ai/jobs/:id — poll Redis for queued/running/completed AI result.
func (h *AIHandler) GetJob(c *gin.Context) {
	result, err := h.getJobResult(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *AIHandler) startAIWorker(ctx context.Context) {
	if !h.cfg.AIWorkerEnabled || len(h.cfg.KafkaBrokers) == 0 {
		return
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  h.cfg.KafkaBrokers,
		Topic:    h.cfg.AIRequestsTopic,
		GroupID:  h.cfg.AIConsumerGroup,
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  1 * time.Second,
	})

	go func() {
		defer reader.Close()
		log.Printf("[AI] Kafka worker listening topic=%s group=%s brokers=%s",
			h.cfg.AIRequestsTopic, h.cfg.AIConsumerGroup, strings.Join(h.cfg.KafkaBrokers, ","))
		for {
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[AI] Kafka fetch failed: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			var job AIJobRequest
			if err := json.Unmarshal(msg.Value, &job); err != nil {
				log.Printf("[AI] invalid Kafka job payload: %v", err)
				_ = reader.CommitMessages(ctx, msg)
				continue
			}

			h.processAIJob(ctx, job, "kafka-worker")

			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("[AI] Kafka commit failed for job %s: %v", job.JobID, err)
			}
		}
	}()
}

func (h *AIHandler) processAIJob(ctx context.Context, job AIJobRequest, worker string) {
	if existing, err := h.getJobResult(ctx, job.JobID); err == nil && isTerminalJobStatus(existing.Status) {
		log.Printf("[AI] job %s already %s, skipping worker=%s", job.JobID, existing.Status, worker)
		return
	}

	lockKey := "ai_job_lock:" + job.JobID
	if h.rdb != nil {
		acquired, err := h.rdb.SetNX(ctx, lockKey, worker, 30*time.Minute).Result()
		if err == nil && !acquired {
			log.Printf("[AI] job %s already claimed, skipping worker=%s", job.JobID, worker)
			return
		}
		if err != nil {
			log.Printf("[AI] lock warning for job %s: %v", job.JobID, err)
		}
		defer h.rdb.Del(ctx, lockKey)
	}

	if existing, err := h.getJobResult(ctx, job.JobID); err == nil && isTerminalJobStatus(existing.Status) {
		log.Printf("[AI] job %s became %s before worker=%s started, skipping", job.JobID, existing.Status, worker)
		return
	}

	log.Printf("[AI] processing job=%s worker=%s question=%q", job.JobID, worker, job.Question)
	h.saveJobResult(ctx, AIJobResult{
		JobID:    job.JobID,
		Status:   "running",
		Question: job.Question,
		Mode:     job.Mode,
	})

	result := h.runAIRequest(job)
	h.saveJobResult(ctx, result)

	notif := NotificationEvent{
		UserID: job.UserID,
		Icon:   "ai",
	}
	if result.Status == "success" {
		notif.Type = "ai_query"
		notif.Title = "AI query completed"
		notif.Body = fmt.Sprintf("%q completed with %d result(s)", truncate(job.Question, 80), result.ResultCount)
	} else {
		notif.Type = "ai_error"
		notif.Title = "AI query failed"
		notif.Body = fmt.Sprintf("%q failed: %s", truncate(job.Question, 80), result.Error)
		notif.Icon = "warning"
	}
	h.publishNotification(ctx, notif)
	log.Printf("[AI] finished job=%s worker=%s status=%s elapsed_ms=%d", job.JobID, worker, result.Status, result.ExecutionMS)
}

func isTerminalJobStatus(status string) bool {
	return status == "success" || status == "error"
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
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
			Content: `You are a PostgreSQL expert. Your ONLY job is to output a single read-only SQL query.

` + dbSchemaContext + `

STRICT RULES (READ-ONLY; NO WRITES):
- You may ONLY output SELECT, or WITH ... SELECT (CTEs that read data only). One statement only.
- Forbidden: INSERT, UPDATE, DELETE, MERGE, COPY (write), TRUNCATE, ALTER, DROP, CREATE, REPLACE, VACUUM, REINDEX, GRANT, REVOKE, DO, CALL, EXECUTE, or any command that changes data, schema, or permissions.
- Forbidden: SELECT ... INTO, CREATE TABLE AS, or any pattern that creates or overwrites data.
- Output ONLY the raw SQL. Zero explanation, zero markdown, zero text before or after.
- Start with SELECT or WITH — nothing else.
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
			{Role: "system", Content: "Output ONLY a single read-only PostgreSQL SELECT (or WITH ... SELECT) query. No INSERT/UPDATE/DELETE/DDL. No explanation. Start with SELECT or WITH."},
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
			{Role: "system", Content: "Fix the PostgreSQL SQL error. Output ONLY a single read-only SELECT or WITH ... SELECT. No INSERT/UPDATE/DELETE/DDL. No explanation."},
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

func (h *AIHandler) handleDBAndFullDocs(question string, fileIDs []string) (string, string, int, string, error) {
	dbAnswer, generatedSQL, resultCount, reasoning, dbErr := h.handleNLToSQL(question)
	docContext := h.loadHybridDocumentContext(question, fileIDs, 5)

	messages := []Message{
		{
			Role: "system",
			Content: `You are an enterprise assistant that can compare database results with uploaded documents.
Use BOTH sources:
1. Database answer and generated SQL.
2. The top retrieved document chunks.

Important rules:
- Do not say the answer is missing unless it is missing from both the retrieved document chunks and database answer below.
- If the user asks for relationships between documents and database, compare names, emails, course/employee-like fields, dates, departments, IDs, and other overlapping values.
- Clearly separate "Database evidence", "Document evidence", and "Relationship / conclusion" when useful.
- If the database is unrelated to the uploaded documents, say that clearly and explain why.`,
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Question: %s\n\n## Database Answer\n%s\n\n## Generated SQL\n%s\n\n## Top Document Chunks\n%s",
				question, dbAnswer, generatedSQL, docContext),
		},
	}

	answer, extraReasoning, err := h.nvidia.ChatComplete(messages, true)
	if reasoning == "" {
		reasoning = extraReasoning
	}
	if dbErr != nil && err == nil {
		err = dbErr
	}
	return answer, generatedSQL, resultCount, reasoning, err
}

func (h *AIHandler) handleDocumentQA(question string, fileIDs []string) (string, string, error) {
	context := h.loadHybridDocumentContext(question, fileIDs, 5)
	messages := []Message{
		{
			Role: "system",
			Content: `You are a helpful enterprise knowledge assistant. Answer questions using the retrieved document chunks.
The chunks were selected with hybrid search and capped to the top 5. If the answer is not present in the context, say so clearly.`,
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Document Context:\n\n%s\n\nQuestion: %s", context, question),
		},
	}

	answer, reasoning, err := h.nvidia.ChatComplete(messages, true)
	return answer, reasoning, err
}

func (h *AIHandler) loadHybridDocumentContext(question string, fileIDs []string, limit int) string {
	type RetrievedDocChunk struct {
		ID         string `gorm:"column:id"`
		FileName   string `gorm:"column:file_name"`
		ChunkIndex int    `gorm:"column:chunk_index"`
		Content    string `gorm:"column:content"`
	}

	if limit <= 0 {
		limit = 5
	}

	queryTokens := tokenizeForSearch(question)
	chunkByID := map[string]RetrievedDocChunk{}
	scoreByID := map[string]int{}

	base := h.db.Table("file_chunks AS fc").
		Select("fc.id::text AS id, uf.original_name AS file_name, fc.chunk_index, fc.content").
		Joins("JOIN uploaded_files AS uf ON uf.id = fc.file_id").
		Where("uf.status = ?", "ready")
	if len(fileIDs) > 0 {
		base = base.Where("fc.file_id IN ?", fileIDs)
	}

	var ftsMatches []RetrievedDocChunk
	base.Session(&gorm.Session{}).
		Where("to_tsvector('english', fc.content) @@ plainto_tsquery('english', ?)", question).
		Order(gorm.Expr("ts_rank_cd(to_tsvector('english', fc.content), plainto_tsquery('english', ?)) DESC", question)).
		Limit(20).
		Find(&ftsMatches)
	for rank, ch := range ftsMatches {
		chunkByID[ch.ID] = ch
		scoreByID[ch.ID] += 100 - rank
	}

	for _, token := range queryTokens {
		if len(token) < 3 {
			continue
		}
		var tokenMatches []RetrievedDocChunk
		base.Session(&gorm.Session{}).
			Where("fc.content ILIKE ?", "%"+token+"%").
			Order("uf.created_at DESC, fc.chunk_index ASC").
			Limit(20).
			Find(&tokenMatches)
		for rank, ch := range tokenMatches {
			chunkByID[ch.ID] = ch
			scoreByID[ch.ID] += 25 - minInt(rank, 20)
		}
	}

	if len(chunkByID) == 0 {
		var fallback []RetrievedDocChunk
		base.Session(&gorm.Session{}).
			Order("uf.created_at DESC, uf.original_name ASC, fc.chunk_index ASC").
			Limit(limit).
			Find(&fallback)
		for rank, ch := range fallback {
			chunkByID[ch.ID] = ch
			scoreByID[ch.ID] = 1 - rank
		}
	}

	chunks := make([]RetrievedDocChunk, 0, len(chunkByID))
	for _, ch := range chunkByID {
		tokenOverlap := overlapScore(queryTokens, tokenizeForSearch(ch.Content))
		scoreByID[ch.ID] += tokenOverlap * 10
		chunks = append(chunks, ch)
	}

	sort.SliceStable(chunks, func(i, j int) bool {
		if scoreByID[chunks[i].ID] == scoreByID[chunks[j].ID] {
			if chunks[i].FileName == chunks[j].FileName {
				return chunks[i].ChunkIndex < chunks[j].ChunkIndex
			}
			return chunks[i].FileName < chunks[j].FileName
		}
		return scoreByID[chunks[i].ID] > scoreByID[chunks[j].ID]
	})
	if len(chunks) > limit {
		chunks = chunks[:limit]
	}

	if len(chunks) == 0 {
		return "No uploaded document content is available."
	}

	var sb strings.Builder
	for _, ch := range chunks {
		sb.WriteString(fmt.Sprintf("\n\n===== DOCUMENT: %s | Chunk %d | Score %d =====\n\n",
			ch.FileName, ch.ChunkIndex, scoreByID[ch.ID]))
		sb.WriteString(ch.Content)
		sb.WriteString("\n\n")
	}

	context := strings.TrimSpace(sb.String())
	log.Printf("[AI] Hybrid doc context loaded | top_chunks=%d | chars=%d", len(chunks), len(context))
	return context
}

func tokenizeForSearch(s string) []string {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	stop := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "this": {}, "that": {},
		"what": {}, "which": {}, "where": {}, "when": {}, "between": {}, "relation": {},
		"any": {}, "are": {}, "is": {}, "to": {}, "of": {}, "in": {}, "on": {},
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		if len(p) < 2 {
			continue
		}
		if _, ok := stop[p]; ok {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func overlapScore(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	bset := map[string]struct{}{}
	for _, token := range b {
		bset[token] = struct{}{}
	}
	score := 0
	for _, token := range a {
		if _, ok := bset[token]; ok {
			score++
		}
	}
	return score
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
			Content: `You are a PostgreSQL expert. Output ONLY a single read-only SQL query — no explanation, no markdown, no text before or after.
You may ONLY use SELECT, or WITH ... SELECT (read-only CTEs). One statement. No INSERT, UPDATE, DELETE, MERGE, COPY, TRUNCATE, DDL, or any write.
No SELECT INTO, CREATE TABLE AS, or changes to data/schema. Start with SELECT or WITH. Always use to_date='9999-01-01' for current records in dept_emp, titles, salaries tables.
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
			{Role: "system", Content: "Output ONLY a single read-only PostgreSQL SELECT (or WITH ... SELECT). No INSERT/UPDATE/DELETE/DDL. No explanation. Start with SELECT or WITH."},
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
			{Role: "system", Content: "Fix the PostgreSQL SQL error. Output ONLY a single read-only SELECT or WITH ... SELECT. No INSERT/UPDATE/DELETE/DDL. No explanation."},
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

	// ── Step 3: Hybrid retrieval over ready file chunks (if include_docs) ─────
	var docContext string
	if req.IncludeDocs {
		t3 := time.Now()
		docContext = h.loadHybridDocumentContext(req.Question, req.FileIDs, 5)
		log.Printf("[AGENT] Hybrid doc retrieval | %.2fs | chars=%d", time.Since(t3).Seconds(), len(docContext))
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

	// 4. Read-only validation (SELECT/WITH only; no DML/DDL; single statement)
	if readOnlySQLViolations(raw) != "" {
		return ""
	}

	return raw
}

// readOnlySQLForbidden: uppercase substrings with leading/trailing space padding after normalization.
// Used with a space-padded query string so we match whole-token commands, not identifiers.
var readOnlySQLForbidden = []string{
	" INSERT ",
	" UPDATE ",
	" DELETE ",
	" MERGE ",
	" TRUNCATE ",
	" DROP ",
	" ALTER ",
	" CREATE ",
	" REPLACE ",
	" GRANT ",
	" REVOKE ",
	" COPY ", // COPY TO/FROM
	" VACUUM",
	" REINDEX",
	" CALL ",
	" EXECUTE ",
	" DO ", // DO $$
	" INTO ", // SELECT INTO (INSERT INTO is blocked by INSERT)
}

// readOnlySQLViolations returns a non-empty reason if s is not a single read-only SELECT/WITH.
func readOnlySQLViolations(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "empty"
	}
	// Strip a single trailing semicolon (Postgres allows it)
	if strings.HasSuffix(s, ";") {
		s = strings.TrimSpace(s[:len(s)-1])
	}
	if s == "" {
		return "empty"
	}
	parts := strings.Split(s, ";")
	var chunks []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			chunks = append(chunks, t)
		}
	}
	if len(chunks) > 1 {
		return "multi"
	}
	if len(chunks) == 0 {
		return "empty"
	}
	s = chunks[0]
	trimmed := strings.TrimSpace(s)
	upper0 := strings.ToUpper(trimmed)
	// Reject copy/export commands that are not SELECT-shaped
	if strings.HasPrefix(upper0, "COPY") {
		return "copy"
	}
	if !strings.HasPrefix(upper0, "SELECT") && !strings.HasPrefix(upper0, "WITH") {
		return "prefix"
	}
	norm := " " + strings.ToUpper(s) + " "
	norm = strings.ReplaceAll(norm, "\n", " ")
	norm = strings.ReplaceAll(norm, "\t", " ")
	for _, d := range readOnlySQLForbidden {
		if strings.Contains(norm, d) {
			return "forbidden"
		}
	}
	return ""
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

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisURL, Password: cfg.RedisAuth})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Printf("Redis connection warning: %v (AI job status will be unavailable until Redis is reachable)", err)
	}

	aiWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        cfg.AIRequestsTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	notificationWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        cfg.NotificationsTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}

	h := &AIHandler{
		db:                 db,
		nvidia:             newNvidiaClient(cfg),
		sqlDB:              directDB,
		rdb:                rdb,
		cfg:                cfg,
		aiWriter:           aiWriter,
		notificationWriter: notificationWriter,
	}
	h.startAIWorker(context.Background())

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
		api.POST("/jobs", h.QueueQuery)
		api.GET("/jobs/:id", h.GetJob)
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
