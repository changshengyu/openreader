package sourcecandidates

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

type candidateRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn candidateRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestMergeBoundsRowsAndRetainsCurrentCandidate(t *testing.T) {
	database := candidateServiceDatabase(t)
	service := New(database)
	book := models.Book{
		ID: 1, UserID: 7, SourceID: 99, Title: "上限书", Author: "作者",
		URL: "https://current.example/book",
	}
	rows := make([]models.BookSourceCandidate, 0, MaxCandidatesPerBook+6)
	for index := 0; index < MaxCandidatesPerBook+5; index++ {
		rows = append(rows, models.BookSourceCandidate{
			SourceID: uint(index + 1), Title: book.Title, Author: book.Author,
			BookURL: "https://candidate.example/" + strconv.Itoa(index),
		})
	}
	rows = append(rows, CandidateFromBook(book, nil))

	stored, err := service.Merge(book.UserID, book, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != MaxCandidatesPerBook {
		t.Fatalf("candidate cap = %d, want %d", len(stored), MaxCandidatesPerBook)
	}
	foundCurrent := false
	for _, row := range stored {
		if row.SourceID == book.SourceID && row.BookURL == book.URL {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatal("candidate pruning removed the current source")
	}
	if stored[0].BookURL == "https://candidate.example/0" {
		t.Fatalf("candidate pruning did not remove stable oldest rows: %+v", stored[0])
	}

	stored, err = service.Merge(book.UserID, book, []models.BookSourceCandidate{
		{SourceID: 100, Title: book.Title, Author: book.Author, BookURL: book.URL, SourceName: "更新名称"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != MaxCandidatesPerBook {
		t.Fatalf("duplicate URL created another candidate row: %d", len(stored))
	}
	for _, row := range stored {
		if row.BookURL == book.URL && row.SourceID != book.SourceID {
			t.Fatalf("same-URL remote result replaced the authoritative current source: %+v", row)
		}
	}
}

func TestSearchHonorsCanceledContextWithoutRemoteWork(t *testing.T) {
	database := candidateServiceDatabase(t)
	service := New(database)
	book := models.Book{ID: 1, UserID: 3, Title: "取消书", Author: "作者"}
	source := models.BookSource{ID: 1, Name: "取消源", BaseURL: "https://cancel.example", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL: "https://cancel.example/search?q={keyword}", BookListRule: ".book",
		BookNameRule: ".title", BookAuthorRule: ".author", BookURLRule: "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	remoteCalls := 0
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: candidateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		remoteCalls++
		return nil, errors.New("canceled search reached transport")
	})})
	defer restore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	batch := service.Search(ctx, book, []models.BookSource{source}, nil, 0, 10)

	if remoteCalls != 0 {
		t.Fatalf("canceled candidate search made %d remote requests", remoteCalls)
	}
	if len(batch.Candidates) != 0 || batch.NextOffset != 0 || !batch.HasMore {
		t.Fatalf("unexpected canceled search projection: %+v", batch)
	}
}

func candidateServiceDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "candidates.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.BookSourceCandidate{}); err != nil {
		t.Fatal(err)
	}
	return database
}
