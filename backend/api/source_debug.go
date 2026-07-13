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

type testSearchRequest struct {
	Keyword string `json:"keyword" binding:"required"`
}

func (s *Server) testSourceSearch(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	var req testSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is required"})
		return
	}

	results, err := engine.SearchBooks(source, strings.TrimSpace(req.Keyword))
	if err != nil {
		userID, _ := middleware.UserID(c)
		s.recordSourceFailure(userID, source, err)
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "error": errToString(err)})
}

type testChapterRequest struct {
	BookURL string `json:"bookUrl" binding:"required"`
}

func (s *Server) testSourceChapter(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	var req testChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bookUrl is required"})
		return
	}

	chapters, err := engine.ParseTOC(strings.TrimSpace(req.BookURL), source)
	if err != nil {
		userID, _ := middleware.UserID(c)
		s.recordSourceFailure(userID, source, err)
	}
	c.JSON(http.StatusOK, gin.H{"chapters": chapters, "count": len(chapters), "error": errToString(err)})
}

type testContentRequest struct {
	ChapterURL string `json:"chapterUrl" binding:"required"`
}

func (s *Server) testSourceContent(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	var req testContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapterUrl is required"})
		return
	}

	content, err := engine.FetchChapterContent(strings.TrimSpace(req.ChapterURL), source)
	if err != nil {
		userID, _ := middleware.UserID(c)
		s.recordSourceFailure(userID, source, err)
	}
	preview := content
	if len([]rune(preview)) > 2000 {
		preview = string([]rune(preview)[:2000]) + "..."
	}
	c.JSON(http.StatusOK, gin.H{"content": preview, "fullLength": len([]rune(content)), "error": errToString(err)})
}

type batchTestSourcesRequest struct {
	SourceIDs  []uint `json:"sourceIds"`
	Keyword    string `json:"keyword"`
	TimeoutMS  int    `json:"timeout"`
	Concurrent int    `json:"concurrent"`
}

type batchTestSourceResult struct {
	SourceID uint   `json:"sourceId"`
	Name     string `json:"name"`
	Group    string `json:"group"`
	Enabled  bool   `json:"enabled"`
	OK       bool   `json:"ok"`
	Count    int    `json:"count"`
	Message  string `json:"message"`
}

func (s *Server) batchTestSources(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req batchTestSourcesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch test payload"})
		return
	}
	keyword := strings.TrimSpace(req.Keyword)
	if keyword == "" {
		keyword = "测试"
	}
	concurrent := req.Concurrent
	if concurrent < 3 {
		concurrent = 3
	}
	if concurrent > 15 {
		concurrent = 15
	}
	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	if timeoutMS < 1000 {
		timeoutMS = 1000
	}
	if timeoutMS > 15000 {
		timeoutMS = 15000
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	parentCtx := c.Request.Context()

	var sources []models.BookSource
	query := s.db.Model(&models.BookSource{})
	if len(req.SourceIDs) > 0 {
		query = query.Where("id IN ?", req.SourceIDs)
	} else {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Order("custom_order asc, id asc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}

	results := make([]batchTestSourceResult, len(sources))
	failureCauses := make([]error, len(sources))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrent)
	for index, source := range sources {
		wg.Add(1)
		go func(index int, source models.BookSource) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(parentCtx, timeout)
			searchResults, err := engine.SearchBooksContext(ctx, source, keyword)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				err = errTimeout
			}
			failureCauses[index] = err
			results[index] = batchTestSourceResult{
				SourceID: source.ID,
				Name:     source.Name,
				Group:    source.Group,
				Enabled:  source.Enabled,
				OK:       err == nil,
				Count:    len(searchResults),
				Message:  errToString(err),
			}
			if err == nil {
				results[index].Message = "可用"
			}
		}(index, source)
	}
	wg.Wait()
	for index, cause := range failureCauses {
		if cause != nil {
			s.recordSourceFailure(userID, sources[index], cause)
		}
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
