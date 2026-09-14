package api

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	xhtml "golang.org/x/net/html"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/audioreader"
	"openreader/backend/services/booksources"
	"openreader/backend/services/cbzreader"
	"openreader/backend/services/chaptercache"
	"openreader/backend/services/chapterimage"
	"openreader/backend/services/contentsearch"
	"openreader/backend/services/epubreader"
)

type bookListItem struct {
	models.Book
	CategoryIDs        []uint                  `json:"categoryIds"`
	Progress           *models.ReadingProgress `json:"progress,omitempty"`
	ShelfOrderAt       time.Time               `json:"shelfOrderAt"`
	CachedChapterCount int64                   `json:"cachedChapterCount"`
	CoverResourceURL   *string                 `json:"coverResourceUrl,omitempty"`
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
		return s.projectBookShelfListItem(book, categoryIDs, models.ReadingProgress{}, cachedCount)
	}
	return s.projectBookShelfListItem(book, categoryIDs, progress, cachedCount)
}

func (s *Server) bookShelfListItemWithCategoryIDs(userID uint, book models.Book, categoryIDs []uint) bookListItem {
	var progress models.ReadingProgress
	err := s.db.Where("user_id = ? AND book_id = ?", userID, book.ID).First(&progress).Error
	cachedCount := s.cachedChapterCount(book.ID, book.SourceID)
	if err != nil {
		return s.projectBookShelfListItem(book, categoryIDs, models.ReadingProgress{}, cachedCount)
	}
	return s.projectBookShelfListItem(book, categoryIDs, progress, cachedCount)
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
		items = append(items, s.projectBookShelfListItem(book, categoryIDsByBookID[book.ID], progressByBookID[book.ID], cacheCountByBookID[book.ID]))
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

func (s *Server) projectBookShelfListItem(book models.Book, categoryIDs []uint, progress models.ReadingProgress, cachedChapterCount int64) bookListItem {
	if strings.TrimSpace(book.CoverURL) == "" && strings.TrimSpace(book.CustomCoverURL) == "" && cbzreader.IsLocalCBZ(book) {
		if prepared, err := s.cbzReader.PrepareCover(book); err == nil {
			// This is an ephemeral, user/book/archive-scoped response projection.
			// Do not save the capability into SQLite, archives, backups or events.
			book.CoverURL = prepared.ResourceURL
		}
	}
	item := bookShelfListItem(book, categoryIDs, progress, cachedChapterCount)
	if strings.TrimSpace(book.CustomCoverURL) == "" {
		item.CoverResourceURL = s.projectCoverResource(book.UserID, book.SourceID, book.CoverURL)
	}
	return item
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
	result, _ := loadBookCategoryIDsByBookID(s.db, userID, books)
	return result
}

func loadBookCategoryIDsByBookID(db *gorm.DB, userID uint, books []models.Book) (map[uint][]uint, error) {
	result := make(map[uint][]uint, len(books))
	if len(books) == 0 {
		return result, nil
	}
	bookIDs := make([]uint, 0, len(books))
	legacyByBookID := make(map[uint]*uint, len(books))
	for _, book := range books {
		bookIDs = append(bookIDs, book.ID)
		legacyByBookID[book.ID] = book.CategoryID
	}
	var rows []models.BookCategory
	err := db.Where("user_id = ? AND book_id IN ?", userID, bookIDs).Order("id asc").Find(&rows).Error
	for _, row := range rows {
		result[row.BookID] = append(result[row.BookID], row.CategoryID)
	}
	for _, book := range books {
		result[book.ID] = normalizeBookCategoryIDs(book, result[book.ID])
		if len(result[book.ID]) == 0 && legacyByBookID[book.ID] != nil && *legacyByBookID[book.ID] > 0 {
			result[book.ID] = []uint{*legacyByBookID[book.ID]}
		}
	}
	return result, err
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

func (s *Server) broadcastBookShelfUpdateWithCategoryIDs(userID uint, book models.Book, categoryIDs []uint) bookListItem {
	item := s.bookShelfListItemWithCategoryIDs(userID, book, categoryIDs)
	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": item})
	return item
}

func shelfOrderAt(book models.Book, progress *models.ReadingProgress) time.Time {
	orderAt := book.CreatedAt
	if orderAt.IsZero() {
		orderAt = book.UpdatedAt
	}
	if progress != nil && progress.UpdatedAt.After(orderAt) {
		orderAt = progress.UpdatedAt
	}
	return orderAt
}

func (s *Server) createBook(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	request, ok := decodeBookCreateRequest(c)
	if !ok {
		return
	}
	title, ok := normalizeBookWriteField(c, request.Title, maxBookTitleBytes, "book title is too long")
	if !ok {
		return
	}
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book title is required"})
		return
	}
	author, ok := normalizeBookWriteField(c, request.Author, maxBookAuthorBytes, "book author is too long")
	if !ok {
		return
	}
	coverURL, ok := normalizeBookWriteField(c, request.CoverURL, maxBookCoverURLBytes, "book cover url is too long")
	if !ok {
		return
	}
	customCoverURL, ok := normalizeBookWriteField(c, request.CustomCoverURL, maxBookCustomCoverURLBytes, "book custom cover url is too long")
	if !ok {
		return
	}
	intro := ""
	if request.Intro != nil {
		intro = strings.TrimSpace(*request.Intro)
	}
	kind, ok := normalizeBookWriteField(c, request.Kind, maxBookKindBytes, "book kind is too long")
	if !ok {
		return
	}
	wordCount, ok := normalizeBookWriteField(c, request.WordCount, maxBookWordCountBytes, "book word count is too long")
	if !ok {
		return
	}
	bookURL, ok := normalizeBookWriteField(c, request.URL, maxBookURLBytes, "book url is too long")
	if !ok {
		return
	}
	var unlockAssets func()
	if customCoverURL != "" {
		var err error
		unlockAssets, err = s.lockUserAssets(c.Request.Context(), userID)
		if err != nil {
			return
		}
		defer unlockAssets()
	}
	if err := s.validateBookCustomCoverURL(userID, "", customCoverURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid custom cover url"})
		return
	}
	categoryIDs := categoryIDsFromRequest(request.CategoryID, request.CategoryIDs)
	if !s.validateCategoryIDs(c, userID, categoryIDs) {
		return
	}

	book := models.Book{
		UserID:         userID,
		Title:          title,
		Author:         author,
		CoverURL:       coverURL,
		CustomCoverURL: customCoverURL,
		Intro:          intro,
		Kind:           kind,
		WordCount:      wordCount,
		URL:            bookURL,
		CanUpdate:      true,
	}
	if request.CanUpdate != nil {
		book.CanUpdate = *request.CanUpdate
	}
	if len(categoryIDs) > 0 {
		book.CategoryID = &categoryIDs[0]
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&book).Error; err != nil {
			return err
		}
		// GORM applies the model's true default to a false bool during Create.
		// Preserve an explicitly submitted false without changing the model schema.
		if request.CanUpdate != nil && !*request.CanUpdate {
			if err := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", book.ID, userID).
				UpdateColumn("can_update", false).Error; err != nil {
				return err
			}
			book.CanUpdate = false
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

// bookPatchWriteLifecycleTestHook pauses a validated write before its
// transaction so contract tests can deterministically exercise stale reads.
var bookPatchWriteLifecycleTestHook func(string)

var errBookPatchTargetNotFound = errors.New("book not found during patch")
var errBookInvalidCustomCover = errors.New("invalid custom cover url during patch")

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

	raw, request, ok := decodeBookUpdateRequest(c)
	if !ok {
		return
	}
	_, categoryIDSet := raw["categoryId"]
	_, categoryIDsSet := raw["categoryIds"]
	var nextCategoryIDs []uint
	if categoryIDsSet {
		nextCategoryIDs = uniquePositiveUintIDs(request.CategoryIDs)
		if !s.validateCategoryIDs(c, userID, nextCategoryIDs) {
			return
		}
	} else if categoryIDSet && !s.validateCategory(c, userID, request.CategoryID) {
		return
	}

	if request.Title != nil {
		title, ok := normalizeBookWriteField(c, request.Title, maxBookTitleBytes, "book title is too long")
		if !ok {
			return
		}
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "book title is required"})
			return
		}
		book.Title = title
	}
	if request.Author != nil {
		author, ok := normalizeBookWriteField(c, request.Author, maxBookAuthorBytes, "book author is too long")
		if !ok {
			return
		}
		book.Author = author
	}
	if request.CoverURL != nil {
		coverURL, ok := normalizeBookWriteField(c, request.CoverURL, maxBookCoverURLBytes, "book cover url is too long")
		if !ok {
			return
		}
		book.CoverURL = coverURL
	}
	if request.CustomCoverURL != nil {
		customCoverURL, ok := normalizeBookWriteField(c, request.CustomCoverURL, maxBookCustomCoverURLBytes, "book custom cover url is too long")
		if !ok {
			return
		}
		book.CustomCoverURL = customCoverURL
	}
	if request.Intro != nil {
		book.Intro = strings.TrimSpace(*request.Intro)
	}
	if categoryIDSet {
		book.CategoryID = request.CategoryID
	}
	if categoryIDsSet {
		if len(nextCategoryIDs) > 0 {
			book.CategoryID = &nextCategoryIDs[0]
		} else {
			book.CategoryID = nil
		}
	}
	if request.CanUpdate != nil {
		book.CanUpdate = *request.CanUpdate
	}
	updates := make(map[string]any)
	if request.Title != nil {
		updates["title"] = book.Title
	}
	if request.Author != nil {
		updates["author"] = book.Author
	}
	if request.CoverURL != nil {
		updates["cover_url"] = book.CoverURL
	}
	if request.CustomCoverURL != nil {
		updates["custom_cover_url"] = book.CustomCoverURL
	}
	if request.Intro != nil {
		updates["intro"] = book.Intro
	}
	if categoryIDSet || categoryIDsSet {
		if book.CategoryID == nil {
			updates["category_id"] = nil
		} else {
			updates["category_id"] = *book.CategoryID
		}
	}
	if request.CanUpdate != nil {
		updates["can_update"] = book.CanUpdate
	}
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		return
	}
	var unlockAssets func()
	if request.CustomCoverURL != nil {
		var err error
		unlockAssets, err = s.lockUserAssets(ctx, userID)
		if err != nil {
			return
		}
		defer unlockAssets()
	}
	if bookPatchWriteLifecycleTestHook != nil {
		bookPatchWriteLifecycleTestHook("metadata")
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", bookID, userID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBookPatchTargetNotFound
			}
			return err
		}
		if request.CustomCoverURL != nil {
			if err := s.validateBookCustomCoverURL(userID, current.CustomCoverURL, book.CustomCoverURL); err != nil {
				return errBookInvalidCustomCover
			}
		}
		if len(updates) > 0 {
			write := tx.Model(&models.Book{}).
				Where("id = ? AND user_id = ?", current.ID, current.UserID).
				Updates(updates)
			if write.Error != nil {
				return write.Error
			}
			if write.RowsAffected != 1 {
				return errBookPatchTargetNotFound
			}
		}
		if categoryIDsSet {
			if err := s.setBookCategories(tx, userID, current.ID, nextCategoryIDs); err != nil {
				return err
			}
		} else if categoryIDSet {
			if request.CategoryID == nil {
				if err := s.setBookCategories(tx, userID, current.ID, nil); err != nil {
					return err
				}
			} else if err := s.setBookCategories(tx, userID, current.ID, []uint{*request.CategoryID}); err != nil {
				return err
			}
		}
		return tx.Where("id = ? AND user_id = ?", current.ID, current.UserID).First(&book).Error
	}); err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		if errors.Is(err, errBookPatchTargetNotFound) {
			notFound(c, "book not found")
			return
		}
		if errors.Is(err, errBookInvalidCustomCover) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid custom cover url"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update book"})
		return
	}
	c.JSON(http.StatusOK, s.broadcastBookShelfUpdate(userID, book))
}

func (s *Server) validateBookCustomCoverURL(userID uint, currentURL string, nextURL string) error {
	if nextURL == "" || nextURL == currentURL {
		return nil
	}
	asset, err := s.userUploadAsset(nextURL)
	if err != nil || asset.UserID != userID || asset.Kind != "covers" {
		return os.ErrPermission
	}
	opened, err := s.openUserUploadAsset(asset)
	if err != nil {
		return err
	}
	_ = opened.File.Close()
	return nil
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

// batchBookCategoryWriteLifecycleTestHook exposes deterministic transaction
// barriers for package-level lifecycle contract tests.
var batchBookCategoryWriteLifecycleTestHook func(string, *gorm.DB, uint)

func (s *Server) batchBooks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	request, ok := decodeBookControlRequest[batchBooksRequest](c, maxBookControlRequestBodyBytes, "invalid batch payload")
	if !ok {
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
	if len(request.CategoryIDs) > maxBookControlCategoryIDs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many categories"})
		return
	}
	switch request.Action {
	case "delete", "category", "category-add", "category-remove", "cache", "clear-cache":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported batch action"})
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
	var updatedCategoryIDs map[uint][]uint
	var cleanupPlans []bookCleanupPlan
	ctx := c.Request.Context()
	categoryAction := request.Action == "category" || request.Action == "category-add" || request.Action == "category-remove"
	transactionDB := s.db
	if categoryAction {
		if ctx.Err() != nil {
			return
		}
		transactionDB = s.db.WithContext(ctx)
	}
	err := transactionDB.Transaction(func(tx *gorm.DB) error {
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
			var currentBooks []models.Book
			if err := tx.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Find(&currentBooks).Error; err != nil {
				return err
			}
			currentByID := make(map[uint]models.Book, len(currentBooks))
			for _, book := range currentBooks {
				currentByID[book.ID] = book
			}
			currentBooks = currentBooks[:0]
			for _, bookID := range request.BookIDs {
				if book, exists := currentByID[bookID]; exists {
					currentBooks = append(currentBooks, book)
				}
			}
			categoryIDsByBookID, err := loadBookCategoryIDsByBookID(tx, userID, currentBooks)
			if err != nil {
				return err
			}
			for i := range currentBooks {
				nextIDs := requestCategoryIDs(*request)
				if request.Action == "category-add" {
					nextIDs = mergeCategoryID(currentBooks[i], categoryIDsByBookID[currentBooks[i].ID], request.CategoryID)
				} else if request.Action == "category-remove" {
					nextIDs = removeCategoryID(currentBooks[i], categoryIDsByBookID[currentBooks[i].ID], request.CategoryID)
				}
				var primaryCategoryID any
				if len(nextIDs) > 0 {
					primaryCategoryID = nextIDs[0]
				}
				if batchBookCategoryWriteLifecycleTestHook != nil {
					batchBookCategoryWriteLifecycleTestHook("before_book_write", tx, currentBooks[i].ID)
				}
				write := tx.Model(&models.Book{}).
					Where("id = ? AND user_id = ?", currentBooks[i].ID, userID).
					Update("category_id", primaryCategoryID)
				if write.Error != nil {
					return write.Error
				}
				if write.RowsAffected == 0 {
					continue
				}
				if err := s.setBookCategories(tx, userID, currentBooks[i].ID, nextIDs); err != nil {
					return err
				}
				if batchBookCategoryWriteLifecycleTestHook != nil {
					batchBookCategoryWriteLifecycleTestHook("after_book_write", tx, currentBooks[i].ID)
				}
				affected++
			}
			updatedBooks = make([]models.Book, 0, affected)
			if affected == 0 {
				updatedCategoryIDs = map[uint][]uint{}
				return nil
			}
			var reloaded []models.Book
			if err := tx.Where("user_id = ? AND id IN ?", userID, request.BookIDs).Find(&reloaded).Error; err != nil {
				return err
			}
			reloadedByID := make(map[uint]models.Book, len(reloaded))
			for _, book := range reloaded {
				reloadedByID[book.ID] = book
			}
			for _, bookID := range request.BookIDs {
				if book, exists := reloadedByID[bookID]; exists {
					updatedBooks = append(updatedBooks, book)
				}
			}
			affected = int64(len(updatedBooks))
			updatedCategoryIDs, err = loadBookCategoryIDsByBookID(tx, userID, updatedBooks)
			return err
		default:
			return fmt.Errorf("unsupported batch action")
		}
		return nil
	})
	if err != nil {
		if categoryAction {
			if ctx.Err() != nil || isRequestContextError(err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update book categories"})
			return
		}
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
			var progress models.ReadingProgress
			_ = s.db.Where("user_id = ? AND book_id = ?", userID, book.ID).First(&progress).Error
			items = append(items, s.projectBookShelfListItem(
				book,
				updatedCategoryIDs[book.ID],
				progress,
				s.cachedChapterCount(book.ID, book.SourceID),
			))
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
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, bookIDs).Find(&books).Error; err != nil {
		if isRequestContextError(err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load books"})
		return
	}

	cached := 0
	requested := 0
	failed := 0
	for i := range books {
		if err := ctx.Err(); err != nil {
			return
		}
		if books[i].SourceID == 0 {
			continue
		}
		result, err := s.cacheBookChapters(ctx, books[i], nil, true, 10, false, nil)
		cached += result.SelectedCached
		requested += result.Total
		if isRequestContextError(err) || ctx.Err() != nil {
			return
		}
		if err != nil {
			failed++
		}
	}
	if err := ctx.Err(); err != nil {
		return
	}
	items := make([]bookListItem, 0, len(books))
	for _, book := range books {
		items = append(items, s.bookShelfListItem(userID, book))
	}
	if err := ctx.Err(); err != nil {
		return
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
	for _, book := range books {
		_, _ = s.chapterImages.RemoveBook(book)
	}
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
	request, ok := decodeBookControlRequest[bookIDsRequest](c, maxBookControlRequestBodyBytes, "bookIds is required")
	if !ok {
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
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format != "" && format != "json" && format != "txt" && format != "epub" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported export format"})
		return
	}
	ownedBookIDs, ok := s.requireOwnedBookIDs(c, userID, request.BookIDs)
	if !ok {
		return
	}
	request.BookIDs = ownedBookIDs

	var books []models.Book
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, request.BookIDs).Order("updated_at desc").Find(&books).Error; err != nil {
		if isRequestContextError(err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load books"})
		return
	}
	if format == "" || format == "json" {
		s.exportBooksJSON(c, userID, books)
		return
	}
	if len(books) == 1 && books[0].SourceID == 0 && (format == "txt" || format == "epub") {
		if s.exportOriginalLocalBook(c, books[0]) {
			return
		}
		if ctx.Err() != nil {
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
}

func (s *Server) exportOriginalLocalBook(c *gin.Context, book models.Book) bool {
	if err := c.Request.Context().Err(); err != nil {
		return false
	}
	source, ok := s.openLocalBookSource(book)
	if !ok {
		return false
	}
	defer source.close()
	content, err := readOpenedFile(source.file)
	if err != nil {
		return false
	}
	if err := c.Request.Context().Err(); err != nil {
		return false
	}
	setAttachmentHeader(c, source.name)
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
	ctx := c.Request.Context()
	for _, book := range books {
		if err := ctx.Err(); err != nil {
			return
		}
		var chapters []models.Chapter
		if err := s.db.WithContext(ctx).Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
			if isRequestContextError(err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load chapters"})
			return
		}
		var bookmarks []models.Bookmark
		if err := s.db.WithContext(ctx).Where("user_id = ? AND book_id = ?", userID, book.ID).Order("updated_at desc").Find(&bookmarks).Error; err != nil {
			if isRequestContextError(err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load bookmarks"})
			return
		}
		exported = append(exported, exportedBook{
			Book:      book,
			Chapters:  chapters,
			Bookmarks: bookmarks,
		})
	}
	if err := ctx.Err(); err != nil {
		return
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
	ctx := c.Request.Context()
	if len(books) == 1 {
		book := books[0]
		content, err := s.exportBookPlainTextContext(ctx, book)
		if err != nil {
			if isRequestContextError(err) {
				return
			}
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
		if err := ctx.Err(); err != nil {
			_ = zipWriter.Close()
			return
		}
		content, err := s.exportBookPlainTextContext(ctx, book)
		if err != nil {
			_ = zipWriter.Close()
			if isRequestContextError(err) {
				return
			}
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
	if err := ctx.Err(); err != nil {
		return
	}
	setAttachmentHeader(c, "openreader-books-txt.zip")
	c.Data(http.StatusOK, "application/zip", buffer.Bytes())
}

func (s *Server) exportBooksEPUB(c *gin.Context, books []models.Book) {
	ctx := c.Request.Context()
	if len(books) == 1 {
		book := books[0]
		content, err := s.exportBookEPUBContext(ctx, book)
		if err != nil {
			if isRequestContextError(err) {
				return
			}
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
		if err := ctx.Err(); err != nil {
			_ = zipWriter.Close()
			return
		}
		content, err := s.exportBookEPUBContext(ctx, book)
		if err != nil {
			_ = zipWriter.Close()
			if isRequestContextError(err) {
				return
			}
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
	if err := ctx.Err(); err != nil {
		return
	}
	setAttachmentHeader(c, "openreader-books-epub.zip")
	c.Data(http.StatusOK, "application/zip", buffer.Bytes())
}

func (s *Server) exportBookPlainText(book models.Book) (string, error) {
	return s.exportBookPlainTextContext(context.Background(), book)
}

func (s *Server) exportBookPlainTextContext(ctx context.Context, book models.Book) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var chapters []models.Chapter
	if err := s.db.WithContext(ctx).Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
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
		if err := ctx.Err(); err != nil {
			return "", err
		}
		chapterTitle := strings.TrimSpace(chapter.Title)
		if chapterTitle != "" {
			builder.WriteString(chapterTitle)
			builder.WriteString("\n\n")
		}
		content, err := s.loadChapterTextContextResult(ctx, book, &chapter)
		if isRequestContextError(err) {
			return "", err
		}
		content = strings.TrimSpace(content)
		if content != "" {
			builder.WriteString(content)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

type exportedChapterContent struct {
	Title      string
	Content    string
	ChapterURL string
	Images     map[string]string
}

type exportedChapterImage struct {
	Key         string
	Href        string
	ContentType string
	Data        []byte
}

func (s *Server) exportBookEPUB(book models.Book) ([]byte, error) {
	return s.exportBookEPUBContext(context.Background(), book)
}

func (s *Server) exportBookEPUBContext(ctx context.Context, book models.Book) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var chapters []models.Chapter
	if err := s.db.WithContext(ctx).Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		return nil, err
	}
	contents := make([]exportedChapterContent, 0, len(chapters))
	imagesByKey := make(map[string]exportedChapterImage)
	for _, chapter := range chapters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content, err := s.loadChapterTextContextResult(ctx, book, &chapter)
		if isRequestContextError(err) {
			return nil, err
		}
		content = strings.TrimSpace(content)
		imageHrefs := make(map[string]string)
		if book.SourceID > 0 {
			if cachedFiles, imageErr := s.chapterImages.CachedFiles(book, chapter, content); imageErr == nil {
				for _, cachedFile := range cachedFiles {
					extension := chapterimage.ExtensionForContentType(cachedFile.ContentType)
					if extension == "" {
						continue
					}
					href := "Images/" + cachedFile.Key + extension
					imageHrefs[cachedFile.OriginalURL] = href
					imagesByKey[cachedFile.Key] = exportedChapterImage{
						Key:         cachedFile.Key,
						Href:        href,
						ContentType: cachedFile.ContentType,
						Data:        cachedFile.Data,
					}
				}
			}
		}
		contents = append(contents, exportedChapterContent{
			Title:      strings.TrimSpace(chapter.Title),
			Content:    content,
			ChapterURL: strings.TrimSpace(chapter.URL),
			Images:     imageHrefs,
		})
	}
	images := make([]exportedChapterImage, 0, len(imagesByKey))
	for _, image := range imagesByKey {
		images = append(images, image)
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Key < images[j].Key })

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
	if err := writeEPUBFile(zipWriter, "OEBPS/content.opf", []byte(epubContentOPF(book, contents, images))); err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	if err := writeEPUBFile(zipWriter, "OEBPS/nav.xhtml", []byte(epubNavXHTML(book, contents))); err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	for _, image := range images {
		if err := writeEPUBFile(zipWriter, "OEBPS/"+image.Href, image.Data); err != nil {
			_ = zipWriter.Close()
			return nil, err
		}
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

func epubContentOPF(book models.Book, chapters []exportedChapterContent, images []exportedChapterImage) string {
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
	for index, image := range images {
		manifest.WriteString(fmt.Sprintf(`    <item id="image-%04d" href="%s" media-type="%s"/>`+"\n",
			index+1, html.EscapeString(image.Href), html.EscapeString(image.ContentType)))
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
	paragraphs := epubChapterParagraphs(chapter)
	bookTitle := html.EscapeString(strings.TrimSpace(book.Title))
	if bookTitle == "" {
		bookTitle = "OpenReader Book"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="zh-CN">
<head>
  <title>%s - %s</title>
  <style>body{line-height:1.8;font-family:serif;}p{text-indent:2em;margin:0 0 1em;}img{display:block;max-width:100%%;height:auto;margin:0.8em auto;}img.full-image{width:100%%;}</style>
</head>
<body>
  <section>
    <h1>%s</h1>
%s  </section>
</body>
</html>`, bookTitle, title, title, paragraphs)
}

func epubChapterParagraphs(chapter exportedChapterContent) string {
	var paragraphs strings.Builder
	for _, rawLine := range strings.Split(chapter.Content, "\n") {
		if strings.TrimSpace(rawLine) == "" {
			continue
		}
		tokenizer := xhtml.NewTokenizer(strings.NewReader(rawLine))
		var body strings.Builder
		for {
			tokenType := tokenizer.Next()
			if tokenType == xhtml.ErrorToken {
				break
			}
			token := tokenizer.Token()
			switch tokenType {
			case xhtml.TextToken:
				body.WriteString(html.EscapeString(token.Data))
			case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
				if strings.EqualFold(token.Data, "br") {
					body.WriteString("<br/>")
					continue
				}
				if !strings.EqualFold(token.Data, "img") {
					continue
				}
				source := epubImageAttribute(token.Attr, "src", "data-src", "data-original", "data-url")
				normalized := normalizeEPUBImageURL(source, chapter.ChapterURL)
				href := chapter.Images[normalized]
				alt := epubImageAttribute(token.Attr, "alt")
				if href == "" {
					if alt != "" {
						body.WriteString(`<span class="missing-image">`)
						body.WriteString(html.EscapeString(alt))
						body.WriteString(`</span>`)
					}
					continue
				}
				body.WriteString(`<img src="`)
				body.WriteString(html.EscapeString(href))
				body.WriteString(`" alt="`)
				body.WriteString(html.EscapeString(alt))
				if strings.EqualFold(epubImageAttribute(token.Attr, "data-image-style"), "FULL") {
					body.WriteString(`" class="full-image`)
				}
				body.WriteString(`"/>`)
			}
		}
		if strings.TrimSpace(body.String()) == "" {
			continue
		}
		paragraphs.WriteString("    <p>")
		paragraphs.WriteString(body.String())
		paragraphs.WriteString("</p>\n")
	}
	return paragraphs.String()
}

func epubImageAttribute(attributes []xhtml.Attribute, names ...string) string {
	for _, name := range names {
		for _, attribute := range attributes {
			if strings.EqualFold(strings.TrimSpace(attribute.Key), name) {
				if value := strings.TrimSpace(attribute.Val); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func normalizeEPUBImageURL(raw, chapterURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() {
		base, baseErr := url.Parse(strings.TrimSpace(chapterURL))
		if baseErr != nil || base.Scheme == "" || base.Host == "" {
			return ""
		}
		parsed = base.ResolveReference(parsed)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	parsed.Fragment = ""
	return parsed.String()
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
	ctx := c.Request.Context()
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

	source, err := s.bookSources.FindForBook(userID, book.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}
	remoteInfo, remoteChapters, variable, err := engine.FetchBookInfoAndTOCWithVariablesContext(ctx, book.URL, source, book.Variable, book.Title, nil)
	if err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadRequest, "failed to fetch chapters", err, "book_info")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if len(remoteChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}

	var supersededCachePaths []string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", book.ID, userID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errRemoteBookRefreshStale
			}
			return err
		}
		if !sameRemoteBookRefreshSnapshot(current, book) {
			return errRemoteBookRefreshStale
		}
		previousChapterCount := current.ChapterCount
		nextChapters := make([]models.Chapter, 0, len(remoteChapters))
		for _, remoteChapter := range remoteChapters {
			nextChapters = append(nextChapters, models.Chapter{
				BookID:   current.ID,
				Index:    remoteChapter.Index,
				Title:    remoteChapter.Title,
				URL:      remoteChapter.URL,
				IsVolume: remoteChapter.IsVolume,
				Tag:      remoteChapter.Tag,
				Variable: remoteChapter.Variable,
			})
		}
		var err error
		supersededCachePaths, _, err = s.replaceBookChapterRows(tx, userID, current.ID, nextChapters)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"title":         firstNonBlankCanRename(remoteInfo.Title, current.Title, remoteInfo.CanRename),
			"author":        firstNonBlankCanRename(remoteInfo.Author, current.Author, remoteInfo.CanRename),
			"cover_url":     firstNonBlank(remoteInfo.CoverURL, current.CoverURL),
			"intro":         firstNonBlank(remoteInfo.Intro, current.Intro),
			"kind":          firstNonBlank(remoteInfo.Kind, current.Kind),
			"word_count":    firstNonBlank(remoteInfo.WordCount, current.WordCount),
			"last_chapter":  remoteChapters[len(remoteChapters)-1].Title,
			"chapter_count": len(remoteChapters),
			"variable":      variable,
		}
		if len(remoteChapters) > previousChapterCount {
			updates["last_check_time"] = time.Now().UnixMilli()
		}
		write := tx.Model(&models.Book{}).
			Where("id = ? AND user_id = ? AND source_id = ? AND url = ? AND variable = ? AND updated_at = ?",
				current.ID, current.UserID, current.SourceID, current.URL, current.Variable, current.UpdatedAt).
			Updates(updates)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return errRemoteBookRefreshStale
		}
		return tx.Where("id = ? AND user_id = ?", current.ID, current.UserID).First(&book).Error
	})
	if err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		if errors.Is(err, errRemoteBookRefreshStale) {
			c.JSON(http.StatusConflict, gin.H{"error": "book changed during refresh"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh book"})
		return
	}
	s.pruneUnreferencedRemoteCachePaths(supersededCachePaths)
	_, _ = s.chapterImages.RemoveBook(book)

	c.JSON(http.StatusOK, gin.H{"book": s.broadcastBookShelfUpdate(userID, book), "added": len(remoteChapters), "chapterCount": len(remoteChapters)})
}

var errRemoteBookRefreshStale = errors.New("book changed during refresh")

func sameRemoteBookRefreshSnapshot(current, snapshot models.Book) bool {
	return current.ID == snapshot.ID &&
		current.UserID == snapshot.UserID &&
		current.SourceID == snapshot.SourceID &&
		current.URL == snapshot.URL &&
		current.Variable == snapshot.Variable &&
		current.UpdatedAt.Equal(snapshot.UpdatedAt)
}

func (s *Server) refreshLocalBook(c *gin.Context) {
	ctx := c.Request.Context()
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
	request, ok := decodeOptionalLocalRefreshRequest(c)
	if !ok {
		return
	}
	tocRule := strings.TrimSpace(book.TOCRule)
	if request.TOCRule != nil {
		tocRule = strings.TrimSpace(*request.TOCRule)
		if len(tocRule) > engine.MaxTXTTocRuleBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "toc rule is too large"})
			return
		}
	}
	if ctx.Err() != nil {
		return
	}

	source, ok := s.openLocalBookSource(book)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local source file not found"})
		return
	}
	defer source.close()
	if ctx.Err() != nil {
		return
	}
	legacyLimits := engine.LegacyLocalBookParseLimits()
	data, err := readBoundedOpenedLocalBookSourceContext(ctx, source.file, source.info, legacyLimits.MaxArchiveBytes)
	if err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		if errors.Is(err, engine.ErrLocalBookParseLimit) {
			c.JSON(http.StatusBadRequest, gin.H{"error": engine.ErrLocalBookParseLimit.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read local source file"})
		return
	}
	parsed, err := parseLocalBookChapters(filepath.Ext(source.name), data, tocRule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse local book: %v", err)})
		return
	}
	if ctx.Err() != nil {
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
	stage, nextChapters, err := s.stageLocalRefreshContext(ctx, book, source.archive, parsed, bookURL)
	if err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stage local refreshed content"})
		return
	}
	defer stage.cleanup()
	s.localCacheMu.Lock()
	defer s.localCacheMu.Unlock()

	lastChapter := strings.TrimSpace(parsed[len(parsed)-1].Title)
	if lastChapter == "" {
		lastChapter = fmt.Sprintf("第 %d 章", len(parsed))
	}
	var supersededCachePaths []string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", book.ID, userID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errLocalBookRefreshStale
			}
			return err
		}
		if !sameLocalBookRefreshSnapshot(current, book) {
			return errLocalBookRefreshStale
		}
		var nextChapterIDs map[int]uint
		var err error
		supersededCachePaths, nextChapterIDs, err = s.replaceBookChapterRows(tx, userID, current.ID, nextChapters)
		if err != nil {
			return err
		}
		write := tx.Model(&models.Book{}).
			Where("id = ? AND user_id = ? AND source_id = ? AND url = ? AND library_path = ? AND original_file = ? AND toc_file = ? AND source_file = ? AND toc_rule = ? AND updated_at = ?",
				current.ID, current.UserID, current.SourceID, current.URL, current.LibraryPath, current.OriginalFile, current.TOCFile, current.SourceFile, current.TOCRule, current.UpdatedAt).
			Updates(map[string]any{
				"url":           bookURL,
				"last_chapter":  lastChapter,
				"chapter_count": len(parsed),
				"toc_rule":      tocRule,
				"variable":      "",
			})
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return errLocalBookRefreshStale
		}
		if err := tx.Where("id = ? AND user_id = ?", current.ID, current.UserID).First(&book).Error; err != nil {
			return err
		}
		archivedChapters := make([]engine.ArchivedChapter, 0, len(parsed))
		for index, parsedChapter := range parsed {
			chapter := nextChapters[index]
			chapter.ID = nextChapterIDs[chapter.Index]
			archivedChapters = append(archivedChapters, engine.ArchivedChapter{
				ID:                  chapter.ID,
				URL:                 chapter.URL,
				Title:               chapter.Title,
				IsVolume:            false,
				BaseURL:             "",
				BookURL:             book.OriginalFile,
				Index:               chapter.Index,
				Start:               parsedChapter.Start,
				End:                 parsedChapter.End,
				CachePath:           chapter.CachePath,
				ResourcePath:        chapter.ResourcePath,
				ResourceFragment:    chapter.ResourceFragment,
				ResourceEndFragment: chapter.ResourceEndFragment,
			})
		}
		archive := engine.ArchivedBook{
			Directory:    book.LibraryPath,
			OriginalFile: book.OriginalFile,
			TOCFile:      book.TOCFile,
			SourceFile:   book.SourceFile,
		}
		return stage.stageArchiveMetadataContext(ctx, archive, archivedChapters, engine.ArchivedBookSource{
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
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		if errors.Is(err, errLocalBookRefreshStale) {
			c.JSON(http.StatusConflict, gin.H{"error": "book changed during refresh"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh local book"})
		return
	}
	if err := stage.promote(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish local refreshed content"})
		return
	}
	s.pruneSupersededLocalDerivedContent(book, source.archive, supersededCachePaths)

	c.JSON(http.StatusOK, gin.H{"book": s.broadcastBookShelfUpdate(userID, book), "chapterCount": len(parsed)})
}

var errLocalBookRefreshStale = errors.New("book changed during refresh")

func sameLocalBookRefreshSnapshot(current, snapshot models.Book) bool {
	return current.ID == snapshot.ID &&
		current.UserID == snapshot.UserID &&
		current.SourceID == snapshot.SourceID &&
		current.URL == snapshot.URL &&
		current.LibraryPath == snapshot.LibraryPath &&
		current.OriginalFile == snapshot.OriginalFile &&
		current.TOCFile == snapshot.TOCFile &&
		current.SourceFile == snapshot.SourceFile &&
		current.TOCRule == snapshot.TOCRule &&
		current.UpdatedAt.Equal(snapshot.UpdatedAt)
}

type cacheBookRequest struct {
	ChapterIndex *int `json:"chapterIndex"`
	All          bool `json:"all"`
	Count        int  `json:"count"`
	Refresh      bool `json:"refresh"`
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

	decoded, ok := decodeRemoteWorkRequest[cacheBookRequest](c, maxRemoteControlRequestBytes, "invalid cache payload")
	if !ok {
		return
	}
	request := *decoded

	if !request.All && request.ChapterIndex == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapterIndex is required"})
		return
	}
	if book.SourceID == 0 {
		c.JSON(http.StatusOK, gin.H{"cached": 0, "requested": 0, "message": "local books do not need server cache"})
		return
	}
	result, err := s.cacheBookChapters(c.Request.Context(), book, request.ChapterIndex, request.All, request.Count, request.Refresh, nil)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cache chapters"})
		return
	}
	item := s.broadcastBookShelfUpdate(userID, book)
	c.JSON(http.StatusOK, cacheBookResponse(result, item))
}

func cacheBookResponse(result chaptercache.Progress, book any) gin.H {
	response := gin.H{
		"cachedCount":  result.CachedCount,
		"successCount": result.SuccessCount,
		"failedCount":  result.FailedCount,
		"processed":    result.Processed,
		"total":        result.Total,
		"cached":       result.SelectedCached,
		"requested":    result.Total,
		"failed":       result.FailedCount,
	}
	if book != nil {
		response["book"] = book
	}
	return response
}

func deleteBookRecords(tx *gorm.DB, userID, bookID uint, book *models.Book) error {
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Delete(&models.BookSourceCandidate{}).Error; err != nil {
		return err
	}
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

	request, ok := decodeBookGroupWriteRequest[bookCategoryRequest](c, "invalid category payload")
	if !ok {
		return
	}
	nextIDs := categoryIDsFromRequest(request.CategoryID, request.CategoryIDs)
	if !s.validateCategoryIDs(c, userID, nextIDs) {
		return
	}

	if len(nextIDs) > 0 {
		book.CategoryID = &nextIDs[0]
	} else {
		book.CategoryID = nil
	}
	if bookPatchWriteLifecycleTestHook != nil {
		bookPatchWriteLifecycleTestHook("category")
	}
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		return
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", bookID, userID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBookPatchTargetNotFound
			}
			return err
		}
		var categoryID any
		if len(nextIDs) > 0 {
			categoryID = nextIDs[0]
		}
		write := tx.Model(&models.Book{}).
			Where("id = ? AND user_id = ?", current.ID, current.UserID).
			Update("category_id", categoryID)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return errBookPatchTargetNotFound
		}
		if err := s.setBookCategories(tx, userID, current.ID, nextIDs); err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ?", current.ID, current.UserID).First(&book).Error
	}); err != nil {
		if ctx.Err() != nil || isRequestContextError(err) {
			return
		}
		if errors.Is(err, errBookPatchTargetNotFound) {
			notFound(c, "book not found")
			return
		}
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

// remoteBookExistingAddWriteLifecycleTestHook exposes deterministic barriers
// around the existing-URL branch for package-level lifecycle contract tests.
var remoteBookExistingAddWriteLifecycleTestHook func(string, *gorm.DB, uint)

var errRemoteBookExistingAddTargetNotFound = errors.New("existing remote book not found during add")

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

	req, ok := decodeBookControlRequest[remoteBookRequest](c, maxRemoteBookControlRequestBodyBytes, "title, bookUrl, and sourceId are required")
	if !ok {
		return
	}
	if !normalizeRemoteBookRequest(c, req) {
		return
	}
	if req.Title == "" || req.BookURL == "" || req.SourceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, bookUrl, and sourceId are required"})
		return
	}
	if len(req.CategoryIDs) > maxBookControlCategoryIDs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many categories"})
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
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return
	}

	source, err := s.bookSources.FindActive(userID, req.SourceID)
	if errors.Is(err, booksources.ErrSourceNotFound) || err == nil && !source.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	var existing models.Book
	var existingCategoryIDs []uint
	if err := s.db.WithContext(ctx).Where("user_id = ? AND url = ?", userID, req.BookURL).First(&existing).Error; err == nil {
		if remoteBookExistingAddWriteLifecycleTestHook != nil {
			remoteBookExistingAddWriteLifecycleTestHook("after_lookup", nil, existing.ID)
		}
		hasCategorySelection := len(req.CategoryIDs) > 0 || req.CategoryID != nil
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("id = ? AND user_id = ?", existing.ID, userID).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errRemoteBookExistingAddTargetNotFound
				}
				return err
			}
			if hasCategorySelection {
				var primaryCategoryID any
				if len(categoryIDs) > 0 {
					primaryCategoryID = categoryIDs[0]
				}
				write := tx.Model(&models.Book{}).
					Where("id = ? AND user_id = ?", existing.ID, userID).
					Update("category_id", primaryCategoryID)
				if write.Error != nil {
					return write.Error
				}
				if write.RowsAffected != 1 {
					return errRemoteBookExistingAddTargetNotFound
				}
				if err := s.setBookCategories(tx, userID, existing.ID, categoryIDs); err != nil {
					return err
				}
				if remoteBookExistingAddWriteLifecycleTestHook != nil {
					remoteBookExistingAddWriteLifecycleTestHook("after_relation_write", tx, existing.ID)
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if remoteBookExistingAddWriteLifecycleTestHook != nil {
					remoteBookExistingAddWriteLifecycleTestHook("after_book_write", tx, existing.ID)
				}
			}
			if err := tx.Where("id = ? AND user_id = ?", existing.ID, userID).First(&existing).Error; err != nil {
				return err
			}
			categoryIDsByBookID, err := loadBookCategoryIDsByBookID(tx, userID, []models.Book{existing})
			if err != nil {
				return err
			}
			existingCategoryIDs = categoryIDsByBookID[existing.ID]
			return nil
		}); err != nil {
			if ctx.Err() != nil || isRequestContextError(err) {
				return
			}
			if errors.Is(err, errRemoteBookExistingAddTargetNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
				notFound(c, "book not found")
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update book categories"})
			return
		}
		if err := ctx.Err(); err != nil {
			return
		}
		c.JSON(http.StatusOK, s.broadcastBookShelfUpdateWithCategoryIDs(userID, existing, existingCategoryIDs))
		return
	}

	remoteInfo, chapters, variable, err := engine.FetchBookInfoAndTOCWithVariablesContext(ctx, req.BookURL, source, variable, req.Title, nil)
	if err != nil {
		if isRequestContextError(err) || ctx.Err() != nil {
			return
		}
		s.recordSourceFailure(userID, source, err)
		writeSourceError(c, http.StatusBadRequest, "failed to fetch chapters", err, "book_info")
		return
	}
	if err := ctx.Err(); err != nil {
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

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := tx.Create(&book).Error; err != nil {
			return err
		}
		if err := s.setBookCategories(tx, userID, book.ID, categoryIDs); err != nil {
			return err
		}
		if err := s.sourceCandidates.SeedCurrent(tx, book, &source); err != nil {
			return err
		}
		for _, ch := range chapters {
			if err := ctx.Err(); err != nil {
				return err
			}
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
		return ctx.Err()
	})
	if err != nil {
		if isRequestContextError(err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create book"})
		return
	}
	if err := ctx.Err(); err != nil {
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

var errReaderSourceChangeStale = errors.New("book changed during source switch")

type contentMatch struct {
	ChapterID                uint    `json:"chapterId"`
	ChapterIndex             int     `json:"chapterIndex"`
	ChapterTitle             string  `json:"chapterTitle"`
	Excerpt                  string  `json:"excerpt"`
	Query                    string  `json:"query"`
	ResultCountWithinChapter int     `json:"resultCountWithinChapter"`
	QueryIndexInResult       int     `json:"queryIndexInResult"`
	QueryIndexInChapter      int     `json:"queryIndexInChapter"`
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

	req, ok := decodeBookControlRequest[changeSourceRequest](c, maxRemoteBookControlRequestBodyBytes, "sourceId is required")
	if !ok {
		return
	}
	if !normalizeChangeSourceRequest(c, req) {
		return
	}
	if req.SourceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourceId is required"})
		return
	}

	newSource, err := s.bookSources.FindActive(userID, req.SourceID)
	if errors.Is(err, booksources.ErrSourceNotFound) || err == nil && !newSource.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
		return
	}

	newBookURL := strings.TrimSpace(req.BookURL)
	if newBookURL == "" {
		newBookURL = book.URL
	}
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return
	}
	remoteInfo, newChapters, variable, err := engine.FetchBookInfoAndTOCWithVariablesContext(ctx, newBookURL, newSource, "", book.Title, nil)
	if err != nil {
		if isRequestContextError(err) || ctx.Err() != nil {
			return
		}
		s.recordSourceFailure(userID, newSource, err)
		writeSourceError(c, http.StatusBadRequest, "failed to fetch chapters from new source", err, "book_info")
		return
	}
	if len(newChapters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source returned no chapters"})
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}

	var supersededCachePaths []string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", book.ID, userID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errReaderSourceChangeStale
			}
			return err
		}
		if current.SourceID != book.SourceID || current.URL != book.URL {
			return errReaderSourceChangeStale
		}
		currentSource, err := validateReaderSourceChangeTarget(tx, userID, newSource)
		if err != nil {
			return err
		}
		nextChapters := make([]models.Chapter, 0, len(newChapters))
		for _, ch := range newChapters {
			if err := ctx.Err(); err != nil {
				return err
			}
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
		supersededCachePaths, _, err = s.replaceBookChapterRows(tx, userID, bookID, nextChapters)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"source_id":       currentSource.ID,
			"type":            currentSource.SourceType,
			"url":             newBookURL,
			"variable":        variable,
			"title":           firstNonBlankCanRename(remoteInfo.Title, firstNonBlank(req.Title, current.Title), remoteInfo.CanRename),
			"author":          firstNonBlankCanRename(remoteInfo.Author, firstNonBlank(req.Author, current.Author), remoteInfo.CanRename),
			"cover_url":       firstNonBlank(remoteInfo.CoverURL, req.CoverURL, current.CoverURL),
			"intro":           firstNonBlank(remoteInfo.Intro, req.Intro, current.Intro),
			"kind":            firstNonBlank(remoteInfo.Kind, req.Kind, current.Kind),
			"word_count":      firstNonBlank(remoteInfo.WordCount, req.WordCount, current.WordCount),
			"last_chapter":    newChapters[len(newChapters)-1].Title,
			"chapter_count":   len(newChapters),
			"last_check_time": time.Now().UnixMilli(),
		}
		write := tx.Model(&models.Book{}).
			Where("id = ? AND user_id = ? AND source_id = ? AND url = ?", current.ID, current.UserID, current.SourceID, current.URL).
			Updates(updates)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return errReaderSourceChangeStale
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", current.ID, current.UserID).First(&book).Error; err != nil {
			return err
		}
		return s.sourceCandidates.SeedCurrent(tx, book, &currentSource)
	})
	if err != nil {
		if isRequestContextError(err) {
			return
		}
		if errors.Is(err, errReaderSourceChangeStale) {
			c.JSON(http.StatusConflict, gin.H{"error": errReaderSourceChangeStale.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change source"})
		return
	}
	s.pruneUnreferencedRemoteCachePaths(supersededCachePaths)
	_, _ = s.chapterImages.RemoveBook(book)
	if err := ctx.Err(); err != nil {
		return
	}

	c.JSON(http.StatusOK, s.broadcastBookShelfUpdate(userID, book))
}

func validateReaderSourceChangeTarget(
	db *gorm.DB,
	userID uint,
	snapshot models.BookSource,
) (models.BookSource, error) {
	var association models.UserBookSource
	if err := db.Where(
		"user_id = ? AND source_id = ? AND detached = ?",
		userID,
		snapshot.ID,
		false,
	).First(&association).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.BookSource{}, errReaderSourceChangeStale
		}
		return models.BookSource{}, err
	}
	var source models.BookSource
	if err := db.First(&source, snapshot.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.BookSource{}, errReaderSourceChangeStale
		}
		return models.BookSource{}, err
	}
	if !sameBookSourceFetchSemantics(source, snapshot) {
		return models.BookSource{}, errReaderSourceChangeStale
	}
	return source, nil
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
		if errors.Is(contentErr, errReaderChapterContentStale) {
			c.JSON(http.StatusConflict, gin.H{"error": errReaderChapterContentStale.Error()})
			return
		}
		if isRequestContextError(contentErr) {
			return
		}
		if book.SourceID > 0 {
			if source, err := s.bookSources.FindForBook(userID, book.SourceID); err == nil {
				s.recordSourceFailure(userID, source, contentErr)
			}
		}
		writeSourceError(c, http.StatusBadGateway, "failed to load chapter content", contentErr, "content")
		return
	}
	var currentChapter models.Chapter
	if err := s.db.WithContext(c.Request.Context()).
		Where("id = ? AND book_id = ? AND `index` = ? AND url = ?", chapter.ID, book.ID, chapter.Index, chapter.URL).
		First(&currentChapter).Error; err != nil {
		if isRequestContextError(err) {
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": errReaderChapterContentStale.Error()})
		return
	}
	chapter = currentChapter
	response := gin.H{
		"chapter": chapter,
		"content": content,
		"format":  "text",
	}
	if book.SourceID > 0 && book.Type != 1 {
		if cachedImages, expiresAt, imageErr := s.chapterImages.CachedImages(book, chapter, content); imageErr == nil && len(cachedImages) > 0 {
			response["cachedImages"] = cachedImages
			response["cachedImagesExpiresAt"] = expiresAt.UTC().Format(time.RFC3339)
		}
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
	keyword := exactContentSearchQuery(c.Request.URL.Query())
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}
	if err := s.requireContentSearchSource(book); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, booksources.ErrSourceNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未配置书源"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load source"})
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

	scan := s.collectContentMatchesContext(c.Request.Context(), book, chapters, keyword, 0, len(chapters), 200, 20)
	if scan.Canceled {
		return
	}
	c.JSON(http.StatusOK, scan.Matches)
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
	QueryIndexInResult       int     `json:"queryIndexInResult"`
	QueryIndexInChapter      int     `json:"queryIndexInChapter"`
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
		request := &req
		if err := decodeBoundedSingleUTF8JSON(c, &request, maxBookControlRequestBodyBytes); err != nil || request == nil {
			c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "请求格式不正确"})
			return
		}
		req = *request
		if req.LastIndex != nil {
			lastIndex := min(max(*req.LastIndex, -1), 1000000)
			req.LastIndex = &lastIndex
		}
		if req.Size != nil {
			size := min(max(*req.Size, 1), 20000)
			req.Size = &size
		}
	}

	bookURL := strings.TrimSpace(firstNonBlank(req.URL, req.BookURL))
	if bookURL == "" {
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "请输入书籍链接"})
		return
	}
	keyword := req.Keyword
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
	if err := s.requireContentSearchSource(book); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, booksources.ErrSourceNotFound) {
			c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "未配置书源"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"isSuccess": false, "errorMsg": "加载书源失败"})
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
	scan := s.collectContentMatchesContext(c.Request.Context(), book, chapters, keyword, start, len(chapters)-start, max(size, 1), max(size, 1))
	if scan.Canceled {
		return
	}
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

func exactContentSearchQuery(values url.Values) string {
	if query, ok := values["q"]; ok && len(query) > 0 {
		return query[0]
	}
	if query, ok := values["keyword"]; ok && len(query) > 0 {
		return query[0]
	}
	return ""
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
			QueryIndexInResult:       match.QueryIndexInResult,
			QueryIndexInChapter:      match.QueryIndexInChapter,
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
		content, err := s.loadChapterSearchTextContextResult(ctx, book, &chapters[i])
		if err != nil {
			if ctx.Err() != nil {
				scan.Canceled = true
				return scan
			}
			scan.UnavailableChapters++
			continue
		}
		chapterMatches, chapterTruncated := contentsearch.Find(content, keyword, contentSearchMaxMatchesPerChapter)
		scan.Truncated = scan.Truncated || chapterTruncated
		for matchIndex, match := range chapterMatches {
			scan.Matches = append(scan.Matches, contentMatch{
				ChapterID:                chapters[i].ID,
				ChapterIndex:             chapters[i].Index,
				ChapterTitle:             chapters[i].Title,
				Excerpt:                  match.Excerpt,
				Query:                    keyword,
				ResultCountWithinChapter: matchIndex,
				QueryIndexInResult:       match.QueryIndexInResult,
				QueryIndexInChapter:      match.QueryIndexInChapter,
				Offset:                   match.ByteOffset,
				LineIndex:                lineIndexAtByte(content, match.ByteOffset),
				Percent:                  float64(match.ByteOffset) / float64(max(len(content), 1)),
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

func (s *Server) loadChapterText(book models.Book, chapter *models.Chapter) string {
	return s.loadChapterTextContext(context.Background(), book, chapter)
}

func (s *Server) loadChapterTextContext(ctx context.Context, book models.Book, chapter *models.Chapter) string {
	content, _ := s.loadChapterTextContextResult(ctx, book, chapter)
	return content
}

func (s *Server) loadChapterTextContextResult(ctx context.Context, book models.Book, chapter *models.Chapter) (string, error) {
	return s.loadChapterTextContextResultWithPolicy(ctx, &book, chapter, chapterTextLoadPolicy{
		ApplyReaderReplaceRules: true,
	})
}

func (s *Server) loadChapterTextContextResultWithOptions(ctx context.Context, book *models.Book, chapter *models.Chapter, refresh bool) (string, error) {
	return s.loadChapterTextContextResultWithPolicy(ctx, book, chapter, chapterTextLoadPolicy{
		Refresh:                 refresh,
		ApplyReaderReplaceRules: true,
	})
}

func (s *Server) loadChapterSearchTextContextResult(ctx context.Context, book models.Book, chapter *models.Chapter) (string, error) {
	return s.loadChapterTextContextResultWithPolicy(ctx, &book, chapter, chapterTextLoadPolicy{})
}

type chapterTextLoadPolicy struct {
	Refresh                 bool
	ApplyReaderReplaceRules bool
}

var errReaderChapterContentStale = errors.New("chapter content changed; retry")

type readerChapterContentSnapshot struct {
	book    models.Book
	chapter models.Chapter
	source  models.BookSource
}

type readerChapterGateKey struct {
	userID    uint
	bookID    uint
	chapterID uint
}

type readerChapterGate struct {
	token chan struct{}
	refs  int
}

// readerChapterContentLifecycleTestHook exposes deterministic boundaries for
// request-lifecycle contract tests without changing production behavior.
var readerChapterContentLifecycleTestHook func(string, context.Context, models.Book, models.Chapter)

func (s *Server) loadChapterTextContextResultWithPolicy(ctx context.Context, book *models.Book, chapter *models.Chapter, policy chapterTextLoadPolicy) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	content := ""
	if !policy.Refresh && chapter.CachePath != "" {
		if bytes, path, err := s.readChapterCache(*book, chapter.CachePath); err == nil {
			content = string(bytes)
			if book.SourceID == 0 {
				if normalizedPath := s.localChapterCachePath(*book, path); normalizedPath != "" && normalizedPath != chapter.CachePath {
					if readerChapterContentLifecycleTestHook != nil {
						readerChapterContentLifecycleTestHook("before_cache_path_normalize", ctx, *book, *chapter)
					}
					s.normalizeChapterCachePath(ctx, chapter, normalizedPath)
				}
			} else if path != "" && path != chapter.CachePath {
				if readerChapterContentLifecycleTestHook != nil {
					readerChapterContentLifecycleTestHook("before_cache_path_normalize", ctx, *book, *chapter)
				}
				normalizedPath := s.remoteChapterCachePath(path)
				if normalizedPath == "" {
					normalizedPath = path
				}
				s.normalizeChapterCachePath(ctx, chapter, normalizedPath)
			}
		}
	}

	if content == "" && book.SourceID == 0 {
		var currentBook models.Book
		var currentChapter models.Chapter
		var rebuildErr error
		content, currentBook, currentChapter, rebuildErr = s.rebuildLocalChapterTextContext(ctx, *book, *chapter)
		if rebuildErr != nil {
			return "", rebuildErr
		}
		if content != "" {
			*book = currentBook
			*chapter = currentChapter
		}
	}

	if content == "" && chapter.URL != "" && book.SourceID > 0 {
		release, err := s.acquireReaderChapterGate(ctx, readerChapterGateKey{
			userID:    book.UserID,
			bookID:    book.ID,
			chapterID: chapter.ID,
		})
		if err != nil {
			return "", err
		}
		defer release()

		currentBook, currentChapter, err := s.reloadReaderChapterFetchState(ctx, *book, *chapter)
		if err != nil {
			return "", err
		}
		*book = currentBook
		*chapter = currentChapter
		if !policy.Refresh && chapter.CachePath != "" {
			if cached, path, cacheErr := s.readChapterCache(*book, chapter.CachePath); cacheErr == nil {
				if path != "" && path != chapter.CachePath {
					normalizedPath := s.remoteChapterCachePath(path)
					if normalizedPath == "" {
						normalizedPath = path
					}
					s.normalizeChapterCachePath(ctx, chapter, normalizedPath)
				}
				content = string(cached)
			}
		}
		if content != "" {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if policy.ApplyReaderReplaceRules && !epubreader.IsLocalEPUB(*book) && book.Type != 1 {
				content = s.applyUserReplaceRules(*book, content)
			}
			return content, nil
		}

		source, err := s.bookSources.FindForBook(book.UserID, book.SourceID)
		if err != nil {
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
		snapshot := readerChapterContentSnapshot{book: *book, chapter: *chapter, source: source}
		fetched, variableState, fetchErr := engine.FetchChapterContentContextWithState(ctx, chapter.URL, nextChapterURL, source, engine.SourceRuleVariableState{
			BookVariable:    book.Variable,
			ChapterVariable: chapter.Variable,
			BookName:        book.Title,
			ChapterTitle:    chapter.Title,
		})
		if fetchErr != nil {
			return "", fetchErr
		}
		if readerChapterContentLifecycleTestHook != nil {
			readerChapterContentLifecycleTestHook("after_remote_fetch", ctx, *book, *chapter)
		}
		currentBook, currentChapter, persistErr := s.persistRemoteChapterFetch(ctx, snapshot, fetched, variableState)
		if persistErr != nil {
			return "", persistErr
		}
		*book = currentBook
		*chapter = currentChapter
		if fetched != "" {
			content = fetched
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if policy.ApplyReaderReplaceRules && !epubreader.IsLocalEPUB(*book) && book.Type != 1 {
		content = s.applyUserReplaceRules(*book, content)
	}
	return content, nil
}

func (s *Server) acquireReaderChapterGate(ctx context.Context, key readerChapterGateKey) (func(), error) {
	s.remoteChapterMu.Lock()
	if s.remoteChapterMap == nil {
		s.remoteChapterMap = make(map[readerChapterGateKey]*readerChapterGate)
	}
	gate := s.remoteChapterMap[key]
	if gate == nil {
		gate = &readerChapterGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.remoteChapterMap[key] = gate
	}
	gate.refs++
	s.remoteChapterMu.Unlock()

	select {
	case <-ctx.Done():
		s.releaseReaderChapterGateRef(key, gate)
		return nil, ctx.Err()
	case <-gate.token:
		return func() {
			gate.token <- struct{}{}
			s.releaseReaderChapterGateRef(key, gate)
		}, nil
	}
}

func (s *Server) releaseReaderChapterGateRef(key readerChapterGateKey, gate *readerChapterGate) {
	s.remoteChapterMu.Lock()
	defer s.remoteChapterMu.Unlock()
	gate.refs--
	if gate.refs == 0 && s.remoteChapterMap[key] == gate {
		delete(s.remoteChapterMap, key)
	}
}

func (s *Server) reloadReaderChapterFetchState(
	ctx context.Context,
	requestedBook models.Book,
	requestedChapter models.Chapter,
) (models.Book, models.Chapter, error) {
	var book models.Book
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", requestedBook.ID, requestedBook.UserID).
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if book.SourceID != requestedBook.SourceID || book.Type != requestedBook.Type || book.URL != requestedBook.URL {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}

	var chapter models.Chapter
	if err := s.db.WithContext(ctx).
		Where("id = ? AND book_id = ?", requestedChapter.ID, requestedBook.ID).
		First(&chapter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if chapter.Index != requestedChapter.Index || chapter.URL != requestedChapter.URL || chapter.Title != requestedChapter.Title {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}
	return book, chapter, nil
}

func (s *Server) normalizeChapterCachePath(ctx context.Context, chapter *models.Chapter, normalizedPath string) {
	previousPath := chapter.CachePath
	if normalizedPath == "" || normalizedPath == previousPath {
		return
	}
	result := s.db.WithContext(ctx).
		Model(&models.Chapter{}).
		Where("id = ? AND book_id = ? AND COALESCE(cache_path, '') = ?", chapter.ID, chapter.BookID, previousPath).
		UpdateColumn("cache_path", normalizedPath)
	if result.Error == nil && result.RowsAffected == 1 {
		chapter.CachePath = normalizedPath
	}
}

func (s *Server) persistRemoteChapterFetch(
	ctx context.Context,
	snapshot readerChapterContentSnapshot,
	fetched string,
	variableState engine.SourceRuleVariableState,
) (models.Book, models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return models.Book{}, models.Chapter{}, err
	}
	s.remoteCacheMu.Lock()
	defer s.remoteCacheMu.Unlock()

	if _, _, err := s.validateReaderChapterContentSnapshot(s.db.WithContext(ctx), snapshot); err != nil {
		return models.Book{}, models.Chapter{}, err
	}

	var staged *stagedChapterCache
	var err error
	if fetched != "" {
		staged, err = s.stageRemoteChapterCache(ctx, snapshot.book.URL, snapshot.chapter.URL, fetched)
		if err != nil {
			return models.Book{}, models.Chapter{}, err
		}
		defer staged.rollback()
	}

	var currentBook models.Book
	var currentChapter models.Chapter
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bookResult := tx.Model(&models.Book{}).
			Where(
				"id = ? AND user_id = ? AND source_id = ? AND type = ? AND url = ? AND title = ? AND COALESCE(variable, '') = ?",
				snapshot.book.ID,
				snapshot.book.UserID,
				snapshot.book.SourceID,
				snapshot.book.Type,
				snapshot.book.URL,
				snapshot.book.Title,
				snapshot.book.Variable,
			).
			UpdateColumn("variable", variableState.BookVariable)
		if bookResult.Error != nil {
			return bookResult.Error
		}
		if bookResult.RowsAffected != 1 {
			return errReaderChapterContentStale
		}
		committedSnapshot := snapshot
		committedSnapshot.book.Variable = variableState.BookVariable
		var validateErr error
		currentBook, currentChapter, validateErr = s.validateReaderChapterContentSnapshot(tx, committedSnapshot)
		if validateErr != nil {
			return validateErr
		}

		cachePath := currentChapter.CachePath
		if staged != nil {
			cachePath = staged.relative
		}
		chapterResult := tx.Model(&models.Chapter{}).
			Where(
				"id = ? AND book_id = ? AND `index` = ? AND url = ? AND title = ? AND COALESCE(variable, '') = ? AND COALESCE(cache_path, '') = ?",
				snapshot.chapter.ID,
				snapshot.chapter.BookID,
				snapshot.chapter.Index,
				snapshot.chapter.URL,
				snapshot.chapter.Title,
				snapshot.chapter.Variable,
				snapshot.chapter.CachePath,
			).
			UpdateColumns(map[string]any{
				"variable":   variableState.ChapterVariable,
				"cache_path": cachePath,
			})
		if chapterResult.Error != nil {
			return chapterResult.Error
		}
		if chapterResult.RowsAffected != 1 {
			return errReaderChapterContentStale
		}
		if staged != nil {
			if err := staged.publish(ctx); err != nil {
				return err
			}
		}
		currentBook.Variable = variableState.BookVariable
		currentChapter.Variable = variableState.ChapterVariable
		currentChapter.CachePath = cachePath
		return nil
	})
	if err != nil {
		return models.Book{}, models.Chapter{}, err
	}
	if staged != nil {
		staged.finalize()
	}
	return currentBook, currentChapter, nil
}

func (s *Server) validateReaderChapterContentSnapshot(
	db *gorm.DB,
	snapshot readerChapterContentSnapshot,
) (models.Book, models.Chapter, error) {
	var association models.UserBookSource
	if err := db.Where(
		"user_id = ? AND source_id = ? AND detached = ?",
		snapshot.book.UserID,
		snapshot.source.ID,
		false,
	).First(&association).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	var source models.BookSource
	if err := db.First(&source, snapshot.source.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if !sameBookSourceFetchSemantics(source, snapshot.source) {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}

	var book models.Book
	if err := db.Where("id = ? AND user_id = ?", snapshot.book.ID, snapshot.book.UserID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if book.SourceID != snapshot.book.SourceID || book.Type != snapshot.book.Type || book.URL != snapshot.book.URL ||
		book.Title != snapshot.book.Title || book.Variable != snapshot.book.Variable {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}

	var chapter models.Chapter
	if err := db.Where("id = ? AND book_id = ?", snapshot.chapter.ID, snapshot.book.ID).First(&chapter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if chapter.Index != snapshot.chapter.Index || chapter.URL != snapshot.chapter.URL ||
		chapter.Title != snapshot.chapter.Title || chapter.Variable != snapshot.chapter.Variable ||
		chapter.CachePath != snapshot.chapter.CachePath {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}
	return book, chapter, nil
}

func sameBookSourceFetchSemantics(current models.BookSource, snapshot models.BookSource) bool {
	return current.ID == snapshot.ID &&
		current.BaseURL == snapshot.BaseURL &&
		current.SearchURL == snapshot.SearchURL &&
		current.BookURLPattern == snapshot.BookURLPattern &&
		current.SourceType == snapshot.SourceType &&
		current.Charset == snapshot.Charset &&
		current.Header == snapshot.Header &&
		current.LoginURL == snapshot.LoginURL &&
		current.LoginCheckJS == snapshot.LoginCheckJS &&
		current.Rules == snapshot.Rules &&
		current.Enabled == snapshot.Enabled
}

func (s *Server) requireContentSearchSource(book models.Book) error {
	if book.SourceID == 0 {
		return nil
	}
	_, err := s.bookSources.FindForBook(book.UserID, book.SourceID)
	return err
}

func (s *Server) localChapterCachePath(book models.Book, fullPath string) string {
	fullPath = strings.TrimSpace(fullPath)
	if fullPath == "" {
		return ""
	}
	if !filepath.IsAbs(fullPath) {
		return fullPath
	}
	if book.SourceID == 0 {
		if archiveRoot, ok := s.localBookArchiveRoot(book); ok {
			if rel, ok := relativePathInside(archiveRoot, fullPath); ok {
				return rel
			}
		}
		return ""
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

type readerLocalChapterCacheSnapshot struct {
	book    models.Book
	chapter models.Chapter
}

func (s *Server) rebuildLocalChapterTextContext(
	ctx context.Context,
	book models.Book,
	chapter models.Chapter,
) (string, models.Book, models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return "", models.Book{}, models.Chapter{}, err
	}
	source, ok := s.openLocalBookSource(book)
	if !ok {
		return "", models.Book{}, models.Chapter{}, nil
	}
	defer source.close()
	snapshot := readerLocalChapterCacheSnapshot{book: book, chapter: chapter}
	content := ""
	if epubreader.IsLocalEPUB(book) {
		workingChapter := chapter
		var err error
		content, err = s.epubReader.ReadChapterTextContext(ctx, book, &workingChapter)
		if err != nil {
			if isRequestContextError(err) {
				return "", models.Book{}, models.Chapter{}, err
			}
			return "", models.Book{}, models.Chapter{}, nil
		}
	} else {
		legacyLimits := engine.LegacyLocalBookParseLimits()
		data, err := readBoundedOpenedLocalBookSourceContext(ctx, source.file, source.info, legacyLimits.MaxArchiveBytes)
		if err != nil {
			if isRequestContextError(err) {
				return "", models.Book{}, models.Chapter{}, err
			}
			return "", models.Book{}, models.Chapter{}, nil
		}
		chapters, err := parseLocalBookChapters(filepath.Ext(source.name), data, book.TOCRule)
		if err != nil || chapter.Index < 0 || chapter.Index >= len(chapters) {
			return "", models.Book{}, models.Chapter{}, nil
		}
		content = strings.TrimSpace(chapters[chapter.Index].Content)
	}
	if content == "" {
		return "", models.Book{}, models.Chapter{}, nil
	}
	if readerLocalChapterCacheRebuildLifecycleTestHook != nil {
		readerLocalChapterCacheRebuildLifecycleTestHook("after_local_rebuild", book, chapter)
	}
	return s.persistRebuiltLocalChapterTextContext(ctx, snapshot, source, content)
}

// readerLocalChapterCacheRebuildLifecycleTestHook exposes the post-parse
// persistence boundary for deterministic lifecycle contract tests.
var readerLocalChapterCacheRebuildLifecycleTestHook func(string, models.Book, models.Chapter)

func (s *Server) persistRebuiltLocalChapterTextContext(
	ctx context.Context,
	snapshot readerLocalChapterCacheSnapshot,
	source *openedLocalBookSource,
	content string,
) (string, models.Book, models.Chapter, error) {
	if err := ctx.Err(); err != nil {
		return "", models.Book{}, models.Chapter{}, err
	}
	s.localCacheMu.Lock()
	defer s.localCacheMu.Unlock()
	if !source.current() {
		return "", models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}
	currentBook, currentChapter, err := s.validateReaderLocalChapterCacheSnapshot(s.db.WithContext(ctx), snapshot)
	if err != nil {
		return "", models.Book{}, models.Chapter{}, err
	}
	chapterURL := strings.TrimSpace(snapshot.chapter.URL)
	if chapterURL == "" {
		chapterURL = fmt.Sprintf("local://book_%d/chapter_%d", snapshot.book.ID, snapshot.chapter.Index)
	}
	bookURL := strings.TrimSpace(snapshot.book.URL)
	if bookURL == "" {
		bookURL = fmt.Sprintf("local://book_%d", snapshot.book.ID)
	}
	staged, err := s.stageLocalChapterCache(ctx, source.archive, bookURL, chapterURL, content)
	if err != nil {
		return "", models.Book{}, models.Chapter{}, err
	}
	defer staged.rollback()
	if readerLocalChapterCacheRebuildLifecycleTestHook != nil {
		readerLocalChapterCacheRebuildLifecycleTestHook("after_local_cache_stage", snapshot.book, snapshot.chapter)
	}
	if err := ctx.Err(); err != nil {
		return "", models.Book{}, models.Chapter{}, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var validateErr error
		currentBook, currentChapter, validateErr = s.validateReaderLocalChapterCacheSnapshot(tx, snapshot)
		if validateErr != nil {
			return validateErr
		}
		if !source.current() {
			return errReaderChapterContentStale
		}
		if readerLocalChapterCacheRebuildLifecycleTestHook != nil {
			readerLocalChapterCacheRebuildLifecycleTestHook("before_local_cache_update", snapshot.book, snapshot.chapter)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		write := tx.Model(&models.Chapter{}).
			Where(
				"id = ? AND book_id = ? AND `index` = ? AND title = ? AND url = ? AND is_volume = ? AND tag = ? AND COALESCE(cache_path, '') = ? AND COALESCE(resource_path, '') = ? AND COALESCE(resource_fragment, '') = ? AND COALESCE(resource_end_fragment, '') = ? AND COALESCE(variable, '') = ?",
				snapshot.chapter.ID,
				snapshot.chapter.BookID,
				snapshot.chapter.Index,
				snapshot.chapter.Title,
				snapshot.chapter.URL,
				snapshot.chapter.IsVolume,
				snapshot.chapter.Tag,
				snapshot.chapter.CachePath,
				snapshot.chapter.ResourcePath,
				snapshot.chapter.ResourceFragment,
				snapshot.chapter.ResourceEndFragment,
				snapshot.chapter.Variable,
			).
			Where(
				"EXISTS (SELECT 1 FROM books WHERE books.id = chapters.book_id AND books.id = ? AND books.user_id = ? AND books.source_id = ? AND books.type = ? AND books.url = ? AND books.library_path = ? AND books.original_file = ? AND books.toc_file = ? AND books.source_file = ? AND books.toc_rule = ?)",
				snapshot.book.ID,
				snapshot.book.UserID,
				snapshot.book.SourceID,
				snapshot.book.Type,
				snapshot.book.URL,
				snapshot.book.LibraryPath,
				snapshot.book.OriginalFile,
				snapshot.book.TOCFile,
				snapshot.book.SourceFile,
				snapshot.book.TOCRule,
			).
			UpdateColumn("cache_path", staged.relative)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return errReaderChapterContentStale
		}
		if !source.current() {
			return errReaderChapterContentStale
		}
		if readerLocalChapterCacheRebuildLifecycleTestHook != nil {
			readerLocalChapterCacheRebuildLifecycleTestHook("before_local_cache_publish", snapshot.book, snapshot.chapter)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := staged.publish(ctx); err != nil {
			return err
		}
		currentChapter.CachePath = staged.relative
		return nil
	})
	if err != nil {
		return "", models.Book{}, models.Chapter{}, err
	}
	staged.finalize()
	return content, currentBook, currentChapter, nil
}

func (s *Server) validateReaderLocalChapterCacheSnapshot(
	db *gorm.DB,
	snapshot readerLocalChapterCacheSnapshot,
) (models.Book, models.Chapter, error) {
	var book models.Book
	if err := db.Where("id = ? AND user_id = ?", snapshot.book.ID, snapshot.book.UserID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if !sameReaderLocalChapterCacheBookSnapshot(book, snapshot.book) {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}
	var chapter models.Chapter
	if err := db.Where("id = ? AND book_id = ?", snapshot.chapter.ID, snapshot.book.ID).First(&chapter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Book{}, models.Chapter{}, errReaderChapterContentStale
		}
		return models.Book{}, models.Chapter{}, err
	}
	if !sameReaderLocalChapterCacheChapterSnapshot(chapter, snapshot.chapter) {
		return models.Book{}, models.Chapter{}, errReaderChapterContentStale
	}
	return book, chapter, nil
}

func sameReaderLocalChapterCacheBookSnapshot(current, snapshot models.Book) bool {
	return current.ID == snapshot.ID &&
		current.UserID == snapshot.UserID &&
		current.SourceID == 0 && snapshot.SourceID == 0 &&
		current.Type == snapshot.Type &&
		current.URL == snapshot.URL &&
		current.LibraryPath == snapshot.LibraryPath &&
		current.OriginalFile == snapshot.OriginalFile &&
		current.TOCFile == snapshot.TOCFile &&
		current.SourceFile == snapshot.SourceFile &&
		current.TOCRule == snapshot.TOCRule
}

func sameReaderLocalChapterCacheChapterSnapshot(current, snapshot models.Chapter) bool {
	return current.ID == snapshot.ID &&
		current.BookID == snapshot.BookID &&
		current.Index == snapshot.Index &&
		current.Title == snapshot.Title &&
		current.URL == snapshot.URL &&
		current.IsVolume == snapshot.IsVolume &&
		current.Tag == snapshot.Tag &&
		current.CachePath == snapshot.CachePath &&
		current.ResourcePath == snapshot.ResourcePath &&
		current.ResourceFragment == snapshot.ResourceFragment &&
		current.ResourceEndFragment == snapshot.ResourceEndFragment &&
		current.Variable == snapshot.Variable
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
	source, ok := s.openLocalBookSource(book)
	if !ok {
		return "", false
	}
	defer source.close()
	return source.file.Name(), true
}

// localBookArchiveRoot is the sole authority for local-book filesystem access.
// Old SQLite rows are persisted input: a historical absolute OriginalFile or
// CachePath can describe a former Docker host, but must never authorize a read
// outside the current owner's private library root.
func (s *Server) localBookArchiveRoot(book models.Book) (string, bool) {
	archive, ok := s.resolveLocalBookArchive(book)
	if !ok {
		return "", false
	}
	return archive.root, true
}

func pathInside(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	if book.SourceID != 0 {
		return s.readRemoteChapterCache(cachePath)
	}
	file, _, ok := s.openLocalChapterCache(book, cachePath)
	if !ok {
		return nil, "", os.ErrNotExist
	}
	defer file.Close()
	bytes, err := readOpenedFile(file)
	if err != nil {
		return nil, "", err
	}
	return bytes, file.Name(), nil
}

func (s *Server) chapterCacheCandidates(book models.Book, cachePath string) []string {
	if book.SourceID != 0 {
		if filepath.IsAbs(cachePath) {
			return []string{cachePath}
		}
		return []string{filepath.Join(s.cfg.CacheDir, cachePath)}
	}
	file, _, ok := s.openLocalChapterCache(book, cachePath)
	if !ok {
		return nil
	}
	path := file.Name()
	_ = file.Close()
	return []string{path}
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
		next, err := applyReaderReplaceRule(content, rule.Pattern, rule.Replacement, isRegex)
		if err != nil {
			break
		}
		content = next
	}
	return content
}

func replaceRuleAppliesToBook(scope string, book models.Book) bool {
	if scope == "" || scope == "*" {
		return true
	}
	parts := strings.Split(scope, ";")
	if parts[0] != "*" && parts[0] != book.Title {
		return false
	}
	if len(parts) < 2 {
		return true
	}
	return parts[1] == book.URL
}

func (s *Server) checkUpdates(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	result, err := s.scheduler.CheckNowForUserDetailed(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查书籍更新失败"})
		return
	}
	items := make([]bookListItem, 0, len(result.UpdatedBookIDs))
	updatedBooks := make(map[uint]models.Book, len(result.UpdatedBookIDs))
	if len(result.UpdatedBookIDs) > 0 {
		var books []models.Book
		if err := s.db.Where("user_id = ? AND id IN ?", userID, result.UpdatedBookIDs).Find(&books).Error; err != nil || len(books) != len(result.UpdatedBookIDs) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "检查书籍更新失败"})
			return
		}
		for _, book := range books {
			updatedBooks[book.ID] = book
		}
		for _, bookID := range result.UpdatedBookIDs {
			book, exists := updatedBooks[bookID]
			if !exists {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "检查书籍更新失败"})
				return
			}
			items = append(items, s.bookShelfListItem(userID, book))
		}
	}
	s.pruneUnreferencedRemoteCachePaths(result.SupersededCachePaths)
	for _, bookID := range result.ReplacedBookIDs {
		if book, exists := updatedBooks[bookID]; exists {
			_, _ = s.chapterImages.RemoveBook(book)
		}
	}
	if len(items) > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
	}
	c.JSON(http.StatusOK, gin.H{
		"checked":         result.Checked,
		"updated":         result.Updated,
		"failed":          result.Failed,
		"newChapters":     result.NewChapters,
		"replacedBookIds": result.ReplacedBookIDs,
		"books":           items,
	})
}
