package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	testBookControlBodyBytes       = 16 << 10
	testLocalRefreshBodyBytes      = 32 << 10
	testRemoteBookControlBodyBytes = 1 << 20
)

type bookControlBoundaryFixture struct {
	router     http.Handler
	server     *Server
	auth       string
	user       models.User
	remoteBook models.Book
	localBook  models.Book
}

type bookControlBoundaryRoute struct {
	name           string
	path           string
	body           string
	maxBytes       int
	malformedError string
	legacy         bool
}

func newBookControlBoundaryFixture(t *testing.T) bookControlBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	remoteBook := models.Book{
		UserID:   user.ID,
		SourceID: 999999,
		Title:    "Book control remote",
		URL:      "https://book-control.invalid/current",
	}
	localBook := models.Book{
		UserID:       user.ID,
		Title:        "Book control local",
		URL:          "local://book-control",
		LibraryPath:  "data/testuser/missing-book-control",
		OriginalFile: "source.txt",
	}
	if err := server.db.Create(&remoteBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&localBook).Error; err != nil {
		t.Fatal(err)
	}
	return bookControlBoundaryFixture{
		router:     router,
		server:     server,
		auth:       auth,
		user:       user,
		remoteBook: remoteBook,
		localBook:  localBook,
	}
}

func bookControlBoundaryRoutes(fixture bookControlBoundaryFixture) []bookControlBoundaryRoute {
	return []bookControlBoundaryRoute{
		{
			name:           "batch",
			path:           "/api/books/batch",
			body:           fmt.Sprintf(`{"action":"unsupported","bookIds":[%d]}`, fixture.remoteBook.ID),
			maxBytes:       testBookControlBodyBytes,
			malformedError: "invalid batch payload",
		},
		{
			name:           "export",
			path:           "/api/books/export",
			body:           fmt.Sprintf(`{"bookIds":[%d],"format":"unsupported"}`, fixture.remoteBook.ID),
			maxBytes:       testBookControlBodyBytes,
			malformedError: "bookIds is required",
		},
		{
			name:           "refresh-local",
			path:           "/api/books/" + uintString(fixture.localBook.ID) + "/refresh-local",
			body:           `{"tocRule":null}`,
			maxBytes:       testLocalRefreshBodyBytes,
			malformedError: "invalid local refresh payload",
		},
		{
			name:           "remote-add",
			path:           "/api/books/remote",
			body:           `{"title":"Remote boundary","bookUrl":"https://book-control.invalid/new","sourceId":999999}`,
			maxBytes:       testRemoteBookControlBodyBytes,
			malformedError: "title, bookUrl, and sourceId are required",
		},
		{
			name:           "change-source",
			path:           "/api/books/" + uintString(fixture.remoteBook.ID) + "/change-source",
			body:           `{"sourceId":999999}`,
			maxBytes:       testRemoteBookControlBodyBytes,
			malformedError: "sourceId is required",
		},
		{
			name:           "legacy-content-search",
			path:           "/api/reader3/searchBookContent",
			body:           `{"bookUrl":"https://book-control.invalid/missing","keyword":"boundary"}`,
			maxBytes:       testBookControlBodyBytes,
			malformedError: "请求格式不正确",
			legacy:         true,
		},
	}
}

func TestBookControlBoundaryRejectsDeclaredAndChunkedOverflow(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		transport := "declared"
		if chunked {
			transport = "chunked"
		}
		fixture := newBookControlBoundaryFixture(t)
		for _, route := range bookControlBoundaryRoutes(fixture) {
			t.Run(route.name+"/"+transport, func(t *testing.T) {
				body := padBookControlObject(route.body, route.maxBytes+1)
				response := performBookControlRequest(fixture.router, fixture.auth, route.path, []byte(body), chunked, nil)
				assertBookControlWireError(t, response, route, true)
			})
		}
	}
}

func TestBookControlBoundaryAcceptsExactRequestLimit(t *testing.T) {
	fixture := newBookControlBoundaryFixture(t)
	for _, route := range bookControlBoundaryRoutes(fixture) {
		t.Run(route.name, func(t *testing.T) {
			body := padBookControlObject(route.body, route.maxBytes)
			response := performBookControlRequest(fixture.router, fixture.auth, route.path, []byte(body), false, nil)
			if route.legacy {
				var envelope struct {
					ErrorMsg string `json:"errorMsg"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.ErrorMsg == route.malformedError {
					t.Fatalf("exact request limit was rejected as malformed: %s", response.Body.String())
				}
				return
			}
			if response.Code == http.StatusRequestEntityTooLarge || strings.Contains(response.Body.String(), "request body too large") {
				t.Fatalf("exact request limit was rejected as overflow: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestBookControlBoundaryRejectsAmbiguousOrInvalidUTF8Documents(t *testing.T) {
	for _, wire := range []struct {
		name string
		body func(bookControlBoundaryRoute) []byte
	}{
		{name: "second-json", body: func(route bookControlBoundaryRoute) []byte {
			return []byte(route.body + `{"ignored":true}`)
		}},
		{name: "invalid-utf8", body: func(bookControlBoundaryRoute) []byte {
			return []byte{'{', '"', 'p', 'a', 'd', 'd', 'i', 'n', 'g', '"', ':', '"', 0xff, '"', '}'}
		}},
		{name: "null", body: func(bookControlBoundaryRoute) []byte { return []byte("null") }},
		{name: "array", body: func(bookControlBoundaryRoute) []byte { return []byte("[]") }},
	} {
		fixture := newBookControlBoundaryFixture(t)
		for _, route := range bookControlBoundaryRoutes(fixture) {
			t.Run(route.name+"/"+wire.name, func(t *testing.T) {
				response := performBookControlRequest(fixture.router, fixture.auth, route.path, wire.body(route), false, nil)
				assertBookControlWireError(t, response, route, false)
			})
		}
	}
}

func TestBookControlBoundaryAuthenticationPrecedesBodyAdmission(t *testing.T) {
	fixture := newBookControlBoundaryFixture(t)
	for _, route := range bookControlBoundaryRoutes(fixture) {
		for _, auth := range []string{"", "Bearer invalid"} {
			t.Run(route.name+"/"+auth, func(t *testing.T) {
				body := &bookControlNoReadBody{}
				request := httptest.NewRequest(http.MethodPost, route.path, body)
				request.ContentLength = int64(route.maxBytes + 1)
				request.Header.Set("Content-Type", "application/json")
				if auth != "" {
					request.Header.Set("Authorization", auth)
				}
				response := httptest.NewRecorder()
				fixture.router.ServeHTTP(response, request)
				if response.Code != http.StatusUnauthorized {
					t.Fatalf("auth-first response = %d: %s", response.Code, response.Body.String())
				}
				if body.reads != 0 {
					t.Fatalf("unauthorized request read body %d times", body.reads)
				}
			})
		}
	}
}

func TestBookControlBoundaryRejectsTrailingBatchDeleteWithoutSideEffects(t *testing.T) {
	fixture := newBookControlBoundaryFixture(t)
	body := fmt.Sprintf(`{"action":"delete","bookIds":[%d]}{"ignored":true}`, fixture.remoteBook.ID)
	response := performBookControlRequest(fixture.router, fixture.auth, "/api/books/batch", []byte(body), false, nil)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid batch payload") {
		t.Fatalf("trailing batch delete = %d: %s", response.Code, response.Body.String())
	}
	var count int64
	if err := fixture.server.db.Model(&models.Book{}).Where("id = ? AND user_id = ?", fixture.remoteBook.ID, fixture.user.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("trailing JSON batch request deleted the durable book")
	}
}

func TestBookControlBoundaryValidatesRawCategoryAndStoredFieldLimitsBeforeWork(t *testing.T) {
	t.Run("batch categories", func(t *testing.T) {
		fixture := newBookControlBoundaryFixture(t)
		category := createBookGroupWriteCategory(t, fixture.server, fixture.user.ID, "book-control-category", 10)
		ids := repeatedBookControlIDs(category.ID, 201)
		body := fmt.Sprintf(`{"action":"category","bookIds":[%d],"categoryIds":[%s]}`, fixture.remoteBook.ID, ids)
		response := performBookControlRequest(fixture.router, fixture.auth, "/api/books/batch", []byte(body), false, nil)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "too many categories") {
			t.Fatalf("201 batch categories = %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("remote categories", func(t *testing.T) {
		fixture := newBookControlBoundaryFixture(t)
		category := createBookGroupWriteCategory(t, fixture.server, fixture.user.ID, "remote-control-category", 10)
		body := fmt.Sprintf(
			`{"title":"remote","bookUrl":"https://book-control.invalid/new","sourceId":999999,"categoryIds":[%s]}`,
			repeatedBookControlIDs(category.ID, 201),
		)
		response := performBookControlRequest(fixture.router, fixture.auth, "/api/books/remote", []byte(body), false, nil)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "too many categories") {
			t.Fatalf("201 remote categories = %d: %s", response.Code, response.Body.String())
		}
	})

	for _, route := range []struct {
		name string
		path func(bookControlBoundaryFixture) string
		body string
	}{
		{
			name: "remote title",
			path: func(bookControlBoundaryFixture) string { return "/api/books/remote" },
			body: fmt.Sprintf(`{"title":%q,"bookUrl":"https://book-control.invalid/new","sourceId":999999}`, strings.Repeat("t", 241)),
		},
		{
			name: "change title",
			path: func(fixture bookControlBoundaryFixture) string {
				return "/api/books/" + uintString(fixture.remoteBook.ID) + "/change-source"
			},
			body: fmt.Sprintf(`{"sourceId":999999,"title":%q}`, strings.Repeat("t", 241)),
		},
	} {
		t.Run(route.name, func(t *testing.T) {
			fixture := newBookControlBoundaryFixture(t)
			response := performBookControlRequest(fixture.router, fixture.auth, route.path(fixture), []byte(route.body), false, nil)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "book title is too long") {
				t.Fatalf("oversized book title = %d: %s", response.Code, response.Body.String())
			}
		})
	}

	t.Run("local toc rule", func(t *testing.T) {
		fixture := newBookControlBoundaryFixture(t)
		body := fmt.Sprintf(`{"tocRule":%q}`, strings.Repeat("r", engine.MaxTXTTocRuleBytes+1))
		response := performBookControlRequest(
			fixture.router,
			fixture.auth,
			"/api/books/"+uintString(fixture.localBook.ID)+"/refresh-local",
			[]byte(body),
			false,
			nil,
		)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "toc rule is too large") {
			t.Fatalf("oversized local TOC rule = %d: %s", response.Code, response.Body.String())
		}
	})
}

func TestBookControlBoundaryAcceptsExactCategoryAndStoredFieldLimits(t *testing.T) {
	t.Run("batch categories", func(t *testing.T) {
		fixture := newBookControlBoundaryFixture(t)
		category := createBookGroupWriteCategory(t, fixture.server, fixture.user.ID, "book-control-category-exact", 10)
		body := fmt.Sprintf(
			`{"action":"category","bookIds":[%d],"categoryIds":[%s]}`,
			fixture.remoteBook.ID,
			repeatedBookControlIDs(category.ID, 200),
		)
		response := performBookControlRequest(fixture.router, fixture.auth, "/api/books/batch", []byte(body), false, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("200 batch categories = %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("remote categories", func(t *testing.T) {
		fixture := newBookControlBoundaryFixture(t)
		category := createBookGroupWriteCategory(t, fixture.server, fixture.user.ID, "remote-control-category-exact", 10)
		body := fmt.Sprintf(
			`{"title":"remote","bookUrl":"https://book-control.invalid/new","sourceId":999999,"categoryIds":[%s]}`,
			repeatedBookControlIDs(category.ID, 200),
		)
		response := performBookControlRequest(fixture.router, fixture.auth, "/api/books/remote", []byte(body), false, nil)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "source not found") {
			t.Fatalf("200 remote categories = %d: %s", response.Code, response.Body.String())
		}
	})

	for _, field := range []struct {
		name       string
		jsonName   string
		maxBytes   int
		tooLongErr string
	}{
		{name: "title", jsonName: "title", maxBytes: 240, tooLongErr: "book title is too long"},
		{name: "author", jsonName: "author", maxBytes: 160, tooLongErr: "book author is too long"},
		{name: "cover", jsonName: "coverUrl", maxBytes: 600, tooLongErr: "book cover url is too long"},
		{name: "kind", jsonName: "kind", maxBytes: 400, tooLongErr: "book kind is too long"},
		{name: "word-count", jsonName: "wordCount", maxBytes: 120, tooLongErr: "book word count is too long"},
		{name: "book-url", jsonName: "bookUrl", maxBytes: 800, tooLongErr: "book url is too long"},
	} {
		for _, route := range []string{"remote-add", "change-source"} {
			t.Run(route+"/"+field.name, func(t *testing.T) {
				for _, delta := range []int{0, 1} {
					fixture := newBookControlBoundaryFixture(t)
					payload := map[string]any{"sourceId": 999999}
					path := "/api/books/" + uintString(fixture.remoteBook.ID) + "/change-source"
					if route == "remote-add" {
						payload["title"] = "remote"
						payload["bookUrl"] = "https://book-control.invalid/new"
						path = "/api/books/remote"
					}
					payload[field.jsonName] = strings.Repeat("v", field.maxBytes+delta)
					body, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					response := performBookControlRequest(fixture.router, fixture.auth, path, body, false, nil)
					if delta == 0 {
						if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "source not found") {
							t.Fatalf("exact %s = %d: %s", field.name, response.Code, response.Body.String())
						}
						continue
					}
					if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), field.tooLongErr) {
						t.Fatalf("oversized %s = %d: %s", field.name, response.Code, response.Body.String())
					}
				}
			})
		}
	}
}

func TestBookControlLocalRefreshAcceptsEmptyBodiesAndExactTOCRuleLimit(t *testing.T) {
	fixture := newBookControlBoundaryFixture(t)
	path := "/api/books/" + uintString(fixture.localBook.ID) + "/refresh-local"
	for _, request := range []struct {
		name    string
		body    []byte
		chunked bool
	}{
		{name: "known-empty", body: []byte{}},
		{name: "unknown-empty", body: []byte{}, chunked: true},
		{name: "exact-toc-rule", body: []byte(fmt.Sprintf(`{"tocRule":%q}`, strings.Repeat("r", engine.MaxTXTTocRuleBytes)))},
	} {
		t.Run(request.name, func(t *testing.T) {
			response := performBookControlRequest(fixture.router, fixture.auth, path, request.body, request.chunked, nil)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "local source file not found") {
				t.Fatalf("accepted refresh body = %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestBookControlRemoteWorkPropagatesRequestCancellation(t *testing.T) {
	for _, mode := range []string{"remote-add", "change-source", "batch-cache", "export"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newBookControlCancellationFixture(t)
			started := make(chan struct{}, 1)
			contextCanceled := make(chan bool, 1)
			restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-request.Context().Done():
					contextCanceled <- true
					return nil, request.Context().Err()
				case <-time.After(350 * time.Millisecond):
					contextCanceled <- false
					return sourceDebugHTTPResponse(request, `<html><div class="detail-title">Boundary</div><main class="content">content</main></html>`), nil
				}
			})})
			defer restore()

			path, body := bookControlCancellationRequest(mode, fixture)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- performBookControlRequest(fixture.router, fixture.auth, path, []byte(body), false, ctx)
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				cancel()
				t.Fatal("book control request did not start remote work")
			}
			cancel()
			wasCanceled := <-contextCanceled
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("book control request did not finish after cancellation")
			}
			if !wasCanceled {
				t.Fatalf("%s did not propagate caller cancellation to remote transport", mode)
			}
			if got := len(fixture.events); got != 0 {
				t.Fatalf("canceled %s emitted %d completion events", mode, got)
			}
		})
	}
}

type bookControlCancellationFixture struct {
	bookControlBoundaryFixture
	source  models.BookSource
	chapter models.Chapter
	events  <-chan []byte
}

func newBookControlCancellationFixture(t *testing.T) bookControlCancellationFixture {
	t.Helper()
	base := newBookControlBoundaryFixture(t)
	source, err := base.server.bookSources.Create(base.user.ID, sourceDebugModeSource(t, "https://book-control.example"))
	if err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:   base.user.ID,
		SourceID: source.ID,
		Title:    "Cancelable book control",
		URL:      "https://book-control.example/book",
	}
	if err := base.server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{
		BookID: book.ID,
		Index:  0,
		Title:  "Cancelable chapter",
		URL:    "https://book-control.example/chapter",
	}
	if err := base.server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	base.remoteBook = book
	client := base.server.hub.AddClient(base.user.ID, nil)
	t.Cleanup(func() { base.server.hub.RemoveClient(client) })
	return bookControlCancellationFixture{
		bookControlBoundaryFixture: base,
		source:                     source,
		chapter:                    chapter,
		events:                     client.Send,
	}
}

func bookControlCancellationRequest(mode string, fixture bookControlCancellationFixture) (string, string) {
	switch mode {
	case "remote-add":
		return "/api/books/remote", fmt.Sprintf(
			`{"title":"Cancelable new book","bookUrl":"https://book-control.example/new","sourceId":%d}`,
			fixture.source.ID,
		)
	case "change-source":
		return "/api/books/" + uintString(fixture.remoteBook.ID) + "/change-source", fmt.Sprintf(
			`{"sourceId":%d,"bookUrl":"https://book-control.example/change"}`,
			fixture.source.ID,
		)
	case "batch-cache":
		return "/api/books/batch", fmt.Sprintf(`{"action":"cache","bookIds":[%d]}`, fixture.remoteBook.ID)
	case "export":
		return "/api/books/export", fmt.Sprintf(`{"bookIds":[%d],"format":"txt"}`, fixture.remoteBook.ID)
	default:
		panic("unknown book control cancellation mode")
	}
}

func performBookControlRequest(
	router http.Handler,
	auth string,
	path string,
	body []byte,
	chunked bool,
	ctx context.Context,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", auth)
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	if ctx != nil {
		request = request.WithContext(ctx)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertBookControlWireError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	route bookControlBoundaryRoute,
	overflow bool,
) {
	t.Helper()
	if route.legacy {
		var envelope struct {
			Success  bool   `json:"isSuccess"`
			ErrorMsg string `json:"errorMsg"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode legacy response %d %q: %v", response.Code, response.Body.String(), err)
		}
		if response.Code != http.StatusOK || envelope.Success || envelope.ErrorMsg != route.malformedError {
			t.Fatalf("legacy wire error = %d %+v, want 200 failure %q", response.Code, envelope, route.malformedError)
		}
		return
	}
	wantStatus := http.StatusBadRequest
	wantMessage := route.malformedError
	if overflow {
		wantStatus = http.StatusRequestEntityTooLarge
		wantMessage = "request body too large"
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode modern response %d %q: %v", response.Code, response.Body.String(), err)
	}
	if response.Code != wantStatus || envelope.Error != wantMessage {
		t.Fatalf("wire error = %d %q, want %d %q", response.Code, envelope.Error, wantStatus, wantMessage)
	}
}

func padBookControlObject(body string, total int) string {
	prefix := strings.TrimSuffix(body, "}") + `,"padding":"`
	suffix := `"}`
	if total < len(prefix)+len(suffix) {
		panic("book control target body is too small")
	}
	return prefix + strings.Repeat("p", total-len(prefix)-len(suffix)) + suffix
}

func repeatedBookControlIDs(id uint, count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = uintString(id)
	}
	return strings.Join(values, ",")
}

type bookControlNoReadBody struct {
	reads int
}

func (body *bookControlNoReadBody) Read([]byte) (int, error) {
	body.reads++
	return 0, io.EOF
}
