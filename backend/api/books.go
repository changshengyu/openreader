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
	err := s.db.Transaction(func(tx *gorm.DB) error {
		switch request.Action {
		case "delete":
			var books []models.Book
			if err := tx.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Find(&books).Error; err != nil {
				return err
			}
			for i := range books {
				deletedIDs = append(deletedIDs, books[i].ID)
				if err := deleteBookRecords(tx, userID, books[i].ID, &books[i]); err != nil {
					return err
				}
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
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" || format == "json" {
		s.exportBooksJSON(c, userID, books)
		return
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
	remoteInfo, remoteChapters, err := engine.FetchBookInfoAndTOC(book.URL, source)
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
				chapter.IsVolume = remoteChapter.IsVolume
				chapter.Tag = remoteChapter.Tag
				if err := tx.Save(&chapter).Error; err != nil {
					return err
				}
				continue
			}
			chapter := models.Chapter{
				BookID:   book.ID,
				Index:    remoteChapter.Index,
				Title:    remoteChapter.Title,
				URL:      remoteChapter.URL,
				IsVolume: remoteChapter.IsVolume,
				Tag:      remoteChapter.Tag,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			added++
		}
		book.Title = firstNonBlank(remoteInfo.Title, book.Title)
		book.Author = firstNonBlank(remoteInfo.Author, book.Author)
		book.CoverURL = firstNonBlank(remoteInfo.CoverURL, book.CoverURL)
		book.Intro = firstNonBlank(remoteInfo.Intro, book.Intro)
		book.Kind = firstNonBlank(remoteInfo.Kind, book.Kind)
		book.WordCount = firstNonBlank(remoteInfo.WordCount, book.WordCount)
		book.LastChapter = remoteChapters[len(remoteChapters)-1].Title
		book.ChapterCount = len(remoteChapters)
		return tx.Save(&book).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh book"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"book": s.broadcastBookShelfUpdate(userID, book), "added": added, "chapterCount": len(remoteChapters)})
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

	newChapterIDs := make(map[int]uint, len(parsed))
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("book_id = ?", book.ID).Delete(&models.Chapter{}).Error; err != nil {
			return err
		}
		archive := engine.ArchivedBook{
			Directory:    book.LibraryPath,
			OriginalFile: book.OriginalFile,
			TOCFile:      book.TOCFile,
			SourceFile:   book.SourceFile,
		}
		archivedChapters := make([]engine.ArchivedChapter, 0, len(parsed))
		contentDir := s.cfg.CacheDir
		useLibraryContent := strings.TrimSpace(book.LibraryPath) != ""
		if useLibraryContent {
			contentDir = filepath.Join(s.cfg.LibraryDir, book.LibraryPath, "content")
		}
		bookURL := strings.TrimSpace(book.URL)
		if bookURL == "" {
			bookURL = fmt.Sprintf("local://book_%d", book.ID)
			book.URL = bookURL
		}
		for index, parsedChapter := range parsed {
			title := strings.TrimSpace(parsedChapter.Title)
			if title == "" {
				title = fmt.Sprintf("第 %d 章", index+1)
			}
			chapterURL := fmt.Sprintf("%s/chapter_%d", bookURL, index)
			cachePath, err := engine.WriteChapterCache(contentDir, bookURL, chapterURL, parsedChapter.Content)
			if err != nil {
				return err
			}
			chapterCachePath := cachePath
			if useLibraryContent {
				chapterCachePath = filepath.Join("content", cachePath)
			}
			chapter := models.Chapter{
				BookID:    book.ID,
				Index:     index,
				Title:     title,
				URL:       chapterURL,
				CachePath: chapterCachePath,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			newChapterIDs[index] = chapter.ID
			archivedChapters = append(archivedChapters, engine.ArchivedChapter{
				ID:        chapter.ID,
				URL:       chapterURL,
				Title:     title,
				IsVolume:  false,
				BaseURL:   "",
				BookURL:   book.OriginalFile,
				Index:     index,
				Start:     parsedChapter.Start,
				End:       parsedChapter.End,
				CachePath: chapter.CachePath,
			})
		}
		book.LastChapter = strings.TrimSpace(parsed[len(parsed)-1].Title)
		if book.LastChapter == "" {
			book.LastChapter = fmt.Sprintf("第 %d 章", len(parsed))
		}
		book.ChapterCount = len(parsed)
		book.TOCRule = tocRule
		if err := tx.Save(&book).Error; err != nil {
			return err
		}
		if archive.TOCFile != "" {
			if err := engine.WriteChapterArchive(s.cfg.LibraryDir, archive, archivedChapters); err != nil {
				return err
			}
		}
		if archive.SourceFile != "" {
			source := engine.ArchivedBookSource{
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
			}
			if err := engine.WriteBookSource(s.cfg.LibraryDir, archive, source); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh local book"})
		return
	}

	for index, chapterID := range newChapterIDs {
		_ = s.db.Model(&models.ReadingProgress{}).
			Where("user_id = ? AND book_id = ? AND chapter_index = ?", userID, book.ID, index).
			Update("chapter_id", chapterID).Error
		_ = s.db.Model(&models.Bookmark{}).
			Where("user_id = ? AND book_id = ? AND chapter_index = ?", userID, book.ID, index).
			Update("chapter_id", chapterID).Error
	}
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
	c.JSON(http.StatusOK, gin.H{"cached": cached, "requested": requested, "book": s.bookShelfListItem(userID, book)})
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

func (s *Server) clearBookCache(bookID uint) (int, error) {
	var book models.Book
	if err := s.db.Select("id", "source_id").First(&book, bookID).Error; err != nil {
		return 0, err
	}
	if book.SourceID == 0 {
		return 0, nil
	}

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
	Type        int    `json:"type"`
	CategoryID  *uint  `json:"categoryId"`
	CategoryIDs []uint `json:"categoryIds"`
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

	remoteInfo, chapters, err := engine.FetchBookInfoAndTOC(req.BookURL, source)
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
		Type:         source.SourceType,
		Title:        firstNonBlank(remoteInfo.Title, req.Title),
		Author:       firstNonBlank(remoteInfo.Author, req.Author),
		CoverURL:     firstNonBlank(remoteInfo.CoverURL, req.CoverURL),
		Intro:        firstNonBlank(remoteInfo.Intro, req.Intro),
		Kind:         firstNonBlank(remoteInfo.Kind, req.Kind),
		WordCount:    firstNonBlank(remoteInfo.WordCount, req.WordCount),
		URL:          req.BookURL,
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
		Failed     bool
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
				channel <- sourceCandidateBatch{Index: index, Failed: true}
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
		if batch.Failed {
			failedSources++
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
	remoteInfo, newChapters, err := engine.FetchBookInfoAndTOC(newBookURL, newSource)
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
				BookID:   bookID,
				Index:    ch.Index,
				Title:    ch.Title,
				URL:      ch.URL,
				IsVolume: ch.IsVolume,
				Tag:      ch.Tag,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
		}
		book.SourceID = req.SourceID
		book.Type = newSource.SourceType
		book.URL = newBookURL
		if title := firstNonBlank(remoteInfo.Title, req.Title); title != "" {
			book.Title = title
		}
		if author := firstNonBlank(remoteInfo.Author, req.Author); author != "" {
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

	if c.Query("paged") == "1" || c.Query("paged") == "true" {
		start := 0
		if strings.TrimSpace(c.Query("offset")) != "" {
			start = parseBoundedInt(c.Query("offset"), 0, 0, len(chapters))
		} else {
			start = parseBoundedInt(c.Query("lastIndex"), -1, -1, len(chapters)) + 1
		}
		chapterLimit := parseBoundedInt(c.Query("chapterLimit"), 30, 1, 500)
		matchLimit := parseBoundedInt(c.Query("matchLimit"), 80, 1, 200)
		perChapterLimit := parseBoundedInt(c.Query("perChapterLimit"), 20, 1, 100)
		if book.SourceID == 0 && (c.Query("localFull") == "1" || c.Query("localFull") == "true") {
			chapterLimit = parseBoundedInt(c.Query("chapterLimit"), 160, 1, 2000)
			matchLimit = parseBoundedInt(c.Query("matchLimit"), 5000, 1, 20000)
			perChapterLimit = parseBoundedInt(c.Query("perChapterLimit"), 500, 1, 2000)
		}
		matches, lastIndex := s.collectContentMatches(book, chapters, keyword, start, chapterLimit, matchLimit, perChapterLimit)
		if (c.Query("scanUntilMatch") == "1" || c.Query("scanUntilMatch") == "true") && len(matches) == 0 && lastIndex >= 0 && lastIndex < len(chapters)-1 {
			scanLimit := parseBoundedInt(c.Query("scanLimit"), chapterLimit, chapterLimit, 2000)
			if book.SourceID > 0 {
				scanLimit = parseBoundedInt(c.Query("scanLimit"), chapterLimit, chapterLimit, 500)
			}
			scanned := lastIndex - start + 1
			for scanned < scanLimit && lastIndex >= 0 && lastIndex < len(chapters)-1 && len(matches) < matchLimit {
				nextStart := lastIndex + 1
				nextLimit := min(chapterLimit, scanLimit-scanned)
				nextMatches, nextLastIndex := s.collectContentMatches(book, chapters, keyword, nextStart, nextLimit, matchLimit-len(matches), perChapterLimit)
				if nextLastIndex < 0 {
					break
				}
				scanned += nextLastIndex - nextStart + 1
				lastIndex = nextLastIndex
				matches = append(matches, nextMatches...)
				if len(nextMatches) > 0 {
					break
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"list":      matches,
			"lastIndex": lastIndex,
			"hasMore":   lastIndex >= 0 && lastIndex < len(chapters)-1,
			"total":     len(chapters),
		})
		return
	}

	matches, _ := s.collectContentMatches(book, chapters, keyword, 0, len(chapters), 200, 20)
	c.JSON(http.StatusOK, matches)
}

func (s *Server) collectContentMatches(book models.Book, chapters []models.Chapter, keyword string, start int, chapterLimit int, matchLimit int, perChapterLimit int) ([]contentMatch, int) {
	matches := make([]contentMatch, 0)
	if start < 0 {
		start = 0
	}
	if start >= len(chapters) || chapterLimit <= 0 || matchLimit <= 0 || perChapterLimit <= 0 {
		return matches, -1
	}
	end := start + chapterLimit
	if end > len(chapters) {
		end = len(chapters)
	}
	lastIndex := start - 1
	for i := start; i < end; i++ {
		lastIndex = i
		content := s.loadChapterText(book, &chapters[i])
		if content == "" {
			continue
		}
		positions := searchContentPositions(content, keyword, perChapterLimit)
		for matchIndex, position := range positions {
			matches = append(matches, contentMatch{
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
			if len(matches) >= matchLimit {
				break
			}
		}
		if len(matches) >= matchLimit {
			break
		}
	}

	return matches, lastIndex
}

func searchContentPositions(content string, keyword string, limit int) []int {
	if content == "" || keyword == "" || limit <= 0 {
		return nil
	}
	seen := make(map[int]struct{})
	lowerContent := strings.ToLower(content)
	needle := strings.ToLower(keyword)
	positions := make([]int, 0)
	for offset := 0; offset < len(lowerContent) && len(positions) < limit; {
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
		if len(positions) > limit {
			return positions[:limit]
		}
		return positions
	}
	for offset := 0; offset < len(normalizedContent) && len(positions) < limit; {
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
	if len(positions) < limit {
		termPosition := searchContentTermPosition(normalizedContent, contentMap, keyword)
		if termPosition >= 0 {
			if _, ok := seen[termPosition]; !ok {
				positions = append(positions, termPosition)
			}
		}
	}
	sort.Ints(positions)
	return positions
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
	content = s.applyUserReplaceRules(book, content)
	return content
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
	case ".txt", ".text", ".md", ".epub", ".pdf", ".umd":
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
	if err := s.db.Where("user_id = ? AND enabled = ?", book.UserID, true).Find(&rules).Error; err != nil {
		return content
	}
	replacements := make([]models.TextReplaceRule, 0, len(rules))
	for _, rule := range rules {
		if !replaceRuleAppliesToBook(rule.Scope, book) {
			continue
		}
		isRegex := true
		if rule.IsRegex != nil {
			isRegex = *rule.IsRegex
		}
		replacements = append(replacements, models.TextReplaceRule{
			Pattern:     rule.Pattern,
			Replacement: rule.Replacement,
			IsRegex:     &isRegex,
		})
	}
	return engine.ApplyTextReplacements(content, replacements)
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
