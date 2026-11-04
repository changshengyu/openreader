package rss

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// immediateRSSDatabase opens the application database through db.Open, so
// every transaction begins with BEGIN IMMEDIATE, plus a second connection with
// busy_timeout 0. The second connection stands in for another user's request:
// its write fails at once, instead of waiting, while an RSS transaction holds
// SQLite's write lock.
func immediateRSSDatabase(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "openreader.db")
	database, err := readerdb.Open(config.Config{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite3", path+"?_busy_timeout=0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	return database, other
}

func isSQLiteBusy(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError) && sqliteError.Code == sqlite3.ErrBusy
}

func inRSSTransaction(tx *gorm.DB) bool {
	committer, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	return ok && committer != nil
}

func afterRSSQuery(t *testing.T, database *gorm.DB, name string, hook func(*gorm.DB)) {
	t.Helper()
	if err := database.Callback().Query().After("gorm:query").Register(name, hook); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(name) })
}

func TestRSSWritesHoldTheWriteLockAgainstAConcurrentCommit(t *testing.T) {
	type fixture struct {
		database *gorm.DB
		service  *Service
		source   models.RSSSource
		article  models.RSSArticle
	}
	writes := []struct {
		name  string
		write func(fixture) error
		check func(fixture) error
	}{
		{
			name: "create",
			write: func(f fixture) error {
				_, created, err := f.service.CreateOrReplaceSource(1, SourceInput{Source: models.RSSSource{Title: "新订阅", URL: "https://rss.example/new.xml"}})
				if err == nil && !created {
					return errors.New("a new URL was not created")
				}
				return err
			},
			check: func(f fixture) error {
				return f.database.Where("user_id = ? AND url = ?", 1, "https://rss.example/new.xml").First(&models.RSSSource{}).Error
			},
		},
		{
			name: "update",
			write: func(f fixture) error {
				_, err := f.service.UpdateSource(1, f.source.ID, SourceInput{Source: models.RSSSource{Title: "改名订阅", URL: f.source.URL}})
				return err
			},
			check: func(f fixture) error {
				var source models.RSSSource
				if err := f.database.First(&source, f.source.ID).Error; err != nil {
					return err
				}
				if source.Title != "改名订阅" {
					return fmt.Errorf("title = %q, want 改名订阅", source.Title)
				}
				return nil
			},
		},
		{
			name: "import",
			write: func(f fixture) error {
				result, err := f.service.ImportSources(1, []models.RSSSource{
					{Title: "导入更新", URL: f.source.URL, Enabled: true},
					{Title: "导入新增", URL: "https://rss.example/imported.xml", Enabled: true},
				})
				if err == nil && (result.Created != 1 || result.Updated != 1) {
					return fmt.Errorf("import result = %+v, want 1 created and 1 updated", result)
				}
				return err
			},
			check: func(f fixture) error {
				var count int64
				if err := f.database.Model(&models.RSSSource{}).Where("user_id = ?", 1).Count(&count).Error; err != nil {
					return err
				}
				if count != 2 {
					return fmt.Errorf("user 1 sources = %d, want 2", count)
				}
				return nil
			},
		},
		{
			name: "delete",
			write: func(f fixture) error {
				return f.service.DeleteSource(1, f.source.ID)
			},
			check: func(f fixture) error {
				var sources, articles int64
				if err := f.database.Model(&models.RSSSource{}).Where("id = ?", f.source.ID).Count(&sources).Error; err != nil {
					return err
				}
				if err := f.database.Model(&models.RSSArticle{}).Where("source_id = ?", f.source.ID).Count(&articles).Error; err != nil {
					return err
				}
				if sources != 0 || articles != 0 {
					return fmt.Errorf("deleted source left %d source and %d article rows", sources, articles)
				}
				return nil
			},
		},
		{
			name: "refresh-articles",
			write: func(f fixture) error {
				_, created, err := f.service.UpsertArticlePage(1, f.source.ID, "news", []models.RSSArticle{
					{Title: "更新文章", Link: f.article.Link},
					{Title: "新文章", Link: "https://rss.example/article/2"},
				})
				if err == nil && created != 1 {
					return fmt.Errorf("created articles = %d, want 1", created)
				}
				return err
			},
			check: func(f fixture) error {
				var article models.RSSArticle
				if err := f.database.First(&article, f.article.ID).Error; err != nil {
					return err
				}
				if article.Title != "更新文章" {
					return fmt.Errorf("refreshed title = %q, want 更新文章", article.Title)
				}
				return nil
			},
		},
		{
			name: "article-content",
			write: func(f fixture) error {
				content := "<p>正文</p>"
				_, err := f.service.CommitArticleContent(1, f.source.ID, f.article.ID, &content)
				return err
			},
			check: func(f fixture) error {
				var article models.RSSArticle
				if err := f.database.First(&article, f.article.ID).Error; err != nil {
					return err
				}
				if article.Content != "<p>正文</p>" {
					return fmt.Errorf("content = %q, want <p>正文</p>", article.Content)
				}
				return nil
			},
		},
	}

	for _, write := range writes {
		t.Run(write.name, func(t *testing.T) {
			database, other := immediateRSSDatabase(t)
			source := models.RSSSource{UserID: 1, Title: "订阅", URL: "https://rss.example/feed.xml", Enabled: true}
			neighbour := models.RSSSource{UserID: 2, Title: "邻居订阅", URL: "https://rss.example/neighbour.xml", Enabled: true}
			if err := database.Create(&[]*models.RSSSource{&source, &neighbour}).Error; err != nil {
				t.Fatal(err)
			}
			article := models.RSSArticle{UserID: 1, SourceID: source.ID, Sort: "news", Title: "文章", Link: "https://rss.example/article/1"}
			if err := database.Create(&article).Error; err != nil {
				t.Fatal(err)
			}

			// Another user's RSS write commits between this write's first read
			// and its first write.
			renameNeighbour := func() error {
				_, err := other.Exec("UPDATE rss_sources SET title = ? WHERE id = ?", "邻居并发改名", neighbour.ID)
				return err
			}
			var concurrentErr error
			var intercepted atomic.Bool
			afterRSSQuery(t, database, "openreader:test:rss-concurrent-commit", func(tx *gorm.DB) {
				if inRSSTransaction(tx) && intercepted.CompareAndSwap(false, true) {
					concurrentErr = renameNeighbour()
				}
			})

			f := fixture{database: database, service: New(database), source: source, article: article}
			if err := write.write(f); err != nil {
				t.Fatalf("%s failed once another user's write tried to commit between its read and its write: %v (concurrent commit error: %v)", write.name, err, concurrentErr)
			}
			if !isSQLiteBusy(concurrentErr) {
				t.Fatalf("another user's write while the %s transaction was open = %v, want SQLITE_BUSY until it commits", write.name, concurrentErr)
			}
			if err := write.check(f); err != nil {
				t.Fatalf("%s result: %v", write.name, err)
			}
			if err := renameNeighbour(); err != nil {
				t.Fatalf("the waiting write after %s committed: %v", write.name, err)
			}
			var stored models.RSSSource
			if err := database.First(&stored, neighbour.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.Title != "邻居并发改名" {
				t.Fatalf("neighbour title = %q, want 邻居并发改名", stored.Title)
			}
		})
	}
}

func TestConcurrentRSSWritesFromSeveralUsersAllCommit(t *testing.T) {
	database, _ := immediateRSSDatabase(t)
	service := New(database)

	// Hold the first two transactions that read a source together. Two
	// deferred transactions would both read there before either writes; a
	// transaction that holds the write lock from BEGIN keeps the second one
	// out, so the first goes on alone after the timeout.
	var arrivals atomic.Int32
	partner := make(chan struct{})
	afterRSSQuery(t, database, "openreader:test:rss-concurrent-users", func(tx *gorm.DB) {
		if tx.Statement.Table != "rss_sources" || !inRSSTransaction(tx) {
			return
		}
		switch arrivals.Add(1) {
		case 1:
			select {
			case <-partner:
			case <-time.After(200 * time.Millisecond):
			}
		case 2:
			close(partner)
		}
	})

	const users = 4
	start := make(chan struct{})
	failures := make(chan error, users*3)
	var requests sync.WaitGroup
	for userID := uint(1); userID <= users; userID++ {
		// Two requests from the same user add the same feed at once.
		for range 2 {
			requests.Add(1)
			go func() {
				defer requests.Done()
				<-start
				if _, _, err := service.CreateOrReplaceSource(userID, SourceInput{Source: models.RSSSource{Title: "同址订阅", URL: "https://rss.example/shared.xml"}}); err != nil {
					failures <- fmt.Errorf("user %d create-or-replace: %w", userID, err)
				}
			}()
		}
		requests.Add(1)
		go func() {
			defer requests.Done()
			<-start
			if err := exerciseRSSWrites(service, userID); err != nil {
				failures <- fmt.Errorf("user %d %w", userID, err)
			}
		}()
	}
	close(start)
	requests.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent RSS write failed: %v", err)
	}

	for userID := uint(1); userID <= users; userID++ {
		var sources []models.RSSSource
		if err := database.Where("user_id = ?", userID).Order("url asc").Find(&sources).Error; err != nil {
			t.Fatal(err)
		}
		summary := make([]string, 0, len(sources))
		for _, source := range sources {
			summary = append(summary, source.URL+" "+source.Title)
		}
		if len(sources) != 2 || sources[0].URL != "https://rss.example/kept.xml" || sources[0].Title != "改名订阅" || sources[1].URL != "https://rss.example/shared.xml" {
			t.Errorf("user %d sources = %q, want the renamed kept feed and one shared feed", userID, summary)
			continue
		}
		var articles []models.RSSArticle
		if err := database.Where("user_id = ? AND source_id = ?", userID, sources[0].ID).Order("id asc").Find(&articles).Error; err != nil {
			t.Fatal(err)
		}
		if len(articles) != 2 || articles[0].Content != "<p>正文</p>" {
			t.Errorf("user %d has %d articles on the kept feed, want 2 with the first one's content saved", userID, len(articles))
		}
	}
}

// exerciseRSSWrites runs every source and article write once for one user:
// import two feeds, rename one, refresh its articles, save one article's
// content, then delete the other feed.
func exerciseRSSWrites(service *Service, userID uint) error {
	result, err := service.ImportSources(userID, []models.RSSSource{
		{Title: "保留订阅", URL: "https://rss.example/kept.xml", Enabled: true},
		{Title: "待删订阅", URL: "https://rss.example/removed.xml", Enabled: true},
	})
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if result.Created != 2 {
		return fmt.Errorf("import result = %+v, want 2 created", result)
	}
	var kept, removed models.RSSSource
	if err := service.db.Where("user_id = ? AND url = ?", userID, "https://rss.example/kept.xml").First(&kept).Error; err != nil {
		return err
	}
	if err := service.db.Where("user_id = ? AND url = ?", userID, "https://rss.example/removed.xml").First(&removed).Error; err != nil {
		return err
	}
	if _, err := service.UpdateSource(userID, kept.ID, SourceInput{Source: models.RSSSource{Title: "改名订阅", URL: kept.URL}}); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	articles, _, err := service.UpsertArticlePage(userID, kept.ID, "news", []models.RSSArticle{
		{Title: "文章一", Link: "https://rss.example/article/1"},
		{Title: "文章二", Link: "https://rss.example/article/2"},
	})
	if err != nil {
		return fmt.Errorf("refresh articles: %w", err)
	}
	content := "<p>正文</p>"
	if _, err := service.CommitArticleContent(userID, kept.ID, articles[0].ID, &content); err != nil {
		return fmt.Errorf("article content: %w", err)
	}
	if err := service.DeleteSource(userID, removed.ID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}
