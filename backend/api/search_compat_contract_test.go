package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestSearchUsesUpstreamWorkspaceConcurrencyDefault(t *testing.T) {
	if got := normalizedConcurrentCount(0, 100); got != 24 {
		t.Fatalf("default concurrent count = %d, want upstream workspace default 24", got)
	}
	if got := normalizedConcurrentCount(-1, 100); got != 24 {
		t.Fatalf("negative concurrent count = %d, want upstream workspace default 24", got)
	}
	if got := normalizedConcurrentCount(60, 3); got != 3 {
		t.Fatalf("positive concurrent count should remain bounded by source count, got %d", got)
	}
}

func TestSearchReportsNoConfiguredSourceInsteadOfEmptySuccess(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"keyword":"测试","page":1,"lastIndex":-1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("no-source search status = %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "未配置书源" {
		t.Fatalf("no-source search error = %q, want 未配置书源", body.Error)
	}
}

func TestSearchMultiCursorKeepsOriginalSourceOrdinalsAcrossFailureSuppression(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerSourceFailureUser(t, router, "searchcursorfailureuser")
	userID := sourceFailureUserID(t, server, "searchcursorfailureuser")

	sources := []models.BookSource{
		sourceFailureTestSource(t, "游标源一", "https://cursor-one.example"),
		sourceFailureTestSource(t, "游标源二", "https://cursor-two.example"),
		sourceFailureTestSource(t, "游标源三", "https://cursor-three.example"),
	}
	for index := range sources {
		sources[index].CustomOrder = index + 1
		if err := server.db.Create(&sources[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<div class="book"><span class="name">` + request.URL.Host + `</span></div>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	first := searchMultiCursorPage(t, router, token, sources, -1)
	if first.LastIndex != 0 || !first.HasMore {
		t.Fatalf("first multi cursor = %#v, want lastIndex 0 with more sources", first)
	}

	if err := server.db.Create(&models.SourceFailure{
		UserID:    userID,
		SourceID:  sources[1].ID,
		SourceURL: sources[1].BaseURL,
		Message:   "请求书源失败",
		FailedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}

	second := searchMultiCursorPage(t, router, token, sources, first.LastIndex)
	if second.LastIndex != 2 || second.HasMore {
		t.Fatalf("failure-suppressed source must not renumber the cursor: %#v", second)
	}
	if len(second.List) != 1 || second.List[0].Title != "cursor-three.example" {
		t.Fatalf("second multi cursor result = %#v, want only the third source", second.List)
	}
}

func searchMultiCursorPage(t *testing.T, router http.Handler, token string, sources []models.BookSource, lastIndex int) searchResponse {
	t.Helper()
	ids := make([]string, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, fmt.Sprintf("%d", source.ID))
	}
	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(fmt.Sprintf(
		`{"keyword":"游标","sourceIds":[%s],"concurrentCount":1,"lastIndex":%d,"searchSize":1}`,
		strings.Join(ids, ","), lastIndex,
	)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("multi cursor search: expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	var response searchResponse
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}
