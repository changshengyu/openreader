package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/chaptercache"
)

type cacheStreamProgress struct {
	BookID       uint `json:"bookId"`
	ChapterIndex int  `json:"chapterIndex"`
	Processed    int  `json:"processed"`
	Total        int  `json:"total"`
	CachedCount  int  `json:"cachedCount"`
	SuccessCount int  `json:"successCount"`
	FailedCount  int  `json:"failedCount"`
	Cached       int  `json:"cached"`
	Requested    int  `json:"requested"`
	Failed       int  `json:"failed"`
}

// cacheBookContentStream mirrors reader-dev's cacheBookSSE interaction while
// retaining OpenReader's authenticated JSON request. Explicit windows remain
// bounded while BookManage can request the whole catalogue. The request
// context is the job lifetime: a fetch AbortController or browser disconnect
// cancels source work before another chapter is scheduled.
func (s *Server) cacheBookContentStream(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}
	var request cacheBookRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cache payload"})
		return
	}
	if !request.All && request.ChapterIndex == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapterIndex is required"})
		return
	}
	if book.SourceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local books do not need server cache"})
		return
	}
	var sourceCount int64
	if err := s.db.Model(&models.BookSource{}).Where("id = ?", book.SourceID).Count(&sourceCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify book source"})
		return
	}
	if sourceCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book source not found"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ctx := c.Request.Context()
	result, err := s.cacheBookChapters(ctx, book, request.ChapterIndex, request.All, request.Count, request.Refresh, func(progress cacheStreamProgress) error {
		progress.BookID = book.ID
		return writeCacheStreamEvent(c, "message", progress)
	})
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	if err != nil {
		_ = writeCacheStreamEvent(c, "error", gin.H{"bookId": book.ID, "error": "缓存章节失败"})
		return
	}
	if result.SelectedCached == 0 && result.FailedCount > 0 {
		payload := cacheStreamPayload(book.ID, result)
		payload["error"] = "未能缓存章节内容"
		_ = writeCacheStreamEvent(c, "error", payload)
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	item := s.broadcastBookShelfUpdate(userID, book)
	payload := cacheStreamPayload(book.ID, result)
	payload["book"] = item
	_ = writeCacheStreamEvent(c, "end", payload)
}

func cacheStreamPayload(bookID uint, result chaptercache.Progress) gin.H {
	return gin.H{
		"bookId":       bookID,
		"chapterIndex": result.ChapterIndex,
		"processed":    result.Processed,
		"total":        result.Total,
		"cachedCount":  result.CachedCount,
		"successCount": result.SuccessCount,
		"failedCount":  result.FailedCount,
		"cached":       result.SelectedCached,
		"requested":    result.Processed,
		"failed":       result.FailedCount,
	}
}

func cacheStreamProgressFromResult(result chaptercache.Progress) cacheStreamProgress {
	return cacheStreamProgress{
		ChapterIndex: result.ChapterIndex,
		Processed:    result.Processed,
		Total:        result.Total,
		CachedCount:  result.CachedCount,
		SuccessCount: result.SuccessCount,
		FailedCount:  result.FailedCount,
		Cached:       result.SelectedCached,
		Requested:    result.Processed,
		Failed:       result.FailedCount,
	}
}

func (s *Server) cacheBookChapters(
	ctx context.Context,
	book models.Book,
	chapterIndex *int,
	all bool,
	count int,
	refresh bool,
	onProgress func(cacheStreamProgress) error,
) (chaptercache.Progress, error) {
	var catalogue []models.Chapter
	if err := s.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&catalogue).Error; err != nil {
		return chaptercache.Progress{}, err
	}

	existingByChapterID := make(map[uint]bool, len(catalogue))
	staleChapterIDs := make([]uint, 0)
	initialCached := 0
	for i := range catalogue {
		valid, stale := verifiedChapterCache(s, book, catalogue[i])
		if valid {
			existingByChapterID[catalogue[i].ID] = true
			initialCached++
		} else if stale {
			staleChapterIDs = append(staleChapterIDs, catalogue[i].ID)
			catalogue[i].CachePath = ""
		}
	}
	if len(staleChapterIDs) > 0 {
		if err := s.db.Model(&models.Chapter{}).
			Where("book_id = ? AND id IN ?", book.ID, staleChapterIDs).
			Update("cache_path", "").Error; err != nil {
			return chaptercache.Progress{}, err
		}
	}

	selected := selectCacheChapters(catalogue, chapterIndex, all, count)
	var source models.BookSource
	if len(selected) > 0 {
		if err := s.db.First(&source, book.SourceID).Error; err != nil {
			return chaptercache.Progress{}, err
		}
	}
	items := make([]chaptercache.Item, 0, len(selected))
	chapterByIndex := make(map[int]*models.Chapter, len(selected))
	for i := range selected {
		items = append(items, chaptercache.Item{
			Index:    selected[i].Index,
			Existing: existingByChapterID[selected[i].ID],
		})
		chapterByIndex[selected[i].Index] = &selected[i]
	}

	return chaptercache.Run(ctx, items, initialCached, refresh, func(ctx context.Context, item chaptercache.Item) error {
		chapter := chapterByIndex[item.Index]
		content, err := s.loadChapterTextContextResultWithOptions(ctx, &book, chapter, refresh)
		if err != nil {
			return err
		}
		if strings.TrimSpace(content) == "" {
			return errors.New("empty chapter content")
		}
		if _, imageErr := s.chapterImages.CacheChapter(ctx, source, book, *chapter, content); imageErr != nil {
			if errors.Is(imageErr, context.Canceled) || errors.Is(imageErr, context.DeadlineExceeded) {
				return imageErr
			}
		}
		return nil
	}, func(progress chaptercache.Progress) error {
		if onProgress == nil {
			return nil
		}
		return onProgress(cacheStreamProgressFromResult(progress))
	})
}

func verifiedChapterCache(s *Server, book models.Book, chapter models.Chapter) (valid bool, stale bool) {
	if strings.TrimSpace(chapter.CachePath) == "" {
		return false, false
	}
	content, _, err := s.readChapterCache(book, chapter.CachePath)
	if err != nil {
		return false, errors.Is(err, os.ErrNotExist)
	}
	if strings.TrimSpace(string(content)) == "" {
		return false, true
	}
	return true, false
}

func selectCacheChapters(catalogue []models.Chapter, chapterIndex *int, all bool, count int) []models.Chapter {
	start := 0
	if chapterIndex != nil {
		start = len(catalogue)
		for i := range catalogue {
			if catalogue[i].Index >= *chapterIndex {
				start = i
				break
			}
		}
	}
	if !all {
		if chapterIndex == nil || start >= len(catalogue) || catalogue[start].Index != *chapterIndex {
			return nil
		}
		return append([]models.Chapter(nil), catalogue[start])
	}
	end := len(catalogue)
	if count > 0 {
		if count > 300 {
			count = 300
		}
		if start+count < end {
			end = start + count
		}
	}
	if start >= end {
		return nil
	}
	return append([]models.Chapter(nil), catalogue[start:end]...)
}

func writeCacheStreamEvent(c *gin.Context, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
