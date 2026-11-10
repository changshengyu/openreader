package rss

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

func seedRSSSourcesForUser(t *testing.T, database *gorm.DB, userID uint, count int) {
	t.Helper()
	if err := database.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO rss_sources (user_id, title, url, custom_order, enabled, created_at, updated_at)
		SELECT ?, '已有订阅 ' || i, 'https://rss-large.example/' || ? || '/' || i || '.xml', i, 1, datetime('now'), datetime('now') FROM n`,
		count, userID, userID).Error; err != nil {
		t.Fatal(err)
	}
}

// A maximal import is one BEGIN IMMEDIATE transaction, so its duration is how
// long every other writer waits at BEGIN. It must not grow with the sources
// the user already has until another writer's busy_timeout runs out.
func TestMaximalRSSImportForALargeUserDoesNotFailAConcurrentWrite(t *testing.T) {
	database, _ := immediateRSSDatabase(t)
	var busyTimeoutMilliseconds int64
	if err := database.Raw("PRAGMA busy_timeout").Scan(&busyTimeoutMilliseconds).Error; err != nil {
		t.Fatal(err)
	}
	busyTimeout := time.Duration(busyTimeoutMilliseconds) * time.Millisecond
	if busyTimeout <= 0 {
		t.Fatalf("busy_timeout = %s, want the application's positive wait", busyTimeout)
	}

	const existing = 40000
	seedRSSSourcesForUser(t, database, 1, existing)
	other := models.RSSSource{UserID: 2, Title: "另一用户的订阅", URL: "https://rss-large.example/other.xml", Enabled: true}
	if err := database.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	candidates := make([]models.RSSSource, 0, MaxImportSources)
	for i := 1; i <= MaxImportSources; i++ {
		candidates = append(candidates, models.RSSSource{
			Title:   fmt.Sprintf("导入订阅 %d", i),
			URL:     fmt.Sprintf("https://rss-large.example/imported/%d.xml", i),
			Enabled: true,
		})
	}

	// The import's first query runs inside its transaction, which holds
	// SQLite's write lock from BEGIN IMMEDIATE on; the other user's write
	// starts then and waits at its own BEGIN.
	importHoldsWriteLock := make(chan struct{})
	var signal sync.Once
	afterRSSQuery(t, database, "test:rss-maximal-import-holds-write-lock", func(tx *gorm.DB) {
		if tx.Statement.Table == "rss_sources" && inRSSTransaction(tx) {
			signal.Do(func() { close(importHoldsWriteLock) })
		}
	})
	type writeOutcome struct {
		err    error
		waited time.Duration
	}
	written := make(chan writeOutcome, 1)
	go func() {
		<-importHoldsWriteLock
		start := time.Now()
		err := database.Model(&models.RSSSource{}).
			Where("user_id = ? AND id = ?", other.UserID, other.ID).
			Update("title", "并发改名").Error
		written <- writeOutcome{err: err, waited: time.Since(start)}
	}()

	start := time.Now()
	result, err := New(database).ImportSources(1, candidates)
	held := time.Since(start)
	if err != nil || result.Created != MaxImportSources {
		t.Fatalf("import = %+v, %v; want %d created", result, err, MaxImportSources)
	}
	var outcome writeOutcome
	select {
	case outcome = <-written:
	case <-time.After(busyTimeout + 10*time.Second):
		t.Fatal("another user's write did not return")
	}
	if outcome.err != nil || held >= busyTimeout {
		t.Fatalf("a %d-source import for a user with %d sources held the write lock for %s (busy_timeout %s); another user's write that waited for it returned after %s with %v; want the import to commit within busy_timeout and the other write to succeed",
			MaxImportSources, existing, held.Round(time.Millisecond), busyTimeout, outcome.waited.Round(time.Millisecond), outcome.err)
	}
	var renamed models.RSSSource
	if err := database.First(&renamed, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "并发改名" {
		t.Fatalf("another user's write was lost: %+v", renamed)
	}
}

// Historical databases can hold several rows of one user for one URL. The
// index must not change which row a write finds or what it may do with the
// others.
func TestRSSWritesKeepHistoricalDuplicateURLBehavior(t *testing.T) {
	database, _ := immediateRSSDatabase(t)
	if err := database.Exec("DROP INDEX IF EXISTS idx_rss_sources_user_url").Error; err != nil {
		t.Fatal(err)
	}
	const shared = "https://rss-history.example/feed.xml"
	if err := database.Exec(`INSERT INTO rss_sources (id, user_id, title, url, custom_order, enabled, created_at, updated_at) VALUES
		(11, 1, '最早的订阅', ?, 3, 1, datetime('now'), datetime('now')),
		(12, 1, '重复的订阅', ?, 7, 1, datetime('now'), datetime('now')),
		(13, 2, '其他用户的订阅', ?, 1, 1, datetime('now'), datetime('now')),
		(14, 1, '另一个订阅', 'https://rss-history.example/another.xml', 9, 1, datetime('now'), datetime('now'))`,
		shared, shared, shared).Error; err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatalf("upgrade a database with duplicate RSS URLs: %v", err)
	}
	service := New(database)
	title := func(id uint) string {
		t.Helper()
		var source models.RSSSource
		if err := database.First(&source, id).Error; err != nil {
			t.Fatalf("source %d: %v", id, err)
		}
		return source.Title
	}

	saved, created, err := service.CreateOrReplaceSource(1, SourceInput{Source: models.RSSSource{Title: "单条保存", URL: " " + shared + " "}})
	if err != nil || created || saved.ID != 11 || saved.Title != "单条保存" {
		t.Fatalf("same-URL save = %+v created=%v err=%v, want the earliest row 11 replaced", saved, created, err)
	}
	result, err := service.ImportSources(1, []models.RSSSource{
		{Title: "导入改写", URL: shared, Enabled: true},
		{Title: "导入新增", URL: "https://rss-history.example/imported.xml", Enabled: true},
	})
	if err != nil || result.Created != 1 || result.Updated != 1 {
		t.Fatalf("import = %+v, %v; want 1 created and 1 updated", result, err)
	}
	if got := title(11); got != "导入改写" {
		t.Fatalf("import rewrote %q into the earliest row, want 导入改写", got)
	}
	if got := title(12); got != "重复的订阅" {
		t.Fatalf("a write touched the later duplicate: title %q", got)
	}
	for _, id := range []uint{11, 12} {
		if _, err := service.UpdateSource(1, id, SourceInput{Source: models.RSSSource{Title: "改名", URL: shared}}); !errors.Is(err, ErrSourceURLConflict) {
			t.Fatalf("update of duplicate %d keeping its URL = %v, want ErrSourceURLConflict", id, err)
		}
	}
	if err := service.DeleteSource(1, 12); err != nil {
		t.Fatalf("delete a duplicate: %v", err)
	}
	if updated, err := service.UpdateSource(1, 11, SourceInput{Source: models.RSSSource{Title: "删除重复后改名", URL: shared}}); err != nil || updated.Title != "删除重复后改名" {
		t.Fatalf("update after the duplicate was deleted = %+v, %v", updated, err)
	}
	var ids []uint
	if err := database.Model(&models.RSSSource{}).Where("user_id = ?", 1).Order("id asc").Pluck("id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 11 || ids[1] != 14 {
		t.Fatalf("user 1 sources = %v, want 11, 14 and the imported one", ids)
	}
	if got := title(13); got != "其他用户的订阅" {
		t.Fatalf("another user's row for the same URL changed: title %q", got)
	}
}
