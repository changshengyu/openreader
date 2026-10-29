package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/encoding/simplifiedchinese"

	"openreader/backend/models"
)

// txtCharsetProbeBytes is reader-dev TextFile's 512000-byte bufferSize, the
// prefix of a TXT book whose charset is detected before the whole file is
// decoded.
const txtCharsetProbeBytes = 512000

const txtProbeBoundaryMarker = "探测边界之后的第二章正文。"

// gbkTXTSplitAtCharsetProbe returns a two-chapter GBK book whose charset probe
// ends on the lead byte of a double-byte character.
func gbkTXTSplitAtCharsetProbe(t *testing.T) []byte {
	t.Helper()
	body := strings.Repeat("孔泉抬手、做投降状，片刻后才开口说话。\n", txtCharsetProbeBytes/39+64)
	for padding := 0; padding < 4; padding++ {
		text := "第1章 开端\n" + strings.Repeat(" ", padding) + body + "第2章 尾声\n" + txtProbeBoundaryMarker + "\n"
		data, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		probe, err := simplifiedchinese.GBK.NewDecoder().Bytes(data[:txtCharsetProbeBytes])
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(string(probe), "�") {
			return data
		}
	}
	t.Fatal("GBK fixture does not split a character at the charset probe end")
	return nil
}

func TestTXTSplitAtCharsetProbeImportsThroughEveryLocalEntry(t *testing.T) {
	data := gbkTXTSplitAtCharsetProbe(t)
	if len(data) <= txtCharsetProbeBytes {
		t.Fatalf("fixture must be longer than the charset probe, got %d bytes", len(data))
	}

	t.Run("direct upload", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)
		previewW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", "split-probe.txt", data, nil)
		if previewW.Code != http.StatusOK {
			t.Fatalf("preview: expected 200, got %d: %s", previewW.Code, previewW.Body.String())
		}
		var preview struct {
			ChapterCount int    `json:"chapterCount"`
			ImportToken  string `json:"importToken"`
		}
		if err := json.Unmarshal(previewW.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.ChapterCount != 2 || !validLocalImportToken(preview.ImportToken) {
			t.Fatalf("preview = %+v, want two staged chapters", preview)
		}
		importW := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "split-probe.txt", nil, map[string]string{
			"importToken": preview.ImportToken,
		})
		if importW.Code != http.StatusCreated {
			t.Fatalf("stage import: expected 201, got %d: %s", importW.Code, importW.Body.String())
		}
		var imported bookListItem
		if err := json.Unmarshal(importW.Body.Bytes(), &imported); err != nil {
			t.Fatal(err)
		}
		assertTXTProbeBoundaryBookReadable(t, router, server, auth, imported.ID)
	})

	storageTests := []struct {
		name            string
		previewEndpoint string
		importEndpoint  string
		filePath        func(*Server) string
	}{
		{
			name:            "local store",
			previewEndpoint: "/api/local-store/import-preview",
			importEndpoint:  "/api/local-store/import",
			filePath: func(server *Server) string {
				return filepath.Join(server.cfg.LocalStoreDir, "split-probe.txt")
			},
		},
		{
			name:            "WebDAV",
			previewEndpoint: "/api/webdav/import-preview",
			importEndpoint:  "/api/webdav/import",
			filePath: func(server *Server) string {
				return filepath.Join(server.cfg.DataDir, "webdav", "split-probe.txt")
			},
		},
	}
	for _, tt := range storageTests {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			path := tt.filePath(server)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create fixture root: %v", err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatalf("write GBK fixture: %v", err)
			}
			preview := previewStorageBook(t, router, auth, tt.previewEndpoint, "split-probe.txt")
			if preview.Items[0].Book.ChapterCount != 2 {
				t.Fatalf("preview = %+v, want two staged chapters", preview.Items[0])
			}
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove mounted source after preview: %v", err)
			}
			book := importStagedStorageBook(t, router, auth, tt.importEndpoint, "split-probe.txt", preview.Items[0].ImportToken, "探测边界导入")
			assertTXTProbeBoundaryBookReadable(t, router, server, auth, book.ID)
		})
	}
}

// assertTXTProbeBoundaryBookReadable checks the imported catalogue and rebuilds
// the chapter after the probe from the archived bytes.
func assertTXTProbeBoundaryBookReadable(t *testing.T, router *gin.Engine, server *Server, auth string, bookID uint) {
	t.Helper()
	var book models.Book
	if err := server.db.First(&book, bookID).Error; err != nil {
		t.Fatal(err)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Title != "第1章 开端" || chapters[1].Title != "第2章 尾声" {
		t.Fatalf("imported chapters = %+v", chapters)
	}
	if err := os.Remove(filepath.Join(server.cfg.LibraryDir, book.LibraryPath, chapters[1].CachePath)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove derived chapter cache: %v", err)
	}
	contentReq := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/1/content", nil)
	contentReq.Header.Set("Authorization", auth)
	contentW := httptest.NewRecorder()
	router.ServeHTTP(contentW, contentReq)
	if contentW.Code != http.StatusOK {
		t.Fatalf("rebuild chapter after the probe: expected 200, got %d: %s", contentW.Code, contentW.Body.String())
	}
	var content struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(contentW.Body.Bytes(), &content); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content.Content, txtProbeBoundaryMarker) {
		t.Fatalf("rebuilt chapter after the probe lost %q: %q", txtProbeBoundaryMarker, content.Content)
	}
}
