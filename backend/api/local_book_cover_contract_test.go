package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

// Contract: docs/compat/local-book-cover-fixed-baseline-second-audit-p1-contract.md

var localBookCoverJPEG = append([]byte{0xff, 0xd8, 0xff, 0xe0}, []byte("reader-dev cover bytes")...)

func localBookCoverEPUB(t *testing.T, withCover bool) []byte {
	t.Helper()
	return localBookCoverEPUBWithImage(t, withCover, localBookCoverJPEG)
}

func localBookCoverEPUBWithImage(t *testing.T, withCover bool, image []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	write := func(name string, data []byte) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	coverMeta := ""
	if withCover {
		coverMeta = `<meta name="cover" content="cover-img"/>`
	}
	write("META-INF/container.xml", []byte(`<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`))
	write("OEBPS/content.opf", []byte(`<package xmlns="http://www.idpf.org/2007/opf" version="2.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>封面 EPUB</dc:title><dc:creator>封面作者</dc:creator>`+coverMeta+`</metadata>
<manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/><item id="cover-img" href="images/cover.jpg" media-type="image/jpeg"/></manifest>
<spine><itemref idref="c1"/></spine></package>`))
	write("OEBPS/c1.xhtml", []byte(`<html><head><title>第一章</title></head><body><p>正文</p></body></html>`))
	write("OEBPS/images/cover.jpg", image)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// localBookCoverUMD appends umdlib's segment 130 and its cover additional
// section to the reader-dev UMD import fixture.
func localBookCoverUMD(t *testing.T, cover []byte) []byte {
	t.Helper()
	data := append([]byte(nil), readerDevUMDImportFixture(t)...)
	if cover == nil {
		return data
	}
	const coverCheck uint32 = 0x0badcafe
	var check [4]byte
	binary.LittleEndian.PutUint32(check[:], coverCheck)
	data = append(data, '#', 0x82, 0x00, 0x10, 10, 0x01)
	data = append(data, check[:]...)
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(cover)+9))
	data = append(data, '$')
	data = append(data, check[:]...)
	data = append(data, length[:]...)
	return append(data, cover...)
}

type localBookCoverPreview struct {
	CoverURL    string `json:"coverUrl"`
	ImportToken string `json:"importToken"`
}

func previewLocalBookCover(t *testing.T, router http.Handler, auth, fileName string, data []byte) localBookCoverPreview {
	t.Helper()
	response := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", fileName, data, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("preview %s: expected 200, got %d: %s", fileName, response.Code, response.Body.String())
	}
	var preview localBookCoverPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

func importLocalBookCover(t *testing.T, router http.Handler, server *Server, auth, fileName, importToken string) (bookListItem, models.Book) {
	t.Helper()
	response := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", fileName, nil, map[string]string{"importToken": importToken})
	if response.Code != http.StatusCreated {
		t.Fatalf("import %s: expected 201, got %d: %s", fileName, response.Code, response.Body.String())
	}
	var imported bookListItem
	if err := json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	var book models.Book
	if err := server.db.First(&book, imported.ID).Error; err != nil {
		t.Fatal(err)
	}
	return imported, book
}

func shelfAndDetailCoverURLs(t *testing.T, router http.Handler, auth string, bookID uint) (string, string) {
	t.Helper()
	listReq := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	listReq.Header.Set("Authorization", auth)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("shelf: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var items []bookListItem
	if err := json.Unmarshal(listW.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	shelfCover := ""
	for _, item := range items {
		if item.ID == bookID {
			shelfCover = item.CoverURL
		}
	}
	detailReq := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(bookID), 10), nil)
	detailReq.Header.Set("Authorization", auth)
	detailW := httptest.NewRecorder()
	router.ServeHTTP(detailW, detailReq)
	if detailW.Code != http.StatusOK {
		t.Fatalf("detail: expected 200, got %d: %s", detailW.Code, detailW.Body.String())
	}
	var detail bookListItem
	if err := json.Unmarshal(detailW.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return shelfCover, detail.CoverURL
}

func assertServedLocalBookCover(t *testing.T, router http.Handler, auth, label, prefix, coverURL string) {
	t.Helper()
	if !strings.HasPrefix(coverURL, prefix) || strings.Contains(coverURL, strings.TrimPrefix(auth, "Bearer ")) {
		t.Fatalf("%s cover URL must be a same-origin %s capability without the login JWT: %q", label, prefix, coverURL)
	}
	request := httptest.NewRequest(http.MethodGet, coverURL, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" || !bytes.Equal(response.Body.Bytes(), localBookCoverJPEG) {
		t.Fatalf("%s cover: got %d %q %q", label, response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%s cover must be served with nosniff", label)
	}
}

// reader-dev writes an imported EPUB/UMD cover during preview and shows it in
// the import dialog, on the shelf and in book info; a custom cover wins.
func TestEPUBAndUMDCoversFollowReaderDev(t *testing.T) {
	wantPreview := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(localBookCoverJPEG)
	for _, test := range []struct {
		name     string
		fileName string
		data     func(*testing.T) []byte
		prefix   string
	}{
		{name: "EPUB", fileName: "封面.epub", data: func(t *testing.T) []byte { return localBookCoverEPUB(t, true) }, prefix: "/api/epub-resource/"},
		{name: "UMD", fileName: "封面.umd", data: func(t *testing.T) []byte { return localBookCoverUMD(t, localBookCoverJPEG) }, prefix: "/api/umd-cover/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			preview := previewLocalBookCover(t, router, auth, test.fileName, test.data(t))
			if preview.CoverURL != wantPreview {
				t.Fatalf("preview coverUrl = %.80q, want the cover as a data URL", preview.CoverURL)
			}
			imported, book := importLocalBookCover(t, router, server, auth, test.fileName, preview.ImportToken)
			if book.CoverURL != "" {
				t.Fatalf("the cover capability must not be stored in the book row: %q", book.CoverURL)
			}
			assertServedLocalBookCover(t, router, auth, "import response", test.prefix, imported.CoverURL)
			shelfCover, detailCover := shelfAndDetailCoverURLs(t, router, auth, book.ID)
			assertServedLocalBookCover(t, router, auth, "shelf", test.prefix, shelfCover)
			assertServedLocalBookCover(t, router, auth, "detail", test.prefix, detailCover)

			if err := server.db.Model(&models.Book{}).Where("id = ?", book.ID).Update("custom_cover_url", "/uploads/covers/user-choice.jpg").Error; err != nil {
				t.Fatal(err)
			}
			_, detailCover = shelfAndDetailCoverURLs(t, router, auth, book.ID)
			if detailCover != "" {
				t.Fatalf("a custom cover must stay first without a generated fallback: %q", detailCover)
			}
		})
	}
}

func TestCoverlessEPUBAndUMDKeepTheDefaultCover(t *testing.T) {
	for _, test := range []struct {
		fileName string
		data     func(*testing.T) []byte
	}{
		{fileName: "无封面.epub", data: func(t *testing.T) []byte { return localBookCoverEPUB(t, false) }},
		{fileName: "无封面.umd", data: func(t *testing.T) []byte { return localBookCoverUMD(t, nil) }},
		{fileName: "坏封面.umd", data: func(t *testing.T) []byte { return localBookCoverUMD(t, []byte("not an image")) }},
	} {
		t.Run(test.fileName, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			preview := previewLocalBookCover(t, router, auth, test.fileName, test.data(t))
			if preview.CoverURL != "" {
				t.Fatalf("preview coverUrl = %.80q, want none", preview.CoverURL)
			}
			imported, book := importLocalBookCover(t, router, server, auth, test.fileName, preview.ImportToken)
			shelfCover, detailCover := shelfAndDetailCoverURLs(t, router, auth, book.ID)
			if imported.CoverURL != "" || shelfCover != "" || detailCover != "" {
				t.Fatalf("a book without a cover keeps the default cover: %q %q %q", imported.CoverURL, shelfCover, detailCover)
			}
		})
	}
}

// Books imported before covers existed have extraction markers without any
// cover information; their cover must still appear.
func TestEPUBCoverAppearsForAnExtractionMarkerWithoutCoverInfo(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	preview := previewLocalBookCover(t, router, auth, "旧书.epub", localBookCoverEPUB(t, true))
	_, book := importLocalBookCover(t, router, server, auth, "旧书.epub", preview.ImportToken)
	markers, err := filepath.Glob(filepath.Join(server.cfg.LibraryDir, book.LibraryPath, ".epub-resources", "*", ".openreader-complete"))
	if err != nil || len(markers) != 1 {
		t.Fatalf("EPUB extraction markers = %v, %v", markers, err)
	}
	// Raw values: decoding modTimeNano into float64 would lose nanoseconds and
	// make the marker describe a different file.
	var marker map[string]json.RawMessage
	data, err := os.ReadFile(markers[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(map[string]json.RawMessage{
		"fingerprint": marker["fingerprint"],
		"size":        marker["size"],
		"modTimeNano": marker["modTimeNano"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markers[0], legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	shelfCover, _ := shelfAndDetailCoverURLs(t, router, auth, book.ID)
	assertServedLocalBookCover(t, router, auth, "legacy shelf", "/api/epub-resource/", shelfCover)
}

func TestUMDCoverCapabilityRejectsTamperingAndDeletedBooks(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	preview := previewLocalBookCover(t, router, auth, "封面.umd", localBookCoverUMD(t, localBookCoverJPEG))
	imported, book := importLocalBookCover(t, router, server, auth, "封面.umd", preview.ImportToken)
	if !strings.HasPrefix(imported.CoverURL, "/api/umd-cover/") {
		t.Fatalf("UMD cover URL = %q", imported.CoverURL)
	}
	// The capability is "<payload>.<signature>" in base64url; change one full
	// sextet in the middle of the signature so it decodes to other bytes.
	capability := strings.TrimPrefix(imported.CoverURL, "/api/umd-cover/")
	flipped := []byte(capability)
	middle := strings.LastIndex(capability, ".") + (len(capability)-strings.LastIndex(capability, "."))/2
	if flipped[middle] == 'A' {
		flipped[middle] = 'B'
	} else {
		flipped[middle] = 'A'
	}
	for _, test := range []struct {
		url    string
		status int
	}{
		{url: "/api/umd-cover/" + string(flipped), status: http.StatusForbidden},
		{url: "/api/umd-cover/not-a-capability", status: http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodGet, test.url, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s: expected %d, got %d: %s", test.url, test.status, response.Code, response.Body.String())
		}
	}
	if err := server.db.Delete(&models.Book{}, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, imported.CoverURL, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("a deleted book's UMD cover: expected 404, got %d", response.Code)
	}
}

// A LocalStore/WebDAV preview carries up to 200 books, so their inline covers
// share one 8 MiB response budget (contract A7); a single-row reparse gets a
// fresh budget.
func TestBatchPreviewCoversShareOneResponseBudget(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	largeCover := append(append([]byte(nil), localBookCoverJPEG...), bytes.Repeat([]byte{0x42}, 3<<19)...)
	paths := make([]string, 0, 6)
	for index := 0; index < 6; index++ {
		name := "大封面" + strconv.Itoa(index) + ".epub"
		if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, name), localBookCoverEPUBWithImage(t, true, largeCover), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	type previewItems struct {
		Items []struct {
			Path        string `json:"path"`
			Error       string `json:"error"`
			ImportToken string `json:"importToken"`
			Book        struct {
				CoverURL string `json:"coverUrl"`
			} `json:"book"`
		} `json:"items"`
	}
	preview := func(body any) previewItems {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/local-store/import-preview", bytes.NewReader(encoded))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("LocalStore preview: %d %s", response.Code, response.Body.String())
		}
		var result previewItems
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	batch := preview(map[string]any{"paths": paths})
	if len(batch.Items) != len(paths) {
		t.Fatalf("preview items = %d, want %d", len(batch.Items), len(paths))
	}
	total, coverless := 0, -1
	for index, item := range batch.Items {
		if item.Error != "" || item.ImportToken == "" {
			t.Fatalf("item %d must still preview: %+v", index, item.Error)
		}
		total += len(item.Book.CoverURL)
		if item.Book.CoverURL == "" && coverless < 0 {
			coverless = index
		}
	}
	if total > 8<<20 {
		t.Fatalf("inline preview covers = %d bytes in one response, want at most 8 MiB", total)
	}
	if !strings.HasPrefix(batch.Items[0].Book.CoverURL, "data:image/jpeg;base64,") {
		t.Fatal("the first row must keep its preview cover")
	}
	if coverless < 0 {
		t.Fatal("rows beyond the response budget must fall back to the default cover")
	}

	reparse := preview(map[string]any{"items": []map[string]string{{
		"path": batch.Items[coverless].Path, "importToken": batch.Items[coverless].ImportToken,
	}}})
	if len(reparse.Items) != 1 || !strings.HasPrefix(reparse.Items[0].Book.CoverURL, "data:image/jpeg;base64,") {
		t.Fatalf("a single-row reparse must bring the cover back: %+v", reparse.Items)
	}
}
