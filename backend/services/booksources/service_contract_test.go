package booksources

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

func TestInitializeCopiesDefaultOnceAndPreservesExplicitEmptyNamespace(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	alice := createSourceUser(t, database, "source-service-alice")
	bob := createSourceUser(t, database, "source-service-bob")
	empty := createSourceUser(t, database, "source-service-empty")

	defaultA := createSourceSnapshot(t, database, "默认 A", "https://default-a.example")
	attachSource(t, database, 0, defaultA.ID, false)
	markSourceNamespace(t, database, 0)
	markSourceNamespace(t, database, empty.ID)

	aliceSources, err := service.ListActive(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceSources) != 1 || aliceSources[0].ID != defaultA.ID {
		t.Fatalf("alice initial sources = %+v", aliceSources)
	}

	defaultB := createSourceSnapshot(t, database, "默认 B", "https://default-b.example")
	attachSource(t, database, 0, defaultB.ID, false)

	aliceSources, err = service.ListActive(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceSources) != 1 || aliceSources[0].ID != defaultA.ID {
		t.Fatalf("initialized alice changed with later default: %+v", aliceSources)
	}
	bobSources, err := service.ListActive(bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bobSources) != 2 || bobSources[0].ID != defaultA.ID || bobSources[1].ID != defaultB.ID {
		t.Fatalf("new bob did not copy current defaults: %+v", bobSources)
	}
	emptySources, err := service.ListActive(empty.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(emptySources) != 0 {
		t.Fatalf("explicit empty namespace unexpectedly inherited defaults: %+v", emptySources)
	}
}

func TestUpdateUsesCopyOnWriteAndRemapsOnlyTheTargetUser(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	alice := createSourceUser(t, database, "source-cow-alice")
	bob := createSourceUser(t, database, "source-cow-bob")
	shared := createSourceSnapshot(t, database, "共享源", "https://shared.example")
	for _, userID := range []uint{0, alice.ID, bob.ID} {
		attachSource(t, database, userID, shared.ID, false)
		markSourceNamespace(t, database, userID)
	}

	aliceBook := models.Book{
		UserID: alice.ID, SourceID: shared.ID, Title: "Alice 共享书",
		Variable: `{"token":"alice"}`,
	}
	bobBook := models.Book{
		UserID: bob.ID, SourceID: shared.ID, Title: "Bob 共享书",
		Variable: `{"token":"bob"}`,
	}
	if err := database.Create(&[]*models.Book{&aliceBook, &bobBook}).Error; err != nil {
		t.Fatal(err)
	}
	aliceChapter := models.Chapter{BookID: aliceBook.ID, Index: 0, Title: "A", Variable: `{"chapter":"alice"}`}
	bobChapter := models.Chapter{BookID: bobBook.ID, Index: 0, Title: "B", Variable: `{"chapter":"bob"}`}
	if err := database.Create(&[]*models.Chapter{&aliceChapter, &bobChapter}).Error; err != nil {
		t.Fatal(err)
	}
	failedAt := time.Now().UTC()
	aliceFailure := models.SourceFailure{
		UserID: alice.ID, SourceID: shared.ID, SourceURL: shared.BaseURL,
		Message: "alice", FailedAt: failedAt, ExpiresAt: failedAt.Add(time.Hour),
	}
	bobFailure := models.SourceFailure{
		UserID: bob.ID, SourceID: shared.ID, SourceURL: shared.BaseURL,
		Message: "bob", FailedAt: failedAt, ExpiresAt: failedAt.Add(time.Hour),
	}
	if err := database.Create(&[]*models.SourceFailure{&aliceFailure, &bobFailure}).Error; err != nil {
		t.Fatal(err)
	}

	next := shared
	next.Name = "Alice 私有源"
	next.Header = `{"Authorization":"alice"}`
	next.Rules = `{"contentRule":".alice"}`
	updated, err := service.Update(alice.ID, shared.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID == shared.ID {
		t.Fatalf("shared source was mutated in place: %+v", updated)
	}

	aliceActive, err := service.FindActive(alice.ID, updated.ID)
	if err != nil || aliceActive.Name != "Alice 私有源" {
		t.Fatalf("alice private source = %+v err=%v", aliceActive, err)
	}
	if _, err := service.FindActive(alice.ID, shared.ID); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("alice still has the shared source active: %v", err)
	}
	bobActive, err := service.FindActive(bob.ID, shared.ID)
	if err != nil || bobActive.Name != "共享源" || bobActive.Header != "" {
		t.Fatalf("bob shared source changed: %+v err=%v", bobActive, err)
	}
	defaultActive, err := service.FindActive(0, shared.ID)
	if err != nil || defaultActive.Name != "共享源" || defaultActive.Rules == updated.Rules {
		t.Fatalf("default source changed: %+v err=%v", defaultActive, err)
	}

	assertBookSourceAndVariable(t, database, aliceBook.ID, updated.ID, "")
	assertBookSourceAndVariable(t, database, bobBook.ID, shared.ID, `{"token":"bob"}`)
	assertChapterVariable(t, database, aliceChapter.ID, "")
	assertChapterVariable(t, database, bobChapter.ID, `{"chapter":"bob"}`)
	assertFailureSource(t, database, aliceFailure.ID, updated.ID)
	assertFailureSource(t, database, bobFailure.ID, shared.ID)
}

func TestCreateAndDeleteAreScopedToTheCallingUser(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	alice := createSourceUser(t, database, "source-crud-alice")
	bob := createSourceUser(t, database, "source-crud-bob")
	markSourceNamespace(t, database, alice.ID)
	markSourceNamespace(t, database, bob.ID)

	created, err := service.Create(alice.ID, models.BookSource{
		Name: "Alice 新源", BaseURL: "https://alice-source.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FindActive(bob.ID, created.ID); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("bob can access alice source: %v", err)
	}

	attachSource(t, database, bob.ID, created.ID, false)
	bobBook := models.Book{UserID: bob.ID, SourceID: created.ID, Title: "Bob 使用共享源"}
	if err := database.Create(&bobBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(alice.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FindActive(alice.ID, created.ID); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("alice source association remained after delete: %v", err)
	}
	if _, err := service.FindActive(bob.ID, created.ID); err != nil {
		t.Fatalf("alice deletion removed bob source: %v", err)
	}
	deleteErr := service.Delete(bob.ID, created.ID)
	if !errors.Is(deleteErr, ErrSourceInUse) {
		t.Fatalf("deleting bob's used source = %v, want ErrSourceInUse", deleteErr)
	}
	if usage := SourceUsage(deleteErr); usage != 1 {
		t.Fatalf("used-source count = %d, want 1", usage)
	}
}

func TestClearActiveDetachesUsedSourcesAndKeepsOtherUsersIntact(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	alice := createSourceUser(t, database, "source-clear-alice")
	bob := createSourceUser(t, database, "source-clear-bob")
	shared := createSourceSnapshot(t, database, "共享在用源", "https://clear-shared.example")
	free := createSourceSnapshot(t, database, "Alice 空闲源", "https://clear-free.example")
	for _, sourceID := range []uint{shared.ID, free.ID} {
		attachSource(t, database, alice.ID, sourceID, false)
	}
	attachSource(t, database, bob.ID, shared.ID, false)
	markSourceNamespace(t, database, alice.ID)
	markSourceNamespace(t, database, bob.ID)
	aliceBook := models.Book{UserID: alice.ID, SourceID: shared.ID, Title: "Alice 在用书"}
	if err := database.Create(&aliceBook).Error; err != nil {
		t.Fatal(err)
	}

	affected, err := service.ClearActive(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 {
		t.Fatalf("clear affected = %d, want 2", affected)
	}
	aliceActive, err := service.ListActive(alice.ID)
	if err != nil || len(aliceActive) != 0 {
		t.Fatalf("alice active after clear = %+v err=%v", aliceActive, err)
	}
	if _, err := service.FindForBook(alice.ID, shared.ID); err != nil {
		t.Fatalf("used detached source cannot serve alice book: %v", err)
	}
	bobActive, err := service.FindActive(bob.ID, shared.ID)
	if err != nil || bobActive.ID != shared.ID {
		t.Fatalf("bob shared source changed during alice clear: %+v err=%v", bobActive, err)
	}
	var freeCount int64
	if err := database.Model(&models.BookSource{}).Where("id = ?", free.ID).Count(&freeCount).Error; err != nil {
		t.Fatal(err)
	}
	if freeCount != 0 {
		t.Fatalf("unreferenced alice source was not collected: %d", freeCount)
	}
}

func TestDefaultSnapshotInitializesOnceAndRestoreIsExplicit(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	admin := createSourceUser(t, database, "source-default-admin")
	alice := createSourceUser(t, database, "source-default-alice")
	bob := createSourceUser(t, database, "source-default-bob")
	markSourceNamespace(t, database, admin.ID)
	markSourceNamespace(t, database, alice.ID)

	defaultSource, err := service.Create(admin.ID, models.BookSource{
		Name: "管理员默认源", BaseURL: "https://default-service.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	count, err := service.SaveDefaultFromUser(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("saved default count = %d, want 1", count)
	}
	configured, defaultCount, err := service.DefaultStatus()
	if err != nil || !configured || defaultCount != 1 {
		t.Fatalf("default status = configured:%v count:%d err:%v", configured, defaultCount, err)
	}

	bobSources, err := service.ListActive(bob.ID)
	if err != nil || len(bobSources) != 1 || bobSources[0].ID != defaultSource.ID {
		t.Fatalf("uninitialized bob did not inherit default: %+v err=%v", bobSources, err)
	}
	aliceSources, err := service.ListActive(alice.ID)
	if err != nil || len(aliceSources) != 0 {
		t.Fatalf("initialized-empty alice inherited without restore: %+v err=%v", aliceSources, err)
	}

	result, err := service.RestoreDefault(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Updated != 0 {
		t.Fatalf("alice restore result = %+v", result)
	}
	aliceSources, err = service.ListActive(alice.ID)
	if err != nil || len(aliceSources) != 1 || aliceSources[0].ID != defaultSource.ID {
		t.Fatalf("alice explicit restore failed: %+v err=%v", aliceSources, err)
	}

	updatedAdmin := defaultSource
	updatedAdmin.Name = "管理员后改私有源"
	adminCopy, err := service.Update(admin.ID, defaultSource.ID, updatedAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if adminCopy.ID == defaultSource.ID {
		t.Fatalf("editing source shared with defaults did not copy: %+v", adminCopy)
	}
	defaultRows, err := service.ListActive(0)
	if err != nil || len(defaultRows) != 1 || defaultRows[0].Name != "管理员默认源" {
		t.Fatalf("default snapshot changed with admin edit: %+v err=%v", defaultRows, err)
	}
}

func TestImportMatchesURLInsideOnlyTheTargetNamespace(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	alice := createSourceUser(t, database, "source-import-alice")
	bob := createSourceUser(t, database, "source-import-bob")
	shared := createSourceSnapshot(t, database, "共享导入源", "https://import-shared.example")
	for _, userID := range []uint{alice.ID, bob.ID} {
		attachSource(t, database, userID, shared.ID, false)
		markSourceNamespace(t, database, userID)
	}

	result, err := service.Import(alice.ID, []models.BookSource{
		{Name: "Alice 覆盖", BaseURL: " https://import-shared.example ", Enabled: true},
		{Name: "Alice 新源", BaseURL: "https://import-new.example", Enabled: true},
		{Name: "", BaseURL: "https://invalid.example", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Updated != 1 || result.Skipped != 1 {
		t.Fatalf("import result = %+v", result)
	}
	aliceSources, err := service.ListActive(alice.ID)
	if err != nil || len(aliceSources) != 2 {
		t.Fatalf("alice imported sources = %+v err=%v", aliceSources, err)
	}
	bobSource, err := service.FindActive(bob.ID, shared.ID)
	if err != nil || bobSource.Name != "共享导入源" || bobSource.BaseURL != "https://import-shared.example" {
		t.Fatalf("alice import changed bob source: %+v err=%v", bobSource, err)
	}
}

func TestBatchUpdateRollsBackEverySourceWhenOneWriteFails(t *testing.T) {
	database := sourceServiceDatabase(t)
	service := New(database)
	alice := createSourceUser(t, database, "source-batch-rollback")
	markSourceNamespace(t, database, alice.ID)
	first, err := service.Create(alice.ID, models.BookSource{
		Name: "批量一", BaseURL: "https://batch-one.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(alice.ID, models.BookSource{
		Name: "批量二", BaseURL: "https://batch-two.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE TRIGGER block_second_source_update
		BEFORE UPDATE ON book_sources
		WHEN OLD.id = ` + uintText(second.ID) + `
		BEGIN
			SELECT RAISE(ABORT, 'blocked');
		END`
	if err := database.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.BatchSetGroup(alice.ID, []uint{first.ID, second.ID}, "不得部分提交"); err == nil {
		t.Fatal("batch update unexpectedly succeeded")
	}
	var stored []models.BookSource
	if err := database.Where("id IN ?", []uint{first.ID, second.ID}).Order("id asc").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 || stored[0].Group != "" || stored[1].Group != "" {
		t.Fatalf("batch update partially committed: %+v", stored)
	}
}

func sourceServiceDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := readerdb.Open(config.Config{
		DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	return database
}

func createSourceUser(t *testing.T, database *gorm.DB, username string) models.User {
	t.Helper()
	user := models.User{Username: username, PasswordHash: "hash", Role: "user"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func createSourceSnapshot(t *testing.T, database *gorm.DB, name, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: name, BaseURL: baseURL, Enabled: true}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func attachSource(t *testing.T, database *gorm.DB, userID, sourceID uint, detached bool) {
	t.Helper()
	if err := database.Create(&models.UserBookSource{
		UserID: userID, SourceID: sourceID, Detached: detached,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func markSourceNamespace(t *testing.T, database *gorm.DB, userID uint) {
	t.Helper()
	if err := database.Create(&models.BookSourceNamespace{UserID: userID}).Error; err != nil {
		t.Fatal(err)
	}
}

func assertBookSourceAndVariable(t *testing.T, database *gorm.DB, bookID, sourceID uint, variable string) {
	t.Helper()
	var book models.Book
	if err := database.First(&book, bookID).Error; err != nil {
		t.Fatal(err)
	}
	if book.SourceID != sourceID || book.Variable != variable {
		t.Fatalf("book %d = %+v, want source=%d variable=%q", bookID, book, sourceID, variable)
	}
}

func assertChapterVariable(t *testing.T, database *gorm.DB, chapterID uint, variable string) {
	t.Helper()
	var chapter models.Chapter
	if err := database.First(&chapter, chapterID).Error; err != nil {
		t.Fatal(err)
	}
	if chapter.Variable != variable {
		t.Fatalf("chapter %d variable = %q, want %q", chapterID, chapter.Variable, variable)
	}
}

func assertFailureSource(t *testing.T, database *gorm.DB, failureID, sourceID uint) {
	t.Helper()
	var failure models.SourceFailure
	if err := database.First(&failure, failureID).Error; err != nil {
		t.Fatal(err)
	}
	if failure.SourceID != sourceID {
		t.Fatalf("failure %d source = %d, want %d", failureID, failure.SourceID, sourceID)
	}
}

func uintText(value uint) string {
	return fmt.Sprintf("%d", value)
}
