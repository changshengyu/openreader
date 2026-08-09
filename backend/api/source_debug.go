package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/booksources"
	"openreader/backend/services/sourcedebug"
)

type testSearchRequest struct {
	Keyword string `json:"keyword" binding:"required"`
}

func (s *Server) testSourceSearch(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	userID, _ := middleware.UserID(c)
	source, err := s.bookSources.FindActive(userID, uint(id))
	if errors.Is(err, booksources.ErrSourceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	var req testSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is required"})
		return
	}

	results, err := engine.SearchBooks(source, strings.TrimSpace(req.Keyword))
	c.JSON(http.StatusOK, sourceDebugPayload(gin.H{"results": results}, err, "search"))
}

type testChapterRequest struct {
	BookURL string `json:"bookUrl" binding:"required"`
}

func (s *Server) testSourceChapter(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	userID, _ := middleware.UserID(c)
	source, err := s.bookSources.FindActive(userID, uint(id))
	if errors.Is(err, booksources.ErrSourceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	var req testChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bookUrl is required"})
		return
	}

	chapters, err := engine.ParseTOC(strings.TrimSpace(req.BookURL), source)
	c.JSON(http.StatusOK, sourceDebugPayload(gin.H{"chapters": chapters, "count": len(chapters)}, err, "toc"))
}

type testContentRequest struct {
	ChapterURL string `json:"chapterUrl" binding:"required"`
}

func (s *Server) testSourceContent(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	userID, _ := middleware.UserID(c)
	source, err := s.bookSources.FindActive(userID, uint(id))
	if errors.Is(err, booksources.ErrSourceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	var req testContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapterUrl is required"})
		return
	}

	content, err := engine.FetchChapterContent(strings.TrimSpace(req.ChapterURL), source)
	preview := content
	if len([]rune(preview)) > 2000 {
		preview = string([]rune(preview)[:2000]) + "..."
	}
	c.JSON(http.StatusOK, sourceDebugPayload(gin.H{"content": preview, "fullLength": len([]rune(content))}, err, "content"))
}

type sourceDebugStreamRequest struct {
	Keyword string `json:"keyword"`
}

const maxSourceDebugRequestBytes = 16 * 1024

func (s *Server) debugSourceStream(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	userID, _ := middleware.UserID(c)
	source, err := s.bookSources.FindActive(userID, uint(id))
	if errors.Is(err, booksources.ErrSourceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSourceDebugRequestBytes)
	var req sourceDebugStreamRequest
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source debug payload"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source debug payload"})
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	startedAt := time.Now()
	sequence := 0
	emit := func(event sourcedebug.Event) error {
		if err := c.Request.Context().Err(); err != nil {
			return err
		}
		sequence++
		data := make(map[string]any, len(event.Data)+2)
		for key, value := range event.Data {
			data[key] = value
		}
		data["seq"] = sequence
		data["elapsedMs"] = time.Since(startedAt).Milliseconds()
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Name, encoded); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	result, err := sourcedebug.Run(c.Request.Context(), source, req.Keyword, emit)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	if err != nil {
		stage := sourcedebug.ErrorStage(err)
		details := sourceErrorDetailsFor(err, stage)
		message := sourceErrorMessage(err)
		data := map[string]any{
			"stage":   stage,
			"status":  "error",
			"message": message,
			"error":   message,
		}
		if details.Code != "" {
			data["code"] = details.Code
		}
		_ = emit(sourcedebug.Event{Name: "error", Data: data})
		return
	}
	_ = emit(sourcedebug.Event{Name: "end", Data: map[string]any{
		"status":        "success",
		"message":       "调试完成",
		"contentLength": result.ContentLength,
	}})
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
	Code     string `json:"code,omitempty"`
	Stage    string `json:"stage,omitempty"`
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

	sources, err := s.bookSources.ListActiveByIDs(userID, req.SourceIDs, len(req.SourceIDs) == 0)
	if err != nil {
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
				Message:  sourceErrorMessage(err),
			}
			if details := sourceErrorDetailsFor(err, "search"); details.Code != "" {
				results[index].Code = details.Code
				results[index].Stage = details.Stage
			}
			if err == nil {
				results[index].Message = "可用"
			}
		}(index, source)
	}
	wg.Wait()
	for index, cause := range failureCauses {
		if cause != nil {
			s.recordSourceHealthFailure(userID, sources[index], cause)
		}
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}
