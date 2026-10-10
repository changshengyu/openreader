package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestLocalImportStageLaterCancelKeepsEarlierDurableBookAndEvent(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		t.Run(source, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			first, second := stageLifecycleToken(t, server), stageLifecycleToken(t, server)
			client := server.hub.AddClient(1, nil)
			t.Cleanup(func() { server.hub.RemoveClient(client) })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fired := false
			localImportStageLifecycleTestHook = func(phase, _, token, _ string) {
				if phase == "load-metadata" && token == second {
					fired = true
					cancel()
				}
			}
			t.Cleanup(func() { localImportStageLifecycleTestHook = nil })
			body := `{"items":[{"path":"first.txt","importToken":"` + first + `"},{"path":"second.txt","importToken":"` + second + `"}]}`
			request := httptest.NewRequest(http.MethodPost, "/api/"+source+"/import", strings.NewReader(body)).WithContext(ctx)
			request.Header.Set("Authorization", auth)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !fired || response.Code != 500 || !strings.Contains(response.Body.String(), "local import stage canceled") {
				t.Fatalf("later cancel: %v %d %s", fired, response.Code, response.Body.String())
			}
			var books []models.Book
			if err := server.db.Find(&books).Error; err != nil || len(books) != 1 {
				t.Fatalf("earlier durable book lost: %+v %v", books, err)
			}
			select {
			case message := <-client.Send:
				var event struct {
					Type    string
					Payload []struct{ ID uint }
				}
				if err := json.Unmarshal(message, &event); err != nil {
					t.Fatal(err)
				}
				if event.Type != "bookshelf_update" || len(event.Payload) != 1 || event.Payload[0].ID != books[0].ID {
					t.Fatalf("durable-only event: %s", message)
				}
			default:
				t.Fatal("earlier durable notification swallowed")
			}
			if events := drainBookWriteEvents(client.Send); len(events) != 0 {
				t.Fatalf("failed-item extra events: %v", events)
			}
			if _, _, err := server.loadStagedLocalImport(1, second); err != nil {
				t.Fatalf("failed token no longer retryable: %v", err)
			}
		})
	}
}

func TestLocalImportStagePublicationFailureKeepsExistingWireError(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		t.Run(route.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			token := stageLifecycleToken(t, server)
			var newcomer string
			fired := stageLifecycleHook(t, "prepared-ready", func(dir, token, _ string) {
				newcomer = filepath.Join(dir, token+".parsed.json")
				stageLifecycleWrite(t, newcomer, []byte("unknown-publication-target"))
			})
			request, response := stageLifecycleRequest(t, server, route, auth, token)
			router.ServeHTTP(response, request)
			wantStatus, wantError := 200, "failed to stage parsed import"
			if route.direct {
				wantStatus = 500
				if !route.preview {
					wantError = "failed to import book"
				}
			}
			if !*fired || response.Code != wantStatus || !strings.Contains(response.Body.String(), wantError) {
				t.Fatalf("write rejected as wrong wire error: fired=%v %d %s", *fired, response.Code, response.Body.String())
			}
			stageLifecycleAssertBytes(t, newcomer, []byte("unknown-publication-target"))
			stageLifecycleNoBooks(t, server)
			if strings.Contains(response.Body.String(), server.cfg.CacheDir) {
				t.Fatal("host path leaked")
			}
		})
	}
}

func TestLocalImportStageCancelAfterDurableStillSucceedsAndNotifies(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		if route.preview {
			continue
		}
		t.Run(route.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			token := stageLifecycleToken(t, server)
			client := server.hub.AddClient(1, nil)
			t.Cleanup(func() { server.hub.RemoveClient(client) })
			request, response := stageLifecycleRequest(t, server, route, auth, token)
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			fired := stageLifecycleHook(t, "consume-admitted", func(_, _, _ string) { cancel() })
			router.ServeHTTP(response, request.WithContext(ctx))
			want := 200
			if route.direct {
				want = 201
			}
			if !*fired || response.Code != want {
				t.Fatalf("post-durable cancellation changed success: %v %d %s", *fired, response.Code, response.Body.String())
			}
			var count int64
			if err := server.db.Model(&models.Book{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("durable Book not retained: %d %v", count, err)
			}
			if events := drainBookWriteEvents(client.Send); len(events) != 1 || !strings.Contains(string(events[0]), "bookshelf_update") {
				t.Fatalf("durable event missing: %v", events)
			}
			if _, _, err := server.loadStagedLocalImport(1, token); err != nil {
				t.Fatalf("cancelled cleanup altered residual bundle: %v", err)
			}
		})
	}
}
