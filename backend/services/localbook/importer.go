package localbook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported local book format")
	ErrParseFailed       = errors.New("failed to parse local book")
)

type Importer struct {
	cfg config.Config
	db  *gorm.DB
}

type ImportRequest struct {
	UserID     uint
	UserName   string
	FileName   string
	Extension  string
	Data       []byte
	Title      string
	Author     string
	CategoryID *uint
	TOCRule    string
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
	ImportToken  string           `json:"importToken,omitempty"`
}

func NewImporter(cfg config.Config, db *gorm.DB) Importer {
	return Importer{cfg: cfg, db: db}
}

func (importer Importer) Preview(request ImportRequest) (PreviewResult, error) {
	parsedBook, err := parseUploadedBookWithLimits(request.Extension, request.Data, request.TOCRule, importer.parseLimits())
	if err != nil {
		if errors.Is(err, ErrUnsupportedFormat) {
			return PreviewResult{}, err
		}
		return PreviewResult{}, fmt.Errorf("%w: %w", ErrParseFailed, err)
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = strings.TrimSpace(parsedBook.Title)
	}
	if title == "" {
		title = strings.TrimSuffix(request.FileName, filepath.Ext(request.FileName))
	}
	author := strings.TrimSpace(request.Author)
	if author == "" {
		author = strings.TrimSpace(parsedBook.Author)
	}

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
	}, nil
}

func (importer Importer) Import(request ImportRequest) (models.Book, error) {
	parsedBook, err := parseUploadedBookWithLimits(request.Extension, request.Data, request.TOCRule, importer.parseLimits())
	if err != nil {
		if errors.Is(err, ErrUnsupportedFormat) {
			return models.Book{}, err
		}
		return models.Book{}, fmt.Errorf("%w: %w", ErrParseFailed, err)
	}
	chapters := parsedBook.Chapters

	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = parsedBook.Title
	}
	if title == "" {
		title = strings.TrimSuffix(request.FileName, filepath.Ext(request.FileName))
	}

	author := strings.TrimSpace(request.Author)
	if author == "" {
		author = parsedBook.Author
	}

	archive, err := engine.ArchiveImportedBook(importer.cfg.LibraryDir, request.UserName, title, author, request.FileName, request.Data)
	if err != nil {
		return models.Book{}, err
	}

	var book models.Book
	err = importer.db.Transaction(func(tx *gorm.DB) error {
		lastChapter := ""
		if len(chapters) > 0 {
			lastChapter = chapters[len(chapters)-1].Title
		}
		book = models.Book{
			UserID:       request.UserID,
			SourceID:     0,
			CategoryID:   request.CategoryID,
			Title:        title,
			Author:       author,
			URL:          fmt.Sprintf("local://pending/%d", request.UserID),
			LibraryPath:  archive.Directory,
			OriginalFile: archive.OriginalFile,
			TOCFile:      archive.TOCFile,
			TOCRule:      strings.TrimSpace(request.TOCRule),
			SourceFile:   archive.SourceFile,
			LastChapter:  lastChapter,
			ChapterCount: len(chapters),
		}
		if err := tx.Create(&book).Error; err != nil {
			return err
		}

		book.URL = fmt.Sprintf("local://book_%d", book.ID)
		if err := tx.Save(&book).Error; err != nil {
			return err
		}

		archivedChapters := make([]engine.ArchivedChapter, 0, len(chapters))
		for index, parsedChapter := range chapters {
			chapterTitle := strings.TrimSpace(parsedChapter.Title)
			if chapterTitle == "" {
				chapterTitle = fmt.Sprintf("第 %d 章", index+1)
			}
			chapterURL := fmt.Sprintf("%s/chapter_%d", book.URL, index)
			contentDir := filepath.Join(importer.cfg.LibraryDir, archive.Directory, "content")
			contentPath, err := engine.WriteChapterCache(contentDir, book.URL, chapterURL, parsedChapter.Content)
			if err != nil {
				return err
			}
			cachePath := filepath.Join("content", contentPath)

			chapter := models.Chapter{
				BookID:              book.ID,
				Index:               index,
				Title:               chapterTitle,
				URL:                 chapterURL,
				CachePath:           cachePath,
				ResourcePath:        parsedChapter.ResourcePath,
				ResourceFragment:    parsedChapter.ResourceFragment,
				ResourceEndFragment: parsedChapter.ResourceEndFragment,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			archivedChapters = append(archivedChapters, engine.ArchivedChapter{
				ID:                  chapter.ID,
				URL:                 chapterURL,
				Title:               chapterTitle,
				IsVolume:            false,
				BaseURL:             "",
				BookURL:             archive.OriginalFile,
				Index:               index,
				Start:               parsedChapter.Start,
				End:                 parsedChapter.End,
				CachePath:           cachePath,
				ResourcePath:        parsedChapter.ResourcePath,
				ResourceFragment:    parsedChapter.ResourceFragment,
				ResourceEndFragment: parsedChapter.ResourceEndFragment,
			})
		}

		source := engine.ArchivedBookSource{
			BookURL:            archive.OriginalFile,
			Origin:             "loc_book",
			OriginName:         archive.OriginalFile,
			Type:               0,
			Name:               title,
			Author:             author,
			LatestChapterTitle: book.LastChapter,
			TOCURL:             archive.TOCFile,
			Time:               0,
			OriginOrder:        0,
			UserNameSpace:      request.UserName,
		}
		if err := engine.WriteBookSource(importer.cfg.LibraryDir, archive, source); err != nil {
			return err
		}
		if err := engine.WriteChapterArchive(importer.cfg.LibraryDir, archive, archivedChapters); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return models.Book{}, err
	}
	return book, nil
}

// RestoreExisting rehydrates a local-book shelf row from a portable backup archive. The logical
// backup restore has already recreated the caller-owned book, progress, bookmarks and categories;
// keeping that row and URL is what prevents those records from drifting to a newly allocated local
// identifier. This is deliberately separate from Import, whose normal user-facing behavior creates
// a new book and therefore a new local URL.
func (importer Importer) RestoreExisting(existing models.Book, request ImportRequest) (models.Book, error) {
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
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = strings.TrimSpace(existing.Title)
	}
	if title == "" {
		title = strings.TrimSpace(parsedBook.Title)
	}
	if title == "" {
		title = strings.TrimSuffix(request.FileName, filepath.Ext(request.FileName))
	}
	author := strings.TrimSpace(request.Author)
	if author == "" {
		author = strings.TrimSpace(existing.Author)
	}
	if author == "" {
		author = strings.TrimSpace(parsedBook.Author)
	}

	archive, err := engine.ArchiveImportedBook(importer.cfg.LibraryDir, request.UserName, title, author, request.FileName, request.Data)
	if err != nil {
		return models.Book{}, err
	}
	archiveRoot := filepath.Join(importer.cfg.LibraryDir, archive.Directory)
	cleanupArchive := true
	defer func() {
		if cleanupArchive {
			_ = os.RemoveAll(archiveRoot)
		}
	}()

	book := existing
	err = importer.db.Transaction(func(tx *gorm.DB) error {
		var target models.Book
		if err := tx.Where("id = ? AND user_id = ? AND source_id = ?", existing.ID, request.UserID, 0).First(&target).Error; err != nil {
			return err
		}
		if err := tx.Where("book_id = ?", target.ID).Delete(&models.Chapter{}).Error; err != nil {
			return err
		}
		lastChapter := ""
		if len(parsedBook.Chapters) > 0 {
			lastChapter = parsedBook.Chapters[len(parsedBook.Chapters)-1].Title
		}
		target.Type = 0
		target.Title = title
		target.Author = author
		target.LibraryPath = archive.Directory
		target.OriginalFile = archive.OriginalFile
		target.TOCFile = archive.TOCFile
		target.TOCRule = strings.TrimSpace(request.TOCRule)
		target.SourceFile = archive.SourceFile
		target.LastChapter = lastChapter
		target.ChapterCount = len(parsedBook.Chapters)
		if err := tx.Save(&target).Error; err != nil {
			return err
		}

		archivedChapters := make([]engine.ArchivedChapter, 0, len(parsedBook.Chapters))
		for index, parsedChapter := range parsedBook.Chapters {
			chapterTitle := strings.TrimSpace(parsedChapter.Title)
			if chapterTitle == "" {
				chapterTitle = fmt.Sprintf("第 %d 章", index+1)
			}
			chapterURL := fmt.Sprintf("%s/chapter_%d", target.URL, index)
			contentDir := filepath.Join(importer.cfg.LibraryDir, archive.Directory, "content")
			contentPath, err := engine.WriteChapterCache(contentDir, target.URL, chapterURL, parsedChapter.Content)
			if err != nil {
				return err
			}
			cachePath := filepath.Join("content", contentPath)
			chapter := models.Chapter{
				BookID:              target.ID,
				Index:               index,
				Title:               chapterTitle,
				URL:                 chapterURL,
				CachePath:           cachePath,
				ResourcePath:        parsedChapter.ResourcePath,
				ResourceFragment:    parsedChapter.ResourceFragment,
				ResourceEndFragment: parsedChapter.ResourceEndFragment,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			archivedChapters = append(archivedChapters, engine.ArchivedChapter{
				ID:                  chapter.ID,
				URL:                 chapterURL,
				Title:               chapterTitle,
				IsVolume:            false,
				BaseURL:             "",
				BookURL:             archive.OriginalFile,
				Index:               index,
				Start:               parsedChapter.Start,
				End:                 parsedChapter.End,
				CachePath:           cachePath,
				ResourcePath:        parsedChapter.ResourcePath,
				ResourceFragment:    parsedChapter.ResourceFragment,
				ResourceEndFragment: parsedChapter.ResourceEndFragment,
			})
		}
		source := engine.ArchivedBookSource{
			BookURL:            archive.OriginalFile,
			Origin:             "loc_book",
			OriginName:         archive.OriginalFile,
			Type:               0,
			Name:               target.Title,
			Author:             target.Author,
			LatestChapterTitle: target.LastChapter,
			TOCURL:             archive.TOCFile,
			Time:               0,
			OriginOrder:        0,
			UserNameSpace:      request.UserName,
		}
		if err := engine.WriteBookSource(importer.cfg.LibraryDir, archive, source); err != nil {
			return err
		}
		if err := engine.WriteChapterArchive(importer.cfg.LibraryDir, archive, archivedChapters); err != nil {
			return err
		}
		book = target
		return nil
	})
	if err != nil {
		return models.Book{}, err
	}
	cleanupArchive = false
	return book, nil
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
		chapters, err := engine.ParseTXTWithRule(data, tocRule)
		if err != nil {
			return engine.ParsedBook{}, err
		}
		return engine.ParsedBook{Chapters: chapters}, nil
	case ".pdf":
		return engine.ParsePDFWithLimits(data, limits)
	case ".umd":
		return engine.ParseUMDWithLimits(data, limits)
	default:
		return engine.ParsedBook{}, ErrUnsupportedFormat
	}
}
