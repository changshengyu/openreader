package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	"openreader/backend/services/audioreader"
	"openreader/backend/services/cbzreader"
	"openreader/backend/services/epubreader"
)

type bookListItem struct {
	models.Book
	CategoryIDs        []uint                  `json:"categoryIds"`
	Progress           *models.ReadingProgress `json:"progress,omitempty"`
	ShelfOrderAt       time.Time               `json:"shelfOrderAt"`
	CachedChapterCount int64                   `json:"cachedChapterCount"`
}

func (s *Server) listBooks(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var books []models.Book
	query := s.db.Where("user_id = ?", userID)
	if err := query.Order("updated_at desc").Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list books"})
		return
	}
	if categoryID := strings.TrimSpace(c.Query("categoryId")); categoryID != "" {
		categoryIDsByBookID := s.bookCategoryIDsByBookID(userID, books)
		filtered := make([]models.Book, 0, len(books))
		for _, book := range books {
			if bookMatchesCategoryFilter(book, categoryIDsByBookID[book.ID], categoryID) {
				filtered = append(filtered, book)
			}
		}
		books = filtered
	}

	c.JSON(http.StatusOK, s.bookShelfListItems(userID, books))
}

func (s *Server) bookShelfListItem(userID uint, book models.Book) bookListItem {
	var progress models.ReadingProgress
	err := s.db.Where("user_id = ? AND book_id = ?", userID, book.ID).First(&progress).Error
	cachedCount := s.cachedChapterCount(book.ID, book.SourceID)
	categoryIDs := s.bookCategoryIDs(userID, book)
	if err != nil {
		return bookShelfListItem(book, categoryIDs, models.ReadingProgress{}, cachedCount)
	}
	return bookShelfListItem(book, categoryIDs, progress, cachedCount)
}

func (s *Server) listAllBookShelfItems(userID uint) ([]bookListItem, error) {
	var books []models.Book
	if err := s.db.Where("user_id = ?", userID).Find(&books).Error; err != nil {
		return nil, err
	}
	return s.bookShelfListItems(userID, books), nil
}

func (s *Server) bookShelfListItems(userID uint, books []models.Book) []bookListItem {
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
	cacheCountByBookID := s.cachedChapterCounts(books)
	categoryIDsByBookID := s.bookCategoryIDsByBookID(userID, books)

	items := make([]bookListItem, 0, len(books))
	for _, book := range books {
		items = append(items, bookShelfListItem(book, categoryIDsByBookID[book.ID], progressByBookID[book.ID], cacheCountByBookID[book.ID]))
	}
	sort.SliceStable(items, func(i, j int) bool {
		iShelfAt := items[i].ShelfOrderAt
		jShelfAt := items[j].ShelfOrderAt
		if !iShelfAt.Equal(jShelfAt) {
			return iShelfAt.After(jShelfAt)
		}
		return items[i].ID > items[j].ID
	})
	return items
}

func bookShelfListItem(book models.Book, categoryIDs []uint, progress models.ReadingProgress, cachedChapterCount int64) bookListItem {
	item := bookListItem{Book: book, CategoryIDs: normalizeBookCategoryIDs(book, categoryIDs), CachedChapterCount: cachedChapterCount}
	if len(item.CategoryIDs) > 0 {
		primary := item.CategoryIDs[0]
		item.CategoryID = &primary
	}
	if progress.BookID != 0 {
		item.Progress = &progress
	}
	item.ShelfOrderAt = shelfOrderAt(item.Book, item.Progress)
	return item
}

func (s *Server) bookCategoryIDs(userID uint, book models.Book) []uint {
	return s.bookCategoryIDsByBookID(userID, []models.Book{book})[book.ID]
}

func (s *Server) bookCategoryIDsByBookID(userID uint, books []models.Book) map[uint][]uint {
	result := make(map[uint][]uint, len(books))
	if len(books) == 0 {
		return result
	}
	bookIDs := make([]uint, 0, len(books))
	legacyByBookID := make(map[uint]*uint, len(books))
	for _, book := range books {
		bookIDs = append(bookIDs, book.ID)
		legacyByBookID[book.ID] = book.CategoryID
	}
	var rows []models.BookCategory
	_ = s.db.Where("user_id = ? AND book_id IN ?", userID, bookIDs).Order("id asc").Find(&rows).Error
	for _, row := range rows {
		result[row.BookID] = append(result[row.BookID], row.CategoryID)
	}
	for _, book := range books {
		result[book.ID] = normalizeBookCategoryIDs(book, result[book.ID])
		if len(result[book.ID]) == 0 && legacyByBookID[book.ID] != nil && *legacyByBookID[book.ID] > 0 {
			result[book.ID] = []uint{*legacyByBookID[book.ID]}
		}
	}
	return result
}

func normalizeBookCategoryIDs(book models.Book, categoryIDs []uint) []uint {
	ids := uniquePositiveUintIDs(categoryIDs)
	if len(ids) == 0 && book.CategoryID != nil && *book.CategoryID > 0 {
		ids = append(ids, *book.CategoryID)
	}
	return ids
}

func bookMatchesCategoryFilter(book models.Book, categoryIDs []uint, categoryID string) bool {
	if categoryID == "none" {
		return len(normalizeBookCategoryIDs(book, categoryIDs)) == 0
	}
	for _, id := range normalizeBookCategoryIDs(book, categoryIDs) {
		if strconv.FormatUint(uint64(id), 10) == categoryID {
			return true
		}
	}
	return false
}

func categoryIDsFromRequest(categoryID *uint, categoryIDs []uint) []uint {
	ids := uniquePositiveUintIDs(categoryIDs)
	if len(ids) == 0 && categoryID != nil && *categoryID > 0 {
		ids = append(ids, *categoryID)
	}
	return ids
}

func requestCategoryIDs(request batchBooksRequest) []uint {
	return categoryIDsFromRequest(request.CategoryID, request.CategoryIDs)
}

func mergeCategoryID(book models.Book, categoryIDs []uint, categoryID *uint) []uint {
	ids := normalizeBookCategoryIDs(book, categoryIDs)
	if categoryID == nil || *categoryID == 0 {
		return ids
	}
	for _, id := range ids {
		if id == *categoryID {
			return ids
		}
	}
	return append(ids, *categoryID)
}

func removeCategoryID(book models.Book, categoryIDs []uint, categoryID *uint) []uint {
	if categoryID == nil || *categoryID == 0 {
		return normalizeBookCategoryIDs(book, categoryIDs)
	}
	ids := normalizeBookCategoryIDs(book, categoryIDs)
	next := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != *categoryID {
			next = append(next, id)
		}
	}
	return next
}

func (s *Server) setBookCategories(tx *gorm.DB, userID, bookID uint, categoryIDs []uint) error {
	ids := uniquePositiveUintIDs(categoryIDs)
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Delete(&models.BookCategory{}).Error; err != nil {
		return err
	}
	for _, categoryID := range ids {
		row := models.BookCategory{UserID: userID, BookID: bookID, CategoryID: categoryID}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) cachedChapterCount(bookID uint, sourceID uint) int64 {
	if sourceID == 0 {
		return 0
	}
	var count int64
	_ = s.db.Model(&models.Chapter{}).Where("book_id = ? AND cache_path <> ''", bookID).Count(&count).Error
	return count
}

func (s *Server) cachedChapterCounts(books []models.Book) map[uint]int64 {
	bookIDs := make([]uint, 0, len(books))
	for _, book := range books {
		if book.SourceID > 0 {
			bookIDs = append(bookIDs, book.ID)
		}
	}
	if len(bookIDs) == 0 {
		return map[uint]int64{}
	}
	type row struct {
		BookID uint
		Count  int64
	}
	var rows []row
	_ = s.db.Model(&models.Chapter{}).
		Select("book_id, COUNT(*) as count").
		Where("book_id IN ? AND cache_path <> ''", bookIDs).
		Group("book_id").
		Scan(&rows).Error
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.BookID] = row.Count
	}
	return counts
}

func (s *Server) broadcastBookShelfUpdate(userID uint, book models.Book) bookListItem {
	item := s.bookShelfListItem(userID, book)
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": item})
	return item
}

func shelfOrderAt(book models.Book, progress *models.ReadingProgress) time.Time {
	orderAt := book.UpdatedAt
	if book.CreatedAt.After(orderAt) {
		orderAt = book.CreatedAt
	}
	if progress != nil && progress.UpdatedAt.After(orderAt) {
		orderAt = progress.UpdatedAt
	}
	return orderAt
}

func (s *Server) createBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var book models.Book
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	if err := json.Unmarshal(data, &book); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	var request struct {
		CategoryIDs []uint `json:"categoryIds"`
	}
	_ = json.Unmarshal(data, &request)
	book.UserID = userID
	book.Title = strings.TrimSpace(book.Title)
	if book.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book title is required"})
		return
	}
	if len(request.CategoryIDs) > 0 {
		if !s.validateCategoryIDs(c, userID, request.CategoryIDs) {
			return
		}
	} else if !s.validateCategory(c, userID, book.CategoryID) {
		return
	}
	categoryIDs := categoryIDsFromRequest(book.CategoryID, request.CategoryIDs)
	if len(categoryIDs) > 0 {
		book.CategoryID = &categoryIDs[0]
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&book).Error; err != nil {
			return err
		}
		return s.setBookCategories(tx, userID, book.ID, categoryIDs)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create book"})
		return
	}
	c.JSON(http.StatusCreated, s.broadcastBookShelfUpdate(userID, book))
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
	c.JSON(http.StatusOK, s.bookShelfListItem(userID, book))
}

type bookUpdateRequest struct {
	Title          *string `json:"title"`
	Author         *string `json:"author"`
	CoverURL       *string `json:"coverUrl"`
	CustomCoverURL *string `json:"customCoverUrl"`
	Intro          *string `json:"intro"`
	CategoryID     *uint   `json:"categoryId"`
	CategoryIDs    []uint  `json:"categoryIds"`
	CanUpdate      *bool   `json:"canUpdate"`
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

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	var request bookUpdateRequest
	if err := json.Unmarshal(data, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid book payload"})
		return
	}
	_, categoryIDSet := raw["categoryId"]
	_, categoryIDsSet := raw["categoryIds"]
	if categoryIDSet && !s.validateCategory(c, userID, request.CategoryID) {
		return
	}
	if categoryIDsSet && !s.validateCategoryIDs(c, userID, request.CategoryIDs) {
		return
	}

	if request.Title != nil {
		title := strings.TrimSpace(*request.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "book title is required"})
			return
		}
		book.Title = title
	}
	if request.Author != nil {
		book.Author = strings.TrimSpace(*request.Author)
	}
	if request.CoverURL != nil {
		book.CoverURL = strings.TrimSpace(*request.CoverURL)
	}
	if request.CustomCoverURL != nil {
		book.CustomCoverURL = strings.TrimSpace(*request.CustomCoverURL)
	}
	if request.Intro != nil {
		book.Intro = strings.TrimSpace(*request.Intro)
	}
	if categoryIDSet {
		book.CategoryID = request.CategoryID
	}
	if categoryIDsSet {
		ids := uniquePositiveUintIDs(request.CategoryIDs)
		if len(ids) > 0 {
			book.CategoryID = &ids[0]
		} else {
			book.CategoryID = nil
		}
	}
	if request.CanUpdate != nil {
		book.CanUpdate = *request.CanUpdate
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&book).Error; err != nil {
			return err
		}
		if categoryIDsSet {
			return s.setBookCategories(tx, userID, book.ID, request.CategoryIDs)
		}
		if categoryIDSet {
			if request.CategoryID == nil {
				return s.setBookCategories(tx, userID, book.ID, nil)
			}
			return s.setBookCategories(tx, userID, book.ID, []uint{*request.CategoryID})
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update book"})
		return
	}
	c.JSON(http.StatusOK, s.broadcastBookShelfUpdate(userID, book))
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

	var cleanup bookCleanupPlan
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		cleanup, err = s.captureBookCleanup(tx, userID, book)
		if err != nil {
			return err
		}
		return deleteBookRecords(tx, userID, bookID, &book)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete book"})
		return
	}

	s.cleanupDeletedBookArtifacts([]bookCleanupPlan{cleanup})
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_delete", "payload": gin.H{"id": bookID}})
	c.Status(http.StatusNoContent)
}

type batchBooksRequest struct {
	Action      string `json:"action" binding:"required"`
	BookIDs     []uint `json:"bookIds" binding:"required"`
	CategoryID  *uint  `json:"categoryId"`
	CategoryIDs []uint `json:"categoryIds"`
}

type bookIDsRequest struct {
	BookIDs []uint `json:"bookIds" binding:"required"`
	Format  string `json:"format"`
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
	ownedBookIDs, ok := s.requireOwnedBookIDs(c, userID, request.BookIDs)
	if !ok {
		return
	}
	request.BookIDs = ownedBookIDs
	if request.Action == "cache" {
		s.batchCacheBooks(c, userID, request.BookIDs)
		return
	}
	if request.Action == "clear-cache" {
		s.batchClearBookCache(c, userID, request.BookIDs)
		return
	}
	switch request.Action {
	case "category":
		if len(request.CategoryIDs) > 0 {
			if !s.validateCategoryIDs(c, userID, request.CategoryIDs) {
				return
			}
		} else if !s.validateCategory(c, userID, request.CategoryID) {
			return
		}
	case "category-add", "category-remove":
		if !s.validateCategory(c, userID, request.CategoryID) {
			return
		}
	}

	var affected int64
	var deletedIDs []uint
	var updatedBooks []models.Book
	var cleanupPlans []bookCleanupPlan
	err := s.db.Transaction(func(tx *gorm.DB) error {
		switch request.Action {
		case "delete":
			var books []models.Book
			if err := tx.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Find(&books).Error; err != nil {
				return err
			}
			for i := range books {
				cleanup, err := s.captureBookCleanup(tx, userID, books[i])
				if err != nil {
					return err
				}
				deletedIDs = append(deletedIDs, books[i].ID)
				if err := deleteBookRecords(tx, userID, books[i].ID, &books[i]); err != nil {
					return err
				}
				cleanupPlans = append(cleanupPlans, cleanup)
				affected++
			}
		case "category", "category-add", "category-remove":
			if err := tx.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Find(&updatedBooks).Error; err != nil {
				return err
			}
			for i := range updatedBooks {
				nextIDs := requestCategoryIDs(request)
				if request.Action == "category-add" {
					nextIDs = mergeCategoryID(updatedBooks[i], s.bookCategoryIDs(userID, updatedBooks[i]), request.CategoryID)
				} else if request.Action == "category-remove" {
					nextIDs = removeCategoryID(updatedBooks[i], s.bookCategoryIDs(userID, updatedBooks[i]), request.CategoryID)
				}
				if err := s.setBookCategories(tx, userID, updatedBooks[i].ID, nextIDs); err != nil {
					return err
				}
				if len(nextIDs) > 0 {
					updatedBooks[i].CategoryID = &nextIDs[0]
				} else {
					updatedBooks[i].CategoryID = nil
				}
				if err := tx.Save(&updatedBooks[i]).Error; err != nil {
					return err
				}
				affected++
			}
		default:
			return fmt.Errorf("unsupported batch action")
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch request.Action {
	case "delete":
		s.cleanupDeletedBookArtifacts(cleanupPlans)
		if len(deletedIDs) > 0 {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_delete", "payload": gin.H{"ids": deletedIDs}})
		}
		c.JSON(http.StatusOK, gin.H{"affected": affected, "deletedIds": deletedIDs})
	case "category", "category-add", "category-remove":
		items := make([]bookListItem, 0, len(updatedBooks))
		for _, book := range updatedBooks {
			items = append(items, s.bookShelfListItem(userID, book))
		}
		if len(items) > 0 {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
		}
		c.JSON(http.StatusOK, gin.H{"affected": affected, "books": items})
	}
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
		if books[i].SourceID == 0 {
			continue
		}
		bookCached, bookRequested, err := s.cacheBookChapters(books[i], nil, true, 10)
		cached += bookCached
		requested += bookRequested
		if err != nil {
			failed++
		}
	}
	items := make([]bookListItem, 0, len(books))
	for _, book := range books {
		items = append(items, s.bookShelfListItem(userID, book))
	}
	if len(items) > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
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

	ownedBookIDs := make([]uint, 0, len(books))
	for _, book := range books {
		ownedBookIDs = append(ownedBookIDs, book.ID)
	}
	cleared := 0
	var cachePaths []string
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		cleared, cachePaths, err = s.clearRemoteBookCacheRows(tx, ownedBookIDs)
		return err
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cache"})
		return
	}
	s.pruneUnreferencedRemoteCachePaths(cachePaths)
	items := make([]bookListItem, 0, len(books))
	for _, book := range books {
		items = append(items, s.bookShelfListItem(userID, book))
	}
	if len(items) > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
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
	ownedBookIDs, ok := s.requireOwnedBookIDs(c, userID, request.BookIDs)
	if !ok {
		return
	}
	request.BookIDs = ownedBookIDs

	var books []models.Book
	if err := s.db.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Order("updated_at desc").Find(&books).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load books"})
		return
	}
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" || format == "json" {
		s.exportBooksJSON(c, userID, books)
		return
	}
	if len(books) == 1 && books[0].SourceID == 0 && (format == "txt" || format == "epub") {
		if s.exportOriginalLocalBook(c, books[0]) {
			return
		}
	}
	if format == "txt" {
		s.exportBooksTXT(c, books)
		return
	}
	if format == "epub" {
		s.exportBooksEPUB(c, books)
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported export format"})
}

func (s *Server) exportOriginalLocalBook(c *gin.Context, book models.Book) bool {
	path, ok := s.localBookSourcePath(book)
	if !ok {
		return false
	}
	if _, ok := relativePathInside(s.cfg.LibraryDir, path); !ok {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	setAttachmentHeader(c, filepath.Base(path))
	c.Data(http.StatusOK, "application/octet-stream", content)
	return true
}

func (s *Server) exportBooksJSON(c *gin.Context, userID uint, books []models.Book) {
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

func (s *Server) exportBooksTXT(c *gin.Context, books []models.Book) {
	if len(books) == 1 {
		book := books[0]
		content, err := s.exportBookPlainText(book)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
		filename := safeDownloadFilename(book.Title, "txt")
		setAttachmentHeader(c, filename)
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(content))
		return
	}

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	for _, book := range books {
		content, err := s.exportBookPlainText(book)
		if err != nil {
			_ = zipWriter.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
		writer, err := zipWriter.Create(safeDownloadFilename(fmt.Sprintf("%s-%d", book.Title, book.ID), "txt"))
		if err != nil {
			_ = zipWriter.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			_ = zipWriter.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
	}
	if err := zipWriter.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
		return
	}
	setAttachmentHeader(c, "openreader-books-txt.zip")
	c.Data(http.StatusOK, "application/zip", buffer.Bytes())
}

func (s *Server) exportBooksEPUB(c *gin.Context, books []models.Book) {
	if len(books) == 1 {
		book := books[0]
		content, err := s.exportBookEPUB(book)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
		filename := safeDownloadFilename(book.Title, "epub")
		setAttachmentHeader(c, filename)
		c.Data(http.StatusOK, "application/epub+zip", content)
		return
	}

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	for _, book := range books {
		content, err := s.exportBookEPUB(book)
		if err != nil {
			_ = zipWriter.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
		writer, err := zipWriter.Create(safeDownloadFilename(fmt.Sprintf("%s-%d", book.Title, book.ID), "epub"))
		if err != nil {
			_ = zipWriter.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
		if _, err := writer.Write(content); err != nil {
			_ = zipWriter.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
			return
		}
	}
	if err := zipWriter.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export book"})
		return
	}
	setAttachmentHeader(c, "openreader-books-epub.zip")
	c.Data(http.StatusOK, "application/zip", buffer.Bytes())
}

func (s *Server) exportBookPlainText(book models.Book) (string, error) {
	var chapters []models.Chapter
	if err := s.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		return "", err
	}
	var builder strings.Builder
	title := strings.TrimSpace(book.Title)
	if title != "" {
		builder.WriteString(title)
		builder.WriteString("\n")
	}
	author := strings.TrimSpace(book.Author)
	if author != "" {
		builder.WriteString("作者：")
		builder.WriteString(author)
		builder.WriteString("\n")
	}
	if title != "" || author != "" {
		builder.WriteString("\n")
	}
	for _, chapter := range chapters {
		chapterTitle := strings.TrimSpace(chapter.Title)
		if chapterTitle != "" {
			builder.WriteString(chapterTitle)
			builder.WriteString("\n\n")
		}
		content := strings.TrimSpace(s.loadChapterText(book, &chapter))
		if content != "" {
			builder.WriteString(content)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

type exportedChapterContent struct {
	Title   string
	Content string
}

func (s *Server) exportBookEPUB(book models.Book) ([]byte, error) {
	var chapters []models.Chapter
	if err := s.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		return nil, err
	}
	contents := make([]exportedChapterContent, 0, len(chapters))
	for _, chapter := range chapters {
		contents = append(contents, exportedChapterContent{
			Title:   strings.TrimSpace(chapter.Title),
			Content: strings.TrimSpace(s.loadChapterText(book, &chapter)),
		})
	}

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	if err := writeEPUBStoredFile(zipWriter, "mimetype", []byte("application/epub+zip")); err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	if err := writeEPUBFile(zipWriter, "META-INF/container.xml", []byte(epubContainerXML())); err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	if err := writeEPUBFile(zipWriter, "OEBPS/content.opf", []byte(epubContentOPF(book, contents))); err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	if err := writeEPUBFile(zipWriter, "OEBPS/nav.xhtml", []byte(epubNavXHTML(book, contents))); err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	for index, chapter := range contents {
		if err := writeEPUBFile(zipWriter, fmt.Sprintf("OEBPS/chapter-%04d.xhtml", index+1), []byte(epubChapterXHTML(book, chapter, index))); err != nil {
			_ = zipWriter.Close()
			return nil, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeEPUBStoredFile(zipWriter *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetModTime(time.Unix(0, 0))
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}

func writeEPUBFile(zipWriter *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(time.Unix(0, 0))
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}

func epubContainerXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
}

func epubContentOPF(book models.Book, chapters []exportedChapterContent) string {
	title := html.EscapeString(strings.TrimSpace(book.Title))
	if title == "" {
		title = "OpenReader Book"
	}
	author := html.EscapeString(strings.TrimSpace(book.Author))
	if author == "" {
		author = "Unknown"
	}
	var manifest strings.Builder
	var spine strings.Builder
	for index := range chapters {
		id := fmt.Sprintf("chapter-%04d", index+1)
		href := fmt.Sprintf("chapter-%04d.xhtml", index+1)
		manifest.WriteString(fmt.Sprintf(`    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`+"\n", id, href))
		spine.WriteString(fmt.Sprintf(`    <itemref idref="%s"/>`+"\n", id))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="book-id">openreader-book-%d</dc:identifier>
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dc:language>zh-CN</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
%s  </manifest>
  <spine>
%s  </spine>
</package>`, book.ID, title, author, manifest.String(), spine.String())
}

func epubNavXHTML(book models.Book, chapters []exportedChapterContent) string {
	title := html.EscapeString(strings.TrimSpace(book.Title))
	if title == "" {
		title = "OpenReader Book"
	}
	var items strings.Builder
	for index, chapter := range chapters {
		chapterTitle := html.EscapeString(strings.TrimSpace(chapter.Title))
		if chapterTitle == "" {
			chapterTitle = fmt.Sprintf("第%d章", index+1)
		}
		items.WriteString(fmt.Sprintf(`      <li><a href="chapter-%04d.xhtml">%s</a></li>`+"\n", index+1, chapterTitle))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="zh-CN">
<head><title>%s</title></head>
<body>
  <nav epub:type="toc" id="toc">
    <h1>%s</h1>
    <ol>
%s    </ol>
  </nav>
</body>
</html>`, title, title, items.String())
}

func epubChapterXHTML(book models.Book, chapter exportedChapterContent, index int) string {
	title := html.EscapeString(strings.TrimSpace(chapter.Title))
	if title == "" {
		title = fmt.Sprintf("第%d章", index+1)
	}
	var paragraphs strings.Builder
	for _, line := range strings.Split(chapter.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paragraphs.WriteString("    <p>")
		paragraphs.WriteString(html.EscapeString(line))
		paragraphs.WriteString("</p>\n")
	}
	bookTitle := html.EscapeString(strings.TrimSpace(book.Title))
	if bookTitle == "" {
		bookTitle = "OpenReader Book"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="zh-CN">
<head>
  <title>%s - %s</title>
  <style>body{line-height:1.8;font-family:serif;}p{text-indent:2em;margin:0 0 1em;}</style>
</head>
<body>
  <section>
    <h1>%s</h1>
%s  </section>
</body>
</html>`, bookTitle, title, title, paragraphs.String())
}

func safeDownloadFilename(name string, ext string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "openreader-book"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "", "<", "-", ">", "-", "|", "-", "\r", "", "\n", "")
	name = replacer.Replace(name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "openreader-book"
	}
	return name + "." + strings.TrimPrefix(ext, ".")
}

func setAttachmentHeader(c *gin.Context, filename string) {
	ascii := strings.Map(func(r rune) rune {
		if r > 127 {
			return -1
		}
		return r
	}, filename)
	if strings.TrimSpace(ascii) == "" || strings.HasPrefix(ascii, ".") {
		ascii = "openreader-export" + filepath.Ext(filename)
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(filename)))
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
	remoteInfo, remoteChapters, variable, err := engine.FetchBookInfoAndTOCWithVariables(book.URL, source, book.Variable, book.Title)
	if err != nil {
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadRequest, "failed to fetch chapters", err, "book_info")
		return
	}
	if len(remoteChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	var supersededCachePaths []string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		nextChapters := make([]models.Chapter, 0, len(remoteChapters))
		for _, remoteChapter := range remoteChapters {
			nextChapters = append(nextChapters, models.Chapter{
				BookID:   book.ID,
				Index:    remoteChapter.Index,
				Title:    remoteChapter.Title,
				URL:      remoteChapter.URL,
				IsVolume: remoteChapter.IsVolume,
				Tag:      remoteChapter.Tag,
				Variable: remoteChapter.Variable,
			})
		}
		var err error
		supersededCachePaths, _, err = s.replaceBookChapterRows(tx, userID, book.ID, nextChapters)
		if err != nil {
			return err
		}
		book.Title = firstNonBlankCanRename(remoteInfo.Title, book.Title, remoteInfo.CanRename)
		book.Author = firstNonBlankCanRename(remoteInfo.Author, book.Author, remoteInfo.CanRename)
		book.CoverURL = firstNonBlank(remoteInfo.CoverURL, book.CoverURL)
		book.Intro = firstNonBlank(remoteInfo.Intro, book.Intro)
		book.Kind = firstNonBlank(remoteInfo.Kind, book.Kind)
		book.WordCount = firstNonBlank(remoteInfo.WordCount, book.WordCount)
		book.LastChapter = remoteChapters[len(remoteChapters)-1].Title
		book.ChapterCount = len(remoteChapters)
		book.Variable = variable
		return tx.Save(&book).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh book"})
		return
	}
	s.pruneUnreferencedRemoteCachePaths(supersededCachePaths)

	c.JSON(http.StatusOK, gin.H{"book": s.broadcastBookShelfUpdate(userID, book), "added": len(remoteChapters), "chapterCount": len(remoteChapters)})
}

func (s *Server) refreshLocalBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	book, ok := s.ensureBook(c, userID, bookID)
	if !ok {
		return
	}
	if book.SourceID != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only local books can be refreshed"})
		return
	}

	sourcePath, ok := s.localBookSourcePath(book)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local source file not found"})
		return
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read local source file"})
		return
	}
	var request struct {
		TOCRule *string `json:"tocRule"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid local refresh payload"})
			return
		}
	}
	tocRule := strings.TrimSpace(book.TOCRule)
	if request.TOCRule != nil {
		tocRule = strings.TrimSpace(*request.TOCRule)
	}
	parsed, err := parseLocalBookChapters(filepath.Ext(sourcePath), data, tocRule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse local book: %v", err)})
		return
	}
	if len(parsed) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local book has no readable chapters"})
		return
	}

	bookURL := strings.TrimSpace(book.URL)
	if bookURL == "" {
		bookURL = fmt.Sprintf("local://book_%d", book.ID)
	}
	archiveRoot, _ := s.ownedLocalRefreshArchiveRoot(userID, book)
	stage, nextChapters, err := s.stageLocalRefresh(book, archiveRoot, parsed, bookURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stage local refreshed content"})
		return
	}
	defer stage.cleanup()

	archive := engine.ArchivedBook{
		Directory:    book.LibraryPath,
		OriginalFile: book.OriginalFile,
		TOCFile:      book.TOCFile,
		SourceFile:   book.SourceFile,
	}
	var supersededCachePaths []string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var nextChapterIDs map[int]uint
		var err error
		supersededCachePaths, nextChapterIDs, err = s.replaceBookChapterRows(tx, userID, book.ID, nextChapters)
		if err != nil {
			return err
		}
		book.URL = bookURL
		book.LastChapter = strings.TrimSpace(parsed[len(parsed)-1].Title)
		if book.LastChapter == "" {
			book.LastChapter = fmt.Sprintf("第 %d 章", len(parsed))
		}
		book.ChapterCount = len(parsed)
		book.TOCRule = tocRule
		book.Variable = ""
		if err := tx.Save(&book).Error; err != nil {
			return err
		}
		archivedChapters := make([]engine.ArchivedChapter, 0, len(parsed))
		for index, parsedChapter := range parsed {
			chapter := nextChapters[index]
			chapter.ID = nextChapterIDs[chapter.Index]
			archivedChapters = append(archivedChapters, engine.ArchivedChapter{
				ID:           chapter.ID,
				URL:          chapter.URL,
				Title:        chapter.Title,
				IsVolume:     false,
				BaseURL:      "",
				BookURL:      book.OriginalFile,
				Index:        chapter.Index,
				Start:        parsedChapter.Start,
				End:          parsedChapter.End,
				CachePath:    chapter.CachePath,
				ResourcePath: chapter.ResourcePath,
			})
		}
		return stage.stageArchiveMetadata(s.cfg.LibraryDir, archive, archivedChapters, engine.ArchivedBookSource{
			BookURL:            book.OriginalFile,
			Origin:             "loc_book",
			OriginName:         book.OriginalFile,
			Type:               0,
			Name:               book.Title,
			Author:             book.Author,
			LatestChapterTitle: book.LastChapter,
			TOCURL:             book.TOCFile,
			Time:               0,
			OriginOrder:        0,
		})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh local book"})
		return
	}
	if err := stage.promote(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish local refreshed content"})
		return
	}
	s.pruneSupersededLocalDerivedContent(book, archiveRoot, supersededCachePaths)

	c.JSON(http.StatusOK, gin.H{"book": s.broadcastBookShelfUpdate(userID, book), "chapterCount": len(parsed)})
}

type cacheBookRequest struct {
	ChapterIndex *int `json:"chapterIndex"`
	All          bool `json:"all"`
	Count        int  `json:"count"`
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
	if book.SourceID == 0 {
		c.JSON(http.StatusOK, gin.H{"cached": 0, "requested": 0, "message": "local books do not need server cache"})
		return
	}
	cached, requested, err := s.cacheBookChapters(book, request.ChapterIndex, request.All, request.Count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list chapters"})
		return
	}
	item := s.broadcastBookShelfUpdate(userID, book)
	c.JSON(http.StatusOK, gin.H{"cached": cached, "requested": requested, "book": item})
}

func (s *Server) cacheBookChapters(book models.Book, chapterIndex *int, all bool, count int) (int, int, error) {
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

func deleteBookRecords(tx *gorm.DB, userID, bookID uint, book *models.Book) error {
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Delete(&models.BookCategory{}).Error; err != nil {
		return err
	}
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
	CategoryID  *uint  `json:"categoryId"`
	CategoryIDs []uint `json:"categoryIds"`
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
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category payload"})
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category payload"})
		return
	}
	if err := json.Unmarshal(data, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category payload"})
		return
	}
	_, categoryIDsSet := raw["categoryIds"]
	if categoryIDsSet {
		if !s.validateCategoryIDs(c, userID, request.CategoryIDs) {
			return
		}
	} else if !s.validateCategory(c, userID, request.CategoryID) {
		return
	}

	nextIDs := categoryIDsFromRequest(request.CategoryID, request.CategoryIDs)
	if !categoryIDsSet && request.CategoryID != nil {
		nextIDs = []uint{*request.CategoryID}
	}
	if len(nextIDs) > 0 {
		book.CategoryID = &nextIDs[0]
	} else {
		book.CategoryID = nil
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.setBookCategories(tx, userID, book.ID, nextIDs); err != nil {
			return err
		}
		return tx.Save(&book).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update category"})
		return
	}
	c.JSON(http.StatusOK, s.broadcastBookShelfUpdate(userID, book))
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
	Title       string `json:"title" binding:"required"`
	Author      string `json:"author"`
	CoverURL    string `json:"coverUrl"`
	Intro       string `json:"intro"`
	Kind        string `json:"kind"`
	WordCount   string `json:"wordCount"`
	BookURL     string `json:"bookUrl" binding:"required"`
	SourceID    uint   `json:"sourceId" binding:"required"`
	SourceName  string `json:"sourceName"`
	Variable    string `json:"variable"`
	Type        int    `json:"type"`
	CategoryID  *uint  `json:"categoryId"`
	CategoryIDs []uint `json:"categoryIds"`
}

func firstNonBlankCanRename(remote string, current string, allowRename bool) string {
	current = strings.TrimSpace(current)
	remote = strings.TrimSpace(remote)
	if current == "" {
		return remote
	}
	if allowRename && remote != "" {
		return remote
	}
	return current
}

func (s *Server) createRemoteBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var req remoteBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, bookUrl, and sourceId are required"})
		return
	}
	if len(req.CategoryIDs) > 0 {
		if !s.validateCategoryIDs(c, userID, req.CategoryIDs) {
			return
		}
	} else if !s.validateCategory(c, userID, req.CategoryID) {
		return
	}
	categoryIDs := categoryIDsFromRequest(req.CategoryID, req.CategoryIDs)
	variable, err := engine.NormalizeSourceRuleVariables(req.Variable)
	if err != nil {
		writeSourceError(c, http.StatusBadRequest, "book source variables are invalid", err, "book_info")
		return
	}

	var source models.BookSource
	if err := s.db.First(&source, req.SourceID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}

	var existing models.Book
	if err := s.db.Where("user_id = ? AND url = ?", userID, strings.TrimSpace(req.BookURL)).First(&existing).Error; err == nil {
		if len(req.CategoryIDs) > 0 || req.CategoryID != nil {
			if len(categoryIDs) > 0 {
				existing.CategoryID = &categoryIDs[0]
			} else {
				existing.CategoryID = nil
			}
			_ = s.db.Transaction(func(tx *gorm.DB) error {
				if err := s.setBookCategories(tx, userID, existing.ID, categoryIDs); err != nil {
					return err
				}
				return tx.Save(&existing).Error
			})
		}
		c.JSON(http.StatusOK, s.broadcastBookShelfUpdate(userID, existing))
		return
	}

	remoteInfo, chapters, variable, err := engine.FetchBookInfoAndTOCWithVariables(req.BookURL, source, variable, req.Title)
	if err != nil {
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadRequest, "failed to fetch chapters", err, "book_info")
		return
	}
	if len(chapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	book := models.Book{
		UserID:       userID,
		SourceID:     req.SourceID,
		Type:         source.SourceType,
		Title:        firstNonBlankCanRename(remoteInfo.Title, req.Title, remoteInfo.CanRename),
		Author:       firstNonBlankCanRename(remoteInfo.Author, req.Author, remoteInfo.CanRename),
		CoverURL:     firstNonBlank(remoteInfo.CoverURL, req.CoverURL),
		Intro:        firstNonBlank(remoteInfo.Intro, req.Intro),
		Kind:         firstNonBlank(remoteInfo.Kind, req.Kind),
		WordCount:    firstNonBlank(remoteInfo.WordCount, req.WordCount),
		URL:          req.BookURL,
		Variable:     variable,
		LastChapter:  chapters[len(chapters)-1].Title,
		ChapterCount: len(chapters),
		CanUpdate:    true,
	}
	if len(categoryIDs) > 0 {
		book.CategoryID = &categoryIDs[0]
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&book).Error; err != nil {
			return err
		}
		if err := s.setBookCategories(tx, userID, book.ID, categoryIDs); err != nil {
			return err
		}
		for _, ch := range chapters {
			chapter := models.Chapter{
				BookID:   book.ID,
				Index:    ch.Index,
				Title:    ch.Title,
				URL:      ch.URL,
				IsVolume: ch.IsVolume,
				Tag:      ch.Tag,
				Variable: ch.Variable,
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

	c.JSON(http.StatusCreated, s.broadcastBookShelfUpdate(userID, book))
}

type changeSourceRequest struct {
	SourceID  uint   `json:"sourceId" binding:"required"`
	BookURL   string `json:"bookUrl"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	CoverURL  string `json:"coverUrl"`
	Intro     string `json:"intro"`
	Kind      string `json:"kind"`
	WordCount string `json:"wordCount"`
}

type contentMatch struct {
	ChapterID                uint    `json:"chapterId"`
	ChapterIndex             int     `json:"chapterIndex"`
	ChapterTitle             string  `json:"chapterTitle"`
	Excerpt                  string  `json:"excerpt"`
	Query                    string  `json:"query"`
	ResultCountWithinChapter int     `json:"resultCountWithinChapter"`
	Offset                   int     `json:"offset"`
	LineIndex                int     `json:"lineIndex"`
	Percent                  float64 `json:"percent"`
}

const contentSearchMaxMatchesPerChapter = 2000

type contentSearchScan struct {
	Matches             []contentMatch
	LastIndex           int
	UnavailableChapters int
	Truncated           bool
	Canceled            bool
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

	group := strings.TrimSpace(c.Query("group"))
	keyword := strings.TrimSpace(c.Query("q"))
	if keyword == "" {
		keyword = book.Title
	}
	limit := parseBoundedInt(c.Query("limit"), 10, 1, 80)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 10000)
	paged := c.Query("paged") == "1" || c.Query("paged") == "true"

	var sources []models.BookSource
	query := s.db.Where("enabled = ?", true)
	if group != "" {
		query = query.Where("COALESCE(\"group\", '') = ?", group)
	}
	var totalSources int64
	if paged {
		if err := query.Model(&models.BookSource{}).Count(&totalSources).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count sources"})
			return
		}
	}
	if err := query.Order("custom_order asc, id asc").Offset(offset).Limit(limit).Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sources"})
		return
	}
	sources = s.filterActiveSourceFailures(userID, sources)

	type sourceCandidate struct {
		SourceID           uint   `json:"sourceId"`
		SourceName         string `json:"sourceName"`
		Group              string `json:"group"`
		Title              string `json:"title"`
		Author             string `json:"author"`
		CoverURL           string `json:"coverUrl"`
		Intro              string `json:"intro"`
		Kind               string `json:"kind"`
		WordCount          string `json:"wordCount"`
		LatestChapterTitle string `json:"latestChapterTitle"`
		BookURL            string `json:"bookUrl"`
		Time               int64  `json:"time,omitempty"`
		Current            bool   `json:"current"`
		Type               int    `json:"type"`
	}
	type sourceCandidateBatch struct {
		Index      int
		Candidates []sourceCandidate
		Failure    error
		Empty      bool
	}

	results := make([]sourceCandidate, 0)
	if offset == 0 && book.SourceID > 0 {
		var currentSource models.BookSource
		if err := s.db.First(&currentSource, book.SourceID).Error; err == nil && (group == "" || currentSource.Group == group) {
			results = append(results, sourceCandidate{
				SourceID:           currentSource.ID,
				SourceName:         currentSource.Name,
				Group:              currentSource.Group,
				Title:              book.Title,
				Author:             book.Author,
				CoverURL:           book.CoverURL,
				Intro:              book.Intro,
				Kind:               book.Kind,
				WordCount:          book.WordCount,
				LatestChapterTitle: book.LastChapter,
				BookURL:            book.URL,
				Current:            true,
				Type:               currentSource.SourceType,
			})
		}
	}
	channel := make(chan sourceCandidateBatch, len(sources))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	parentCtx := c.Request.Context()
	for index, source := range sources {
		source := source
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(parentCtx, 12*time.Second)
			started := time.Now()
			searchResults, err := engine.SearchBooksContext(ctx, source, keyword)
			cancel()
			elapsed := time.Since(started).Milliseconds()
			if err != nil {
				channel <- sourceCandidateBatch{Index: index, Failure: err}
				return
			}
			candidates := make([]sourceCandidate, 0)
			for _, item := range searchResults {
				if item.BookURL == "" {
					continue
				}
				candidates = append(candidates, sourceCandidate{
					SourceID:           source.ID,
					SourceName:         source.Name,
					Group:              source.Group,
					Title:              item.Title,
					Author:             item.Author,
					CoverURL:           item.CoverURL,
					Intro:              item.Intro,
					Kind:               item.Kind,
					WordCount:          item.WordCount,
					LatestChapterTitle: item.LatestChapter,
					BookURL:            item.BookURL,
					Time:               elapsed,
					Current:            source.ID == book.SourceID && item.BookURL == book.URL,
					Type:               source.SourceType,
				})
				if len(candidates) >= 3 {
					break
				}
			}
			channel <- sourceCandidateBatch{
				Index:      index,
				Candidates: candidates,
				Empty:      len(candidates) == 0,
			}
		}(index)
	}
	go func() {
		wg.Wait()
		close(channel)
	}()
	failedSources := 0
	emptySources := 0
	matchedSources := 0
	batches := make([]sourceCandidateBatch, len(sources))
	for batch := range channel {
		batches[batch.Index] = batch
	}
	for _, batch := range batches {
		if batch.Failure != nil {
			failedSources++
			if batch.Index >= 0 && batch.Index < len(sources) {
				s.recordSourceFailure(userID, sources[batch.Index], batch.Failure)
			}
			continue
		}
		if batch.Empty {
			emptySources++
			continue
		}
		matchedSources++
		results = append(results, batch.Candidates...)
		if len(results) >= 120 {
			break
		}
	}

	if paged {
		c.JSON(http.StatusOK, gin.H{
			"list":       results,
			"offset":     offset,
			"nextOffset": offset + len(sources),
			"hasMore":    int64(offset+len(sources)) < totalSources,
			"total":      totalSources,
			"searched":   len(sources),
			"matched":    matchedSources,
			"failed":     failedSources,
			"empty":      emptySources,
		})
		return
	}

	c.JSON(http.StatusOK, results)
}

func parseBoundedInt(value string, fallback int, minValue int, maxValue int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
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
	remoteInfo, newChapters, variable, err := engine.FetchBookInfoAndTOCWithVariables(newBookURL, newSource, "", book.Title)
	if err != nil {
		s.recordSourceFailure(userID, newSource, err)
		writeSourceError(c, http.StatusBadRequest, "failed to fetch chapters from new source", err, "book_info")
		return
	}
	if len(newChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	var supersededCachePaths []string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		nextChapters := make([]models.Chapter, 0, len(newChapters))
		for _, ch := range newChapters {
			nextChapters = append(nextChapters, models.Chapter{
				BookID:   bookID,
				Index:    ch.Index,
				Title:    ch.Title,
				URL:      ch.URL,
				IsVolume: ch.IsVolume,
				Tag:      ch.Tag,
				Variable: ch.Variable,
			})
		}
		var err error
		supersededCachePaths, _, err = s.replaceBookChapterRows(tx, userID, bookID, nextChapters)
		if err != nil {
			return err
		}
		book.SourceID = req.SourceID
		book.Type = newSource.SourceType
		book.URL = newBookURL
		book.Variable = variable
		if title := firstNonBlankCanRename(remoteInfo.Title, firstNonBlank(req.Title, book.Title), remoteInfo.CanRename); title != "" {
			book.Title = title
		}
		if author := firstNonBlankCanRename(remoteInfo.Author, firstNonBlank(req.Author, book.Author), remoteInfo.CanRename); author != "" {
			book.Author = author
		}
		if coverURL := firstNonBlank(remoteInfo.CoverURL, req.CoverURL); coverURL != "" {
			book.CoverURL = coverURL
		}
		if intro := firstNonBlank(remoteInfo.Intro, req.Intro); intro != "" {
			book.Intro = intro
		}
		if kind := firstNonBlank(remoteInfo.Kind, req.Kind); kind != "" {
			book.Kind = kind
		}
		if wordCount := firstNonBlank(remoteInfo.WordCount, req.WordCount); wordCount != "" {
			book.WordCount = wordCount
		}
		book.LastChapter = newChapters[len(newChapters)-1].Title
		book.ChapterCount = len(newChapters)
		return tx.Save(&book).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change source"})
		return
	}
	s.pruneUnreferencedRemoteCachePaths(supersededCachePaths)

	c.JSON(http.StatusOK, s.broadcastBookShelfUpdate(userID, book))
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

	content, contentErr := s.loadChapterTextContextResult(c.Request.Context(), book, &chapter)
	if contentErr != nil {
		if book.SourceID > 0 {
			var source models.BookSource
			if err := s.db.First(&source, book.SourceID).Error; err == nil {
				s.recordSourceFailure(userID, source, contentErr)
			}
		}
		if errors.Is(contentErr, context.Canceled) {
			return
		}
		writeSourceError(c, http.StatusBadGateway, "failed to load chapter content", contentErr, "content")
		return
	}
	response := gin.H{
		"chapter": chapter,
		"content": content,
		"format":  "text",
	}
	if book.Type == 1 {
		prepared, err := audioreader.PrepareDirectOrLocal(s.audioReader, book, &chapter, content)
		if err != nil {
			writeAudioChapterPrepareError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"chapter":           chapter,
			"content":           prepared.ResourceURL,
			"format":            "audio",
			"resourceUrl":       prepared.ResourceURL,
			"resourceExpiresAt": prepared.ExpiresAt.UTC().Format(time.RFC3339),
		})
		return
	}
	if cbzreader.IsLocalCBZ(book) {
		prepared, err := s.cbzReader.PrepareChapter(book, &chapter)
		if err != nil {
			writeCBZServiceError(c, err, "failed to prepare CBZ chapter")
			return
		}
		response["chapter"] = chapter
		response["content"] = `<img src="` + html.EscapeString(prepared.ResourceURL) + `" />`
		response["format"] = "cbz"
		response["resourceUrl"] = prepared.ResourceURL
		response["resourceExpiresAt"] = prepared.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if epubreader.IsLocalEPUB(book) {
		prepared, err := s.epubReader.PrepareChapter(book, &chapter)
		if err != nil {
			writeEPUBServiceError(c, err, "failed to prepare EPUB chapter")
			return
		}
		response["chapter"] = chapter
		response["format"] = "epub"
		response["resourceUrl"] = prepared.ResourceURL
		response["resourceExpiresAt"] = prepared.ExpiresAt.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, response)
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

	keyword := strings.TrimSpace(firstNonBlank(c.Query("q"), c.Query("keyword")))
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}

	var chapters []models.Chapter
	if err := s.db.Where("book_id = ?", bookID).Order("`index` asc").Find(&chapters).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list chapters"})
		return
	}

	if c.Query("paged") == "1" || c.Query("paged") == "true" {
		start := 0
		if strings.TrimSpace(c.Query("offset")) != "" {
			start = parseBoundedInt(c.Query("offset"), 0, 0, len(chapters))
		} else {
			start = parseBoundedInt(c.Query("lastIndex"), -1, -1, len(chapters)) + 1
		}
		matchLimitQuery := firstNonBlank(c.Query("matchLimit"), c.Query("size"))
		chapterLimit := parseBoundedInt(c.Query("chapterLimit"), 30, 1, 500)
		matchLimit := parseBoundedInt(matchLimitQuery, 80, 1, 200)
		perChapterLimit := parseBoundedInt(c.Query("perChapterLimit"), 20, 1, 100)
		if book.SourceID == 0 && (c.Query("localFull") == "1" || c.Query("localFull") == "true") {
			chapterLimit = parseBoundedInt(c.Query("chapterLimit"), 160, 1, 2000)
			matchLimit = parseBoundedInt(matchLimitQuery, 5000, 1, 20000)
			perChapterLimit = parseBoundedInt(c.Query("perChapterLimit"), 500, 1, 2000)
		}
		scan := s.collectContentMatchesContext(c.Request.Context(), book, chapters, keyword, start, chapterLimit, matchLimit, perChapterLimit)
		if scan.Canceled {
			return
		}
		matches := scan.Matches
		lastIndex := scan.LastIndex
		unavailableChapters := scan.UnavailableChapters
		truncated := scan.Truncated
		if (c.Query("scanUntilMatch") == "1" || c.Query("scanUntilMatch") == "true") && len(matches) == 0 && lastIndex >= 0 && lastIndex < len(chapters)-1 {
			scanLimit := parseBoundedInt(c.Query("scanLimit"), chapterLimit, chapterLimit, 2000)
			if book.SourceID > 0 {
				scanLimit = parseBoundedInt(c.Query("scanLimit"), chapterLimit, chapterLimit, 500)
			}
			scanned := lastIndex - start + 1
			for scanned < scanLimit && lastIndex >= 0 && lastIndex < len(chapters)-1 && len(matches) < matchLimit {
				nextStart := lastIndex + 1
				nextLimit := min(chapterLimit, scanLimit-scanned)
				nextScan := s.collectContentMatchesContext(c.Request.Context(), book, chapters, keyword, nextStart, nextLimit, matchLimit-len(matches), perChapterLimit)
				if nextScan.Canceled {
					return
				}
				if nextScan.LastIndex < 0 {
					break
				}
				scanned += nextScan.LastIndex - nextStart + 1
				lastIndex = nextScan.LastIndex
				matches = append(matches, nextScan.Matches...)
				unavailableChapters += nextScan.UnavailableChapters
				truncated = truncated || nextScan.Truncated
				if len(nextScan.Matches) > 0 {
					break
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"list":                matches,
			"lastIndex":           lastIndex,
			"hasMore":             lastIndex >= 0 && lastIndex < len(chapters)-1,
			"total":               len(chapters),
			"incomplete":          unavailableChapters > 0 || truncated,
			"unavailableChapters": unavailableChapters,
			"truncated":           truncated,
		})
		return
	}

	matches, _ := s.collectContentMatches(book, chapters, keyword, 0, len(chapters), 200, 20)
	c.JSON(http.StatusOK, matches)
}

type legacySearchBookContentRequest struct {
	URL       string `json:"url"`
	BookURL   string `json:"bookUrl"`
	Keyword   string `json:"keyword"`
	LastIndex *int   `json:"lastIndex"`
	Size      *int   `json:"size"`
}

type legacyContentMatch struct {
	ChapterID                uint    `json:"chapterId"`
	ChapterIndex             int     `json:"chapterIndex"`
	ChapterTitle             string  `json:"chapterTitle"`
	ResultText               string  `json:"resultText"`
	Query                    string  `json:"query"`
	ResultCountWithinChapter int     `json:"resultCountWithinChapter"`
	Offset                   int     `json:"offset"`
	LineIndex                int     `json:"lineIndex"`
	Percent                  float64 `json:"percent"`
}

func (s *Server) legacySearchBookContent(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	req := legacySearchBookContentRequest{
		URL:     c.Query("url"),
		BookURL: c.Query("bookUrl"),
		Keyword: c.Query("keyword"),
	}
	if lastIndexValue := strings.TrimSpace(c.Query("lastIndex")); lastIndexValue != "" {
		lastIndex := parseBoundedInt(lastIndexValue, 0, -1, 1000000)
		req.LastIndex = &lastIndex
	}
	if sizeValue := strings.TrimSpace(c.Query("size")); sizeValue != "" {
		size := parseBoundedInt(sizeValue, 20, 1, 20000)
		req.Size = &size
	}
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "请求格式不正确"})
			return
		}
	}

	bookURL := strings.TrimSpace(firstNonBlank(req.URL, req.BookURL))
	if bookURL == "" {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "请输入书籍链接"})
		return
	}
	keyword := strings.TrimSpace(req.Keyword)
	if keyword == "" {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "请输入搜索关键词"})
		return
	}

	var book models.Book
	err := s.db.Where("user_id = ? AND url = ?", userID, bookURL).First(&book).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "请先加入书架"})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "加载书籍失败"})
		return
	}

	var chapters []models.Chapter
	if err := s.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "加载目录失败"})
		return
	}

	lastIndex := 0
	if req.LastIndex != nil {
		lastIndex = *req.LastIndex
	}
	if lastIndex >= len(chapters) {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "没有更多了"})
		return
	}
	size := 20
	if req.Size != nil {
		size = *req.Size
	}
	start := lastIndex + 1
	if start >= len(chapters) {
		c.JSON(http.StatusOK, gin.H{
			"isSuccess": true,
			"data": gin.H{
				"list":      []legacyContentMatch{},
				"lastIndex": start,
				"hasMore":   false,
				"total":     len(chapters),
			},
		})
		return
	}
	scan := s.collectContentMatchesContext(context.Background(), book, chapters, keyword, start, len(chapters)-start, max(size, 1), max(size, 1))
	matches, currentIndex := scan.Matches, scan.LastIndex
	c.JSON(http.StatusOK, gin.H{
		"isSuccess": true,
		"data": gin.H{
			"list":                legacyContentMatches(matches),
			"lastIndex":           currentIndex,
			"hasMore":             currentIndex >= 0 && currentIndex < len(chapters)-1,
			"total":               len(chapters),
			"incomplete":          scan.UnavailableChapters > 0 || scan.Truncated,
			"unavailableChapters": scan.UnavailableChapters,
			"truncated":           scan.Truncated,
		},
	})
}

func legacyContentMatches(matches []contentMatch) []legacyContentMatch {
	result := make([]legacyContentMatch, 0, len(matches))
	for _, match := range matches {
		result = append(result, legacyContentMatch{
			ChapterID:                match.ChapterID,
			ChapterIndex:             match.ChapterIndex,
			ChapterTitle:             match.ChapterTitle,
			ResultText:               match.Excerpt,
			Query:                    match.Query,
			ResultCountWithinChapter: match.ResultCountWithinChapter,
			Offset:                   match.Offset,
			LineIndex:                match.LineIndex,
			Percent:                  match.Percent,
		})
	}
	return result
}

func (s *Server) collectContentMatches(book models.Book, chapters []models.Chapter, keyword string, start int, chapterLimit int, matchLimit int, perChapterLimit int) ([]contentMatch, int) {
	scan := s.collectContentMatchesContext(context.Background(), book, chapters, keyword, start, chapterLimit, matchLimit, perChapterLimit)
	return scan.Matches, scan.LastIndex
}

func (s *Server) collectContentMatchesContext(ctx context.Context, book models.Book, chapters []models.Chapter, keyword string, start int, chapterLimit int, matchLimit int, perChapterLimit int) contentSearchScan {
	scan := contentSearchScan{Matches: make([]contentMatch, 0), LastIndex: -1}
	if start < 0 {
		start = 0
	}
	if start >= len(chapters) || chapterLimit <= 0 || matchLimit <= 0 || perChapterLimit <= 0 {
		return scan
	}
	// `perChapterLimit` used to silently discard a dense chapter and then move
	// its cursor forward. Reader-dev completes the final scanned chapter first;
	// retain the input for deployed clients but use the explicit safe cap below.
	_ = perChapterLimit
	end := start + chapterLimit
	if end > len(chapters) {
		end = len(chapters)
	}
	for i := start; i < end; i++ {
		if err := ctx.Err(); err != nil {
			scan.Canceled = true
			return scan
		}
		scan.LastIndex = i
		content, err := s.loadChapterTextContextResult(ctx, book, &chapters[i])
		if err != nil {
			if ctx.Err() != nil {
				scan.Canceled = true
				return scan
			}
			scan.UnavailableChapters++
			continue
		}
		positions, chapterTruncated := searchContentPositionsBounded(content, keyword, contentSearchMaxMatchesPerChapter)
		scan.Truncated = scan.Truncated || chapterTruncated
		for matchIndex, position := range positions {
			scan.Matches = append(scan.Matches, contentMatch{
				ChapterID:                chapters[i].ID,
				ChapterIndex:             chapters[i].Index,
				ChapterTitle:             chapters[i].Title,
				Excerpt:                  excerptAround(content, position, keyword),
				Query:                    keyword,
				ResultCountWithinChapter: matchIndex,
				Offset:                   position,
				LineIndex:                lineIndexAtByte(content, position),
				Percent:                  float64(position) / float64(max(len(content), 1)),
			})
		}
		// Like reader-dev, the requested page size is a threshold checked after
		// a complete chapter, so a dense final chapter is never skipped by the
		// next cursor. The explicit safety cap above is surfaced as `truncated`.
		if len(scan.Matches) >= matchLimit {
			break
		}
	}

	return scan
}

func searchContentPositions(content string, keyword string, limit int) []int {
	positions, _ := searchContentPositionsBounded(content, keyword, limit)
	return positions
}

func searchContentPositionsBounded(content string, keyword string, limit int) ([]int, bool) {
	if content == "" || keyword == "" || limit <= 0 {
		return nil, false
	}
	capacity := limit + 1
	seen := make(map[int]struct{})
	lowerContent := strings.ToLower(content)
	needle := strings.ToLower(keyword)
	positions := make([]int, 0)
	for offset := 0; offset < len(lowerContent) && len(positions) < capacity; {
		position := strings.Index(lowerContent[offset:], needle)
		if position < 0 {
			break
		}
		absolute := offset + position
		if _, ok := seen[absolute]; !ok {
			seen[absolute] = struct{}{}
			positions = append(positions, absolute)
		}
		offset = absolute + len(needle)
	}

	normalizedContent, contentMap := normalizeSearchText(content)
	normalizedKeyword, _ := normalizeSearchText(keyword)
	if normalizedKeyword == "" {
		sort.Ints(positions)
		truncated := len(positions) > limit
		if truncated {
			positions = positions[:limit]
		}
		return positions, truncated
	}
	for offset := 0; offset < len(normalizedContent) && len(positions) < capacity; {
		position := strings.Index(normalizedContent[offset:], normalizedKeyword)
		if position < 0 {
			break
		}
		absolute := offset + position
		if absolute >= 0 && absolute < len(contentMap) {
			mappedPosition := contentMap[absolute]
			if _, ok := seen[mappedPosition]; !ok {
				seen[mappedPosition] = struct{}{}
				positions = append(positions, mappedPosition)
			}
		}
		offset = absolute + len(normalizedKeyword)
	}
	if len(positions) < capacity {
		termPosition := searchContentTermPosition(normalizedContent, contentMap, keyword)
		if termPosition >= 0 {
			if _, ok := seen[termPosition]; !ok {
				positions = append(positions, termPosition)
			}
		}
	}
	sort.Ints(positions)
	truncated := len(positions) > limit
	if truncated {
		positions = positions[:limit]
	}
	return positions, truncated
}

func searchContentTermPosition(normalizedContent string, contentMap []int, keyword string) int {
	terms := normalizeSearchTerms(keyword)
	if len(terms) < 2 || normalizedContent == "" {
		return -1
	}
	offset := 0
	firstNormalizedPosition := -1
	for _, term := range terms {
		if term == "" {
			continue
		}
		position := strings.Index(normalizedContent[offset:], term)
		if position < 0 {
			return -1
		}
		absolute := offset + position
		if firstNormalizedPosition < 0 {
			firstNormalizedPosition = absolute
		}
		offset = absolute + len(term)
	}
	if firstNormalizedPosition < 0 || firstNormalizedPosition >= len(contentMap) {
		return -1
	}
	return contentMap[firstNormalizedPosition]
}

func normalizeSearchTerms(value string) []string {
	terms := make([]string, 0)
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		terms = append(terms, builder.String())
		builder.Reset()
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			flush()
			continue
		}
		builder.WriteString(strings.ToLower(string(r)))
	}
	flush()
	return terms
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
	return s.loadChapterTextContext(context.Background(), book, chapter)
}

func (s *Server) loadChapterTextContext(ctx context.Context, book models.Book, chapter *models.Chapter) string {
	content, _ := s.loadChapterTextContextResult(ctx, book, chapter)
	return content
}

func (s *Server) loadChapterTextContextResult(ctx context.Context, book models.Book, chapter *models.Chapter) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	content := ""
	if chapter.CachePath != "" {
		if bytes, path, err := s.readChapterCache(book, chapter.CachePath); err == nil {
			content = string(bytes)
			if book.SourceID == 0 {
				if normalizedPath := s.localChapterCachePath(book, path); normalizedPath != "" && normalizedPath != chapter.CachePath {
					chapter.CachePath = normalizedPath
					_ = s.db.Save(chapter)
				}
			} else if path != "" && path != chapter.CachePath {
				if normalizedPath := s.remoteChapterCachePath(path); normalizedPath != "" {
					chapter.CachePath = normalizedPath
				} else {
					chapter.CachePath = path
				}
				_ = s.db.Save(chapter)
			}
		}
	}

	if content == "" && book.SourceID == 0 {
		content = s.rebuildLocalChapterText(book, chapter)
	}

	if content == "" && chapter.URL != "" && book.SourceID > 0 {
		var source models.BookSource
		if err := s.db.First(&source, book.SourceID).Error; err != nil {
			return "", err
		}
		nextChapterURL := ""
		if source.SourceType != 1 {
			var nextChapter models.Chapter
			nextErr := s.db.Select("url").Where("book_id = ? AND `index` = ?", book.ID, chapter.Index+1).First(&nextChapter).Error
			if nextErr == nil {
				nextChapterURL = nextChapter.URL
			} else if !errors.Is(nextErr, gorm.ErrRecordNotFound) {
				return "", nextErr
			}
		}
		fetched, variableState, fetchErr := engine.FetchChapterContentContextWithState(ctx, chapter.URL, nextChapterURL, source, engine.SourceRuleVariableState{
			BookVariable:    book.Variable,
			ChapterVariable: chapter.Variable,
			BookName:        book.Title,
			ChapterTitle:    chapter.Title,
		})
		if fetchErr != nil {
			return "", fetchErr
		}
		book.Variable = variableState.BookVariable
		chapter.Variable = variableState.ChapterVariable
		if fetched != "" {
			content = fetched
			cachePath, cacheErr := engine.WriteChapterCache(s.cfg.CacheDir, book.URL, chapter.URL, content)
			if cacheErr == nil {
				chapter.CachePath = cachePath
			}
		}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&models.Book{}).
				Where("id = ? AND user_id = ?", book.ID, book.UserID).
				Update("variable", book.Variable).Error; err != nil {
				return err
			}
			return tx.Model(&models.Chapter{}).
				Where("id = ? AND book_id = ?", chapter.ID, book.ID).
				Updates(map[string]any{"variable": chapter.Variable, "cache_path": chapter.CachePath}).Error
		}); err != nil {
			return "", err
		}
	}
	if !epubreader.IsLocalEPUB(book) && book.Type != 1 {
		content = s.applyUserReplaceRules(book, content)
	}
	return content, nil
}

func (s *Server) localChapterCachePath(book models.Book, fullPath string) string {
	fullPath = strings.TrimSpace(fullPath)
	if fullPath == "" {
		return ""
	}
	if !filepath.IsAbs(fullPath) {
		return fullPath
	}
	if strings.TrimSpace(book.LibraryPath) != "" {
		libraryRoot := filepath.Join(s.cfg.LibraryDir, book.LibraryPath)
		if rel, ok := relativePathInside(libraryRoot, fullPath); ok {
			return rel
		}
	}
	return s.remoteChapterCachePath(fullPath)
}

func (s *Server) remoteChapterCachePath(fullPath string) string {
	if rel, ok := relativePathInside(s.cfg.CacheDir, fullPath); ok {
		return rel
	}
	return ""
}

func relativePathInside(root string, path string) (string, bool) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	if cleanPath != cleanRoot && !strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator)) {
		return "", false
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

func (s *Server) rebuildLocalChapterText(book models.Book, chapter *models.Chapter) string {
	sourcePath, ok := s.localBookSourcePath(book)
	if !ok {
		return ""
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return ""
	}
	chapters, err := parseLocalBookChapters(filepath.Ext(sourcePath), data, book.TOCRule)
	if err != nil || chapter.Index < 0 || chapter.Index >= len(chapters) {
		return ""
	}
	content := strings.TrimSpace(chapters[chapter.Index].Content)
	if content == "" {
		return ""
	}

	chapterURL := strings.TrimSpace(chapter.URL)
	if chapterURL == "" {
		chapterURL = fmt.Sprintf("local://book_%d/chapter_%d", book.ID, chapter.Index)
		chapter.URL = chapterURL
	}
	bookURL := strings.TrimSpace(book.URL)
	if bookURL == "" {
		bookURL = fmt.Sprintf("local://book_%d", book.ID)
	}
	if strings.TrimSpace(book.LibraryPath) != "" {
		contentDir := filepath.Join(s.cfg.LibraryDir, book.LibraryPath, "content")
		if cachePath, err := engine.WriteChapterCache(contentDir, bookURL, chapterURL, content); err == nil {
			chapter.CachePath = filepath.Join("content", cachePath)
			_ = s.db.Save(chapter)
		}
	}
	return content
}

func parseLocalBookChapters(ext string, data []byte, tocRule string) ([]engine.TXTChapter, error) {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".txt", ".text", ".md":
		return engine.ParseTXTWithRule(data, tocRule)
	case ".epub":
		book, err := engine.ParseEPUBWithRule(data, tocRule)
		return book.Chapters, err
	case ".pdf":
		book, err := engine.ParsePDF(data)
		return book.Chapters, err
	case ".umd":
		book, err := engine.ParseUMD(data)
		return book.Chapters, err
	case ".cbz":
		book, err := engine.ParseCBZ(data)
		return book.Chapters, err
	default:
		return nil, fmt.Errorf("unsupported local book extension: %s", ext)
	}
}

func (s *Server) localBookSourcePath(book models.Book) (string, bool) {
	candidates := make([]string, 0, 4)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	libraryRoot := ""
	if strings.TrimSpace(book.LibraryPath) != "" {
		libraryRoot = filepath.Join(s.cfg.LibraryDir, book.LibraryPath)
	}
	originalFile := strings.TrimSpace(book.OriginalFile)
	if filepath.IsAbs(originalFile) {
		add(originalFile)
		if libraryRoot != "" {
			if suffix, ok := suffixAfterPathSegment(originalFile, book.LibraryPath); ok {
				add(filepath.Join(libraryRoot, suffix))
			}
			add(filepath.Join(libraryRoot, filepath.Base(originalFile)))
		}
	} else if originalFile != "" {
		add(filepath.Join(s.cfg.LibraryDir, originalFile))
		if libraryRoot != "" {
			add(filepath.Join(libraryRoot, filepath.Base(originalFile)))
		}
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && isSupportedLocalBookFile(path) {
			return path, true
		}
	}
	if libraryRoot == "" {
		return "", false
	}
	entries, err := os.ReadDir(libraryRoot)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(libraryRoot, entry.Name())
		if isSupportedLocalBookFile(path) {
			return path, true
		}
	}
	return "", false
}

func isSupportedLocalBookFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".text", ".md", ".epub", ".pdf", ".umd", ".cbz":
		return true
	default:
		return false
	}
}

func (s *Server) readChapterCache(book models.Book, cachePath string) ([]byte, string, error) {
	var lastErr error
	for _, path := range s.chapterCacheCandidates(book, cachePath) {
		bytes, err := os.ReadFile(path)
		if err == nil {
			return bytes, path, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, "", lastErr
}

func (s *Server) chapterCacheCandidates(book models.Book, cachePath string) []string {
	cachePath = strings.TrimSpace(cachePath)
	if cachePath == "" {
		return nil
	}

	candidates := make([]string, 0, 5)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	if filepath.IsAbs(cachePath) {
		add(cachePath)
	} else {
		add(filepath.Join(s.cfg.CacheDir, cachePath))
	}

	if book.SourceID == 0 && strings.TrimSpace(book.LibraryPath) != "" {
		libraryRoot := filepath.Join(s.cfg.LibraryDir, book.LibraryPath)
		contentRoot := filepath.Join(libraryRoot, "content")
		if filepath.IsAbs(cachePath) {
			if suffix, ok := suffixAfterPathSegment(cachePath, "content"); ok {
				add(filepath.Join(contentRoot, suffix))
			}
			if suffix, ok := suffixAfterPathSegment(cachePath, book.LibraryPath); ok {
				add(filepath.Join(libraryRoot, suffix))
			}
		} else {
			add(filepath.Join(libraryRoot, cachePath))
			add(filepath.Join(contentRoot, cachePath))
		}
	}

	return candidates
}

func suffixAfterPathSegment(path string, segment string) (string, bool) {
	segment = strings.Trim(segment, `/\`)
	if segment == "" {
		return "", false
	}
	segmentParts := splitPathSegments(segment)
	pathParts := splitPathSegments(filepath.Clean(path))
	if len(segmentParts) == 0 || len(pathParts) <= len(segmentParts) {
		return "", false
	}
	for i := 0; i+len(segmentParts) < len(pathParts); i++ {
		match := true
		for j := range segmentParts {
			if pathParts[i+j] != segmentParts[j] {
				match = false
				break
			}
		}
		if match {
			return filepath.Join(pathParts[i+len(segmentParts):]...), true
		}
	}
	return "", false
}

func splitPathSegments(path string) []string {
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	filtered := parts[:0]
	for _, part := range parts {
		if part != "" && part != "." {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func (s *Server) applyUserReplaceRules(book models.Book, content string) string {
	if content == "" {
		return content
	}
	var rules []models.ReplaceRule
	if err := s.db.Where("user_id = ? AND enabled = ?", book.UserID, true).Order("id asc").Find(&rules).Error; err != nil {
		return content
	}
	for _, rule := range rules {
		if !replaceRuleAppliesToBook(rule.Scope, book) {
			continue
		}
		isRegex := false
		if rule.IsRegex != nil {
			isRegex = *rule.IsRegex
		}
		if err := validateReaderReplaceRulePattern(rule.Pattern, isRegex); err != nil {
			// reader-dev aborts the remaining pipeline when a malformed regex is
			// encountered; it never treats the malformed pattern as plain text.
			break
		}
		content = applyReaderReplaceRule(content, rule.Pattern, rule.Replacement, isRegex)
	}
	return content
}

func replaceRuleAppliesToBook(scope string, book models.Book) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "*" {
		return true
	}
	parts := strings.Split(scope, ";")
	name := strings.TrimSpace(parts[0])
	if name != "*" && name != strings.TrimSpace(book.Title) {
		return false
	}
	if len(parts) < 2 {
		return true
	}
	url := strings.TrimSpace(parts[1])
	return url == "" || url == strings.TrimSpace(book.URL)
}

func (s *Server) checkUpdates(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	count, updatedBookIDs := s.scheduler.CheckNowForUser(userID)
	items := make([]bookListItem, 0, len(updatedBookIDs))
	if len(updatedBookIDs) > 0 {
		var books []models.Book
		if err := s.db.Where("user_id = ? AND id IN ?", userID, updatedBookIDs).Find(&books).Error; err == nil {
			for _, book := range books {
				items = append(items, s.bookShelfListItem(userID, book))
			}
		}
	}
	if len(items) > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
	} else if count > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
	}
	c.JSON(http.StatusOK, gin.H{"newChapters": count, "books": items})
}
