package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ── Config ─────────────────────────────────────────────────────────────────

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
		Port:       getEnv("PORT", "8085"),
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

// ── Handlers ──────────────────────────────────────────────────────────────

type AnalyticsHandler struct {
	db *gorm.DB
}

func NewAnalyticsHandler(db *gorm.DB) *AnalyticsHandler {
	return &AnalyticsHandler{db: db}
}

// GET /health
func (h *AnalyticsHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "analytics-service"})
}

// GET /api/analytics/dashboard
func (h *AnalyticsHandler) GetDashboard(c *gin.Context) {
	// Headcount per department
	type DeptHeadcount struct {
		Department string `json:"department"`
		Count      int64  `json:"count"`
	}
	var headcount []DeptHeadcount
	h.db.Raw(`SELECT d.dept_name AS department, COUNT(de.emp_no) AS count
		FROM departments d
		LEFT JOIN dept_emp de ON d.dept_no=de.dept_no AND de.to_date='9999-01-01'
		GROUP BY d.dept_name ORDER BY count DESC`).Scan(&headcount)

	// Average salary per department
	type DeptSalary struct {
		Department string  `json:"department"`
		AvgSalary  float64 `json:"avg_salary"`
	}
	var salaries []DeptSalary
	h.db.Raw(`SELECT d.dept_name AS department, ROUND(AVG(s.salary)::numeric,2) AS avg_salary
		FROM salaries s
		JOIN dept_emp de ON s.emp_no=de.emp_no AND de.to_date='9999-01-01'
		JOIN departments d ON de.dept_no=d.dept_no
		WHERE s.to_date='9999-01-01'
		GROUP BY d.dept_name ORDER BY avg_salary DESC`).Scan(&salaries)

	// Hiring trend (employees hired per year, last 10 years)
	type HiringTrend struct {
		Year  int   `json:"year"`
		Count int64 `json:"count"`
	}
	var hiringTrend []HiringTrend
	h.db.Raw(`SELECT EXTRACT(YEAR FROM hire_date)::int AS year, COUNT(*) AS count
		FROM employees
		WHERE hire_date >= NOW() - INTERVAL '10 years'
		GROUP BY year ORDER BY year`).Scan(&hiringTrend)

	// Gender distribution
	type GenderDist struct {
		Gender string `json:"gender"`
		Count  int64  `json:"count"`
	}
	var genderDist []GenderDist
	h.db.Raw(`SELECT gender, COUNT(*) AS count FROM employees GROUP BY gender`).Scan(&genderDist)

	// Title distribution (current)
	type TitleDist struct {
		Title string `json:"title"`
		Count int64  `json:"count"`
	}
	var titleDist []TitleDist
	h.db.Raw(`SELECT title, COUNT(*) AS count FROM titles WHERE to_date='9999-01-01' GROUP BY title ORDER BY count DESC`).Scan(&titleDist)

	// Overall stats
	type Stats struct {
		TotalEmployees int64   `json:"total_employees"`
		AvgSalary      float64 `json:"avg_salary"`
		MaxSalary      int     `json:"max_salary"`
	}
	var stats Stats
	h.db.Raw(`SELECT COUNT(DISTINCT e.emp_no) AS total_employees,
		ROUND(AVG(s.salary)::numeric,2) AS avg_salary,
		MAX(s.salary) AS max_salary
		FROM employees e
		LEFT JOIN salaries s ON e.emp_no=s.emp_no AND s.to_date='9999-01-01'`).Scan(&stats)

	c.JSON(http.StatusOK, gin.H{
		"headcount_by_dept": headcount,
		"salary_by_dept":    salaries,
		"hiring_trend":      hiringTrend,
		"gender_dist":       genderDist,
		"title_dist":        titleDist,
		"stats":             stats,
	})
}

// GET /api/analytics/salary-distribution
func (h *AnalyticsHandler) GetSalaryDistribution(c *gin.Context) {
	type Bucket struct {
		Bucket string `json:"bucket"`
		Count  int64  `json:"count"`
	}
	var buckets []Bucket
	h.db.Raw(`SELECT
		CASE
			WHEN salary < 50000  THEN '<50k'
			WHEN salary < 70000  THEN '50k-70k'
			WHEN salary < 90000  THEN '70k-90k'
			WHEN salary < 110000 THEN '90k-110k'
			WHEN salary < 130000 THEN '110k-130k'
			ELSE '>130k'
		END AS bucket,
		COUNT(*) AS count
		FROM salaries WHERE to_date='9999-01-01'
		GROUP BY bucket ORDER BY MIN(salary)`).Scan(&buckets)

	c.JSON(http.StatusOK, gin.H{"data": buckets})
}

// GET /api/analytics/tenure
func (h *AnalyticsHandler) GetTenure(c *gin.Context) {
	type TenureBucket struct {
		YearsRange string `json:"years_range"`
		Count      int64  `json:"count"`
	}
	var tenure []TenureBucket
	h.db.Raw(`SELECT
		CASE
			WHEN EXTRACT(YEAR FROM AGE(NOW(), hire_date)) < 5  THEN '0-5 years'
			WHEN EXTRACT(YEAR FROM AGE(NOW(), hire_date)) < 10 THEN '5-10 years'
			WHEN EXTRACT(YEAR FROM AGE(NOW(), hire_date)) < 20 THEN '10-20 years'
			ELSE '20+ years'
		END AS years_range,
		COUNT(*) AS count
		FROM employees
		GROUP BY years_range ORDER BY MIN(hire_date)`).Scan(&tenure)

	c.JSON(http.StatusOK, gin.H{"data": tenure})
}

// GET /api/analytics/reports  (saved reports list)
func (h *AnalyticsHandler) GetReports(c *gin.Context) {
	type Report struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		ReportType  string    `json:"report_type"`
		Status      string    `json:"status"`
		GeneratedAt *time.Time `json:"generated_at"`
		CreatedAt   time.Time `json:"created_at"`
	}
	var reports []Report
	h.db.Raw(`SELECT id, title, report_type, status, generated_at, created_at
		FROM analytics_reports ORDER BY created_at DESC LIMIT 50`).Scan(&reports)
	c.JSON(http.StatusOK, gin.H{"data": reports})
}

// GET /api/analytics/top-earners — top 10 highest paid employees
func (h *AnalyticsHandler) GetTopEarners(c *gin.Context) {
	type TopEarner struct {
		EmpNo      int    `json:"emp_no"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		Department string `json:"department"`
		Title      string `json:"title"`
		Salary     int    `json:"salary"`
	}
	var earners []TopEarner
	h.db.Raw(`
		SELECT e.emp_no, e.first_name, e.last_name,
		       d.dept_name AS department, t.title, s.salary
		FROM employees e
		JOIN salaries   s  ON e.emp_no = s.emp_no  AND s.to_date  = '9999-01-01'
		JOIN dept_emp   de ON e.emp_no = de.emp_no AND de.to_date = '9999-01-01'
		JOIN departments d ON de.dept_no = d.dept_no
		JOIN titles     t  ON e.emp_no = t.emp_no  AND t.to_date  = '9999-01-01'
		ORDER BY s.salary DESC
		LIMIT 10
	`).Scan(&earners)
	c.JSON(http.StatusOK, gin.H{"data": earners})
}

// GET /api/analytics/salary-by-gender
func (h *AnalyticsHandler) GetSalaryByGender(c *gin.Context) {
	type GenderSalary struct {
		Gender    string  `json:"gender"`
		Count     int64   `json:"count"`
		AvgSalary float64 `json:"avg_salary"`
		MaxSalary int     `json:"max_salary"`
		MinSalary int     `json:"min_salary"`
	}
	var data []GenderSalary
	h.db.Raw(`
		SELECT e.gender,
		       COUNT(DISTINCT e.emp_no) AS count,
		       ROUND(AVG(s.salary)::numeric, 2) AS avg_salary,
		       MAX(s.salary) AS max_salary,
		       MIN(s.salary) AS min_salary
		FROM employees e
		JOIN salaries s ON e.emp_no = s.emp_no AND s.to_date = '9999-01-01'
		GROUP BY e.gender ORDER BY e.gender
	`).Scan(&data)
	c.JSON(http.StatusOK, gin.H{"data": data})
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

	h := NewAnalyticsHandler(db)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:    []string{"Authorization", "Content-Type", "X-User-ID", "X-User-Email", "X-User-Role"},
	}))

	r.GET("/health", h.Health)

	api := r.Group("/api/analytics")
	api.GET("/dashboard",           h.GetDashboard)
	api.GET("/salary-distribution", h.GetSalaryDistribution)
	api.GET("/tenure",              h.GetTenure)
	api.GET("/reports",             h.GetReports)
	api.GET("/top-earners",         h.GetTopEarners)
	api.GET("/salary-by-gender",    h.GetSalaryByGender)

	log.Printf("Analytics service running on :%s", cfg.Port)
	log.Fatal(r.Run(":" + cfg.Port))
}
