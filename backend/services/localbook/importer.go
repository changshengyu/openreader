package localbook

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
)

var (
	ErrUnsupportedFormat  = errors.New("unsupported local book format")
	ErrParseFailed        = errors.New("failed to parse local book")
	ErrNoReadableChapters = errors.New("no readable chapters found")
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
	parsedBook, err := parseUploadedBook(request.Extension, request.Data, request.TOCRule)
	if err != nil {
		if errors.Is(err, ErrUnsupportedFormat) {
			return PreviewResult{}, err
		}
		return PreviewResult{}, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}
	if len(parsedBook.Chapters) == 0 {
		return PreviewResult{}, ErrNoReadableChapters
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
	parsedBook, err := parseUploadedBook(request.Extension, request.Data, request.TOCRule)
	if err != nil {
		if errors.Is(err, ErrUnsupportedFormat) {
			return models.Book{}, err
		}
		return models.Book{}, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}
	chapters := parsedBook.Chapters
	if len(chapters) == 0 {
		return models.Book{}, ErrNoReadableChapters
	}

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
			LastChapter:  chapters[len(chapters)-1].Title,
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
				BookID:    book.ID,
				Index:     index,
				Title:     chapterTitle,
				URL:       chapterURL,
				CachePath: cachePath,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			archivedChapters = append(archivedChapters, engine.ArchivedChapter{
				ID:        chapter.ID,
				URL:       chapterURL,
				Title:     chapterTitle,
				IsVolume:  false,
				BaseURL:   "",
				BookURL:   archive.OriginalFile,
				Index:     index,
				Start:     parsedChapter.Start,
				End:       parsedChapter.End,
				CachePath: cachePath,
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

func parseUploadedBook(ext string, data []byte, tocRule string) (engine.ParsedBook, error) {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".epub":
		return engine.ParseEPUBWithRule(data, tocRule)
	case ".txt", ".text", ".md":
		chapters, err := engine.ParseTXTWithRule(data, tocRule)
		if err != nil {
			return engine.ParsedBook{}, err
		}
		return engine.ParsedBook{Chapters: chapters}, nil
	case ".pdf":
		return engine.ParsePDF(data)
	case ".umd":
		return engine.ParseUMD(data)
	default:
		return engine.ParsedBook{}, ErrUnsupportedFormat
	}
}
