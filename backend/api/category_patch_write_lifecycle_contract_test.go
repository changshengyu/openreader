package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
	"openreader/backend/services/bookgroups"
)

func TestCategoryPatchMergesConcurrentReorderAndPartialUpdate(t *testing.T) {
	fixture := newCategoryPatchWriteLifecycleFixture(t, "categorypatchmerge")
	blocker := installCategoryPatchWriteLifecycleBlocker(t)

	response, done := startCategoryPatchWriteLifecycleRequest(
		fixture,
		`{"name":"renamed category"}`,
		context.Background(),
	)
	blocker.wait(t, "Category patch did not reach the pre-persistence barrier")

	reorder := performCategoryPatchWriteLifecycleJSON(
		fixture,
		http.MethodPut,
		"/api/categories/reorder",
		fmt.Sprintf(`{"ids":[%d,%d]}`, fixture.other.ID, fixture.category.ID),
	)
	assertCategoryPatchWriteLifecycleStatus(t, reorder, http.StatusOK)
	partial := performCategoryPatchWriteLifecycleJSON(
		fixture,
		http.MethodPut,
		fixture.categoryPath(),
		`{"color":"#445566","show":false}`,
	)
	assertCategoryPatchWriteLifecycleStatus(t, partial, http.StatusOK)
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 4 {
		t.Fatalf("concurrent Category writes emitted %d events, want 4: %v", len(events), events)
	}

	blocker.unblock()
	waitCategoryPatchWriteLifecycleHandler(t, done)
	assertCategoryPatchWriteLifecycleStatus(t, response, http.StatusOK)

	category := loadCategoryPatchWriteLifecycleCategory(t, fixture)
	assertCategoryPatchWriteLifecycleCategory(t, category, "renamed category", "#445566", false, 20)
	assertCategoryPatchWriteLifecycleResponse(t, response.Body.Bytes(), "renamed category", "#445566", false, 20)
	assertCategoryPatchWriteLifecycleEvents(t, fixture, "renamed category", "#445566", false, 20)
}

func TestCategoryPatchDoesNotResurrectConcurrentDelete(t *testing.T) {
	fixture := newCategoryPatchWriteLifecycleFixture(t, "categorypatchdelete")
	blocker := installCategoryPatchWriteLifecycleBlocker(t)

	response, done := startCategoryPatchWriteLifecycleRequest(
		fixture,
		`{"name":"late category"}`,
		context.Background(),
	)
	blocker.wait(t, "Category patch did not reach the pre-persistence barrier")

	deleteRequest := httptest.NewRequest(http.MethodDelete, fixture.categoryPath(), nil)
	deleteRequest.Header.Set("Authorization", fixture.auth)
	deleteResponse := httptest.NewRecorder()
	fixture.router.ServeHTTP(deleteResponse, deleteRequest)
	assertCategoryPatchWriteLifecycleStatus(t, deleteResponse, http.StatusNoContent)
	drainBookGroupWriteEvents(fixture.events)

	blocker.unblock()
	waitCategoryPatchWriteLifecycleHandler(t, done)
	if response.Code != http.StatusNotFound || response.Body.String() != `{"error":"category not found"}` {
		t.Errorf("late Category patch = %d %s, want stable flat 404", response.Code, response.Body.String())
	}
	var count int64
	if err := fixture.server.db.Model(&models.Category{}).Where("id = ?", fixture.category.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("late Category patch resurrected deleted target, count=%d", count)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("late Category patch broadcast after delete: %v", events)
	}
}

func TestCategoryPatchHonorsCancellationBeforePersistence(t *testing.T) {
	fixture := newCategoryPatchWriteLifecycleFixture(t, "categorypatchcancel")
	before := snapshotCategoryPatchWriteLifecycleState(t, fixture)
	blocker := installCategoryPatchWriteLifecycleBlocker(t)

	ctx, cancel := context.WithCancel(context.Background())
	response, done := startCategoryPatchWriteLifecycleRequest(fixture, `{"show":false}`, ctx)
	blocker.wait(t, "Category patch did not reach the pre-persistence barrier")
	cancel()
	blocker.unblock()
	waitCategoryPatchWriteLifecycleHandler(t, done)

	if response.Body.Len() != 0 {
		t.Errorf("cancelled Category patch wrote a business response: %d %s", response.Code, response.Body.String())
	}
	after := snapshotCategoryPatchWriteLifecycleState(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("cancelled Category patch changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("cancelled Category patch broadcast events: %v", events)
	}
}

func TestEmptyCategoryPatchesDoNotAdvanceUpdatedAt(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "empty", body: `{}`},
		{name: "null-and-unknown", body: `{"name":null,"color":null,"show":null,"unknown":"ignored"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCategoryPatchWriteLifecycleFixture(t, "categorypatchempty"+strings.ReplaceAll(test.name, "-", ""))
			fixed := time.Date(2024, time.February, 3, 4, 5, 6, 0, time.UTC)
			if err := fixture.server.db.Model(&models.Category{}).
				Where("id = ?", fixture.category.ID).UpdateColumn("updated_at", fixed).Error; err != nil {
				t.Fatal(err)
			}
			before := loadCategoryPatchWriteLifecycleCategory(t, fixture)

			response := performCategoryPatchWriteLifecycleJSON(
				fixture,
				http.MethodPut,
				fixture.categoryPath(),
				test.body,
			)
			assertCategoryPatchWriteLifecycleStatus(t, response, http.StatusOK)
			after := loadCategoryPatchWriteLifecycleCategory(t, fixture)
			if !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Errorf("%s Category patch advanced updated_at: before=%s after=%s", test.name, before.UpdatedAt, after.UpdatedAt)
			}
		})
	}
}

type categoryPatchWriteLifecycleFixture struct {
	router   *gin.Engine
	server   *Server
	auth     string
	owner    models.User
	category models.Category
	other    models.Category
	events   <-chan []byte
}

func newCategoryPatchWriteLifecycleFixture(t *testing.T, username string) categoryPatchWriteLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	category := models.Category{
		UserID: owner.ID, Name: "initial category", Color: "#112233", Show: true, SortOrder: 10,
	}
	other := models.Category{
		UserID: owner.ID, Name: "other category", Color: "#778899", Show: true, SortOrder: 20,
	}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	events := server.hub.AddClient(owner.ID, nil).Send
	return categoryPatchWriteLifecycleFixture{
		router: router, server: server, auth: auth, owner: owner,
		category: category, other: other, events: events,
	}
}

func (fixture categoryPatchWriteLifecycleFixture) categoryPath() string {
	return "/api/categories/" + strconv.FormatUint(uint64(fixture.category.ID), 10)
}

type categoryPatchWriteLifecycleBlocker struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func installCategoryPatchWriteLifecycleBlocker(t *testing.T) *categoryPatchWriteLifecycleBlocker {
	t.Helper()
	blocker := &categoryPatchWriteLifecycleBlocker{started: make(chan struct{}), release: make(chan struct{})}
	categoryPatchWriteLifecycleTestHook = func() {
		categoryPatchWriteLifecycleTestHook = nil
		blocker.startedOnce.Do(func() { close(blocker.started) })
		<-blocker.release
	}
	t.Cleanup(func() {
		blocker.unblock()
		categoryPatchWriteLifecycleTestHook = nil
	})
	return blocker
}

func (blocker *categoryPatchWriteLifecycleBlocker) wait(t *testing.T, message string) {
	t.Helper()
	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		blocker.unblock()
		t.Fatal(message)
	}
}

func (blocker *categoryPatchWriteLifecycleBlocker) unblock() {
	blocker.releaseOnce.Do(func() { close(blocker.release) })
}

func startCategoryPatchWriteLifecycleRequest(
	fixture categoryPatchWriteLifecycleFixture,
	body string,
	ctx context.Context,
) (*httptest.ResponseRecorder, <-chan struct{}) {
	request := httptest.NewRequest(http.MethodPut, fixture.categoryPath(), strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		fixture.router.ServeHTTP(response, request)
		close(done)
	}()
	return response, done
}

func performCategoryPatchWriteLifecycleJSON(
	fixture categoryPatchWriteLifecycleFixture,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func waitCategoryPatchWriteLifecycleHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Category patch handler did not finish")
	}
}

func assertCategoryPatchWriteLifecycleStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("Category patch status = %d, want %d: %s", response.Code, want, response.Body.String())
	}
}

func loadCategoryPatchWriteLifecycleCategory(t *testing.T, fixture categoryPatchWriteLifecycleFixture) models.Category {
	t.Helper()
	var category models.Category
	if err := fixture.server.db.First(&category, fixture.category.ID).Error; err != nil {
		t.Fatal(err)
	}
	return category
}

func snapshotCategoryPatchWriteLifecycleState(t *testing.T, fixture categoryPatchWriteLifecycleFixture) []byte {
	t.Helper()
	category := loadCategoryPatchWriteLifecycleCategory(t, fixture)
	payload, err := json.Marshal(category)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertCategoryPatchWriteLifecycleCategory(
	t *testing.T,
	category models.Category,
	name string,
	color string,
	show bool,
	sortOrder int,
) {
	t.Helper()
	if category.Name != name || category.Color != color || category.Show != show || category.SortOrder != sortOrder {
		t.Errorf("Category projection lost a concurrent column write: %+v", category)
	}
}

func assertCategoryPatchWriteLifecycleResponse(
	t *testing.T,
	payload []byte,
	name string,
	color string,
	show bool,
	sortOrder int,
) {
	t.Helper()
	var category models.Category
	if err := json.Unmarshal(payload, &category); err != nil {
		t.Fatalf("decode Category response: %v: %s", err, payload)
	}
	assertCategoryPatchWriteLifecycleCategory(t, category, name, color, show, sortOrder)
}

func assertCategoryPatchWriteLifecycleEvents(
	t *testing.T,
	fixture categoryPatchWriteLifecycleFixture,
	name string,
	color string,
	show bool,
	sortOrder int,
) {
	t.Helper()
	emitted := drainBookGroupWriteEvents(fixture.events)
	if len(emitted) != 2 {
		t.Fatalf("Category patch emitted %d events, want 2: %v", len(emitted), emitted)
	}
	seenCategory := false
	seenBookGroups := false
	for _, raw := range emitted {
		var event struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatalf("decode Category event: %v: %s", err, raw)
		}
		switch event.Type {
		case "category_update":
			var category models.Category
			if err := json.Unmarshal(event.Payload, &category); err != nil {
				t.Fatalf("decode category_update payload: %v: %s", err, event.Payload)
			}
			assertCategoryPatchWriteLifecycleCategory(t, category, name, color, show, sortOrder)
			seenCategory = true
		case "book_groups_update":
			var rows []bookgroups.Row
			if err := json.Unmarshal(event.Payload, &rows); err != nil {
				t.Fatalf("decode book_groups_update payload: %v: %s", err, event.Payload)
			}
			for _, row := range rows {
				if row.CategoryID != nil && *row.CategoryID == fixture.category.ID {
					if row.Name != name || row.Show != show || row.SortOrder != sortOrder {
						t.Errorf("BookGroup event lost a current Category field: %+v", row)
					}
					seenBookGroups = true
				}
			}
		}
	}
	if !seenCategory || !seenBookGroups {
		t.Errorf("Category patch events missing authoritative payloads: category=%t bookGroups=%t", seenCategory, seenBookGroups)
	}
}
