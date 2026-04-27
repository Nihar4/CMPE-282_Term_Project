package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─── Models ──────────────────────────────────────────────────────────────────

type User struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email       string     `gorm:"uniqueIndex;not null" json:"email"`
	Name        string     `gorm:"not null" json:"name"`
	Role        string     `gorm:"default:user" json:"role"`
	OktaID      string     `json:"okta_id,omitempty"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	Department  string     `json:"department,omitempty"`
	IsActive    bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// ─── Config ──────────────────────────────────────────────────────────────────

type Config struct {
	Port                  string
	DBDSN                 string
	RedisURL              string
	RedisAuth             string
	JWTSecret             []byte
	JWTExpiry             time.Duration
	OktaDomain           string
	OktaIssuer           string
	OktaClientID         string
	OktaClientSecret     string
	OktaRedirectURI      string
	OktaLogoutRedirectURI string
	Auth0Domain           string
	Auth0ClientID         string
	Auth0ClientSecret     string
	FrontendURL           string
	DevMode               bool
}

func loadConfig() *Config {
	return &Config{
		Port: getEnv("PORT", "8081"),
		DBDSN: fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
			getEnv("DB_HOST", "localhost"),
			getEnv("DB_PORT", "5432"),
			getEnv("DB_USER", "portal_user"),
			getEnv("DB_PASSWORD", "portal_password"),
			getEnv("DB_NAME", "enterprise_portal"),
		),
		RedisURL:          getEnv("REDIS_URL", "localhost:6379"),
		RedisAuth:         getEnv("REDIS_AUTH", ""),
		JWTSecret:         []byte(getEnv("JWT_SECRET", "change-me-in-production")),
		JWTExpiry:         8 * time.Hour,
		OktaDomain:            getEnv("OKTA_DOMAIN", "dev-example.okta.com"),
		OktaIssuer:            normalizeIssuer(getEnv("OKTA_ISSUER", "")),
		OktaClientID:          getEnv("OKTA_CLIENT_ID", ""),
		OktaClientSecret:      getEnv("OKTA_CLIENT_SECRET", ""),
		OktaRedirectURI:       getEnv("OKTA_REDIRECT_URI", "http://localhost:3000/authorization-code/callback"),
		OktaLogoutRedirectURI: getEnv("OKTA_LOGOUT_REDIRECT_URI", "http://localhost:3000"),
		Auth0Domain:           getEnv("AUTH0_DOMAIN", "dev-xbnsordr5elttyug.us.auth0.com"),
		Auth0ClientID:         getEnv("AUTH0_CLIENT_ID", "x8pFzWFtyYCBXrr6U2NkxABroQqqMvxM"),
		Auth0ClientSecret:     getEnv("AUTH0_CLIENT_SECRET", ""),
		FrontendURL:           strings.TrimRight(getEnv("FRONTEND_URL", "http://localhost:3000"), "/"),
		DevMode:               getEnv("DEV_MODE", "true") == "true",
	}
}

func normalizeIssuer(issuer string) string {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		domain := strings.TrimRight(strings.TrimSpace(getEnv("OKTA_DOMAIN", "dev-example.okta.com")), "/")
		domain = strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
		return "https://" + domain + "/oauth2/default"
	}
	return issuer
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ─── JWT Helpers ─────────────────────────────────────────────────────────────

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

func generateToken(user *User, secret []byte, expiry time.Duration) (string, error) {
	claims := Claims{
		UserID: user.ID.String(),
		Email:  user.Email,
		Role:   user.Role,
		Name:   user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ─── Handlers ────────────────────────────────────────────────────────────────

type AuthHandler struct {
	db    *gorm.DB
	rdb   *redis.Client
	cfg   *Config
}

func redisOpContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// POST /api/auth/dev-login — only available when DEV_MODE=true
func (h *AuthHandler) DevLogin(c *gin.Context) {
	if !h.cfg.DevMode {
		c.JSON(http.StatusForbidden, gin.H{"error": "dev login disabled in production"})
		return
	}

	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// In dev mode: accept any password, auto-create user if not found
	var user User
	result := h.db.Where("email = ?", req.Email).First(&user)
	if result.Error != nil {
		// Auto-create for dev
		role := "user"
		if strings.Contains(req.Email, "admin") {
			role = "admin"
		} else if strings.Contains(req.Email, "analyst") {
			role = "analyst"
		}
		user = User{
			Email:    req.Email,
			Name:     strings.Split(req.Email, "@")[0],
			Role:     role,
			IsActive: true,
		}
		if err := h.db.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			return
		}
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
		return
	}

	now := time.Now()
	h.db.Model(&user).Update("last_login_at", &now)

	accessToken, err := generateToken(&user, h.cfg.JWTSecret, h.cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	refreshToken, _ := generateRefreshToken()
	ctx := context.Background()
	h.rdb.Set(ctx, "refresh:"+refreshToken, user.ID.String(), 7*24*time.Hour)

	c.SetCookie("portal_token", accessToken, int(h.cfg.JWTExpiry.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    int(h.cfg.JWTExpiry.Seconds()),
		"user": gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"name":       user.Name,
			"role":       user.Role,
			"department": user.Department,
			"avatar_url": user.AvatarURL,
		},
	})
}

// GET /api/auth/login — Redirect to Okta
func (h *AuthHandler) OktaLogin(c *gin.Context) {
	if h.cfg.OktaClientID == "" || h.cfg.OktaClientSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Okta OIDC is not configured"})
		return
	}

	state, _ := generateRefreshToken()
	ctx, cancel := redisOpContext()
	defer cancel()
	if err := h.rdb.Set(ctx, "okta_state:"+state, "1", 10*time.Minute).Err(); err != nil {
		log.Printf("Okta login state storage failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "login state store unavailable"})
		return
	}

	authURL := fmt.Sprintf(
		"%s/v1/authorize?response_type=code&client_id=%s&redirect_uri=%s&scope=openid+profile+email&state=%s",
		h.cfg.OktaIssuer,
		url.QueryEscape(h.cfg.OktaClientID),
		url.QueryEscape(h.cfg.OktaRedirectURI),
		state,
	)
	c.Redirect(http.StatusFound, authURL)
}

// GET /api/auth/callback — Okta OAuth callback
func (h *AuthHandler) OktaCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	ctx, cancel := redisOpContext()
	stateValue, err := h.rdb.Get(ctx, "okta_state:"+state).Result()
	cancel()
	if err == redis.Nil || stateValue == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}
	if err != nil {
		log.Printf("Okta callback state lookup failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "login state store unavailable"})
		return
	}
	ctx, cancel = redisOpContext()
	h.rdb.Del(ctx, "okta_state:"+state)
	cancel()

	// Exchange code for tokens on the backend so the client secret never reaches the browser.
	tokenURL := fmt.Sprintf("%s/v1/token", h.cfg.OktaIssuer)
	resp, err := http.PostForm(tokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {h.cfg.OktaRedirectURI},
		"client_id":     {h.cfg.OktaClientID},
		"client_secret": {h.cfg.OktaClientSecret},
	})
	if err != nil || resp.StatusCode != 200 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to exchange code with Okta"})
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	// Get user info from Okta
	userInfoURL := fmt.Sprintf("%s/v1/userinfo", h.cfg.OktaIssuer)
	req, _ := http.NewRequest("GET", userInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	uiResp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to get user info from Okta"})
		return
	}
	defer uiResp.Body.Close()
	body, _ := io.ReadAll(uiResp.Body)

	var oktaUser struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	json.Unmarshal(body, &oktaUser)

	// Upsert user in DB
	var user User
	h.db.Where("okta_id = ?", oktaUser.Sub).Or("email = ?", oktaUser.Email).First(&user)
	if user.ID == uuid.Nil {
		user = User{
			Email:    oktaUser.Email,
			Name:     oktaUser.Name,
			OktaID:   oktaUser.Sub,
			Role:     "user",
			IsActive: true,
		}
		h.db.Create(&user)
	} else {
		now := time.Now()
		h.db.Model(&user).Updates(User{Name: oktaUser.Name, OktaID: oktaUser.Sub, LastLoginAt: &now})
	}

	accessToken, err := generateToken(&user, h.cfg.JWTSecret, h.cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}
	refreshToken, _ := generateRefreshToken()
	ctx, cancel = redisOpContext()
	if err := h.rdb.Set(ctx, "refresh:"+refreshToken, user.ID.String(), 7*24*time.Hour).Err(); err != nil {
		cancel()
		log.Printf("Refresh token storage failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session store unavailable"})
		return
	}
	if tokenResp.IDToken != "" {
		if err := h.rdb.Set(ctx, "okta_id_token:"+refreshToken, tokenResp.IDToken, 7*24*time.Hour).Err(); err != nil {
			cancel()
			log.Printf("Okta ID token storage failed: %v", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session store unavailable"})
			return
		}
	}
	cancel()

	secureCookie := strings.HasPrefix(h.cfg.FrontendURL, "https://")
	if secureCookie {
		c.SetSameSite(http.SameSiteNoneMode)
	} else {
		c.SetSameSite(http.SameSiteLaxMode)
	}
	c.SetCookie("portal_token", accessToken, int(h.cfg.JWTExpiry.Seconds()), "/", "", secureCookie, true)
	c.SetCookie("portal_refresh", refreshToken, int((7*24*time.Hour).Seconds()), "/", "", secureCookie, true)

	handoffToken, _ := generateRefreshToken()
	sessionPayload, err := json.Marshal(gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    int(h.cfg.JWTExpiry.Seconds()),
		"user": gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"name":       user.Name,
			"role":       user.Role,
			"department": user.Department,
			"avatar_url": user.AvatarURL,
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session handoff failed"})
		return
	}
	ctx, cancel = redisOpContext()
	if err := h.rdb.Set(ctx, "login_session:"+handoffToken, sessionPayload, 2*time.Minute).Err(); err != nil {
		cancel()
		log.Printf("Login session handoff storage failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session handoff unavailable"})
		return
	}
	cancel()

	c.Redirect(http.StatusFound, h.cfg.FrontendURL+"/authorization-code/callback?session="+url.QueryEscape(handoffToken))
}

// GET /api/auth/session — Exchanges a one-time Okta callback handoff for a portal JWT.
func (h *AuthHandler) Session(c *gin.Context) {
	sessionID := c.Query("session")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session"})
		return
	}

	ctx, cancel := redisOpContext()
	payload, err := h.rdb.Get(ctx, "login_session:"+sessionID).Result()
	if err == nil {
		h.rdb.Del(ctx, "login_session:"+sessionID)
	}
	cancel()

	if err == redis.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login session expired"})
		return
	}
	if err != nil {
		log.Printf("Login session handoff lookup failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session handoff unavailable"})
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(payload))
}

// POST /api/auth/auth0-exchange — Exchange Auth0 access token for portal JWT
func (h *AuthHandler) Auth0Exchange(c *gin.Context) {
	var req struct {
		AccessToken string `json:"access_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Call Auth0 /userinfo to validate token and get user profile
	userInfoURL := fmt.Sprintf("https://%s/userinfo", h.cfg.Auth0Domain)
	httpReq, _ := http.NewRequest("GET", userInfoURL, nil)
	httpReq.Header.Set("Authorization", "Bearer "+req.AccessToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil || resp.StatusCode != 200 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid Auth0 token"})
		return
	}
	defer resp.Body.Close()

	var auth0User struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	json.NewDecoder(resp.Body).Decode(&auth0User)

	if auth0User.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not get email from Auth0"})
		return
	}

	// Upsert user in our database
	var user User
	result := h.db.Where("okta_id = ? OR email = ?", auth0User.Sub, auth0User.Email).First(&user)
	now := time.Now()
	if result.Error != nil {
		// New user — auto-provision
		role := "user"
		if strings.Contains(auth0User.Email, "admin") {
			role = "admin"
		}
		user = User{
			Email:       auth0User.Email,
			Name:        auth0User.Name,
			OktaID:      auth0User.Sub,
			AvatarURL:   auth0User.Picture,
			Role:        role,
			IsActive:    true,
			LastLoginAt: &now,
		}
		if err := h.db.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user provisioning failed"})
			return
		}
	} else {
		h.db.Model(&user).Updates(User{
			Name:        auth0User.Name,
			OktaID:      auth0User.Sub,
			AvatarURL:   auth0User.Picture,
			LastLoginAt: &now,
		})
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
		return
	}

	// Issue our own portal JWT
	accessToken, err := generateToken(&user, h.cfg.JWTSecret, h.cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	refreshToken, _ := generateRefreshToken()
	h.rdb.Set(context.Background(), "refresh:"+refreshToken, user.ID.String(), 7*24*time.Hour)

	c.SetCookie("portal_token", accessToken, int(h.cfg.JWTExpiry.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    int(h.cfg.JWTExpiry.Seconds()),
		"user": gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"name":       user.Name,
			"role":       user.Role,
			"avatar_url": user.AvatarURL,
		},
	})
}

// GET /api/auth/me — Current user profile
func (h *AuthHandler) Me(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	var user User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"email":        user.Email,
		"name":         user.Name,
		"role":         user.Role,
		"department":   user.Department,
		"avatar_url":   user.AvatarURL,
		"last_login_at": user.LastLoginAt,
		"created_at":   user.CreatedAt,
	})
}

// POST /api/auth/refresh — Refresh access token
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()
	userID := h.rdb.Get(ctx, "refresh:"+req.RefreshToken).Val()
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	var user User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	accessToken, err := generateToken(&user, h.cfg.JWTSecret, h.cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	// Rotate refresh token
	newRefresh, _ := generateRefreshToken()
	h.rdb.Del(ctx, "refresh:"+req.RefreshToken)
	h.rdb.Set(ctx, "refresh:"+newRefresh, userID, 7*24*time.Hour)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": newRefresh,
		"expires_in":    int(h.cfg.JWTExpiry.Seconds()),
	})
}

// POST /api/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	c.ShouldBindJSON(&req)
	refreshToken := req.RefreshToken
	if refreshToken == "" {
		refreshToken, _ = c.Cookie("portal_refresh")
	}
	if refreshToken != "" {
		ctx, cancel := redisOpContext()
		h.rdb.Del(ctx, "refresh:"+refreshToken, "okta_id_token:"+refreshToken)
		cancel()
	}
	secureCookie := strings.HasPrefix(h.cfg.FrontendURL, "https://")
	if secureCookie {
		c.SetSameSite(http.SameSiteNoneMode)
	} else {
		c.SetSameSite(http.SameSiteLaxMode)
	}
	c.SetCookie("portal_token", "", -1, "/", "", secureCookie, true)
	c.SetCookie("portal_refresh", "", -1, "/", "", secureCookie, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

// GET /api/auth/logout — clears the portal session and redirects to Okta logout.
func (h *AuthHandler) LogoutRedirect(c *gin.Context) {
	refreshToken, _ := c.Cookie("portal_refresh")
	var idToken string
	if refreshToken != "" {
		ctx, cancel := redisOpContext()
		idToken = h.rdb.Get(ctx, "okta_id_token:"+refreshToken).Val()
		h.rdb.Del(ctx, "refresh:"+refreshToken, "okta_id_token:"+refreshToken)
		cancel()
	}

	secureCookie := strings.HasPrefix(h.cfg.FrontendURL, "https://")
	if secureCookie {
		c.SetSameSite(http.SameSiteNoneMode)
	} else {
		c.SetSameSite(http.SameSiteLaxMode)
	}
	c.SetCookie("portal_token", "", -1, "/", "", secureCookie, true)
	c.SetCookie("portal_refresh", "", -1, "/", "", secureCookie, true)

	logoutURL := fmt.Sprintf("%s/v1/logout?post_logout_redirect_uri=%s",
		h.cfg.OktaIssuer,
		url.QueryEscape(h.cfg.OktaLogoutRedirectURI),
	)
	if idToken != "" {
		logoutURL += "&id_token_hint=" + url.QueryEscape(idToken)
	}
	c.Redirect(http.StatusFound, logoutURL)
}

// GET /api/auth/logout-token — returns the Okta ID token hint for SDK logout.
func (h *AuthHandler) LogoutToken(c *gin.Context) {
	refreshToken, _ := c.Cookie("portal_refresh")
	if refreshToken == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active Okta session"})
		return
	}

	ctx, cancel := redisOpContext()
	idToken := h.rdb.Get(ctx, "okta_id_token:"+refreshToken).Val()
	cancel()
	if idToken == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Okta id token not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id_token": idToken})
}

// GET /api/auth/users — Admin: list users
func (h *AuthHandler) ListUsers(c *gin.Context) {
	if c.GetHeader("X-User-Role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}
	var users []User
	h.db.Select("id, email, name, role, department, is_active, last_login_at, created_at").
		Order("created_at desc").Find(&users)
	c.JSON(http.StatusOK, gin.H{"users": users, "count": len(users)})
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

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisURL, Password: cfg.RedisAuth})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Printf("Redis connection warning: %v (continuing without Redis)", err)
	}

	h := &AuthHandler{db: db, rdb: rdb, cfg: cfg}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:8080"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-User-ID", "X-User-Role"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "auth-service"})
	})

	auth := r.Group("/api/auth")
	{
		auth.POST("/dev-login", h.DevLogin)
		auth.POST("/auth0-exchange", h.Auth0Exchange)
		auth.GET("/login", h.OktaLogin)
		auth.GET("/callback", h.OktaCallback)
		auth.GET("/session", h.Session)
		auth.GET("/logout-token", h.LogoutToken)
		auth.GET("/logout", h.LogoutRedirect)
		auth.POST("/logout", h.Logout)
		auth.POST("/refresh", h.Refresh)
		auth.GET("/me", h.Me)
		auth.GET("/users", h.ListUsers)
	}

	log.Printf("Auth Service listening on :%s (DevMode=%v)", cfg.Port, cfg.DevMode)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
