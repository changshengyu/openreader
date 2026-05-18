package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
)

func (s *Server) listBooks(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var books []models.Book
	query := s.db.Where("user_id = ?", userID)
	if categoryID := strings.TrimSpace(c.Query("categoryId")); categoryID != "" {
		if categoryID == "none" {
			query = query.Where("category_id IS NULL")
		} else {
			query = query.Where("category_id = ?", categoryID)
		}
	}
	if err := query.Order("updated_at desc").Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list books"})
		return
	}

	bookIDs := make([]uint, 0, len(books))
	for _, book := range books {
		bookIDs = append(bookIDs, book.ID)
	}
	var progresses []models.ReadingProgress
	if len(bookIDs) > 0 {
		_ = s.db.Where("user_id = ? AND book_id IN ?", userID, bookIDs).Find(&progresses).Error
	}
	progressByBookID := make(map[uint]models.ReadingProgress, len(progresses))
	for _, progress := range progresses {
		progressByBookID[progress.BookID] = progress
	}

	type bookListItem struct {
		models.Book
		Progress *models.ReadingProgress `json:"progress,omitempty"`
	}
	items := make([]bookListItem, 0, len(books))
	for _, book := range books {
		item := bookListItem{Book: book}
		if progress, ok := progressByBookID[book.ID]; ok {
			item.Progress = &progress
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) createBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var book models.Book
	if err := c.ShouldBindJSON(&book); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	book.UserID = userID
	book.Title = strings.TrimSpace(book.Title)
	if book.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book title is required"})
		return
	}
	if !s.validateCategory(c, userID, book.CategoryID) {
		return
	}

	if err := s.db.Create(&book).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create book"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusCreated, book)
}

func (s *Server) getBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, book)
}

type bookUpdateRequest struct {
	Title      string `json:"title"`
	Author     string `json:"author"`
	CoverURL   string `json:"coverUrl"`
	Intro      string `json:"intro"`
	CategoryID *uint  `json:"categoryId"`
}

func (s *Server) updateBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	var request bookUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	if !s.validateCategory(c, userID, request.CategoryID) {
		return
	}

	if title := strings.TrimSpace(request.Title); title != "" {
		book.Title = title
	}
	book.Author = strings.TrimSpace(request.Author)
	book.CoverURL = strings.TrimSpace(request.CoverURL)
	book.Intro = strings.TrimSpace(request.Intro)
	book.CategoryID = request.CategoryID

	if err := s.db.Save(&book).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update book"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusOK, book)
}

func (s *Server) deleteBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return deleteBookRecords(tx, userID, bookID, &book)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete book"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_delete", "payload": gin.H{"id": bookID}})
	c.Status(http.StatusNoContent)
}

type batchBooksRequest struct {
	Action     string `json:"action" binding:"required"`
	BookIDs    []uint `json:"bookIds" binding:"required"`
	CategoryID *uint  `json:"categoryId"`
}

type bookIDsRequest struct {
	BookIDs []uint `json:"bookIds" binding:"required"`
}

func (s *Server) batchBooks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var request batchBooksRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch payload"})
		return
	}
	if len(request.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bookIds is required"})
		return
	}
	if len(request.BookIDs) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many books"})
		return
	}
	if request.Action == "cache" {
		s.batchCacheBooks(c, userID, request.BookIDs)
		return
	}
	if request.Action == "clear-cache" {
		s.batchClearBookCache(c, userID, request.BookIDs)
		return
	}
	if request.Action == "category" && !s.validateCategory(c, userID, request.CategoryID) {
		return
	}

	var affected int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		switch request.Action {
		case "delete":
			var books []models.Book
			if err := tx.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Find(&books).Error; err != nil {
				return err
			}
			for i := range books {
				if err := deleteBookRecords(tx, userID, books[i].ID, &books[i]); err != nil {
					return err
				}
				affected++
			}
		case "category":
			result := tx.Model(&models.Book{}).
				Where("user_id = ? AND id IN ?", userID, request.BookIDs).
				Update("category_id", request.CategoryID)
			if result.Error != nil {
				return result.Error
			}
			affected = result.RowsAffected
		default:
			return fmt.Errorf("unsupported batch action")
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
	c.JSON(http.StatusOK, gin.H{"affected": affected})
}

func (s *Server) batchCacheBooks(c *gin.Context, userID uint, bookIDs []uint) {
	if len(bookIDs) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch cache supports up to 50 books at a time"})
		return
	}

	var books []models.Book
	if err := s.db.Where("user_id = ? AND id IN ?", userID, bookIDs).Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load books"})
		return
	}

	cached := 0
	requested := 0
	failed := 0
	for i := range books {
		bookCached, bookRequested, err := s.cacheBookChapters(books[i], nil, true)
		cached += bookCached
		requested += bookRequested
		if err != nil {
			failed++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"affected":  len(books),
		"cached":    cached,
		"requested": requested,
		"failed":    failed,
	})
}

func (s *Server) batchClearBookCache(c *gin.Context, userID uint, bookIDs []uint) {
	if len(bookIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clear cache supports up to 100 books at a time"})
		return
	}

	var books []models.Book
	if err := s.db.Where("user_id = ? AND id IN ?", userID, bookIDs).Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load books"})
		return
	}

	cleared := 0
	for _, book := range books {
		bookCleared, err := s.clearBookCache(book.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cache"})
			return
		}
		cleared += bookCleared
	}

	c.JSON(http.StatusOK, gin.H{
		"affected": len(books),
		"cleared":  cleared,
	})
}

func (s *Server) exportBooks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var request bookIDsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bookIds is required"})
		return
	}
	if len(request.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bookIds is required"})
		return
	}
	if len(request.BookIDs) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many books"})
		return
	}

	var books []models.Book
	if err := s.db.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Order("updated_at desc").Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load books"})
		return
	}

	type exportedBook struct {
		Book      models.Book       `json:"book"`
		Chapters  []models.Chapter  `json:"chapters"`
		Bookmarks []models.Bookmark `json:"bookmarks"`
	}

	exported := make([]exportedBook, 0, len(books))
	for _, book := range books {
		var chapters []models.Chapter
		if err := s.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load chapters"})
			return
		}
		var bookmarks []models.Bookmark
		if err := s.db.Where("user_id = ? AND book_id = ?", userID, book.ID).Order("updated_at desc").Find(&bookmarks).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load bookmarks"})
			return
		}
		exported = append(exported, exportedBook{
			Book:      book,
			Chapters:  chapters,
			Bookmarks: bookmarks,
		})
	}

	c.Header("Content-Disposition", `attachment; filename="openreader-books.json"`)
	c.JSON(http.StatusOK, gin.H{
		"version":    1,
		"exportedAt": time.Now().UTC(),
		"count":      len(exported),
		"books":      exported,
	})
}

func (s *Server) refreshBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}
	if book.SourceID == 0 || strings.TrimSpace(book.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only remote books can be refreshed"})
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, book.SourceID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}
	remoteChapters, err := engine.ParseTOC(book.URL, source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to fetch chapters: %v", err)})
		return
	}
	if len(remoteChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	var added int
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing []models.Chapter
		if err := tx.Where("book_id = ?", book.ID).Find(&existing).Error; err != nil {
			return err
		}
		existingByIndex := make(map[int]models.Chapter, len(existing))
		for _, chapter := range existing {
			existingByIndex[chapter.Index] = chapter
		}
		for _, remoteChapter := range remoteChapters {
			if chapter, ok := existingByIndex[remoteChapter.Index]; ok {
				chapter.Title = remoteChapter.Title
				chapter.URL = remoteChapter.URL
				if err := tx.Save(&chapter).Error; err != nil {
					return err
				}
				continue
			}
			chapter := models.Chapter{
				BookID: book.ID,
				Index:  remoteChapter.Index,
				Title:  remoteChapter.Title,
				URL:    remoteChapter.URL,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			added++
		}
		book.LastChapter = remoteChapters[len(remoteChapters)-1].Title
		book.ChapterCount = len(remoteChapters)
		return tx.Save(&book).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh book"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusOK, gin.H{"book": book, "added": added, "chapterCount": len(remoteChapters)})
}

type cacheBookRequest struct {
	ChapterIndex *int `json:"chapterIndex"`
	All          bool `json:"all"`
}

func (s *Server) cacheBookContent(c *gin.Context) {
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
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cache payload"})
		return
	}

	if !request.All && request.ChapterIndex == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapterIndex is required"})
		return
	}
	cached, requested, err := s.cacheBookChapters(book, request.ChapterIndex, request.All)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list chapters"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cached": cached, "requested": requested})
}

func (s *Server) cacheBookChapters(book models.Book, chapterIndex *int, all bool) (int, int, error) {
	query := s.db.Where("book_id = ?", book.ID).Order("`index` asc")
	if all {
		query = query.Limit(300)
	} else {
		query = query.Where("`index` = ?", *chapterIndex)
	}

	var chapters []models.Chapter
	if err := query.Find(&chapters).Error; err != nil {
		return 0, 0, err
	}
	cached := 0
	for i := range chapters {
		content := s.loadChapterText(book, &chapters[i])
		if content != "" {
			cached++
		}
	}
	return cached, len(chapters), nil
}

func (s *Server) clearBookCache(bookID uint) (int, error) {
	var chapters []models.Chapter
	if err := s.db.Where("book_id = ? AND cache_path <> ''", bookID).Find(&chapters).Error; err != nil {
		return 0, err
	}

	cleared := 0
	for i := range chapters {
		if s.deleteCacheFile(chapters[i].CachePath) {
			cleared++
		}
		chapters[i].CachePath = ""
		if err := s.db.Save(&chapters[i]).Error; err != nil {
			return cleared, err
		}
	}
	return cleared, nil
}

func (s *Server) deleteCacheFile(cachePath string) bool {
	cachePath = strings.TrimSpace(cachePath)
	if cachePath == "" {
		return false
	}
	fullPath := cachePath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(s.cfg.CacheDir, cachePath)
	}
	cleanPath, err := filepath.Abs(fullPath)
	if err != nil {
		return false
	}
	cleanCacheDir, err := filepath.Abs(s.cfg.CacheDir)
	if err != nil {
		return false
	}
	if cleanPath != cleanCacheDir && !strings.HasPrefix(cleanPath, cleanCacheDir+string(os.PathSeparator)) {
		return false
	}
	if err := os.Remove(cleanPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func deleteBookRecords(tx *gorm.DB, userID, bookID uint, book *models.Book) error {
	if err := tx.Where("book_id = ?", bookID).Delete(&models.Chapter{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Delete(&models.Bookmark{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Delete(&models.ReadingProgress{}).Error; err != nil {
		return err
	}
	return tx.Delete(book).Error
}

type bookCategoryRequest struct {
	CategoryID *uint `json:"categoryId"`
}

func (s *Server) updateBookCategory(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	var request bookCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category payload"})
		return
	}
	if !s.validateCategory(c, userID, request.CategoryID) {
		return
	}

	book.CategoryID = request.CategoryID
	if err := s.db.Save(&book).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update category"})
		return
	}
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusOK, book)
}

func (s *Server) listChapters(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var chapters []models.Chapter
	if err := s.db.Where("book_id = ?", bookID).Order("`index` asc").Find(&chapters).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list chapters"})
		return
	}
	c.JSON(http.StatusOK, chapters)
}

type remoteBookRequest struct {
	Title      string `json:"title" binding:"required"`
	Author     string `json:"author"`
	CoverURL   string `json:"coverUrl"`
	Intro      string `json:"intro"`
	BookURL    string `json:"bookUrl" binding:"required"`
	SourceID   uint   `json:"sourceId" binding:"required"`
	SourceName string `json:"sourceName"`
	CategoryID *uint  `json:"categoryId"`
}

func (s *Server) createRemoteBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var req remoteBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, bookUrl, and sourceId are required"})
		return
	}
	if !s.validateCategory(c, userID, req.CategoryID) {
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, req.SourceID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}

	var existing models.Book
	if err := s.db.Where("user_id = ? AND url = ?", userID, strings.TrimSpace(req.BookURL)).First(&existing).Error; err == nil {
		if req.CategoryID != nil {
			existing.CategoryID = req.CategoryID
			_ = s.db.Save(&existing).Error
		}
		c.JSON(http.StatusOK, existing)
		return
	}

	chapters, err := engine.ParseTOC(req.BookURL, source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to fetch chapters: %v", err)})
		return
	}
	if len(chapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	book := models.Book{
		UserID:       userID,
		SourceID:     req.SourceID,
		Title:        strings.TrimSpace(req.Title),
		Author:       strings.TrimSpace(req.Author),
		CoverURL:     strings.TrimSpace(req.CoverURL),
		Intro:        strings.TrimSpace(req.Intro),
		URL:          req.BookURL,
		CategoryID:   req.CategoryID,
		LastChapter:  chapters[len(chapters)-1].Title,
		ChapterCount: len(chapters),
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&book).Error; err != nil {
			return err
		}
		for _, ch := range chapters {
			chapter := models.Chapter{
				BookID: book.ID,
				Index:  ch.Index,
				Title:  ch.Title,
				URL:    ch.URL,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create book"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusCreated, book)
}

type changeSourceRequest struct {
	SourceID uint   `json:"sourceId" binding:"required"`
	BookURL  string `json:"bookUrl"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	CoverURL string `json:"coverUrl"`
	Intro    string `json:"intro"`
}

func (s *Server) listBookSourceCandidates(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	var sources []models.BookSource
	if err := s.db.Where("enabled = ?", true).Limit(80).Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sources"})
		return
	}

	type sourceCandidate struct {
		SourceID   uint   `json:"sourceId"`
		SourceName string `json:"sourceName"`
		Group      string `json:"group"`
		Title      string `json:"title"`
		Author     string `json:"author"`
		CoverURL   string `json:"coverUrl"`
		Intro      string `json:"intro"`
		BookURL    string `json:"bookUrl"`
		Current    bool   `json:"current"`
	}

	results := make([]sourceCandidate, 0)
	channel := make(chan []sourceCandidate, len(sources))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, source := range sources {
		source := source
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			done := make(chan []sourceCandidate, 1)
			go func() {
				searchResults, err := engine.SearchBooks(source, book.Title)
				if err != nil {
					done <- nil
					return
				}
				candidates := make([]sourceCandidate, 0)
				for _, item := range searchResults {
					if item.BookURL == "" {
						continue
					}
					candidates = append(candidates, sourceCandidate{
						SourceID:   source.ID,
						SourceName: source.Name,
						Group:      source.Group,
						Title:      item.Title,
						Author:     item.Author,
						CoverURL:   item.CoverURL,
						Intro:      item.Intro,
						BookURL:    item.BookURL,
						Current:    source.ID == book.SourceID && item.BookURL == book.URL,
					})
					if len(candidates) >= 3 {
						break
					}
				}
				done <- candidates
			}()
			select {
			case candidates := <-done:
				channel <- candidates
			case <-time.After(12 * time.Second):
				channel <- nil
			}
		}()
	}
	go func() {
		wg.Wait()
		close(channel)
	}()
	for candidates := range channel {
		results = append(results, candidates...)
		if len(results) >= 120 {
			break
		}
	}

	c.JSON(http.StatusOK, results)
}

func (s *Server) changeBookSource(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	var req changeSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourceId is required"})
		return
	}

	var newSource models.BookSource
	if err := s.db.First(&newSource, req.SourceID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}

	newBookURL := strings.TrimSpace(req.BookURL)
	if newBookURL == "" {
		newBookURL = book.URL
	}
	newChapters, err := engine.ParseTOC(newBookURL, newSource)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to fetch chapters from new source: %v", err)})
		return
	}
	if len(newChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("book_id = ?", bookID).Delete(&models.Chapter{}).Error; err != nil {
			return err
		}
		for _, ch := range newChapters {
			chapter := models.Chapter{
				BookID: bookID,
				Index:  ch.Index,
				Title:  ch.Title,
				URL:    ch.URL,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
		}
		book.SourceID = req.SourceID
		book.URL = newBookURL
		if title := strings.TrimSpace(req.Title); title != "" {
			book.Title = title
		}
		if author := strings.TrimSpace(req.Author); author != "" {
			book.Author = author
		}
		if coverURL := strings.TrimSpace(req.CoverURL); coverURL != "" {
			book.CoverURL = coverURL
		}
		if intro := strings.TrimSpace(req.Intro); intro != "" {
			book.Intro = intro
		}
		book.LastChapter = newChapters[len(newChapters)-1].Title
		book.ChapterCount = len(newChapters)
		return tx.Save(&book).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change source"})
		return
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": book})
	c.JSON(http.StatusOK, book)
}

func (s *Server) chapterContent(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter index"})
		return
	}

	var chapter models.Chapter
	err = s.db.Where("book_id = ? AND `index` = ?", bookID, index).First(&chapter).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "chapter not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load chapter"})
		return
	}

	content := s.loadChapterText(book, &chapter)

	c.JSON(http.StatusOK, gin.H{
		"chapter": chapter,
		"content": content,
	})
}

func (s *Server) searchBookContent(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}

	keyword := strings.TrimSpace(c.Query("q"))
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}

	var chapters []models.Chapter
	if err := s.db.Where("book_id = ?", bookID).Order("`index` asc").Find(&chapters).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list chapters"})
		return
	}

	type contentMatch struct {
		ChapterID    uint    `json:"chapterId"`
		ChapterIndex int     `json:"chapterIndex"`
		ChapterTitle string  `json:"chapterTitle"`
		Excerpt      string  `json:"excerpt"`
		Offset       int     `json:"offset"`
		LineIndex    int     `json:"lineIndex"`
		Percent      float64 `json:"percent"`
	}

	matches := make([]contentMatch, 0)
	for i := range chapters {
		content := s.loadChapterText(book, &chapters[i])
		if content == "" {
			continue
		}
		positions := searchContentPositions(content, keyword, 6)
		for _, position := range positions {
			matches = append(matches, contentMatch{
				ChapterID:    chapters[i].ID,
				ChapterIndex: chapters[i].Index,
				ChapterTitle: chapters[i].Title,
				Excerpt:      excerptAround(content, position, keyword),
				Offset:       position,
				LineIndex:    lineIndexAtByte(content, position),
				Percent:      float64(position) / float64(max(len(content), 1)),
			})
			if len(matches) >= 200 {
				break
			}
		}
		if len(matches) >= 200 {
			break
		}
	}

	c.JSON(http.StatusOK, matches)
}

func searchContentPositions(content string, keyword string, limit int) []int {
	if content == "" || keyword == "" || limit <= 0 {
		return nil
	}
	lowerContent := strings.ToLower(content)
	needle := strings.ToLower(keyword)
	positions := make([]int, 0)
	for offset := 0; offset < len(lowerContent) && len(positions) < limit; {
		position := strings.Index(lowerContent[offset:], needle)
		if position < 0 {
			break
		}
		absolute := offset + position
		positions = append(positions, absolute)
		offset = absolute + len(needle)
	}
	if len(positions) > 0 {
		return positions
	}

	normalizedContent, contentMap := normalizeSearchText(content)
	normalizedKeyword, _ := normalizeSearchText(keyword)
	if normalizedKeyword == "" {
		return nil
	}
	for offset := 0; offset < len(normalizedContent) && len(positions) < limit; {
		position := strings.Index(normalizedContent[offset:], normalizedKeyword)
		if position < 0 {
			break
		}
		absolute := offset + position
		if absolute >= 0 && absolute < len(contentMap) {
			positions = append(positions, contentMap[absolute])
		}
		offset = absolute + len(normalizedKeyword)
	}
	return positions
}

func normalizeSearchText(value string) (string, []int) {
	var builder strings.Builder
	bytePositions := make([]int, 0, len(value))
	for position, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		lower := strings.ToLower(string(r))
		builder.WriteString(lower)
		for range []byte(lower) {
			bytePositions = append(bytePositions, position)
		}
	}
	return builder.String(), bytePositions
}

func lineIndexAtByte(content string, bytePosition int) int {
	if bytePosition <= 0 {
		return 0
	}
	if bytePosition > len(content) {
		bytePosition = len(content)
	}
	lineIndex := 0
	for _, r := range content[:bytePosition] {
		if r == '\n' {
			lineIndex++
		}
	}
	return lineIndex
}

func excerptAround(content string, bytePosition int, keyword string) string {
	runes := []rune(content)
	center := utf8.RuneCountInString(content[:bytePosition])
	keywordWidth := utf8.RuneCountInString(keyword)
	start := center - 42
	if start < 0 {
		start = 0
	}
	end := center + keywordWidth + 82
	if end > len(runes) {
		end = len(runes)
	}
	return strings.TrimSpace(string(runes[start:end]))
}

func (s *Server) loadChapterText(book models.Book, chapter *models.Chapter) string {
	content := ""
	if chapter.CachePath != "" {
		path := chapter.CachePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.cfg.CacheDir, path)
		}
		if bytes, err := os.ReadFile(path); err == nil {
			content = string(bytes)
		}
	}

	if content == "" && chapter.URL != "" && book.SourceID > 0 {
		var source models.BookSource
		if err := s.db.First(&source, book.SourceID).Error; err == nil {
			fetched, fetchErr := engine.FetchChapterContent(chapter.URL, source)
			if fetchErr == nil && fetched != "" {
				content = fetched
				cachePath, cacheErr := engine.WriteChapterCache(s.cfg.CacheDir, book.URL, chapter.URL, content)
				if cacheErr == nil {
					chapter.CachePath = cachePath
					_ = s.db.Save(chapter)
				}
			}
		}
	}
	content = s.applyUserReplaceRules(book.UserID, content)
	return content
}

func (s *Server) applyUserReplaceRules(userID uint, content string) string {
	if content == "" {
		return content
	}
	var rules []models.ReplaceRule
	if err := s.db.Where("user_id = ? AND enabled = ?", userID, true).Find(&rules).Error; err != nil {
		return content
	}
	replacements := make([]models.TextReplaceRule, 0, len(rules))
	for _, rule := range rules {
		replacements = append(replacements, models.TextReplaceRule{
			Pattern:     rule.Pattern,
			Replacement: rule.Replacement,
		})
	}
	return engine.ApplyTextReplacements(content, replacements)
}

func (s *Server) checkUpdates(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	count := s.scheduler.CheckNow()
	if count > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
	}
	c.JSON(http.StatusOK, gin.H{"newChapters": count})
}
