package db

import (
	"path/filepath"
	"testing"
	"time"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestAutoMigrateBackfillsBookLastCheckTimeFromCreationWithoutTouchingUpdatedAt(t *testing.T) {
	root := t.TempDir()
	database, err := Open(config.Config{DatabasePath: filepath.Join(root, "data", "openreader.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	createdAt := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(48 * time.Hour)
	book := models.Book{UserID: 1, Title: "旧书架时间", CreatedAt: createdAt, UpdatedAt: updatedAt}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.LastCheckTime <= 0 {
		t.Fatalf("new shelf row did not initialize lastCheckTime: %+v", book)
	}
	if err := database.Migrator().DropColumn(&models.Book{}, "LastCheckTime"); err != nil {
		t.Fatal(err)
	}
	if database.Migrator().HasColumn(&models.Book{}, "LastCheckTime") {
		t.Fatal("legacy books fixture unexpectedly retained last_check_time")
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	var migrated models.Book
	if err := database.First(&migrated, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.LastCheckTime != createdAt.UnixMilli() {
		t.Fatalf("lastCheckTime = %d, want creation time %d", migrated.LastCheckTime, createdAt.UnixMilli())
	}
	if !migrated.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("migration changed generic updatedAt: got %s want %s", migrated.UpdatedAt, updatedAt)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	var second models.Book
	if err := database.First(&second, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if second.LastCheckTime != migrated.LastCheckTime || !second.UpdatedAt.Equal(migrated.UpdatedAt) {
		t.Fatalf("second migration was not idempotent: first=%+v second=%+v", migrated, second)
	}
}
