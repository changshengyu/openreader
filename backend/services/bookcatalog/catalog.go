package bookcatalog

import (
	"path"
	"sort"
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
	if err := reconcileChapterReferences(tx, userID, bookID, previous, next); err != nil {
		return nil, nil, err
	}
	return previousCachePaths, nextChapterIDs, nil
}

// reconcileChapterReferences keeps recoverable book-level positions after a
// catalogue replacement. Canonical resource identity takes precedence, then a
// unique chapter title, then the existing index. References outside a shortened
// catalogue are clamped to a readable chapter. Offsets and percentages remain.
func reconcileChapterReferences(tx *gorm.DB, userID, bookID uint, previous, next []models.Chapter) error {
	previousByID := make(map[uint]models.Chapter, len(previous))
	previousByIndex := make(map[int]models.Chapter, len(previous))
	previousTitleCounts := make(map[string]int, len(previous))
	for _, chapter := range previous {
		previousByID[chapter.ID] = chapter
		previousByIndex[chapter.Index] = chapter
		if title := chapterTitleIdentity(chapter.Title); title != "" {
			previousTitleCounts[title]++
		}
	}
	nextResourceCounts := make(map[string]int, len(next))
	nextByResource := make(map[string]models.Chapter, len(next))
	nextByIndex := make(map[int]models.Chapter, len(next))
	nextTitleCounts := make(map[string]int, len(next))
	nextByTitle := make(map[string]models.Chapter, len(next))
	nextIndexes := make([]int, 0, len(next))
	for _, chapter := range next {
		nextByIndex[chapter.Index] = chapter
		nextIndexes = append(nextIndexes, chapter.Index)
		if resource := chapterResourceIdentity(chapter); resource != "" {
			nextResourceCounts[resource]++
			nextByResource[resource] = chapter
		}
		if title := chapterTitleIdentity(chapter.Title); title != "" {
			nextTitleCounts[title]++
			nextByTitle[title] = chapter
		}
	}
	sort.Ints(nextIndexes)
	resolve := func(chapterID uint, chapterIndex int, storedTitle string) (models.Chapter, bool) {
		oldChapter, exists := previousByID[chapterID]
		if !exists {
			oldChapter, exists = previousByIndex[chapterIndex]
		}
		if exists {
			if resource := chapterResourceIdentity(oldChapter); resource != "" && nextResourceCounts[resource] == 1 {
				if replacement, ok := nextByResource[resource]; ok {
					return replacement, true
				}
			}
		}
		title := chapterTitleIdentity(storedTitle)
		if exists && chapterTitleIdentity(oldChapter.Title) != "" {
			title = chapterTitleIdentity(oldChapter.Title)
		}
		oldTitleIsUnique := !exists || previousTitleCounts[title] == 1
		if title != "" && oldTitleIsUnique && nextTitleCounts[title] == 1 {
			return nextByTitle[title], true
		}
		if replacement, ok := nextByIndex[chapterIndex]; ok {
			return replacement, true
		}
		if len(nextIndexes) == 0 {
			return models.Chapter{}, false
		}
		if chapterIndex <= nextIndexes[0] {
			return nextByIndex[nextIndexes[0]], true
		}
		return nextByIndex[nextIndexes[len(nextIndexes)-1]], true
	}

	var progresses []models.ReadingProgress
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Find(&progresses).Error; err != nil {
		return err
	}
	for _, progress := range progresses {
		replacement, ok := resolve(progress.ChapterID, progress.ChapterIndex, progress.ChapterTitle)
		if !ok {
			replacement = models.Chapter{Index: progress.ChapterIndex, Title: progress.ChapterTitle}
		}
		if progress.ChapterID == replacement.ID && progress.ChapterIndex == replacement.Index && progress.ChapterTitle == replacement.Title {
			continue
		}
		if err := tx.Model(&models.ReadingProgress{}).Where("id = ?", progress.ID).Updates(map[string]any{
			"chapter_id": replacement.ID, "chapter_index": replacement.Index, "chapter_title": replacement.Title,
		}).Error; err != nil {
			return err
		}
	}

	var bookmarks []models.Bookmark
	if err := tx.Where("user_id = ? AND book_id = ?", userID, bookID).Find(&bookmarks).Error; err != nil {
		return err
	}
	for _, bookmark := range bookmarks {
		replacement, ok := resolve(bookmark.ChapterID, bookmark.ChapterIndex, "")
		if !ok {
			replacement = models.Chapter{Index: bookmark.ChapterIndex}
		}
		if bookmark.ChapterID == replacement.ID && bookmark.ChapterIndex == replacement.Index {
			continue
		}
		if err := tx.Model(&models.Bookmark{}).Where("id = ?", bookmark.ID).Updates(map[string]any{
			"chapter_id": replacement.ID, "chapter_index": replacement.Index,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func chapterTitleIdentity(value string) string {
	return strings.TrimSpace(value)
}

func chapterResourceIdentity(chapter models.Chapter) string {
	resourcePath := strings.TrimSpace(strings.ReplaceAll(chapter.ResourcePath, "\\", "/"))
	if resourcePath == "" {
		return ""
	}
	resourcePath = path.Clean(resourcePath)
	if resourcePath == "." || resourcePath == "/" || strings.HasPrefix(resourcePath, "../") || strings.HasPrefix(resourcePath, "/") {
		return ""
	}
	return strings.Join([]string{
		resourcePath,
		strings.TrimSpace(chapter.ResourceFragment),
		strings.TrimSpace(chapter.ResourceEndFragment),
	}, "\x00")
}
