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
	"openreader/backend/middleware"
	"openreader/backend/models"
)

type searchRequest struct {
	Keyword         string `json:"keyword" binding:"required"`
	SourceIDs       []uint `json:"sourceIds"`
	ConcurrentCount int    `json:"concurrentCount"`
	Page            *int   `json:"page"`
	LastIndex       *int   `json:"lastIndex"`
	SearchSize      *int   `json:"searchSize"`
}

type searchResponse struct {
	List      []engine.SearchResult `json:"list"`
	Page      int                   `json:"page"`
	LastIndex int                   `json:"lastIndex"`
	HasMore   bool                  `json:"hasMore"`
}

func (s *Server) search(c *gin.Context) {
	userID, _ := middleware.UserID(c)
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
	query := s.db.Where("enabled = ?", true).Order("custom_order ASC, id ASC")
	if len(req.SourceIDs) > 0 {
		query = query.Where("id IN ?", req.SourceIDs)
	}
	if err := query.Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sources"})
		return
	}
	sources = orderSearchSources(sources, req.SourceIDs)
	pagedRequest := req.Page != nil || req.LastIndex != nil || req.SearchSize != nil
	if len(sources) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未配置书源"})
		return
	}
	// The failure cache is a per-user execution skip, not a different source
	// list. Keep the original ordered list so a multi-source lastIndex remains a
	// stable cursor even when a source expires from, or enters, suppression.
	activeFailures, err := s.activeSourceFailures(userID, sources)
	if err != nil {
		activeFailures = nil
	}

	if !pagedRequest {
		results := s.concurrentSearch(c.Request.Context(), userID, sources, req.Keyword, req.ConcurrentCount, activeFailures)
		c.JSON(http.StatusOK, results)
		return
	}

	page := normalizedSearchPage(req.Page)
	if len(sources) == 1 {
		if _, suppressed := activeFailures[sources[0].ID]; suppressed {
			c.JSON(http.StatusOK, searchResponse{
				List:      []engine.SearchResult{},
				Page:      page,
				LastIndex: -1,
				HasMore:   false,
			})
			return
		}
		result, err := searchSingleSourcePage(c.Request.Context(), sources[0], req.Keyword, page)
		if err != nil {
			s.recordSourceFailure(userID, sources[0], err)
			writeSourceError(c, http.StatusBadGateway, "failed to search source", err, "search")
			return
		}
		c.JSON(http.StatusOK, searchResponse{
			List:      result.Items,
			Page:      result.Page,
			LastIndex: -1,
			HasMore:   result.HasMore,
		})
		return
	}

	lastIndex := -1
	if req.LastIndex != nil {
		lastIndex = *req.LastIndex
	}
	results, nextIndex := s.concurrentSearchFrom(
		c.Request.Context(),
		userID,
		sources,
		req.Keyword,
		req.ConcurrentCount,
		lastIndex,
		normalizedSearchSize(req.SearchSize),
		activeFailures,
	)
	c.JSON(http.StatusOK, searchResponse{
		List:      results,
		Page:      page,
		LastIndex: nextIndex,
		HasMore:   hasActiveSearchSourceAfter(sources, activeFailures, nextIndex),
	})
}

func (s *Server) concurrentSearch(parent context.Context, userID uint, sources []models.BookSource, keyword string, concurrentCount int, activeFailures map[uint]models.SourceFailure) []engine.SearchResult {
	results, _ := s.concurrentSearchFrom(parent, userID, sources, keyword, concurrentCount, -1, 0, activeFailures)
	return results
}

func (s *Server) concurrentSearchFrom(parent context.Context, userID uint, sources []models.BookSource, keyword string, concurrentCount, lastIndex, searchSize int, activeFailures map[uint]models.SourceFailure) ([]engine.SearchResult, int) {
	start := lastIndex + 1
	if start < 0 {
		start = 0
	}
	if start >= len(sources) {
		return []engine.SearchResult{}, len(sources) - 1
	}
	seen := make(map[string]bool)
	aggregated := make([]engine.SearchResult, 0)
	nextIndex := lastIndex

	for start < len(sources) {
		remaining := len(sources) - start
		limit := normalizedConcurrentCount(concurrentCount, remaining)
		batch := make([]models.BookSource, 0, limit)
		end := start
		for end < len(sources) && len(batch) < limit {
			source := sources[end]
			end += 1
			if _, suppressed := activeFailures[source.ID]; suppressed {
				continue
			}
			batch = append(batch, source)
		}
		nextIndex = end - 1
		if len(batch) == 0 {
			start = end
			continue
		}
		outcomes := searchSourceBatch(parent, batch, keyword, len(batch))
		for _, outcome := range outcomes {
			if outcome.Error != nil {
				if outcome.Index >= 0 && outcome.Index < len(batch) {
					s.recordSourceFailure(userID, batch[outcome.Index], outcome.Error)
				}
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
		if searchSize > 0 && len(aggregated) >= searchSize {
			break
		}
		start = end
	}
	return aggregated, nextIndex
}

func hasActiveSearchSourceAfter(sources []models.BookSource, activeFailures map[uint]models.SourceFailure, lastIndex int) bool {
	for index := lastIndex + 1; index < len(sources); index++ {
		if _, suppressed := activeFailures[sources[index].ID]; !suppressed {
			return true
		}
	}
	return false
}

type searchOutcome struct {
	Index   int
	Results []engine.SearchResult
	Error   error
}

func searchSourceBatch(parent context.Context, sources []models.BookSource, keyword string, concurrentCount int) []searchOutcome {
	var wg sync.WaitGroup
	channel := make(chan searchOutcome, len(sources))
	timeout := 15 * time.Second
	limit := normalizedConcurrentCount(concurrentCount, len(sources))
	workerGate := make(chan struct{}, limit)

	for index, source := range sources {
		wg.Add(1)
		source := source
		go func(index int) {
			defer wg.Done()
			select {
			case workerGate <- struct{}{}:
			case <-parent.Done():
				channel <- searchOutcome{Index: index, Error: parent.Err()}
				return
			}
			defer func() { <-workerGate }()
			ctx, cancel := context.WithTimeout(parent, timeout)
			results, err := engine.SearchBooksContext(ctx, source, keyword)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				err = errTimeout
			}
			channel <- searchOutcome{Index: index, Results: results, Error: err}
		}(index)
	}

	go func() {
		wg.Wait()
		close(channel)
	}()

	outcomes := make([]searchOutcome, len(sources))
	for outcome := range channel {
		outcomes[outcome.Index] = outcome
	}
	return outcomes
}

func searchSingleSourcePage(parent context.Context, source models.BookSource, keyword string, page int) (engine.SearchPageResult, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	result, err := engine.SearchBooksPageContext(ctx, source, keyword, page)
	if errors.Is(err, context.DeadlineExceeded) {
		return engine.SearchPageResult{}, errTimeout
	}
	return result, err
}

func normalizedConcurrentCount(value, sourceCount int) int {
	if value <= 0 {
		value = 24
	}
	if value > sourceCount {
		value = sourceCount
	}
	if value < 1 {
		value = 1
	}
	return value
}

func normalizedSearchPage(value *int) int {
	if value == nil || *value < 1 {
		return 1
	}
	return *value
}

func normalizedSearchSize(value *int) int {
	if value == nil || *value <= 0 {
		return 20
	}
	if *value > 200 {
		return 200
	}
	return *value
}

func orderSearchSources(sources []models.BookSource, sourceIDs []uint) []models.BookSource {
	if len(sourceIDs) == 0 {
		return sources
	}
	byID := make(map[uint]models.BookSource, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	ordered := make([]models.BookSource, 0, len(sources))
	seen := make(map[uint]bool, len(sources))
	for _, id := range sourceIDs {
		source, ok := byID[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, source)
	}
	return ordered
}

var errTimeout = &searchTimeoutError{}

type searchTimeoutError struct{}

func (e *searchTimeoutError) Error() string { return "search timeout" }

func (e *searchTimeoutError) Unwrap() error { return engine.ErrSourceRequest }
