package db

import (
	"path/filepath"
	"reflect"
	"testing"

	"openreader/backend/config"
)

// The order of a new or restored source without customOrder is the user's
// COALESCE(MAX(custom_order), 0) + 1; the (user_id, custom_order) index lets
// SQLite answer it with one seek instead of reading every row of the user.
func TestAutoMigrateAddsTheRSSSourceUserCustomOrderIndex(t *testing.T) {
	for _, historical := range []bool{false, true} {
		name := "fresh"
		if historical {
			name = "historical"
		}
		t.Run(name, func(t *testing.T) {
			database, err := Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openreader.db")})
			if err != nil {
				t.Fatal(err)
			}
			if historical {
				for _, statement := range legacyRSSSourcesSchema {
					if err := database.Exec(statement).Error; err != nil {
						t.Fatal(err)
					}
				}
				// Duplicate URLs and orders, and a NULL order, as older rows can hold.
				if err := database.Exec("INSERT INTO rss_sources (id, user_id, title, url, custom_order, enabled, created_at, updated_at) VALUES " +
					"(1, 1, '最早的订阅', 'https://history.example/feed.xml', 4, 1, '2025-01-02 03:04:05+00:00', '2025-01-03 03:04:05+00:00')," +
					"(2, 1, '重复的订阅', 'https://history.example/feed.xml', 4, 0, '2025-02-02 03:04:05+00:00', '2025-02-03 03:04:05+00:00')," +
					"(3, 1, '没有排序', 'https://history.example/unordered.xml', NULL, 1, NULL, NULL)," +
					"(4, 2, '其他用户的订阅', 'https://history.example/feed.xml', 9, 1, '2025-03-02 03:04:05+00:00', '2025-03-03 03:04:05+00:00')").Error; err != nil {
					t.Fatal(err)
				}
			}
			var rowsBefore []string
			var tableBefore string
			if historical {
				rowsBefore = storedRSSSourceRows(t, database)
				tableBefore = rssSourcesTableSQL(t, database)
			}

			for run := 1; run <= 2; run++ {
				if err := AutoMigrate(database); err != nil {
					t.Fatalf("migration %d: %v", run, err)
				}
				if historical {
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
				}
				indexes := rssSourceIndexes(t, database)
				want := rssSourceIndex{Columns: []string{"user_id", "custom_order"}}
				if got, ok := indexes["idx_rss_sources_user_custom_order"]; !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("migration %d: rss_sources indexes = %+v, want idx_rss_sources_user_custom_order %+v (non-unique, not partial): the order of each new or restored source otherwise reads every row of the user", run, indexes, want)
				}
				assertRSSSourceUserURLIndex(t, database)
			}
			if historical {
				var maxOrder int
				if err := database.Raw("SELECT COALESCE(MAX(custom_order), 0) FROM rss_sources WHERE user_id = ?", 1).Scan(&maxOrder).Error; err != nil {
					t.Fatal(err)
				}
				if maxOrder != 4 {
					t.Fatalf("user 1 MAX(custom_order) = %d, want 4 with the NULL order ignored", maxOrder)
				}
			}
		})
	}
}
