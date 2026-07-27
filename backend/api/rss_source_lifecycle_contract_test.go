package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestRSSSourceRequiresNameWithoutMutatingExistingData(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	createReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"   ","url":"https://rss.example/blank-name.xml"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", token)
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusBadRequest || !strings.Contains(createW.Body.String(), `"error":"title is required"`) {
		t.Fatalf("blank RSS source title: expected 400 title error, got %d: %s", createW.Code, createW.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.RSSSource{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("blank RSS source title must not create a row, count=%d", count)
	}

	validReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"保留名称","url":"https://rss.example/valid.xml"}`))
	validReq.Header.Set("Content-Type", "application/json")
	validReq.Header.Set("Authorization", token)
	validW := httptest.NewRecorder()
	router.ServeHTTP(validW, validReq)
	if validW.Code != http.StatusCreated {
		t.Fatalf("create valid RSS source: got %d: %s", validW.Code, validW.Body.String())
	}
	var source models.RSSSource
	if err := json.Unmarshal(validW.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10), strings.NewReader(`{"title":"","url":"https://rss.example/changed.xml"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", token)
	updateW := httptest.NewRecorder()
	router.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusBadRequest || !strings.Contains(updateW.Body.String(), `"error":"title is required"`) {
		t.Fatalf("blank RSS update title: expected 400 title error, got %d: %s", updateW.Code, updateW.Body.String())
	}
	var persisted models.RSSSource
	if err := server.db.First(&persisted, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Title != "保留名称" || persisted.URL != "https://rss.example/valid.xml" {
		t.Fatalf("rejected update mutated RSS source: %+v", persisted)
	}
}

func TestCreateRSSSourceReplacesSameUserURLAndPreservesUserIsolation(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	other := models.User{Username: "rss-other", PasswordHash: "hash"}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherSource := models.RSSSource{
		UserID: other.ID,
		Title:  "其他用户",
		URL:    "https://rss.example/shared.xml",
	}
	if err := server.db.Create(&otherSource).Error; err != nil {
		t.Fatal(err)
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"第一版","url":"https://rss.example/shared.xml","singleUrl":false}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", token)
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, firstReq)
	if firstW.Code != http.StatusCreated {
		t.Fatalf("first same-URL source: got %d: %s", firstW.Code, firstW.Body.String())
	}
	var first models.RSSSource
	if err := json.Unmarshal(firstW.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/rss/sources", strings.NewReader(`{"title":"最终配置","url":" https://rss.example/shared.xml ","singleUrl":true}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", token)
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, secondReq)
	if secondW.Code != http.StatusOK {
		t.Fatalf("same-user same-URL save: expected 200 update, got %d: %s", secondW.Code, secondW.Body.String())
	}
	var replaced models.RSSSource
	if err := json.Unmarshal(secondW.Body.Bytes(), &replaced); err != nil {
		t.Fatal(err)
	}
	if replaced.ID != first.ID || replaced.Title != "最终配置" || !replaced.SingleURL {
		t.Fatalf("same-URL source was not replaced in place: first=%+v replaced=%+v", first, replaced)
	}
	var ownerCount int64
	if err := server.db.Model(&models.RSSSource{}).Where("user_id = ? AND url = ?", owner.ID, "https://rss.example/shared.xml").Count(&ownerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ownerCount != 1 {
		t.Fatalf("same user should own one normalized URL row, count=%d", ownerCount)
	}
	var otherPersisted models.RSSSource
	if err := server.db.First(&otherPersisted, otherSource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if otherPersisted.Title != "其他用户" {
		t.Fatalf("same URL save crossed user boundary: %+v", otherPersisted)
	}
}

func TestUpdateRSSSourceRejectsSameUserURLCollision(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	first := models.RSSSource{UserID: user.ID, Title: "源一", URL: "https://rss.example/one.xml"}
	second := models.RSSSource{UserID: user.ID, Title: "源二", URL: "https://rss.example/two.xml"}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/rss/sources/"+strconv.FormatUint(uint64(second.ID), 10), strings.NewReader(`{"title":"冲突","url":"https://rss.example/one.xml"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"error":"RSS source URL already exists"`) {
		t.Fatalf("RSS source URL collision: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var persisted models.RSSSource
	if err := server.db.First(&persisted, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Title != "源二" || persisted.URL != "https://rss.example/two.xml" {
		t.Fatalf("collision mutated source: %+v", persisted)
	}
}

func TestDeleteRSSSourceRollsBackArticlesWhenSourceDeleteFails(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.RSSSource{UserID: user.ID, Title: "不可删源", URL: "https://rss.example/blocked.xml"}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	article := models.RSSArticle{UserID: user.ID, SourceID: source.ID, Title: "必须回滚"}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE TRIGGER block_rss_source_delete BEFORE DELETE ON rss_sources BEGIN SELECT RAISE(ABORT, 'blocked'); END`
	if err := server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/rss/sources/"+strconv.FormatUint(uint64(source.ID), 10), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("forced RSS source delete failure: expected 500, got %d: %s", w.Code, w.Body.String())
	}
	var articleCount int64
	if err := server.db.Model(&models.RSSArticle{}).Where("id = ?", article.ID).Count(&articleCount).Error; err != nil {
		t.Fatal(err)
	}
	if articleCount != 1 {
		t.Fatalf("RSS article deletion was not rolled back, count=%d", articleCount)
	}
}

func TestDeleteMissingRSSSourceDoesNotDeleteOrphanArticle(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	const missingSourceID = 987654
	article := models.RSSArticle{UserID: user.ID, SourceID: missingSourceID, Title: "孤立缓存"}
	if err := server.db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/rss/sources/"+strconv.Itoa(missingSourceID), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing RSS source delete: expected 404, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.RSSArticle{}).Where("id = ?", article.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("missing source delete removed orphan article, count=%d", count)
	}
}
