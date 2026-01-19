package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	adminKey     string
	adminKeyOnce sync.Once
)

// getAdminKey returns the configured admin key, loading it once from environment.
// Returns empty string if ADMIN_KEY is not set (auth disabled).
func getAdminKey() string {
	adminKeyOnce.Do(func() {
		adminKey = os.Getenv("ADMIN_KEY")
	})
	return adminKey
}

// AdminKeyAuth returns middleware that requires a valid admin key for access.
// If ADMIN_KEY environment variable is not set, all requests are allowed (backwards compatible).
// The key should be provided in the Authorization header as "Bearer <key>".
func AdminKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := getAdminKey()

		// If no admin key is configured, allow all requests (backwards compatible for local dev)
		if key == "" {
			c.Next()
			return
		}

		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
				"code":  "AUTH_REQUIRED",
			})
			return
		}

		// Expect "Bearer <token>" format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization format. Use: Bearer <admin_key>",
				"code":  "AUTH_INVALID_FORMAT",
			})
			return
		}

		providedKey := parts[1]

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(key)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid admin key",
				"code":  "AUTH_INVALID_KEY",
			})
			return
		}

		c.Next()
	}
}

// VerifyAdminKey is a handler that verifies if the provided admin key is valid.
// Used by clients to check if their stored key is still valid.
func VerifyAdminKey(c *gin.Context) {
	key := getAdminKey()

	// If no admin key is configured, auth is disabled
	if key == "" {
		c.JSON(http.StatusOK, gin.H{
			"valid":        true,
			"auth_enabled": false,
			"message":      "Authentication is not configured",
		})
		return
	}

	// Extract token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"valid": false,
			"error": "Authorization header required",
			"code":  "AUTH_REQUIRED",
		})
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		c.JSON(http.StatusUnauthorized, gin.H{
			"valid": false,
			"error": "Invalid authorization format",
			"code":  "AUTH_INVALID_FORMAT",
		})
		return
	}

	providedKey := parts[1]

	if subtle.ConstantTimeCompare([]byte(providedKey), []byte(key)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"valid": false,
			"error": "Invalid admin key",
			"code":  "AUTH_INVALID_KEY",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":        true,
		"auth_enabled": true,
	})
}

// GetAuthStatus returns whether authentication is enabled (ADMIN_KEY is set).
// This is a public endpoint that doesn't require authentication.
func GetAuthStatus(c *gin.Context) {
	key := getAdminKey()
	c.JSON(http.StatusOK, gin.H{
		"auth_enabled": key != "",
	})
}
