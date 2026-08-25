package api

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

type backupRestoreMultipartPart struct {
	field    string
	filename string
	data     []byte
}

type backupRestoreBodyTracker struct {
	reads int
}

func (tracker *backupRestoreBodyTracker) Read(_ []byte) (int, error) {
	tracker.reads++
	return 0, io.EOF
}

func makeBackupRestoreMultipartRequest(
	t *testing.T,
	auth string,
	parts []backupRestoreMultipartPart,
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		var target io.Writer
		var err error
		if part.filename == "" {
			target, err = writer.CreateFormField(part.field)
		} else {
			target, err = writer.CreateFormFile(part.field, part.filename)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := target.Write(part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", auth)
	t.Cleanup(func() {
		if request.MultipartForm != nil {
			_ = request.MultipartForm.RemoveAll()
		}
	})
	return request
}

func backupRestoreBoundaryZIP(t *testing.T, title string) []byte {
	t.Helper()
	return makeBackupRestoreZIP(t, map[string]string{
		"myBookShelf.json": `[{"name":"` + title + `","bookUrl":"https://backup-wire.example/` + title + `"}]`,
	})
}

func assertBackupRestoreBoundaryError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if response.Code != status {
		t.Errorf("status = %d, want %d: %s", response.Code, status, response.Body.String())
		return
	}
	if !strings.Contains(response.Body.String(), `"error":"`+message+`"`) {
		t.Errorf("response error = %s, want %q", response.Body.String(), message)
	}
}

func backupRestoreBookCount(t *testing.T, server *Server) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.Book{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestBackupRestoreMultipartRejectsAmbiguousPartsBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		parts   func([]byte) []backupRestoreMultipartPart
		message string
	}{
		{
			name: "scalar part",
			parts: func(archive []byte) []backupRestoreMultipartPart {
				return []backupRestoreMultipartPart{
					{field: "file", filename: "backup.zip", data: archive},
					{field: "ignored", data: []byte("value")},
				}
			},
			message: "invalid backup upload",
		},
		{
			name: "duplicate file part",
			parts: func(archive []byte) []backupRestoreMultipartPart {
				return []backupRestoreMultipartPart{
					{field: "file", filename: "backup.zip", data: archive},
					{field: "file", filename: "second.zip", data: []byte("ignored")},
				}
			},
			message: "invalid backup upload",
		},
		{
			name: "other file field",
			parts: func(archive []byte) []backupRestoreMultipartPart {
				return []backupRestoreMultipartPart{
					{field: "file", filename: "backup.zip", data: archive},
					{field: "other", filename: "ignored.bin", data: []byte("ignored")},
				}
			},
			message: "invalid backup upload",
		},
		{
			name: "oversized filename",
			parts: func(archive []byte) []backupRestoreMultipartPart {
				return []backupRestoreMultipartPart{{
					field:    "file",
					filename: strings.Repeat("a", 252) + ".zip",
					data:     archive,
				}}
			},
			message: "backup file must be a zip archive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			before := backupRestoreBookCount(t, server)
			archive := backupRestoreBoundaryZIP(t, "rejected "+test.name)
			request := makeBackupRestoreMultipartRequest(t, auth, test.parts(archive))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			assertBackupRestoreBoundaryError(t, response, http.StatusBadRequest, test.message)
			if after := backupRestoreBookCount(t, server); after != before {
				t.Errorf("rejected multipart changed books: before=%d after=%d", before, after)
			}
			if staged := countFilesBelow(t, server.cfg.CacheDir); staged != 0 {
				t.Errorf("rejected multipart left %d cache/stage files", staged)
			}
		})
	}
}

func TestBackupRestoreMultipartOwnsTemporaryFiles(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		data       func(*testing.T) []byte
		extra      []backupRestoreMultipartPart
		maxBytes   int64
		wantStatus int
	}{
		{
			name:       "success",
			filename:   "backup.zip",
			data:       func(t *testing.T) []byte { return backupRestoreBoundaryZIP(t, "temp success") },
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid shape",
			filename:   "backup.zip",
			data:       func(t *testing.T) []byte { return backupRestoreBoundaryZIP(t, "temp shape") },
			extra:      []backupRestoreMultipartPart{{field: "ignored", data: []byte("value")}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "wrong extension",
			filename:   "backup.txt",
			data:       func(t *testing.T) []byte { return backupRestoreBoundaryZIP(t, "temp extension") },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized file",
			filename:   "backup.zip",
			data:       func(t *testing.T) []byte { return backupRestoreBoundaryZIP(t, "temp oversized") },
			maxBytes:   8,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "invalid zip",
			filename:   "backup.zip",
			data:       func(*testing.T) []byte { return bytes.Repeat([]byte("not-a-zip"), 64) },
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempRoot := t.TempDir()
			t.Setenv("TMPDIR", tempRoot)
			router, _ := setupTestServerWithConfig(t, func(cfg *config.Config) {
				if test.maxBytes > 0 {
					cfg.MaxBackupRestoreBytes = test.maxBytes
				}
			})
			router.MaxMultipartMemory = 1
			auth := authHeader(t, router)
			parts := []backupRestoreMultipartPart{{field: "file", filename: test.filename, data: test.data(t)}}
			parts = append(parts, test.extra...)
			request := makeBackupRestoreMultipartRequest(t, auth, parts)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Errorf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			entries, err := os.ReadDir(tempRoot)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Errorf("multipart temporary files remain after handler return: %+v", entries)
			}
		})
	}
}

func TestBackupRestoreMultipartPreservesAuthAndValidZIPBehavior(t *testing.T) {
	t.Run("unauthenticated body unread", func(t *testing.T) {
		router, server := setupTestServer(t)
		tracker := &backupRestoreBodyTracker{}
		request := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", tracker)
		request.Header.Set("Content-Type", "multipart/form-data; boundary=unread")
		request.ContentLength = server.portableLimits().maxCompressed + backupMultipartEnvelopeBytes + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", response.Code, response.Body.String())
		}
		if tracker.reads != 0 {
			t.Fatalf("unauthenticated request body read %d times", tracker.reads)
		}
	})

	t.Run("forbidden body unread", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)
		var user models.User
		if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
			t.Fatal(err)
		}
		denied := false
		if err := server.db.Model(&models.User{}).Where("id = ?", user.ID).
			Update("can_access_webdav", &denied).Error; err != nil {
			t.Fatal(err)
		}
		tracker := &backupRestoreBodyTracker{}
		request := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", tracker)
		request.Header.Set("Authorization", auth)
		request.Header.Set("Content-Type", "multipart/form-data; boundary=unread")
		request.ContentLength = server.portableLimits().maxCompressed + backupMultipartEnvelopeBytes + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", response.Code, response.Body.String())
		}
		if tracker.reads != 0 {
			t.Fatalf("forbidden request body read %d times", tracker.reads)
		}
	})

	t.Run("uppercase zip remains valid", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)
		request := makeBackupRestoreMultipartRequest(t, auth, []backupRestoreMultipartPart{{
			field:    "file",
			filename: "backup.ZIP",
			data:     backupRestoreBoundaryZIP(t, "uppercase zip"),
		}})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
		}
		if count := backupRestoreBookCount(t, server); count != 1 {
			t.Fatalf("uppercase ZIP restored %d books, want 1", count)
		}
	})
}
