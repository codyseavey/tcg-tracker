package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminKeyAuth(t *testing.T) {
	// Save original env and restore after test
	originalKey := os.Getenv("ADMIN_KEY")
	defer os.Setenv("ADMIN_KEY", originalKey)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		adminKey       string // env var value
		authHeader     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "no admin key configured - allows all requests",
			adminKey:       "",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedBody:   "ok",
		},
		{
			name:           "valid admin key",
			adminKey:       "test-secret-key",
			authHeader:     "Bearer test-secret-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "ok",
		},
		{
			name:           "missing auth header",
			adminKey:       "test-secret-key",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "AUTH_REQUIRED",
		},
		{
			name:           "invalid auth format - no Bearer",
			adminKey:       "test-secret-key",
			authHeader:     "test-secret-key",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "AUTH_INVALID_FORMAT",
		},
		{
			name:           "invalid admin key",
			adminKey:       "test-secret-key",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "AUTH_INVALID_KEY",
		},
		{
			name:           "case insensitive Bearer",
			adminKey:       "test-secret-key",
			authHeader:     "bearer test-secret-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the cached admin key for each test
			adminKeyOnce = sync.Once{}
			adminKey = ""

			// Set the admin key env var
			os.Setenv("ADMIN_KEY", tt.adminKey)

			// Create a test router with the middleware
			router := gin.New()
			router.Use(AdminKeyAuth())
			router.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Execute request
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check response body contains expected string
			if tt.expectedBody != "" && !contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestVerifyAdminKey(t *testing.T) {
	originalKey := os.Getenv("ADMIN_KEY")
	defer os.Setenv("ADMIN_KEY", originalKey)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		adminKey       string
		authHeader     string
		expectedStatus int
		expectedValid  bool
	}{
		{
			name:           "auth disabled - always valid",
			adminKey:       "",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			expectedValid:  true,
		},
		{
			name:           "valid key",
			adminKey:       "test-key",
			authHeader:     "Bearer test-key",
			expectedStatus: http.StatusOK,
			expectedValid:  true,
		},
		{
			name:           "invalid key",
			adminKey:       "test-key",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
			expectedValid:  false,
		},
		{
			name:           "missing header",
			adminKey:       "test-key",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cached values
			adminKeyOnce = sync.Once{}
			adminKey = ""

			os.Setenv("ADMIN_KEY", tt.adminKey)

			router := gin.New()
			router.POST("/auth/verify", VerifyAdminKey)

			req := httptest.NewRequest(http.MethodPost, "/auth/verify", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check valid field in response
			if tt.expectedValid && !contains(w.Body.String(), `"valid":true`) {
				t.Errorf("expected valid:true in response, got %s", w.Body.String())
			}
			if !tt.expectedValid && contains(w.Body.String(), `"valid":true`) {
				t.Errorf("expected valid:false in response, got %s", w.Body.String())
			}
		})
	}
}

func TestGetAuthStatus(t *testing.T) {
	originalKey := os.Getenv("ADMIN_KEY")
	defer os.Setenv("ADMIN_KEY", originalKey)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		adminKey    string
		authEnabled bool
	}{
		{
			name:        "auth disabled when no key",
			adminKey:    "",
			authEnabled: false,
		},
		{
			name:        "auth enabled when key set",
			adminKey:    "some-key",
			authEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adminKeyOnce = sync.Once{}
			adminKey = ""

			os.Setenv("ADMIN_KEY", tt.adminKey)

			router := gin.New()
			router.GET("/auth/status", GetAuthStatus)

			req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			expectedStr := "false"
			if tt.authEnabled {
				expectedStr = "true"
			}
			if !contains(w.Body.String(), `"auth_enabled":`+expectedStr) {
				t.Errorf("expected auth_enabled:%s, got %s", expectedStr, w.Body.String())
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
