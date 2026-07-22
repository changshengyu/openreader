package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

const (
	maxReplaceRuleNameBytes        = 120
	maxReplaceRulePatternBytes     = 16 * 1024
	maxReplaceRuleReplacementBytes = 64 * 1024
	maxReplaceRuleScopeBytes       = 800
)

type replaceRuleRequest struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Scope       string `json:"scope"`
	IsRegex     *bool  `json:"isRegex"`
	IsEnabled   *bool  `json:"isEnabled"`
	Enabled     *bool  `json:"enabled"`
	Order       int    `json:"order"`
}

type replaceRuleTestRequest struct {
	Pattern     string `json:"pattern" binding:"required"`
	Replacement string `json:"replacement"`
	IsRegex     *bool  `json:"isRegex"`
	Text        string `json:"text" binding:"required"`
}

type replaceRuleResponse struct {
	models.ReplaceRule
	IsEnabled bool `json:"isEnabled"`
}

func replacementRuleResponse(rule models.ReplaceRule) replaceRuleResponse {
	return replaceRuleResponse{
		ReplaceRule: rule,
		IsEnabled:   rule.Enabled,
	}
}

func replacementRuleResponses(rules []models.ReplaceRule) []replaceRuleResponse {
	responses := make([]replaceRuleResponse, 0, len(rules))
	for _, rule := range rules {
		responses = append(responses, replacementRuleResponse(rule))
	}
	return responses
}

func (s *Server) listReplaceRules(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var rules []models.ReplaceRule
	if err := s.db.Where("user_id = ?", userID).Order("sort_order asc, id asc").Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list replace rules"})
		return
	}
	c.JSON(http.StatusOK, replacementRuleResponses(rules))
}

func (s *Server) createReplaceRule(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req replaceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
		return
	}
	rule, err := replaceRuleFromRequest(userID, req)
	if err != nil {
		writeReplaceRuleValidationError(c, err)
		return
	}

	var existing models.ReplaceRule
	err = s.db.Where("user_id = ? AND name = ?", userID, rule.Name).Order("id asc").First(&existing).Error
	switch {
	case err == nil:
		existing.Group = rule.Group
		existing.Pattern = rule.Pattern
		existing.Replacement = rule.Replacement
		existing.Scope = rule.Scope
		existing.IsRegex = rule.IsRegex
		existing.Enabled = rule.Enabled
		existing.Order = rule.Order
		if err := s.db.Save(&existing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save replace rule"})
			return
		}
		s.broadcastReplaceRulesUpdate(userID, "update")
		c.JSON(http.StatusOK, replacementRuleResponse(existing))
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.Create(&rule).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create replace rule"})
			return
		}
		s.broadcastReplaceRulesUpdate(userID, "create")
		c.JSON(http.StatusCreated, replacementRuleResponse(rule))
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create replace rule"})
	}
}

func replaceRuleFromRequest(userID uint, req replaceRuleRequest) (models.ReplaceRule, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	isRegex := false
	if req.IsRegex != nil {
		isRegex = *req.IsRegex
	}
	rule := models.ReplaceRule{
		UserID:      userID,
		Name:        strings.TrimSpace(req.Name),
		Group:       strings.TrimSpace(req.Group),
		Pattern:     strings.TrimSpace(req.Pattern),
		Replacement: req.Replacement,
		Scope:       strings.TrimSpace(req.Scope),
		IsRegex:     &isRegex,
		Enabled:     enabled,
		Order:       req.Order,
	}
	if rule.Name == "" {
		return models.ReplaceRule{}, errors.New("name is required")
	}
	if rule.Pattern == "" {
		return models.ReplaceRule{}, errors.New("pattern is required")
	}
	if rule.Scope == "" {
		return models.ReplaceRule{}, errors.New("scope is required")
	}
	if len(rule.Name) > maxReplaceRuleNameBytes ||
		len(rule.Pattern) > maxReplaceRulePatternBytes ||
		len(rule.Replacement) > maxReplaceRuleReplacementBytes ||
		len(rule.Scope) > maxReplaceRuleScopeBytes {
		return models.ReplaceRule{}, errors.New("replace rule is too large")
	}
	if err := validateReaderReplaceRulePattern(rule.Pattern, isRegex); err != nil {
		return models.ReplaceRule{}, err
	}
	return rule, nil
}

func (s *Server) upsertReplaceRules(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var requests []replaceRuleRequest
	if err := c.ShouldBindJSON(&requests); err != nil || len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid replace rules payload"})
		return
	}

	prepared := make([]models.ReplaceRule, 0, len(requests))
	skipped := 0
	for _, request := range requests {
		if strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Pattern) == "" {
			skipped++
			continue
		}
		next, err := replaceRuleFromRequest(userID, request)
		if err != nil {
			writeReplaceRuleValidationError(c, err)
			return
		}
		prepared = append(prepared, next)
	}

	rules := make([]models.ReplaceRule, 0, len(prepared))
	created := 0
	updated := 0
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, next := range prepared {
			var existing models.ReplaceRule
			err := tx.Where("user_id = ? AND name = ?", userID, next.Name).Order("id asc").First(&existing).Error
			switch {
			case err == nil:
				existing.Group = next.Group
				existing.Pattern = next.Pattern
				existing.Replacement = next.Replacement
				existing.Scope = next.Scope
				existing.IsRegex = next.IsRegex
				existing.Enabled = next.Enabled
				existing.Order = next.Order
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
		"rules":   replacementRuleResponses(rules),
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid replace rule"})
		return
	}
	next, err := replaceRuleFromRequest(userID, req)
	if err != nil {
		writeReplaceRuleValidationError(c, err)
		return
	}
	if next.Name != rule.Name {
		var conflict models.ReplaceRule
		err := s.db.Where("user_id = ? AND name = ? AND id <> ?", userID, next.Name, rule.ID).Order("id asc").First(&conflict).Error
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "replace rule name already exists"})
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate replace rule"})
			return
		}
	}
	rule.Name = next.Name
	rule.Group = next.Group
	rule.Pattern = next.Pattern
	rule.Replacement = next.Replacement
	rule.Scope = next.Scope
	rule.IsRegex = next.IsRegex
	rule.Enabled = next.Enabled
	rule.Order = next.Order
	if err := s.db.Save(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update replace rule"})
		return
	}
	s.broadcastReplaceRulesUpdate(userID, "update")
	c.JSON(http.StatusOK, replacementRuleResponse(rule))
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
	pattern := strings.TrimSpace(req.Pattern)
	isRegex := req.IsRegex != nil && *req.IsRegex
	if err := validateReaderReplaceRulePattern(pattern, isRegex); err != nil {
		writeReplaceRuleValidationError(c, err)
		return
	}
	input := req.Text
	output := applyReaderReplaceRule(input, pattern, req.Replacement, isRegex)
	c.JSON(http.StatusOK, gin.H{"input": input, "output": output, "changed": input != output})
}

func validateReaderReplaceRulePattern(pattern string, isRegex bool) error {
	if pattern == "" {
		return errors.New("pattern is required")
	}
	if !isRegex {
		return nil
	}
	_, err := compileReaderReplaceRule(pattern)
	if err != nil {
		return errors.New("invalid replace rule regex")
	}
	return nil
}

func compileReaderReplaceRule(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i:" + pattern + ")")
}

func applyReaderReplaceRule(content, pattern, replacement string, isRegex bool) string {
	if !isRegex {
		return strings.Replace(content, pattern, replacement, 1)
	}
	re, err := compileReaderReplaceRule(pattern)
	if err != nil {
		return content
	}
	return re.ReplaceAllString(content, replacement)
}

func writeReplaceRuleValidationError(c *gin.Context, err error) {
	message := "invalid replace rule"
	if err != nil {
		message = err.Error()
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": message})
}
