package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

// ─── Config ──────────────────────────────────────────────────────────────────

type Config struct {
	Port                string
	JWTSecret           []byte
	AuthServiceURL      string
	DataServiceURL      string
	FileServiceURL      string
	AIServiceURL        string
	AnalyticsServiceURL string
	AllowedOrigins      []string
}

func loadConfig() *Config {
	origins := strings.Split(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	return &Config{
		Port:                getEnv("PORT", "8080"),
		JWTSecret:           []byte(getEnv("JWT_SECRET", "change-me-in-production")),
		AuthServiceURL:      getEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		DataServiceURL:      getEnv("DATA_SERVICE_URL", "http://localhost:8082"),
		FileServiceURL:      getEnv("FILE_SERVICE_URL", "http://localhost:8083"),
		AIServiceURL:        getEnv("AI_SERVICE_URL", "http://localhost:8084"),
		AnalyticsServiceURL: getEnv("ANALYTICS_SERVICE_URL", "http://localhost:8085"),
		AllowedOrigins:      origins,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var localDevOriginPattern = regexp.MustCompile(`^https?://(localhost|127\.0\.0\.1|0\.0\.0\.0|host\.docker\.internal)(:\d+)?$`)

func isAllowedOrigin(origin string, allowedOriginSet map[string]struct{}) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	if _, ok := allowedOriginSet[origin]; ok {
		return true
	}
	// Keep local development ergonomics for Docker + local dev servers.
	return localDevOriginPattern.MatchString(origin)
}

// ─── Rate Limiter ────────────────────────────────────────────────────────────

type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

func newIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	return &IPRateLimiter{limiters: make(map[string]*rate.Limiter), r: r, b: b}
}

func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()
	l, exists := i.limiters[ip]
	if !exists {
		l = rate.NewLimiter(i.r, i.b)
		i.limiters[ip] = l
	}
	return l
}

// ─── Middleware ───────────────────────────────────────────────────────────────

func rateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.getLimiter(ip).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func jwtMiddleware(secret []byte) gin.HandlerFunc {
	publicPaths := []string{
		"/api/auth/login",
		"/api/auth/callback",
		"/api/auth/dev-login",
		"/api/auth/refresh",
		"/health",
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, p := range publicPaths {
			if strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}

		tokenStr := extractToken(c)
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_id", fmt.Sprintf("%v", claims["user_id"]))
			c.Set("email", fmt.Sprintf("%v", claims["email"]))
			c.Set("role", fmt.Sprintf("%v", claims["role"]))
		}

		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if cookie, err := c.Cookie("portal_token"); err == nil {
		return cookie
	}
	return ""
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("[GW] %s %s %d %v", c.Request.Method, c.Request.URL.Path,
			c.Writer.Status(), time.Since(start))
	}
}

// ─── Reverse Proxy ────────────────────────────────────────────────────────────

func newProxy(target string) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ModifyResponse = func(resp *http.Response) error {
		// CORS is enforced at the gateway. Strip upstream CORS headers to avoid
		// duplicate Access-Control-* values, which browsers treat as CORS errors.
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Expose-Headers")
		resp.Header.Del("Access-Control-Max-Age")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[GW] proxy error to %s: %v", target, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"service temporarily unavailable"}`)
	}
	return proxy, nil
}

func proxyHandler(proxy *httputil.ReverseProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Header.Set("X-User-ID", c.GetString("user_id"))
		c.Request.Header.Set("X-User-Email", c.GetString("email"))
		c.Request.Header.Set("X-User-Role", c.GetString("role"))
		c.Request.Header.Del("Cookie") // don't forward cookies to internal services

		// streaming-friendly: disable buffering for AI SSE responses
		if strings.Contains(c.Request.URL.Path, "/ai/") {
			c.Header("X-Accel-Buffering", "no")
		}

		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()

	authProxy, err := newProxy(cfg.AuthServiceURL)
	if err != nil {
		log.Fatalf("invalid auth service URL: %v", err)
	}
	dataProxy, _ := newProxy(cfg.DataServiceURL)
	fileProxy, _ := newProxy(cfg.FileServiceURL)
	aiProxy, _ := newProxy(cfg.AIServiceURL)
	analyticsProxy, _ := newProxy(cfg.AnalyticsServiceURL)

	limiter := newIPRateLimiter(rate.Every(time.Second), 60) // 60 req/s per IP

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())
	allowedOriginSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o != "" {
			allowedOriginSet[o] = struct{}{}
		}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins: cfg.AllowedOrigins,
		AllowOriginFunc: func(origin string) bool {
			return isAllowedOrigin(origin, allowedOriginSet)
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Requested-With", "Cache-Control"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	r.Use(rateLimitMiddleware(limiter))
	r.Use(jwtMiddleware(cfg.JWTSecret))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "api-gateway",
			"time":    time.Now().UTC(),
		})
	})

	api := r.Group("/api")
	{
		// Auth — public endpoints
		api.Any("/auth/*path", proxyHandler(authProxy))

		// Data — protected
		api.Any("/data/*path", proxyHandler(dataProxy))

		// Files — protected, allow large uploads
		api.Any("/files/*path", proxyHandler(fileProxy))

		// AI — protected, streaming
		api.Any("/ai/*path", proxyHandler(aiProxy))

		// Analytics — protected
		api.Any("/analytics/*path", proxyHandler(analyticsProxy))
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  120 * time.Second, // generous for file uploads
		WriteTimeout: 300 * time.Second, // generous for AI streaming
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("API Gateway listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
