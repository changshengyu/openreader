package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openreader/backend/models"
)

const webDAVRealm = `Basic realm="OpenReader WebDAV"`

// WebDAVAuthRequired accepts the application's Bearer token and reader-dev's
// Basic credentials. Both paths resolve to the same persisted user id before
// storage authorization derives a caller-scoped root.
func WebDAVAuthRequired(secret string, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if c.Request.Method == http.MethodOptions && header == "" {
			c.Next()
			return
		}

		userID, ok := authenticateWebDAVHeader(secret, db, header)
		if !ok {
			c.Header("WWW-Authenticate", webDAVRealm)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

func authenticateWebDAVHeader(secret string, db *gorm.DB, header string) (uint, bool) {
	if strings.HasPrefix(header, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" {
			return 0, false
		}
		userID, err := ParseToken(secret, token)
		return userID, err == nil && userID != 0
	}

	if !strings.HasPrefix(strings.ToLower(header), "basic ") {
		return 0, false
	}
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		return 0, false
	}
	request.Header.Set("Authorization", header)
	username, password, ok := request.BasicAuth()
	username = strings.TrimSpace(username)
	if !ok || username == "" || password == "" {
		return 0, false
	}

	var user models.User
	if err := db.Select("id", "password_hash").Where("username = ?", username).First(&user).Error; err != nil {
		return 0, false
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return 0, false
	}
	return user.ID, user.ID != 0
}
