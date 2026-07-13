package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/sourcefailure"
)

type invalidSourceResponse struct {
	models.BookSource
	ErrorMessage string    `json:"errorMessage"`
	FailedAt     time.Time `json:"failedAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

func (s *Server) listInvalidSources(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	rows, err := s.invalidSourceResponses(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invalid sources"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (s *Server) legacyInvalidBookSources(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	rows, err := s.invalidSourceResponses(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invalid sources"})
		return
	}
	result := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		result = append(result, gin.H{
			"sourceUrl": row.BaseURL,
			"time":      row.FailedAt.UnixMilli(),
			"error":     row.ErrorMessage,
		})
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) invalidSourceResponses(userID uint) ([]invalidSourceResponse, error) {
	var sources []models.BookSource
	if err := s.db.Order("custom_order asc, id asc").Find(&sources).Error; err != nil {
		return nil, err
	}
	active, err := s.activeSourceFailures(userID, sources)
	if err != nil {
		return nil, err
	}
	result := make([]invalidSourceResponse, 0, len(active))
	for _, source := range sources {
		failure, ok := active[source.ID]
		if !ok {
			continue
		}
		result = append(result, invalidSourceResponse{
			BookSource:   source,
			ErrorMessage: failure.Message,
			FailedAt:     failure.FailedAt,
			ExpiresAt:    failure.ExpiresAt,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].FailedAt.After(result[j].FailedAt)
	})
	return result, nil
}

func (s *Server) activeSourceFailures(userID uint, sources []models.BookSource) (map[uint]models.SourceFailure, error) {
	if s.sourceFailures == nil {
		s.sourceFailures = sourcefailure.New(s.db)
	}
	return s.sourceFailures.Active(userID, sources)
}

func (s *Server) filterActiveSourceFailures(userID uint, sources []models.BookSource) []models.BookSource {
	active, err := s.activeSourceFailures(userID, sources)
	if err != nil {
		return sources
	}
	if len(active) == 0 {
		return sources
	}
	filtered := make([]models.BookSource, 0, len(sources))
	for _, source := range sources {
		if _, failed := active[source.ID]; !failed {
			filtered = append(filtered, source)
		}
	}
	return filtered
}

func (s *Server) recordSourceFailure(userID uint, source models.BookSource, cause error) {
	if !engine.IsSourceRequestError(cause) {
		return
	}
	s.recordSourceHealthFailure(userID, source, cause)
}

// recordSourceHealthFailure is reserved for an explicit source-manager test.
// Normal reading/search flows must not suppress a source for a local parser or
// configuration error that a user can correct without waiting for cache expiry.
func (s *Server) recordSourceHealthFailure(userID uint, source models.BookSource, cause error) {
	if userID == 0 || source.ID == 0 || cause == nil || errors.Is(cause, context.Canceled) || engine.IsSourceRuleError(cause) {
		return
	}
	if s.sourceFailures == nil {
		s.sourceFailures = sourcefailure.New(s.db)
	}
	s.sourceFailures.Record(userID, source, cause)
}

func (s *Server) clearSourceFailureIDs(sourceIDs []uint) {
	if s.sourceFailures == nil {
		s.sourceFailures = sourcefailure.New(s.db)
	}
	s.sourceFailures.ClearSourceIDs(sourceIDs)
}

func (s *Server) clearAllSourceFailures() {
	if s.sourceFailures == nil {
		s.sourceFailures = sourcefailure.New(s.db)
	}
	s.sourceFailures.ClearAll()
}
