package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	testSourceDocumentBodyLimit = 16 << 20
	testSourceControlBodyLimit  = 16 << 10
	testSourceImportEntryLimit  = 5000
)

type sourceWriteBoundaryFixture struct {
	router  *gin.Engine
	server  *Server
	account sourceContractAccount
	method  string
	path    string
	body    string
	limit   int
	error   string
	before  []byte
	events  <-chan []byte
}

func newSourceWriteBoundaryFixture(t *testing.T, route string) sourceWriteBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	account := registerSourceContractAccount(t, router, "sourceboundary"+strings.ReplaceAll(route, "-", ""))
	fixture := sourceWriteBoundaryFixture{
		router:  router,
		server:  server,
		account: account,
		method:  http.MethodPost,
		limit:   testSourceControlBodyLimit,
	}

	switch route {
	case "create":
		fixture.path = "/api/sources"
		fixture.body = `{"name":"bounded create","baseUrl":"https://bounded-create.example"}`
		fixture.limit = testSourceDocumentBodyLimit
		fixture.error = "invalid source payload"
	case "update":
		source := createSourceThroughAPI(t, router, account.Auth, `{
			"name":"bounded update",
			"baseUrl":"https://bounded-update.example",
			"enabled":true
		}`)
		fixture.method = http.MethodPut
		fixture.path = "/api/sources/" + uintString(source.ID)
		fixture.body = `{"name":"bounded update after","baseUrl":"https://bounded-update-after.example","enabled":true}`
		fixture.limit = testSourceDocumentBodyLimit
		fixture.error = "invalid source payload"
	case "batch":
		source := createSourceThroughAPI(t, router, account.Auth, `{
			"name":"bounded batch",
			"baseUrl":"https://bounded-batch.example",
			"enabled":true
		}`)
		fixture.path = "/api/sources/batch"
		fixture.body = fmt.Sprintf(`{"action":"disable","sourceIds":[%d]}`, source.ID)
		fixture.error = "action and sourceIds are required"
	case "remote-preview":
		fixture.path = "/api/sources/remote-preview"
		fixture.body = `{"url":" "}`
		fixture.error = "url is required"
		installSourceBoundaryFailingTransport(t)
	case "remote":
		fixture.path = "/api/sources/remote"
		fixture.body = `{"url":" "}`
		fixture.error = "url is required"
		installSourceBoundaryFailingTransport(t)
	default:
		t.Fatalf("unknown source write route %q", route)
	}

	fixture.before = snapshotSourceWriteState(t, server, account.ID)
	fixture.events = server.hub.AddClient(account.ID, nil).Send
	return fixture
}

func TestBookSourceJSONWritesRejectDeclaredAndChunkedOversizedBodies(t *testing.T) {
	for _, route := range []string{"create", "update", "batch", "remote-preview", "remote"} {
		for _, chunked := range []bool{false, true} {
			transport := "declared"
			if chunked {
				transport = "chunked"
			}
			t.Run(route+"/"+transport, func(t *testing.T) {
				fixture := newSourceWriteBoundaryFixture(t, route)
				body := padSourceWriteBody(fixture.body, fixture.limit+1)
				response := performSourceWriteRequest(
					fixture.router,
					fixture.account.Auth,
					fixture.method,
					fixture.path,
					body,
					chunked,
				)
				assertSourceWriteFlatError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
				assertSourceWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestBookSourceJSONWritesAcceptExactLimitWithTrailingWhitespace(t *testing.T) {
	for _, route := range []string{"create", "update", "batch", "remote-preview", "remote"} {
		t.Run(route, func(t *testing.T) {
			fixture := newSourceWriteBoundaryFixture(t, route)
			body := fixture.body
			if strings.HasPrefix(route, "remote") {
				installSourceBoundaryPayloadTransport(t, `[
					{"bookSourceName":"exact remote source","bookSourceUrl":"https://exact-remote-source.example"}
				]`)
				body = `{"url":"https://exact-source-body.example/bookSources.json"}`
			}
			if len(body) >= fixture.limit {
				t.Fatalf("source %s body length %d exceeds test limit %d", route, len(body), fixture.limit)
			}
			body += strings.Repeat(" ", fixture.limit-len(body))
			response := performSourceWriteRequest(
				fixture.router,
				fixture.account.Auth,
				fixture.method,
				fixture.path,
				body,
				false,
			)
			want := http.StatusOK
			if route == "create" {
				want = http.StatusCreated
			}
			if response.Code != want {
				t.Fatalf(
					"exact-limit source %s = %d %s, want %d",
					route,
					response.Code,
					sourceBoundaryDiagnostic(response.Body.String()),
					want,
				)
			}
		})
	}
}

func TestBookSourceJSONWritesAcceptOnlyOneNonNullObject(t *testing.T) {
	for _, route := range []string{"create", "update", "batch", "remote-preview", "remote"} {
		for _, document := range []struct {
			name string
			body func(sourceWriteBoundaryFixture) string
		}{
			{name: "null", body: func(sourceWriteBoundaryFixture) string { return "null" }},
			{name: "array", body: func(sourceWriteBoundaryFixture) string { return `[]` }},
			{name: "scalar", body: func(sourceWriteBoundaryFixture) string { return `"source"` }},
			{name: "second-json", body: func(fixture sourceWriteBoundaryFixture) string { return fixture.body + `{}` }},
			{name: "trailing-garbage", body: func(fixture sourceWriteBoundaryFixture) string { return fixture.body + `garbage` }},
		} {
			t.Run(route+"/"+document.name, func(t *testing.T) {
				fixture := newSourceWriteBoundaryFixture(t, route)
				response := performSourceWriteRequest(
					fixture.router,
					fixture.account.Auth,
					fixture.method,
					fixture.path,
					document.body(fixture),
					false,
				)
				assertSourceWriteFlatError(t, response, http.StatusBadRequest, fixture.error)
				assertSourceWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestBookSourceWritePrioritizesAuthorizationAndOwnedTargetBeforeBody(t *testing.T) {
	t.Run("authentication-precedes-create-body", func(t *testing.T) {
		router, _ := setupTestServer(t)
		body := &sourceWriteNoReadBody{}
		response := performSourceWriteReaderRequest(router, "", http.MethodPost, "/api/sources", body)
		assertSourceWriteFlatError(t, response, http.StatusUnauthorized, "missing bearer token")
		if body.read {
			t.Fatal("unauthenticated source create body was read")
		}
	})

	for _, target := range []string{"missing", "foreign"} {
		t.Run(target+"-update-target-precedes-body", func(t *testing.T) {
			router, _ := setupTestServer(t)
			account := registerSourceContractAccount(t, router, "sourcepriority")
			sourceID := uint(999999)
			if target == "foreign" {
				other := registerSourceContractAccount(t, router, "sourcepriorityother")
				source := createSourceThroughAPI(t, router, other.Auth, `{
					"name":"foreign source",
					"baseUrl":"https://foreign-priority.example"
				}`)
				sourceID = source.ID
			}
			body := &sourceWriteNoReadBody{}
			response := performSourceWriteReaderRequest(
				router,
				account.Auth,
				http.MethodPut,
				"/api/sources/"+uintString(sourceID),
				body,
			)
			assertSourceWriteFlatError(t, response, http.StatusNotFound, "source not found")
			if body.read {
				t.Fatal("missing or foreign source update body was read")
			}
		})
	}
}

func TestDecodeBookSourcesEnforcesRawEntryLimit(t *testing.T) {
	for _, format := range []string{"array", "bookSources-wrapper", "sources-wrapper"} {
		t.Run(format, func(t *testing.T) {
			exact := sourceBoundaryPayloadDocument(format, testSourceImportEntryLimit)
			sources, err := decodeBookSources([]byte(exact))
			if err != nil {
				t.Fatalf("decode exact source entry limit: %v", err)
			}
			if len(sources) != testSourceImportEntryLimit {
				t.Fatalf("decoded exact source entries = %d, want %d", len(sources), testSourceImportEntryLimit)
			}

			_, err = decodeBookSources([]byte(sourceBoundaryPayloadDocument(format, testSourceImportEntryLimit+1)))
			if err == nil || err.Error() != "too many sources" {
				t.Fatalf("decode over source entry limit = %v, want too many sources", err)
			}
		})
	}
}

func TestBookSourceImportsRejectRawEntryOverflowWithoutSideEffects(t *testing.T) {
	payload := sourceBoundaryPayloadArray(testSourceImportEntryLimit + 1)

	t.Run("local", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourceentrylocal")
		before := snapshotSourceWriteState(t, server, account.ID)
		events := server.hub.AddClient(account.ID, nil).Send
		response := importSourcesThroughAPI(t, router, account.Auth, payload)
		assertSourceWriteFlatError(t, response, http.StatusBadRequest, "too many sources")
		assertSourceWriteSnapshotAndEvents(t, server, account.ID, before, events)
	})

	for _, route := range []string{"remote-preview", "remote"} {
		t.Run(route, func(t *testing.T) {
			installSourceBoundaryPayloadTransport(t, payload)
			router, server := setupTestServer(t)
			account := registerSourceContractAccount(t, router, "sourceentry"+strings.ReplaceAll(route, "-", ""))
			before := snapshotSourceWriteState(t, server, account.ID)
			events := server.hub.AddClient(account.ID, nil).Send
			response := performSourceWriteRequest(
				router,
				account.Auth,
				http.MethodPost,
				"/api/sources/"+route,
				`{"url":"https://source-entry-limit.example/bookSources.json"}`,
				false,
			)
			assertSourceWriteFlatError(t, response, http.StatusBadRequest, "too many sources")
			assertSourceWriteSnapshotAndEvents(t, server, account.ID, before, events)
		})
	}
}

func TestBookSourceLimitRejectsDirectCreateAndAtomicImportOverflow(t *testing.T) {
	t.Run("direct-create", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcelimitcreate")
		setSourceLimit(t, server, account.ID, 1)
		first := createSourceThroughAPI(t, router, account.Auth, `{
			"name":"first limited source",
			"baseUrl":"https://limit-first.example"
		}`)
		events := server.hub.AddClient(account.ID, nil).Send
		failure := seedSourceBoundaryFailure(t, server, account.ID, first)
		before := snapshotSourceWriteState(t, server, account.ID)

		response := performSourceWriteRequest(
			router,
			account.Auth,
			http.MethodPost,
			"/api/sources",
			`{"name":"second limited source","baseUrl":"https://limit-second.example"}`,
			false,
		)
		assertSourceWriteFlatError(t, response, http.StatusConflict, "source limit exceeded")
		assertSourceWriteSnapshotAndEvents(t, server, account.ID, before, events)
		assertSourceBoundaryFailureExists(t, server, failure.ID)
	})

	t.Run("projected-import-rolls-back-update-and-create", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcelimitimport")
		setSourceLimit(t, server, account.ID, 1)
		existing := createSourceThroughAPI(t, router, account.Auth, `{
			"name":"existing before overflow",
			"baseUrl":"https://limit-existing.example"
		}`)
		failure := seedSourceBoundaryFailure(t, server, account.ID, existing)
		book := models.Book{
			UserID: account.ID, SourceID: existing.ID, Title: "quota variable book", Variable: `{"book":"retain"}`,
		}
		if err := server.db.Create(&book).Error; err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&models.Chapter{
			BookID: book.ID, Index: 0, Title: "quota variable chapter", Variable: `{"chapter":"retain"}`,
		}).Error; err != nil {
			t.Fatal(err)
		}
		before := snapshotSourceWriteState(t, server, account.ID)
		events := server.hub.AddClient(account.ID, nil).Send

		response := importSourcesThroughAPI(t, router, account.Auth, `[
			{"bookSourceName":"existing changed by rejected batch","bookSourceUrl":"https://limit-existing.example"},
			{"bookSourceName":"new overflow source","bookSourceUrl":"https://limit-new.example"}
		]`)
		assertSourceWriteFlatError(t, response, http.StatusConflict, "source limit exceeded")
		assertSourceWriteSnapshotAndEvents(t, server, account.ID, before, events)
		assertSourceBoundaryFailureExists(t, server, failure.ID)
	})
}

func TestBookSourceLimitAllowsExistingIdentityUpdateAndFullQuotaCOW(t *testing.T) {
	t.Run("existing-identity-import", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcelimitexisting")
		setSourceLimit(t, server, account.ID, 1)
		createSourceThroughAPI(t, router, account.Auth, `{
			"name":"existing before",
			"baseUrl":"https://limit-update.example"
		}`)
		response := importSourcesThroughAPI(t, router, account.Auth, `[
			{"bookSourceName":"existing after","bookSourceUrl":"https://limit-update.example"}
		]`)
		if response.Code != http.StatusOK {
			t.Fatalf("full-quota existing import = %d %s", response.Code, response.Body.String())
		}
		assertActiveSourceCount(t, server, account.ID, 1)
	})

	t.Run("copy-on-write", func(t *testing.T) {
		router, server := setupTestServer(t)
		owner := registerSourceContractAccount(t, router, "sourcelimitcowowner")
		limited := registerSourceContractAccount(t, router, "sourcelimitcowlimited")
		shared := createSourceThroughAPI(t, router, owner.Auth, `{
			"name":"shared before",
			"baseUrl":"https://limit-cow.example",
			"enabled":true
		}`)
		if err := server.db.Create(&models.UserBookSource{UserID: limited.ID, SourceID: shared.ID}).Error; err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&models.BookSourceNamespace{UserID: limited.ID}).Error; err != nil {
			t.Fatal(err)
		}
		setSourceLimit(t, server, limited.ID, 1)

		response := performSourceWriteRequest(
			router,
			limited.Auth,
			http.MethodPut,
			"/api/sources/"+uintString(shared.ID),
			`{"name":"shared limited after","baseUrl":"https://limit-cow.example","enabled":true}`,
			false,
		)
		if response.Code != http.StatusOK {
			t.Fatalf("full-quota COW update = %d %s", response.Code, response.Body.String())
		}
		assertActiveSourceCount(t, server, limited.ID, 1)
		assertActiveSourceCount(t, server, owner.ID, 1)
	})
}

func TestBookSourceLimitSerializesConcurrentFinalSlot(t *testing.T) {
	for _, scenario := range []string{"create-create", "create-import", "import-import"} {
		t.Run(scenario, func(t *testing.T) {
			router, server := setupTestServer(t)
			account := registerSourceContractAccount(t, router, "sourceconcurrent"+strings.ReplaceAll(scenario, "-", ""))
			_ = sourceList(t, router, account.Auth)
			setSourceLimit(t, server, account.ID, 1)
			events := server.hub.AddClient(account.ID, nil).Send

			operations := sourceBoundaryConcurrentOperations(t, router, account.Auth, scenario)
			start := make(chan struct{})
			codes := make(chan int, len(operations))
			var wait sync.WaitGroup
			for _, operation := range operations {
				operation := operation
				wait.Add(1)
				go func() {
					defer wait.Done()
					<-start
					codes <- operation()
				}()
			}
			close(start)
			wait.Wait()
			close(codes)

			var got []int
			for code := range codes {
				got = append(got, code)
			}
			sort.Ints(got)
			if len(got) != 2 || (got[0] != http.StatusOK && got[0] != http.StatusCreated) || got[1] != http.StatusConflict {
				t.Errorf("concurrent %s statuses = %v, want one success and one 409", scenario, got)
			}
			assertActiveSourceCount(t, server, account.ID, 1)
			if emitted := drainBookWriteEvents(events); len(emitted) != 1 {
				t.Errorf("concurrent %s events = %v, want one", scenario, emitted)
			}
			var sourceRows int64
			if err := server.db.Model(&models.BookSource{}).Count(&sourceRows).Error; err != nil {
				t.Fatal(err)
			}
			if sourceRows != 1 {
				t.Errorf("concurrent %s source rows = %d, want 1", scenario, sourceRows)
			}
		})
	}
}

func TestBookSourceLimitPreservesUnlimitedHistoricalAndDetachedSemantics(t *testing.T) {
	t.Run("zero-is-unlimited", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcelimitunlimited")
		setSourceLimit(t, server, account.ID, 0)
		for index := 1; index <= 2; index++ {
			createSourceThroughAPI(t, router, account.Auth, fmt.Sprintf(`{
				"name":"unlimited source %d",
				"baseUrl":"https://unlimited-%d.example"
			}`, index, index))
		}
		assertActiveSourceCount(t, server, account.ID, 2)
	})

	t.Run("historical-over-limit-can-update-but-not-add", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcelimithistorical")
		first := createSourceThroughAPI(t, router, account.Auth, `{
			"name":"historical first",
			"baseUrl":"https://historical-first.example",
			"enabled":true
		}`)
		createSourceThroughAPI(t, router, account.Auth, `{
			"name":"historical second",
			"baseUrl":"https://historical-second.example",
			"enabled":true
		}`)
		setSourceLimit(t, server, account.ID, 1)

		update := performSourceWriteRequest(
			router,
			account.Auth,
			http.MethodPut,
			"/api/sources/"+uintString(first.ID),
			`{"name":"historical first updated","baseUrl":"https://historical-first.example","enabled":true}`,
			false,
		)
		if update.Code != http.StatusOK {
			t.Fatalf("historical over-limit update = %d %s", update.Code, update.Body.String())
		}

		create := performSourceWriteRequest(
			router,
			account.Auth,
			http.MethodPost,
			"/api/sources",
			`{"name":"historical rejected add","baseUrl":"https://historical-rejected.example"}`,
			false,
		)
		assertSourceWriteFlatError(t, create, http.StatusConflict, "source limit exceeded")
		assertActiveSourceCount(t, server, account.ID, 2)
	})

	t.Run("detached-association-does-not-consume", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcelimitdetached")
		detached := createSourceThroughAPI(t, router, account.Auth, `{
			"name":"detached historical source",
			"baseUrl":"https://detached-historical.example"
		}`)
		if err := server.db.Model(&models.UserBookSource{}).
			Where("user_id = ? AND source_id = ?", account.ID, detached.ID).
			Update("detached", true).Error; err != nil {
			t.Fatal(err)
		}
		setSourceLimit(t, server, account.ID, 1)

		createSourceThroughAPI(t, router, account.Auth, `{
			"name":"active after detached",
			"baseUrl":"https://active-after-detached.example"
		}`)
		assertActiveSourceCount(t, server, account.ID, 1)
	})
}

func TestBookSourceNoOpImportAndBatchPreserveFailureAndDoNotBroadcast(t *testing.T) {
	for _, operation := range []string{"empty-import", "foreign-only-batch", "used-only-delete"} {
		t.Run(operation, func(t *testing.T) {
			router, server := setupTestServer(t)
			account := registerSourceContractAccount(t, router, "sourcenoop"+strings.ReplaceAll(operation, "-", ""))
			source := createSourceThroughAPI(t, router, account.Auth, `{
				"name":"no-op retained source",
				"baseUrl":"https://no-op-retained.example",
				"enabled":true
			}`)
			if operation == "used-only-delete" {
				book := models.Book{UserID: account.ID, SourceID: source.ID, Title: "retained used source"}
				if err := server.db.Create(&book).Error; err != nil {
					t.Fatal(err)
				}
			}
			failure := seedSourceBoundaryFailure(t, server, account.ID, source)
			before := snapshotSourceWriteState(t, server, account.ID)
			events := server.hub.AddClient(account.ID, nil).Send

			var response *httptest.ResponseRecorder
			if operation == "empty-import" {
				response = importSourcesThroughAPI(t, router, account.Auth, `[]`)
			} else {
				sourceID := uint(999999)
				action := "disable"
				if operation == "used-only-delete" {
					sourceID = source.ID
					action = "delete"
				}
				response = performSourceWriteRequest(
					router,
					account.Auth,
					http.MethodPost,
					"/api/sources/batch",
					fmt.Sprintf(`{"action":%q,"sourceIds":[%d]}`, action, sourceID),
					false,
				)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("%s = %d %s", operation, response.Code, response.Body.String())
			}
			assertSourceWriteSnapshotAndEvents(t, server, account.ID, before, events)
			assertSourceBoundaryFailureExists(t, server, failure.ID)
		})
	}
}

func sourceBoundaryConcurrentOperations(
	t *testing.T,
	router http.Handler,
	auth string,
	scenario string,
) []func() int {
	t.Helper()
	create := func(index int) func() int {
		return func() int {
			response := performSourceWriteRequest(
				router,
				auth,
				http.MethodPost,
				"/api/sources",
				fmt.Sprintf(`{"name":"concurrent create %d","baseUrl":"https://concurrent-create-%d.example"}`, index, index),
				false,
			)
			return response.Code
		}
	}
	importOne := func(index int) func() int {
		return func() int {
			response := importSourcesThroughAPI(t, router.(*gin.Engine), auth, fmt.Sprintf(`[
				{"bookSourceName":"concurrent import %d","bookSourceUrl":"https://concurrent-import-%d.example"}
			]`, index, index))
			return response.Code
		}
	}

	switch scenario {
	case "create-create":
		return []func() int{create(1), create(2)}
	case "create-import":
		return []func() int{create(1), importOne(2)}
	case "import-import":
		return []func() int{importOne(1), importOne(2)}
	default:
		t.Fatalf("unknown concurrent source-limit scenario %q", scenario)
		return nil
	}
}

func installSourceBoundaryFailingTransport(t *testing.T) {
	t.Helper()
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("source boundary transport should not succeed")
	})})
	t.Cleanup(restore)
}

func installSourceBoundaryPayloadTransport(t *testing.T, payload string) {
	t.Helper()
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})})
	t.Cleanup(restore)
}

func sourceBoundaryPayloadArray(count int) string {
	const entry = `{"bookSourceName":"entry","bookSourceUrl":"https://entry-limit.example"}`
	var body strings.Builder
	body.Grow(2 + count*(len(entry)+1))
	body.WriteByte('[')
	for index := 0; index < count; index++ {
		if index > 0 {
			body.WriteByte(',')
		}
		body.WriteString(entry)
	}
	body.WriteByte(']')
	return body.String()
}

func sourceBoundaryPayloadDocument(format string, count int) string {
	payload := sourceBoundaryPayloadArray(count)
	switch format {
	case "array":
		return payload
	case "bookSources-wrapper":
		return `{"bookSources":` + payload + `}`
	case "sources-wrapper":
		return `{"sources":` + payload + `}`
	default:
		panic("unknown source boundary payload format " + format)
	}
}

func padSourceWriteBody(body string, total int) string {
	prefix := strings.TrimSuffix(body, "}") + `,"padding":"`
	suffix := `"}`
	if total < len(prefix)+len(suffix) {
		panic("source write contract target body is too small")
	}
	return prefix + strings.Repeat("p", total-len(prefix)-len(suffix)) + suffix
}

func performSourceWriteRequest(
	router http.Handler,
	auth, method, path, body string,
	chunked bool,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performSourceWriteReaderRequest(
	router http.Handler,
	auth, method, path string,
	body io.Reader,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertSourceWriteFlatError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	want := fmt.Sprintf(`{"error":%q}`, message)
	if response.Code != status || response.Body.String() != want {
		t.Errorf(
			"source write error = %d %s, want %d %s",
			response.Code,
			sourceBoundaryDiagnostic(response.Body.String()),
			status,
			want,
		)
	}
}

func snapshotSourceWriteState(t *testing.T, server *Server, userID uint) []byte {
	t.Helper()
	var snapshot struct {
		Sources      []models.BookSource
		Associations []models.UserBookSource
		Failures     []models.SourceFailure
		Books        []models.Book
		Chapters     []models.Chapter
	}
	if err := server.db.Order("id asc").Find(&snapshot.Sources).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("source_id asc").Find(&snapshot.Associations).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Failures).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Books).Error; err != nil {
		t.Fatal(err)
	}
	bookIDs := make([]uint, 0, len(snapshot.Books))
	for _, book := range snapshot.Books {
		bookIDs = append(bookIDs, book.ID)
	}
	if len(bookIDs) > 0 {
		if err := server.db.Where("book_id IN ?", bookIDs).Order("id asc").Find(&snapshot.Chapters).Error; err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertSourceWriteFailureStable(t *testing.T, fixture sourceWriteBoundaryFixture) {
	t.Helper()
	assertSourceWriteSnapshotAndEvents(t, fixture.server, fixture.account.ID, fixture.before, fixture.events)
}

func assertSourceWriteSnapshotAndEvents(
	t *testing.T,
	server *Server,
	userID uint,
	before []byte,
	events <-chan []byte,
) {
	t.Helper()
	after := snapshotSourceWriteState(t, server, userID)
	if !bytes.Equal(after, before) {
		t.Errorf(
			"rejected or no-op source write changed persistence\nbefore=%s\nafter=%s",
			sourceBoundaryDiagnostic(string(before)),
			sourceBoundaryDiagnostic(string(after)),
		)
	}
	if emitted := drainBookWriteEvents(events); len(emitted) != 0 {
		t.Errorf("rejected or no-op source write broadcast events: %v", emitted)
	}
}

func sourceBoundaryDiagnostic(value string) string {
	const limit = 512
	if len(value) <= limit {
		return value
	}
	return fmt.Sprintf("%s... (%d bytes total)", value[:limit], len(value))
}

func setSourceLimit(t *testing.T, server *Server, userID uint, limit int) {
	t.Helper()
	if err := server.db.Model(&models.User{}).Where("id = ?", userID).Update("source_limit", limit).Error; err != nil {
		t.Fatal(err)
	}
}

func assertActiveSourceCount(t *testing.T, server *Server, userID uint, want int64) {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND detached = ?", userID, false).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Errorf("active source count = %d, want %d", count, want)
	}
}

func seedSourceBoundaryFailure(
	t *testing.T,
	server *Server,
	userID uint,
	source models.BookSource,
) models.SourceFailure {
	t.Helper()
	failure := models.SourceFailure{
		UserID:    userID,
		SourceID:  source.ID,
		SourceURL: source.BaseURL,
		Message:   "retain source failure",
		FailedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := server.db.Create(&failure).Error; err != nil {
		t.Fatal(err)
	}
	return failure
}

func assertSourceBoundaryFailureExists(t *testing.T, server *Server, failureID uint) {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.SourceFailure{}).Where("id = ?", failureID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("source failure %d count = %d, want 1", failureID, count)
	}
}

type sourceWriteNoReadBody struct {
	read bool
}

func (body *sourceWriteNoReadBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("request body must not be read")
}

var _ io.Reader = (*sourceWriteNoReadBody)(nil)
