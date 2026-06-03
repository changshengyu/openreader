package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

type replaceRuleRequest struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern" binding:"required"`
	Replacement string `json:"replacement"`
	Enabled     *bool  `json:"enabled"`
}

type replaceRuleTestRequest struct {
	Pattern     string `json:"pattern" binding:"required"`
	Replacement string `json:"replacement"`
	Text        string `json:"text" binding:"required"`
}

func (s *Server) listReplaceRules(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var rules []models.ReplaceRule
	if err := s.db.Where("user_id = ?", userID).Order("updated_at desc").Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list replace rules"})
		return
	}
	c.JSON(http.StatusOK, rules)
}

func (s *Server) createReplaceRule(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req replaceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule := models.ReplaceRule{
		UserID:      userID,
		Name:        strings.TrimSpace(req.Name),
		Pattern:     strings.TrimSpace(req.Pattern),
		Replacement: req.Replacement,
		Enabled:     enabled,
	}
	if rule.Pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
		return
	}
	if rule.Name == "" {
		rule.Name = rule.Pattern
	}
	if err := s.db.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create replace rule"})
		return
	}
	s.broadcastReplaceRulesUpdate(userID, "create")
	c.JSON(http.StatusCreated, rule)
}

func (s *Server) updateReplaceRule(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	ruleID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var rule models.ReplaceRule
	if err := s.db.Where("user_id = ? AND id = ?", userID, ruleID).First(&rule).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "replace rule not found"})
		return
	}

	var req replaceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
		return
	}
	rule.Name = strings.TrimSpace(req.Name)
	rule.Pattern = strings.TrimSpace(req.Pattern)
	rule.Replacement = req.Replacement
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if rule.Pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
		return
	}
	if rule.Name == "" {
		rule.Name = rule.Pattern
	}
	if err := s.db.Save(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update replace rule"})
		return
	}
	s.broadcastReplaceRulesUpdate(userID, "update")
	c.JSON(http.StatusOK, rule)
}

func (s *Server) deleteReplaceRule(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	ruleID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	result := s.db.Where("user_id = ? AND id = ?", userID, ruleID).Delete(&models.ReplaceRule{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete replace rule"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "replace rule not found"})
		return
	}
	s.broadcastReplaceRulesUpdate(userID, "delete")
	c.Status(http.StatusNoContent)
}

func (s *Server) broadcastReplaceRulesUpdate(userID uint, kind string) {
	if s.hub == nil {
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{
		"type":    "replace_rules_update",
		"payload": gin.H{"kind": kind},
	})
}

func (s *Server) testReplaceRule(c *gin.Context) {
	var req replaceRuleTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern and text are required"})
		return
	}
	input := req.Text
	output := engine.ApplyTextReplacements(input, []models.TextReplaceRule{{
		Pattern:     strings.TrimSpace(req.Pattern),
		Replacement: req.Replacement,
	}})
	c.JSON(http.StatusOK, gin.H{"input": input, "output": output, "changed": input != output})
}
