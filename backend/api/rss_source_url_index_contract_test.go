package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/models"
)

var (
	rssSourceURLFilter   = regexp.MustCompile(`\burl = \?`)
	rssSourceURLIndexUse = regexp.MustCompile(`^SEARCH rss_sources USING (COVERING )?INDEX idx_rss_sources_user_url \(user_id=\? AND url=\?\)$`)
)

// TestRSSSourceURLLookupsUseTheUserURLIndex plans every rss_sources query that
// a source write filters by URL, with the SQL, arguments and connection the
// write used. Without the (user_id, url) index each of them reads every row
// of the user, once per imported or restored source.
func TestRSSSourceURLLookupsUseTheUserURLIndex(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)

	var mu sync.Mutex
	step := ""
	plans := map[string][]string{}
	const hook = "openreader:test:rss-source-url-lookup-plans"
	if err := server.db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		mu.Lock()
		defer mu.Unlock()
		if step == "" || tx.Error != nil || tx.Statement.Table != "rss_sources" {
			return
		}
		query := tx.Statement.SQL.String()
		if !rssSourceURLFilter.MatchString(query) {
			return
		}
		rows, err := tx.Statement.ConnPool.QueryContext(tx.Statement.Context, "EXPLAIN QUERY PLAN "+query, tx.Statement.Vars...)
		if err != nil {
			plans[step] = append(plans[step], query+" => EXPLAIN failed: "+err.Error())
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
				plans[step] = append(plans[step], query+" => EXPLAIN scan failed: "+err.Error())
				return
			}
			plans[step] = append(plans[step], query+" => "+detail)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Query().Remove(hook) })

	const feedURL = "https://rss-index.example/feed.xml"
	perform := func(name string, request *http.Request, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		mu.Lock()
		step = name
		mu.Unlock()
		request.Header.Set("Authorization", auth)
		if request.Header.Get("Content-Type") == "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		mu.Lock()
		step = ""
		mu.Unlock()
		if response.Code != wantStatus {
			t.Fatalf("%s: status %d, want %d: %s", name, response.Code, wantStatus, response.Body.String())
		}
		return response
	}

	created := perform("create", httptest.NewRequest(http.MethodPost, "/api/rss/sources",
		strings.NewReader(`{"title":"索引订阅","url":"`+feedURL+`"}`)), http.StatusCreated)
	var source models.RSSSource
	if err := json.Unmarshal(created.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	perform("replace", httptest.NewRequest(http.MethodPost, "/api/rss/sources",
		strings.NewReader(`{"title":"索引订阅替换","url":"`+feedURL+`"}`)), http.StatusOK)
	perform("update", httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/rss/sources/%d", source.ID),
		strings.NewReader(`{"title":"索引订阅改名","url":"`+feedURL+`"}`)), http.StatusOK)
	perform("import", httptest.NewRequest(http.MethodPost, "/api/rss/sources/import",
		strings.NewReader(`[{"sourceName":"索引导入","sourceUrl":"`+feedURL+`"},{"sourceName":"索引导入新增","sourceUrl":"https://rss-index.example/imported.xml"}]`)), http.StatusOK)
	archive := makeBackupRestoreZIP(t, map[string]string{
		"rssSources.json": `[{"sourceName":"索引恢复","sourceUrl":"` + feedURL + `","customOrder":3},{"sourceName":"索引恢复新增","sourceUrl":"https://rss-index.example/restored.xml","customOrder":4}]`,
	})
	perform("restore", multipartBackupRestoreRequest(t, "openreader-backup.zip", archive), http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"create", "replace", "update", "import", "restore"} {
		if len(plans[name]) == 0 {
			t.Fatalf("%s: no rss_sources lookup by URL was captured", name)
		}
		for _, plan := range plans[name] {
			detail := plan[strings.LastIndex(plan, " => ")+len(" => "):]
			if !rssSourceURLIndexUse.MatchString(detail) {
				t.Errorf("%s: %s; want SEARCH rss_sources USING [COVERING] INDEX idx_rss_sources_user_url (user_id=? AND url=?) and nothing else", name, plan)
			}
		}
	}
}
