package db

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/config"
)

// legacyRSSSourcesSchema is the rss_sources table and index exactly as
// AutoMigrate wrote them up to 752aa4e, before idx_rss_sources_user_url.
var legacyRSSSourcesSchema = []string{
	"CREATE TABLE `rss_sources` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer NOT NULL,`title` text NOT NULL,`url` text NOT NULL,`icon` text,`group` text,`comment` text,`custom_order` integer DEFAULT 0,`concurrent_rate` text,`header` text,`login_url` text,`login_check_js` text,`single_url` numeric,`article_style` integer,`sort_url` text,`rule_articles` text,`rule_next_page` text,`rule_title` text,`rule_pub_date` text,`rule_description` text,`rule_image` text,`rule_link` text,`rule_content` text,`style` text,`enable_js` numeric,`load_with_base_url` numeric,`enabled` numeric,`created_at` datetime,`updated_at` datetime)",
	"CREATE INDEX `idx_rss_sources_user_id` ON `rss_sources`(`user_id`)",
}

type rssSourceIndex struct {
	Unique  bool
	Partial bool
	Columns []string
}

func rssSourceIndexes(t *testing.T, database *gorm.DB) map[string]rssSourceIndex {
	t.Helper()
	var listed []struct {
		Name    string
		Unique  int
		Partial int
	}
	if err := database.Raw("SELECT name, `unique`, partial FROM pragma_index_list('rss_sources')").Scan(&listed).Error; err != nil {
		t.Fatal(err)
	}
	indexes := make(map[string]rssSourceIndex, len(listed))
	for _, index := range listed {
		var columns []string
		if err := database.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", index.Name).Scan(&columns).Error; err != nil {
			t.Fatal(err)
		}
		indexes[index.Name] = rssSourceIndex{Unique: index.Unique != 0, Partial: index.Partial != 0, Columns: columns}
	}
	return indexes
}

// assertRSSSourceUserURLIndex requires the non-unique (user_id, url) index
// that the source writes look URLs up with, next to the user_id index.
func assertRSSSourceUserURLIndex(t *testing.T, database *gorm.DB) {
	t.Helper()
	indexes := rssSourceIndexes(t, database)
	want := rssSourceIndex{Columns: []string{"user_id", "url"}}
	if got, ok := indexes["idx_rss_sources_user_url"]; !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("rss_sources indexes = %+v, want idx_rss_sources_user_url %+v (non-unique, not partial): every URL lookup of a source write otherwise scans all of the user's rows", indexes, want)
	}
	if got, ok := indexes["idx_rss_sources_user_id"]; !ok || !reflect.DeepEqual(got, rssSourceIndex{Columns: []string{"user_id"}}) {
		t.Fatalf("rss_sources indexes = %+v, want idx_rss_sources_user_id on user_id kept", indexes)
	}
}

// storedRSSSourceRows renders every stored value with quote(), so a
// comparison sees storage classes and bytes, not driver conversions.
func storedRSSSourceRows(t *testing.T, database *gorm.DB) []string {
	t.Helper()
	var columns []string
	if err := database.Raw("SELECT name FROM pragma_table_info('rss_sources') ORDER BY cid").Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = "quote(`" + column + "`)"
	}
	var rows []string
	if err := database.Raw("SELECT " + strings.Join(quoted, " || ',' || ") + " FROM rss_sources ORDER BY id").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func rssSourcesTableSQL(t *testing.T, database *gorm.DB) string {
	t.Helper()
	var statement string
	if err := database.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'rss_sources'").Scan(&statement).Error; err != nil {
		t.Fatal(err)
	}
	return statement
}

func TestAutoMigrateAddsTheRSSSourceUserURLIndex(t *testing.T) {
	database, err := Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	assertRSSSourceUserURLIndex(t, database)
}

func TestAutoMigrateIndexesHistoricalRSSSourcesWithDuplicateURLs(t *testing.T) {
	database, err := Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db")})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range legacyRSSSourcesSchema {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Rows 1 and 2 are one user's two rows for one URL, which earlier
	// releases could store; row 3 is another user's row for the same URL.
	if err := database.Exec("INSERT INTO rss_sources (id, user_id, title, url, `group`, custom_order, header, single_url, article_style, rule_content, enable_js, load_with_base_url, enabled, created_at, updated_at) VALUES " +
		"(1, 1, '最早的订阅', 'https://history.example/feed.xml', '历史', 2, '{\"User-Agent\":\"legacy\"}', 1, 0, 'div.content', 1, 0, 1, '2025-01-02 03:04:05.123456+00:00', '2025-01-03 03:04:05+00:00')," +
		"(2, 1, '重复的订阅', 'https://history.example/feed.xml', '', 5, '', 0, 2, '', 0, 1, 0, '2025-02-02 03:04:05+00:00', '2025-02-03 03:04:05+00:00')," +
		"(3, 2, '其他用户的订阅', 'https://history.example/feed.xml', '', 1, NULL, 0, 0, NULL, 0, 0, 1, '2025-03-02 03:04:05+00:00', '2025-03-03 03:04:05+00:00')," +
		"(4, 1, '另一个订阅', 'https://history.example/other.xml', '', 3, '', 0, 0, '', 0, 0, 1, '2025-04-02 03:04:05+00:00', '2025-04-03 03:04:05+00:00')").Error; err != nil {
		t.Fatal(err)
	}
	rowsBefore := storedRSSSourceRows(t, database)
	tableBefore := rssSourcesTableSQL(t, database)

	for run := 1; run <= 2; run++ {
		if err := AutoMigrate(database); err != nil {
			t.Fatalf("migration %d of a historical database with duplicate RSS URLs: %v", run, err)
		}
		if rows := storedRSSSourceRows(t, database); !reflect.DeepEqual(rows, rowsBefore) {
			t.Fatalf("migration %d changed historical RSS rows:\n got %q\nwant %q", run, rows, rowsBefore)
		}
		if table := rssSourcesTableSQL(t, database); table != tableBefore {
			t.Fatalf("migration %d rebuilt rss_sources:\n got %s\nwant %s", run, table, tableBefore)
		}
		var integrity string
		if err := database.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
			t.Fatal(err)
		}
		if integrity != "ok" {
			t.Fatalf("integrity_check after migration %d = %q", run, integrity)
		}
		assertRSSSourceUserURLIndex(t, database)
	}
}
