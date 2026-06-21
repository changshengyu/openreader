package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
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
		Order("chapter_index asc, offset asc, created_at asc").
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
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var request bookmarkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark payload"})
		return
	}

	bookmark := bookmarkFromRequest(userID, bookID, request)

	if err := s.db.Create(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create bookmark"})
		return
	}
	s.broadcastBookmarksUpdate(userID, "create", bookmark.BookID, gin.H{"bookmark": bookmark})
	c.JSON(http.StatusCreated, bookmark)
}

func bookmarkFromRequest(userID, bookID uint, request bookmarkRequest) models.Bookmark {
	bookmark := models.Bookmark{
		UserID:       userID,
		BookID:       bookID,
		ChapterID:    request.ChapterID,
		ChapterIndex: request.ChapterIndex,
		Offset:       request.Offset,
		Percent:      request.Percent,
		Title:        strings.TrimSpace(request.Title),
		Excerpt:      strings.TrimSpace(request.Excerpt),
		Note:         strings.TrimSpace(request.Note),
	}
	if bookmark.Title == "" {
		bookmark.Title = "书签"
	}
	return bookmark
}

func (s *Server) createBookmarks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var requests []bookmarkRequest
	if err := c.ShouldBindJSON(&requests); err != nil || len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmarks payload"})
		return
	}

	bookmarks := make([]models.Bookmark, 0, len(requests))
	for _, request := range requests {
		bookmarks = append(bookmarks, bookmarkFromRequest(userID, bookID, request))
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

	if title := strings.TrimSpace(request.Title); title != "" {
		bookmark.Title = title
	}
	bookmark.Excerpt = strings.TrimSpace(request.Excerpt)
	bookmark.Note = strings.TrimSpace(request.Note)

	if err := s.db.Save(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update bookmark"})
		return
	}
	s.broadcastBookmarksUpdate(userID, "update", bookmark.BookID, gin.H{"bookmark": bookmark})
	c.JSON(http.StatusOK, bookmark)
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
