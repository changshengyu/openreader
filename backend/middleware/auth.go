package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"openreader/backend/services/authsession"
)

const userIDKey = "userID"
const sessionIdentityKey = "authenticatedSessionIdentity"

const (
	legacyDefaultJWTSecret  = "change-me-in-production"
	currentDefaultJWTSecret = "change-this-before-deploy"
)

type Claims struct {
	UserID uint `json:"userId"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, userID uint) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseToken(secret, tokenString string) (uint, error) {
	var lastErr error
	for _, candidate := range compatibleJWTSecrets(secret) {
		userID, err := parseTokenWithSecret(candidate, tokenString)
		if err == nil {
			return userID, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func parseTokenWithSecret(secret, tokenString string) (uint, error) {
	parsed, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithoutClaimsValidation())
	if err != nil {
		return 0, err
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid || claims.UserID == 0 {
		return 0, errors.New("invalid token")
	}
	return claims.UserID, nil
}

func compatibleJWTSecrets(secret string) []string {
	switch secret {
	case legacyDefaultJWTSecret:
		return []string{legacyDefaultJWTSecret, currentDefaultJWTSecret}
	case currentDefaultJWTSecret:
		return []string{currentDefaultJWTSecret, legacyDefaultJWTSecret}
	default:
		return []string{secret}
	}
}

func AuthRequired(secret string, sessionServices ...*authsession.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenString := strings.TrimPrefix(header, "Bearer ")
		if tokenString == "" || tokenString == header {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		var userID uint
		if len(sessionServices) > 0 && sessionServices[0] != nil {
			identity, err := sessionServices[0].Authenticate(c.Request.Context(), tokenString)
			if err != nil {
				if c.Request.Context().Err() != nil {
					c.Abort()
					return
				}
				if errors.Is(err, authsession.ErrInvalidSession) {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				} else {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
				}
				return
			}
			userID = identity.UserID
			c.Set(sessionIdentityKey, identity)
		} else {
			parsedUserID, err := ParseToken(secret, tokenString)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			userID = parsedUserID
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
}

func SessionIdentity(c *gin.Context) (authsession.Identity, bool) {
	value, exists := c.Get(sessionIdentityKey)
	if !exists {
		return authsession.Identity{}, false
	}
	identity, ok := value.(authsession.Identity)
	return identity, ok
}

func TrackActivity(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		userID, ok := UserID(c)
		if !ok {
			return
		}
		_ = db.Table("users").Where("id = ?", userID).Update("last_active_at", time.Now()).Error
	}
}

func UserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get(userIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint)
	return userID, ok
}
