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

func TestBookMetadataPatchPreservesConcurrentShelfFields(t *testing.T) {
	router, server := setupTestServer(t)
	ownerToken := registerLifecycleToken(t, router, "bookeditowner")
	otherToken := registerLifecycleToken(t, router, "bookeditother")
	owner := lifecycleUser(t, server, "bookeditowner")
	category := models.Category{UserID: owner.ID, Name: "并发新分组", Show: true}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:    owner.ID,
		Title:     "编辑前书名",
		Author:    "旧作者",
		Intro:     "旧简介",
		CanUpdate: false,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&book).UpdateColumn("can_update", false).Error; err != nil {
		t.Fatal(err)
	}
	book.CanUpdate = false
	if err := server.db.Create(&models.BookCategory{
		UserID: owner.ID, BookID: book.ID, CategoryID: category.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	path := "/api/books/" + strconv.FormatUint(uint64(book.ID), 10)
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(
		`{"title":"编辑后书名","author":"新作者","customCoverUrl":"","intro":"新简介"}`,
	))
	req.Header.Set("Authorization", ownerToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metadata patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Title       string `json:"title"`
		Author      string `json:"author"`
		Intro       string `json:"intro"`
		CanUpdate   bool   `json:"canUpdate"`
		CategoryIDs []uint `json:"categoryIds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Title != "编辑后书名" || response.Author != "新作者" || response.Intro != "新简介" {
		t.Fatalf("metadata response did not contain confirmed values: %+v", response)
	}
	if response.CanUpdate || len(response.CategoryIDs) != 1 || response.CategoryIDs[0] != category.ID {
		t.Fatalf("metadata patch overwrote concurrent shelf fields: %+v", response)
	}

	var stored models.Book
	if err := server.db.First(&stored, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CanUpdate || stored.Title != "编辑后书名" || stored.Author != "新作者" || stored.Intro != "新简介" {
		t.Fatalf("unexpected stored metadata after precise patch: %+v", stored)
	}
	var relationCount int64
	if err := server.db.Model(&models.BookCategory{}).
		Where("user_id = ? AND book_id = ? AND category_id = ?", owner.ID, book.ID, category.ID).
		Count(&relationCount).Error; err != nil {
		t.Fatal(err)
	}
	if relationCount != 1 {
		t.Fatalf("metadata patch changed category relations, count=%d", relationCount)
	}

	foreignReq := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"title":"越权"}`))
	foreignReq.Header.Set("Authorization", otherToken)
	foreignReq.Header.Set("Content-Type", "application/json")
	foreignW := httptest.NewRecorder()
	router.ServeHTTP(foreignW, foreignReq)
	if foreignW.Code != http.StatusNotFound {
		t.Fatalf("foreign metadata patch: expected 404, got %d: %s", foreignW.Code, foreignW.Body.String())
	}

	emptyReq := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"title":"   "}`))
	emptyReq.Header.Set("Authorization", ownerToken)
	emptyReq.Header.Set("Content-Type", "application/json")
	emptyW := httptest.NewRecorder()
	router.ServeHTTP(emptyW, emptyReq)
	if emptyW.Code != http.StatusBadRequest {
		t.Fatalf("empty title patch: expected 400, got %d: %s", emptyW.Code, emptyW.Body.String())
	}
	if err := server.db.First(&stored, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "编辑后书名" {
		t.Fatalf("failed metadata patch changed persisted title to %q", stored.Title)
	}
}
