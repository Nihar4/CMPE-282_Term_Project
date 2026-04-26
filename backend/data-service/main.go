package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	Port       string
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
}

func loadConfig() Config {
	return Config{
		Port:       getEnv("PORT", "8082"),
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBName:     getEnv("DB_NAME", "enterprise_portal"),
		DBUser:     getEnv("DB_USER", "portal_user"),
		DBPassword: getEnv("DB_PASSWORD", "portal_password"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── test_db Models ─────────────────────────────────────────────────────────

type Department struct {
	DeptNo   string `gorm:"primaryKey;column:dept_no" json:"dept_no"`
	DeptName string `gorm:"column:dept_name"          json:"dept_name"`
}

func (Department) TableName() string { return "departments" }

type Employee struct {
	EmpNo     int       `gorm:"primaryKey;column:emp_no" json:"emp_no"`
	BirthDate time.Time `gorm:"column:birth_date"        json:"birth_date"`
	FirstName string    `gorm:"column:first_name"        json:"first_name"`
	LastName  string    `gorm:"column:last_name"         json:"last_name"`
	Gender    string    `gorm:"column:gender"            json:"gender"`
	HireDate  time.Time `gorm:"column:hire_date"         json:"hire_date"`
}

func (Employee) TableName() string { return "employees" }

type Title struct {
	EmpNo    int        `gorm:"primaryKey;column:emp_no"    json:"emp_no"`
	Title    string     `gorm:"primaryKey;column:title"     json:"title"`
	FromDate time.Time  `gorm:"primaryKey;column:from_date" json:"from_date"`
	ToDate   *time.Time `gorm:"column:to_date"              json:"to_date"`
}

func (Title) TableName() string { return "titles" }

type Salary struct {
	EmpNo    int       `gorm:"primaryKey;column:emp_no"    json:"emp_no"`
	Amount   int       `gorm:"column:salary"               json:"salary"`
	FromDate time.Time `gorm:"primaryKey;column:from_date" json:"from_date"`
	ToDate   time.Time `gorm:"column:to_date"              json:"to_date"`
}

func (Salary) TableName() string { return "salaries" }

type DeptEmp struct {
	EmpNo    int       `gorm:"primaryKey;column:emp_no"  json:"emp_no"`
	DeptNo   string    `gorm:"primaryKey;column:dept_no" json:"dept_no"`
	FromDate time.Time `gorm:"column:from_date"          json:"from_date"`
	ToDate   time.Time `gorm:"column:to_date"            json:"to_date"`
}

func (DeptEmp) TableName() string { return "dept_emp" }

type DeptManager struct {
	EmpNo    int       `gorm:"primaryKey;column:emp_no"  json:"emp_no"`
	DeptNo   string    `gorm:"primaryKey;column:dept_no" json:"dept_no"`
	FromDate time.Time `gorm:"column:from_date"          json:"from_date"`
	ToDate   time.Time `gorm:"column:to_date"            json:"to_date"`
}

func (DeptManager) TableName() string { return "dept_manager" }

// ── Rich response types ────────────────────────────────────────────────────

type EmployeeDetail struct {
	EmpNo      int    `json:"emp_no"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Gender     string `json:"gender"`
	HireDate   string `json:"hire_date"`
	BirthDate  string `json:"birth_date"`
	Department string `json:"department"`
	DeptNo     string `json:"dept_no"`
	Title      string `json:"title"`
	Salary     int    `json:"salary"`
}

type DataOverview struct {
	TotalEmployees  int64   `json:"total_employees"`
	TotalDepts      int64   `json:"total_departments"`
	AvgSalary       float64 `json:"avg_salary"`
	MaxSalary       int     `json:"max_salary"`
	MinSalary       int     `json:"min_salary"`
	MaleCount       int64   `json:"male_count"`
	FemaleCount     int64   `json:"female_count"`
	MostCommonTitle string  `json:"most_common_title"`
}

type Pagination struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
	Pages int   `json:"pages"`
}

// ── Handlers ──────────────────────────────────────────────────────────────

type DataHandler struct {
	db *gorm.DB
}

func NewDataHandler(db *gorm.DB) *DataHandler {
	return &DataHandler{db: db}
}

// GET /health
func (h *DataHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "data-service"})
}

// GET /api/data/overview
func (h *DataHandler) GetOverview(c *gin.Context) {
	var ov DataOverview

	h.db.Model(&Employee{}).Count(&ov.TotalEmployees)
	h.db.Model(&Department{}).Count(&ov.TotalDepts)
	h.db.Model(&Employee{}).Where("gender = 'M'").Count(&ov.MaleCount)
	h.db.Model(&Employee{}).Where("gender = 'F'").Count(&ov.FemaleCount)

	type Stats struct {
		Avg float64
		Max int
		Min int
	}
	var s Stats
	h.db.Raw(`SELECT COALESCE(AVG(salary),0)::float AS avg, COALESCE(MAX(salary),0) AS max, COALESCE(MIN(salary),0) AS min
		FROM salaries WHERE to_date='9999-01-01'`).Scan(&s)
	ov.AvgSalary = math.Round(s.Avg*100) / 100
	ov.MaxSalary = s.Max
	ov.MinSalary = s.Min

	type TC struct {
		Title string
		Count int64
	}
	var tc TC
	h.db.Raw(`SELECT title, COUNT(*) AS count FROM titles WHERE to_date='9999-01-01' GROUP BY title ORDER BY count DESC LIMIT 1`).Scan(&tc)
	ov.MostCommonTitle = tc.Title

	c.JSON(http.StatusOK, gin.H{"data": ov})
}

// GET /api/data/departments
func (h *DataHandler) GetDepartments(c *gin.Context) {
	var depts []Department
	if err := h.db.Order("dept_no").Find(&depts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type DeptResult struct {
		Department
		EmployeeCount int    `json:"employee_count"`
		Manager       string `json:"current_manager"`
	}

	out := make([]DeptResult, 0, len(depts))
	for _, d := range depts {
		var cnt int64
		h.db.Raw(`SELECT COUNT(*) FROM dept_emp WHERE dept_no=? AND to_date='9999-01-01'`, d.DeptNo).Scan(&cnt)
		var mgr string
		h.db.Raw(`SELECT CONCAT(e.first_name,' ',e.last_name) FROM dept_manager dm
			JOIN employees e ON dm.emp_no=e.emp_no WHERE dm.dept_no=? AND dm.to_date='9999-01-01' LIMIT 1`, d.DeptNo).Scan(&mgr)
		out = append(out, DeptResult{d, int(cnt), mgr})
	}

	c.JSON(http.StatusOK, gin.H{"data": out, "total": len(out)})
}

// GET /api/data/employees
func (h *DataHandler) GetEmployees(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")
	dept := c.Query("dept")
	gender := c.Query("gender")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	where := "WHERE 1=1"
	var args []interface{}
	var countArgs []interface{}
	idx := 1

	if search != "" {
		where += fmt.Sprintf(` AND (LOWER(e.first_name) LIKE $%d OR LOWER(e.last_name) LIKE $%d)`, idx, idx+1)
		pat := "%" + strings.ToLower(search) + "%"
		args = append(args, pat, pat)
		countArgs = append(countArgs, pat, pat)
		idx += 2
	}
	if dept != "" {
		where += fmt.Sprintf(` AND de.dept_no=$%d`, idx)
		args = append(args, dept)
		countArgs = append(countArgs, dept)
		idx++
	}
	if gender != "" {
		where += fmt.Sprintf(` AND e.gender=$%d`, idx)
		args = append(args, gender)
		countArgs = append(countArgs, gender)
		idx++
	}

	countQ := `SELECT COUNT(*) FROM employees e
		LEFT JOIN dept_emp de ON e.emp_no=de.emp_no AND de.to_date='9999-01-01' ` + where
	var total int64
	h.db.Raw(countQ, countArgs...).Scan(&total)

	mainQ := `SELECT e.emp_no, e.first_name, e.last_name, e.gender,
		TO_CHAR(e.hire_date,'YYYY-MM-DD') AS hire_date,
		TO_CHAR(e.birth_date,'YYYY-MM-DD') AS birth_date,
		COALESCE(d.dept_name,'') AS department, COALESCE(de.dept_no,'') AS dept_no,
		COALESCE(t.title,'') AS title, COALESCE(s.salary,0) AS salary
		FROM employees e
		LEFT JOIN dept_emp de ON e.emp_no=de.emp_no AND de.to_date='9999-01-01'
		LEFT JOIN departments d ON de.dept_no=d.dept_no
		LEFT JOIN titles t ON e.emp_no=t.emp_no AND t.to_date='9999-01-01'
		LEFT JOIN salaries s ON e.emp_no=s.emp_no AND s.to_date='9999-01-01' ` +
		where + fmt.Sprintf(` ORDER BY e.emp_no LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, limit, offset)

	var employees []EmployeeDetail
	h.db.Raw(mainQ, args...).Scan(&employees)

	c.JSON(http.StatusOK, gin.H{
		"data": employees,
		"pagination": Pagination{
			Page:  page,
			Limit: limit,
			Total: total,
			Pages: int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

// GET /api/data/employees/:id
func (h *DataHandler) GetEmployee(c *gin.Context) {
	empNo := c.Param("id")

	var emp EmployeeDetail
	h.db.Raw(`SELECT e.emp_no, e.first_name, e.last_name, e.gender,
		TO_CHAR(e.hire_date,'YYYY-MM-DD') AS hire_date,
		TO_CHAR(e.birth_date,'YYYY-MM-DD') AS birth_date,
		COALESCE(d.dept_name,'') AS department, COALESCE(de.dept_no,'') AS dept_no,
		COALESCE(t.title,'') AS title, COALESCE(s.salary,0) AS salary
		FROM employees e
		LEFT JOIN dept_emp de ON e.emp_no=de.emp_no AND de.to_date='9999-01-01'
		LEFT JOIN departments d ON de.dept_no=d.dept_no
		LEFT JOIN titles t ON e.emp_no=t.emp_no AND t.to_date='9999-01-01'
		LEFT JOIN salaries s ON e.emp_no=s.emp_no AND s.to_date='9999-01-01'
		WHERE e.emp_no=$1`, empNo).Scan(&emp)

	if emp.EmpNo == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found"})
		return
	}

	var salaryHistory []Salary
	h.db.Where("emp_no = ?", empNo).Order("from_date").Find(&salaryHistory)

	var titleHistory []Title
	h.db.Where("emp_no = ?", empNo).Order("from_date").Find(&titleHistory)

	c.JSON(http.StatusOK, gin.H{
		"data":           emp,
		"salary_history": salaryHistory,
		"title_history":  titleHistory,
	})
}

// GET /api/data/salaries
func (h *DataHandler) GetSalaries(c *gin.Context) {
	dept := c.Query("dept")

	type SalaryStat struct {
		Department string  `json:"department"`
		DeptNo     string  `json:"dept_no"`
		AvgSalary  float64 `json:"avg_salary"`
		MaxSalary  int     `json:"max_salary"`
		MinSalary  int     `json:"min_salary"`
		Count      int64   `json:"employee_count"`
	}

	query := `SELECT d.dept_name AS department, d.dept_no,
		ROUND(AVG(s.salary)::numeric,2) AS avg_salary,
		MAX(s.salary) AS max_salary, MIN(s.salary) AS min_salary,
		COUNT(DISTINCT e.emp_no) AS count
		FROM salaries s
		JOIN employees e ON s.emp_no=e.emp_no
		JOIN dept_emp de ON e.emp_no=de.emp_no AND de.to_date='9999-01-01'
		JOIN departments d ON de.dept_no=d.dept_no
		WHERE s.to_date='9999-01-01'`

	var args []interface{}
	if dept != "" {
		query += ` AND d.dept_no=$1`
		args = append(args, dept)
	}
	query += ` GROUP BY d.dept_name, d.dept_no ORDER BY avg_salary DESC`

	var stats []SalaryStat
	h.db.Raw(query, args...).Scan(&stats)

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GET /api/data/titles
func (h *DataHandler) GetTitles(c *gin.Context) {
	type TitleStat struct {
		Title string `json:"title"`
		Count int64  `json:"count"`
	}
	var stats []TitleStat
	h.db.Raw(`SELECT title, COUNT(*) AS count FROM titles WHERE to_date='9999-01-01' GROUP BY title ORDER BY count DESC`).Scan(&stats)
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GET /api/data/search
func (h *DataHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query required"})
		return
	}
	pat := "%" + strings.ToLower(q) + "%"

	var employees []EmployeeDetail
	h.db.Raw(`SELECT e.emp_no, e.first_name, e.last_name, e.gender,
		TO_CHAR(e.hire_date,'YYYY-MM-DD') AS hire_date,
		TO_CHAR(e.birth_date,'YYYY-MM-DD') AS birth_date,
		COALESCE(d.dept_name,'') AS department, COALESCE(de.dept_no,'') AS dept_no,
		COALESCE(t.title,'') AS title, COALESCE(s.salary,0) AS salary
		FROM employees e
		LEFT JOIN dept_emp de ON e.emp_no=de.emp_no AND de.to_date='9999-01-01'
		LEFT JOIN departments d ON de.dept_no=d.dept_no
		LEFT JOIN titles t ON e.emp_no=t.emp_no AND t.to_date='9999-01-01'
		LEFT JOIN salaries s ON e.emp_no=s.emp_no AND s.to_date='9999-01-01'
		WHERE LOWER(e.first_name) LIKE $1 OR LOWER(e.last_name) LIKE $2
		      OR LOWER(d.dept_name) LIKE $3 OR LOWER(t.title) LIKE $4
		ORDER BY e.emp_no LIMIT 50`, pat, pat, pat, pat).Scan(&employees)

	c.JSON(http.StatusOK, gin.H{"data": employees, "query": q, "count": len(employees)})
}

// ── Notifications ─────────────────────────────────────────────────────────────

type Notification struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`    // file_ready | file_failed | ai_query | system
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Icon      string    `json:"icon"`    // file | ai | system | warning
	CreatedAt time.Time `json:"created_at"`
}

// GET /api/data/notifications
func (h *DataHandler) GetNotifications(c *gin.Context) {
	var notifs []Notification

	// ── 1. File upload events ────────────────────────────────────────────────
	type FileRow struct {
		ID           string    `gorm:"column:id"`
		OriginalName string    `gorm:"column:original_name"`
		FileType     string    `gorm:"column:file_type"`
		Status       string    `gorm:"column:status"`
		CreatedAt    time.Time `gorm:"column:created_at"`
	}
	var files []FileRow
	h.db.Raw(`SELECT id, original_name, file_type, status, created_at
		FROM uploaded_files ORDER BY created_at DESC LIMIT 10`).Scan(&files)

	for _, f := range files {
		n := Notification{
			ID:        "file-" + f.ID,
			CreatedAt: f.CreatedAt,
		}
		ext := strings.ToUpper(f.FileType)
		switch f.Status {
		case "ready":
			n.Type = "file_ready"
			n.Icon = "file"
			n.Title = ext + " file ready"
			n.Body = "\"" + f.OriginalName + "\" has been processed and is available for AI queries"
		case "failed":
			n.Type = "file_failed"
			n.Icon = "warning"
			n.Title = "File processing failed"
			n.Body = "\"" + f.OriginalName + "\" could not be processed — please re-upload"
		default:
			n.Type = "file_processing"
			n.Icon = "file"
			n.Title = "File processing"
			n.Body = "\"" + f.OriginalName + "\" is being processed..."
		}
		notifs = append(notifs, n)
	}

	// ── 2. AI query events ───────────────────────────────────────────────────
	type QueryRow struct {
		ID          string    `gorm:"column:id"`
		QueryText   string    `gorm:"column:query_text"`
		Status      string    `gorm:"column:status"`
		ResultCount int       `gorm:"column:result_count"`
		CreatedAt   time.Time `gorm:"column:created_at"`
	}
	var queries []QueryRow
	h.db.Raw(`SELECT id, query_text, status, result_count, created_at
		FROM query_history ORDER BY created_at DESC LIMIT 8`).Scan(&queries)

	for _, q := range queries {
		text := q.QueryText
		if len(text) > 60 {
			text = text[:57] + "..."
		}
		n := Notification{
			ID:        "ai-" + q.ID,
			CreatedAt: q.CreatedAt,
			Icon:      "ai",
		}
		if q.Status == "success" {
			n.Type = "ai_query"
			n.Title = "AI query completed"
			n.Body = "\"" + text + "\" — " + strconv.Itoa(q.ResultCount) + " result(s) returned"
		} else {
			n.Type = "ai_error"
			n.Icon = "warning"
			n.Title = "AI query failed"
			n.Body = "\"" + text + "\" could not be answered"
		}
		notifs = append(notifs, n)
	}

	// ── 3. System notification ───────────────────────────────────────────────
	var empCount int64
	var deptCount int64
	h.db.Raw(`SELECT COUNT(*) FROM employees`).Scan(&empCount)
	h.db.Raw(`SELECT COUNT(*) FROM departments`).Scan(&deptCount)

	notifs = append(notifs, Notification{
		ID:        "system-db-ready",
		Type:      "system",
		Icon:      "system",
		Title:     "Dataset ready",
		Body:      fmt.Sprintf("test_db loaded: %d employees across %d departments — all analytics available", empCount, deptCount),
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	// ── Sort by created_at desc ──────────────────────────────────────────────
	for i := 0; i < len(notifs)-1; i++ {
		for j := i + 1; j < len(notifs); j++ {
			if notifs[j].CreatedAt.After(notifs[i].CreatedAt) {
				notifs[i], notifs[j] = notifs[j], notifs[i]
			}
		}
	}

	// Cap at 20
	if len(notifs) > 20 {
		notifs = notifs[:20]
	}

	c.JSON(http.StatusOK, gin.H{"notifications": notifs, "total": len(notifs)})
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		log.Fatalf("DB connect failed: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(time.Hour)

	h := NewDataHandler(db)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Authorization", "Content-Type", "X-User-ID", "X-User-Email", "X-User-Role"},
	}))

	r.GET("/health", h.Health)

	api := r.Group("/api/data")
	api.GET("/overview",      h.GetOverview)
	api.GET("/departments",   h.GetDepartments)
	api.GET("/employees",     h.GetEmployees)
	api.GET("/employees/:id", h.GetEmployee)
	api.GET("/salaries",      h.GetSalaries)
	api.GET("/titles",        h.GetTitles)
	api.GET("/search",        h.Search)
	api.GET("/notifications", h.GetNotifications)

	log.Printf("Data service running on :%s", cfg.Port)
	log.Fatal(r.Run(":" + cfg.Port))
}
