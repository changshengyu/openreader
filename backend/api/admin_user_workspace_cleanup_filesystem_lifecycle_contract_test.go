package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestDeletedUserWorkspaceCleanupNeverFollowsAncestorSymlink(t *testing.T) {
	type cleanupRoot struct {
		name     string
		root     func(*Server) string
		ancestor string
		target   func(userID uint, username string) string
	}
	roots := []cleanupRoot{
		{
			name:     "webdav",
			root:     func(server *Server) string { return server.cfg.DataDir },
			ancestor: filepath.Join("webdav", "users"),
			target:   func(_ uint, username string) string { return engine.SafeFilename(username) },
		},
		{
			name:     "local-store",
			root:     func(server *Server) string { return server.cfg.LocalStoreDir },
			ancestor: "users",
			target:   func(_ uint, username string) string { return engine.SafeFilename(username) },
		},
		{
			name:     "local-library",
			root:     func(server *Server) string { return server.cfg.LibraryDir },
			ancestor: "data",
			target:   func(_ uint, username string) string { return engine.SafeFilename(username) },
		},
		{
			name:     "uploads",
			root:     func(server *Server) string { return server.cfg.DataDir },
			ancestor: filepath.Join("uploads", "users"),
			target:   func(userID uint, _ string) string { return strconv.FormatUint(uint64(userID), 10) },
		},
		{
			name:     "cover-cache",
			root:     func(server *Server) string { return server.cfg.CacheDir },
			ancestor: "cover-images",
			target:   func(userID uint, _ string) string { return "user-" + strconv.FormatUint(uint64(userID), 10) },
		},
	}

	for _, action := range []string{"batch-delete", "cleanup-inactive"} {
		for _, cleanupRoot := range roots {
			t.Run(action+"/"+cleanupRoot.name, func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				lastActive := time.Now()
				if action == "cleanup-inactive" {
					lastActive = lastActive.Add(-91 * 24 * time.Hour)
				}
				user := createUserManagementP2User(t, server, "cleanup"+cleanupRoot.name, lastActive)

				configuredRoot := cleanupRoot.root(server)
				ancestor := filepath.Join(configuredRoot, cleanupRoot.ancestor)
				if err := os.RemoveAll(ancestor); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(ancestor), 0o755); err != nil {
					t.Fatal(err)
				}
				outside := t.TempDir()
				outsideTarget := filepath.Join(outside, cleanupRoot.target(user.ID, user.Username))
				if err := os.MkdirAll(outsideTarget, 0o755); err != nil {
					t.Fatal(err)
				}
				sentinel := filepath.Join(outsideTarget, "outside-sentinel")
				if err := os.WriteFile(sentinel, []byte("must remain"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, ancestor); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}

				response := deleteUserWorkspaceByAction(t, router, auth, action, user.ID)
				if response.Deleted != 1 {
					t.Fatalf("deleted=%d, want 1", response.Deleted)
				}
				assertUserManagementP2DataRemoved(t, server, user)
				if response.CleanupFailures == 0 {
					t.Errorf("unsafe %s ancestor was reported as successful", cleanupRoot.name)
				}
				if data, err := os.ReadFile(sentinel); err != nil || string(data) != "must remain" {
					t.Errorf("unsafe %s cleanup touched root-external data: data=%q err=%v", cleanupRoot.name, data, err)
				}
				if info, err := os.Lstat(ancestor); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("unsafe %s ancestor link was changed: info=%v err=%v", cleanupRoot.name, info, err)
				}
			})
		}
	}
}

func TestDeletedUserWorkspaceCleanupPreservesCollidingLegacyUsernameDirectory(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	target := createUserManagementP2User(t, server, "legacy/name", time.Now())
	survivor := createUserManagementP2User(t, server, "legacy:name", time.Now())
	if engine.SafeFilename(target.Username) != engine.SafeFilename(survivor.Username) {
		t.Fatal("legacy username fixture must share one persisted directory projection")
	}

	sharedRoot := filepath.Join(server.cfg.DataDir, "webdav", "users", engine.SafeFilename(target.Username))
	if err := os.MkdirAll(sharedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(sharedRoot, "survivor-data")
	if err := os.WriteFile(sentinel, []byte(survivor.Username), 0o600); err != nil {
		t.Fatal(err)
	}

	response := deleteUserWorkspaceByAction(t, router, auth, "batch-delete", target.ID)
	if response.Deleted != 1 {
		t.Fatalf("deleted=%d, want 1", response.Deleted)
	}
	assertUserManagementP2DataRemoved(t, server, target)
	var survivorCount int64
	if err := server.db.Model(&models.User{}).Where("id = ?", survivor.ID).Count(&survivorCount).Error; err != nil || survivorCount != 1 {
		t.Fatalf("colliding legacy survivor row changed: count=%d err=%v", survivorCount, err)
	}
	if response.CleanupFailures == 0 {
		t.Errorf("colliding legacy workspace was reported as safely cleaned")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != survivor.Username {
		t.Errorf("deleting colliding legacy user touched survivor data: data=%q err=%v", data, err)
	}
}

type deletedUserWorkspaceResponse struct {
	Deleted         int `json:"deleted"`
	CleanupFailures int `json:"cleanupFailures"`
}

func deleteUserWorkspaceByAction(
	t *testing.T,
	router *gin.Engine,
	authorization string,
	action string,
	userID uint,
) deletedUserWorkspaceResponse {
	t.Helper()
	path := "/api/admin/users/batch-delete"
	body := `{"ids":[` + strconv.FormatUint(uint64(userID), 10) + `]}`
	if action == "cleanup-inactive" {
		path = "/api/admin/cleanup-inactive"
		body = `{}`
	}
	response := adminContractRequest(router, http.MethodPost, path, body, authorization)
	if response.Code != http.StatusOK {
		t.Fatalf("%s: status=%d body=%s", action, response.Code, response.Body.String())
	}
	var payload deletedUserWorkspaceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s response: %v", action, err)
	}
	return payload
}
