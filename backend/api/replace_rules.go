package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

type replaceRuleRequest struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Scope       string `json:"scope"`
	IsRegex     *bool  `json:"isRegex"`
	IsEnabled   *bool  `json:"isEnabled"`
	Enabled     *bool  `json:"enabled"`
}

type replaceRuleTestRequest struct {
	Pattern     string `json:"pattern" binding:"required"`
	Replacement string `json:"replacement"`
	IsRegex     *bool  `json:"isRegex"`
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
	rule, ok := replaceRuleFromRequest(userID, req, true)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
		return
	}
	if err := s.db.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create replace rule"})
		return
	}
	s.broadcastReplaceRulesUpdate(userID, "create")
	c.JSON(http.StatusCreated, rule)
}

func replaceRuleFromRequest(userID uint, req replaceRuleRequest, defaultName bool) (models.ReplaceRule, bool) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	isRegex := true
	if req.IsRegex != nil {
		isRegex = *req.IsRegex
	}
	rule := models.ReplaceRule{
		UserID:      userID,
		Name:        strings.TrimSpace(req.Name),
		Pattern:     strings.TrimSpace(req.Pattern),
		Replacement: req.Replacement,
		Scope:       normalizeReplaceRuleScope(req.Scope),
		IsRegex:     &isRegex,
		Enabled:     enabled,
	}
	if rule.Pattern == "" {
		return models.ReplaceRule{}, false
	}
	if rule.Name == "" && defaultName {
		rule.Name = rule.Pattern
	}
	return rule, rule.Name != ""
}

func (s *Server) upsertReplaceRules(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var requests []replaceRuleRequest
	if err := c.ShouldBindJSON(&requests); err != nil || len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid replace rules payload"})
		return
	}

	rules := make([]models.ReplaceRule, 0, len(requests))
	created := 0
	updated := 0
	skipped := 0
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, request := range requests {
			next, ok := replaceRuleFromRequest(userID, request, false)
			if !ok {
				skipped++
				continue
			}
			var existing models.ReplaceRule
			err := tx.Where("user_id = ? AND name = ?", userID, next.Name).First(&existing).Error
			switch {
			case err == nil:
				existing.Pattern = next.Pattern
				existing.Replacement = next.Replacement
				existing.Scope = next.Scope
				existing.IsRegex = next.IsRegex
				existing.Enabled = next.Enabled
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
				rules = append(rules, existing)
				updated++
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.Create(&next).Error; err != nil {
					return err
				}
				rules = append(rules, next)
				created++
			default:
				return err
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save replace rules"})
		return
	}
	s.broadcastReplaceRulesUpdate(userID, "batch-upsert")
	c.JSON(http.StatusOK, gin.H{
		"rules":   rules,
		"created": created,
		"updated": updated,
		"skipped": skipped,
	})
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
	rule.Scope = normalizeReplaceRuleScope(req.Scope)
	if req.IsRegex != nil {
		rule.IsRegex = req.IsRegex
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.IsEnabled != nil {
		rule.Enabled = *req.IsEnabled
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

type replaceRuleBatchDeleteRequest struct {
	IDs []uint `json:"ids"`
}

func (s *Server) deleteReplaceRules(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var request replaceRuleBatchDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid replace rule ids"})
		return
	}
	ids := uniquePositiveUintIDs(request.IDs)
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "replace rule ids are required"})
		return
	}

	var rules []models.ReplaceRule
	if err := s.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load replace rules"})
		return
	}
	existing := make(map[uint]struct{}, len(rules))
	for _, rule := range rules {
		existing[rule.ID] = struct{}{}
	}
	deletedIDs := make([]uint, 0, len(rules))
	for _, id := range ids {
		if _, ok := existing[id]; ok {
			deletedIDs = append(deletedIDs, id)
		}
	}
	if len(deletedIDs) > 0 {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return tx.Where("user_id = ? AND id IN ?", userID, deletedIDs).Delete(&models.ReplaceRule{}).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete replace rules"})
			return
		}
		s.broadcastReplaceRulesUpdate(userID, "batch-delete")
	}
	c.JSON(http.StatusOK, gin.H{"deletedIds": deletedIDs})
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
		IsRegex:     req.IsRegex,
	}})
	c.JSON(http.StatusOK, gin.H{"input": input, "output": output, "changed": input != output})
}

func normalizeReplaceRuleScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "*"
	}
	return scope
}
