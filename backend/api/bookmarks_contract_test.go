package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

func bookmarkContractRequest(t *testing.T, router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func bookmarkContractBook(t *testing.T, server *Server, userID uint, title string) (models.Book, models.Chapter) {
	t.Helper()
	book := models.Book{UserID: userID, Title: title, URL: "local://" + title}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return book, chapter
}

func bookmarkContractUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func TestBookmarkCreateRejectsEmptyContextAndForeignChapter(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := bookmarkContractUser(t, server)
	book, _ := bookmarkContractBook(t, server, user.ID, "书签契约书")
	_, foreignChapter := bookmarkContractBook(t, server, user.ID, "另一册书")

	empty := bookmarkContractRequest(t, router, http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/bookmarks", `{
		"chapterIndex":0,"title":"第一章","excerpt":""
	}`, token)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty bookmark context must be rejected, got %d: %s", empty.Code, empty.Body.String())
	}

	foreign := bookmarkContractRequest(t, router, http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/bookmarks", `{
		"chapterId":`+strconv.FormatUint(uint64(foreignChapter.ID), 10)+`,"chapterIndex":0,"title":"第一章","excerpt":"上下文"
	}`, token)
	if foreign.Code != http.StatusBadRequest {
		t.Fatalf("foreign chapter must be rejected, got %d: %s", foreign.Code, foreign.Body.String())
	}

	batch := bookmarkContractRequest(t, router, http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/bookmarks/batch", `[
		{"chapterIndex":0,"title":"第一章","excerpt":"有效上下文"},
		{"chapterIndex":0,"title":"第一章","excerpt":""}
	]`, token)
	if batch.Code != http.StatusBadRequest {
		t.Fatalf("mixed-validity bookmark batch must fail atomically, got %d: %s", batch.Code, batch.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.Bookmark{}).Where("book_id = ?", book.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected bookmark batch must not persist valid prefix, got %d rows", count)
	}
}

func TestBookmarkRoutesHideRowsAcrossUsers(t *testing.T) {
	router, server := setupTestServer(t)
	ownerToken := registerLifecycleToken(t, router, "bookmarkowner")
	otherToken := registerLifecycleToken(t, router, "bookmarkother")
	owner := lifecycleUser(t, server, "bookmarkowner")
	book, chapter := bookmarkContractBook(t, server, owner.ID, "用户隔离书签书")
	bookmark := models.Bookmark{
		UserID: owner.ID, BookID: book.ID, ChapterID: chapter.ID,
		ChapterIndex: chapter.Index, Title: chapter.Title, Excerpt: "仅 owner 可见",
	}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	bookPath := "/api/books/" + strconv.FormatUint(uint64(book.ID), 10) + "/bookmarks"
	bookmarkPath := "/api/bookmarks/" + strconv.FormatUint(uint64(bookmark.ID), 10)
	for _, response := range []*httptest.ResponseRecorder{
		bookmarkContractRequest(t, router, http.MethodGet, bookPath, "", otherToken),
		bookmarkContractRequest(t, router, http.MethodPut, bookmarkPath, `{"note":"越权编辑"}`, otherToken),
		bookmarkContractRequest(t, router, http.MethodDelete, bookmarkPath, "", otherToken),
		bookmarkContractRequest(t, router, http.MethodPost, bookPath+"/batch-delete", `{"ids":[`+strconv.FormatUint(uint64(bookmark.ID), 10)+`]}`, otherToken),
	} {
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-user bookmark route must return 404, got %d: %s", response.Code, response.Body.String())
		}
	}

	var persisted models.Bookmark
	if err := server.db.First(&persisted, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Note != "" || persisted.Excerpt != bookmark.Excerpt {
		t.Fatalf("cross-user requests mutated bookmark: %+v", persisted)
	}

	ownerList := bookmarkContractRequest(t, router, http.MethodGet, bookPath, "", ownerToken)
	if ownerList.Code != http.StatusOK || !strings.Contains(ownerList.Body.String(), bookmark.Excerpt) {
		t.Fatalf("owner bookmark list failed after denied requests: %d %s", ownerList.Code, ownerList.Body.String())
	}
}

func TestBookmarkBatchDeleteScopesIDsToTheCurrentBook(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := bookmarkContractUser(t, server)
	currentBook, currentChapter := bookmarkContractBook(t, server, user.ID, "当前书签书")
	otherBook, otherChapter := bookmarkContractBook(t, server, user.ID, "同账号另一册书")
	currentBookmark := models.Bookmark{
		UserID: user.ID, BookID: currentBook.ID, ChapterID: currentChapter.ID,
		ChapterIndex: currentChapter.Index, Title: currentChapter.Title, Excerpt: "当前书上下文",
	}
	otherBookmark := models.Bookmark{
		UserID: user.ID, BookID: otherBook.ID, ChapterID: otherChapter.ID,
		ChapterIndex: otherChapter.Index, Title: otherChapter.Title, Excerpt: "另一册书上下文",
	}
	if err := server.db.Create(&currentBookmark).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&otherBookmark).Error; err != nil {
		t.Fatal(err)
	}

	path := "/api/books/" + strconv.FormatUint(uint64(currentBook.ID), 10) + "/bookmarks/batch-delete"
	response := bookmarkContractRequest(t, router, http.MethodPost, path, `{"ids":[`+
		strconv.FormatUint(uint64(otherBookmark.ID), 10)+`,`+
		strconv.FormatUint(uint64(currentBookmark.ID), 10)+`]}`, token)
	if response.Code != http.StatusOK {
		t.Fatalf("current-book batch delete: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		DeletedIDs []uint `json:"deletedIds"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.DeletedIDs) != 1 || result.DeletedIDs[0] != currentBookmark.ID {
		t.Fatalf("batch delete leaked another book id: %+v", result.DeletedIDs)
	}
	if err := server.db.First(&models.Bookmark{}, currentBookmark.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("current-book bookmark was not deleted: %v", err)
	}
	var persisted models.Bookmark
	if err := server.db.First(&persisted, otherBookmark.ID).Error; err != nil {
		t.Fatalf("same-account other-book bookmark was deleted: %v", err)
	}
}

func TestBookmarkBatchWritesBroadcastOnceOnlyAfterDurableMutation(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := bookmarkContractUser(t, server)
	book, _ := bookmarkContractBook(t, server, user.ID, "书签事务广播书")
	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)
	path := "/api/books/" + strconv.FormatUint(uint64(book.ID), 10) + "/bookmarks"

	callbackName := "test:bookmark-batch-write-failure"
	if err := server.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "bookmarks" {
			tx.AddError(errors.New("injected bookmark write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed := bookmarkContractRequest(t, router, http.MethodPost, path+"/batch", `[
		{"chapterIndex":0,"title":"第一章","excerpt":"失败上下文一"},
		{"chapterIndex":0,"title":"第一章","excerpt":"失败上下文二"}
	]`, token)
	if err := server.db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("injected batch failure: %d %s", failed.Code, failed.Body.String())
	}
	if queuedPayloadContains(client.Send, `"type":"bookmarks_update"`) {
		t.Fatal("failed bookmark batch emitted a sync event")
	}
	var count int64
	if err := server.db.Model(&models.Bookmark{}).Where("book_id = ?", book.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed bookmark batch persisted %d rows", count)
	}

	created := bookmarkContractRequest(t, router, http.MethodPost, path+"/batch", `[
		{"chapterIndex":0,"title":"第一章","excerpt":"成功上下文一"},
		{"chapterIndex":0,"title":"第一章","excerpt":"成功上下文二"}
	]`, token)
	if created.Code != http.StatusCreated {
		t.Fatalf("successful bookmark batch: %d %s", created.Code, created.Body.String())
	}
	var createdRows []models.Bookmark
	if err := json.Unmarshal(created.Body.Bytes(), &createdRows); err != nil {
		t.Fatal(err)
	}
	assertSingleBookmarkEvent(t, client.Send, `"kind":"create_many"`)

	deleted := bookmarkContractRequest(t, router, http.MethodPost, path+"/batch-delete", `{"ids":[`+
		strconv.FormatUint(uint64(createdRows[0].ID), 10)+`,`+
		strconv.FormatUint(uint64(createdRows[1].ID), 10)+`]}`, token)
	if deleted.Code != http.StatusOK {
		t.Fatalf("successful bookmark batch delete: %d %s", deleted.Code, deleted.Body.String())
	}
	assertSingleBookmarkEvent(t, client.Send, `"kind":"delete_many"`)
}

func assertSingleBookmarkEvent(t *testing.T, channel <-chan []byte, needle string) {
	t.Helper()
	count := 0
	matched := false
	for {
		select {
		case payload := <-channel:
			count++
			if strings.Contains(string(payload), `"type":"bookmarks_update"`) && strings.Contains(string(payload), needle) {
				matched = true
			}
		default:
			if count != 1 || !matched {
				t.Fatalf("expected one matching bookmark event, count=%d matched=%t", count, matched)
			}
			return
		}
	}
}

func TestBookmarksKeepInsertionOrderAndImmutableReaderContextOnEdit(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := bookmarkContractUser(t, server)
	book, chapter := bookmarkContractBook(t, server, user.ID, "顺序书")
	first := models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Title: "第一章", Excerpt: "第一段"}
	second := models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Title: "第一章", Excerpt: "第二段"}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&second).Where("id = ?", second.ID).Update("updated_at", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	updated := bookmarkContractRequest(t, router, http.MethodPut, "/api/bookmarks/"+strconv.FormatUint(uint64(second.ID), 10), `{"note":"新备注"}`, token)
	if updated.Code != http.StatusOK {
		t.Fatalf("bookmark note update: %d: %s", updated.Code, updated.Body.String())
	}
	var result models.Bookmark
	if err := json.Unmarshal(updated.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Excerpt != "第二段" || result.Note != "新备注" || result.ChapterID != chapter.ID {
		t.Fatalf("editing note must not mutate reader context: %+v", result)
	}

	listed := bookmarkContractRequest(t, router, http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/bookmarks", "", token)
	if listed.Code != http.StatusOK {
		t.Fatalf("list bookmarks: %d: %s", listed.Code, listed.Body.String())
	}
	var bookmarks []models.Bookmark
	if err := json.Unmarshal(listed.Body.Bytes(), &bookmarks); err != nil {
		t.Fatal(err)
	}
	if len(bookmarks) != 2 || bookmarks[0].ID != first.ID || bookmarks[1].ID != second.ID {
		t.Fatalf("bookmark manager must retain stable creation order, got %+v", bookmarks)
	}
}

func TestBookmarkBackupAndRestoreKeepCreationOrderAndSameLocationRows(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	sourceUser := bookmarkContractUser(t, server)
	sourceBook, sourceChapter := bookmarkContractBook(t, server, sourceUser.ID, "书签备份顺序书")
	createdAt := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	first := models.Bookmark{
		UserID: sourceUser.ID, BookID: sourceBook.ID, ChapterID: sourceChapter.ID,
		ChapterIndex: 0, Offset: 18, Title: "同一位置", Excerpt: "第一条独立上下文",
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	second := models.Bookmark{
		UserID: sourceUser.ID, BookID: sourceBook.ID, ChapterID: sourceChapter.ID,
		ChapterIndex: 0, Offset: 18, Title: "同一位置", Excerpt: "第二条独立上下文",
		CreatedAt: createdAt.Add(time.Minute), UpdatedAt: createdAt.Add(time.Minute),
	}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&second).Where("id = ?", second.ID).Update("updated_at", createdAt.Add(24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	backupPath, err := server.backupSvc.RunNow()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var bookmarkFile *zip.File
	for _, file := range archive.File {
		if file.Name == "bookmarks.json" {
			bookmarkFile = file
			break
		}
	}
	if bookmarkFile == nil {
		t.Fatal("bookmarks.json not found in backup")
	}
	stream, err := bookmarkFile.Open()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	var exported []models.Bookmark
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported) != 2 || exported[0].ID != first.ID || exported[1].ID != second.ID {
		t.Fatalf("backup must retain creation order despite later edits, got %+v", exported)
	}

	targetUser := models.User{Username: "bookmark-restore-user", PasswordHash: "not-used"}
	if err := server.db.Create(&targetUser).Error; err != nil {
		t.Fatal(err)
	}
	targetBook := models.Book{UserID: targetUser.ID, Title: sourceBook.Title, URL: sourceBook.URL}
	if err := server.db.Create(&targetBook).Error; err != nil {
		t.Fatal(err)
	}
	targetChapter := models.Chapter{BookID: targetBook.ID, Index: 0, Title: "恢复后的第一章"}
	if err := server.db.Create(&targetChapter).Error; err != nil {
		t.Fatal(err)
	}

	if restored, err := server.restoreBookmarksFromZip(bookmarkFile, targetUser.ID); err != nil || restored != 2 {
		t.Fatalf("restore independent same-location bookmarks: restored=%d err=%v", restored, err)
	}
	// The same export is idempotent, but must not merge its two independent rows.
	if restored, err := server.restoreBookmarksFromZip(bookmarkFile, targetUser.ID); err != nil || restored != 2 {
		t.Fatalf("repeat restore should remain idempotent: restored=%d err=%v", restored, err)
	}
	var restored []models.Bookmark
	if err := server.db.Where("user_id = ? AND book_id = ?", targetUser.ID, targetBook.ID).Order("id asc").Find(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 || restored[0].Excerpt != first.Excerpt || restored[1].Excerpt != second.Excerpt {
		t.Fatalf("restore must retain both same-location rows in export order, got %+v", restored)
	}
	if restored[0].ChapterID != targetChapter.ID || restored[1].ChapterID != targetChapter.ID {
		t.Fatalf("restore must rebind the current book chapter IDs, got %+v", restored)
	}
}
