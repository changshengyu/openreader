package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/readingprogress"
)

type progressRequest struct {
	BookID          uint    `json:"bookId" binding:"required"`
	ChapterID       uint    `json:"chapterId"`
	ChapterIndex    int     `json:"chapterIndex"`
	Offset          int     `json:"offset"`
	Percent         float64 `json:"percent"`
	ChapterPercent  float64 `json:"chapterPercent"`
	ChapterTitle    string  `json:"chapterTitle"`
	Mode            string  `json:"mode"`
	BaseUpdatedAt   string  `json:"baseUpdatedAt"`
	ClientUpdatedAt string  `json:"clientUpdatedAt"`
	ClientID        string  `json:"clientId"`
}

type progressBroadcast struct {
	models.ReadingProgress
	ClientID string       `json:"clientId,omitempty"`
	Book     bookListItem `json:"book,omitempty"`
}

func (s *Server) getProgress(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, err := strconv.Atoi(c.Param("bookID"))
	if err != nil || bookID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book id"})
		return
	}
	progress, found, err := s.progressSvc.Get(userID, uint(bookID))
	if errors.Is(err, readingprogress.ErrBookNotFound) {
		notFound(c, "book not found")
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load progress"})
		return
	}
	if !found {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	c.JSON(http.StatusOK, progress)
}

func (s *Server) updateProgress(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var request progressRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid progress payload"})
		return
	}
	result, err := s.progressSvc.Save(readingprogress.Input{
		UserID:          userID,
		BookID:          request.BookID,
		ChapterID:       request.ChapterID,
		ChapterIndex:    request.ChapterIndex,
		Offset:          request.Offset,
		Percent:         request.Percent,
		ChapterPercent:  request.ChapterPercent,
		Mode:            request.Mode,
		BaseUpdatedAt:   request.BaseUpdatedAt,
		ClientUpdatedAt: request.ClientUpdatedAt,
	})
	if errors.Is(err, readingprogress.ErrBookNotFound) {
		notFound(c, "book not found")
		return
	}
	if errors.Is(err, readingprogress.ErrInvalidProgress) ||
		errors.Is(err, readingprogress.ErrChapterNotFound) ||
		errors.Is(err, readingprogress.ErrChapterIdentity) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid progress payload"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save progress"})
		return
	}
	if result.Conflict {
		c.Header("X-OpenReader-Progress-Conflict", "1")
		c.JSON(http.StatusOK, result.Progress)
		return
	}
	if result.MirrorStatus == readingprogress.MirrorFailed {
		c.Header("X-OpenReader-Progress-WebDAV", "failed")
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{
		"type":    "progress_update",
		"payload": progressBroadcast{ReadingProgress: result.Progress, ClientID: request.ClientID, Book: s.bookShelfListItem(userID, result.Book)},
	})

	c.JSON(http.StatusOK, result.Progress)
}

func isStaleProgressUpdate(serverUpdatedAt time.Time, baseUpdatedAt string, clientUpdatedAt string) bool {
	return readingprogress.IsStaleUpdate(serverUpdatedAt, baseUpdatedAt, clientUpdatedAt)
}

func clampProgressPercent(percent float64) float64 {
	return readingprogress.ClampPercent(percent)
}
