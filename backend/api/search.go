package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

type searchRequest struct {
	Keyword         string `json:"keyword" binding:"required"`
	SourceIDs       []uint `json:"sourceIds"`
	ConcurrentCount int    `json:"concurrentCount"`
}

func (s *Server) search(c *gin.Context) {
	var req searchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is required"})
		return
	}
	req.Keyword = strings.TrimSpace(req.Keyword)
	if req.Keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is required"})
		return
	}

	var sources []models.BookSource
	query := s.db.Where("enabled = ?", true)
	if len(req.SourceIDs) > 0 {
		query = query.Where("id IN ?", req.SourceIDs)
	}
	if err := query.Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sources"})
		return
	}
	if len(sources) == 0 {
		c.JSON(http.StatusOK, []engine.SearchResult{})
		return
	}

	results := concurrentSearch(c.Request.Context(), sources, req.Keyword, req.ConcurrentCount)
	c.JSON(http.StatusOK, results)
}

func concurrentSearch(parent context.Context, sources []models.BookSource, keyword string, concurrentCount int) []engine.SearchResult {
	type searchOutcome struct {
		Results []engine.SearchResult
		Error   error
	}

	var wg sync.WaitGroup
	channel := make(chan searchOutcome, len(sources))
	timeout := 15 * time.Second
	limit := concurrentCount
	if limit <= 0 {
		limit = 60
	}
	if limit > len(sources) {
		limit = len(sources)
	}
	if limit < 1 {
		limit = 1
	}
	workerGate := make(chan struct{}, limit)

	for _, source := range sources {
		wg.Add(1)
		source := source
		go func() {
			defer wg.Done()
			select {
			case workerGate <- struct{}{}:
			case <-parent.Done():
				channel <- searchOutcome{Error: parent.Err()}
				return
			}
			defer func() { <-workerGate }()
			ctx, cancel := context.WithTimeout(parent, timeout)
			results, err := engine.SearchBooksContext(ctx, source, keyword)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				err = errTimeout
			}
			channel <- searchOutcome{Results: results, Error: err}
		}()
	}

	go func() {
		wg.Wait()
		close(channel)
	}()

	seen := make(map[string]bool)
	aggregated := make([]engine.SearchResult, 0)
	for outcome := range channel {
		if outcome.Error != nil {
			continue
		}
		for _, result := range outcome.Results {
			key := result.Title + "|" + result.Author
			if seen[key] {
				continue
			}
			seen[key] = true
			aggregated = append(aggregated, result)
		}
	}
	if aggregated == nil {
		aggregated = []engine.SearchResult{}
	}
	return aggregated
}

var errTimeout = &searchTimeoutError{}

type searchTimeoutError struct{}

func (e *searchTimeoutError) Error() string { return "search timeout" }
