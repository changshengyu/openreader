package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
