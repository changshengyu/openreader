package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

type localImportMetadataPreview struct {
	Title        string `json:"title"`
	Author       string `json:"author"`
	TOCRule      string `json:"tocRule"`
	ChapterCount int    `json:"chapterCount"`
	ImportToken  string `json:"importToken"`
}

var localImportMetadataTXT = []byte("第1章 开端\n这里是开端的内容。\n第2章 尾声\n这里是尾声的内容。\n")

func decodeLocalImportMetadataPreview(t *testing.T, response *httptest.ResponseRecorder) localImportMetadataPreview {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("preview: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var preview localImportMetadataPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

// BookController.importBookPreview + LocalBook.analyzeNameAuthor: the user's
// file is previewed and imported as 前浪(校对加料版全本 1-425) by 刀一耕, the
// automatically selected TXT rule is shown and saved with the book.
func TestDirectImportRecognizesReaderDevNameAuthorAndTOCRule(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	fileName := "前浪(校对加料版全本 1-425) 作者：刀一耕.txt"
	preview := decodeLocalImportMetadataPreview(t, directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", fileName, localImportMetadataTXT, nil))
	rule := engine.DefaultTXTTocRules()[0].Rule
	if preview.Title != "前浪(校对加料版全本 1-425)" || preview.Author != "刀一耕" || preview.TOCRule != rule || preview.ChapterCount != 2 {
		t.Fatalf("direct preview = %+v", preview)
	}

	importW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", fileName, nil, map[string]string{
		"importToken": preview.ImportToken,
	})
	if importW.Code != http.StatusCreated {
		t.Fatalf("import: expected 201, got %d: %s", importW.Code, importW.Body.String())
	}
	var imported bookListItem
	if err := json.Unmarshal(importW.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	var book models.Book
	if err := server.db.First(&book, imported.ID).Error; err != nil {
		t.Fatal(err)
	}
	if book.Title != "前浪(校对加料版全本 1-425)" || book.Author != "刀一耕" || book.TOCRule != rule {
		t.Fatalf("imported book = %q / %q / %q", book.Title, book.Author, book.TOCRule)
	}
}

// Direct uploads strip \/:*?"<>|. before analyzeNameAuthor; LocalStore and
// WebDAV (importFromLocalPathPreview) use the original file name. A token
// reparse keeps the rule of the source that created the stage.
func TestImportNameNormalizationFollowsTheReaderDevSource(t *testing.T) {
	fileName := "三体.第一部 作者：刘慈欣.txt"

	t.Run("direct upload and token reparse", func(t *testing.T) {
		router, _ := setupTestServer(t)
		auth := authHeader(t, router)
		preview := decodeLocalImportMetadataPreview(t, directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", fileName, localImportMetadataTXT, nil))
		if preview.Title != "三体第一部" || preview.Author != "刘慈欣" {
			t.Fatalf("direct preview = %q / %q", preview.Title, preview.Author)
		}
		reparsed := decodeLocalImportMetadataPreview(t, directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", fileName, nil, map[string]string{
			"importToken": preview.ImportToken,
			"tocRule":     `^第.+章.*$`,
		}))
		if reparsed.Title != "三体第一部" || reparsed.Author != "刘慈欣" || reparsed.TOCRule != `^第.+章.*$` {
			t.Fatalf("direct token reparse = %+v", reparsed)
		}
	})

	t.Run("local store", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)
		path := filepath.Join(server.cfg.LocalStoreDir, fileName)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, localImportMetadataTXT, 0o644); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/local-store/import-preview", strings.NewReader(`{"paths":["`+fileName+`"]}`))
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, req)
		if writer.Code != http.StatusOK {
			t.Fatalf("local store preview: expected 200, got %d: %s", writer.Code, writer.Body.String())
		}
		var response struct {
			Items []struct {
				ImportToken string                      `json:"importToken"`
				Book        *localImportMetadataPreview `json:"book"`
			} `json:"items"`
		}
		if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Items) != 1 || response.Items[0].Book == nil {
			t.Fatalf("local store preview = %s", writer.Body.String())
		}
		item := response.Items[0]
		if item.Book.Title != "三体.第一部" || item.Book.Author != "刘慈欣" || item.Book.TOCRule != engine.DefaultTXTTocRules()[0].Rule {
			t.Fatalf("local store preview book = %+v", item.Book)
		}
		book := importStagedStorageBookWithTOCRule(t, router, auth, "/api/local-store/import", fileName, item.ImportToken, "", "")
		if book.Title != "三体.第一部" || book.Author != "刘慈欣" {
			t.Fatalf("local store book = %q / %q", book.Title, book.Author)
		}
	})
}
