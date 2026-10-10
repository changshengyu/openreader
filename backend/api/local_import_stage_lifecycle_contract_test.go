package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/localbook"
)

type stageLifecycleRoute struct {
	name, endpoint  string
	direct, preview bool
}

func stageLifecycleRoutes() []stageLifecycleRoute {
	return []stageLifecycleRoute{
		{"direct-preview", "/api/imports/books/preview", true, true},
		{"direct-confirm", "/api/imports/books", true, false},
		{"txt-alias-confirm", "/api/imports/txt", true, false},
		{"local-preview", "/api/local-store/import-preview", false, true},
		{"local-confirm", "/api/local-store/import", false, false},
		{"webdav-preview", "/api/webdav/import-preview", false, true},
		{"webdav-confirm", "/api/webdav/import", false, false},
	}
}

func stageLifecycleRequest(t *testing.T, server *Server, route stageLifecycleRoute, auth, token string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	if route.direct {
		part := directLocalImportMultipartPart{name: "importToken", data: []byte(token)}
		if token == "" {
			part = directLocalImportMultipartPart{name: "file", filename: "original.txt", data: []byte("第一章 原章\noriginal-owned-bytes")}
		}
		return directLocalImportMultipartRequest(t, route.endpoint, auth, []directLocalImportMultipartPart{part})
	}
	body := `{"items":[{"path":"original.txt","importToken":"` + token + `"}]}`
	if token == "" {
		root := server.cfg.LocalStoreDir
		if strings.HasPrefix(route.name, "webdav") {
			root = server.webdavDir()
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		stageLifecycleWrite(t, filepath.Join(root, "original.txt"), []byte("第一章 原章\noriginal-owned-bytes"))
		body = `{"paths":["original.txt"]}`
	}
	request := httptest.NewRequest(http.MethodPost, route.endpoint, strings.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", "application/json")
	return request, httptest.NewRecorder()
}

func stageLifecycleWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func stageLifecycleAssertBytes(t *testing.T, path string, expected []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, expected) {
		t.Errorf("owned/foreign fixture changed: %q %v", data, err)
	}
}

func stageLifecycleToken(t *testing.T, server *Server) string {
	t.Helper()
	token, err := server.stageLocalImport(1, "original.txt", ".txt", []byte("第一章 原章\noriginal-owned-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func stageLifecycleHook(t *testing.T, wanted string, callback func(dir, token, path string)) *bool {
	t.Helper()
	fired := new(bool)
	localImportStageLifecycleTestHook = func(phase, dir, token, path string) {
		if *fired || phase != wanted {
			return
		}
		*fired = true
		callback(dir, token, path)
	}
	t.Cleanup(func() { localImportStageLifecycleTestHook = nil })
	return fired
}

func stageLifecycleNoBooks(t *testing.T, server *Server) {
	t.Helper()
	for _, model := range []any{&models.Book{}, &models.Chapter{}, &models.BookCategory{}} {
		var count int64
		if err := server.db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("rejected stage accepted %T rows: %d", model, count)
		}
	}
}

func TestLocalImportStageLifecycleCreateRejectsChangedAncestors(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		if !route.preview {
			continue
		}
		for _, level := range []string{"cache", "stage-root", "user"} {
			for _, kind := range []string{"directory", "symlink"} {
				t.Run(route.name+"/"+level+"/"+kind, func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					request, response := stageLifecycleRequest(t, server, route, auth, "")
					var foreignDir string
					fired := stageLifecycleHook(t, "create-admitted", func(dir, _, _ string) {
						replaced := dir
						if level == "cache" {
							replaced = server.cfg.CacheDir
						}
						if level == "stage-root" {
							replaced = filepath.Dir(dir)
						}
						relative, err := filepath.Rel(replaced, dir)
						if err != nil {
							t.Fatal(err)
						}
						outside := filepath.Join(t.TempDir(), "foreign")
						if err := os.MkdirAll(filepath.Join(outside, relative), 0o750); err != nil {
							t.Fatal(err)
						}
						stageLifecycleWrite(t, filepath.Join(outside, relative, "sentinel"), []byte("foreign-sentinel"))
						if err := os.Rename(replaced, replaced+"-held"); err != nil {
							t.Fatal(err)
						}
						if kind == "symlink" {
							if err := os.Symlink(outside, replaced); err != nil {
								t.Fatal(err)
							}
							foreignDir = filepath.Join(outside, relative)
						} else {
							if err := os.Rename(outside, replaced); err != nil {
								t.Fatal(err)
							}
							foreignDir = filepath.Join(replaced, relative)
						}
					})
					router.ServeHTTP(response, request)
					if !*fired {
						t.Fatal("create admission fixture did not fire")
					}
					if strings.Contains(response.Body.String(), `"importToken"`) && !strings.Contains(response.Body.String(), `"error"`) {
						t.Errorf("changed stage namespace accepted preview: %d %s", response.Code, response.Body.String())
					}
					entries, err := os.ReadDir(foreignDir)
					if err != nil {
						t.Fatal(err)
					}
					if len(entries) != 1 || entries[0].Name() != "sentinel" {
						t.Errorf("stage wrote foreign namespace: %d entries", len(entries))
					}
					stageLifecycleAssertBytes(t, filepath.Join(foreignDir, "sentinel"), []byte("foreign-sentinel"))
					stageLifecycleNoBooks(t, server)
				})
			}
		}
	}
}

func TestLocalImportStageLifecycleWorkCancellation(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		phases := []string{"load-metadata", "prepared-ready"}
		if route.preview {
			phases = append(phases, "create-admitted")
		}
		for _, phase := range phases {
			t.Run(route.name+"/"+phase, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				token := ""
				if phase != "create-admitted" {
					token = stageLifecycleToken(t, server)
				}
				request, response := stageLifecycleRequest(t, server, route, auth, token)
				ctx, cancel := context.WithCancel(request.Context())
				defer cancel()
				request = request.WithContext(ctx)
				client := server.hub.AddClient(1, nil)
				t.Cleanup(func() { server.hub.RemoveClient(client) })
				fired := stageLifecycleHook(t, phase, func(_, _, _ string) { cancel() })
				router.ServeHTTP(response, request)
				if !*fired {
					t.Fatal("authorized stage cancellation fixture did not fire")
				}
				if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "local import stage canceled") {
					t.Errorf("stage cancellation returned success/wrong error: %d %s", response.Code, response.Body.String())
				}
				stageLifecycleNoBooks(t, server)
				if events := drainBookWriteEvents(client.Send); len(events) != 0 {
					t.Errorf("canceled stage emitted %d events", len(events))
				}
				if phase == "create-admitted" && directLocalImportStageEntryCount(t, server, 1) != 0 {
					t.Error("canceled create published token files")
				}
			})
		}
	}
}

func TestLocalImportStageLifecycleLoadRejectsMetadataToBytesSubstitution(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		for _, level := range []string{"cache", "user", "book"} {
			t.Run(route.name+"/"+level, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				token := stageLifecycleToken(t, server)
				dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), token)
				metadata, err := os.ReadFile(metadataPath)
				if err != nil {
					t.Fatal(err)
				}
				foreign := []byte("第一章 Foreign章节\nforeign-secret-bytes")
				var heldPath string
				fired := stageLifecycleHook(t, "load-metadata", func(dir, _, _ string) {
					replaced := dataPath
					if level == "user" {
						replaced = dir
					}
					if level == "cache" {
						replaced = server.cfg.CacheDir
					}
					if err := os.Rename(replaced, replaced+"-held"); err != nil {
						t.Fatal(err)
					}
					if level == "book" {
						heldPath = replaced + "-held"
					} else {
						relative, err := filepath.Rel(replaced, dataPath)
						if err != nil {
							t.Fatal(err)
						}
						heldPath = filepath.Join(replaced+"-held", relative)
						if err := os.MkdirAll(dir, 0o700); err != nil {
							t.Fatal(err)
						}
						stageLifecycleWrite(t, metadataPath, metadata)
					}
					stageLifecycleWrite(t, dataPath, foreign)
				})
				request, response := stageLifecycleRequest(t, server, route, auth, token)
				router.ServeHTTP(response, request)
				if !*fired {
					t.Fatal("metadata-to-bytes fixture did not fire")
				}
				if !strings.Contains(response.Body.String(), "invalid or expired local import token") {
					t.Errorf("substituted token accepted: %d %s", response.Code, response.Body.String())
				}
				if strings.Contains(response.Body.String(), "Foreign") || storageImportTreeContainsBytes(t, server.cfg.LibraryDir, foreign) {
					t.Error("foreign bytes reached preview/library")
				}
				stageLifecycleNoBooks(t, server)
				stageLifecycleAssertBytes(t, heldPath, []byte("第一章 原章\noriginal-owned-bytes"))
				stageLifecycleAssertBytes(t, dataPath, foreign)
			})
		}
	}
}

func stageLifecyclePrepared() (localbook.ImportRequest, localbook.PreparedImport) {
	request := localbook.ImportRequest{Extension: ".txt", Data: []byte("第一章 原章\noriginal-owned-bytes")}
	return request, localbook.NewPreparedImport(request, engine.ParsedBook{Title: "原书", Chapters: []engine.TXTChapter{{Title: "原章", Content: "original-owned-bytes"}}})
}

func TestLocalImportStageLifecycleStableLinksRejected(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		for _, kind := range []string{"book", "user", "prepared"} {
			if route.preview && kind == "prepared" {
				continue
			}
			t.Run(route.name+"/"+kind, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				token := stageLifecycleToken(t, server)
				dir := server.localImportStageDir(1)
				dataPath, _ := localImportStagePaths(dir, token)
				outside := filepath.Join(t.TempDir(), "foreign")
				original := []byte("第一章 原章\noriginal-owned-bytes")
				foreignPath, foreignBytes := outside, original
				switch kind {
				case "book":
					if err := os.Rename(dataPath, dataPath+"-held"); err != nil {
						t.Fatal(err)
					}
					stageLifecycleWrite(t, outside, original)
					if err := os.Symlink(outside, dataPath); err != nil {
						t.Fatal(err)
					}
				case "user":
					if err := os.Rename(dir, outside); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, dir); err != nil {
						t.Fatal(err)
					}
					foreignPath = filepath.Join(outside, token+".book")
				case "prepared":
					_, prepared := stageLifecyclePrepared()
					prepared.Book.Title = "Foreign书"
					prepared.Book.Chapters[0].Content = "foreign-secret-bytes"
					encoded, err := json.Marshal(prepared)
					if err != nil {
						t.Fatal(err)
					}
					stageLifecycleWrite(t, outside, encoded)
					foreignBytes = encoded
					if err := os.Symlink(outside, localImportPreparedStagePath(dir, token)); err != nil {
						t.Fatal(err)
					}
				}
				request, response := stageLifecycleRequest(t, server, route, auth, token)
				router.ServeHTTP(response, request)
				if !strings.Contains(response.Body.String(), "invalid or expired local import token") {
					t.Errorf("stable stage symlink accepted: %d %s", response.Code, response.Body.String())
				}
				stageLifecycleNoBooks(t, server)
				stageLifecycleAssertBytes(t, foreignPath, foreignBytes)
			})
		}
	}
}

func TestLocalImportStageLifecyclePreparedPublicationPreservesUnknownEntity(t *testing.T) {
	for _, kind := range []string{"target", "temporary", "user"} {
		t.Run(kind, func(t *testing.T) {
			_, server := setupTestServer(t)
			token := stageLifecycleToken(t, server)
			_, prepared := stageLifecyclePrepared()
			if err := server.saveStagedPreparedImport(1, token, prepared); err != nil {
				t.Fatal(err)
			}
			target := localImportPreparedStagePath(server.localImportStageDir(1), token)
			original, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			foreign := []byte("foreign-newcomer")
			var unknown, held string
			fired := stageLifecycleHook(t, "prepared-ready", func(dir, _, temporary string) {
				switch kind {
				case "target":
					if err := os.Rename(target, target+"-held"); err != nil {
						t.Fatal(err)
					}
					unknown, held = target, target+"-held"
					stageLifecycleWrite(t, unknown, foreign)
				case "temporary":
					if err := os.Rename(temporary, temporary+"-held"); err != nil {
						t.Fatal(err)
					}
					unknown, held = temporary, target
					stageLifecycleWrite(t, unknown, foreign)
				case "user":
					if err := os.Rename(dir, dir+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(dir, 0o700); err != nil {
						t.Fatal(err)
					}
					unknown, held = target, filepath.Join(dir+"-held", filepath.Base(target))
					stageLifecycleWrite(t, unknown, foreign)
				}
			})
			err = server.saveStagedPreparedImport(1, token, prepared)
			if !*fired {
				t.Fatal("prepared publication fixture did not fire")
			}
			if err == nil {
				t.Error("unknown entity accepted at prepared publication")
			}
			stageLifecycleAssertBytes(t, unknown, foreign)
			stageLifecycleAssertBytes(t, held, original)
		})
	}
}

func TestLocalImportStageLifecycleConsumeAfterDurablePreservesNewcomer(t *testing.T) {
	for _, route := range stageLifecycleRoutes() {
		if route.preview {
			continue
		}
		t.Run(route.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			token := stageLifecycleToken(t, server)
			_, prepared := stageLifecyclePrepared()
			if err := server.saveStagedPreparedImport(1, token, prepared); err != nil {
				t.Fatal(err)
			}
			paths := []string{}
			fired := stageLifecycleHook(t, "consume-admitted", func(dir, _, _ string) {
				var count int64
				if err := server.db.Model(&models.Book{}).Count(&count).Error; err != nil || count != 1 {
					t.Fatalf("consume must be after durable book: %d %v", count, err)
				}
				for _, suffix := range []string{".book", ".json", ".parsed.json"} {
					path := filepath.Join(dir, token+suffix)
					if err := os.Rename(path, path+"-held"); err != nil {
						t.Fatal(err)
					}
					stageLifecycleWrite(t, path, []byte("foreign-newcomer"+suffix))
					paths = append(paths, path)
				}
			})
			request, response := stageLifecycleRequest(t, server, route, auth, token)
			router.ServeHTTP(response, request)
			if !*fired {
				t.Fatal("durable consumption fixture did not fire")
			}
			want := http.StatusOK
			if route.direct {
				want = http.StatusCreated
			}
			if response.Code != want || strings.Contains(response.Body.String(), `"error"`) {
				t.Errorf("durable import falsely failed: %d %s", response.Code, response.Body.String())
			}
			for _, path := range paths {
				stageLifecycleAssertBytes(t, path, []byte("foreign-newcomer"+strings.TrimPrefix(filepath.Base(path), token)))
			}
		})
	}
}

func TestLocalImportStageLifecycleCleanupPreservesChangedClassification(t *testing.T) {
	for _, kind := range []string{"expired-bundle", "user", "orphan-book", "orphan-parsed", "temporary"} {
		t.Run(kind, func(t *testing.T) {
			_, server := setupTestServer(t)
			token := stageLifecycleToken(t, server)
			dir := server.localImportStageDir(1)
			dataPath, metadataPath := localImportStagePaths(dir, token)
			metadata := localImportStageMetadata{FileName: "original.txt", Extension: ".txt", CreatedAt: time.Now().Add(-25 * time.Hour)}
			encoded, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			stageLifecycleWrite(t, metadataPath, encoded)
			phase, selected := "cleanup-remove", dataPath
			if strings.HasPrefix(kind, "orphan") || kind == "temporary" {
				if err := os.Remove(metadataPath); err != nil {
					t.Fatal(err)
				}
				if kind != "orphan-book" {
					if err := os.Remove(dataPath); err != nil {
						t.Fatal(err)
					}
					selected = filepath.Join(dir, token+".parsed.json")
					if kind == "temporary" {
						selected = filepath.Join(dir, token+".parsed-crash")
					}
					stageLifecycleWrite(t, selected, []byte("old-owned"))
					phase = "cleanup-unlink"
				}
				old := time.Now().Add(-25 * time.Hour)
				if err := os.Chtimes(selected, old, old); err != nil {
					t.Fatal(err)
				}
			}
			var unknown string
			fired := stageLifecycleHook(t, phase, func(_, _, _ string) {
				if kind == "user" {
					if err := os.Rename(dir, dir+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(dir, 0o700); err != nil {
						t.Fatal(err)
					}
					stageLifecycleWrite(t, metadataPath, encoded)
					unknown = dataPath
				} else {
					if err := os.Rename(selected, selected+"-held"); err != nil {
						t.Fatal(err)
					}
					unknown = selected
				}
				stageLifecycleWrite(t, unknown, []byte("fresh-foreign-newcomer"))
			})
			CleanupExpiredLocalImportStages(server.cfg.CacheDir)
			if !*fired {
				t.Fatal("TTL classification fixture did not fire")
			}
			stageLifecycleAssertBytes(t, unknown, []byte("fresh-foreign-newcomer"))
		})
	}
}

func TestLocalImportStageLifecycleMetadataReadBound(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(map[int]string{0: "exact-1MiB", 1: "plus-one"}[extra], func(t *testing.T) {
			_, server := setupTestServer(t)
			token := stageLifecycleToken(t, server)
			_, path := localImportStagePaths(server.localImportStageDir(1), token)
			metadata, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			encoded := append(metadata, bytes.Repeat([]byte(" "), (1<<20)+extra-len(metadata))...)
			stageLifecycleWrite(t, path, encoded)
			_, _, err = server.loadStagedLocalImport(1, token)
			if extra == 0 && err != nil {
				t.Fatalf("valid exact-limit metadata rejected: %v", err)
			}
			if extra == 1 && !errors.Is(err, errInvalidLocalImportToken) {
				t.Errorf("over-limit metadata accepted: %v", err)
			}
			stageLifecycleAssertBytes(t, path, encoded)
		})
	}
}

func TestLocalImportStageLifecycleMaxInt64ReadDoesNotWrap(t *testing.T) {
	_, server := setupTestServer(t)
	server.cfg.MaxImportBytes = math.MaxInt64
	input := []byte("original-owned-bytes")
	data, err := server.readBoundedLocalImport(bytes.NewReader(input))
	if err != nil || !bytes.Equal(data, input) {
		t.Errorf("MaxInt64 read budget wrapped: %q %v", data, err)
	}
	var output bytes.Buffer
	if err := server.copyBoundedLocalImport(&output, bytes.NewReader(input)); err != nil || !bytes.Equal(output.Bytes(), input) {
		t.Errorf("MaxInt64 copy budget wrapped: %q %v", output.Bytes(), err)
	}
}

func TestLocalImportStageLifecycleFIFOHasBoundedRejection(t *testing.T) {
	for _, kind := range []string{"metadata", "book", "prepared"} {
		t.Run(kind, func(t *testing.T) {
			_, server := setupTestServer(t)
			token := stageLifecycleToken(t, server)
			dir := server.localImportStageDir(1)
			dataPath, metadataPath := localImportStagePaths(dir, token)
			path := dataPath
			payload := []byte("第一章 原章\noriginal-owned-bytes")
			request, prepared := stageLifecyclePrepared()
			switch kind {
			case "metadata":
				path = metadataPath
				var err error
				payload, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			case "prepared":
				path = localImportPreparedStagePath(dir, token)
				var err error
				payload, err = json.Marshal(prepared)
				if err != nil {
					t.Fatal(err)
				}
				stageLifecycleWrite(t, path, payload)
			}
			if err := os.Rename(path, path+"-held"); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if kind == "prepared" {
					_, accepted := server.loadStagedPreparedImport(1, token, request)
					if accepted {
						done <- errors.New("prepared FIFO accepted")
					} else {
						done <- nil
					}
					return
				}
				_, _, err := server.loadStagedLocalImport(1, token)
				if errors.Is(err, errInvalidLocalImportToken) {
					done <- nil
				} else {
					done <- errors.New("token FIFO accepted")
				}
			}()
			var result error
			blocked := false
			select {
			case result = <-done:
			case <-time.After(150 * time.Millisecond):
				blocked = true
				// Release the LEGACY blocking reader using a test-owned nonblocking
				// peer, then join it. No hanging FIFO operation is left behind.
				fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
				if err != nil {
					t.Fatalf("test peer could not release legacy reader: %v", err)
				}
				peer := os.NewFile(uintptr(fd), "test-owned-fifo-peer")
				_, writeErr := peer.Write(payload)
				closeErr := peer.Close()
				if writeErr != nil || closeErr != nil {
					t.Errorf("peer release failed: %v %v", writeErr, closeErr)
				}
				select {
				case result = <-done:
				case <-time.After(2 * time.Second):
					t.Fatal("legacy FIFO worker did not join after peer release")
				}
			}
			if blocked || result != nil {
				t.Errorf("stage FIFO not promptly rejected: blocked=%v result=%v", blocked, result)
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Errorf("unknown FIFO deleted/changed: %v %v", info, err)
			}
			stageLifecycleAssertBytes(t, path+"-held", payload)
		})
	}
}
