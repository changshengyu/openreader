package db

import (
	"path/filepath"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestAutoMigrateAddsBookSourceCandidatesWithoutRewritingHistoricalRows(t *testing.T) {
	database, err := Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.BookSource{}, &models.Book{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "candidate-migration", PasswordHash: "hash", Role: "user"}
	source := models.BookSource{Name: "历史来源", BaseURL: "https://history.example", Enabled: true}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "历史书", Author: "历史作者", URL: "https://history.example/book"}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatalf("migrate historical candidate volume: %v", err)
	}
	if !database.Migrator().HasTable(&models.BookSourceCandidate{}) {
		t.Fatal("book_source_candidates table was not added")
	}
	var persisted models.Book
	if err := database.First(&persisted, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.UserID != book.UserID || persisted.SourceID != book.SourceID || persisted.Title != book.Title || persisted.URL != book.URL {
		t.Fatalf("candidate migration rewrote historical book: got %+v want %+v", persisted, book)
	}
	var candidateCount int64
	if err := database.Model(&models.BookSourceCandidate{}).Count(&candidateCount).Error; err != nil {
		t.Fatal(err)
	}
	if candidateCount != 0 {
		t.Fatalf("migration eagerly rewrote historical books into %d candidates", candidateCount)
	}
}
