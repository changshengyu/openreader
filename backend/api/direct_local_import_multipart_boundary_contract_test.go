package api

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

const directLocalImportEnvelopeBytes int64 = 1 << 20

type directLocalImportMultipartPart struct {
	name     string
	filename string
	data     []byte
}

func TestDirectLocalImportMultipartEnvelopeAndAuthPriority(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxImportBytes = 64
	})
	auth := authHeader(t, router)
	requestLimit := server.maxLocalImportBytes() + directLocalImportEnvelopeBytes

	validBody, contentType := directLocalImportMultipartBody(t, []directLocalImportMultipartPart{
		{name: "file", filename: "declared.txt", data: []byte("第一章\n正文")},
	})

	t.Run("declared overflow", func(t *testing.T) {
		beforeStages := directLocalImportStageEntryCount(t, server, 1)
		request := httptest.NewRequest(http.MethodPost, "/api/imports/books/preview", bytes.NewReader(validBody))
		request.Header.Set("Authorization", auth)
		request.Header.Set("Content-Type", contentType)
		request.ContentLength = requestLimit + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		assertDirectLocalImportError(t, response, http.StatusRequestEntityTooLarge, "local import request is too large")
		if after := directLocalImportStageEntryCount(t, server, 1); after != beforeStages {
			t.Errorf("declared overflow changed stage entries: before=%d after=%d", beforeStages, after)
		}
	})

	t.Run("chunked overflow", func(t *testing.T) {
		body, overflowType := directLocalImportMultipartBody(t, []directLocalImportMultipartPart{
			{name: "file", filename: "chunked.txt", data: []byte("第一章\n正文")},
			{name: "padding", data: bytes.Repeat([]byte("x"), int(requestLimit))},
		})
		beforeStages := directLocalImportStageEntryCount(t, server, 1)
		request := httptest.NewRequest(http.MethodPost, "/api/imports/books/preview", bytes.NewReader(body))
		request.Header.Set("Authorization", auth)
		request.Header.Set("Content-Type", overflowType)
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		assertDirectLocalImportError(t, response, http.StatusRequestEntityTooLarge, "local import request is too large")
		if after := directLocalImportStageEntryCount(t, server, 1); after != beforeStages {
			t.Errorf("chunked overflow changed stage entries: before=%d after=%d", beforeStages, after)
		}
	})

	t.Run("authentication remains first", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/imports/books/preview", bytes.NewReader(validBody))
		request.Header.Set("Content-Type", contentType)
		request.ContentLength = requestLimit + 1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated declared overflow = %d, want 401: %s", response.Code, response.Body.String())
		}
	})
}

func TestDirectLocalImportExactMultipartAndFileLimits(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxImportBytes = 64
	})
	auth := authHeader(t, router)

	t.Run("exact file limit is admitted", func(t *testing.T) {
		data := append([]byte("第一章\n"), bytes.Repeat([]byte("x"), 64-len([]byte("第一章\n")))...)
		request, response := directLocalImportMultipartRequest(t, "/api/imports/books/preview", auth, []directLocalImportMultipartPart{
			{name: "file", filename: "exact-file.txt", data: data},
		})
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("exact file limit = %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("exact request envelope reaches the file-size gate", func(t *testing.T) {
		requestLimit := server.maxLocalImportBytes() + directLocalImportEnvelopeBytes
		emptyBody, _ := directLocalImportMultipartBody(t, []directLocalImportMultipartPart{
			{name: "file", filename: "exact-envelope.txt", data: nil},
		})
		payloadBytes := int(requestLimit) - len(emptyBody)
		body, contentType := directLocalImportMultipartBody(t, []directLocalImportMultipartPart{
			{name: "file", filename: "exact-envelope.txt", data: bytes.Repeat([]byte("x"), payloadBytes)},
		})
		if delta := int(requestLimit) - len(body); delta != 0 {
			payloadBytes += delta
			body, contentType = directLocalImportMultipartBody(t, []directLocalImportMultipartPart{
				{name: "file", filename: "exact-envelope.txt", data: bytes.Repeat([]byte("x"), payloadBytes)},
			})
		}
		if int64(len(body)) != requestLimit {
			t.Fatalf("exact envelope fixture = %d bytes, want %d", len(body), requestLimit)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/imports/books/preview", bytes.NewReader(body))
		request.Header.Set("Authorization", auth)
		request.Header.Set("Content-Type", contentType)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		assertDirectLocalImportError(t, response, http.StatusRequestEntityTooLarge, "local book exceeds maximum import size")
	})
}

func TestDirectLocalImportRejectsAmbiguousMultipartBeforeStaging(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	token, err := server.stageLocalImport(1, "token.txt", ".txt", []byte("第一章\n暂存正文"))
	if err != nil {
		t.Fatal(err)
	}
	validFile := directLocalImportMultipartPart{name: "file", filename: "shape.txt", data: []byte("第一章\n正文")}

	tests := []struct {
		name  string
		parts []directLocalImportMultipartPart
	}{
		{
			name: "duplicate file",
			parts: []directLocalImportMultipartPart{
				validFile,
				{name: "file", filename: "second.txt", data: []byte("第二章\n正文")},
			},
		},
		{
			name: "extra file field",
			parts: []directLocalImportMultipartPart{
				validFile,
				{name: "other", filename: "other.txt", data: []byte("第二章\n正文")},
			},
		},
		{
			name: "file and token",
			parts: []directLocalImportMultipartPart{
				validFile,
				{name: "importToken", data: []byte(token)},
			},
		},
		{
			name: "duplicate title",
			parts: []directLocalImportMultipartPart{
				validFile,
				{name: "title", data: []byte("first")},
				{name: "title", data: []byte("second")},
			},
		},
		{
			name: "duplicate token",
			parts: []directLocalImportMultipartPart{
				{name: "importToken", data: []byte(token)},
				{name: "importToken", data: []byte(token)},
			},
		},
		{
			name: "uppercase token",
			parts: []directLocalImportMultipartPart{
				{name: "importToken", data: []byte(strings.ToUpper(token))},
			},
		},
		{
			name: "unknown scalar",
			parts: []directLocalImportMultipartPart{
				validFile,
				{name: "unexpected", data: []byte("value")},
			},
		},
		{
			name: "preview category",
			parts: []directLocalImportMultipartPart{
				validFile,
				{name: "categoryIds", data: []byte("1")},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeStages := directLocalImportStageEntryCount(t, server, 1)
			request, response := directLocalImportMultipartRequest(t, "/api/imports/books/preview", auth, test.parts)
			router.ServeHTTP(response, request)

			assertDirectLocalImportError(t, response, http.StatusBadRequest, "invalid local import request")
			if after := directLocalImportStageEntryCount(t, server, 1); after != beforeStages {
				t.Errorf("invalid multipart changed stage entries: before=%d after=%d", beforeStages, after)
			}
		})
	}
}

func TestDirectLocalImportAcceptsRepeatedCategoriesForBothAliases(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	categories := []models.Category{
		{UserID: user.ID, Name: "direct one"},
		{UserID: user.ID, Name: "direct two"},
	}
	if err := server.db.Create(&categories).Error; err != nil {
		t.Fatal(err)
	}

	for index, endpoint := range []string{"/api/imports/books", "/api/imports/txt"} {
		t.Run(endpoint, func(t *testing.T) {
			request, response := directLocalImportMultipartRequest(t, endpoint, auth, []directLocalImportMultipartPart{
				{name: "file", filename: "compatible-" + strconv.Itoa(index) + ".txt", data: []byte("第一章\n正文")},
				{name: "categoryIds", data: []byte(strconv.FormatUint(uint64(categories[0].ID), 10) + "," + strconv.FormatUint(uint64(categories[1].ID), 10))},
				{name: "categoryIds", data: []byte(strconv.FormatUint(uint64(categories[0].ID), 10))},
				{name: "categoryIds", data: []byte("0,")},
			})
			router.ServeHTTP(response, request)
			if response.Code != http.StatusCreated {
				t.Fatalf("compatible repeated categories = %d: %s", response.Code, response.Body.String())
			}
			var book models.Book
			if err := json.Unmarshal(response.Body.Bytes(), &book); err != nil {
				t.Fatal(err)
			}
			var rows []models.BookCategory
			if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).Order("id ASC").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			got := make([]uint, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.CategoryID)
			}
			want := []uint{categories[0].ID, categories[1].ID}
			if !slices.Equal(got, want) {
				t.Fatalf("category relations = %v, want %v", got, want)
			}
		})
	}
}

func TestDirectLocalImportRejectsMetadataBeforeStaging(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	validFile := directLocalImportMultipartPart{name: "file", filename: "metadata.txt", data: []byte("第一章\n正文")}

	tests := []struct {
		name  string
		parts []directLocalImportMultipartPart
	}{
		{
			name: "filename over 255 bytes",
			parts: []directLocalImportMultipartPart{
				{name: "file", filename: strings.Repeat("a", 252) + ".txt", data: validFile.data},
			},
		},
		{
			name:  "title over 240 bytes",
			parts: []directLocalImportMultipartPart{validFile, {name: "title", data: bytes.Repeat([]byte("t"), 241)}},
		},
		{
			name:  "author over 160 bytes",
			parts: []directLocalImportMultipartPart{validFile, {name: "author", data: bytes.Repeat([]byte("a"), 161)}},
		},
		{
			name:  "toc rule over 16 KiB",
			parts: []directLocalImportMultipartPart{validFile, {name: "tocRule", data: bytes.Repeat([]byte("r"), (16<<10)+1)}},
		},
		{
			name:  "invalid UTF-8 title",
			parts: []directLocalImportMultipartPart{validFile, {name: "title", data: []byte{0xff, 0xfe}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeStages := directLocalImportStageEntryCount(t, server, 1)
			request, response := directLocalImportMultipartRequest(t, "/api/imports/books/preview", auth, test.parts)
			router.ServeHTTP(response, request)

			assertDirectLocalImportError(t, response, http.StatusBadRequest, "invalid local import request")
			if after := directLocalImportStageEntryCount(t, server, 1); after != beforeStages {
				t.Errorf("invalid metadata changed stage entries: before=%d after=%d", beforeStages, after)
			}
		})
	}
}

func TestDirectLocalImportRejectsMalformedCategoriesBeforePersistence(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: user.ID, Name: "direct boundary"}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	file := directLocalImportMultipartPart{name: "file", filename: "category.txt", data: []byte("第一章\n正文")}

	tests := []struct {
		name     string
		endpoint string
		parts    []directLocalImportMultipartPart
	}{
		{
			name:     "201 raw category values",
			endpoint: "/api/imports/books",
			parts: func() []directLocalImportMultipartPart {
				parts := []directLocalImportMultipartPart{file}
				for range 201 {
					parts = append(parts, directLocalImportMultipartPart{name: "categoryIds", data: []byte(strconv.FormatUint(uint64(category.ID), 10))})
				}
				return parts
			}(),
		},
		{
			name:     "malformed category on alias",
			endpoint: "/api/imports/txt",
			parts: []directLocalImportMultipartPart{
				file,
				{name: "categoryIds", data: []byte("not-a-number")},
			},
		},
		{
			name:     "duplicate singular category",
			endpoint: "/api/imports/books",
			parts: []directLocalImportMultipartPart{
				file,
				{name: "categoryId", data: []byte(strconv.FormatUint(uint64(category.ID), 10))},
				{name: "categoryId", data: []byte(strconv.FormatUint(uint64(category.ID), 10))},
			},
		},
		{
			name:     "unknown import scalar",
			endpoint: "/api/imports/txt",
			parts: []directLocalImportMultipartPart{
				file,
				{name: "unexpected", data: []byte("value")},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeBooks := directLocalImportBookCount(t, server)
			beforeLibrary := directLocalImportTreeEntryCount(t, server.cfg.LibraryDir)
			request, response := directLocalImportMultipartRequest(t, test.endpoint, auth, test.parts)
			router.ServeHTTP(response, request)

			assertDirectLocalImportError(t, response, http.StatusBadRequest, "invalid local import request")
			if after := directLocalImportBookCount(t, server); after != beforeBooks {
				t.Errorf("invalid categories changed books: before=%d after=%d", beforeBooks, after)
			}
			if after := directLocalImportTreeEntryCount(t, server.cfg.LibraryDir); after != beforeLibrary {
				t.Errorf("invalid categories changed library tree: before=%d after=%d", beforeLibrary, after)
			}
		})
	}

	t.Run("invalid singular category cannot hide behind category list", func(t *testing.T) {
		beforeBooks := directLocalImportBookCount(t, server)
		beforeLibrary := directLocalImportTreeEntryCount(t, server.cfg.LibraryDir)
		request, response := directLocalImportMultipartRequest(t, "/api/imports/books", auth, []directLocalImportMultipartPart{
			file,
			{name: "categoryId", data: []byte("999999")},
			{name: "categoryIds", data: []byte(strconv.FormatUint(uint64(category.ID), 10))},
		})
		router.ServeHTTP(response, request)

		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "category not found") {
			t.Fatalf("invalid singular category = %d: %s", response.Code, response.Body.String())
		}
		if after := directLocalImportBookCount(t, server); after != beforeBooks {
			t.Errorf("invalid categories changed books: before=%d after=%d", beforeBooks, after)
		}
		if after := directLocalImportTreeEntryCount(t, server.cfg.LibraryDir); after != beforeLibrary {
			t.Errorf("invalid categories changed library tree: before=%d after=%d", beforeLibrary, after)
		}
	})
}

func TestDirectLocalImportRemovesMultipartTemporaryFiles(t *testing.T) {
	router, _ := setupTestServer(t)
	router.MaxMultipartMemory = 1
	auth := authHeader(t, router)
	request, response := directLocalImportMultipartRequest(t, "/api/imports/books/preview", auth, []directLocalImportMultipartPart{
		{name: "file", filename: "temporary.txt", data: bytes.Repeat([]byte("第一章\n正文\n"), 1024)},
	})
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("valid disk-backed preview = %d: %s", response.Code, response.Body.String())
	}
	if request.MultipartForm == nil || len(request.MultipartForm.File["file"]) != 1 {
		t.Fatalf("request did not retain parsed multipart metadata: %+v", request.MultipartForm)
	}
	file, err := request.MultipartForm.File["file"][0].Open()
	if err == nil {
		_ = file.Close()
		t.Fatal("handler left its disk-backed multipart temporary file openable after response")
	}
}

func directLocalImportMultipartBody(t *testing.T, parts []directLocalImportMultipartPart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		var destination interface{ Write([]byte) (int, error) }
		var err error
		if part.filename != "" {
			destination, err = writer.CreateFormFile(part.name, part.filename)
		} else {
			destination, err = writer.CreateFormField(part.name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := destination.Write(part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func directLocalImportMultipartRequest(
	t *testing.T,
	endpoint string,
	auth string,
	parts []directLocalImportMultipartPart,
) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	body, contentType := directLocalImportMultipartBody(t, parts)
	request := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", contentType)
	return request, httptest.NewRecorder()
}

func assertDirectLocalImportError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if response.Code != status {
		t.Errorf("response status = %d, want %d: %s", response.Code, status, response.Body.String())
		return
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Errorf("decode error response: %v: %s", err, response.Body.String())
		return
	}
	if payload.Error != message {
		t.Errorf("response error = %q, want %q", payload.Error, message)
	}
}

func directLocalImportStageEntryCount(t *testing.T, server *Server, userID uint) int {
	t.Helper()
	entries, err := os.ReadDir(server.localImportStageDir(userID))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func directLocalImportBookCount(t *testing.T, server *Server) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.Book{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func directLocalImportTreeEntryCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			count++
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return count
}
