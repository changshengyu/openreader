package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestAutoMigrateAddsBookSourceOwnershipWithoutRewritingReferences(t *testing.T) {
	database := openLegacyBookSourceDatabase(t)

	alice := models.User{Username: "source-owner-alice", PasswordHash: "hash", Role: "user"}
	bob := models.User{Username: "source-owner-bob", PasswordHash: "hash", Role: "user"}
	if err := database.Create(&[]*models.User{&alice, &bob}).Error; err != nil {
		t.Fatal(err)
	}
	sourceA := models.BookSource{Name: "旧全局源 A", BaseURL: "https://source-a.example", Enabled: true}
	sourceB := models.BookSource{Name: "旧全局源 B", BaseURL: "https://source-b.example", Enabled: true}
	if err := database.Create(&[]*models.BookSource{&sourceA, &sourceB}).Error; err != nil {
		t.Fatal(err)
	}
	aliceBook := models.Book{UserID: alice.ID, SourceID: sourceA.ID, Title: "Alice 旧书"}
	bobBook := models.Book{UserID: bob.ID, SourceID: sourceA.ID, Title: "Bob 旧书"}
	if err := database.Create(&[]*models.Book{&aliceBook, &bobBook}).Error; err != nil {
		t.Fatal(err)
	}
	failedAt := time.Now().UTC().Truncate(time.Second)
	aliceFailure := models.SourceFailure{
		UserID: alice.ID, SourceID: sourceB.ID, SourceURL: sourceB.BaseURL,
		Message: "alice failed", FailedAt: failedAt, ExpiresAt: failedAt.Add(time.Hour),
	}
	bobFailure := models.SourceFailure{
		UserID: bob.ID, SourceID: sourceB.ID, SourceURL: sourceB.BaseURL,
		Message: "bob failed", FailedAt: failedAt, ExpiresAt: failedAt.Add(time.Hour),
	}
	if err := database.Create(&[]*models.SourceFailure{&aliceFailure, &bobFailure}).Error; err != nil {
		t.Fatal(err)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatalf("migrate legacy source ownership: %v", err)
	}

	assertSourceAssociations(t, database, 0, []uint{sourceA.ID, sourceB.ID})
	assertSourceAssociations(t, database, alice.ID, []uint{sourceA.ID, sourceB.ID})
	assertSourceAssociations(t, database, bob.ID, []uint{sourceA.ID, sourceB.ID})
	assertSourceNamespace(t, database, 0, true)
	assertSourceNamespace(t, database, alice.ID, true)
	assertSourceNamespace(t, database, bob.ID, true)

	var sources []models.BookSource
	if err := database.Order("id asc").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].ID != sourceA.ID || sources[1].ID != sourceB.ID {
		t.Fatalf("ownership migration copied or replaced source rows: %+v", sources)
	}
	var books []models.Book
	if err := database.Order("id asc").Find(&books).Error; err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 || books[0].SourceID != sourceA.ID || books[1].SourceID != sourceA.ID {
		t.Fatalf("ownership migration rewrote book source IDs: %+v", books)
	}
	var failures []models.SourceFailure
	if err := database.Order("id asc").Find(&failures).Error; err != nil {
		t.Fatal(err)
	}
	if len(failures) != 2 || failures[0].SourceID != sourceB.ID || failures[1].SourceID != sourceB.ID {
		t.Fatalf("ownership migration rewrote failure source IDs: %+v", failures)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatalf("repeat source ownership migration: %v", err)
	}
	assertSourceAssociations(t, database, 0, []uint{sourceA.ID, sourceB.ID})
	assertSourceAssociations(t, database, alice.ID, []uint{sourceA.ID, sourceB.ID})
	assertSourceAssociations(t, database, bob.ID, []uint{sourceA.ID, sourceB.ID})
}

func TestAutoMigrateMarksExistingEmptyUserSourceNamespace(t *testing.T) {
	database := openLegacyBookSourceDatabase(t)
	user := models.User{Username: "empty-source-owner", PasswordHash: "hash", Role: "user"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatalf("migrate empty legacy source ownership: %v", err)
	}

	assertSourceAssociations(t, database, user.ID, nil)
	assertSourceNamespace(t, database, user.ID, true)
	assertSourceNamespace(t, database, 0, false)
}

func TestBookSourceOwnershipMigrationRollsBackAndRetries(t *testing.T) {
	database := openLegacyBookSourceDatabase(t)
	user := models.User{Username: "retry-source-owner", PasswordHash: "hash", Role: "user"}
	source := models.BookSource{Name: "迁移回滚源", BaseURL: "https://rollback.example", Enabled: true}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "迁移回滚书"}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	const callbackName = "test:fail-book-source-ownership-migration"
	if err := database.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_book_sources" {
			tx.AddError(errors.New("injected ownership migration failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err == nil {
		t.Fatal("source ownership migration unexpectedly succeeded after injected persistence failure")
	}
	database.Callback().Create().Remove(callbackName)

	assertSourceAssociations(t, database, user.ID, nil)
	assertSourceNamespace(t, database, user.ID, false)
	var persistedBook models.Book
	if err := database.First(&persistedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedBook.SourceID != source.ID {
		t.Fatalf("failed migration rewrote the old book reference: %+v", persistedBook)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatalf("retry source ownership migration: %v", err)
	}
	assertSourceAssociations(t, database, 0, []uint{source.ID})
	assertSourceAssociations(t, database, user.ID, []uint{source.ID})
	assertSourceNamespace(t, database, user.ID, true)
}

func openLegacyBookSourceDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := Open(config.Config{
		DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&models.User{},
		&models.BookSource{},
		&models.SourceFailure{},
		&models.Book{},
	); err != nil {
		t.Fatal(err)
	}
	return database
}

func assertSourceAssociations(t *testing.T, database *gorm.DB, userID uint, expected []uint) {
	t.Helper()
	var rows []models.UserBookSource
	if err := database.Where("user_id = ?", userID).Order("source_id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(expected) {
		t.Fatalf("user %d source associations = %+v, want %v", userID, rows, expected)
	}
	for index, sourceID := range expected {
		if rows[index].SourceID != sourceID || rows[index].Detached {
			t.Fatalf("user %d source association %d = %+v, want active source %d", userID, index, rows[index], sourceID)
		}
	}
}

func assertSourceNamespace(t *testing.T, database *gorm.DB, userID uint, expected bool) {
	t.Helper()
	var count int64
	if err := database.Model(&models.BookSourceNamespace{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if (count == 1) != expected {
		t.Fatalf("user %d namespace present = %v, want %v", userID, count == 1, expected)
	}
}
