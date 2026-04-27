package main

import (
	"fmt"
	"io"
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

// ─── GCP Identity Token (Cloud Run service-to-service auth) ──────────────────
// Cloud Run internal services require a Google-signed identity token even when
// invoked from within the same VPC. Without it every proxy call returns 403.

type idTokenEntry struct {
	token  string
	expiry time.Time
}

var (
	idTokenMu    sync.Mutex
	idTokenStore = make(map[string]idTokenEntry)
)

// fetchGCPIdentityToken fetches a Google identity token for the given audience
// from the GCE/Cloud Run metadata server. Returns "" when running locally.
// Tokens are cached for 45 minutes (Cloud Run tokens last 1 hour).
func fetchGCPIdentityToken(audience string) string {
	idTokenMu.Lock()
	entry, ok := idTokenStore[audience]
	idTokenMu.Unlock()
	if ok && time.Now().Before(entry.expiry) {
		return entry.token
	}

	metaURL := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=" +
		url.QueryEscape(audience) + "&format=full"
	req, err := http.NewRequest("GET", metaURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("[GW] identity token unavailable for %s (local dev?): %v", audience, err)
		return ""
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	token := strings.TrimSpace(string(b))

	idTokenMu.Lock()
	idTokenStore[audience] = idTokenEntry{token: token, expiry: time.Now().Add(45 * time.Minute)}
	idTokenMu.Unlock()
	return token
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
		"/api/auth/session",
		"/api/auth/dev-login",
		"/api/auth/auth0-exchange",
		"/api/auth/refresh",
		"/api/auth/logout-token",
		"/api/auth/logout",
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
	// Audience for the identity token is the scheme+host of the target service.
	audience := u.Scheme + "://" + u.Host

	proxy := httputil.NewSingleHostReverseProxy(u)
	defaultDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		defaultDirector(req)

		// Cloud Run routes requests by Host. Without this, the gateway can proxy
		// to an internal service URL while still presenting the gateway host,
		// which loops the request back into the gateway.
		req.Host = u.Host
		req.Header.Set("X-Forwarded-Host", originalHost)
		req.Header.Set("X-Forwarded-Proto", "https")

		// Cloud Run internal services require a Google identity token for
		// service-to-service authentication (IAM roles/run.invoker).
		// User JWT is NOT forwarded — user context travels via X-User-* headers
		// set by the jwtMiddleware → proxyHandler chain above.
		if token := fetchGCPIdentityToken(audience); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			// Local dev: strip the user JWT so internal services don't try to
			// validate it as an identity token. User info is in X-User-* headers.
			req.Header.Del("Authorization")
		}
	}
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
		c.Request.Header.Del("Origin") // gateway owns CORS; internal services should not reject browser origins
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
