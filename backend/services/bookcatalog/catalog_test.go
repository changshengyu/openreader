package bookcatalog

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestReplaceChapterRowsRebindsReferencesByUniqueTitleBeforeIndex(t *testing.T) {
	db := openBookCatalogTestDB(t)
	const userID = 7
	const bookID = 11
	previous := []models.Chapter{
		{BookID: bookID, Index: 0, Title: "第一章", URL: "old/0"},
		{BookID: bookID, Index: 1, Title: "第二章", URL: "old/1"},
	}
	for index := range previous {
		if err := db.Create(&previous[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	progress := models.ReadingProgress{
		UserID: userID, BookID: bookID, ChapterID: previous[1].ID, ChapterIndex: 1,
		ChapterTitle: "第二章", Offset: 37, Percent: 0.5,
	}
	bookmark := models.Bookmark{
		UserID: userID, BookID: bookID, ChapterID: previous[1].ID, ChapterIndex: 1,
		Offset: 19, Title: "保留书签", Note: "保留备注",
	}
	if err := db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	next := []models.Chapter{
		{Index: 0, Title: "新增前言", URL: "new/0"},
		{Index: 1, Title: "第一章", URL: "new/1"},
		{Index: 2, Title: "第二章", URL: "new/2"},
	}
	var ids map[int]uint
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		_, ids, err = ReplaceChapterRows(tx, userID, bookID, next)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&progress, progress.ID).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != ids[2] || progress.ChapterIndex != 2 || progress.ChapterTitle != "第二章" || progress.Offset != 37 {
		t.Fatalf("title-rebound progress = %+v, want new 第二章", progress)
	}
	if err := db.First(&bookmark, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if bookmark.ChapterID != ids[2] || bookmark.ChapterIndex != 2 || bookmark.Offset != 19 || bookmark.Note != "保留备注" {
		t.Fatalf("title-rebound bookmark = %+v, want new 第二章", bookmark)
	}
}

func TestReplaceChapterRowsUsesProgressTitleWithoutOldChapterAndClampsMissingIndex(t *testing.T) {
	db := openBookCatalogTestDB(t)
	const userID = 8
	const bookID = 12
	titled := models.ReadingProgress{UserID: userID, BookID: bookID, ChapterIndex: 0, ChapterTitle: "第二章"}
	if err := db.Create(&titled).Error; err != nil {
		t.Fatal(err)
	}
	bookmark := models.Bookmark{UserID: userID, BookID: bookID, ChapterIndex: 99, Title: "越界书签"}
	if err := db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}
	next := []models.Chapter{
		{Index: 0, Title: "新增前言", URL: "new/0"},
		{Index: 1, Title: "第二章", URL: "new/1"},
	}
	var ids map[int]uint
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		_, ids, err = ReplaceChapterRows(tx, userID, bookID, next)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&titled, titled.ID).Error; err != nil {
		t.Fatal(err)
	}
	if titled.ChapterID != ids[1] || titled.ChapterIndex != 1 || titled.ChapterTitle != "第二章" {
		t.Fatalf("progress restored without old rows = %+v, want title index 1", titled)
	}
	if err := db.First(&bookmark, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if bookmark.ChapterID != ids[1] || bookmark.ChapterIndex != 1 {
		t.Fatalf("out-of-range bookmark = %+v, want final valid chapter", bookmark)
	}
}

func TestReplaceChapterRowsUsesEPUBFragmentsAndRejectsAmbiguousTitles(t *testing.T) {
	db := openBookCatalogTestDB(t)
	const userID = 9
	const bookID = 13
	previous := []models.Chapter{
		{BookID: bookID, Index: 0, Title: "同名小节", ResourcePath: "OPS/chapter.xhtml", ResourceFragment: "part-a"},
		{BookID: bookID, Index: 1, Title: "同名小节", ResourcePath: "OPS/chapter.xhtml", ResourceFragment: "part-b"},
	}
	for index := range previous {
		if err := db.Create(&previous[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	progress := models.ReadingProgress{
		UserID: userID, BookID: bookID, ChapterID: previous[1].ID, ChapterIndex: 1, ChapterTitle: "同名小节",
	}
	if err := db.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}

	next := []models.Chapter{
		{Index: 0, Title: "新增前言", ResourcePath: "OPS/preface.xhtml"},
		{Index: 1, Title: "同名小节", ResourcePath: "OPS/chapter.xhtml", ResourceFragment: "part-a"},
		{Index: 2, Title: "同名小节", ResourcePath: "OPS/chapter.xhtml", ResourceFragment: "part-b"},
	}
	var ids map[int]uint
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		_, ids, err = ReplaceChapterRows(tx, userID, bookID, next)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&progress, progress.ID).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != ids[2] || progress.ChapterIndex != 2 {
		t.Fatalf("fragment-rebound progress = %+v, want part-b at index 2", progress)
	}

	const ambiguousBookID = 14
	ambiguousPrevious := []models.Chapter{
		{BookID: ambiguousBookID, Index: 0, Title: "重复标题"},
		{BookID: ambiguousBookID, Index: 1, Title: "重复标题"},
	}
	for index := range ambiguousPrevious {
		if err := db.Create(&ambiguousPrevious[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	ambiguousProgress := models.ReadingProgress{
		UserID: userID, BookID: ambiguousBookID, ChapterID: ambiguousPrevious[1].ID, ChapterIndex: 1, ChapterTitle: "重复标题",
	}
	if err := db.Create(&ambiguousProgress).Error; err != nil {
		t.Fatal(err)
	}
	ambiguousNext := []models.Chapter{
		{Index: 0, Title: "新增前言"},
		{Index: 1, Title: "重复标题"},
		{Index: 2, Title: "重复标题"},
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := ReplaceChapterRows(tx, userID, ambiguousBookID, ambiguousNext)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&ambiguousProgress, ambiguousProgress.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ambiguousProgress.ChapterIndex != 1 {
		t.Fatalf("ambiguous title moved progress to index %d", ambiguousProgress.ChapterIndex)
	}
}

func openBookCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:bookcatalog-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Chapter{}, &models.ReadingProgress{}, &models.Bookmark{}); err != nil {
		t.Fatal(err)
	}
	return db
}
