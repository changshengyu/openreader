package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
)

const (
	testLocalStoreMetadataBodyLimit = 16 << 10
	testLocalStoreImportBodyLimit   = 1 << 20
)

type localStoreMultipartValue struct {
	name  string
	value string
}

type localStoreMultipartFile struct {
	field    string
	filename string
	data     []byte
}

func localStoreMultipartPayload(
	t *testing.T,
	values []localStoreMultipartValue,
	files []localStoreMultipartFile,
) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, value := range values {
		if err := writer.WriteField(value.name, value.value); err != nil {
			t.Fatalf("write multipart value %s: %v", value.name, err)
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile(file.field, file.filename)
		if err != nil {
			t.Fatalf("create multipart file %s: %v", file.filename, err)
		}
		if _, err := part.Write(file.data); err != nil {
			t.Fatalf("write multipart file %s: %v", file.filename, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart payload: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func performLocalStoreRequest(
	handler http.Handler,
	method string,
	path string,
	auth string,
	contentType string,
	body []byte,
	chunked bool,
) (*httptest.ResponseRecorder, *http.Request) {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response, request
}

func exactPaddedJSONObject(t *testing.T, limit int, prefix string, suffix string) []byte {
	t.Helper()
	if len(prefix)+len(suffix) > limit {
		t.Fatalf("JSON fixture overhead %d exceeds limit %d", len(prefix)+len(suffix), limit)
	}
	return []byte(prefix + strings.Repeat("x", limit-len(prefix)-len(suffix)) + suffix)
}

func TestLocalStoreUploadRequestEnvelopeUsesDeclaredAndActualReadLimits(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		name := "declared"
		if chunked {
			name = "chunked"
		}
		t.Run(name, func(t *testing.T) {
			router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
				cfg.MaxImportBytes = 8
			})
			auth := authHeader(t, router)
			requestLimit := server.maxLocalImportBytes() + (1 << 20)
			body, contentType := localStoreMultipartPayload(t,
				[]localStoreMultipartValue{
					{name: "path", value: "overflow-target"},
					{name: "padding", value: strings.Repeat("x", int(requestLimit))},
				},
				[]localStoreMultipartFile{{field: "file", filename: "tiny.txt", data: []byte("ok")}},
			)

			response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/upload", auth, contentType, body, chunked)
			if response.Code != http.StatusRequestEntityTooLarge ||
				!strings.Contains(response.Body.String(), "local store upload request is too large") {
				t.Fatalf("%s overflow: expected stable 413, got %d: %s", name, response.Code, response.Body.String())
			}
			if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, "overflow-target")); !os.IsNotExist(err) {
				t.Fatalf("%s overflow must not create target directory, stat err=%v", name, err)
			}
		})
	}

	t.Run("authentication precedes oversized body", func(t *testing.T) {
		router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
			cfg.MaxImportBytes = 8
		})
		requestLimit := server.maxLocalImportBytes() + (1 << 20)
		body, contentType := localStoreMultipartPayload(t,
			[]localStoreMultipartValue{{name: "padding", value: strings.Repeat("x", int(requestLimit))}},
			[]localStoreMultipartFile{{field: "file", filename: "tiny.txt", data: []byte("ok")}},
		)
		response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/upload", "", contentType, body, true)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated overflow: expected 401, got %d: %s", response.Code, response.Body.String())
		}
	})
}

func TestLocalStoreUploadRejectsAmbiguousOrOversizedMultipartShapeBeforeWrites(t *testing.T) {
	tests := []struct {
		name   string
		values []localStoreMultipartValue
		files  []localStoreMultipartFile
	}{
		{
			name:   "sixty five files",
			values: []localStoreMultipartValue{{name: "path", value: "many"}},
			files: func() []localStoreMultipartFile {
				files := make([]localStoreMultipartFile, 0, 65)
				for index := 0; index < 65; index++ {
					files = append(files, localStoreMultipartFile{
						field:    "file",
						filename: fmt.Sprintf("file-%02d.txt", index),
						data:     []byte("x"),
					})
				}
				return files
			}(),
		},
		{
			name:   "extra file field",
			values: []localStoreMultipartValue{{name: "path", value: "extra-file"}},
			files: []localStoreMultipartFile{
				{field: "file", filename: "accepted.txt", data: []byte("accepted")},
				{field: "attachment", filename: "ignored.txt", data: []byte("ignored")},
			},
		},
		{
			name: "duplicate path",
			values: []localStoreMultipartValue{
				{name: "path", value: "first"},
				{name: "path", value: "second"},
			},
			files: []localStoreMultipartFile{{field: "file", filename: "book.txt", data: []byte("body")}},
		},
		{
			name: "extra value field",
			values: []localStoreMultipartValue{
				{name: "path", value: "extra-value"},
				{name: "unexpected", value: "ignored"},
			},
			files: []localStoreMultipartFile{{field: "file", filename: "book.txt", data: []byte("body")}},
		},
		{
			name:   "oversized path",
			values: []localStoreMultipartValue{{name: "path", value: strings.Repeat("p", 4097)}},
			files:  []localStoreMultipartFile{{field: "file", filename: "book.txt", data: []byte("body")}},
		},
		{
			name:   "oversized filename",
			values: []localStoreMultipartValue{{name: "path", value: "long-name"}},
			files:  []localStoreMultipartFile{{field: "file", filename: strings.Repeat("n", 252) + ".txt", data: []byte("body")}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			body, contentType := localStoreMultipartPayload(t, test.values, test.files)
			response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/upload", auth, contentType, body, false)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
			entries, err := os.ReadDir(server.cfg.LocalStoreDir)
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("read local-store root: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("invalid multipart shape must not write files, got %d root entries", len(entries))
			}
		})
	}
}

func TestLocalStoreUploadKeepsFileLimitAndSixtyFourFileOrder(t *testing.T) {
	t.Run("exact per-file limit", func(t *testing.T) {
		router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
			cfg.MaxImportBytes = 8
		})
		auth := authHeader(t, router)
		for size, wantStatus := range map[int]int{8: http.StatusCreated, 9: http.StatusRequestEntityTooLarge} {
			body, contentType := localStoreMultipartPayload(t,
				[]localStoreMultipartValue{{name: "path", value: fmt.Sprintf("size-%d", size)}},
				[]localStoreMultipartFile{{field: "file", filename: "book.bin", data: bytes.Repeat([]byte{'x'}, size)}},
			)
			response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/upload", auth, contentType, body, false)
			if response.Code != wantStatus {
				t.Fatalf("file size %d: expected %d, got %d: %s", size, wantStatus, response.Code, response.Body.String())
			}
			if size == 8 {
				data, err := os.ReadFile(filepath.Join(server.cfg.LocalStoreDir, "size-8", "book.bin"))
				if err != nil || len(data) != 8 {
					t.Fatalf("exact-limit file: len=%d err=%v", len(data), err)
				}
			}
		}
	})

	t.Run("sixty four files retain multipart order", func(t *testing.T) {
		router, _ := setupTestServer(t)
		auth := authHeader(t, router)
		files := make([]localStoreMultipartFile, 0, 64)
		for index := 0; index < 64; index++ {
			files = append(files, localStoreMultipartFile{
				field:    "file",
				filename: fmt.Sprintf("ordered-%02d.txt", index),
				data:     []byte(fmt.Sprintf("body-%02d", index)),
			})
		}
		body, contentType := localStoreMultipartPayload(t,
			[]localStoreMultipartValue{{name: "path", value: "ordered"}},
			files,
		)
		response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/upload", auth, contentType, body, false)
		if response.Code != http.StatusCreated {
			t.Fatalf("64-file upload: expected 201, got %d: %s", response.Code, response.Body.String())
		}
		var payload struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode 64-file response: %v", err)
		}
		if len(payload.Paths) != 64 {
			t.Fatalf("expected 64 response paths, got %d", len(payload.Paths))
		}
		for index, path := range payload.Paths {
			want := filepath.Join("ordered", fmt.Sprintf("ordered-%02d.txt", index))
			if path != want {
				t.Fatalf("path %d: expected %q, got %q", index, want, path)
			}
		}
	})
}

func TestLocalStoreUploadOwnsMultipartTemporaryFileCleanup(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		files      []localStoreMultipartFile
		maxBytes   int64
		wantStatus int
	}{
		{
			name:       "success",
			path:       "success",
			files:      []localStoreMultipartFile{{field: "file", filename: "book.bin", data: bytes.Repeat([]byte{'a'}, 64)}},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "file limit",
			path:       "file-limit",
			files:      []localStoreMultipartFile{{field: "file", filename: "book.bin", data: bytes.Repeat([]byte{'b'}, 9)}},
			maxBytes:   8,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "path rejection",
			path:       "../outside",
			files:      []localStoreMultipartFile{{field: "file", filename: "book.bin", data: bytes.Repeat([]byte{'c'}, 64)}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "shape rejection",
			path: "shape",
			files: []localStoreMultipartFile{
				{field: "file", filename: "book.bin", data: bytes.Repeat([]byte{'d'}, 64)},
				{field: "attachment", filename: "extra.bin", data: bytes.Repeat([]byte{'e'}, 64)},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, _ := setupTestServerWithConfig(t, func(cfg *config.Config) {
				if test.maxBytes > 0 {
					cfg.MaxImportBytes = test.maxBytes
				}
			})
			router.MaxMultipartMemory = 1
			auth := authHeader(t, router)
			body, contentType := localStoreMultipartPayload(t,
				[]localStoreMultipartValue{{name: "path", value: test.path}},
				test.files,
			)
			response, request := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/upload", auth, contentType, body, false)
			if request.MultipartForm != nil {
				t.Cleanup(func() { _ = request.MultipartForm.RemoveAll() })
			}
			if response.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
			if request.MultipartForm == nil || len(request.MultipartForm.File["file"]) == 0 {
				t.Fatal("expected parsed multipart file evidence")
			}
			opened, err := request.MultipartForm.File["file"][0].Open()
			if err == nil {
				_ = opened.Close()
				t.Fatal("handler must remove multipart temporary storage on every parsed-form exit")
			}
		})
	}
}

func TestLocalStoreMetadataJSONUsesBoundedSingleDocumentBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   func(target string) []byte
		check  func(t *testing.T, server *Server, target string)
	}{
		{
			name:   "directory declared overflow",
			method: http.MethodPost,
			path:   "/api/local-store/directory",
			body: func(target string) []byte {
				return []byte(fmt.Sprintf(`{"path":"","name":%q,"padding":%q}`, target, strings.Repeat("x", testLocalStoreMetadataBodyLimit)))
			},
			check: func(t *testing.T, server *Server, target string) {
				if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, target)); !os.IsNotExist(err) {
					t.Fatalf("overflow must not create directory, stat err=%v", err)
				}
			},
		},
		{
			name:   "directory second JSON",
			method: http.MethodPost,
			path:   "/api/local-store/directory",
			body: func(target string) []byte {
				return []byte(fmt.Sprintf(`{"path":"","name":%q}{"name":"second"}`, target))
			},
			check: func(t *testing.T, server *Server, target string) {
				if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, target)); !os.IsNotExist(err) {
					t.Fatalf("second JSON must not create directory, stat err=%v", err)
				}
			},
		},
		{
			name:   "directory trailing garbage",
			method: http.MethodPost,
			path:   "/api/local-store/directory",
			body: func(target string) []byte {
				return []byte(fmt.Sprintf(`{"path":"","name":%q} trailing`, target))
			},
			check: func(t *testing.T, server *Server, target string) {
				if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, target)); !os.IsNotExist(err) {
					t.Fatalf("trailing garbage must not create directory, stat err=%v", err)
				}
			},
		},
		{
			name:   "rename chunked overflow",
			method: http.MethodPut,
			path:   "/api/local-store/rename",
			body: func(target string) []byte {
				return []byte(fmt.Sprintf(`{"path":"old.txt","name":%q,"padding":%q}`, target, strings.Repeat("x", testLocalStoreMetadataBodyLimit)))
			},
			check: func(t *testing.T, server *Server, target string) {
				if data, err := os.ReadFile(filepath.Join(server.cfg.LocalStoreDir, "old.txt")); err != nil || string(data) != "old" {
					t.Fatalf("overflow must preserve source: data=%q err=%v", data, err)
				}
				if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, target)); !os.IsNotExist(err) {
					t.Fatalf("overflow must not create rename target, stat err=%v", err)
				}
			},
		},
		{
			name:   "rename second JSON",
			method: http.MethodPut,
			path:   "/api/local-store/rename",
			body: func(target string) []byte {
				return []byte(fmt.Sprintf(`{"path":"old.txt","name":%q}{"name":"second"}`, target))
			},
			check: func(t *testing.T, server *Server, target string) {
				if data, err := os.ReadFile(filepath.Join(server.cfg.LocalStoreDir, "old.txt")); err != nil || string(data) != "old" {
					t.Fatalf("second JSON must preserve source: data=%q err=%v", data, err)
				}
				if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, target)); !os.IsNotExist(err) {
					t.Fatalf("second JSON must not create rename target, stat err=%v", err)
				}
			},
		},
		{
			name:   "rename trailing garbage",
			method: http.MethodPut,
			path:   "/api/local-store/rename",
			body: func(target string) []byte {
				return []byte(fmt.Sprintf(`{"path":"old.txt","name":%q} trailing`, target))
			},
			check: func(t *testing.T, server *Server, target string) {
				if data, err := os.ReadFile(filepath.Join(server.cfg.LocalStoreDir, "old.txt")); err != nil || string(data) != "old" {
					t.Fatalf("trailing garbage must preserve source: data=%q err=%v", data, err)
				}
				if _, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, target)); !os.IsNotExist(err) {
					t.Fatalf("trailing garbage must not create rename target, stat err=%v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
				t.Fatalf("create local-store root: %v", err)
			}
			if strings.Contains(test.name, "rename") {
				if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "old.txt"), []byte("old"), 0o644); err != nil {
					t.Fatalf("write rename source: %v", err)
				}
			}
			target := "target"
			chunked := strings.Contains(test.name, "chunked")
			response, _ := performLocalStoreRequest(router, test.method, test.path, auth, "application/json", test.body(target), chunked)
			if strings.Contains(test.name, "overflow") {
				if response.Code != http.StatusRequestEntityTooLarge {
					t.Fatalf("expected 413, got %d: %s", response.Code, response.Body.String())
				}
			} else if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
			test.check(t, server, target)
		})
	}
}

func TestLocalStoreMetadataJSONAcceptsExactBodyLimit(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	body := exactPaddedJSONObject(
		t,
		testLocalStoreMetadataBodyLimit,
		`{"path":"","name":"exact-limit","padding":"`,
		`"}`,
	)
	response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/local-store/directory", auth, "application/json", body, false)
	if response.Code != http.StatusCreated {
		t.Fatalf("exact 16 KiB metadata body: expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if info, err := os.Stat(filepath.Join(server.cfg.LocalStoreDir, "exact-limit")); err != nil || !info.IsDir() {
		t.Fatalf("exact-limit directory missing: info=%v err=%v", info, err)
	}
}

func TestLocalStoreImportJSONUsesBoundedSingleInputAndCardinality(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       func() []byte
		chunked    bool
		wantStatus int
	}{
		{
			name: "preview declared overflow",
			path: "/api/local-store/import-preview",
			body: func() []byte {
				return []byte(`{"paths":["book.txt"],"padding":"` + strings.Repeat("x", testLocalStoreImportBodyLimit) + `"}`)
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "import chunked overflow",
			path: "/api/local-store/import",
			body: func() []byte {
				return []byte(`{"paths":["book.txt"],"padding":"` + strings.Repeat("x", testLocalStoreImportBodyLimit) + `"}`)
			},
			chunked:    true,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "preview second JSON",
			path: "/api/local-store/import-preview",
			body: func() []byte {
				return []byte(`{"paths":["book.txt"]}{"paths":["second.txt"]}`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "import trailing garbage",
			path: "/api/local-store/import",
			body: func() []byte {
				return []byte(`{"paths":["book.txt"]} trailing`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "preview dual inputs",
			path: "/api/local-store/import-preview",
			body: func() []byte {
				return []byte(`{"paths":["book.txt"],"items":[{"path":"book.txt"}]}`)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "preview 201 paths",
			path: "/api/local-store/import-preview",
			body: func() []byte {
				paths := make([]string, 0, 201)
				for index := 0; index < 201; index++ {
					paths = append(paths, fmt.Sprintf("missing-%03d.txt", index))
				}
				body, err := json.Marshal(map[string]any{"paths": paths})
				if err != nil {
					t.Fatalf("marshal paths: %v", err)
				}
				return body
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "import 201 items",
			path: "/api/local-store/import",
			body: func() []byte {
				items := make([]map[string]string, 0, 201)
				for index := 0; index < 201; index++ {
					items = append(items, map[string]string{"path": fmt.Sprintf("missing-%03d.txt", index)})
				}
				body, err := json.Marshal(map[string]any{"items": items})
				if err != nil {
					t.Fatalf("marshal items: %v", err)
				}
				return body
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "preview exact one MiB",
			path: "/api/local-store/import-preview",
			body: func() []byte {
				return exactPaddedJSONObject(t, testLocalStoreImportBodyLimit, `{"paths":["missing.txt"],"padding":"`, `"}`)
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
				t.Fatalf("create local-store root: %v", err)
			}
			if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "book.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
				t.Fatalf("write import fixture: %v", err)
			}
			response, _ := performLocalStoreRequest(router, http.MethodPost, test.path, auth, "application/json", test.body(), test.chunked)
			if response.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
			if test.wantStatus != http.StatusOK {
				var count int64
				if err := server.db.Table("books").Count(&count).Error; err != nil {
					t.Fatalf("count books: %v", err)
				}
				if count != 0 {
					t.Fatalf("rejected import request must not create books, got %d", count)
				}
			}
		})
	}
}

func TestLocalStoreDirectoryImportExpansionIsBoundedBeforeSideEffects(t *testing.T) {
	for _, endpoint := range []string{"/api/local-store/import-preview", "/api/local-store/import"} {
		t.Run(endpoint, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			bulkDir := filepath.Join(server.cfg.LocalStoreDir, "bulk")
			if err := os.MkdirAll(bulkDir, 0o755); err != nil {
				t.Fatalf("create bulk directory: %v", err)
			}
			for index := 0; index < 201; index++ {
				name := filepath.Join(bulkDir, fmt.Sprintf("book-%03d.txt", index))
				if err := os.WriteFile(name, []byte("第一章 开始\n正文"), 0o644); err != nil {
					t.Fatalf("write bulk fixture %d: %v", index, err)
				}
			}

			response, _ := performLocalStoreRequest(
				router,
				http.MethodPost,
				endpoint,
				auth,
				"application/json",
				[]byte(`{"paths":["bulk"]}`),
				false,
			)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("201-file directory expansion: expected 400, got %d with %d response bytes", response.Code, response.Body.Len())
			}
			var count int64
			if err := server.db.Table("books").Count(&count).Error; err != nil {
				t.Fatalf("count books: %v", err)
			}
			if count != 0 {
				t.Fatalf("bounded expansion must reject before book writes, got %d", count)
			}
			stageRoot := filepath.Join(server.cfg.CacheDir, "import-previews")
			entries, err := os.ReadDir(stageRoot)
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("read stage root: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("bounded expansion must reject before stage writes, got %d entries", len(entries))
			}
		})
	}
}

func TestLocalStoreRejectsSymlinkTraversalAcrossEveryFilesystemAction(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		body      func(t *testing.T) ([]byte, string)
		prepare   func(t *testing.T, outside string)
		unchanged func(t *testing.T, outside string)
	}{
		{
			name:   "list descendant",
			method: http.MethodGet,
			path:   "/api/local-store?path=" + url.QueryEscape("escape"),
		},
		{
			name:   "download descendant",
			method: http.MethodGet,
			path:   "/api/local-store/download?path=" + url.QueryEscape("escape/secret.txt"),
		},
		{
			name:   "create descendant",
			method: http.MethodPost,
			path:   "/api/local-store/directory",
			body: func(t *testing.T) ([]byte, string) {
				return []byte(`{"path":"escape","name":"created"}`), "application/json"
			},
			unchanged: func(t *testing.T, outside string) {
				if _, err := os.Stat(filepath.Join(outside, "created")); !os.IsNotExist(err) {
					t.Fatalf("create must not escape LocalStore root, stat err=%v", err)
				}
			},
		},
		{
			name:   "rename descendant",
			method: http.MethodPut,
			path:   "/api/local-store/rename",
			body: func(t *testing.T) ([]byte, string) {
				return []byte(`{"path":"escape/rename-source.txt","name":"renamed.txt"}`), "application/json"
			},
			prepare: func(t *testing.T, outside string) {
				if err := os.WriteFile(filepath.Join(outside, "rename-source.txt"), []byte("rename"), 0o644); err != nil {
					t.Fatalf("write rename fixture: %v", err)
				}
			},
			unchanged: func(t *testing.T, outside string) {
				if data, err := os.ReadFile(filepath.Join(outside, "rename-source.txt")); err != nil || string(data) != "rename" {
					t.Fatalf("rename source changed: data=%q err=%v", data, err)
				}
				if _, err := os.Stat(filepath.Join(outside, "renamed.txt")); !os.IsNotExist(err) {
					t.Fatalf("rename target escaped root, stat err=%v", err)
				}
			},
		},
		{
			name:   "upload descendant",
			method: http.MethodPost,
			path:   "/api/local-store/upload",
			body: func(t *testing.T) ([]byte, string) {
				return localStoreMultipartPayload(t,
					[]localStoreMultipartValue{{name: "path", value: "escape"}},
					[]localStoreMultipartFile{{field: "file", filename: "uploaded.txt", data: []byte("upload")}},
				)
			},
			unchanged: func(t *testing.T, outside string) {
				if _, err := os.Stat(filepath.Join(outside, "uploaded.txt")); !os.IsNotExist(err) {
					t.Fatalf("upload escaped root, stat err=%v", err)
				}
			},
		},
		{
			name:   "delete descendant",
			method: http.MethodDelete,
			path:   "/api/local-store?path=" + url.QueryEscape("escape/delete-target.txt"),
			prepare: func(t *testing.T, outside string) {
				if err := os.WriteFile(filepath.Join(outside, "delete-target.txt"), []byte("keep"), 0o644); err != nil {
					t.Fatalf("write delete fixture: %v", err)
				}
			},
			unchanged: func(t *testing.T, outside string) {
				if data, err := os.ReadFile(filepath.Join(outside, "delete-target.txt")); err != nil || string(data) != "keep" {
					t.Fatalf("delete escaped root: data=%q err=%v", data, err)
				}
			},
		},
		{
			name:   "preview descendant",
			method: http.MethodPost,
			path:   "/api/local-store/import-preview",
			body: func(t *testing.T) ([]byte, string) {
				return []byte(`{"paths":["escape/book.txt"]}`), "application/json"
			},
		},
		{
			name:   "import descendant",
			method: http.MethodPost,
			path:   "/api/local-store/import",
			body: func(t *testing.T) ([]byte, string) {
				return []byte(`{"paths":["escape/book.txt"]}`), "application/json"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
				t.Fatalf("write secret fixture: %v", err)
			}
			if err := os.WriteFile(filepath.Join(outside, "book.txt"), []byte("第一章 开始\n正文"), 0o644); err != nil {
				t.Fatalf("write book fixture: %v", err)
			}
			if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
				t.Fatalf("create local-store root: %v", err)
			}
			if err := os.Symlink(outside, filepath.Join(server.cfg.LocalStoreDir, "escape")); err != nil {
				t.Fatalf("create LocalStore symlink: %v", err)
			}
			if test.prepare != nil {
				test.prepare(t, outside)
			}
			var body []byte
			var contentType string
			if test.body != nil {
				body, contentType = test.body(t)
			}
			response, _ := performLocalStoreRequest(router, test.method, test.path, auth, contentType, body, false)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected path-safe 400, got %d: %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), outside) {
				t.Fatalf("response exposed host path: %s", response.Body.String())
			}
			if test.unchanged != nil {
				test.unchanged(t, outside)
			}
		})
	}
}

func TestLocalStoreListingOmitsSymlinkButKeepsSafeNeighbors(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	outside := t.TempDir()
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatalf("create local-store root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, "safe.txt"), []byte("safe"), 0o644); err != nil {
		t.Fatalf("write safe neighbor: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(server.cfg.LocalStoreDir, "escape")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	response, _ := performLocalStoreRequest(router, http.MethodGet, "/api/local-store", auth, "", nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("safe root listing: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"name":"safe.txt"`) {
		t.Fatalf("safe neighbor missing from listing: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"name":"escape"`) {
		t.Fatalf("symlink must be hidden from listing: %s", response.Body.String())
	}
}

func TestLocalStoreRejectsSymlinkedConfiguredRoot(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write root secret: %v", err)
	}
	rootParent := t.TempDir()
	linkedRoot := filepath.Join(rootParent, "local-store-link")
	if err := os.Symlink(outside, linkedRoot); err != nil {
		t.Fatalf("create root symlink: %v", err)
	}
	router, _ := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.LocalStoreDir = linkedRoot
	})
	auth := authHeader(t, router)
	response, _ := performLocalStoreRequest(router, http.MethodGet, "/api/local-store", auth, "", nil, false)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("symlinked configured root: expected 400, got %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), outside) {
		t.Fatalf("response exposed root target: %s", response.Body.String())
	}
}
