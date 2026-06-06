package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
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
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book id"})
		return
	}
	if _, ok := s.ensureBook(c, userID, uint(bookID)); !ok {
		return
	}

	var progress models.ReadingProgress
	err = s.db.Where("user_id = ? AND book_id = ?", userID, bookID).First(&progress).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load progress"})
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
	progressBook, ok := s.ensureBook(c, userID, request.BookID)
	if !ok {
		return
	}

	progress := models.ReadingProgress{
		UserID:         userID,
		BookID:         request.BookID,
		ChapterID:      request.ChapterID,
		ChapterIndex:   request.ChapterIndex,
		Offset:         request.Offset,
		Percent:        clampProgressPercent(request.Percent),
		ChapterPercent: clampProgressPercent(request.ChapterPercent),
		ChapterTitle:   request.ChapterTitle,
		Mode:           request.Mode,
		UpdatedAt:      time.Now(),
	}

	var existing models.ReadingProgress
	err := s.db.Where("user_id = ? AND book_id = ?", userID, request.BookID).First(&existing).Error
	if err == nil && isStaleProgressUpdate(existing.UpdatedAt, request.BaseUpdatedAt, request.ClientUpdatedAt) {
		c.Header("X-OpenReader-Progress-Conflict", "1")
		c.JSON(http.StatusOK, existing)
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save progress"})
		return
	}

	err = s.db.Where("user_id = ? AND book_id = ?", userID, request.BookID).
		Assign(progress).
		FirstOrCreate(&progress).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save progress"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{
		"type":    "progress_update",
		"payload": progressBroadcast{ReadingProgress: progress, ClientID: request.ClientID, Book: s.bookShelfListItem(userID, progressBook)},
	})

	c.JSON(http.StatusOK, progress)
}

func isStaleProgressUpdate(serverUpdatedAt time.Time, baseUpdatedAt string, clientUpdatedAt string) bool {
	if baseUpdatedAt == "" || serverUpdatedAt.IsZero() {
		return isServerNewerThanClient(serverUpdatedAt, clientUpdatedAt)
	}
	base, err := time.Parse(time.RFC3339Nano, baseUpdatedAt)
	if err != nil {
		return isServerNewerThanClient(serverUpdatedAt, clientUpdatedAt)
	}
	return serverUpdatedAt.After(base)
}

func isServerNewerThanClient(serverUpdatedAt time.Time, clientUpdatedAt string) bool {
	if clientUpdatedAt == "" || serverUpdatedAt.IsZero() {
		return false
	}
	clientTime, err := time.Parse(time.RFC3339Nano, clientUpdatedAt)
	if err != nil {
		return false
	}
	return serverUpdatedAt.After(clientTime)
}

func clampProgressPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 1 {
		return 1
	}
	return percent
}
