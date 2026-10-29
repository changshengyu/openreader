package localbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/bookcatalog"
	"openreader/backend/services/cbzreader"
	"openreader/backend/services/epubreader"
	"openreader/backend/services/rootedfs"
)

var ErrImportCanceled = errors.New("local book import canceled")

type ImportedBook struct {
	Book        models.Book
	CategoryIDs []uint
}

func (importer Importer) WithContext(ctx context.Context) Importer {
	importer.db = importer.db.WithContext(ctx)
	return importer
}

func (importer Importer) context() context.Context {
	if importer.db != nil && importer.db.Statement != nil && importer.db.Statement.Context != nil {
		return importer.db.Statement.Context
	}
	return context.Background()
}

func importCancellation(err error) error { return fmt.Errorf("%w: %w", ErrImportCanceled, err) }

func (importer Importer) ImportResult(request ImportRequest) (ImportedBook, error) {
	if err := importer.context().Err(); err != nil {
		return ImportedBook{}, importCancellation(err)
	}
	_, prepared, err := importer.Prepare(request)
	if canceled := importer.context().Err(); canceled != nil {
		return ImportedBook{}, importCancellation(canceled)
	}
	if err != nil {
		return ImportedBook{}, err
	}
	return importer.ImportPreparedResult(request, prepared)
}

func (importer Importer) ImportPreparedResult(request ImportRequest, prepared PreparedImport) (ImportedBook, error) {
	if err := importer.context().Err(); err != nil {
		return ImportedBook{}, importCancellation(err)
	}
	if !prepared.Matches(request) {
		return ImportedBook{}, ErrPreparedImportMismatch
	}
	parsed := prepared.Book
	if prepared.EPUBCatalogOnly {
		var err error
		parsed, err = engine.MaterializeEPUBCatalogWithLimits(request.Data, parsed, importer.parseLimits())
		if canceled := importer.context().Err(); canceled != nil {
			return ImportedBook{}, importCancellation(canceled)
		}
		if err != nil {
			return ImportedBook{}, fmt.Errorf("%w: %w", ErrParseFailed, err)
		}
	}
	title, author := localBookTitleAuthor(request, parsed, nil)
	placeholder, restore, err := importer.readerDevLocalPlaceholder(request.UserID, title, author)
	if err != nil {
		if canceled := importer.context().Err(); canceled != nil {
			return ImportedBook{}, importCancellation(canceled)
		}
		return ImportedBook{}, err
	}
	if restore {
		return importer.persistParsedBook(&placeholder, request, parsed, title, author)
	}
	return importer.persistParsedBook(nil, request, parsed, title, author)
}

func selectedImportCategories(request ImportRequest) []uint {
	ids, seen := []uint{}, map[uint]bool{}
	for _, id := range request.CategoryIDs {
		if id > 0 && !seen[id] {
			ids, seen[id] = append(ids, id), true
		}
	}
	if len(ids) == 0 && request.CategoryID != nil && *request.CategoryID > 0 {
		ids = append(ids, *request.CategoryID)
	}
	return ids
}

func importCategories(tx *gorm.DB, userID uint, book models.Book) ([]uint, error) {
	var ids []uint
	if err := tx.Model(&models.BookCategory{}).Where("user_id = ? AND book_id = ?", userID, book.ID).Order("id asc").Pluck("category_id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 && book.CategoryID != nil && *book.CategoryID > 0 {
		ids = append(ids, *book.CategoryID)
	}
	return ids, nil
}

func replaceImportCategories(tx *gorm.DB, userID, bookID uint, ids []uint) error {
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Delete(&models.BookCategory{}).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := tx.Create(&models.BookCategory{UserID: userID, BookID: bookID, CategoryID: id}).Error; err != nil {
			return err
		}
	}
	return nil
}

func writeImportJSON(tree *rootedfs.OwnedTree, relative string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = tree.Write(relative, bytes.NewReader(data), int64(len(data)))
	return err
}

func (importer Importer) persistParsedBook(existing *models.Book, request ImportRequest, parsed engine.ParsedBook, title, author string) (result ImportedBook, returnErr error) {
	ctx := importer.context()
	defer func() {
		if returnErr != nil && ctx.Err() != nil {
			returnErr = importCancellation(ctx.Err())
		}
	}()
	if err := ctx.Err(); err != nil {
		return ImportedBook{}, err
	}
	archive, tree, err := engine.OpenImportedBookArchive(ctx, importer.cfg.LibraryDir, request.UserName, title, author, request.FileName, request.Data)
	if err != nil {
		return ImportedBook{}, err
	}
	defer tree.Close()
	work := func(phase string) error {
		importer.observeWork(phase, tree.PathLabel())
		return tree.Check()
	}
	if err := work("raw-written"); err != nil {
		return ImportedBook{}, err
	}
	preparedTarget := models.Book{UserID: request.UserID, SourceID: 0}
	if existing != nil {
		preparedTarget = *existing
	}
	preparedTarget.LibraryPath, preparedTarget.OriginalFile = archive.Directory, archive.OriginalFile
	switch normalizedLocalBookExtension(request.Extension) {
	case ".epub":
		if err := epubreader.New(importer.cfg, importer.db).PrepareImportedResources(preparedTarget, tree); err != nil {
			return ImportedBook{}, classifyEPUBPreparationError(err)
		}
		if err := work("resources-prepared"); err != nil {
			return ImportedBook{}, err
		}
	case ".cbz":
		if err := cbzreader.New(importer.cfg, importer.db).PrepareImportedResources(preparedTarget, tree); err != nil {
			return ImportedBook{}, classifyCBZPreparationError(err)
		}
		if err := work("resources-prepared"); err != nil {
			return ImportedBook{}, err
		}
	}
	ids := selectedImportCategories(request)
	err = importer.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tree.Check(); err != nil {
			return err
		}
		if len(ids) > 0 {
			var count int64
			if err := tx.Model(&models.Category{}).Where("user_id = ? AND id IN ?", request.UserID, ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return errors.New("local import category unavailable")
			}
		}
		var book models.Book
		if existing != nil {
			if err := tx.Where("id = ? AND user_id = ? AND source_id = 0", existing.ID, request.UserID).First(&book).Error; err != nil {
				return err
			}
			if book.LibraryPath != existing.LibraryPath || book.OriginalFile != existing.OriginalFile || book.URL != existing.URL {
				return gorm.ErrDuplicatedKey
			}
		} else {
			book = models.Book{UserID: request.UserID, SourceID: 0, URL: fmt.Sprintf("local://pending/%d", request.UserID)}
		}
		book.Type, book.Title, book.Author = 0, title, author
		book.LibraryPath, book.OriginalFile, book.TOCFile, book.SourceFile = archive.Directory, archive.OriginalFile, archive.TOCFile, archive.SourceFile
		// reader-dev saves the rule it split the catalogue with (book.tocUrl),
		// including the automatically selected TXT rule.
		book.TOCRule, book.ChapterCount, book.LastChapter = firstNonBlank(request.TOCRule, parsed.TOCRule), len(parsed.Chapters), ""
		// Intro and kind come from the file only when the book has none yet;
		// a reattached reader-dev placeholder keeps its backed-up values.
		metadataColumns := map[string]any{}
		if strings.TrimSpace(book.Intro) == "" && parsed.Intro != "" {
			book.Intro, metadataColumns["intro"] = parsed.Intro, parsed.Intro
		}
		if strings.TrimSpace(book.Kind) == "" && parsed.Kind != "" {
			book.Kind, metadataColumns["kind"] = parsed.Kind, parsed.Kind
		}
		if len(parsed.Chapters) > 0 {
			book.LastChapter = parsed.Chapters[len(parsed.Chapters)-1].Title
		}
		if len(ids) > 0 {
			primary := ids[0]
			book.CategoryID = &primary
		}
		if existing == nil {
			if err := tx.Create(&book).Error; err != nil {
				return err
			}
			book.URL = fmt.Sprintf("local://book_%d", book.ID)
			query := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", book.ID, request.UserID).Update("url", book.URL)
			if query.Error != nil {
				return query.Error
			}
			if query.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		next := make([]models.Chapter, 0, len(parsed.Chapters))
		for index, chapter := range parsed.Chapters {
			chapterTitle := strings.TrimSpace(chapter.Title)
			if chapterTitle == "" {
				chapterTitle = fmt.Sprintf("第 %d 章", index+1)
			}
			url := fmt.Sprintf("%s/chapter_%d", book.URL, index)
			cachePath := filepath.Join("content", engine.ChapterCachePath(book.URL, url))
			if _, err := tree.Write(cachePath, strings.NewReader(chapter.Content), int64(len(chapter.Content))); err != nil {
				return err
			}
			if err := work("chapter-written"); err != nil {
				return err
			}
			row := models.Chapter{BookID: book.ID, Index: index, Title: chapterTitle, URL: url, CachePath: cachePath,
				ResourcePath: chapter.ResourcePath, ResourceFragment: chapter.ResourceFragment, ResourceEndFragment: chapter.ResourceEndFragment}
			if existing == nil {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
			next = append(next, row)
		}
		if existing != nil {
			_, chapterIDs, err := bookcatalog.ReplaceChapterRows(tx, request.UserID, book.ID, next)
			if err != nil {
				return err
			}
			for index := range next {
				next[index].ID = chapterIDs[next[index].Index]
			}
			columns := map[string]any{"type": 0, "title": title, "author": author, "library_path": book.LibraryPath,
				"original_file": book.OriginalFile, "toc_file": book.TOCFile, "source_file": book.SourceFile,
				"toc_rule": book.TOCRule, "chapter_count": book.ChapterCount, "last_chapter": book.LastChapter}
			if len(ids) > 0 {
				columns["category_id"] = ids[0]
			}
			for column, value := range metadataColumns {
				columns[column] = value
			}
			query := tx.Model(&models.Book{}).Where("id = ? AND user_id = ? AND source_id = 0 AND url = ? AND COALESCE(library_path, '') = ? AND COALESCE(original_file, '') = ?",
				book.ID, request.UserID, existing.URL, existing.LibraryPath, existing.OriginalFile).Updates(columns)
			if query.Error != nil {
				return query.Error
			}
			if query.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if err := tree.Check(); err != nil {
			return err
		}
		archived := make([]engine.ArchivedChapter, 0, len(next))
		for index, row := range next {
			chapter := parsed.Chapters[index]
			archived = append(archived, engine.ArchivedChapter{ID: row.ID, URL: row.URL, Title: row.Title, BookURL: archive.OriginalFile,
				Index: index, Start: chapter.Start, End: chapter.End, CachePath: row.CachePath, ResourcePath: row.ResourcePath,
				ResourceFragment: row.ResourceFragment, ResourceEndFragment: row.ResourceEndFragment})
		}
		source := engine.ArchivedBookSource{BookURL: archive.OriginalFile, Origin: "loc_book", OriginName: archive.OriginalFile,
			Name: title, Author: author, LatestChapterTitle: book.LastChapter, TOCURL: archive.TOCFile, UserNameSpace: request.UserName}
		if err := writeImportJSON(tree, filepath.Base(archive.SourceFile), []engine.ArchivedBookSource{source}); err != nil {
			return err
		}
		if err := work("source-written"); err != nil {
			return err
		}
		if err := writeImportJSON(tree, filepath.Base(archive.TOCFile), archived); err != nil {
			return err
		}
		if err := work("toc-written"); err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := replaceImportCategories(tx, request.UserID, book.ID, ids); err != nil {
				return err
			}
		}
		if err := tx.Where("id = ? AND user_id = ?", book.ID, request.UserID).First(&result.Book).Error; err != nil {
			return err
		}
		var err error
		result.CategoryIDs, err = importCategories(tx, request.UserID, result.Book)
		if err != nil {
			return err
		}
		if request.BeforeCommit != nil {
			if err := request.BeforeCommit(); err != nil {
				return err
			}
		}
		return tree.Validate()
	})
	if err != nil {
		return ImportedBook{}, err
	}
	tree.Keep()
	return result, nil
}
