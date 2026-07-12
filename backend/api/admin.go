package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

func (s *Server) requireAdmin(c *gin.Context) bool {
	userID, ok := middleware.UserID(c)
	if !ok {
		unauthorized(c, "login required")
		return false
	}
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil || user.Role != "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "admin access required"))
		return false
	}
	return true
}

func (s *Server) listUsers(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	var users []models.User
	if err := s.db.Order("created_at desc").Find(&users).Error; err != nil {
		internalError(c, "failed to list users")
		return
	}

	type userSummary struct {
		ID             uint      `json:"id"`
		Username       string    `json:"username"`
		Role           string    `json:"role"`
		BookLimit      int       `json:"bookLimit"`
		SourceLimit    int       `json:"sourceLimit"`
		CanEditSources bool      `json:"canEditSources"`
		CanAccessStore bool      `json:"canAccessStore"`
		BookCount      int64     `json:"bookCount"`
		SourceCount    int64     `json:"sourceCount"`
		LastActiveAt   time.Time `json:"lastActiveAt"`
		CreatedAt      time.Time `json:"createdAt"`
	}

	var sourceCount int64
	_ = s.db.Model(&models.BookSource{}).Count(&sourceCount).Error

	results := make([]userSummary, 0, len(users))
	for _, u := range users {
		var bookCount int64
		_ = s.db.Model(&models.Book{}).Where("user_id = ?", u.ID).Count(&bookCount).Error
		results = append(results, userSummary{
			ID:             u.ID,
			Username:       u.Username,
			Role:           u.Role,
			BookLimit:      u.BookLimit,
			SourceLimit:    u.SourceLimit,
			CanEditSources: u.CanEditSources,
			CanAccessStore: u.CanAccessStore,
			BookCount:      bookCount,
			SourceCount:    sourceCount,
			LastActiveAt:   u.LastActiveAt,
			CreatedAt:      u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, results)
}

type createAdminUserRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	Role           string `json:"role"`
	BookLimit      int    `json:"bookLimit"`
	SourceLimit    int    `json:"sourceLimit"`
	CanEditSources *bool  `json:"canEditSources"`
	CanAccessStore *bool  `json:"canAccessStore"`
}

func (s *Server) createUser(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	var req createAdminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(req.Password) < 6 {
		badRequest(c, "username or password is too short")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(c, "failed to hash password")
		return
	}

	role := strings.TrimSpace(req.Role)
	if role != "" && role != "user" {
		badRequest(c, "administrator role assignment is not allowed")
		return
	}

	user := models.User{
		Username:       username,
		PasswordHash:   string(hash),
		Role:           "user",
		BookLimit:      req.BookLimit,
		SourceLimit:    req.SourceLimit,
		CanEditSources: true,
		CanAccessStore: true,
		LastActiveAt:   time.Now(),
	}
	if req.CanEditSources != nil {
		user.CanEditSources = *req.CanEditSources
	}
	if req.CanAccessStore != nil {
		user.CanAccessStore = *req.CanAccessStore
	}

	if err := s.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, errResp("CONFLICT", "username already exists"))
		return
	}
	if req.CanEditSources != nil || req.CanAccessStore != nil {
		updates := map[string]any{}
		if req.CanEditSources != nil {
			updates["can_edit_sources"] = *req.CanEditSources
		}
		if req.CanAccessStore != nil {
			updates["can_access_store"] = *req.CanAccessStore
		}
		if err := s.db.Model(&user).Updates(updates).Error; err != nil {
			internalError(c, "failed to update user permissions")
			return
		}
		if err := s.db.First(&user, user.ID).Error; err != nil {
			internalError(c, "failed to reload user")
			return
		}
	}
	s.broadcastUsersUpdate("create", []uint{user.ID})
	c.JSON(http.StatusCreated, user)
}

type updateUserRequest struct {
	BookLimit      *int  `json:"bookLimit"`
	SourceLimit    *int  `json:"sourceLimit"`
	CanEditSources *bool `json:"canEditSources"`
	CanAccessStore *bool `json:"canAccessStore"`
}

func (s *Server) updateUser(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		notFound(c, "user not found")
		return
	}
	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "protected administrator cannot be modified"))
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	if req.BookLimit != nil {
		user.BookLimit = *req.BookLimit
	}
	if req.SourceLimit != nil {
		user.SourceLimit = *req.SourceLimit
	}
	if req.CanEditSources != nil {
		user.CanEditSources = *req.CanEditSources
	}
	if req.CanAccessStore != nil {
		user.CanAccessStore = *req.CanAccessStore
	}

	if err := s.db.Save(&user).Error; err != nil {
		internalError(c, "failed to update user")
		return
	}
	s.broadcastUsersUpdate("update", []uint{user.ID})
	c.JSON(http.StatusOK, user)
}

type resetUserPasswordRequest struct {
	Password string `json:"password"`
}

func (s *Server) resetUserPassword(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var req resetUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}
	if len(req.Password) < 6 {
		badRequest(c, "password is too short")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(c, "failed to hash password")
		return
	}

	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		notFound(c, "user not found")
		return
	}
	if user.Role == "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "protected administrator password cannot be reset"))
		return
	}
	if err := s.db.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
		internalError(c, "failed to reset password")
		return
	}
	s.broadcastUsersUpdate("password", []uint{id})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type deleteUsersRequest struct {
	IDs []uint `json:"ids"`
}

func (s *Server) deleteUsers(c *gin.Context) {
	currentUserID, ok := middleware.UserID(c)
	if !ok {
		unauthorized(c, "login required")
		return
	}
	if !s.requireAdmin(c) {
		return
	}

	var req deleteUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	ids := make([]uint, 0, len(req.IDs))
	seen := map[uint]bool{}
	for _, id := range req.IDs {
		if id == 0 || id == currentUserID || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		badRequest(c, "no deletable users selected")
		return
	}
	var allowedIDs []uint
	if err := s.db.Model(&models.User{}).
		Where("id IN ? AND id <> ? AND role <> ?", ids, currentUserID, "admin").
		Pluck("id", &allowedIDs).Error; err != nil {
		internalError(c, "failed to check users")
		return
	}
	if len(allowedIDs) == 0 {
		badRequest(c, "no deletable users selected")
		return
	}
	ids = allowedIDs

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var bookIDs []uint
		if err := tx.Model(&models.Book{}).Where("user_id IN ?", ids).Pluck("id", &bookIDs).Error; err != nil {
			return err
		}
		if len(bookIDs) > 0 {
			if err := tx.Where("book_id IN ?", bookIDs).Delete(&models.Chapter{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.Bookmark{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.ReadingProgress{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.Book{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.Category{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.RSSArticle{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.RSSSource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.ReplaceRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&models.UserSetting{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ? AND id <> ?", ids, currentUserID).Delete(&models.User{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		internalError(c, "failed to delete users")
		return
	}
	s.broadcastUsersUpdate("delete", ids)
	c.JSON(http.StatusOK, gin.H{"deleted": len(ids)})
}

func (s *Server) cleanupInactiveUsers(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	result := s.db.Where("role != ? AND last_active_at < ?", "admin", cutoff).Delete(&models.User{})
	if result.Error != nil {
		internalError(c, "cleanup failed")
		return
	}
	if result.RowsAffected > 0 {
		s.broadcastUsersUpdate("cleanup", nil)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": result.RowsAffected})
}

func (s *Server) broadcastUsersUpdate(kind string, userIDs []uint) {
	if s.hub == nil {
		return
	}
	_ = s.hub.BroadcastAll(nil, gin.H{
		"type": "users_update",
		"payload": gin.H{
			"kind":    kind,
			"userIds": userIDs,
		},
	})
}
