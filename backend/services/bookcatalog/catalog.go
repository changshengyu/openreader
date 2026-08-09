package bookcatalog

import (
	"path"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/models"
)

// ReplaceChapterRows is the durable catalogue boundary shared by explicit
// refresh, source change, local refresh and background/manual update checks.
// Returned cache paths may only be pruned after the caller's transaction has
// committed.
func ReplaceChapterRows(tx *gorm.DB, userID, bookID uint, next []models.Chapter) ([]string, map[int]uint, error) {
	var previous []models.Chapter
	if err := tx.Where("book_id = ?", bookID).Order("`index` asc").Find(&previous).Error; err != nil {
		return nil, nil, err
	}
	previousCachePaths := make([]string, 0, len(previous))
	for _, chapter := range previous {
		previousCachePaths = append(previousCachePaths, chapter.CachePath)
	}

	if err := tx.Where("book_id = ?", bookID).Delete(&models.Chapter{}).Error; err != nil {
		return nil, nil, err
	}
	nextChapterIDs := make(map[int]uint, len(next))
	for index := range next {
		chapter := next[index]
		chapter.ID = 0
		chapter.BookID = bookID
		if _, exists := nextChapterIDs[chapter.Index]; exists {
			return nil, nil, gorm.ErrDuplicatedKey
		}
		// Replacement catalogues never carry cached source content forward.
		// Local callers create their derived CachePath explicitly after parsing.
		if err := tx.Create(&chapter).Error; err != nil {
			return nil, nil, err
		}
		nextChapterIDs[chapter.Index] = chapter.ID
		next[index] = chapter
	}
	if err := reconcileChapterReferences(tx, userID, bookID, previous, next, nextChapterIDs); err != nil {
		return nil, nil, err
	}
	return previousCachePaths, nextChapterIDs, nil
}

// reconcileChapterReferences keeps recoverable book-level positions after a
// catalogue replacement. Canonical EPUB resource identity takes precedence;
// all other formats retain the existing index fallback. Offsets and percentages
// are intentionally preserved.
func reconcileChapterReferences(tx *gorm.DB, userID, bookID uint, previous, next []models.Chapter, chapterIDs map[int]uint) error {
	previousByID := make(map[uint]models.Chapter, len(previous))
	previousByIndex := make(map[int]models.Chapter, len(previous))
	for _, chapter := range previous {
		previousByID[chapter.ID] = chapter
		previousByIndex[chapter.Index] = chapter
	}
	nextByResourcePath := make(map[string]models.Chapter, len(next))
	for _, chapter := range next {
		if resourcePath := chapterResourceIdentity(chapter.ResourcePath); resourcePath != "" {
			if _, exists := nextByResourcePath[resourcePath]; !exists {
				nextByResourcePath[resourcePath] = chapter
			}
		}
	}
	resolve := func(chapterID uint, chapterIndex int) (uint, int) {
		oldChapter, exists := previousByID[chapterID]
		if !exists {
			oldChapter, exists = previousByIndex[chapterIndex]
		}
		if exists {
			if resourcePath := chapterResourceIdentity(oldChapter.ResourcePath); resourcePath != "" {
				if replacement, ok := nextByResourcePath[resourcePath]; ok {
					return replacement.ID, replacement.Index
				}
			}
		}
		return chapterIDs[chapterIndex], chapterIndex
	}

	var progresses []models.ReadingProgress
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Find(&progresses).Error; err != nil {
		return err
	}
	for _, progress := range progresses {
		chapterID, chapterIndex := resolve(progress.ChapterID, progress.ChapterIndex)
		if progress.ChapterID == chapterID && progress.ChapterIndex == chapterIndex {
			continue
		}
		if err := tx.Model(&models.ReadingProgress{}).Where("id = ?", progress.ID).Updates(map[string]any{
			"chapter_id": chapterID, "chapter_index": chapterIndex,
		}).Error; err != nil {
			return err
		}
	}

	var bookmarks []models.Bookmark
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Find(&bookmarks).Error; err != nil {
		return err
	}
	for _, bookmark := range bookmarks {
		chapterID, chapterIndex := resolve(bookmark.ChapterID, bookmark.ChapterIndex)
		if bookmark.ChapterID == chapterID && bookmark.ChapterIndex == chapterIndex {
			continue
		}
		if err := tx.Model(&models.Bookmark{}).Where("id = ?", bookmark.ID).Updates(map[string]any{
			"chapter_id": chapterID, "chapter_index": chapterIndex,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func chapterResourceIdentity(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	value = path.Clean(value)
	if value == "." || value == "/" || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "/") {
		return ""
	}
	return value
}
