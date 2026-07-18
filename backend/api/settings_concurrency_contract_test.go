package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestConcurrentInitialUserSettingWritesAreAtomic(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	const writers = 8
	firstQueriesDone := make(chan struct{})
	createQueriesDone := make(chan struct{})
	var queryCount atomic.Int32
	if err := server.db.Callback().Query().After("gorm:query").Register("test:user-setting-create-race", func(tx *gorm.DB) {
		if tx.Statement.Table != "user_settings" {
			return
		}
		position := int(queryCount.Add(1))
		switch {
		case position <= writers:
			if position == writers {
				close(firstQueriesDone)
			}
			<-firstQueriesDone
		case position <= writers*2:
			if position == writers*2 {
				close(createQueriesDone)
			}
			<-createQueriesDone
		}
	}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			body := fmt.Sprintf(`{"value":{"marker":%d},"baseUpdatedAt":""}`, index)
			req := httptest.NewRequest(http.MethodPut, "/api/settings/search", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			results <- response
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	for response := range results {
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent initial setting write: expected 200, got %d: %s", response.Code, response.Body.String())
		}
		var payload struct {
			Key   string         `json:"key"`
			Value map[string]any `json:"value"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode concurrent setting response: %v", err)
		}
		if payload.Key != "search" || payload.Value["marker"] == nil {
			t.Fatalf("unexpected concurrent setting response: %+v", payload)
		}
	}

	var rows []models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", 1, "search").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one unique user setting row, got %d", len(rows))
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(rows[0].Value), &value); err != nil {
		t.Fatalf("decode persisted setting: %v", err)
	}
	marker, ok := value["marker"].(float64)
	if !ok || marker < 0 || marker >= writers {
		t.Fatalf("persisted setting is not one complete concurrent request: %+v", value)
	}
}
