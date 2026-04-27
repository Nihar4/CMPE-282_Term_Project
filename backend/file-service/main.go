package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	pdf "rsc.io/pdf"
)

// ─── Models ──────────────────────────────────────────────────────────────────

type UploadedFile struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`
	OriginalName string     `json:"original_name"`
	StoredName   string     `json:"stored_name"`
	FileType     string     `json:"file_type"`
	MimeType     string     `json:"mime_type"`
	FileSize     int64      `json:"file_size"`
	StoragePath  string     `json:"storage_path"`
	Status       string     `gorm:"default:processing" json:"status"`
	RowCount     int        `json:"row_count,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	Metadata     string     `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (UploadedFile) TableName() string { return "uploaded_files" }

type FileChunk struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FileID     uuid.UUID `gorm:"type:uuid;index" json:"file_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	Metadata   string    `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

func (FileChunk) TableName() string { return "file_chunks" }

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
	Port             string
	DBDSN            string
	UploadDir        string
	MaxUploadBytes   int64
	GCSBucket        string
	GCPProject       string
	UseLocalStorage  bool
	KafkaBrokers     []string
	NotificationsTopic string
	ParserServiceURL string
}

func loadConfig() *Config {
	brokers := strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	return &Config{
		Port: getEnv("PORT", "8083"),
		DBDSN: fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
			getEnv("DB_HOST", "localhost"),
			getEnv("DB_PORT", "5432"),
			getEnv("DB_USER", "portal_user"),
			getEnv("DB_PASSWORD", "portal_password"),
			getEnv("DB_NAME", "enterprise_portal"),
		),
		UploadDir:        getEnv("UPLOAD_DIR", "./uploads"),
		MaxUploadBytes:   50 * 1024 * 1024, // 50 MB
		GCSBucket:        getEnv("GCS_BUCKET", "enterprise-portal-files"),
		GCPProject:       getEnv("GCP_PROJECT_ID", ""),
		UseLocalStorage:  getEnv("USE_LOCAL_STORAGE", "true") == "true",
		KafkaBrokers:     brokers,
		NotificationsTopic: getEnv("NOTIFICATIONS_TOPIC", "portal.notifications"),
		ParserServiceURL: getEnv("PARSER_SERVICE_URL", "http://parser-service:8090"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ─── File Parsers ─────────────────────────────────────────────────────────────

func detectFileType(filename string, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return "csv"
	case ".pdf":
		return "pdf"
	case ".docx", ".doc":
		return "docx"
	case ".txt":
		return "txt"
	case ".json":
		return "json"
	case ".xlsx", ".xls":
		return "xlsx"
	default:
		if strings.Contains(mimeType, "csv") {
			return "csv"
		}
		if strings.Contains(mimeType, "pdf") {
			return "pdf"
		}
		return "txt"
	}
}

// parseCSV parses a CSV file and returns (chunks, rowCount, error)
func parseCSV(data []byte) ([]string, int, error) {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("CSV parse error: %w", err)
	}
	if len(records) == 0 {
		return nil, 0, nil
	}

	headers := records[0]
	var chunks []string
	chunkSize := 50 // rows per chunk

	for i := 1; i < len(records); i += chunkSize {
		end := i + chunkSize
		if end > len(records) {
			end = len(records)
		}

		var sb strings.Builder
		sb.WriteString("CSV Data (columns: " + strings.Join(headers, ", ") + ")\n\n")
		for _, row := range records[i:end] {
			for j, cell := range row {
				if j < len(headers) {
					sb.WriteString(headers[j] + ": " + cell + "\n")
				}
			}
			sb.WriteString("---\n")
		}
		chunks = append(chunks, sb.String())
	}

	return chunks, len(records) - 1, nil
}

// parseDocx extracts readable text from a .docx file (which is a ZIP of XML files)
func parseDocx(data []byte) ([]string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("docx: not a valid zip file: %w", err)
	}

	var textBuilder strings.Builder
	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("docx: cannot open document.xml: %w", err)
		}
		defer rc.Close()

		// Strip XML tags to extract plain text
		decoder := xml.NewDecoder(rc)
		inParagraph := false
		for {
			tok, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				localName := strings.ToLower(t.Name.Local)
				if localName == "p" {
					inParagraph = true
				}
			case xml.EndElement:
				localName := strings.ToLower(t.Name.Local)
				if localName == "p" && inParagraph {
					textBuilder.WriteString("\n")
					inParagraph = false
				}
			case xml.CharData:
				text := strings.TrimSpace(string(t))
				if text != "" {
					textBuilder.WriteString(text + " ")
				}
			}
		}
		break
	}

	fullText := strings.TrimSpace(textBuilder.String())
	if fullText == "" {
		return []string{"[Document appears to be empty or uses unsupported formatting]"}, nil
	}

	// Split into ~2000 char chunks
	return parseTXT([]byte(fullText))
}

// parseTXT splits text file into chunks of ~2000 chars
func parseTXT(data []byte) ([]string, error) {
	// Sanitize: replace invalid UTF-8 bytes with empty string instead of failing
	text := strings.ToValidUTF8(string(data), "")
	var chunks []string
	chunkSize := 2000

	scanner := bufio.NewScanner(strings.NewReader(text))
	// Increase token limit to handle long lines extracted from PDFs/Docs.
	scanner.Buffer(make([]byte, 1024), 10*1024*1024)
	var current strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if current.Len()+len(line) > chunkSize && current.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}
		current.WriteString(line + "\n")
	}
	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("text parse error: %w", err)
	}
	return chunks, nil
}

// parsePDF extracts text from a PDF file using raw content extraction
// For production, integrate pdfcpu or unipdf; this handles text-based PDFs
func parsePDF(data []byte) (chunks []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf parser panic: %v", r)
			chunks = nil
		}
	}()

	// First try structured extraction via rsc.io/pdf.
	if reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
		var extracted strings.Builder
		for p := 1; p <= reader.NumPage(); p++ {
			page := reader.Page(p)
			content := page.Content()
			for _, text := range content.Text {
				s := strings.TrimSpace(text.S)
				if s == "" {
					continue
				}
				extracted.WriteString(s)
				extracted.WriteString(" ")
			}
		}
		clean := strings.TrimSpace(compactWhitespace(extracted.String()))
		if clean != "" {
			return parseTXT([]byte(clean))
		}
	}

	// Fallback extraction for malformed/unsupported PDFs:
	// scan stream blocks and keep printable runs.
	content := string(data)
	var extracted strings.Builder
	inStream := false

	for i := 0; i < len(content)-7; i++ {
		if content[i:i+7] == "stream\n" || content[i:i+8] == "stream\r\n" {
			inStream = true
			continue
		}
		if inStream && i < len(content)-9 && (content[i:i+9] == "endstream") {
			inStream = false
			continue
		}
		if inStream {
			ch := rune(content[i])
			if ch >= 32 && ch < 127 {
				extracted.WriteRune(ch)
			} else if ch == '\n' || ch == '\r' {
				extracted.WriteString(" ")
			}
		}
	}

	text := strings.TrimSpace(compactWhitespace(extracted.String()))
	if text == "" {
		text = fmt.Sprintf("[PDF uploaded: %d bytes. Text extraction limited — use a PDF with selectable text for best AI results.]", len(data))
	}

	return parseTXT([]byte(text))
}

func safeParsePDF(data []byte) (chunks []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf parser panic: %v", r)
			chunks = nil
		}
	}()
	return parsePDF(data)
}

func compactWhitespace(s string) string {
	whitespace := regexp.MustCompile(`\s+`)
	return whitespace.ReplaceAllString(strings.TrimSpace(s), " ")
}

type parserServiceResponse struct {
	Chunks   []string `json:"chunks"`
	RowCount int      `json:"row_count"`
}

func (h *FileHandler) parseViaPythonService(record *UploadedFile, data []byte) ([]string, int, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", record.OriginalName)
	if err != nil {
		return nil, 0, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, 0, err
	}
	_ = writer.WriteField("file_type", record.FileType)
	_ = writer.WriteField("mime_type", record.MimeType)
	if err := writer.Close(); err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest(http.MethodPost, h.cfg.ParserServiceURL+"/parse", &body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("python parser error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var parsed parserServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, 0, err
	}
	if len(parsed.Chunks) == 0 {
		return nil, 0, fmt.Errorf("python parser returned no chunks")
	}
	return parsed.Chunks, parsed.RowCount, nil
}

// ─── Handler ──────────────────────────────────────────────────────────────────

type FileHandler struct {
	db                 *gorm.DB
	cfg                *Config
	notificationWriter *kafka.Writer
}

func (h *FileHandler) publishNotification(ctx context.Context, event NotificationEvent) {
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
		log.Printf("could not marshal notification: %v", err)
		return
	}
	if err := h.notificationWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.ID),
		Value: payload,
		Time:  event.CreatedAt,
	}); err != nil {
		log.Printf("Kafka notification publish failed: %v", err)
	}
}

func (h *FileHandler) saveFile(header *multipart.FileHeader, file multipart.File) (string, string, error) {
	storedName := uuid.New().String() + filepath.Ext(header.Filename)
	destPath := filepath.Join(h.cfg.UploadDir, storedName)

	os.MkdirAll(h.cfg.UploadDir, 0755)
	out, err := os.Create(destPath)
	if err != nil {
		return "", "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return "", "", err
	}
	return storedName, destPath, nil
}

// POST /api/files/upload
func (h *FileHandler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.cfg.MaxUploadBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided: " + err.Error()})
		return
	}

	// Files are shared/global — do not link to a specific user (avoids FK constraint issues)
	var userID *uuid.UUID = nil

	mimeType := fileHeader.Header.Get("Content-Type")
	fileType := detectFileType(fileHeader.Filename, mimeType)

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer file.Close()

	storedName, storagePath, err := h.saveFile(fileHeader, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage failed: " + err.Error()})
		return
	}

	record := UploadedFile{
		UserID:       userID,
		OriginalName: fileHeader.Filename,
		StoredName:   storedName,
		FileType:     fileType,
		MimeType:     mimeType,
		FileSize:     fileHeader.Size,
		StoragePath:  storagePath,
		Status:       "processing",
	}
	if err := h.db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Parse and chunk asynchronously
	go h.processFile(&record, storagePath)

	c.JSON(http.StatusCreated, gin.H{
		"file":    record,
		"message": "File uploaded and being processed",
	})
}

func (h *FileHandler) processFile(record *UploadedFile, path string) {
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("file processing panic: %v", r)
			log.Printf("%s for %s", errMsg, record.OriginalName)
			h.db.Model(record).Updates(map[string]interface{}{
				"status": "error", "error_message": errMsg,
			})
			h.publishNotification(context.Background(), NotificationEvent{
				Type:  "file_failed",
				Title: "File processing failed",
				Body:  fmt.Sprintf("%q could not be processed", record.OriginalName),
				Icon:  "warning",
			})
		}
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		h.db.Model(record).Updates(map[string]interface{}{
			"status": "error", "error_message": "failed to read file",
		})
		h.publishNotification(context.Background(), NotificationEvent{
			Type:  "file_failed",
			Title: "File processing failed",
			Body:  fmt.Sprintf("%q could not be read for processing", record.OriginalName),
			Icon:  "warning",
		})
		return
	}

	var chunks []string
	var rowCount int

	chunks, rowCount, err = h.parseViaPythonService(record, data)
	if err != nil {
		log.Printf("python parser failed for %s: %v — falling back to local parser", record.OriginalName, err)
		switch record.FileType {
		case "csv":
			chunks, rowCount, err = parseCSV(data)
			if err != nil {
				h.db.Model(record).Updates(map[string]interface{}{
					"status": "error", "error_message": err.Error(),
				})
				h.publishNotification(context.Background(), NotificationEvent{
					Type:  "file_failed",
					Title: "File processing failed",
					Body:  fmt.Sprintf("%q could not be parsed: %v", record.OriginalName, err),
					Icon:  "warning",
				})
				return
			}
		case "pdf":
			chunks, err = safeParsePDF(data)
			if err != nil {
				chunks = []string{fmt.Sprintf("PDF content (%d bytes)", len(data))}
			}
		case "docx":
			chunks, err = parseDocx(data)
			if err != nil {
				log.Printf("docx parse error for %s: %v — storing as binary placeholder", record.OriginalName, err)
				chunks = []string{fmt.Sprintf("Document: %s (%d bytes) — text extraction failed", record.OriginalName, len(data))}
			}
		default:
			chunks, err = parseTXT(data)
			if err != nil {
				log.Printf("text parse error for %s: %v — using placeholder", record.OriginalName, err)
				chunks = []string{fmt.Sprintf("File: %s (%d bytes)", record.OriginalName, len(data))}
			}
		}
	}

	// Save chunks to DB
	for i, chunk := range chunks {
		h.db.Create(&FileChunk{
			FileID:     record.ID,
			ChunkIndex: i,
			Content:    chunk,
		})
	}

	meta := map[string]interface{}{
		"chunk_count":  len(chunks),
		"processed_at": time.Now().Format(time.RFC3339),
	}
	metaJSON, _ := json.Marshal(meta)

	h.db.Model(record).Updates(map[string]interface{}{
		"status":    "ready",
		"row_count": rowCount,
		"metadata":  string(metaJSON),
	})

	h.publishNotification(context.Background(), NotificationEvent{
		Type:  "file_ready",
		Title: strings.ToUpper(record.FileType) + " file ready",
		Body:  fmt.Sprintf("%q has been processed with %d chunk(s)", record.OriginalName, len(chunks)),
		Icon:  "file",
	})

	log.Printf("Processed file %s: %d chunks", record.OriginalName, len(chunks))
}

// GET /api/files/  — shared library: all users see all files
func (h *FileHandler) ListFiles(c *gin.Context) {
	var files []UploadedFile
	h.db.Model(&UploadedFile{}).Order("created_at desc").Limit(200).Find(&files)
	c.JSON(http.StatusOK, gin.H{"files": files, "count": len(files)})
}

// GET /api/files/:id
func (h *FileHandler) GetFile(c *gin.Context) {
	var f UploadedFile
	if err := h.db.Where("id = ?", c.Param("id")).First(&f).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	c.JSON(http.StatusOK, f)
}

// GET /api/files/:id/chunks
func (h *FileHandler) GetChunks(c *gin.Context) {
	var chunks []FileChunk
	h.db.Where("file_id = ?", c.Param("id")).Order("chunk_index asc").Find(&chunks)
	c.JSON(http.StatusOK, gin.H{"chunks": chunks, "count": len(chunks)})
}

// GET /api/files/search?q=query&file_id=xxx
func (h *FileHandler) SearchChunks(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q required"})
		return
	}

	q := h.db.Model(&FileChunk{})
	if fileID := c.Query("file_id"); fileID != "" {
		q = q.Where("file_id = ?", fileID)
	}

	// Full-text search
	q = q.Where("to_tsvector('english', content) @@ plainto_tsquery('english', ?)", query)

	var chunks []FileChunk
	q.Order("chunk_index").Limit(10).Find(&chunks)

	c.JSON(http.StatusOK, gin.H{"chunks": chunks, "count": len(chunks), "query": query})
}

// DELETE /api/files/:id — any authenticated user can delete any file (shared library)
func (h *FileHandler) DeleteFile(c *gin.Context) {
	var f UploadedFile
	if err := h.db.Where("id = ?", c.Param("id")).First(&f).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	// Delete chunks first (FK constraint)
	h.db.Where("file_id = ?", f.ID).Delete(&FileChunk{})
	h.db.Delete(&f)

	// Remove from local storage
	if h.cfg.UseLocalStorage {
		os.Remove(f.StoragePath)
	}

	h.publishNotification(c.Request.Context(), NotificationEvent{
		Type:  "file_deleted",
		Title: "File deleted",
		Body:  fmt.Sprintf("%q was removed from the document repository", f.OriginalName),
		Icon:  "file",
	})

	c.JSON(http.StatusOK, gin.H{"message": "file deleted", "id": f.ID})
}

// DELETE /api/files — delete all uploaded files and their chunks
func (h *FileHandler) DeleteAllFiles(c *gin.Context) {
	var files []UploadedFile
	if err := h.db.Find(&files).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list files"})
		return
	}

	// Delete chunks then files to satisfy FK constraints.
	if err := h.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&FileChunk{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file chunks"})
		return
	}
	if err := h.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&UploadedFile{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete files"})
		return
	}

	deletedFromDisk := 0
	if h.cfg.UseLocalStorage {
		for _, f := range files {
			if f.StoragePath == "" {
				continue
			}
			if err := os.Remove(f.StoragePath); err == nil {
				deletedFromDisk++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "all files deleted",
		"deleted_file_count": len(files),
		"deleted_local_files": deletedFromDisk,
	})

	h.publishNotification(c.Request.Context(), NotificationEvent{
		Type:  "file_deleted",
		Title: "All files deleted",
		Body:  fmt.Sprintf("%d file(s) were removed from the document repository", len(files)),
		Icon:  "file",
	})
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
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	os.MkdirAll(cfg.UploadDir, 0755)

	notificationWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        cfg.NotificationsTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}

	h := &FileHandler{db: db, cfg: cfg, notificationWriter: notificationWriter}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"*"},
		ExposeHeaders: []string{"Content-Length"},
	}))

	// Increase max multipart memory for large file uploads
	r.MaxMultipartMemory = 50 << 20 // 50 MB

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "file-service"})
	})

	api := r.Group("/api/files")
	{
		api.POST("/upload", h.Upload)
		api.GET("/", h.ListFiles)
		api.GET("", h.ListFiles)
		api.GET("/search", h.SearchChunks)
		api.DELETE("/", h.DeleteAllFiles)
		api.DELETE("", h.DeleteAllFiles)
		api.GET("/:id", h.GetFile)
		api.GET("/:id/chunks", h.GetChunks)
		api.DELETE("/:id", h.DeleteFile)
	}

	log.Printf("File Service listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
