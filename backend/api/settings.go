package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

type userSettingRequest struct {
	Value         json.RawMessage `json:"value" binding:"required"`
	BaseUpdatedAt string          `json:"baseUpdatedAt"`
}

func (s *Server) getUserSetting(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	key := normalizeUserSettingKey(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid setting key"})
		return
	}

	var setting models.UserSetting
	err := s.db.Where("user_id = ? AND key = ?", userID, key).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load setting"})
		return
	}
	c.JSON(http.StatusOK, userSettingResponse(setting))
}

func (s *Server) updateUserSetting(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	key := normalizeUserSettingKey(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid setting key"})
		return
	}

	var req userSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Value) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "setting value is required"})
		return
	}
	if !json.Valid(req.Value) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "setting value must be valid json"})
		return
	}
	value := sanitizeUserSettingValue(key, req.Value)

	now := time.Now()
	setting := models.UserSetting{
		UserID:    userID,
		Key:       key,
		Value:     string(value),
		UpdatedAt: now,
	}

	var existing models.UserSetting
	err := s.db.Where("user_id = ? AND key = ?", userID, key).First(&existing).Error
	if err == nil && isStaleProgressUpdate(existing.UpdatedAt, req.BaseUpdatedAt) {
		c.Header("X-OpenReader-Setting-Conflict", "1")
		c.JSON(http.StatusOK, userSettingResponse(existing))
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save setting"})
		return
	}

	if err := s.db.Where("user_id = ? AND key = ?", userID, key).
		Assign(setting).
		FirstOrCreate(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save setting"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{
		"type": "settings_update",
		"payload": gin.H{
			"key":       key,
			"updatedAt": setting.UpdatedAt,
		},
	})
	c.JSON(http.StatusOK, userSettingResponse(setting))
}

func normalizeUserSettingKey(key string) string {
	key = strings.TrimSpace(key)
	switch key {
	case "reader", "shelf", "search":
		return key
	default:
		return ""
	}
}

func sanitizeUserSettingValue(key string, value json.RawMessage) json.RawMessage {
	if key != "reader" {
		return value
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(value, &data); err != nil {
		return value
	}
	delete(data, "pageMode")
	if encoded, err := json.Marshal(data); err == nil {
		return encoded
	}
	return value
}

func userSettingResponse(setting models.UserSetting) gin.H {
	var value any
	if err := json.Unmarshal([]byte(setting.Value), &value); err != nil {
		value = gin.H{}
	}
	return gin.H{
		"key":       setting.Key,
		"value":     value,
		"updatedAt": setting.UpdatedAt,
	}
}
