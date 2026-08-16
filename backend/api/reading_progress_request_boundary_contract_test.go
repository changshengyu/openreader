package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
	readersync "openreader/backend/sync"
)

const progressBoundaryRequestBytes = 16 << 10

type progressBoundaryFixture struct {
	router     http.Handler
	server     *Server
	auth       string
	user       models.User
	book       models.Book
	chapter    models.Chapter
	client     *readersync.Client
	mirrorDir  string
	mirrorPath string
}

func newProgressBoundaryFixture(t *testing.T) progressBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book, chapters := progressContractBook(t, server, user, "请求边界进度", "第一章")
	mirrorDir := filepath.Join(server.webdavDir(), "bookProgress")
	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	client := server.hub.AddClient(user.ID, nil)
	t.Cleanup(func() { server.hub.RemoveClient(client) })
	return progressBoundaryFixture{
		router:     router,
		server:     server,
		auth:       auth,
		user:       user,
		book:       book,
		chapter:    chapters[0],
		client:     client,
		mirrorDir:  mirrorDir,
		mirrorPath: filepath.Join(mirrorDir, engine.SafeBookFolderName(book.Title, book.Author)+".json"),
	}
}

func progressBoundaryRequest(
	t *testing.T,
	fixture progressBoundaryFixture,
	body []byte,
	contentLength int64,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/api/progress", bytes.NewReader(body))
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	request.ContentLength = contentLength
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func progressBoundaryPayload(bookID uint, extra string) []byte {
	return []byte(fmt.Sprintf(`{"bookId":%d,"chapterIndex":0%s}`, bookID, extra))
}

func paddedProgressBoundaryPayload(t *testing.T, bookID uint, size int) []byte {
	t.Helper()
	prefix := fmt.Sprintf(`{"bookId":%d,"chapterIndex":0,"padding":"`, bookID)
	suffix := `"}`
	padding := size - len(prefix) - len(suffix)
	if padding < 0 {
		t.Fatalf("requested progress payload size %d is too small", size)
	}
	return []byte(prefix + strings.Repeat("a", padding) + suffix)
}

func assertProgressBoundaryRejectedWithoutSideEffects(
	t *testing.T,
	fixture progressBoundaryFixture,
	response *httptest.ResponseRecorder,
	wantStatus int,
	wantError string,
) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, response.Code, response.Body.String())
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode progress error: %v", err)
	}
	if envelope.Error != wantError {
		t.Fatalf("expected error %q, got %q", wantError, envelope.Error)
	}
	var count int64
	if err := fixture.server.db.Model(&models.ReadingProgress{}).
		Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected progress request created %d durable rows", count)
	}
	if _, err := os.Stat(fixture.mirrorPath); !os.IsNotExist(err) {
		t.Fatalf("rejected progress request changed mirror %q: %v", fixture.mirrorPath, err)
	}
	if got := len(fixture.client.Send); got != 0 {
		t.Fatalf("rejected progress request emitted %d Hub events", got)
	}
}

type progressBoundaryTrackingBody struct {
	reads int
}

func (body *progressBoundaryTrackingBody) Read([]byte) (int, error) {
	body.reads++
	return 0, io.EOF
}

func TestProgressBoundaryAuthenticationPrecedesBodyAdmission(t *testing.T) {
	router, _ := setupTestServer(t)
	for _, test := range []struct {
		name string
		auth string
		want string
	}{
		{name: "missing token", want: "missing bearer token"},
		{name: "invalid token", auth: "Bearer invalid", want: "invalid token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &progressBoundaryTrackingBody{}
			request := httptest.NewRequest(http.MethodPut, "/api/progress", body)
			request.ContentLength = progressBoundaryRequestBytes + 1
			request.Header.Set("Content-Type", "application/json")
			if test.auth != "" {
				request.Header.Set("Authorization", test.auth)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("auth-first response: got %d %s", response.Code, response.Body.String())
			}
			if body.reads != 0 {
				t.Fatalf("unauthorized request body was read %d times", body.reads)
			}
		})
	}
}

func TestProgressBoundaryEnforcesDeclaredAndActualBodyLimits(t *testing.T) {
	t.Run("exact boundary", func(t *testing.T) {
		fixture := newProgressBoundaryFixture(t)
		body := paddedProgressBoundaryPayload(t, fixture.book.ID, progressBoundaryRequestBytes)
		response := progressBoundaryRequest(t, fixture, body, int64(len(body)))
		if response.Code != http.StatusOK {
			t.Fatalf("exact 16 KiB request: got %d %s", response.Code, response.Body.String())
		}
	})

	for _, test := range []struct {
		name          string
		bodySize      int
		contentLength int64
	}{
		{name: "declared overflow", bodySize: 128, contentLength: progressBoundaryRequestBytes + 1},
		{name: "actual chunked overflow", bodySize: progressBoundaryRequestBytes + 1, contentLength: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProgressBoundaryFixture(t)
			body := paddedProgressBoundaryPayload(t, fixture.book.ID, test.bodySize)
			response := progressBoundaryRequest(t, fixture, body, test.contentLength)
			assertProgressBoundaryRejectedWithoutSideEffects(
				t, fixture, response, http.StatusRequestEntityTooLarge, "request body too large",
			)
		})
	}
}

func TestProgressBoundaryRejectsAmbiguousWireAndUnsafeControlFields(t *testing.T) {
	tests := []struct {
		name    string
		payload func(progressBoundaryFixture) []byte
	}{
		{
			name: "empty body",
			payload: func(progressBoundaryFixture) []byte {
				return nil
			},
		},
		{
			name: "null body",
			payload: func(progressBoundaryFixture) []byte {
				return []byte("null")
			},
		},
		{
			name: "array body",
			payload: func(progressBoundaryFixture) []byte {
				return []byte("[]")
			},
		},
		{
			name: "scalar body",
			payload: func(progressBoundaryFixture) []byte {
				return []byte("1")
			},
		},
		{
			name: "malformed body",
			payload: func(progressBoundaryFixture) []byte {
				return []byte(`{"bookId":`)
			},
		},
		{
			name: "trailing JSON value",
			payload: func(fixture progressBoundaryFixture) []byte {
				first := progressBoundaryPayload(fixture.book.ID, "")
				return append(first, []byte(` {"ignored":true}`)...)
			},
		},
		{
			name: "invalid UTF-8",
			payload: func(fixture progressBoundaryFixture) []byte {
				prefix := []byte(fmt.Sprintf(`{"bookId":%d,"chapterIndex":0,"padding":"`, fixture.book.ID))
				return append(append(prefix, 0xff), []byte(`"}`)...)
			},
		},
		{
			name: "missing book ID",
			payload: func(progressBoundaryFixture) []byte {
				return []byte(`{"chapterIndex":0}`)
			},
		},
		{
			name: "missing chapter index",
			payload: func(fixture progressBoundaryFixture) []byte {
				return []byte(fmt.Sprintf(`{"bookId":%d}`, fixture.book.ID))
			},
		},
		{
			name: "mode exceeds model boundary",
			payload: func(fixture progressBoundaryFixture) []byte {
				return progressBoundaryPayload(fixture.book.ID, `,"mode":"`+strings.Repeat("m", 21)+`"`)
			},
		},
		{
			name: "client ID exceeds event boundary",
			payload: func(fixture progressBoundaryFixture) []byte {
				return progressBoundaryPayload(fixture.book.ID, `,"clientId":"`+strings.Repeat("c", 129)+`"`)
			},
		},
		{
			name: "client ID exceeds event boundary in UTF-8 bytes",
			payload: func(fixture progressBoundaryFixture) []byte {
				return progressBoundaryPayload(fixture.book.ID, `,"clientId":"`+strings.Repeat("界", 43)+`"`)
			},
		},
		{
			name: "invalid base timestamp",
			payload: func(fixture progressBoundaryFixture) []byte {
				return progressBoundaryPayload(fixture.book.ID, `,"baseUpdatedAt":"not-a-time"`)
			},
		},
		{
			name: "invalid client timestamp",
			payload: func(fixture progressBoundaryFixture) []byte {
				return progressBoundaryPayload(fixture.book.ID, `,"clientUpdatedAt":"not-a-time"`)
			},
		},
		{
			name: "timestamp exceeds boundary",
			payload: func(fixture progressBoundaryFixture) []byte {
				return progressBoundaryPayload(fixture.book.ID, `,"baseUpdatedAt":"`+strings.Repeat("t", 65)+`"`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProgressBoundaryFixture(t)
			body := test.payload(fixture)
			response := progressBoundaryRequest(t, fixture, body, int64(len(body)))
			assertProgressBoundaryRejectedWithoutSideEffects(
				t, fixture, response, http.StatusBadRequest, "invalid progress payload",
			)
		})
	}
}

func TestProgressBoundaryAcceptsExactFieldLimitsAndPreservesEventClientID(t *testing.T) {
	fixture := newProgressBoundaryFixture(t)
	clientID := strings.Repeat("c", 128)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	body := progressBoundaryPayload(
		fixture.book.ID,
		`,"mode":"`+strings.Repeat("m", 20)+`","clientId":"`+clientID+`","baseUpdatedAt":"`+now+`","clientUpdatedAt":"`+now+`"`,
	)
	response := progressBoundaryRequest(t, fixture, body, int64(len(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("exact field limits: got %d %s", response.Code, response.Body.String())
	}
	select {
	case raw := <-fixture.client.Send:
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				ClientID string `json:"clientId"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "progress_update" || event.Payload.ClientID != clientID {
			t.Fatalf("unexpected bounded progress event: %+v", event)
		}
	default:
		t.Fatal("accepted progress did not broadcast")
	}
}

func TestProgressBoundaryInvalidTimestampCannotBypassExistingCAS(t *testing.T) {
	fixture := newProgressBoundaryFixture(t)
	initial := progressBoundaryPayload(fixture.book.ID, `,"offset":7`)
	initialResponse := progressBoundaryRequest(t, fixture, initial, int64(len(initial)))
	if initialResponse.Code != http.StatusOK {
		t.Fatalf("seed progress: got %d %s", initialResponse.Code, initialResponse.Body.String())
	}
	<-fixture.client.Send
	before, err := os.ReadFile(fixture.mirrorPath)
	if err != nil {
		t.Fatal(err)
	}

	invalid := progressBoundaryPayload(
		fixture.book.ID,
		`,"offset":99,"baseUpdatedAt":"invalid-base","clientUpdatedAt":"invalid-client"`,
	)
	response := progressBoundaryRequest(t, fixture, invalid, int64(len(invalid)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid progress payload") {
		t.Fatalf("invalid CAS timestamp: got %d %s", response.Code, response.Body.String())
	}
	var stored models.ReadingProgress
	if err := fixture.server.db.Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Offset != 7 {
		t.Fatalf("invalid timestamps overwrote committed progress: %+v", stored)
	}
	after, err := os.ReadFile(fixture.mirrorPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("invalid timestamps changed the WebDAV mirror")
	}
	if got := len(fixture.client.Send); got != 0 {
		t.Fatalf("invalid timestamps emitted %d Hub events", got)
	}
}
