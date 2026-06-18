package api

import (
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

	var request categoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category name is required"})
		return
	}

	var maxSort int
	_ = s.db.Model(&models.Category{}).Where("user_id = ?", userID).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSort).Error

	category := models.Category{
		UserID:    userID,
		Name:      strings.TrimSpace(request.Name),
		Color:     strings.TrimSpace(request.Color),
		Show:      true,
		SortOrder: maxSort + 10,
	}
	if category.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category name is required"})
		return
	}
	if category.Color == "" {
		category.Color = "#216869"
	}

	if err := s.db.Create(&category).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "category already exists"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "category_update", "payload": category})
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

	var request categoryUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category payload"})
		return
	}

	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category name is required"})
			return
		}
		category.Name = name
	}
	if request.Color != nil {
		category.Color = strings.TrimSpace(*request.Color)
		if category.Color == "" {
			category.Color = "#216869"
		}
	}
	if request.Show != nil {
		category.Show = *request.Show
	}

	if err := s.db.Save(&category).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "category already exists"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "category_update", "payload": category})
	c.JSON(http.StatusOK, category)
}

func (s *Server) reorderCategories(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var request categoryReorderRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 {
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
	c.Status(http.StatusNoContent)
}
