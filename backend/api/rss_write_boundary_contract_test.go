package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	testRSSSourceWriteBodyLimit  = 8 << 20
	testRSSArticleWriteBodyLimit = 16 << 10
	testRSSImportItemLimit       = 5000
)

type rssWriteBoundaryFixture struct {
	router *gin.Engine
	server *Server
	auth   string
	user   models.User
	method string
	path   string
	body   string
	limit  int
	before []byte
	events <-chan []byte
}

func newRSSWriteBoundaryFixture(t *testing.T, route string) rssWriteBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}

	fixture := rssWriteBoundaryFixture{
		router: router,
		server: server,
		auth:   auth,
		user:   user,
		method: http.MethodPost,
		limit:  testRSSSourceWriteBodyLimit,
	}

	switch route {
	case "source-create":
		fixture.path = "/api/rss/sources"
		fixture.body = `{"title":"bounded create","url":"https://rss.example/bounded-create.xml"}`
	case "source-import":
		fixture.path = "/api/rss/sources/import"
		fixture.body = `[{"title":"bounded import","url":"https://rss.example/bounded-import.xml"}]`
	case "source-update":
		source := models.RSSSource{
			UserID: user.ID, Title: "before update", URL: "https://rss.example/before-update.xml", Enabled: true,
		}
		if err := server.db.Create(&source).Error; err != nil {
			t.Fatal(err)
		}
		fixture.method = http.MethodPut
		fixture.path = "/api/rss/sources/" + uintString(source.ID)
		fixture.body = `{"title":"bounded update","url":"https://rss.example/bounded-update.xml"}`
	case "article-update":
		source, article := seedRSSWriteArticle(t, server, user.ID, "bounded article")
		_ = source
		fixture.method = http.MethodPut
		fixture.path = "/api/rss/articles/" + uintString(article.ID)
		fixture.body = `{"isRead":true}`
		fixture.limit = testRSSArticleWriteBodyLimit
	default:
		t.Fatalf("unknown RSS write route %q", route)
	}

	fixture.before = snapshotRSSWriteRows(t, server, user.ID)
	client := server.hub.AddClient(user.ID, nil)
	t.Cleanup(func() { server.hub.RemoveClient(client) })
	fixture.events = client.Send
	return fixture
}

func TestRSSJSONWritesRejectDeclaredAndChunkedOversizedBodies(t *testing.T) {
	for _, route := range []string{"source-create", "source-import", "source-update", "article-update"} {
		for _, chunked := range []bool{false, true} {
			transport := "declared"
			if chunked {
				transport = "chunked"
			}
			t.Run(route+"/"+transport, func(t *testing.T) {
				fixture := newRSSWriteBoundaryFixture(t, route)
				response := performRSSWriteBoundaryRequest(
					fixture,
					rssWriteBoundaryBodyAtSize(route, fixture.limit+1),
					chunked,
				)
				if route == "source-import" {
					assertRSSWriteBoundaryError(t, response, http.StatusBadRequest, "invalid RSS source import")
				} else {
					assertRSSWriteBoundaryError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
				}
				assertRSSWriteBoundaryFailureStable(t, fixture)
			})
		}
	}
}

func TestRSSJSONWritesAcceptExactLimitWithTrailingWhitespace(t *testing.T) {
	for _, route := range []string{"source-create", "source-import", "source-update", "article-update"} {
		t.Run(route, func(t *testing.T) {
			fixture := newRSSWriteBoundaryFixture(t, route)
			body := fixture.body + strings.Repeat(" ", fixture.limit-len(fixture.body))
			response := performRSSWriteBoundaryRequest(fixture, body, false)
			want := http.StatusOK
			if route == "source-create" {
				want = http.StatusCreated
			}
			if response.Code != want {
				t.Fatalf("exact-limit RSS %s = %d %s, want %d", route, response.Code, boundaryDiagnostic(response.Body.String()), want)
			}
		})
	}
}

func TestRSSJSONWritesAcceptOnlyOneDocumentOfTheExpectedShape(t *testing.T) {
	for _, route := range []string{"source-create", "source-import", "source-update", "article-update"} {
		for _, document := range []struct {
			name string
			body func(rssWriteBoundaryFixture) string
		}{
			{name: "null", body: func(rssWriteBoundaryFixture) string { return "null" }},
			{name: "wrong-container", body: func(fixture rssWriteBoundaryFixture) string {
				if strings.HasSuffix(fixture.path, "/import") {
					return `{}`
				}
				return `[]`
			}},
			{name: "scalar", body: func(rssWriteBoundaryFixture) string { return `"rss"` }},
			{name: "second-json", body: func(fixture rssWriteBoundaryFixture) string { return fixture.body + `{}` }},
			{name: "trailing-garbage", body: func(fixture rssWriteBoundaryFixture) string { return fixture.body + `garbage` }},
		} {
			t.Run(route+"/"+document.name, func(t *testing.T) {
				fixture := newRSSWriteBoundaryFixture(t, route)
				response := performRSSWriteBoundaryRequest(fixture, document.body(fixture), false)
				message := "url is required"
				if route == "source-import" {
					message = "invalid RSS source import"
				} else if route == "article-update" {
					message = "invalid RSS article payload"
				}
				assertRSSWriteBoundaryError(t, response, http.StatusBadRequest, message)
				assertRSSWriteBoundaryFailureStable(t, fixture)
			})
		}
	}
}

func TestRSSWritePrioritizesAuthenticationAndOwnedTargetBeforeBody(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		router, _ := setupTestServer(t)
		body := &rssWriteNoReadBody{}
		response := performRSSWriteReaderRequest(router, "", http.MethodPost, "/api/rss/sources", body)
		if response.Code != http.StatusUnauthorized || body.read {
			t.Fatalf("unauthenticated RSS body priority = %d read=%t body=%s", response.Code, body.read, response.Body.String())
		}
	})

	router, _ := setupTestServer(t)
	auth := authHeader(t, router)
	for _, request := range []struct {
		name string
		path string
	}{
		{name: "source-update", path: "/api/rss/sources/999999"},
		{name: "article-update", path: "/api/rss/articles/999999"},
	} {
		t.Run(request.name, func(t *testing.T) {
			body := &rssWriteNoReadBody{}
			response := performRSSWriteReaderRequest(router, auth, http.MethodPut, request.path, body)
			if response.Code != http.StatusNotFound || body.read {
				t.Errorf("missing RSS target priority = %d read=%t body=%s", response.Code, body.read, response.Body.String())
			}
		})
	}
}

func TestRSSImportEnforcesRawItemLimitBeforeMutation(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		fixture := newRSSWriteBoundaryFixture(t, "source-import")
		response := performRSSWriteBoundaryRequest(fixture, rssImportBoundaryPayload(testRSSImportItemLimit), false)
		if response.Code != http.StatusOK {
			t.Fatalf("exact RSS import item limit = %d %s", response.Code, boundaryDiagnostic(response.Body.String()))
		}
		var count int64
		if err := fixture.server.db.Model(&models.RSSSource{}).Where("user_id = ?", fixture.user.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != testRSSImportItemLimit {
			t.Fatalf("exact RSS import persisted %d rows, want %d", count, testRSSImportItemLimit)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		fixture := newRSSWriteBoundaryFixture(t, "source-import")
		response := performRSSWriteBoundaryRequest(fixture, rssImportBoundaryPayload(testRSSImportItemLimit+1), false)
		assertRSSWriteBoundaryError(t, response, http.StatusBadRequest, "invalid RSS source import")
		assertRSSWriteBoundaryFailureStable(t, fixture)
	})
}

func TestRSSArticleStateRequiresExplicitNonNullBoolean(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"ignored":true}`,
		`{"isRead":null}`,
		`{"favorite":null}`,
		`{"isRead":"yes"}`,
		`{"favorite":1}`,
	} {
		t.Run(body, func(t *testing.T) {
			fixture := newRSSWriteBoundaryFixture(t, "article-update")
			response := performRSSWriteBoundaryRequest(fixture, body, false)
			assertRSSWriteBoundaryError(t, response, http.StatusBadRequest, "invalid RSS article payload")
			assertRSSWriteBoundaryFailureStable(t, fixture)
		})
	}

	t.Run("single-field-preserves-other-state", func(t *testing.T) {
		fixture := newRSSWriteBoundaryFixture(t, "article-update")
		var article models.RSSArticle
		if err := fixture.server.db.Where("user_id = ?", fixture.user.ID).First(&article).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.server.db.Model(&article).Update("favorite", true).Error; err != nil {
			t.Fatal(err)
		}
		response := performRSSWriteBoundaryRequest(fixture, `{"isRead":true}`, false)
		if response.Code != http.StatusOK {
			t.Fatalf("single-field RSS state = %d %s", response.Code, response.Body.String())
		}
		var stored models.RSSArticle
		if err := fixture.server.db.First(&stored, article.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !stored.IsRead || !stored.Favorite {
			t.Fatalf("single-field RSS state lost other state: %+v", stored)
		}
	})
}

func TestRSSSourceUpdateCannotReviveConcurrentlyDeletedTarget(t *testing.T) {
	fixture := newRSSWriteBoundaryFixture(t, "source-update")
	var source models.RSSSource
	if err := fixture.server.db.Where("user_id = ?", fixture.user.ID).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`
		CREATE TRIGGER rss_source_update_concurrent_delete
		BEFORE UPDATE OF title ON rss_sources
		WHEN OLD.id = %d
		BEGIN
			DELETE FROM rss_sources WHERE id = OLD.id;
		END;
	`, source.ID)
	if err := fixture.server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	response := performRSSWriteBoundaryRequest(fixture, fixture.body, false)
	if response.Code != http.StatusNotFound {
		t.Errorf("concurrently deleted RSS source update = %d %s, want 404", response.Code, response.Body.String())
	}
	var count int64
	if err := fixture.server.db.Model(&models.RSSSource{}).Where("id = ?", source.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("RSS source update revived deleted source %d", source.ID)
	}
	if events := drainRSSWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("concurrently deleted RSS source update events = %v, want none", events)
	}
}

func TestRSSArticleStateDoesNotOverwriteConcurrentRemoteColumns(t *testing.T) {
	fixture := newRSSWriteBoundaryFixture(t, "article-update")
	var article models.RSSArticle
	if err := fixture.server.db.Where("user_id = ?", fixture.user.ID).First(&article).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`
		CREATE TRIGGER rss_state_concurrent_remote_update
		BEFORE UPDATE OF is_read ON rss_articles
		WHEN OLD.id = %d
		BEGIN
			UPDATE rss_articles SET title = 'concurrent title', summary = 'concurrent summary' WHERE id = OLD.id;
		END;
	`, article.ID)
	if err := fixture.server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	response := performRSSWriteBoundaryRequest(fixture, `{"isRead":true}`, false)
	if response.Code != http.StatusOK {
		t.Fatalf("RSS state update = %d %s", response.Code, response.Body.String())
	}
	var stored models.RSSArticle
	if err := fixture.server.db.First(&stored, article.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.IsRead || stored.Title != "concurrent title" || stored.Summary != "concurrent summary" {
		t.Fatalf("RSS state update overwrote concurrent remote fields: %+v", stored)
	}
	var projected models.RSSArticle
	if err := json.Unmarshal(response.Body.Bytes(), &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Title != stored.Title || projected.Summary != stored.Summary {
		t.Fatalf("RSS state update returned stale row: %+v stored=%+v", projected, stored)
	}
}

func TestRSSArticleStateCannotReviveConcurrentlyDeletedTarget(t *testing.T) {
	fixture := newRSSWriteBoundaryFixture(t, "article-update")
	var article models.RSSArticle
	if err := fixture.server.db.Where("user_id = ?", fixture.user.ID).First(&article).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`
		CREATE TRIGGER rss_state_concurrent_delete
		BEFORE UPDATE OF favorite ON rss_articles
		WHEN OLD.id = %d
		BEGIN
			DELETE FROM rss_articles WHERE id = OLD.id;
		END;
	`, article.ID)
	if err := fixture.server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	response := performRSSWriteBoundaryRequest(fixture, `{"favorite":true}`, false)
	if response.Code != http.StatusNotFound {
		t.Errorf("concurrently deleted RSS article update = %d %s, want 404", response.Code, response.Body.String())
	}
	var count int64
	if err := fixture.server.db.Model(&models.RSSArticle{}).Where("id = ?", article.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("RSS state update revived deleted article %d", article.ID)
	}
	if events := drainRSSWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("concurrently deleted RSS article update events = %v, want none", events)
	}
}

func TestRSSSourceIdentitySerializesConcurrentCreateAndImport(t *testing.T) {
	for _, scenario := range []string{"create-create", "create-import", "import-import"} {
		t.Run(scenario, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			var user models.User
			if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
				t.Fatal(err)
			}
			const operations = 8
			start := make(chan struct{})
			codes := make(chan int, operations)
			var wait sync.WaitGroup
			for index := 0; index < operations; index++ {
				useImport := scenario == "import-import" || (scenario == "create-import" && index%2 == 1)
				wait.Add(1)
				go func(index int, useImport bool) {
					defer wait.Done()
					<-start
					path := "/api/rss/sources"
					body := fmt.Sprintf(`{"title":"concurrent %d","url":"https://rss.example/concurrent.xml"}`, index)
					if useImport {
						path += "/import"
						body = `[` + body + `]`
					}
					request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
					request.Header.Set("Authorization", auth)
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					codes <- response.Code
				}(index, useImport)
			}
			close(start)
			wait.Wait()
			close(codes)

			for code := range codes {
				if code != http.StatusOK && code != http.StatusCreated {
					t.Errorf("concurrent RSS %s returned %d, want only 200/201", scenario, code)
				}
			}
			var count int64
			if err := server.db.Model(&models.RSSSource{}).
				Where("user_id = ? AND url = ?", user.ID, "https://rss.example/concurrent.xml").
				Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Errorf("concurrent RSS %s left %d same-URL rows, want 1", scenario, count)
			}
		})
	}
}

func TestRSSContentCachePreservesConcurrentStateAndMetadata(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{
		UserID: user.ID, Title: "detail source", URL: "https://rss.example/feed.xml",
		RuleContent: ".content|html", Enabled: true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{
		UserID: user.ID, SourceID: source.ID, Title: "before content", Link: "https://rss.example/article/1",
	}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`
		CREATE TRIGGER rss_content_concurrent_columns
		BEFORE UPDATE OF content ON rss_articles
		WHEN OLD.id = %d
		BEGIN
			UPDATE rss_articles SET title = 'concurrent content title', favorite = 1 WHERE id = OLD.id;
		END;
	`, article.ID)
	if err := server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	restoreClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return rssWriteHTTPResponse(request, `<div class="content"><p>detail body</p></div>`), nil
	})})
	defer restoreClient()

	request := httptest.NewRequest(http.MethodGet, "/api/rss/articles/"+uintString(article.ID)+"/content", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("RSS content cache = %d %s", response.Code, response.Body.String())
	}
	var stored models.RSSArticle
	if err := server.db.First(&stored, article.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "concurrent content title" || !stored.Favorite || !strings.Contains(stored.Content, "detail body") {
		t.Fatalf("RSS content cache overwrote concurrent columns: %+v", stored)
	}
	var projected models.RSSArticle
	if err := json.Unmarshal(response.Body.Bytes(), &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Title != stored.Title || projected.Favorite != stored.Favorite {
		t.Fatalf("RSS content cache returned stale row: %+v stored=%+v", projected, stored)
	}
}

func TestRSSRefreshPreservesAuthoritativeDetailedContent(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{
		UserID: user.ID, Title: "content priority", URL: "https://rss.example/feed.xml",
		RuleContent: ".content|html", Enabled: true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{
		UserID: user.ID, SourceID: source.ID, Title: "old title", Link: "https://rss.example/article/priority",
		Content: "<p>authoritative detail</p>", Favorite: true,
	}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	feed := `<?xml version="1.0"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><item><title>new title</title><link>https://rss.example/article/priority</link><description>new summary</description><content:encoded><![CDATA[<p>feed candidate</p>]]></content:encoded></item></channel></rss>`
	restoreClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return rssWriteHTTPResponse(request, feed), nil
	})})
	defer restoreClient()

	request := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+uintString(source.ID)+"/refresh", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("RSS content-priority refresh = %d %s", response.Code, response.Body.String())
	}
	var stored models.RSSArticle
	if err := server.db.First(&stored, article.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "new title" || stored.Content != "<p>authoritative detail</p>" || !stored.Favorite {
		t.Fatalf("RSS refresh violated detail/state priority: %+v", stored)
	}
}

func TestRSSRefreshCannotCacheAfterSourceDeletedDuringFetch(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{UserID: user.ID, Title: "deleted during fetch", URL: "https://rss.example/deleted-feed.xml", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	events := server.hub.AddClient(user.ID, nil).Send
	feed := `<rss version="2.0"><channel><item><title>late article</title><link>https://rss.example/late</link></item></channel></rss>`
	restoreClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := server.db.Where("user_id = ? AND id = ?", user.ID, source.ID).Delete(&models.RSSSource{}).Error; err != nil {
			return nil, err
		}
		return rssWriteHTTPResponse(request, feed), nil
	})})
	defer restoreClient()

	request := httptest.NewRequest(http.MethodPost, "/api/rss/sources/"+uintString(source.ID)+"/refresh", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("RSS refresh after source deletion = %d %s, want 404", response.Code, response.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.RSSArticle{}).Where("user_id = ? AND source_id = ?", user.ID, source.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("RSS refresh cached %d orphan articles after source deletion", count)
	}
	if emitted := drainRSSWriteEvents(events); len(emitted) != 0 {
		t.Errorf("RSS refresh after source deletion events = %v, want none", emitted)
	}
}

func TestRSSContentCannotReviveRowsDeletedDuringFetch(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{
		UserID: user.ID, Title: "deleted detail source", URL: "https://rss.example/feed.xml",
		RuleContent: ".content|html", Enabled: true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{
		UserID: user.ID, SourceID: source.ID, Title: "deleted detail", Link: "https://rss.example/article/deleted",
	}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	restoreClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := server.db.Where("user_id = ? AND id = ?", user.ID, article.ID).Delete(&models.RSSArticle{}).Error; err != nil {
			return nil, err
		}
		if err := server.db.Where("user_id = ? AND id = ?", user.ID, source.ID).Delete(&models.RSSSource{}).Error; err != nil {
			return nil, err
		}
		return rssWriteHTTPResponse(request, `<div class="content">late detail</div>`), nil
	})})
	defer restoreClient()

	request := httptest.NewRequest(http.MethodGet, "/api/rss/articles/"+uintString(article.ID)+"/content", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Errorf("RSS content after deletion = %d %s, want 404", response.Code, response.Body.String())
	}
	var articleCount int64
	if err := server.db.Model(&models.RSSArticle{}).Where("id = ?", article.ID).Count(&articleCount).Error; err != nil {
		t.Fatal(err)
	}
	if articleCount != 0 {
		t.Errorf("RSS content fetch revived deleted article %d", article.ID)
	}
}

func seedRSSWriteArticle(t *testing.T, server *Server, userID uint, title string) (models.RSSSource, models.RSSArticle) {
	t.Helper()
	source := models.RSSSource{
		UserID: userID, Title: title + " source", URL: "https://rss.example/" + strings.ReplaceAll(title, " ", "-") + ".xml", Enabled: true,
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{
		UserID: userID, SourceID: source.ID, Title: title, Link: "https://rss.example/article/" + strconv.FormatUint(uint64(source.ID), 10),
		Summary: "before summary",
	}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	return source, article
}

func performRSSWriteBoundaryRequest(fixture rssWriteBoundaryFixture, body string, chunked bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(fixture.method, fixture.path, strings.NewReader(body))
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func performRSSWriteReaderRequest(router http.Handler, auth, method, path string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func rssWriteBoundaryBodyAtSize(route string, size int) string {
	var prefix, suffix string
	switch route {
	case "source-import":
		prefix = `[{"title":"bounded","url":"https://rss.example/bounded.xml","padding":"`
		suffix = `"}]`
	case "article-update":
		prefix = `{"isRead":true,"padding":"`
		suffix = `"}`
	default:
		prefix = `{"title":"bounded","url":"https://rss.example/bounded.xml","padding":"`
		suffix = `"}`
	}
	if size < len(prefix)+len(suffix) {
		panic("RSS boundary body size too small")
	}
	return prefix + strings.Repeat("p", size-len(prefix)-len(suffix)) + suffix
}

func rssImportBoundaryPayload(count int) string {
	var builder strings.Builder
	builder.Grow(count * 96)
	builder.WriteByte('[')
	for index := 0; index < count; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `{"title":"source %d","url":"https://rss.example/import/%d.xml"}`, index, index)
	}
	builder.WriteByte(']')
	return builder.String()
}

func snapshotRSSWriteRows(t *testing.T, server *Server, userID uint) []byte {
	t.Helper()
	var sources []models.RSSSource
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	var articles []models.RSSArticle
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&articles).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(struct {
		Sources  []models.RSSSource  `json:"sources"`
		Articles []models.RSSArticle `json:"articles"`
	}{Sources: sources, Articles: articles})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertRSSWriteBoundaryFailureStable(t *testing.T, fixture rssWriteBoundaryFixture) {
	t.Helper()
	after := snapshotRSSWriteRows(t, fixture.server, fixture.user.ID)
	if string(after) != string(fixture.before) {
		t.Errorf("rejected RSS write changed rows\nbefore=%s\nafter=%s", fixture.before, after)
	}
	if events := drainRSSWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("rejected RSS write events = %v, want none", events)
	}
}

func assertRSSWriteBoundaryError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	want := fmt.Sprintf(`{"error":%q}`, message)
	if response.Code != status || response.Body.String() != want {
		t.Errorf("RSS write error = %d %s, want %d %s", response.Code, boundaryDiagnostic(response.Body.String()), status, want)
	}
}

func drainRSSWriteEvents(channel <-chan []byte) []string {
	var events []string
	for {
		select {
		case payload := <-channel:
			events = append(events, string(payload))
		default:
			return events
		}
	}
}

func rssWriteHTTPResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}

type rssWriteNoReadBody struct {
	read bool
}

func (body *rssWriteNoReadBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("RSS request body must not be read")
}

var _ io.Reader = (*rssWriteNoReadBody)(nil)
