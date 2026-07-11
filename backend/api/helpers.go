package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

const storeUserContextKey = "openreader.store-user"

// ---- unified error helpers ----

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func errResp(code, message string) gin.H {
	return gin.H{"error": apiError{Code: code, Message: message}}
}

func badRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, errResp("BAD_REQUEST", message))
}

func notFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, errResp("NOT_FOUND", message))
}

func unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, errResp("UNAUTHORIZED", message))
}

func conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, errResp("CONFLICT", message))
}

func internalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", message))
}

// ---- param helpers ----

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		badRequest(c, "invalid "+name)
		return 0, false
	}
	return uint(value), true
}

func (s *Server) ensureBook(c *gin.Context, userID, bookID uint) (models.Book, bool) {
	var book models.Book
	err := s.db.Where("user_id = ? AND id = ?", userID, bookID).First(&book).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		notFound(c, "book not found")
		return book, false
	}
	if err != nil {
		internalError(c, "failed to load book")
		return book, false
	}
	return book, true
}

func (s *Server) currentUserName(c *gin.Context, userID uint) (string, bool) {
	var user models.User
	err := s.db.Select("username").First(&user, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		unauthorized(c, "user not found")
		return "", false
	}
	if err != nil {
		internalError(c, "failed to load user")
		return "", false
	}
	return user.Username, true
}

// requireStoreAccess is the common authorization boundary for all workspace
// storage operations. Local-store, raw WebDAV and backup endpoints all touch
// mounted files, so a UI-only permission check would be insufficient.
func (s *Server) requireStoreAccess(c *gin.Context) bool {
	userID, ok := middleware.UserID(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("UNAUTHORIZED", "login required"))
		return false
	}

	var user models.User
	if err := s.db.Select("id", "username", "role", "can_access_store").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("UNAUTHORIZED", "user not found"))
			return false
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "failed to load user"))
		return false
	}
	if !user.CanAccessStore {
		c.AbortWithStatusJSON(http.StatusForbidden, errResp("FORBIDDEN", "store access denied"))
		return false
	}
	c.Set(storeUserContextKey, user)
	return true
}

func storeUser(c *gin.Context) (models.User, bool) {
	value, ok := c.Get(storeUserContextKey)
	if !ok {
		return models.User{}, false
	}
	user, ok := value.(models.User)
	return user, ok
}

// storeRoot keeps the pre-multi-user tree as the administrator's compatible
// root while every regular user is contained beneath a private child directory.
// No existing mounted files are moved or deleted by this compatibility layer.
func (s *Server) storeRoot(c *gin.Context, root string) (string, bool) {
	user, ok := storeUser(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("UNAUTHORIZED", "store user missing"))
		return "", false
	}
	if user.Role == "admin" {
		return root, true
	}
	return filepath.Join(root, "users", engine.SafeFilename(user.Username)), true
}

func (s *Server) validateCategory(c *gin.Context, userID uint, categoryID *uint) bool {
	if categoryID == nil || *categoryID == 0 {
		return true
	}

	var count int64
	if err := s.db.Model(&models.Category{}).
		Where("user_id = ? AND id = ?", userID, *categoryID).
		Count(&count).Error; err != nil {
		internalError(c, "failed to validate category")
		return false
	}
	if count == 0 {
		badRequest(c, "category not found")
		return false
	}
	return true
}

func (s *Server) validateCategoryIDs(c *gin.Context, userID uint, categoryIDs []uint) bool {
	ids := uniquePositiveUintIDs(categoryIDs)
	if len(ids) == 0 {
		return true
	}
	var count int64
	if err := s.db.Model(&models.Category{}).
		Where("user_id = ? AND id IN ?", userID, ids).
		Count(&count).Error; err != nil {
		internalError(c, "failed to validate categories")
		return false
	}
	if count != int64(len(ids)) {
		badRequest(c, "category not found")
		return false
	}
	return true
}

func (s *Server) requireOwnedBookIDs(c *gin.Context, userID uint, bookIDs []uint) ([]uint, bool) {
	ids := uniquePositiveUintIDs(bookIDs)
	if len(ids) == 0 || len(ids) != len(bookIDs) {
		badRequest(c, "bookIds must contain unique positive ids")
		return nil, false
	}
	var count int64
	if err := s.db.Model(&models.Book{}).
		Where("user_id = ? AND id IN ?", userID, ids).
		Count(&count).Error; err != nil {
		internalError(c, "failed to validate books")
		return nil, false
	}
	if count != int64(len(ids)) {
		notFound(c, "book not found")
		return nil, false
	}
	return ids, true
}

func uniquePositiveUintIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
