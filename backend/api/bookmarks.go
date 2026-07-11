package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

const (
	maxBookmarkTitleBytes   = 320
	maxBookmarkExcerptBytes = 16 * 1024
	maxBookmarkNoteBytes    = 16 * 1024
)

type bookmarkRequest struct {
	ChapterID    uint    `json:"chapterId"`
	ChapterIndex int     `json:"chapterIndex"`
	Offset       int     `json:"offset"`
	Percent      float64 `json:"percent"`
	Title        string  `json:"title"`
	Excerpt      string  `json:"excerpt"`
	Note         string  `json:"note"`
}

func (s *Server) listBookmarks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var bookmarks []models.Bookmark
	if err := s.db.Where("user_id = ? AND book_id = ?", userID, bookID).
		Order("id asc").
		Find(&bookmarks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bookmarks"})
		return
	}
	c.JSON(http.StatusOK, bookmarks)
}

func (s *Server) createBookmark(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	var request bookmarkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark payload"})
		return
	}

	bookmark, err := s.bookmarkFromRequest(userID, book, request)
	if err != nil {
		writeBookmarkValidationError(c, err)
		return
	}

	if err := s.db.Create(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create bookmark"})
		return
	}
	s.broadcastBookmarksUpdate(userID, "create", bookmark.BookID, gin.H{"bookmark": bookmark})
	c.JSON(http.StatusCreated, bookmark)
}

func (s *Server) bookmarkFromRequest(userID uint, book models.Book, request bookmarkRequest) (models.Bookmark, error) {
	bookmark := models.Bookmark{
		UserID:       userID,
		BookID:       book.ID,
		ChapterID:    request.ChapterID,
		ChapterIndex: request.ChapterIndex,
		Offset:       request.Offset,
		Percent:      request.Percent,
		Title:        strings.TrimSpace(request.Title),
		Excerpt:      strings.TrimSpace(request.Excerpt),
		Note:         strings.TrimSpace(request.Note),
	}
	if bookmark.ChapterIndex < 0 {
		bookmark.ChapterIndex = 0
	}
	if bookmark.Offset < 0 {
		bookmark.Offset = 0
	}
	if bookmark.Percent < 0 {
		bookmark.Percent = 0
	} else if bookmark.Percent > 1 {
		bookmark.Percent = 1
	}
	if bookmark.Excerpt == "" {
		return models.Bookmark{}, errors.New("bookmark context is required")
	}
	if len(bookmark.Title) > maxBookmarkTitleBytes ||
		len(bookmark.Excerpt) > maxBookmarkExcerptBytes ||
		len(bookmark.Note) > maxBookmarkNoteBytes {
		return models.Bookmark{}, errors.New("bookmark is too large")
	}
	if bookmark.ChapterID > 0 {
		var chapter models.Chapter
		if err := s.db.Where("id = ? AND book_id = ?", bookmark.ChapterID, book.ID).First(&chapter).Error; err != nil {
			return models.Bookmark{}, errors.New("bookmark chapter not found")
		}
		bookmark.ChapterIndex = chapter.Index
	}
	if bookmark.Title == "" {
		bookmark.Title = "书签"
	}
	return bookmark, nil
}

func (s *Server) createBookmarks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	var requests []bookmarkRequest
	if err := c.ShouldBindJSON(&requests); err != nil || len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmarks payload"})
		return
	}

	bookmarks := make([]models.Bookmark, 0, len(requests))
	for _, request := range requests {
		bookmark, err := s.bookmarkFromRequest(userID, book, request)
		if err != nil {
			writeBookmarkValidationError(c, err)
			return
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return tx.CreateInBatches(&bookmarks, 100).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create bookmarks"})
		return
	}
	s.broadcastBookmarksUpdate(userID, "create_many", bookID, gin.H{"bookmarks": bookmarks})
	c.JSON(http.StatusCreated, bookmarks)
}

func (s *Server) updateBookmark(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookmarkID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var bookmark models.Bookmark
	if err := s.db.Where("user_id = ? AND id = ?", userID, bookmarkID).First(&bookmark).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}

	var request bookmarkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark payload"})
		return
	}

	note := strings.TrimSpace(request.Note)
	if len(note) > maxBookmarkNoteBytes {
		writeBookmarkValidationError(c, errors.New("bookmark is too large"))
		return
	}
	// Reader-dev's form exposes the saved reading context as read-only. Keep
	// the ID-backed OpenReader location/context immutable on note edits too.
	bookmark.Note = note

	if err := s.db.Save(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update bookmark"})
		return
	}
	s.broadcastBookmarksUpdate(userID, "update", bookmark.BookID, gin.H{"bookmark": bookmark})
	c.JSON(http.StatusOK, bookmark)
}

func writeBookmarkValidationError(c *gin.Context, err error) {
	message := "invalid bookmark payload"
	if err != nil {
		message = err.Error()
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": message})
}

func (s *Server) deleteBookmark(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookmarkID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var bookmark models.Bookmark
	if err := s.db.Where("user_id = ? AND id = ?", userID, bookmarkID).First(&bookmark).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}

	result := s.db.Delete(&bookmark)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete bookmark"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}
	s.broadcastBookmarksUpdate(userID, "delete", bookmark.BookID, gin.H{"id": bookmarkID})
	c.Status(http.StatusNoContent)
}

type bookmarkBatchDeleteRequest struct {
	IDs []uint `json:"ids"`
}

func (s *Server) deleteBookmarks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var request bookmarkBatchDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark ids"})
		return
	}
	ids := uniquePositiveUintIDs(request.IDs)
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bookmark ids are required"})
		return
	}

	var bookmarks []models.Bookmark
	if err := s.db.Where("user_id = ? AND book_id = ? AND id IN ?", userID, bookID, ids).Find(&bookmarks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load bookmarks"})
		return
	}
	deletedSet := make(map[uint]struct{}, len(bookmarks))
	for _, bookmark := range bookmarks {
		deletedSet[bookmark.ID] = struct{}{}
	}
	deletedIDs := make([]uint, 0, len(bookmarks))
	for _, id := range ids {
		if _, ok := deletedSet[id]; ok {
			deletedIDs = append(deletedIDs, id)
		}
	}
	if len(deletedIDs) > 0 {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return tx.Where("user_id = ? AND book_id = ? AND id IN ?", userID, bookID, deletedIDs).
				Delete(&models.Bookmark{}).Error
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete bookmarks"})
			return
		}
		s.broadcastBookmarksUpdate(userID, "delete_many", bookID, gin.H{"deletedIds": deletedIDs})
	}
	c.JSON(http.StatusOK, gin.H{"deletedIds": deletedIDs})
}

func (s *Server) broadcastBookmarksUpdate(userID uint, kind string, bookID uint, payload gin.H) {
	if s.hub == nil {
		return
	}
	if payload == nil {
		payload = gin.H{}
	}
	payload["kind"] = kind
	payload["bookId"] = bookID
	_ = s.hub.Broadcast(userID, nil, gin.H{
		"type":    "bookmarks_update",
		"payload": payload,
	})
}
