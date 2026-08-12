package rss

import (
	"fmt"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/models"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rss-service.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RSSSource{}, &models.RSSArticle{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestImportSourcesIsUserScopedAndTransactional(t *testing.T) {
	db := openTestDB(t)
	service := New(db)
	other := models.RSSSource{UserID: 2, Title: "其他用户", URL: "https://rss.example/shared", Enabled: true}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	result, err := service.ImportSources(1, []models.RSSSource{
		{Title: "当前用户", URL: "https://rss.example/shared", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Updated != 0 {
		t.Fatalf("unexpected cross-user import result: %+v", result)
	}
	var preserved models.RSSSource
	if err := db.First(&preserved, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if preserved.Title != "其他用户" || preserved.UserID != 2 {
		t.Fatalf("cross-user source was mutated: %+v", preserved)
	}

	if err := db.Exec(`CREATE TRIGGER fail_rss_import BEFORE INSERT ON rss_sources
		WHEN NEW.url = 'https://rss.example/fail'
		BEGIN SELECT RAISE(ABORT, 'forced RSS import failure'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.ImportSources(1, []models.RSSSource{
		{Title: "必须回滚", URL: "https://rss.example/rollback", Enabled: true},
		{Title: "触发失败", URL: "https://rss.example/fail", Enabled: true},
	})
	if err == nil {
		t.Fatal("expected forced import failure")
	}
	var rollbackCount int64
	if err := db.Model(&models.RSSSource{}).
		Where("user_id = ? AND url = ?", 1, "https://rss.example/rollback").
		Count(&rollbackCount).Error; err != nil {
		t.Fatal(err)
	}
	if rollbackCount != 0 {
		t.Fatalf("partial RSS import survived rollback: %d", rollbackCount)
	}
}

func TestUpsertArticlePageDoesNotOverwriteConcurrentState(t *testing.T) {
	db := openTestDB(t)
	service := New(db)
	source := models.RSSSource{UserID: 1, Title: "RSS", URL: "https://rss.example/feed.xml", Enabled: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{
		UserID: 1, SourceID: source.ID, Sort: "news", Title: "before", Link: "https://rss.example/article/1",
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TRIGGER rss_refresh_concurrent_state
		BEFORE UPDATE OF title ON rss_articles
		WHEN OLD.id = ` + uintStringForRSSServiceTest(article.ID) + `
		BEGIN
			UPDATE rss_articles SET is_read = 1, favorite = 1 WHERE id = OLD.id;
		END;
	`).Error; err != nil {
		t.Fatal(err)
	}

	persisted, _, err := service.UpsertArticlePage(1, source.ID, "news", []models.RSSArticle{
		{Title: "after", Link: article.Link, Summary: "fresh summary"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored models.RSSArticle
	if err := db.First(&stored, article.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "after" || !stored.IsRead || !stored.Favorite {
		t.Fatalf("RSS refresh overwrote concurrent state: %+v", stored)
	}
	if len(persisted) != 1 || !persisted[0].IsRead || !persisted[0].Favorite {
		t.Fatalf("RSS refresh returned stale state: %+v", persisted)
	}
}

func TestUpsertArticlePageRequiresLiveOwnedSource(t *testing.T) {
	db := openTestDB(t)
	service := New(db)
	source := models.RSSSource{UserID: 1, Title: "deleted RSS", URL: "https://rss.example/deleted.xml", Enabled: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&source).Error; err != nil {
		t.Fatal(err)
	}

	_, _, err := service.UpsertArticlePage(1, source.ID, "", []models.RSSArticle{
		{Title: "orphan", Link: "https://rss.example/orphan"},
	})
	if err == nil {
		t.Fatal("RSS refresh accepted a deleted source")
	}
	var count int64
	if err := db.Model(&models.RSSArticle{}).Where("user_id = ? AND source_id = ?", 1, source.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("RSS refresh persisted %d orphan rows", count)
	}
}

func uintStringForRSSServiceTest(value uint) string {
	return fmt.Sprintf("%d", value)
}
