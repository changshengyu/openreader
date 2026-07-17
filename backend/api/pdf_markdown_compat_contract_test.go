package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestLegacyPDFMarkdownAndTextBooksRemainReadableAndRefreshable(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)

	testCases := []struct {
		fileName string
		data     []byte
		wantText string
	}{
		{fileName: "legacy.pdf", data: legacyLocalBookPDF("Legacy PDF body."), wantText: "Legacy PDF body."},
		{fileName: "legacy.md", data: []byte("第一章 Markdown\n# 历史 Markdown 正文"), wantText: "历史 Markdown 正文"},
		{fileName: "legacy.text", data: []byte("第一章 Text\n历史 Text 正文"), wantText: "历史 Text 正文"},
	}

	for _, tt := range testCases {
		t.Run(tt.fileName, func(t *testing.T) {
			book := importLegacyLocalBook(t, router, auth, tt.fileName, tt.data)
			archivePath := filepath.Join(server.cfg.LibraryDir, book.OriginalFile)
			archiveBefore, err := os.ReadFile(archivePath)
			if err != nil {
				t.Fatalf("read preserved original archive: %v", err)
			}

			var chapter models.Chapter
			if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").First(&chapter).Error; err != nil {
				t.Fatal(err)
			}
			if chapter.CachePath == "" {
				t.Fatal("legacy local import did not create an initial chapter cache")
			}
			if err := os.Remove(filepath.Join(server.cfg.LibraryDir, book.LibraryPath, chapter.CachePath)); err != nil && !os.IsNotExist(err) {
				t.Fatalf("remove derived cache: %v", err)
			}

			content := readLegacyLocalBookContent(t, router, auth, book.ID, 0)
			if !strings.Contains(content, tt.wantText) {
				t.Fatalf("rebuilt legacy chapter = %q, want %q", content, tt.wantText)
			}
			if strings.Contains(content, "<h1") {
				t.Fatalf("legacy text-like content must remain plain reader text, got %q", content)
			}

			refresh := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/books/%d/refresh-local", book.ID), nil)
			refresh.Header.Set("Authorization", auth)
			refreshResponse := httptest.NewRecorder()
			router.ServeHTTP(refreshResponse, refresh)
			if refreshResponse.Code != http.StatusOK {
				t.Fatalf("refresh legacy %s: expected 200, got %d: %s", tt.fileName, refreshResponse.Code, refreshResponse.Body.String())
			}
			archiveAfter, err := os.ReadFile(archivePath)
			if err != nil {
				t.Fatalf("read preserved archive after refresh: %v", err)
			}
			if !bytes.Equal(archiveBefore, archiveAfter) {
				t.Fatal("refresh-local rewrote the preserved legacy original archive")
			}
			if content := readLegacyLocalBookContent(t, router, auth, book.ID, 0); !strings.Contains(content, tt.wantText) {
				t.Fatalf("refreshed legacy chapter = %q, want %q", content, tt.wantText)
			}
		})
	}
}

func TestUnreadableOrOverBudgetLegacyPDFDoesNotCreatePersistentBookData(t *testing.T) {
	testCases := []struct {
		name      string
		configure func(*config.Config)
		data      []byte
	}{
		{name: "no-readable-text", data: legacyLocalBookPDF("")},
		{
			name: "parsed-text-budget",
			configure: func(cfg *config.Config) {
				cfg.MaxParsedTextBytes = 1
			},
			data: legacyLocalBookPDF("这段 PDF 正文超过一字节预算。"),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServerWithConfig(t, tt.configure)
			auth := authHeader(t, router)

			for _, endpoint := range []string{"/api/imports/books/preview", "/api/imports/books"} {
				response := directLocalBookMultipartRequest(t, router, auth, endpoint, "legacy.pdf", tt.data, nil)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("%s %s: expected 400, got %d: %s", tt.name, endpoint, response.Code, response.Body.String())
				}
			}

			var books, chapters int64
			if err := server.db.Model(&models.Book{}).Count(&books).Error; err != nil {
				t.Fatal(err)
			}
			if err := server.db.Model(&models.Chapter{}).Count(&chapters).Error; err != nil {
				t.Fatal(err)
			}
			if books != 0 || chapters != 0 {
				t.Fatalf("failed PDF import persisted book data: books=%d chapters=%d", books, chapters)
			}
			entries, err := os.ReadDir(server.cfg.LibraryDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("failed PDF import persisted library data: %+v", entries)
			}
		})
	}
}

func TestLegacyLocalBookStageCannotCrossUsers(t *testing.T) {
	router, server := setupTestServer(t)
	ownerAuth := authHeader(t, router)
	preview := directLocalBookMultipartRequest(
		t,
		router,
		ownerAuth,
		"/api/imports/books/preview",
		"legacy.md",
		[]byte("第一章 历史\n用户 A 的 Markdown 正文"),
		nil,
	)
	if preview.Code != http.StatusOK {
		t.Fatalf("owner preview: expected 200, got %d: %s", preview.Code, preview.Body.String())
	}
	var staged struct {
		ImportToken string `json:"importToken"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	if !validLocalImportToken(staged.ImportToken) {
		t.Fatalf("owner preview did not return a valid stage token: %+v", staged)
	}

	otherAuth := registerLegacyFormatTestUser(t, router, "legacyother", "secret5678")
	response := directLocalBookMultipartRequest(t, router, otherAuth, "/api/imports/books", "", nil, map[string]string{
		"importToken": staged.ImportToken,
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("other user stage import: expected 400, got %d: %s", response.Code, response.Body.String())
	}
	ownerBookResponse := directLocalBookMultipartRequest(t, router, ownerAuth, "/api/imports/books", "", nil, map[string]string{
		"importToken": staged.ImportToken,
	})
	if ownerBookResponse.Code != http.StatusCreated {
		t.Fatalf("owner stage import: expected 201, got %d: %s", ownerBookResponse.Code, ownerBookResponse.Body.String())
	}
	var ownerBook models.Book
	if err := json.Unmarshal(ownerBookResponse.Body.Bytes(), &ownerBook); err != nil {
		t.Fatal(err)
	}
	otherContentRequest := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/books/%d/chapters/0/content", ownerBook.ID), nil)
	otherContentRequest.Header.Set("Authorization", otherAuth)
	otherContentResponse := httptest.NewRecorder()
	router.ServeHTTP(otherContentResponse, otherContentRequest)
	if otherContentResponse.Code != http.StatusNotFound {
		t.Fatalf("other user legacy chapter read: expected 404, got %d: %s", otherContentResponse.Code, otherContentResponse.Body.String())
	}
	var books int64
	if err := server.db.Model(&models.Book{}).Count(&books).Error; err != nil {
		t.Fatal(err)
	}
	if books != 1 {
		t.Fatalf("cross-user stage import changed owner book count: %d", books)
	}
}

func importLegacyLocalBook(t *testing.T, router http.Handler, auth string, fileName string, data []byte) models.Book {
	t.Helper()
	preview := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", fileName, data, nil)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview %s: expected 200, got %d: %s", fileName, preview.Code, preview.Body.String())
	}
	var staged struct {
		ImportToken string `json:"importToken"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	if !validLocalImportToken(staged.ImportToken) {
		t.Fatalf("preview %s did not return a valid stage token: %+v", fileName, staged)
	}
	imported := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "", nil, map[string]string{
		"importToken": staged.ImportToken,
	})
	if imported.Code != http.StatusCreated {
		t.Fatalf("import %s: expected 201, got %d: %s", fileName, imported.Code, imported.Body.String())
	}
	var book models.Book
	if err := json.Unmarshal(imported.Body.Bytes(), &book); err != nil {
		t.Fatal(err)
	}
	if book.ID == 0 || book.OriginalFile == "" || book.LibraryPath == "" {
		t.Fatalf("legacy import response is missing persisted metadata: %+v", book)
	}
	return book
}

func readLegacyLocalBookContent(t *testing.T, router http.Handler, auth string, bookID uint, index int) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/books/%d/chapters/%d/content", bookID, index), nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy chapter content: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Content
}

func registerLegacyFormatTestUser(t *testing.T, router http.Handler, username string, password string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register %s: expected 200, got %d: %s", username, response.Code, response.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Token == "" {
		t.Fatalf("register %s did not return a token", username)
	}
	return "Bearer " + payload.Token
}

func legacyLocalBookPDF(text string) []byte {
	var body strings.Builder
	body.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	writeObject := func(number int, value string) {
		offsets = append(offsets, body.Len())
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", number, value)
	}
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	writeObject(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET\n"
	writeObject(5, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	xref := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&body, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(body.String())
}
