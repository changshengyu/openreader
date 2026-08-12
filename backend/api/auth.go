package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

const (
	maxAuthRequestBodyBytes = 16 << 10
	maxBcryptPasswordBytes  = 72
)

var (
	errAuthRequestInvalid  = errors.New("invalid auth request")
	errAuthRequestTooLarge = errors.New("auth request body too large")
)

var errUsernameExists = errors.New("username already exists")

var readerDevUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// validateNewAccountCredentials reproduces reader-dev's new-account contract.
// It deliberately applies only while creating or resetting credentials: legacy
// SQLite rows may contain older usernames/passwords and must remain loginable.
func validateNewAccountCredentials(rawUsername, password string) (string, string) {
	username := strings.TrimSpace(rawUsername)
	if len(username) < 5 {
		return "", "username must be at least 5 characters"
	}
	if validationError := validateNewPassword(password); validationError != "" {
		return "", validationError
	}
	if strings.EqualFold(username, "default") {
		return "", "username is reserved"
	}
	if !readerDevUsernamePattern.MatchString(username) {
		return "", "username may contain only letters and numbers"
	}
	return username, ""
}

func validateResetPassword(password string) string {
	return validateNewPassword(password)
}

func validateNewPassword(password string) string {
	if len(utf16.Encode([]rune(password))) < 8 {
		return "password must be at least 8 characters"
	}
	if len([]byte(password)) > maxBcryptPasswordBytes {
		return "password must be at most 72 bytes"
	}
	return ""
}

func boolValue(value bool) *bool {
	return &value
}

func decodeAuthRequest(c *gin.Context, request *authRequest) error {
	if err := decodeBoundedSingleJSON(c, request, maxAuthRequestBodyBytes); err != nil {
		if errors.Is(err, errJSONRequestTooLarge) {
			return errAuthRequestTooLarge
		}
		return errAuthRequestInvalid
	}
	if request.Username == "" || request.Password == "" {
		return errAuthRequestInvalid
	}
	return nil
}

func writeAuthRequestError(c *gin.Context, err error) {
	if errors.Is(err, errAuthRequestTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
}

func (s *Server) register(c *gin.Context) {
	var request authRequest
	if err := decodeAuthRequest(c, &request); err != nil {
		writeAuthRequestError(c, err)
		return
	}
	if len(request.Password) > maxBcryptPasswordBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at most 72 bytes"})
		return
	}

	username, validationError := validateNewAccountCredentials(request.Username, request.Password)
	if validationError != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationError})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	s.registerMu.Lock()
	defer s.registerMu.Unlock()

	user := models.User{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&models.User{}).Where("username = ?", username).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return errUsernameExists
		}

		var userCount int64
		if err := tx.Model(&models.User{}).Count(&userCount).Error; err != nil {
			return err
		}
		role := "user"
		if userCount == 0 {
			role = "admin"
		}
		user = models.User{
			Username:        username,
			PasswordHash:    string(hash),
			Role:            role,
			CanEditSources:  true,
			CanAccessStore:  true,
			CanAccessWebDAV: boolValue(true),
			LastActiveAt:    time.Now().UTC(),
		}
		return tx.Create(&user).Error
	})
	if errors.Is(err, errUsernameExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	s.respondWithToken(c, user)
}

func (s *Server) login(c *gin.Context) {
	var request authRequest
	if err := decodeAuthRequest(c, &request); err != nil {
		writeAuthRequestError(c, err)
		return
	}
	if len(request.Password) > maxBcryptPasswordBytes {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	var user models.User
	err := s.db.Where("username = ?", strings.TrimSpace(request.Username)).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	loginAt := time.Now().UTC()
	if err := s.db.Model(&models.User{}).
		Where("id = ?", user.ID).
		UpdateColumn("last_active_at", loginAt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record login"})
		return
	}
	user.LastActiveAt = loginAt
	s.respondWithToken(c, user)
}

func (s *Server) me(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (s *Server) respondWithToken(c *gin.Context, user models.User) {
	token, err := middleware.GenerateToken(s.cfg.JWTSecret, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": struct {
			models.User
			LastLoginAt time.Time `json:"lastLoginAt"`
		}{
			User:        user,
			LastLoginAt: user.LastActiveAt,
		},
	})
}
