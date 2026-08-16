package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestBackupTriggerRedactsInternalGenerationError(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerStorageTestUser(t, router, "backupboundaryerror")
	const sentinel = "/private/openreader-secret/volume.db: SELECT internal_backup_state"
	if err := server.db.Callback().Query().Before("gorm:query").Register("test:backup-boundary-error", func(tx *gorm.DB) {
		if tx.Statement.Table == "rss_sources" {
			tx.AddError(errors.New(sentinel))
		}
	}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/backup/trigger", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"backup failed"}` {
		t.Fatalf("backup error = %d %s, want fixed safe 500", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), sentinel) || strings.Contains(response.Body.String(), server.cfg.DataDir) {
		t.Fatalf("backup error leaked internal diagnostics: %s", response.Body.String())
	}
	assertNoGeneratedBackupFiles(t, filepath.Join(server.cfg.DataDir, "webdav"))
}

func TestCanceledBackupGenerationRequestsCreateNoPackage(t *testing.T) {
	for _, path := range []string{"/api/backup/trigger", "/api/backup/portable/trigger"} {
		t.Run(path, func(t *testing.T) {
			router, server := setupTestServer(t)
			token := registerStorageTestUser(t, router, "backupcancel"+strings.NewReplacer("/", "", "-", "").Replace(path))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			request := httptest.NewRequest(http.MethodPost, path, nil).WithContext(ctx)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			assertNoGeneratedBackupFiles(t, filepath.Join(server.cfg.DataDir, "webdav"))
		})
	}
}

func assertNoGeneratedBackupFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, "backup_") ||
			strings.HasPrefix(name, "portable_backup_") ||
			strings.HasPrefix(name, ".backup-") ||
			strings.HasPrefix(name, ".portable-backup-") {
			t.Fatalf("rejected backup generation left %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
