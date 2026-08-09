package rss

import (
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
