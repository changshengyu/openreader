package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

func (s *Server) cacheStats(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	stats, err := s.remoteCacheStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count cached chapters"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"files":          stats.files,
		"size":           stats.size,
		"cachedChapters": stats.chapters,
		"scope":          "current-user",
	})
}

func (s *Server) clearCache(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var books []models.Book
	if err := s.db.Where("user_id = ? AND source_id > 0", userID).Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list cached books"})
		return
	}
	bookIDs := make([]uint, 0, len(books))
	for _, book := range books {
		bookIDs = append(bookIDs, book.ID)
	}
	var cachePaths []string
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		_, paths, err := s.clearRemoteBookCacheRows(tx, bookIDs)
		cachePaths = paths
		return err
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset chapter cache state"})
		return
	}
	files, size := s.pruneUnreferencedRemoteCachePaths(cachePaths)
	for _, book := range books {
		imageStats, imageErr := s.chapterImages.RemoveBook(book)
		if imageErr == nil {
			files += imageStats.Files
			size += imageStats.Bytes
		}
	}
	if coverStats, coverErr := s.coverImages.RemoveUser(userID); coverErr == nil {
		files += coverStats.Files
		size += coverStats.Bytes
	}
	if items, err := s.listAllBookShelfItems(userID); err == nil {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
	} else {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
	}

	c.JSON(http.StatusOK, gin.H{"clearedFiles": files, "clearedSize": size})
}

type cacheStatSummary struct {
	files    int
	size     int64
	chapters int64
}

func (s *Server) remoteCacheStats(userID uint) (cacheStatSummary, error) {
	s.remoteCacheMu.Lock()
	defer s.remoteCacheMu.Unlock()

	var chapters []models.Chapter
	if err := s.db.
		Joins("JOIN books ON books.id = chapters.book_id").
		Where("books.user_id = ? AND books.source_id > 0 AND chapters.cache_path <> ''", userID).
		Find(&chapters).Error; err != nil {
		return cacheStatSummary{}, err
	}

	seen := map[string]struct{}{}
	summary := cacheStatSummary{}
	limit := s.remoteChapterCacheReadLimit()
	for _, chapter := range chapters {
		opened, err := s.openRemoteCacheFile(chapter.CachePath)
		if err != nil {
			continue
		}
		_ = opened.file.Close()
		if opened.info.Size() <= 0 || opened.info.Size() > limit {
			continue
		}
		summary.chapters++
		if _, exists := seen[opened.relative]; exists {
			continue
		}
		seen[opened.relative] = struct{}{}
		summary.files++
		summary.size += opened.info.Size()
	}
	imageStats, err := s.chapterImages.StatsUser(userID)
	if err != nil {
		return cacheStatSummary{}, err
	}
	summary.files += imageStats.Files
	summary.size += imageStats.Bytes
	coverStats, err := s.coverImages.StatsUser(userID)
	if err != nil {
		return cacheStatSummary{}, err
	}
	summary.files += coverStats.Files
	summary.size += coverStats.Bytes
	return summary, nil
}
