package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

type authRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

var errUsernameExists = errors.New("username already exists")

func (s *Server) register(c *gin.Context) {
	var request authRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	username := strings.TrimSpace(request.Username)
	if len(username) < 3 || len(request.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username or password is too short"})
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
			Username:     username,
			PasswordHash: string(hash),
			Role:         role,
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
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
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
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}
