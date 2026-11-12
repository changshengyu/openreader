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
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

var (
	rssSourceOrderFilter   = regexp.MustCompile(`MAX\(custom_order\)`)
	rssSourceOrderIndexUse = regexp.MustCompile(`^SEARCH rss_sources USING COVERING INDEX idx_rss_sources_user_custom_order \(user_id=\?\)$`)
)

func rssSourcesOwner(t *testing.T, server *Server) models.User {
	t.Helper()
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	return owner
}

func restoredRSSEntries(prefix string, count int) string {
	entries := make([]map[string]any, 0, count)
	for i := 1; i <= count; i++ {
		entries = append(entries, map[string]any{
			"sourceName": fmt.Sprintf("恢复的订阅 %d", i),
			"sourceUrl":  fmt.Sprintf("%s/%d.xml", prefix, i),
		})
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

// TestRSSRestoreForALargeUserDoesNotFailAConcurrentWrite restores 5,000 RSS
// entries without customOrder for a user with 40,000 sources. Each such entry
// takes the user's MAX(custom_order) inside the restore's BEGIN IMMEDIATE
// transaction, so its cost is how long every other writer waits at BEGIN.
func TestRSSRestoreForALargeUserDoesNotFailAConcurrentWrite(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	owner := rssSourcesOwner(t, server)
	var busyTimeoutMilliseconds int64
	if err := server.db.Raw("PRAGMA busy_timeout").Scan(&busyTimeoutMilliseconds).Error; err != nil {
		t.Fatal(err)
	}
	busyTimeout := time.Duration(busyTimeoutMilliseconds) * time.Millisecond
	if busyTimeout <= 0 {
		t.Fatalf("busy_timeout = %s, want the application's positive wait", busyTimeout)
	}

	const existing, restored = 40000, 5000
	if err := server.db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO rss_sources (user_id, title, url, custom_order, enabled, created_at, updated_at)
		SELECT ?, '已有订阅 ' || i, 'https://rss-restore-large.example/existing/' || i || '.xml', i, 1, datetime('now'), datetime('now') FROM n`,
		existing, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	other := models.User{Username: "rss-restore-other", PasswordHash: "hash"}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherSource := models.RSSSource{UserID: other.ID, Title: "另一用户的订阅", URL: "https://rss-restore-large.example/other.xml", Enabled: true}
	if err := server.db.Create(&otherSource).Error; err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"rssSources.json": restoredRSSEntries("https://rss-restore-large.example/restored", restored),
	})

	// The restore's first rss_sources query runs inside its transaction,
	// which holds SQLite's write lock from BEGIN IMMEDIATE on; the other
	// user's write starts then and waits at its own BEGIN.
	restoreHoldsWriteLock := make(chan struct{})
	var signal sync.Once
	const hook = "openreader:test:rss-restore-holds-write-lock"
	if err := server.db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "rss_sources" && restoreSourceWriteInTransaction(tx) {
			signal.Do(func() { close(restoreHoldsWriteLock) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Query().Remove(hook) })
	type writeOutcome struct {
		err    error
		waited time.Duration
	}
	written := make(chan writeOutcome, 1)
	go func() {
		<-restoreHoldsWriteLock
		start := time.Now()
		err := server.db.Model(&models.RSSSource{}).
			Where("user_id = ? AND id = ?", other.ID, otherSource.ID).
			Update("title", "并发改名").Error
		written <- writeOutcome{err: err, waited: time.Since(start)}
	}()

	request := multipartBackupRestoreRequest(t, "openreader-backup.zip", archive)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	start := time.Now()
	router.ServeHTTP(response, request)
	held := time.Since(start)
	if response.Code != http.StatusOK {
		t.Fatalf("restore: status %d: %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if got, ok := result["rssSources"].(float64); !ok || int(got) != restored {
		t.Fatalf("restore result = %s, want %d RSS sources", response.Body.String(), restored)
	}
	var outcome writeOutcome
	select {
	case outcome = <-written:
	case <-time.After(busyTimeout + 10*time.Second):
		t.Fatal("another user's write did not return")
	}
	if outcome.err != nil || held >= busyTimeout {
		t.Fatalf("restoring %d RSS entries without customOrder for a user with %d sources took %s (busy_timeout %s); another user's write that waited for it returned after %s with %v; want the restore within busy_timeout and the other write to succeed",
			restored, existing, held.Round(time.Millisecond), busyTimeout, outcome.waited.Round(time.Millisecond), outcome.err)
	}
	var orders []int
	if err := server.db.Model(&models.RSSSource{}).
		Where("user_id = ? AND url IN ?", owner.ID, []string{"https://rss-restore-large.example/restored/1.xml", fmt.Sprintf("https://rss-restore-large.example/restored/%d.xml", restored)}).
		Order("id asc").Pluck("custom_order", &orders).Error; err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 || orders[0] != existing+1 || orders[1] != existing+restored {
		t.Fatalf("restored orders = %v, want %d and %d", orders, existing+1, existing+restored)
	}
}

// TestRSSSourceOrderLookupsUseTheUserCustomOrderIndex plans the
// MAX(custom_order) query of a new source, an import and every restored
// entry without customOrder, with the SQL, arguments and connection used.
func TestRSSSourceOrderLookupsUseTheUserCustomOrderIndex(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)

	var mu sync.Mutex
	step := ""
	plans := map[string][]string{}
	// Select(...).Scan(...) runs through gorm's Row callbacks, Find and
	// Pluck through its Query callbacks; capture both.
	capture := func(tx *gorm.DB) {
		mu.Lock()
		defer mu.Unlock()
		if step == "" || tx.Error != nil || tx.Statement.Table != "rss_sources" {
			return
		}
		query := tx.Statement.SQL.String()
		if !rssSourceOrderFilter.MatchString(query) {
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
	}
	const queryHook, rowHook = "openreader:test:rss-source-order-query-plans", "openreader:test:rss-source-order-row-plans"
	if err := server.db.Callback().Query().After("gorm:query").Register(queryHook, capture); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Query().Remove(queryHook) })
	if err := server.db.Callback().Row().After("gorm:row").Register(rowHook, capture); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Row().Remove(rowHook) })

	perform := func(name string, request *http.Request, wantStatus int) {
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
	}

	perform("create", httptest.NewRequest(http.MethodPost, "/api/rss/sources",
		strings.NewReader(`{"title":"排序订阅","url":"https://rss-order.example/feed.xml"}`)), http.StatusCreated)
	perform("import", httptest.NewRequest(http.MethodPost, "/api/rss/sources/import",
		strings.NewReader(`[{"sourceName":"排序导入","sourceUrl":"https://rss-order.example/imported.xml"}]`)), http.StatusOK)
	archive := makeBackupRestoreZIP(t, map[string]string{
		"rssSources.json": `[{"sourceName":"排序恢复","sourceUrl":"https://rss-order.example/restored.xml"},{"sourceName":"排序恢复二","sourceUrl":"https://rss-order.example/restored-2.xml"}]`,
	})
	perform("restore", multipartBackupRestoreRequest(t, "openreader-backup.zip", archive), http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"create", "import", "restore"} {
		if len(plans[name]) == 0 {
			t.Fatalf("%s: no MAX(custom_order) query was captured", name)
		}
		for _, plan := range plans[name] {
			detail := plan[strings.LastIndex(plan, " => ")+len(" => "):]
			if !rssSourceOrderIndexUse.MatchString(detail) {
				t.Errorf("%s: %s; want SEARCH rss_sources USING COVERING INDEX idx_rss_sources_user_custom_order (user_id=?) and nothing else", name, plan)
			}
		}
	}
}

// TestRSSRestoreKeepsItsCustomOrderSequence pins how a restore numbers its
// entries: a positive customOrder is kept, any other entry takes the user's
// MAX(custom_order) + 1 as it stands after the entries before it, and an entry
// for a URL the user already has rewrites that row's order.
func TestRSSRestoreKeepsItsCustomOrderSequence(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	owner := rssSourcesOwner(t, server)
	const base = "https://rss-order-sequence.example/"
	for _, source := range []models.RSSSource{
		{UserID: owner.ID, Title: "A", URL: base + "a.xml", CustomOrder: 5, Enabled: true},
		{UserID: owner.ID, Title: "B", URL: base + "b.xml", CustomOrder: 9, Enabled: true},
		{UserID: owner.ID, Title: "C", URL: base + "c.xml", CustomOrder: 2, Enabled: true},
	} {
		if err := server.db.Create(&source).Error; err != nil {
			t.Fatal(err)
		}
	}
	other := models.User{Username: "rss-order-other", PasswordHash: "hash"}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.RSSSource{UserID: other.ID, Title: "其他用户", URL: base + "other.xml", CustomOrder: 100, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	// B drops from the maximum 9 to 1, so D follows A's 5; A then follows D;
	// E keeps its 3; F follows A; the second F entry follows F itself.
	archive := makeBackupRestoreZIP(t, map[string]string{
		"rssSources.json": `[` +
			`{"sourceName":"B","sourceUrl":"` + base + `b.xml","customOrder":1},` +
			`{"sourceName":"D","sourceUrl":"` + base + `d.xml"},` +
			`{"sourceName":"A","sourceUrl":"` + base + `a.xml"},` +
			`{"sourceName":"E","sourceUrl":"` + base + `e.xml","customOrder":3},` +
			`{"sourceName":"F","sourceUrl":"` + base + `f.xml","customOrder":0},` +
			`{"sourceName":"F","sourceUrl":"` + base + `f.xml"}` +
			`]`,
	})
	request := multipartBackupRestoreRequest(t, "openreader-backup.zip", archive)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("restore: status %d: %s", response.Code, response.Body.String())
	}

	var sources []models.RSSSource
	if err := server.db.Where("user_id = ?", owner.ID).Order("id asc").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]int, len(sources))
	for _, source := range sources {
		got[strings.TrimPrefix(source.URL, base)] = source.CustomOrder
	}
	want := map[string]int{"a.xml": 7, "b.xml": 1, "c.xml": 2, "d.xml": 6, "e.xml": 3, "f.xml": 9}
	if len(sources) != len(want) || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("restored orders = %v (%d rows), want %v", got, len(sources), want)
	}
	var otherOrder int
	if err := server.db.Model(&models.RSSSource{}).Where("user_id = ?", other.ID).Pluck("custom_order", &otherOrder).Error; err != nil {
		t.Fatal(err)
	}
	if otherOrder != 100 {
		t.Fatalf("another user's order changed to %d", otherOrder)
	}
}
