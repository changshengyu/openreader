package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

type categoryRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color"`
}

type categoryUpdateRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
	Show  *bool   `json:"show"`
}

type categoryReorderRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

// categoryPatchWriteLifecycleTestHook pauses a validated update before
// persistence so contract tests can deterministically exercise stale reads.
var categoryPatchWriteLifecycleTestHook func()

var errCategoryPatchTargetNotFound = errors.New("category not found during patch")

func (s *Server) listCategories(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var categories []models.Category
	if err := s.db.Where("user_id = ?", userID).Order("sort_order asc, name asc").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list categories"})
		return
	}
	c.JSON(http.StatusOK, categories)
}

func (s *Server) createCategory(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	request, ok := decodeBookGroupWriteRequest[categoryRequest](c, "category name is required")
	if !ok {
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category name is required"})
		return
	}
	if len(name) > maxBookGroupNameBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category name is too long"})
		return
	}
	color := strings.TrimSpace(request.Color)
	if len(color) > maxCategoryColorBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category color is too long"})
		return
	}
	if color == "" {
		color = "#216869"
	}

	nextSort, err := s.bookGroups.NextSortOrder(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create category"})
		return
	}

	category := models.Category{
		UserID:    userID,
		Name:      name,
		Color:     color,
		Show:      true,
		SortOrder: nextSort,
	}

	if err := s.db.Create(&category).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "category already exists"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "category_update", "payload": category})
	s.broadcastBookGroupsUpdate(userID)
	c.JSON(http.StatusCreated, category)
}

func (s *Server) updateCategory(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	categoryID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var category models.Category
	if err := s.db.Where("user_id = ? AND id = ?", userID, categoryID).First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "category not found"})
		return
	}

	request, ok := decodeBookGroupWriteRequest[categoryUpdateRequest](c, "invalid category payload")
	if !ok {
		return
	}

	updates := make(map[string]any)
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category name is required"})
			return
		}
		if len(name) > maxBookGroupNameBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category name is too long"})
			return
		}
		category.Name = name
		updates["name"] = name
	}
	if request.Color != nil {
		color := strings.TrimSpace(*request.Color)
		if len(color) > maxCategoryColorBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category color is too long"})
			return
		}
		if color == "" {
			color = "#216869"
		}
		category.Color = color
		updates["color"] = color
	}
	if request.Show != nil {
		category.Show = *request.Show
		updates["show"] = *request.Show
	}
	if categoryPatchWriteLifecycleTestHook != nil {
		categoryPatchWriteLifecycleTestHook()
	}

	ctx := c.Request.Context()
	if ctx.Err() != nil {
		return
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Category
		if err := tx.Where("user_id = ? AND id = ?", userID, categoryID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errCategoryPatchTargetNotFound
			}
			return err
		}
		if len(updates) > 0 {
			write := tx.Model(&models.Category{}).
				Where("user_id = ? AND id = ?", current.UserID, current.ID).
				Updates(updates)
			if write.Error != nil {
				return write.Error
			}
			if write.RowsAffected != 1 {
				return errCategoryPatchTargetNotFound
			}
		}
		return tx.Where("user_id = ? AND id = ?", current.UserID, current.ID).First(&category).Error
	}); err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		if errors.Is(err, errCategoryPatchTargetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "category not found"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "category already exists"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "category_update", "payload": category})
	s.broadcastBookGroupsUpdate(userID)
	c.JSON(http.StatusOK, category)
}

func (s *Server) reorderCategories(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	request, ok := decodeBookGroupWriteRequest[categoryReorderRequest](c, "ids is required")
	if !ok {
		return
	}
	if len(request.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		for index, id := range request.IDs {
			result := tx.Model(&models.Category{}).
				Where("user_id = ? AND id = ?", userID, id).
				Update("sort_order", (index+1)*10)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to reorder categories"})
		return
	}

	var categories []models.Category
	if err := s.db.Where("user_id = ?", userID).Order("sort_order asc, name asc").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list categories"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update", "payload": categories})
	s.broadcastBookGroupsUpdate(userID)
	c.JSON(http.StatusOK, categories)
}

func (s *Server) deleteCategory(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	categoryID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var category models.Category
	if err := s.db.Where("user_id = ? AND id = ?", userID, categoryID).First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "category not found"})
		return
	}

	var bookCount int64
	if err := s.db.Model(&models.BookCategory{}).
		Where("user_id = ? AND category_id = ?", userID, categoryID).
		Count(&bookCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check category books"})
		return
	}
	if bookCount == 0 {
		if err := s.db.Model(&models.Book{}).
			Where("user_id = ? AND category_id = ?", userID, categoryID).
			Count(&bookCount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check category books"})
			return
		}
	}
	if bookCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "category is not empty"})
		return
	}

	if err := s.db.Delete(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete category"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "category_delete", "payload": gin.H{"id": categoryID}})
	s.broadcastBookGroupsUpdate(userID)
	c.Status(http.StatusNoContent)
}
