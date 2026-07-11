package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

type cacheStreamProgress struct {
	BookID       uint `json:"bookId"`
	Cached       int  `json:"cached"`
	Requested    int  `json:"requested"`
	Total        int  `json:"total"`
	ChapterIndex int  `json:"chapterIndex"`
	Failed       int  `json:"failed"`
}

// cacheBookContentStream mirrors reader-dev's cacheBookSSE interaction while
// retaining OpenReader's authenticated JSON request and cache bounds. The
// request context is the job lifetime: a fetch AbortController or browser
// disconnect cancels source work before another chapter is scheduled.
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

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ctx := c.Request.Context()
	cached, requested, failed, err := s.cacheBookChaptersStream(ctx, book, request.ChapterIndex, request.All, request.Count, func(progress cacheStreamProgress) error {
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
	if cached == 0 && failed > 0 {
		_ = writeCacheStreamEvent(c, "error", gin.H{
			"bookId":    book.ID,
			"cached":    cached,
			"requested": requested,
			"failed":    failed,
			"error":     "未能缓存章节内容",
		})
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	item := s.broadcastBookShelfUpdate(userID, book)
	_ = writeCacheStreamEvent(c, "end", gin.H{
		"bookId":    book.ID,
		"cached":    cached,
		"requested": requested,
		"failed":    failed,
		"book":      item,
	})
}

func (s *Server) cacheBookChaptersStream(ctx context.Context, book models.Book, chapterIndex *int, all bool, count int, onProgress func(cacheStreamProgress) error) (int, int, int, error) {
	query := s.db.Where("book_id = ?", book.ID).Order("`index` asc")
	if all {
		if chapterIndex != nil {
			query = query.Where("`index` >= ?", *chapterIndex)
		}
		if count <= 0 {
			count = 50
		}
		if count > 300 {
			count = 300
		}
		query = query.Limit(count)
	} else {
		query = query.Where("`index` = ?", *chapterIndex)
	}

	var chapters []models.Chapter
	if err := query.Find(&chapters).Error; err != nil {
		return 0, 0, 0, err
	}
	cached := 0
	failed := 0
	for offset := range chapters {
		if err := ctx.Err(); err != nil {
			return cached, len(chapters), failed, err
		}
		content := s.loadChapterTextContext(ctx, book, &chapters[offset])
		if err := ctx.Err(); err != nil {
			return cached, len(chapters), failed, err
		}
		if content == "" {
			failed++
		} else {
			cached++
		}
		if onProgress != nil {
			if err := onProgress(cacheStreamProgress{
				Cached:       cached,
				Requested:    offset + 1,
				Total:        len(chapters),
				ChapterIndex: chapters[offset].Index,
				Failed:       failed,
			}); err != nil {
				return cached, len(chapters), failed, err
			}
		}
	}
	return cached, len(chapters), failed, nil
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
