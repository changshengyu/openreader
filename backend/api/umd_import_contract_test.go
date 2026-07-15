package api

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

// readerDevUMDImportFixture is a text UMD stream laid out by reader-dev's
// UmdHeader/UmdChapters writer: a 0xde9a9b89 header, 83 offsets, 84 titles,
// zlib chunks separated by F1 records, and the final 81 chunk-check table.
// Keeping it in the API package makes every import entrypoint consume the
// same upstream-compatible binary rather than an engine-only pseudo format.
func readerDevUMDImportFixture(t *testing.T) []byte {
	t.Helper()
	type chapter struct {
		title   string
		content string
	}
	chapters := []chapter{
		{title: "第一章", content: "第一段\u2029第二段"},
		{title: "第二章", content: "第二章正文"},
	}

	utf16le := func(value string) []byte {
		encoded := make([]byte, 0, len(value)*2)
		for _, unit := range value {
			if unit > 0xffff {
				t.Fatalf("test UMD fixture only supports BMP runes: %q", value)
			}
			encoded = append(encoded, byte(unit), byte(unit>>8))
		}
		return encoded
	}

	var result bytes.Buffer
	writeSection := func(segmentType uint16, flag byte, payload []byte) {
		t.Helper()
		if len(payload)+5 > 0xff {
			t.Fatalf("fixture section payload too large: %d", len(payload))
		}
		result.WriteByte('#')
		var number [2]byte
		binary.LittleEndian.PutUint16(number[:], segmentType)
		result.Write(number[:])
		result.WriteByte(flag)
		result.WriteByte(byte(len(payload) + 5))
		result.Write(payload)
	}
	writeAdditional := func(check uint32, payload []byte) {
		result.WriteByte('$')
		var number [4]byte
		binary.LittleEndian.PutUint32(number[:], check)
		result.Write(number[:])
		binary.LittleEndian.PutUint32(number[:], uint32(len(payload)+9))
		result.Write(number[:])
		result.Write(payload)
	}
	uint32Payload := func(value uint32) []byte {
		var payload [4]byte
		binary.LittleEndian.PutUint32(payload[:], value)
		return payload[:]
	}

	result.Write([]byte{0x89, 0x9b, 0x9a, 0xde})
	writeSection(0x01, 0, []byte{0x01, 0x11, 0x22})
	writeSection(0x02, 0, utf16le("上游 UMD 导入"))
	writeSection(0x03, 0, utf16le("导入作者"))

	var contents bytes.Buffer
	offsets := make([]uint32, 0, len(chapters))
	titles := make([]byte, 0)
	for _, value := range chapters {
		offsets = append(offsets, uint32(contents.Len()))
		contents.Write(utf16le(value.content))
		encodedTitle := utf16le(value.title)
		titles = append(titles, byte(len(encodedTitle)))
		titles = append(titles, encodedTitle...)
	}
	writeSection(0x0b, 0, uint32Payload(uint32(contents.Len())))
	const offsetCheck uint32 = 0x11223344
	writeSection(0x83, 0, uint32Payload(offsetCheck))
	offsetPayload := make([]byte, len(offsets)*4)
	for index, offset := range offsets {
		binary.LittleEndian.PutUint32(offsetPayload[index*4:], offset)
	}
	writeAdditional(offsetCheck, offsetPayload)

	const titleCheck uint32 = 0x55667788
	writeSection(0x84, 1, uint32Payload(titleCheck))
	writeAdditional(titleCheck, titles)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(contents.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	const chunkCheck uint32 = 0x99aabbcc
	writeAdditional(chunkCheck, compressed.Bytes())
	writeSection(0x00f1, 0, make([]byte, 16))
	writeSection(0x0081, 1, make([]byte, 4))
	writeAdditional(0, uint32Payload(chunkCheck))
	return result.Bytes()
}

func directUMDImportRequest(t *testing.T, router http.Handler, auth, endpoint, importToken string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if data != nil {
		part, err := writer.CreateFormFile("file", "reader-dev.umd")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if importToken != "" {
		if err := writer.WriteField("importToken", importToken); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, endpoint, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

// legacyOpenReaderUMDArchiveFixture deliberately uses the pre-reader-dev
// #TEXTNOV layout that early OpenReader versions wrote. It is not an
// upstream UMD sample and must never become the normal import format; it
// represents an already-mounted historical library archive whose generated
// chapter cache was lost during an upgrade or cleanup.
func legacyOpenReaderUMDArchiveFixture(t *testing.T) []byte {
	t.Helper()
	var result bytes.Buffer
	result.WriteString("#TEXTNOV")
	result.Write(make([]byte, 6)) // declared length + legacy key
	writeString := func(value string) {
		result.WriteByte(byte(len(value)))
		result.WriteString(value)
	}
	writeString("Legacy pseudo UMD")
	writeString("OpenReader")
	result.Write(make([]byte, 5)) // legacy date
	result.WriteByte(0)           // no legacy content-type table
	var chapterCount [4]byte
	binary.LittleEndian.PutUint32(chapterCount[:], 2)
	result.Write(chapterCount[:])

	offsetPosition := result.Len()
	result.Write(make([]byte, 12)) // two chapter starts and one final end
	writeString("Legacy One")
	writeString("Legacy Two")
	contentStart := result.Len()
	first := "legacy body one"
	second := "legacy body two"
	result.WriteString(first)
	result.WriteString(second)
	contentEnd := result.Len()

	data := result.Bytes()
	binary.LittleEndian.PutUint32(data[offsetPosition:], uint32(contentStart))
	binary.LittleEndian.PutUint32(data[offsetPosition+4:], uint32(contentStart+len(first)))
	binary.LittleEndian.PutUint32(data[offsetPosition+8:], uint32(contentEnd))
	for len(data) < 256 {
		data = append(data, 0)
	}
	return data
}

func localUMDChapterContent(t *testing.T, router http.Handler, auth string, bookID uint, index int) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(bookID), 10)+"/chapters/"+strconv.Itoa(index)+"/content", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("UMD chapter content: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Content
}

func removeDerivedUMDChapterCache(t *testing.T, server *Server, book models.Book, chapter models.Chapter) {
	t.Helper()
	if strings.TrimSpace(chapter.CachePath) == "" {
		t.Fatalf("UMD chapter %d has no derived cache path", chapter.Index)
	}
	path := filepath.Join(server.cfg.LibraryDir, book.LibraryPath, chapter.CachePath)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove UMD derived chapter cache %q: %v", path, err)
	}
}

func assertUMDChapterCacheRebuild(t *testing.T, router http.Handler, server *Server, auth string, book models.Book, index int, want string) {
	t.Helper()
	var chapter models.Chapter
	if err := server.db.Where("book_id = ? AND `index` = ?", book.ID, index).First(&chapter).Error; err != nil {
		t.Fatalf("load UMD chapter %d: %v", index, err)
	}
	removeDerivedUMDChapterCache(t, server, book, chapter)
	if got := localUMDChapterContent(t, router, auth, book.ID, index); got != want {
		t.Fatalf("rebuilt UMD chapter %d content = %q, want %q", index, got, want)
	}
	if err := server.db.Where("id = ?", chapter.ID).First(&chapter).Error; err != nil {
		t.Fatalf("reload rebuilt UMD chapter %d: %v", index, err)
	}
	if strings.TrimSpace(chapter.CachePath) == "" {
		t.Fatalf("rebuilt UMD chapter %d has no cache path", index)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LibraryDir, book.LibraryPath, chapter.CachePath)); err != nil {
		t.Fatalf("rebuilt UMD chapter %d cache: %v", index, err)
	}
}

func TestReaderDevUMDRebuildsArchivedChaptersAndRefreshes(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	data := readerDevUMDImportFixture(t)

	preview := directUMDImportRequest(t, router, auth, "/api/imports/books/preview", "", data)
	if preview.Code != http.StatusOK {
		t.Fatalf("standard UMD preview: expected 200, got %d: %s", preview.Code, preview.Body.String())
	}
	var staged struct {
		ImportToken string `json:"importToken"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	imported := directUMDImportRequest(t, router, auth, "/api/imports/books", staged.ImportToken, nil)
	if imported.Code != http.StatusCreated {
		t.Fatalf("standard UMD import: expected 201, got %d: %s", imported.Code, imported.Body.String())
	}
	var book models.Book
	if err := server.db.Where("title = ?", "上游 UMD 导入").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(server.cfg.LibraryDir, book.OriginalFile)
	original, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read standard UMD archive: %v", err)
	}

	assertUMDChapterCacheRebuild(t, router, server, auth, book, 0, "第一段\n第二段")
	refresh := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	refresh.Header.Set("Authorization", auth)
	refreshResponse := httptest.NewRecorder()
	router.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("standard UMD refresh: expected 200, got %d: %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	if current, err := os.ReadFile(originalPath); err != nil || !bytes.Equal(current, original) {
		t.Fatalf("standard UMD refresh must not rewrite archive, data equal=%t err=%v", bytes.Equal(current, original), err)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Index != 0 || chapters[0].Title != "第一章" || chapters[1].Index != 1 || chapters[1].Title != "第二章" {
		t.Fatalf("refreshed UMD chapters = %+v", chapters)
	}
	assertUMDChapterCacheRebuild(t, router, server, auth, book, 1, "第二章正文")
}

func TestLegacyPseudoUMDArchiveRebuildsWithoutMigration(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	libraryPath := filepath.Join("data", "testuser", "legacy-pseudo-umd")
	originalFile := filepath.Join(libraryPath, "legacy.umd")
	archivePath := filepath.Join(server.cfg.LibraryDir, originalFile)
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, legacyOpenReaderUMDArchiveFixture(t), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "Legacy pseudo UMD", Author: "OpenReader", URL: "local://legacy-pseudo-umd", LibraryPath: libraryPath, OriginalFile: originalFile, ChapterCount: 2}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "Legacy One", URL: "local://legacy-pseudo-umd/chapter_0", CachePath: filepath.Join("content", "missing.txt")}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	if got := localUMDChapterContent(t, router, auth, book.ID, 0); got != "legacy body one" {
		t.Fatalf("legacy pseudo UMD rebuilt content = %q", got)
	}
	if err := server.db.First(&chapter, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if chapter.CachePath == filepath.Join("content", "missing.txt") || strings.TrimSpace(chapter.CachePath) == "" {
		t.Fatalf("legacy pseudo UMD cache path was not migrated lazily: %+v", chapter)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LibraryDir, book.LibraryPath, chapter.CachePath)); err != nil {
		t.Fatalf("legacy pseudo UMD rebuilt cache: %v", err)
	}
}

func TestReaderDevUMDImportsAcrossAllLocalEntrypoints(t *testing.T) {
	data := readerDevUMDImportFixture(t)

	t.Run("direct upload stages and imports standard UMD", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)

		previewWriter := directUMDImportRequest(t, router, auth, "/api/imports/books/preview", "", data)
		if previewWriter.Code != http.StatusOK {
			t.Fatalf("UMD upload preview: expected 200, got %d: %s", previewWriter.Code, previewWriter.Body.String())
		}
		var preview struct {
			Title        string `json:"title"`
			Author       string `json:"author"`
			ChapterCount int    `json:"chapterCount"`
			ImportToken  string `json:"importToken"`
		}
		if err := json.Unmarshal(previewWriter.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.Title != "上游 UMD 导入" || preview.Author != "导入作者" || preview.ChapterCount != 2 || !validLocalImportToken(preview.ImportToken) {
			t.Fatalf("UMD upload preview = %+v", preview)
		}
		dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), preview.ImportToken)
		if _, err := os.Stat(dataPath); err != nil {
			t.Fatalf("UMD preview staging data: %v", err)
		}
		if _, err := os.Stat(metadataPath); err != nil {
			t.Fatalf("UMD preview staging metadata: %v", err)
		}

		importWriter := directUMDImportRequest(t, router, auth, "/api/imports/books", preview.ImportToken, nil)
		if importWriter.Code != http.StatusCreated {
			t.Fatalf("UMD staged upload import: expected 201, got %d: %s", importWriter.Code, importWriter.Body.String())
		}
		var book models.Book
		if err := server.db.Where("title = ?", "上游 UMD 导入").First(&book).Error; err != nil {
			t.Fatalf("load imported UMD book: %v", err)
		}
		if book.Author != "导入作者" || book.ChapterCount != 2 {
			t.Fatalf("imported UMD book = %+v", book)
		}
		if _, err := os.Stat(dataPath); !os.IsNotExist(err) {
			t.Fatalf("consumed UMD staging data must be removed, got %v", err)
		}
		if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
			t.Fatalf("consumed UMD staging metadata must be removed, got %v", err)
		}
	})

	t.Run("corrupted upload retains a safe retry stage", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)
		corrupted := append([]byte(nil), data...)
		compressedStart := bytes.Index(corrupted, []byte{0x78, 0x9c})
		if compressedStart < 0 || compressedStart+2 >= len(corrupted) {
			t.Fatal("fixture has no zlib body to corrupt")
		}
		corrupted[compressedStart+2] ^= 0xff

		previewWriter := directUMDImportRequest(t, router, auth, "/api/imports/books/preview", "", corrupted)
		if previewWriter.Code != http.StatusBadRequest {
			t.Fatalf("corrupted UMD preview: expected 400, got %d: %s", previewWriter.Code, previewWriter.Body.String())
		}
		var failure struct {
			Error       string `json:"error"`
			ImportToken string `json:"importToken"`
		}
		if err := json.Unmarshal(previewWriter.Body.Bytes(), &failure); err != nil {
			t.Fatal(err)
		}
		if !validLocalImportToken(failure.ImportToken) {
			t.Fatalf("corrupted UMD must retain a retry token: %+v", failure)
		}
		if strings.Contains(failure.Error, server.cfg.DataDir) || strings.Contains(failure.Error, server.cfg.LibraryDir) {
			t.Fatalf("corrupted UMD error leaks a host path: %q", failure.Error)
		}
		dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), failure.ImportToken)
		if _, err := os.Stat(dataPath); err != nil {
			t.Fatalf("failed UMD preview must retain staged bytes: %v", err)
		}
		if _, err := os.Stat(metadataPath); err != nil {
			t.Fatalf("failed UMD preview must retain staged metadata: %v", err)
		}
	})

	for _, entrypoint := range []struct {
		name            string
		previewEndpoint string
		importEndpoint  string
		filePath        func(*Server) string
	}{
		{
			name:            "LocalStore",
			previewEndpoint: "/api/local-store/import-preview",
			importEndpoint:  "/api/local-store/import",
			filePath: func(server *Server) string {
				return filepath.Join(server.cfg.LocalStoreDir, "reader-dev.umd")
			},
		},
		{
			name:            "WebDAV",
			previewEndpoint: "/api/webdav/import-preview",
			importEndpoint:  "/api/webdav/import",
			filePath: func(server *Server) string {
				return filepath.Join(server.cfg.DataDir, "webdav", "reader-dev.umd")
			},
		},
	} {
		t.Run(entrypoint.name+" stages source bytes before confirm", func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			path := entrypoint.filePath(server)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create %s fixture directory: %v", entrypoint.name, err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatalf("write %s UMD fixture: %v", entrypoint.name, err)
			}

			preview := previewStorageBook(t, router, auth, entrypoint.previewEndpoint, "reader-dev.umd")
			if preview.Items[0].Book.ChapterCount != 2 {
				t.Fatalf("%s UMD preview = %+v", entrypoint.name, preview.Items[0])
			}
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove %s source after preview: %v", entrypoint.name, err)
			}

			book := importStagedStorageBook(t, router, auth, entrypoint.importEndpoint, "reader-dev.umd", preview.Items[0].ImportToken, entrypoint.name+" UMD 快照")
			if book.ChapterCount != 2 {
				t.Fatalf("%s staged UMD import = %+v", entrypoint.name, book)
			}
			assertUMDChapterCacheRebuild(t, router, server, auth, book, 0, "第一段\n第二段")
		})
	}
}
