package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/bookgroups"
)

type builtInBookGroupUpdateRequest struct {
	Name *string `json:"name"`
	Show *bool   `json:"show"`
}

type bookGroupReorderRequest struct {
	Keys []string `json:"keys"`
}

func (s *Server) listBookGroups(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	rows, err := s.bookGroups.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list book groups"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) updateBuiltInBookGroup(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	key := strings.TrimSpace(c.Param("key"))
	var request builtInBookGroupUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil || (request.Name == nil && request.Show == nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book group payload"})
		return
	}
	row, err := s.bookGroups.UpdateBuiltIn(userID, key, request.Name, request.Show)
	if errors.Is(err, bookgroups.ErrInvalidBuiltIn) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid built-in book group"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update book group"})
		return
	}
	s.broadcastBookGroupsUpdate(userID)
	c.JSON(http.StatusOK, row)
}

func (s *Server) reorderBookGroups(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var request bookGroupReorderRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys is required"})
		return
	}
	rows, err := s.bookGroups.Reorder(userID, request.Keys)
	if errors.Is(err, bookgroups.ErrInvalidOrder) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book group order"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reorder book groups"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "book_groups_update", "payload": rows})
	s.broadcastCategoriesUpdate(userID)
	c.JSON(http.StatusOK, rows)
}

func (s *Server) broadcastBookGroupsUpdate(userID uint) {
	rows, err := s.bookGroups.List(userID)
	if err != nil {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "book_groups_update"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "book_groups_update", "payload": rows})
}

func (s *Server) broadcastCategoriesUpdate(userID uint) {
	var categories []models.Category
	if err := s.db.Where("user_id = ?", userID).Order("sort_order asc, name asc").Find(&categories).Error; err != nil {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update", "payload": categories})
}
