package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

const testBookSourceImportMultipartEnvelopeBytes int64 = 1 << 20

type bookSourceMultipartFile struct {
	field    string
	filename string
	data     []byte
}

func TestBookSourceLocalImportRejectsAmbiguousMultipartBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		files  []bookSourceMultipartFile
		values map[string]string
	}{
		{
			name: "duplicate file field",
			files: []bookSourceMultipartFile{
				{field: "file", filename: "first.json", data: sourceImportFixture("duplicate-first")},
				{field: "file", filename: "second.json", data: sourceImportFixture("duplicate-second")},
			},
		},
		{
			name: "foreign file field",
			files: []bookSourceMultipartFile{
				{field: "file", filename: "first.json", data: sourceImportFixture("foreign-first")},
				{field: "attachment", filename: "second.json", data: sourceImportFixture("foreign-second")},
			},
		},
		{
			name: "scalar metadata",
			files: []bookSourceMultipartFile{
				{field: "file", filename: "first.json", data: sourceImportFixture("scalar-first")},
			},
			values: map[string]string{"note": "ignored by the old handler"},
		},
		{
			name: "scalar file alias",
			files: []bookSourceMultipartFile{
				{field: "file", filename: "first.json", data: sourceImportFixture("scalar-file-first")},
			},
			values: map[string]string{"file": "ambiguous"},
		},
		{
			name: "foreign file only",
			files: []bookSourceMultipartFile{
				{field: "attachment", filename: "only.json", data: sourceImportFixture("foreign-only")},
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			account := registerSourceContractAccount(t, router, fmt.Sprintf("sourcepartshape%d", index))
			response := performBookSourceMultipartImport(t, router, account.Auth, test.files, test.values)
			assertBookSourceImportError(t, response, http.StatusBadRequest, "invalid source import request")

			var associationCount int64
			if err := server.db.Model(&models.UserBookSource{}).
				Where("user_id = ?", account.ID).
				Count(&associationCount).Error; err != nil {
				t.Fatal(err)
			}
			if associationCount != 0 {
				t.Fatalf("ambiguous multipart created %d source associations", associationCount)
			}
		})
	}
}

func TestBookSourceLocalImportMapsDeclaredAndChunkedEnvelopeOverflow(t *testing.T) {
	requestLimit := maxBookSourceImportBytes + testBookSourceImportMultipartEnvelopeBytes

	t.Run("declared overflow is rejected without reading", func(t *testing.T) {
		router, _ := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcedeclaredoverflow")
		body := &bookSourceImportReadTracker{}
		request := httptest.NewRequest(http.MethodPost, "/api/sources/import", body)
		request.Header.Set("Authorization", account.Auth)
		request.Header.Set("Content-Type", "multipart/form-data; boundary=declared")
		request.ContentLength = requestLimit + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		assertBookSourceImportError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
		if body.reads != 0 {
			t.Fatalf("declared overflow consumed request body %d times", body.reads)
		}
	})

	t.Run("chunked actual overflow keeps 413", func(t *testing.T) {
		router, _ := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcechunkedoverflow")
		boundary := "openreader-source-overflow"
		prefix := fmt.Sprintf(
			"--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"bookSources.json\"\r\nContent-Type: application/json\r\n\r\n",
			boundary,
		)
		suffix := fmt.Sprintf("\r\n--%s--\r\n", boundary)
		body := io.MultiReader(
			strings.NewReader(prefix),
			io.LimitReader(zeroBytesReader{}, requestLimit+1),
			strings.NewReader(suffix),
		)
		request := httptest.NewRequest(http.MethodPost, "/api/sources/import", body)
		request.Header.Set("Authorization", account.Auth)
		request.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
		request.ContentLength = -1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		assertBookSourceImportError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
	})
}

func TestBookSourceLocalImportAuthorizationPrecedesMultipartBoundary(t *testing.T) {
	requestLimit := maxBookSourceImportBytes + testBookSourceImportMultipartEnvelopeBytes

	t.Run("missing token", func(t *testing.T) {
		router, _ := setupTestServer(t)
		body := &bookSourceImportReadTracker{}
		request := httptest.NewRequest(http.MethodPost, "/api/sources/import", body)
		request.Header.Set("Content-Type", "multipart/form-data; boundary=unauthorized")
		request.ContentLength = requestLimit + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("missing token = %d %s", response.Code, response.Body.String())
		}
		if body.reads != 0 {
			t.Fatalf("unauthorized request consumed body %d times", body.reads)
		}
	})

	t.Run("source editing disabled", func(t *testing.T) {
		router, server := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcepartforbidden")
		if err := server.db.Model(&models.User{}).
			Where("id = ?", account.ID).
			Update("can_edit_sources", false).Error; err != nil {
			t.Fatal(err)
		}
		body := &bookSourceImportReadTracker{}
		request := httptest.NewRequest(http.MethodPost, "/api/sources/import", body)
		request.Header.Set("Authorization", account.Auth)
		request.Header.Set("Content-Type", "multipart/form-data; boundary=forbidden")
		request.ContentLength = requestLimit + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusForbidden {
			t.Fatalf("disabled source editing = %d %s", response.Code, response.Body.String())
		}
		if body.reads != 0 {
			t.Fatalf("forbidden request consumed body %d times", body.reads)
		}
	})
}

func TestBookSourceLocalImportOwnsMultipartTemporaryFiles(t *testing.T) {
	tests := []struct {
		name       string
		payload    []byte
		wantStatus int
	}{
		{name: "success", payload: sourceImportFixture("temp-success"), wantStatus: http.StatusOK},
		{name: "invalid json", payload: bytes.Repeat([]byte("not-json"), 256), wantStatus: http.StatusBadRequest},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempRoot := t.TempDir()
			t.Setenv("TMPDIR", tempRoot)
			router, _ := setupTestServer(t)
			router.MaxMultipartMemory = 1
			account := registerSourceContractAccount(t, router, fmt.Sprintf("sourcetempowner%d", index))
			response := performBookSourceMultipartImport(t, router, account.Auth, []bookSourceMultipartFile{
				{field: "file", filename: "bookSources.json", data: test.payload},
			}, nil)
			if response.Code != test.wantStatus {
				t.Fatalf("temporary-file case = %d %s", response.Code, response.Body.String())
			}
			entries, err := os.ReadDir(tempRoot)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("multipart temporary files remain after handler return: %+v", entries)
			}
		})
	}
}

func TestBookSourceLocalImportKeepsExactFileByteLimit(t *testing.T) {
	tests := []struct {
		name       string
		size       int64
		wantStatus int
		wantError  string
	}{
		{name: "exact", size: maxBookSourceImportBytes, wantStatus: http.StatusOK},
		{name: "overflow", size: maxBookSourceImportBytes + 1, wantStatus: http.StatusRequestEntityTooLarge, wantError: "source file is too large"},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, _ := setupTestServer(t)
			account := registerSourceContractAccount(t, router, fmt.Sprintf("sourcefilelimit%d", index))
			payload := make([]byte, test.size)
			copy(payload, "[]")
			for offset := 2; offset < len(payload); offset++ {
				payload[offset] = ' '
			}
			response := performBookSourceMultipartImport(t, router, account.Auth, []bookSourceMultipartFile{
				{field: "file", filename: "bookSources.json", data: payload},
			}, nil)
			if test.wantError != "" {
				assertBookSourceImportError(t, response, test.wantStatus, test.wantError)
				return
			}
			if response.Code != test.wantStatus {
				t.Fatalf("file limit = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestBookSourceLocalImportMalformedMultipartErrors(t *testing.T) {
	t.Run("non multipart keeps missing file", func(t *testing.T) {
		router, _ := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcenonmultipart")
		request := httptest.NewRequest(http.MethodPost, "/api/sources/import", strings.NewReader("[]"))
		request.Header.Set("Authorization", account.Auth)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertBookSourceImportError(t, response, http.StatusBadRequest, "file is required")
	})

	t.Run("broken multipart is invalid", func(t *testing.T) {
		router, _ := setupTestServer(t)
		account := registerSourceContractAccount(t, router, "sourcebrokenmultipart")
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/sources/import",
			strings.NewReader("--broken\r\ninvalid\r\n"),
		)
		request.Header.Set("Authorization", account.Auth)
		request.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertBookSourceImportError(t, response, http.StatusBadRequest, "invalid source import request")
	})
}

func sourceImportFixture(identity string) []byte {
	return []byte(fmt.Sprintf(
		`[{"bookSourceName":"%s","bookSourceUrl":"https://%s.example"}]`,
		identity,
		identity,
	))
}

func performBookSourceMultipartImport(
	t *testing.T,
	router *gin.Engine,
	auth string,
	files []bookSourceMultipartFile,
	values map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile(file.field, file.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	for field, value := range values {
		if err := writer.WriteField(field, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sources/import", &body)
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertBookSourceImportError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	status int,
	message string,
) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("source import error = %d %s, want %d %q", response.Code, response.Body.String(), status, message)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != message {
		t.Fatalf("source import error = %q, want %q", payload.Error, message)
	}
}

type bookSourceImportReadTracker struct {
	reads int
}

func (reader *bookSourceImportReadTracker) Read([]byte) (int, error) {
	reader.reads++
	return 0, io.EOF
}
