package localbook

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/cbzreader"
	"openreader/backend/services/epubreader"
)

var (
	ErrUnsupportedFormat      = errors.New("unsupported local book format")
	ErrParseFailed            = errors.New("failed to parse local book")
	ErrPreparedImportMismatch = errors.New("prepared local book does not match staged input")
)

// Version 2 adds the effective TOC rule, intro and kind to the snapshot.
const PreparedImportVersion = 2
const maxLocalBookTOCRuleBytes = engine.MaxTXTTocRuleBytes

type Importer struct {
	cfg config.Config
	db  *gorm.DB
	// Nil outside deterministic work-boundary tests. Observe actual completed
	// I/O, never substitute parser results, contexts or persistence operations.
	testAfterWork func(phase, archiveRoot string)
}

func (importer Importer) observeWork(phase, archiveRoot string) {
	if importer.testAfterWork != nil {
		importer.testAfterWork(phase, archiveRoot)
	}
}

type ImportRequest struct {
	UserID      uint
	UserName    string
	FileName    string
	Extension   string
	Data        []byte
	Title       string
	Author      string
	CategoryID  *uint
	CategoryIDs []uint
	// BeforeCommit retains an upstream stage admission through final durability.
	// It is an internal callback, never decoded from an HTTP/backup payload.
	BeforeCommit func() error
	TOCRule      string
	// DirectUpload marks a browser upload, whose file name reader-dev
	// normalizes before deriving the book name and author.
	DirectUpload bool
}

type PreviewChapter struct {
	Index int    `json:"index"`
	Title string `json:"title"`
}

type PreviewResult struct {
	Title        string           `json:"title"`
	Author       string           `json:"author"`
	ChapterCount int              `json:"chapterCount"`
	Chapters     []PreviewChapter `json:"chapters"`
	TOCRule      string           `json:"tocRule,omitempty"`
	CoverURL     string           `json:"coverUrl,omitempty"`
	ImportToken  string           `json:"importToken,omitempty"`
}

// PreparedImport is a versioned, data-only snapshot of one successful local
// parser run. API staging persists it as caller-scoped derived cache so
// confirmation can materialize the exact catalogue the user reviewed without
// parsing the whole source a second time.
type PreparedImport struct {
	Version         int               `json:"version"`
	Extension       string            `json:"extension"`
	TOCRule         string            `json:"tocRule"`
	SourceSHA256    string            `json:"sourceSha256"`
	EPUBCatalogOnly bool              `json:"epubCatalogOnly,omitempty"`
	Book            engine.ParsedBook `json:"book"`
}

func NewImporter(cfg config.Config, db *gorm.DB) Importer {
	return Importer{cfg: cfg, db: db}
}

func (importer Importer) Preview(request ImportRequest) (PreviewResult, error) {
	preview, _, err := importer.Prepare(request)
	return preview, err
}

func (importer Importer) Prepare(request ImportRequest) (PreviewResult, PreparedImport, error) {
	if len(strings.TrimSpace(request.TOCRule)) > maxLocalBookTOCRuleBytes {
		return PreviewResult{}, PreparedImport{}, fmt.Errorf("%w: local book TOC rule exceeds the limit", ErrParseFailed)
	}
	var parsedBook engine.ParsedBook
	var err error
	if normalizedLocalBookExtension(request.Extension) == ".epub" {
		parsedBook, err = engine.ParseEPUBCatalogWithLimits(request.Data, request.TOCRule, importer.parseLimits())
	} else {
		parsedBook, err = parseUploadedBookWithLimits(request.Extension, request.Data, request.TOCRule, importer.parseLimits())
	}
	if err != nil {
		if errors.Is(err, ErrUnsupportedFormat) {
			return PreviewResult{}, PreparedImport{}, err
		}
		return PreviewResult{}, PreparedImport{}, fmt.Errorf("%w: %w", ErrParseFailed, err)
	}
	prepared := NewPreparedImport(request, parsedBook)
	prepared.EPUBCatalogOnly = normalizedLocalBookExtension(request.Extension) == ".epub"
	return previewResult(request, parsedBook), prepared, nil
}

func NewPreparedImport(request ImportRequest, parsedBook engine.ParsedBook) PreparedImport {
	sourceHash := sha256.Sum256(request.Data)
	return PreparedImport{
		Version:      PreparedImportVersion,
		Extension:    normalizedLocalBookExtension(request.Extension),
		TOCRule:      strings.TrimSpace(request.TOCRule),
		SourceSHA256: hex.EncodeToString(sourceHash[:]),
		Book:         parsedBook,
	}
}

func (prepared PreparedImport) Matches(request ImportRequest) bool {
	if prepared.Version != PreparedImportVersion ||
		prepared.Extension != normalizedLocalBookExtension(request.Extension) ||
		prepared.TOCRule != strings.TrimSpace(request.TOCRule) {
		return false
	}
	sourceHash := sha256.Sum256(request.Data)
	return prepared.SourceSHA256 == hex.EncodeToString(sourceHash[:])
}

// localBookTitleAuthor applies reader-dev's precedence: the name/author typed
// in the import dialog, then (when reattaching) the stored book, then format
// metadata (EPUB/UMD/CBZ, Book.updateFromLocal), then the file name split by
// LocalBook.analyzeNameAuthor (Book.initLocalBook).
func localBookTitleAuthor(request ImportRequest, parsedBook engine.ParsedBook, existing *models.Book) (string, string) {
	fileTitle, fileAuthor := engine.LocalBookNameAuthor(request.FileName, request.DirectUpload)
	existingTitle, existingAuthor := "", ""
	if existing != nil {
		existingTitle, existingAuthor = existing.Title, existing.Author
	}
	title := firstNonBlank(request.Title, existingTitle, parsedBook.Title, fileTitle)
	if title == "" {
		title = strings.TrimSuffix(request.FileName, filepath.Ext(request.FileName))
	}
	return title, firstNonBlank(request.Author, existingAuthor, parsedBook.Author, fileAuthor)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func previewResult(request ImportRequest, parsedBook engine.ParsedBook) PreviewResult {
	title, author := localBookTitleAuthor(request, parsedBook, nil)

	chapters := make([]PreviewChapter, 0, len(parsedBook.Chapters))
	for index, chapter := range parsedBook.Chapters {
		chapterTitle := strings.TrimSpace(chapter.Title)
		if chapterTitle == "" {
			chapterTitle = fmt.Sprintf("第 %d 章", index+1)
		}
		chapters = append(chapters, PreviewChapter{Index: index, Title: chapterTitle})
	}
	return PreviewResult{
		Title:        title,
		Author:       author,
		ChapterCount: len(chapters),
		Chapters:     chapters,
		TOCRule:      parsedBook.TOCRule,
	}
}

func (importer Importer) Import(request ImportRequest) (models.Book, error) {
	result, err := importer.ImportResult(request)
	return result.Book, err
}

func (importer Importer) ImportPrepared(request ImportRequest, prepared PreparedImport) (models.Book, error) {
	result, err := importer.ImportPreparedResult(request, prepared)
	return result.Book, err
}

type epubPreparationParseError struct {
	cause error
}

func (err epubPreparationParseError) Error() string {
	return ErrParseFailed.Error() + ": EPUB archive cannot be prepared safely"
}

func (err epubPreparationParseError) Unwrap() []error {
	return []error{ErrParseFailed, err.cause}
}

func classifyEPUBPreparationError(err error) error {
	if errors.Is(err, epubreader.ErrInvalidArchive) ||
		errors.Is(err, epubreader.ErrExtractionLimit) ||
		errors.Is(err, epubreader.ErrUnsafePath) ||
		errors.Is(err, epubreader.ErrUnsupportedMedia) {
		return epubPreparationParseError{cause: err}
	}
	return err
}

type cbzPreparationParseError struct {
	cause error
}

func (err cbzPreparationParseError) Error() string {
	return ErrParseFailed.Error() + ": CBZ archive cannot be prepared safely"
}

func (err cbzPreparationParseError) Unwrap() []error {
	return []error{ErrParseFailed, err.cause}
}

func classifyCBZPreparationError(err error) error {
	if errors.Is(err, cbzreader.ErrInvalidArchive) ||
		errors.Is(err, cbzreader.ErrExtractionLimit) ||
		errors.Is(err, cbzreader.ErrUnsafePath) ||
		errors.Is(err, cbzreader.ErrUnsupportedMedia) ||
		errors.Is(err, cbzreader.ErrNotFound) {
		return cbzPreparationParseError{cause: err}
	}
	return err
}

func normalizedLocalBookExtension(extension string) string {
	return strings.ToLower(strings.TrimSpace(extension))
}

// RestoreExisting rehydrates the caller-owned shelf row created by logical restore. Portable restore
// calls it explicitly; normal import uses the same path only for one unambiguous reader-dev local-book
// placeholder so progress and bookmarks do not drift to a newly allocated local identifier.
func (importer Importer) RestoreExisting(existing models.Book, request ImportRequest) (models.Book, error) {
	if err := importer.context().Err(); err != nil {
		return models.Book{}, importCancellation(err)
	}
	if existing.ID == 0 || existing.UserID == 0 || existing.UserID != request.UserID || existing.SourceID != 0 {
		return models.Book{}, errors.New("invalid portable local book target")
	}
	if strings.TrimSpace(existing.URL) == "" {
		return models.Book{}, errors.New("portable local book target has no URL")
	}
	parsedBook, err := parseUploadedBookWithLimits(request.Extension, request.Data, request.TOCRule, importer.parseLimits())
	if err != nil {
		if errors.Is(err, ErrUnsupportedFormat) {
			return models.Book{}, err
		}
		return models.Book{}, fmt.Errorf("%w: %w", ErrParseFailed, err)
	}
	title, author := localBookTitleAuthor(request, parsedBook, &existing)
	return importer.restoreExistingParsedBook(existing, request, parsedBook, title, author)
}

func (importer Importer) readerDevLocalPlaceholder(userID uint, title, author string) (models.Book, bool, error) {
	if userID == 0 || strings.TrimSpace(title) == "" {
		return models.Book{}, false, nil
	}
	var candidates []models.Book
	if err := importer.db.
		Where(
			"user_id = ? AND source_id = 0 AND title = ? AND author = ? AND COALESCE(library_path, '') = '' AND COALESCE(original_file, '') = ''",
			userID,
			title,
			author,
		).
		Limit(2).
		Find(&candidates).Error; err != nil {
		return models.Book{}, false, err
	}
	if len(candidates) != 1 {
		return models.Book{}, false, nil
	}
	var chapterCount int64
	if err := importer.db.Model(&models.Chapter{}).Where("book_id = ?", candidates[0].ID).Count(&chapterCount).Error; err != nil {
		return models.Book{}, false, err
	}
	return candidates[0], chapterCount == 0, nil
}

func (importer Importer) restoreExistingParsedBook(
	existing models.Book, request ImportRequest, parsedBook engine.ParsedBook, title, author string,
) (models.Book, error) {
	result, err := importer.persistParsedBook(&existing, request, parsedBook, title, author)
	return result.Book, err
}

func parseUploadedBook(ext string, data []byte, tocRule string) (engine.ParsedBook, error) {
	return parseUploadedBookWithLimits(ext, data, tocRule, engine.DefaultLocalBookParseLimits())
}

func (importer Importer) parseLimits() engine.LocalBookParseLimits {
	limits := engine.DefaultLocalBookParseLimits()
	if importer.cfg.MaxImportBytes > 0 {
		limits.MaxArchiveBytes = importer.cfg.MaxImportBytes
	}
	if importer.cfg.MaxArchiveEntries > 0 {
		limits.MaxArchiveEntries = importer.cfg.MaxArchiveEntries
	}
	if importer.cfg.MaxArchiveEntryBytes > 0 {
		limits.MaxArchiveEntryBytes = importer.cfg.MaxArchiveEntryBytes
	}
	if importer.cfg.MaxArchiveExpandedBytes > 0 {
		limits.MaxArchiveExpandedBytes = importer.cfg.MaxArchiveExpandedBytes
	}
	if importer.cfg.MaxPDFPages > 0 {
		limits.MaxPDFPages = importer.cfg.MaxPDFPages
	}
	if importer.cfg.MaxParsedTextBytes > 0 {
		limits.MaxParsedTextBytes = importer.cfg.MaxParsedTextBytes
	}
	limits.MaxParsedChapters = importer.cfg.ParsedChapterLimit()
	if importer.cfg.MaxUMDChapters > 0 {
		limits.MaxUMDChapters = importer.cfg.MaxUMDChapters
	}
	return limits
}

func parseUploadedBookWithLimits(ext string, data []byte, tocRule string, limits engine.LocalBookParseLimits) (engine.ParsedBook, error) {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".cbz":
		return engine.ParseCBZWithLimits(data, limits)
	case ".epub":
		return engine.ParseEPUBWithLimits(data, tocRule, limits)
	case ".txt", ".text", ".md":
		return engine.ParseTXTBookWithLimits(data, tocRule, limits)
	case ".pdf":
		return engine.ParsePDFWithLimits(data, limits)
	case ".umd":
		return engine.ParseUMDWithLimits(data, limits)
	default:
		return engine.ParsedBook{}, ErrUnsupportedFormat
	}
}
