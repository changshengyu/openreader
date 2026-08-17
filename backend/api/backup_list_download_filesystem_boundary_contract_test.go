package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type backupBoundaryListItem struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Format string `json:"format"`
}

func TestBackupListHidesSymlinksSpecialFilesAndNonZipNames(t *testing.T) {
	router, server := setupTestServer(t)
	registerStorageTestUser(t, router, "backuplistadmin")
	const username = "backuplistuser"
	auth := registerStorageTestUser(t, router, username)
	root := backupBoundaryUserRoot(t, server, username)

	validData := backupBoundaryZIP(t, "bookshelf.json", `[]`)
	if err := os.WriteFile(filepath.Join(root, "backup_valid.zip"), validData, 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(server.cfg.DataDir, "outside-caller-root-secret")
	if err := os.WriteFile(outside, []byte("outside caller root"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "backup_escape.zip")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backup_not_zip.txt"), []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "backup_directory.zip"), 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "backup_pipe.zip")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	items := backupBoundaryList(t, router, auth)
	if len(items) != 1 || items[0].Name != "backup_valid.zip" || items[0].Format != "logical" || items[0].Size != int64(len(validData)) {
		t.Fatalf("unsafe backup entries reached list: %+v", items)
	}
	for _, path := range []string{filepath.Join(root, "backup_escape.zip"), filepath.Join(root, "backup_not_zip.txt"), filepath.Join(root, "backup_directory.zip"), fifo} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("list changed rejected mounted entry %s: %v", path, err)
		}
	}
}

func TestBackupDownloadRejectsSymlinkDirectoryAndNonZipName(t *testing.T) {
	router, server := setupTestServer(t)
	registerStorageTestUser(t, router, "backupdownloadadmin")
	const username = "backupdownloaduser"
	auth := registerStorageTestUser(t, router, username)
	root := backupBoundaryUserRoot(t, server, username)
	outside := filepath.Join(server.cfg.DataDir, "outside-download-secret")
	if err := os.WriteFile(outside, []byte("outside download secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "backup_escape.zip")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "backup_directory.zip"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backup_not_zip.txt"), []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "symlink", path: "backup_escape.zip", wantStatus: http.StatusNotFound, wantBody: `{"error":"backup not found"}`},
		{name: "directory", path: "backup_directory.zip", wantStatus: http.StatusNotFound, wantBody: `{"error":"backup not found"}`},
		{name: "prefix only non zip", path: "backup_not_zip.txt", wantStatus: http.StatusBadRequest, wantBody: `{"error":"invalid backup name"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := backupBoundaryDownload(router, auth, test.path)
			if response.Code != test.wantStatus || strings.TrimSpace(response.Body.String()) != test.wantBody {
				t.Fatalf("download %s = %d %q, want %d %s", test.path, response.Code, response.Body.String(), test.wantStatus, test.wantBody)
			}
			if strings.Contains(response.Body.String(), server.cfg.DataDir) || strings.Contains(response.Body.String(), "outside download secret") {
				t.Fatalf("rejected download exposed host data: %s", response.Body.String())
			}
		})
	}
}

func TestBackupListAndDownloadRejectCallerRootAncestorSymlink(t *testing.T) {
	router, server := setupTestServer(t)
	registerStorageTestUser(t, router, "backuprootadmin")
	const username = "backuprootuser"
	auth := registerStorageTestUser(t, router, username)
	webdavRoot := filepath.Join(server.cfg.DataDir, "webdav")
	if err := os.MkdirAll(webdavRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideUsers := filepath.Join(server.cfg.DataDir, "outside-users")
	outsideRoot := filepath.Join(outsideUsers, username)
	if err := os.MkdirAll(outsideRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideData := backupBoundaryZIP(t, "outside.txt", "outside root")
	if err := os.WriteFile(filepath.Join(outsideRoot, "backup_escape.zip"), outsideData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideUsers, filepath.Join(webdavRoot, "users")); err != nil {
		t.Fatal(err)
	}

	if items := backupBoundaryList(t, router, auth); len(items) != 0 {
		t.Fatalf("ancestor symlink exposed backup list: %+v", items)
	}
	response := backupBoundaryDownload(router, auth, "backup_escape.zip")
	if response.Code != http.StatusNotFound || strings.TrimSpace(response.Body.String()) != `{"error":"backup not found"}` {
		t.Fatalf("ancestor symlink download = %d %q, want safe 404", response.Code, response.Body.String())
	}
}

func TestBackupListAndDownloadKeepValidZipCompatibility(t *testing.T) {
	router, server := setupTestServer(t)
	registerStorageTestUser(t, router, "backupvalidadmin")
	const username = "backupvaliduser"
	auth := registerStorageTestUser(t, router, username)

	if response := backupBoundaryListResponse(router, auth); response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("missing backup root list = %d %q, want 200 []", response.Code, response.Body.String())
	}
	root := backupBoundaryUserRoot(t, server, username)
	logicalData := backupBoundaryZIP(t, "bookshelf.json", `[]`)
	portableData := backupBoundaryZIP(t, "unknown.json", `{}`)
	if err := os.WriteFile(filepath.Join(root, "backup_history.ZIP"), logicalData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "portable_backup_history.ZIP"), portableData, 0o600); err != nil {
		t.Fatal(err)
	}

	items := backupBoundaryList(t, router, auth)
	if len(items) != 2 || items[0].Name != "backup_history.ZIP" || items[0].Format != "logical" ||
		items[1].Name != "portable_backup_history.ZIP" || items[1].Format != "portable-invalid" {
		t.Fatalf("valid historical backup list changed: %+v", items)
	}
	for name, expected := range map[string][]byte{
		"backup_history.ZIP":          logicalData,
		"portable_backup_history.ZIP": portableData,
	} {
		response := backupBoundaryDownload(router, auth, name)
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), expected) {
			t.Fatalf("valid download %s = %d %d bytes, want 200 %d bytes", name, response.Code, response.Body.Len(), len(expected))
		}
	}
	missing := backupBoundaryDownload(router, auth, "backup_missing.zip")
	if missing.Code != http.StatusNotFound || strings.TrimSpace(missing.Body.String()) != `{"error":"backup not found"}` {
		t.Fatalf("missing download = %d %q, want safe 404", missing.Code, missing.Body.String())
	}
}

func backupBoundaryUserRoot(t *testing.T, server *Server, username string) string {
	t.Helper()
	root := filepath.Join(server.cfg.DataDir, "webdav", "users", username)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func backupBoundaryList(t *testing.T, router http.Handler, auth string) []backupBoundaryListItem {
	t.Helper()
	response := backupBoundaryListResponse(router, auth)
	if response.Code != http.StatusOK {
		t.Fatalf("backup list = %d %s", response.Code, response.Body.String())
	}
	var items []backupBoundaryListItem
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func backupBoundaryListResponse(router http.Handler, auth string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/backup/list", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func backupBoundaryDownload(router http.Handler, auth, name string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/backup/download/"+name, nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func backupBoundaryZIP(t *testing.T, name, contents string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
