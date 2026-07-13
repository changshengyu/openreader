package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestInvalidSourceCacheIsCallerScopedSuppressesRetryAndExpires(t *testing.T) {
	router, server := setupTestServer(t)
	tokenA := registerSourceFailureUser(t, router, "failure-user-a")
	tokenB := registerSourceFailureUser(t, router, "failure-user-b")

	source := sourceFailureTestSource(t, "失败隔离源", "https://failure-scope.example")
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int32
	fail := true
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			if fail {
				return nil, errors.New("upstream https://user:secret@failure-scope.example/search?token=private failed")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<div class="book"><span class="name">恢复书</span></div>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	callSourceFailureSearch(t, router, tokenA, source.ID)
	if requests.Load() != 1 {
		t.Fatalf("first source request count = %d, want 1", requests.Load())
	}

	rowsA := getInvalidSourceRows(t, router, tokenA)
	if len(rowsA) != 1 || uintValue(rowsA[0]["id"]) != source.ID {
		t.Fatalf("caller A failures = %#v, want current source", rowsA)
	}
	message, _ := rowsA[0]["errorMessage"].(string)
	if message != "请求书源失败" || strings.Contains(message, "secret") || strings.Contains(message, "token") {
		t.Fatalf("failure message must be client-safe, got %q", message)
	}
	if rowsB := getInvalidSourceRows(t, router, tokenB); len(rowsB) != 0 {
		t.Fatalf("caller B must not see caller A failure: %#v", rowsB)
	}

	callSourceFailureSearch(t, router, tokenA, source.ID)
	if requests.Load() != 1 {
		t.Fatalf("active failure must suppress normal retry, got %d requests", requests.Load())
	}

	var failure models.SourceFailure
	if err := server.db.Where("source_id = ?", source.ID).First(&failure).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&failure).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if rows := getInvalidSourceRows(t, router, tokenA); len(rows) != 0 {
		t.Fatalf("expired failure must not be returned: %#v", rows)
	}

	fail = false
	callSourceFailureSearch(t, router, tokenA, source.ID)
	if requests.Load() != 2 {
		t.Fatalf("expired failure must allow a new request, got %d requests", requests.Load())
	}
}

func TestInvalidSourceCacheIgnoresCanceledRequestsAndStaleSourceURL(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerSourceFailureUser(t, router, "failure-cancel-user")
	source := sourceFailureTestSource(t, "取消源", "https://cancel-failure.example")
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return nil, request.Context().Err()
		}),
	})
	defer restoreHTTPClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"keyword":"取消","sourceIds":[`+uintString(source.ID)+`]}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)

	var count int64
	if err := server.db.Model(&models.SourceFailure{}).Where("source_id = ?", source.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("canceled request must not create a failure cache row, got %d", count)
	}

	userID := sourceFailureUserID(t, server, "failure-cancel-user")
	stale := models.SourceFailure{
		UserID:    userID,
		SourceID:  source.ID,
		SourceURL: source.BaseURL,
		Message:   "请求书源失败",
		FailedAt:  time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := server.db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&source).Update("base_url", "https://edited-source.example").Error; err != nil {
		t.Fatal(err)
	}
	if rows := getInvalidSourceRows(t, router, token); len(rows) != 0 {
		t.Fatalf("source edit must hide stale cached failure: %#v", rows)
	}
}

func TestInvalidSourceCacheIncludesExplicitHealthFailuresOnly(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerSourceFailureUser(t, router, "failure-health-user")
	source := models.BookSource{Name: "无搜索地址", BaseURL: "https://health-failure.example", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sources/batch-test", strings.NewReader(`{"keyword":"测试","sourceIds":[`+uintString(source.ID)+`]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("health check: expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	if rows := getInvalidSourceRows(t, router, token); len(rows) != 1 || uintValue(rows[0]["id"]) != source.ID {
		t.Fatalf("explicit health failure must be cached for the same user: %#v", rows)
	}
}

func sourceFailureTestSource(t *testing.T, name, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: name, BaseURL: baseURL, Enabled: true, Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    baseURL + "/search?q={keyword}",
		BookListRule: ".book",
		BookNameRule: ".name|text",
	}); err != nil {
		t.Fatal(err)
	}
	return source
}

func registerSourceFailureUser(t *testing.T, router http.Handler, username string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+username+`","password":"test1234"}`))
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("register %s: expected 200, got %d: %s", username, writer.Code, writer.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil || response.Token == "" {
		t.Fatalf("register %s token: %v, %s", username, err, writer.Body.String())
	}
	return "Bearer " + response.Token
}

func callSourceFailureSearch(t *testing.T, router http.Handler, token string, sourceID uint) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(`{"keyword":"失败","sourceIds":[`+uintString(sourceID)+`]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("source search: expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
}

func getInvalidSourceRows(t *testing.T, router http.Handler, token string) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sources/invalid", nil)
	req.Header.Set("Authorization", token)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusOK {
		t.Fatalf("list invalid sources: expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(writer.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func sourceFailureUserID(t *testing.T, server *Server, username string) uint {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

func uintValue(value any) uint {
	if number, ok := value.(float64); ok && number > 0 {
		return uint(number)
	}
	return 0
}
