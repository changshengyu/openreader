package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalImportStageDeepActualReadCancellation(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		phases := []string{"load-raw-read"}
		if !route.preview {
			phases = append(phases, "prepared-read")
		}
		for _, phase := range phases {
			t.Run(route.name+"/"+phase, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				token := stageLifecycleToken(t, server)
				if phase == "prepared-read" {
					_, prepared := stageLifecyclePrepared()
					if err := server.saveStagedPreparedImport(1, token, prepared); err != nil {
						t.Fatal(err)
					}
				}
				request, response := stageLifecycleRequest(t, server, route, auth, token)
				ctx, cancel := context.WithCancel(request.Context())
				defer cancel()
				fired := stageLifecycleHook(t, phase, func(_, _, _ string) { cancel() })
				router.ServeHTTP(response, request.WithContext(ctx))
				if !*fired {
					t.Fatal("actual opened-file Read fixture did not fire")
				}
				if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "local import stage canceled") {
					t.Errorf("read cancellation accepted: %d %s", response.Code, response.Body.String())
				}
				stageLifecycleNoBooks(t, server)
			})
		}
	}
}

func TestLocalImportStageDeepRawToParsedRetainsSession(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		for _, kind := range []string{"user", "prepared"} {
			if route.preview && kind == "prepared" {
				continue
			}
			t.Run(route.name+"/"+kind, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				token := stageLifecycleToken(t, server)
				_, prepared := stageLifecyclePrepared()
				if err := server.saveStagedPreparedImport(1, token, prepared); err != nil {
					t.Fatal(err)
				}
				dir := server.localImportStageDir(1)
				dataPath, metaPath := localImportStagePaths(dir, token)
				meta, err := os.ReadFile(metaPath)
				if err != nil {
					t.Fatal(err)
				}
				prepared.Book.Title = "Foreign跨阶段书"
				prepared.Book.Chapters[0].Content = "foreign-prepared-secret"
				encoded, err := json.Marshal(prepared)
				if err != nil {
					t.Fatal(err)
				}
				parsedPath := localImportPreparedStagePath(dir, token)
				fired := stageLifecycleHook(t, "raw-loaded", func(_, _, _ string) {
					if kind == "user" {
						if err := os.Rename(dir, dir+"-held"); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(dir, 0o700); err != nil {
							t.Fatal(err)
						}
						stageLifecycleWrite(t, metaPath, meta)
						stageLifecycleWrite(t, dataPath, []byte("第一章 原章\noriginal-owned-bytes"))
					} else if err := os.Rename(parsedPath, parsedPath+"-held"); err != nil {
						t.Fatal(err)
					}
					stageLifecycleWrite(t, parsedPath, encoded)
				})
				request, response := stageLifecycleRequest(t, server, route, auth, token)
				router.ServeHTTP(response, request)
				if !*fired {
					t.Fatal("raw-to-parsed fixture did not fire")
				}
				if !strings.Contains(response.Body.String(), "invalid or expired local import token") {
					t.Errorf("independent later namespace accepted: %d %s", response.Code, response.Body.String())
				}
				stageLifecycleAssertBytes(t, parsedPath, encoded)
				stageLifecycleNoBooks(t, server)
			})
		}
	}
}

func TestLocalImportStageDeepPartialWriteOwnsRollback(t *testing.T) {
	_, server := setupTestServer(t)
	var changed string
	fired := stageLifecycleHook(t, "create-data-written", func(dir, token, path string) {
		if err := os.Rename(path, path+"-held"); err != nil {
			t.Fatal(err)
		}
		stageLifecycleWrite(t, path, []byte("unknown-newcomer"))
		changed = path
		// Force the next legacy metadata write to fail after bytes exist.
		if err := os.Mkdir(filepath.Join(dir, token+".json"), 0o700); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := server.stageLocalImport(1, "original.txt", ".txt", []byte("original-owned-bytes")); err == nil {
		t.Error("partial stage unexpectedly succeeded")
	}
	if !*fired {
		t.Fatal("partial-stage rollback fixture did not fire")
	}
	stageLifecycleAssertBytes(t, changed, []byte("unknown-newcomer"))
	stageLifecycleAssertBytes(t, changed+"-held", []byte("original-owned-bytes"))
}

func TestLocalImportStageDeepConsumeDirectoryAfterDurable(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		if route.preview {
			continue
		}
		t.Run(route.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			token := stageLifecycleToken(t, server)
			dir := server.localImportStageDir(1)
			fired := stageLifecycleHook(t, "consume-admitted", func(_, _, _ string) {
				if err := os.Rename(dir, dir+"-held"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				for _, suffix := range []string{".book", ".json", ".parsed.json"} {
					stageLifecycleWrite(t, filepath.Join(dir, token+suffix), []byte("unknown"+suffix))
				}
			})
			request, response := stageLifecycleRequest(t, server, route, auth, token)
			router.ServeHTTP(response, request)
			if !*fired {
				t.Fatal("post-durable directory replacement fixture did not fire")
			}
			want := http.StatusOK
			if route.direct {
				want = http.StatusCreated
			}
			if response.Code != want {
				t.Errorf("durable success lost: %d %s", response.Code, response.Body.String())
			}
			for _, suffix := range []string{".book", ".json", ".parsed.json"} {
				stageLifecycleAssertBytes(t, filepath.Join(dir, token+suffix), []byte("unknown"+suffix))
			}
		})
	}
}

func TestLocalImportStageDeepCleanupObservesStartupContext(t *testing.T) {
	for _, phase := range []string{"before-start", "cleanup-remove"} {
		t.Run(phase, func(t *testing.T) {
			_, server := setupTestServer(t)
			token := stageLifecycleToken(t, server)
			dataPath, metaPath := localImportStagePaths(server.localImportStageDir(1), token)
			metadata := localImportStageMetadata{FileName: "original.txt", Extension: ".txt", CreatedAt: time.Now().Add(-25 * time.Hour)}
			encoded, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			stageLifecycleWrite(t, metaPath, encoded)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var fired *bool
			if phase == "before-start" {
				cancel()
			} else {
				fired = stageLifecycleHook(t, phase, func(_, _, _ string) { cancel() })
			}
			StartLocalImportStageCleanup(ctx, server.cfg.CacheDir)
			if fired != nil && !*fired {
				t.Fatal("startup cleanup cancellation fixture did not fire")
			}
			stageLifecycleAssertBytes(t, dataPath, []byte("第一章 原章\noriginal-owned-bytes"))
			stageLifecycleAssertBytes(t, metaPath, encoded)
		})
	}
}

func TestLocalImportStageDeepSameTokenLeaseWaitCanCancel(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	token := stageLifecycleToken(t, server)
	route := stageLifecycleRoutes()[0]
	firstReq, firstResponse := stageLifecycleRequest(t, server, route, auth, token)
	secondReq, secondResponse := stageLifecycleRequest(t, server, route, auth, token)
	ctx, cancel := context.WithCancel(secondReq.Context())
	defer cancel()
	secondReq = secondReq.WithContext(ctx)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	localImportStageLifecycleTestHook = func(phase, _, _, _ string) {
		if phase != "load-metadata" {
			return
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	t.Cleanup(func() { localImportStageLifecycleTestHook = nil })
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	go func() { router.ServeHTTP(firstResponse, firstReq); close(firstDone) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("first token work did not enter")
	}
	go func() { router.ServeHTTP(secondResponse, secondReq); close(secondDone) }()
	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Error("token lease wait ignored cancellation")
	}
	close(release)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first token worker did not join")
	}
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second token worker did not join")
	}
	if calls.Load() != 1 {
		t.Errorf("concurrent same-token request entered another bundle: %d", calls.Load())
	}
	if secondResponse.Code != http.StatusInternalServerError || !strings.Contains(secondResponse.Body.String(), "local import stage canceled") {
		t.Errorf("lease waiter canceled too late: %d %s", secondResponse.Code, secondResponse.Body.String())
	}
	if firstResponse.Code != http.StatusOK {
		t.Errorf("legitimate held request failed: %d %s", firstResponse.Code, firstResponse.Body.String())
	}
}
