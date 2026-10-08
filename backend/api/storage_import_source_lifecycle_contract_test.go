package api

import (
	byteutils "bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"openreader/backend/models"
	"openreader/backend/services/webdavfs"
)

func TestStorageImportDirectoryPlanRejectsReplacementAfterAdmission(t *testing.T) {
	for _, source := range []string{"local", "webdav"} {
		for _, level := range []string{"boundary", "users", "user", "parent", "target"} {
			for _, kind := range []string{"directory", "symlink"} {
				t.Run(source+"/"+level+"/"+kind, func(t *testing.T) {
					_, server := setupTestServer(t)
					boundary := server.cfg.LocalStoreDir
					if source == "webdav" {
						boundary = server.webdavDir()
					}
					root := filepath.Join(boundary, "users", "alice")
					relative := "parent/target"
					target := filepath.Join(root, relative)
					if err := os.MkdirAll(target, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(target, "original.txt"), []byte("original"), 0o640); err != nil {
						t.Fatal(err)
					}
					service, err := webdavfs.NewScoped(boundary, root)
					if err != nil {
						t.Fatal(err)
					}
					replaced := map[string]string{"boundary": boundary, "users": filepath.Join(boundary, "users"),
						"user": root, "parent": filepath.Join(root, "parent"), "target": target}[level]
					suffix, err := filepath.Rel(replaced, target)
					if err != nil {
						t.Fatal(err)
					}
					outside := filepath.Join(t.TempDir(), "foreign")
					if err := os.MkdirAll(filepath.Join(outside, suffix), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(outside, suffix, "foreign-secret.txt"), []byte("foreign"), 0o600); err != nil {
						t.Fatal(err)
					}
					fired := false
					storageImportSourceReadTestHook = func(stage, _, path string) {
						if fired || stage != "directory-scan" || path != relative {
							return
						}
						fired = true
						if err := os.Rename(replaced, replaced+"-held"); err != nil {
							t.Fatal(err)
						}
						if kind == "symlink" {
							if err := os.Symlink(outside, replaced); err != nil {
								t.Fatal(err)
							}
						} else if err := os.Rename(outside, replaced); err != nil {
							t.Fatal(err)
						}
					}
					t.Cleanup(func() { storageImportSourceReadTestHook = nil })
					files, err := server.localStoreImportFilesWithService(service, relative)
					if !fired {
						t.Fatal("directory-scan fixture did not fire")
					}
					if !errors.Is(err, webdavfs.ErrUnsafePath) || len(files) != 0 {
						t.Errorf("changed namespace accepted: err=%v files=%+v", err, files)
					}
					bytes, err := os.ReadFile(filepath.Join(replaced+"-held", suffix, "original.txt"))
					if err != nil || string(bytes) != "original" {
						t.Fatalf("original altered: %q %v", bytes, err)
					}
				})
			}
		}
	}
}

func storageImportTestRoot(server *Server, source string) string {
	if source == "webdav" {
		return server.webdavDir()
	}
	return server.cfg.LocalStoreDir
}

func assertStorageImportNoAcceptedState(t *testing.T, server *Server, events <-chan []byte, response *httptest.ResponseRecorder) {
	t.Helper()
	var body struct {
		Items []struct {
			Book        json.RawMessage
			ImportToken string
		}
		Imported []struct{ Book json.RawMessage }
	}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	for _, item := range body.Items {
		if len(item.Book) > 0 && string(item.Book) != "null" || item.ImportToken != "" {
			t.Errorf("rejected source returned preview state: %d %s", response.Code, response.Body.String())
		}
	}
	for _, item := range body.Imported {
		if len(item.Book) > 0 && string(item.Book) != "null" {
			t.Errorf("rejected source returned imported book: %d %s", response.Code, response.Body.String())
		}
	}
	for _, model := range []any{&models.Book{}, &models.Chapter{}, &models.BookCategory{}} {
		var count int64
		if err := server.db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("rejected source wrote %T count=%d", model, count)
		}
	}
	if count := countFilesBelow(t, filepath.Join(server.cfg.CacheDir, "import-previews")); count != 0 {
		t.Errorf("rejected source left staged files: %d", count)
	}
	if emitted := drainBookWriteEvents(events); len(emitted) != 0 {
		t.Errorf("rejected source emitted events: %v", emitted)
	}
}

func TestStorageImportSourceCanceledBeforeHandoffCreatesNoAcceptedState(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		for _, action := range []string{"import-preview", "import"} {
			for _, phase := range []string{"source-admission", "directory-scan", "deep-directory-scan", "file-read", "source-read", "source-handoff"} {
				t.Run(source+"/"+action+"/"+phase, func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					root := storageImportTestRoot(server, source)
					if err := os.MkdirAll(filepath.Join(root, "selected/deep/deeper"), 0o755); err != nil {
						t.Fatal(err)
					}
					original := filepath.Join(root, "selected/deep/deeper/book.txt")
					bytes := []byte("第一章 原始\n" + string(byteutils.Repeat([]byte("正文保持不变"), 256)))
					if err := os.WriteFile(original, bytes, 0o640); err != nil {
						t.Fatal(err)
					}
					client := server.hub.AddClient(1, nil)
					t.Cleanup(func() { server.hub.RemoveClient(client) })
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					fired := false
					stage := phase
					if phase == "deep-directory-scan" {
						stage = "directory-scan"
					}
					if stage == "file-read" {
						stage = "local-file-read"
						if source == "webdav" {
							stage = "webdav-file-read"
						}
					}
					storageImportSourceReadTestHook = func(current, _, path string) {
						if phase == "deep-directory-scan" && path != "selected/deep/deeper" {
							return
						}
						if current == stage && !fired {
							fired = true
							cancel()
						}
					}
					t.Cleanup(func() { storageImportSourceReadTestHook = nil })
					path := "selected/deep/deeper/book.txt"
					if phase == "directory-scan" || phase == "deep-directory-scan" {
						path = "selected"
					}
					category := models.Category{UserID: 1, Name: "source-lifecycle-fixture"}
					if err := server.db.Create(&category).Error; err != nil {
						t.Fatal(err)
					}
					payload, err := json.Marshal(map[string]any{"paths": []string{path}, "categoryIds": []uint{category.ID}})
					if err != nil {
						t.Fatal(err)
					}
					request := httptest.NewRequest(http.MethodPost, "/api/"+source+"/"+action,
						byteutils.NewReader(payload)).WithContext(ctx)
					request.Header.Set("Authorization", auth)
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if !fired || ctx.Err() == nil {
						t.Fatal("authorized source cancellation fixture did not fire")
					}
					if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"import source read canceled"}` {
						t.Errorf("cancellation must terminate with safe JSON, got %d %s", response.Code, response.Body.String())
					}
					assertStorageImportNoAcceptedState(t, server, client.Send, response)
					after, err := os.ReadFile(original)
					if err != nil || string(after) != string(bytes) {
						t.Fatalf("source altered: %q %v", after, err)
					}
				})
			}
		}
	}
}

func TestStorageImportPlanNeverReadsSameNameReplacement(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		for _, action := range []string{"import-preview", "import"} {
			t.Run(source+"/"+action, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				root := storageImportTestRoot(server, source)
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				original := filepath.Join(root, "book.txt")
				if err := os.WriteFile(original, []byte("第一章 原始\n原字节"), 0o640); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(t.TempDir(), "foreign.txt")
				foreign := []byte("第一章 根外秘密\nforeign-secret-bytes")
				if err := os.WriteFile(outside, foreign, 0o600); err != nil {
					t.Fatal(err)
				}
				client := server.hub.AddClient(1, nil)
				t.Cleanup(func() { server.hub.RemoveClient(client) })
				fired := false
				stage := "local-file-read"
				if source == "webdav" {
					stage = "webdav-file-read"
				}
				storageImportSourceReadTestHook = func(current, _, _ string) {
					if fired || current != stage {
						return
					}
					fired = true
					if err := os.Rename(original, original+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(outside, original); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { storageImportSourceReadTestHook = nil })
				response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/"+source+"/"+action,
					auth, "application/json", []byte(`{"paths":["book.txt"]}`), false)
				if !fired {
					t.Fatal("plan-to-read same-name replacement fixture did not fire")
				}
				assertStorageImportNoAcceptedState(t, server, client.Send, response)
				stagedForeign := storageImportTreeContainsBytes(t, filepath.Join(server.cfg.CacheDir, "import-previews"), foreign)
				importedForeign := storageImportTreeContainsBytes(t, server.cfg.LibraryDir, foreign)
				if stagedForeign || importedForeign {
					t.Errorf("replacement bytes reached accepted state: stage=%v library=%v", stagedForeign, importedForeign)
				}
				if bytes, err := os.ReadFile(original + "-held"); err != nil || string(bytes) != "第一章 原始\n原字节" {
					t.Fatalf("original changed: %q %v", bytes, err)
				}
				if bytes, err := os.ReadFile(original); err != nil || string(bytes) != string(foreign) {
					t.Fatalf("foreign changed: %q %v", bytes, err)
				}
			})
		}
	}
}

func storageImportTreeContainsBytes(t *testing.T, root string, wanted []byte) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		found = found || byteutils.Contains(data, wanted)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestStorageImportDirectoryKeepsHiddenAndSafeNeighborsAndMissingSkip(t *testing.T) {
	for _, source := range []string{"local", "webdav"} {
		t.Run(source, func(t *testing.T) {
			_, server := setupTestServer(t)
			root := storageImportTestRoot(server, source)
			for _, path := range []string{"selected/A.txt", "selected/.hidden/z.txt", "selected/nonbook.bin"} {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte("original"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			outside := filepath.Join(t.TempDir(), "foreign.txt")
			if err := os.WriteFile(outside, []byte("foreign"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "selected", "escape.txt")); err != nil {
				t.Fatal(err)
			}
			service, err := webdavfs.New(root)
			if err != nil {
				t.Fatal(err)
			}
			files, err := server.localStoreImportFilesWithService(service, "selected")
			if err != nil {
				t.Fatal(err)
			}
			var paths []string
			for _, file := range files {
				paths = append(paths, file.relativePath)
			}
			if !reflect.DeepEqual(paths, []string{"selected/.hidden/z.txt", "selected/A.txt"}) {
				t.Fatalf("import hidden/sort/neighbor policy changed: %v", paths)
			}
			files, err = server.localStoreImportFilesWithService(service, "missing/child")
			if err != nil || len(files) != 0 {
				t.Fatalf("missing source must skip: %+v %v", files, err)
			}
			if _, err := os.Stat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing import created directory: %v", err)
			}
		})
	}
}

func TestStorageImportLateSymlinkAndFIFORejectWithoutLeakingWorker(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		for _, action := range []string{"import-preview", "import"} {
			for _, kind := range []string{"same-inode-symlink", "fifo"} {
				t.Run(source+"/"+action+"/"+kind, func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					root := storageImportTestRoot(server, source)
					if err := os.MkdirAll(root, 0o755); err != nil {
						t.Fatal(err)
					}
					original := filepath.Join(root, "book.txt")
					if err := os.WriteFile(original, []byte("第一章 原始\n原字节"), 0o640); err != nil {
						t.Fatal(err)
					}
					client := server.hub.AddClient(1, nil)
					t.Cleanup(func() { server.hub.RemoveClient(client) })
					phase := "local-file-read"
					if source == "webdav" {
						phase = "webdav-file-read"
					}
					var fixtureErr error
					fired := false
					storageImportSourceReadTestHook = func(stage, _, _ string) {
						if fired || stage != phase {
							return
						}
						fired = true
						fixtureErr = os.Rename(original, original+"-held")
						if fixtureErr != nil {
							return
						}
						if kind == "fifo" {
							fixtureErr = syscall.Mkfifo(original, 0o600)
						} else {
							fixtureErr = os.Symlink(original+"-held", original)
						}
					}
					t.Cleanup(func() { storageImportSourceReadTestHook = nil })
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					request := httptest.NewRequest(http.MethodPost, "/api/"+source+"/"+action, byteutils.NewReader([]byte(`{"paths":["book.txt"]}`))).WithContext(ctx)
					request.Header.Set("Authorization", auth)
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					done := make(chan struct{})
					go func() { defer close(done); router.ServeHTTP(response, request) }()
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						cancel()
						select {
						case <-done:
							t.Fatal("unsafe source blocked until cancellation")
						case <-time.After(time.Second):
							t.Fatal("unsafe source left a blocked worker")
						}
					}
					if !fired || fixtureErr != nil {
						t.Fatalf("late source fixture: fired=%v err=%v", fired, fixtureErr)
					}
					if response.Code != http.StatusBadRequest || response.Body.String() != `{"error":"invalid path"}` {
						t.Errorf("unsafe response: %d %s", response.Code, response.Body.String())
					}
					assertStorageImportNoAcceptedState(t, server, client.Send, response)
					if data, err := os.ReadFile(original + "-held"); err != nil || string(data) != "第一章 原始\n原字节" {
						t.Fatalf("original changed: %q %v", data, err)
					}
				})
			}
		}
	}
}

func TestStorageImportDeepDirectoryReplacementCannotBecomeStableSkip(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		for _, action := range []string{"import-preview", "import"} {
			for _, kind := range []string{"directory", "symlink"} {
				t.Run(source+"/"+action+"/"+kind, func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					target := filepath.Join(storageImportTestRoot(server, source), "selected/deep")
					if err := os.MkdirAll(target, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(target, "book.txt"), []byte("第一章 原始\n原字节"), 0o640); err != nil {
						t.Fatal(err)
					}
					outside := filepath.Join(t.TempDir(), "foreign")
					if err := os.Mkdir(outside, 0o755); err != nil {
						t.Fatal(err)
					}
					foreign := []byte("第一章 根外\nforeign-secret-bytes")
					if err := os.WriteFile(filepath.Join(outside, "foreign.txt"), foreign, 0o600); err != nil {
						t.Fatal(err)
					}
					client := server.hub.AddClient(1, nil)
					t.Cleanup(func() { server.hub.RemoveClient(client) })
					fired := false
					storageImportSourceReadTestHook = func(stage, _, relative string) {
						if fired || stage != "directory-scan" || relative != "selected/deep" {
							return
						}
						fired = true
						if err := os.Rename(target, target+"-held"); err != nil {
							t.Fatal(err)
						}
						if kind == "symlink" {
							if err := os.Symlink(outside, target); err != nil {
								t.Fatal(err)
							}
						} else if err := os.Rename(outside, target); err != nil {
							t.Fatal(err)
						}
					}
					t.Cleanup(func() { storageImportSourceReadTestHook = nil })
					response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/"+source+"/"+action, auth, "application/json", []byte(`{"paths":["selected"]}`), false)
					if !fired || response.Code != http.StatusBadRequest {
						t.Fatalf("deep swap result: fired=%v response=%d %s", fired, response.Code, response.Body.String())
					}
					assertStorageImportNoAcceptedState(t, server, client.Send, response)
					if data, err := os.ReadFile(filepath.Join(target+"-held", "book.txt")); err != nil || string(data) != "第一章 原始\n原字节" {
						t.Fatalf("original changed: %q %v", data, err)
					}
					foreignRoot := target
					if kind == "symlink" {
						foreignRoot = outside
					}
					if data, err := os.ReadFile(filepath.Join(foreignRoot, "foreign.txt")); err != nil || string(data) != string(foreign) {
						t.Fatalf("foreign changed: %q %v", data, err)
					}
				})
			}
		}
	}
}

func TestStorageImportOpenedFileRenameKeepsOriginalBytes(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		for _, action := range []string{"import-preview", "import"} {
			t.Run(source+"/"+action, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				root := storageImportTestRoot(server, source)
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				originalPath := filepath.Join(root, "book.txt")
				original := []byte("第一章 原始\n" + string(byteutils.Repeat([]byte("原始正文"), 256)))
				foreign := []byte("第一章 新文件\nforeign-secret-bytes")
				if err := os.WriteFile(originalPath, original, 0o640); err != nil {
					t.Fatal(err)
				}
				fired := false
				storageImportSourceReadTestHook = func(stage, _, _ string) {
					if fired || stage != "source-read" {
						return
					}
					fired = true
					if err := os.Rename(originalPath, originalPath+"-held"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(originalPath, foreign, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { storageImportSourceReadTestHook = nil })
				response, _ := performLocalStoreRequest(router, http.MethodPost, "/api/"+source+"/"+action, auth, "application/json", []byte(`{"paths":["book.txt"]}`), false)
				if !fired || response.Code != http.StatusOK {
					t.Fatalf("opened rename: fired=%v status=%d %s", fired, response.Code, response.Body.String())
				}
				acceptedRoot := server.cfg.LibraryDir
				if action == "import-preview" {
					acceptedRoot = filepath.Join(server.cfg.CacheDir, "import-previews")
				}
				if !storageImportTreeContainsBytes(t, acceptedRoot, original) || storageImportTreeContainsBytes(t, acceptedRoot, foreign) {
					t.Fatal("opened fd did not deliver only original bytes")
				}
				if data, err := os.ReadFile(originalPath + "-held"); err != nil || string(data) != string(original) {
					t.Fatalf("original changed: %q %v", data, err)
				}
				if data, err := os.ReadFile(originalPath); err != nil || string(data) != string(foreign) {
					t.Fatalf("replacement changed: %q %v", data, err)
				}
			})
		}
	}
}

func TestStorageImportLaterSourceCancelPreservesEarlierCommittedBookAndEvent(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		t.Run(source, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			root := storageImportTestRoot(server, source)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"first.txt", "second.txt"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("第一章 原始\n原字节"), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			client := server.hub.AddClient(1, nil)
			t.Cleanup(func() { server.hub.RemoveClient(client) })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			phase := "local-file-read"
			if source == "webdav" {
				phase = "webdav-file-read"
			}
			fired := false
			storageImportSourceReadTestHook = func(stage, _, relative string) {
				if stage == phase && relative == "second.txt" {
					fired = true
					cancel()
				}
			}
			t.Cleanup(func() { storageImportSourceReadTestHook = nil })
			request := httptest.NewRequest(http.MethodPost, "/api/"+source+"/import", byteutils.NewReader([]byte(`{"paths":["first.txt","second.txt"]}`))).WithContext(ctx)
			request.Header.Set("Authorization", auth)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !fired || response.Code != 500 {
				t.Fatalf("later cancel fixture: fired=%v response=%d %s", fired, response.Code, response.Body.String())
			}
			var books []models.Book
			if err := server.db.Find(&books).Error; err != nil {
				t.Fatal(err)
			}
			if len(books) != 1 {
				t.Fatalf("earlier committed book lost or failed source accepted: %+v", books)
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
					t.Errorf("prior commit notification: %s", message)
				}
			default:
				t.Error("source termination swallowed earlier durable book notification")
			}
			if events := drainBookWriteEvents(client.Send); len(events) != 0 {
				t.Errorf("extra source-failure events: %v", events)
			}
		})
	}
}
