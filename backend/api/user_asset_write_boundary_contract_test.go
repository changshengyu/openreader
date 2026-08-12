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
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	contractUploadEnvelopeBytes int64 = 33 << 20
	contractUploadDeleteBytes   int64 = 16 << 10
)

type assetBoundaryFilePart struct {
	field    string
	filename string
	data     []byte
}

type assetBoundaryByteReader byte

func (value assetBoundaryByteReader) Read(target []byte) (int, error) {
	for index := range target {
		target[index] = byte(value)
	}
	return len(target), nil
}

func TestUserAssetUploadEnforcesDeclaredAndActualMultipartEnvelope(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		router, server := setupTestServer(t)
		token, user := registerBookInfoAssetUser(t, router, "assetboundarydeclared")
		body, contentType := makeAssetBoundaryMultipart(t, [][2]string{{"type", "cover"}}, []assetBoundaryFilePart{{
			field: "file", filename: "cover.png", data: readerAppearancePNG(t, 1, 1),
		}})
		request := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
		request.ContentLength = contractUploadEnvelopeBytes + 1
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		assertAssetBoundaryError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
		assertUserAssetRootHasNoFiles(t, server, user.ID)
	})

	t.Run("chunked", func(t *testing.T) {
		router, server := setupTestServer(t)
		token, user := registerBookInfoAssetUser(t, router, "assetboundarychunked")
		body, contentType, _ := makeStreamingAssetMultipart("asset-boundary-chunked", "font", "reader.ttf", contractUploadEnvelopeBytes)
		request := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		cleanupParsedMultipart(t, request)

		assertAssetBoundaryError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
		assertUserAssetRootHasNoFiles(t, server, user.ID)
	})

	t.Run("auth before declared body", func(t *testing.T) {
		router, server := setupTestServer(t)
		_, user := registerBookInfoAssetUser(t, router, "assetboundaryunauthorized")
		request := httptest.NewRequest(http.MethodPost, "/api/uploads", strings.NewReader("not multipart"))
		request.ContentLength = contractUploadEnvelopeBytes + 1
		request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized oversized upload: expected 401, got %d: %s", response.Code, response.Body.String())
		}
		assertUserAssetRootHasNoFiles(t, server, user.ID)
	})
}

func TestUserAssetUploadAcceptsExactFileCapsInsideEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		filename string
		size     int64
	}{
		{name: "image", kind: "cover", filename: "cover.png", size: 8 << 20},
		{name: "font", kind: "font", filename: "reader.ttf", size: 32 << 20},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			token, user := registerBookInfoAssetUser(t, router, "assetexact"+test.name)
			body, contentType, contentLength := makeStreamingAssetMultipart(
				"asset-exact-"+test.name,
				test.kind,
				test.filename,
				test.size,
			)
			if contentLength > contractUploadEnvelopeBytes {
				t.Fatalf("test multipart length %d exceeds contract envelope", contentLength)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
			request.ContentLength = contentLength
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			cleanupParsedMultipart(t, request)

			if response.Code != http.StatusCreated {
				t.Fatalf("exact %s cap: expected 201, got %d: %s", test.name, response.Code, response.Body.String())
			}
			var result struct {
				URL  string `json:"url"`
				Size int64  `json:"size"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Size != test.size {
				t.Fatalf("exact %s cap returned size %d, want %d", test.name, result.Size, test.size)
			}
			path := filepath.Join(server.cfg.DataDir, "uploads", strings.TrimPrefix(result.URL, "/uploads/"))
			info, err := os.Stat(path)
			if err != nil || info.Size() != test.size {
				t.Fatalf("exact %s cap final file: info=%v err=%v", test.name, info, err)
			}
			prefix := "/uploads/users/" + strconv.FormatUint(uint64(user.ID), 10) + "/"
			if !strings.HasPrefix(result.URL, prefix) {
				t.Fatalf("exact %s cap returned foreign URL %q", test.name, result.URL)
			}
		})
	}
}

func TestUserAssetUploadKeepsFileLevelLimitsInsideEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		filename string
		size     int64
	}{
		{name: "image", kind: "cover", filename: "cover.png", size: (8 << 20) + 1},
		{name: "font", kind: "font", filename: "reader.ttf", size: (32 << 20) + 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			token, user := registerBookInfoAssetUser(t, router, "assetover"+test.name)
			body, contentType, contentLength := makeStreamingAssetMultipart(
				"asset-over-"+test.name,
				test.kind,
				test.filename,
				test.size,
			)
			if contentLength > contractUploadEnvelopeBytes {
				t.Fatalf("test multipart length %d exceeds contract envelope", contentLength)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
			request.ContentLength = contentLength
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			cleanupParsedMultipart(t, request)

			assertAssetBoundaryError(t, response, http.StatusBadRequest, "file is too large")
			assertUserAssetRootHasNoFiles(t, server, user.ID)
		})
	}
}

func TestUserAssetUploadRejectsAmbiguousMultipartShape(t *testing.T) {
	validPNG := readerAppearancePNG(t, 1, 1)
	tests := []struct {
		name           string
		fields         [][2]string
		files          []assetBoundaryFilePart
		expectedStatus int
		expectedError  string
		expectedType   string
	}{
		{
			name:           "omitted type remains misc",
			files:          []assetBoundaryFilePart{{field: "file", filename: "asset.png", data: validPNG}},
			expectedStatus: http.StatusCreated,
			expectedType:   "misc",
		},
		{
			name:   "duplicate file",
			fields: [][2]string{{"type", "cover"}},
			files: []assetBoundaryFilePart{
				{field: "file", filename: "first.png", data: validPNG},
				{field: "file", filename: "second.png", data: validPNG},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid upload request",
		},
		{
			name:   "extra file field",
			fields: [][2]string{{"type", "cover"}},
			files: []assetBoundaryFilePart{
				{field: "file", filename: "cover.png", data: validPNG},
				{field: "ignored", filename: "ignored.png", data: validPNG},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid upload request",
		},
		{
			name:           "duplicate type",
			fields:         [][2]string{{"type", "cover"}, {"type", "background"}},
			files:          []assetBoundaryFilePart{{field: "file", filename: "cover.png", data: validPNG}},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid upload request",
		},
		{
			name:           "oversized type",
			fields:         [][2]string{{"type", strings.Repeat("x", 33)}},
			files:          []assetBoundaryFilePart{{field: "file", filename: "asset.png", data: validPNG}},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid upload request",
		},
		{
			name:           "oversized filename",
			fields:         [][2]string{{"type", "cover"}},
			files:          []assetBoundaryFilePart{{field: "file", filename: strings.Repeat("a", 252) + ".png", data: validPNG}},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid upload request",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			token, user := registerBookInfoAssetUser(t, router, "assetshape"+strings.ReplaceAll(test.name, " ", ""))
			body, contentType := makeAssetBoundaryMultipart(t, test.fields, test.files)
			request := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			cleanupParsedMultipart(t, request)

			if test.expectedError != "" {
				assertAssetBoundaryError(t, response, test.expectedStatus, test.expectedError)
				assertUserAssetRootHasNoFiles(t, server, user.ID)
				return
			}
			if response.Code != test.expectedStatus {
				t.Fatalf("expected %d, got %d: %s", test.expectedStatus, response.Code, response.Body.String())
			}
			var result struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Type != test.expectedType {
				t.Fatalf("upload type = %q, want %q", result.Type, test.expectedType)
			}
		})
	}
}

func TestUserAssetUploadRemovesMultipartTemporaryFiles(t *testing.T) {
	tests := []struct {
		name           string
		fields         [][2]string
		files          []assetBoundaryFilePart
		expectedStatus int
	}{
		{
			name:           "success",
			fields:         [][2]string{{"type", "cover"}},
			files:          []assetBoundaryFilePart{{field: "file", filename: "cover.png", data: readerAppearancePNG(t, 1, 1)}},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "content rejection",
			fields:         [][2]string{{"type", "cover"}},
			files:          []assetBoundaryFilePart{{field: "file", filename: "cover.png", data: []byte("not an image")}},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, _ := setupTestServer(t)
			router.MaxMultipartMemory = 1
			token, _ := registerBookInfoAssetUser(t, router, "assettemp"+strings.ReplaceAll(test.name, " ", ""))
			body, contentType := makeAssetBoundaryMultipart(t, test.fields, test.files)
			request := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.expectedStatus {
				cleanupParsedMultipart(t, request)
				t.Fatalf("expected %d, got %d: %s", test.expectedStatus, response.Code, response.Body.String())
			}
			assertParsedMultipartTemporaryFilesRemoved(t, request)
		})
	}
}

func TestUserAssetDeleteRequiresBoundedSingleJSON(t *testing.T) {
	tests := []struct {
		name           string
		makeBody       func(string) io.Reader
		contentLength  int64
		authorized     bool
		expectedStatus int
		expectedError  string
		expectDeleted  bool
	}{
		{
			name:           "declared overflow",
			makeBody:       func(url string) io.Reader { return strings.NewReader(assetDeletePayload(url)) },
			contentLength:  contractUploadDeleteBytes + 1,
			authorized:     true,
			expectedStatus: http.StatusRequestEntityTooLarge,
			expectedError:  "request body too large",
		},
		{
			name: "chunked overflow",
			makeBody: func(url string) io.Reader {
				payload := assetDeletePayload(url)
				return io.MultiReader(
					strings.NewReader(payload),
					io.LimitReader(assetBoundaryByteReader(' '), contractUploadDeleteBytes+1-int64(len(payload))),
				)
			},
			contentLength:  -1,
			authorized:     true,
			expectedStatus: http.StatusRequestEntityTooLarge,
			expectedError:  "request body too large",
		},
		{
			name:           "second json",
			makeBody:       func(url string) io.Reader { return strings.NewReader(assetDeletePayload(url) + ` {"url":"ignored"}`) },
			contentLength:  -2,
			authorized:     true,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "url is required",
		},
		{
			name:           "trailing garbage",
			makeBody:       func(url string) io.Reader { return strings.NewReader(assetDeletePayload(url) + " trailing") },
			contentLength:  -2,
			authorized:     true,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "url is required",
		},
		{
			name: "exact limit",
			makeBody: func(url string) io.Reader {
				payload := assetDeletePayload(url)
				return io.MultiReader(
					strings.NewReader(payload),
					io.LimitReader(assetBoundaryByteReader(' '), contractUploadDeleteBytes-int64(len(payload))),
				)
			},
			contentLength:  contractUploadDeleteBytes,
			authorized:     true,
			expectedStatus: http.StatusOK,
			expectDeleted:  true,
		},
		{
			name:           "auth before declared overflow",
			makeBody:       func(url string) io.Reader { return strings.NewReader(assetDeletePayload(url)) },
			contentLength:  contractUploadDeleteBytes + 1,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			token, user := registerBookInfoAssetUser(t, router, "assetdelete"+strings.ReplaceAll(test.name, " ", ""))
			url, path := createAssetBoundaryFile(t, server, user.ID, test.name+".ttf")
			request := httptest.NewRequest(http.MethodDelete, "/api/uploads", test.makeBody(url))
			if test.contentLength != -2 {
				request.ContentLength = test.contentLength
			}
			if test.contentLength == -1 {
				request.TransferEncoding = []string{"chunked"}
			}
			request.Header.Set("Content-Type", "application/json")
			if test.authorized {
				request.Header.Set("Authorization", token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if test.expectedError != "" {
				assertAssetBoundaryError(t, response, test.expectedStatus, test.expectedError)
			} else if response.Code != test.expectedStatus {
				t.Fatalf("expected %d, got %d: %s", test.expectedStatus, response.Code, response.Body.String())
			}
			_, err := os.Stat(path)
			if test.expectDeleted {
				if !os.IsNotExist(err) {
					t.Fatalf("expected asset deletion, stat err=%v", err)
				}
			} else if err != nil {
				t.Fatalf("rejected delete changed asset: %v", err)
			}
		})
	}
}

func makeAssetBoundaryMultipart(
	t *testing.T,
	fields [][2]string,
	files []assetBoundaryFilePart,
) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile(file.field, file.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func makeStreamingAssetMultipart(boundary, kind, filename string, size int64) (io.Reader, string, int64) {
	prefix := fmt.Sprintf(
		"--%s\r\nContent-Disposition: form-data; name=\"type\"\r\n\r\n%s\r\n"+
			"--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n"+
			"Content-Type: application/octet-stream\r\n\r\n",
		boundary,
		kind,
		boundary,
		filename,
	)
	suffix := fmt.Sprintf("\r\n--%s--\r\n", boundary)
	header := readerAppearanceFont("ttf")
	if strings.HasSuffix(strings.ToLower(filename), ".png") {
		header = readerAppearancePNGHeader(1, 1)
	}
	if size < int64(len(header)) {
		header = header[:size]
	}
	content := io.MultiReader(
		bytes.NewReader(header),
		io.LimitReader(assetBoundaryByteReader(0), size-int64(len(header))),
	)
	return io.MultiReader(strings.NewReader(prefix), content, strings.NewReader(suffix)),
		"multipart/form-data; boundary=" + boundary,
		int64(len(prefix)) + size + int64(len(suffix))
}

func assertAssetBoundaryError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, response.Code, response.Body.String())
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode error response: %v: %s", err, response.Body.String())
	}
	if result.Error != message {
		t.Fatalf("error = %q, want %q", result.Error, message)
	}
}

func assertUserAssetRootHasNoFiles(t *testing.T, server *Server, userID uint) {
	t.Helper()
	root := filepath.Join(server.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(userID), 10))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			t.Errorf("rejected upload left final file %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func cleanupParsedMultipart(t *testing.T, request *http.Request) {
	t.Helper()
	if request.MultipartForm != nil {
		if err := request.MultipartForm.RemoveAll(); err != nil {
			t.Fatalf("clean parsed multipart: %v", err)
		}
	}
}

func assertParsedMultipartTemporaryFilesRemoved(t *testing.T, request *http.Request) {
	t.Helper()
	if request.MultipartForm == nil {
		t.Fatal("handler did not retain parsed multipart metadata")
	}
	form := request.MultipartForm
	t.Cleanup(func() {
		_ = form.RemoveAll()
	})
	fileCount := 0
	for _, headers := range form.File {
		for _, header := range headers {
			fileCount++
			file, err := header.Open()
			if err == nil {
				_ = file.Close()
				t.Errorf("multipart temporary file for %q is still openable", header.Filename)
			}
		}
	}
	if fileCount == 0 {
		t.Fatal("parsed multipart had no file metadata")
	}
}

func assetDeletePayload(url string) string {
	return `{"url":` + strconv.Quote(url) + `}`
}

func createAssetBoundaryFile(t *testing.T, server *Server, userID uint, filename string) (string, string) {
	t.Helper()
	filename = strings.ReplaceAll(filename, " ", "-")
	directory := filepath.Join(
		server.cfg.DataDir,
		"uploads",
		"users",
		strconv.FormatUint(uint64(userID), 10),
		"fonts",
	)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, filename)
	if err := os.WriteFile(path, readerAppearanceFont("ttf"), 0o644); err != nil {
		t.Fatal(err)
	}
	return "/uploads/users/" + strconv.FormatUint(uint64(userID), 10) + "/fonts/" + filename, path
}
