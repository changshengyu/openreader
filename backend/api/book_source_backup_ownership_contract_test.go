package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/models"
	"openreader/backend/services/booksources"
)

func TestP2S4BackupExportsOnlyOwnerActiveSources(t *testing.T) {
	_, server := setupTestServer(t)
	owner := createBackupOwnershipUser(t, server, "backup-source-owner")
	other := createBackupOwnershipUser(t, server, "backup-source-other")
	uninitialized := createBackupOwnershipUser(t, server, "backup-source-new")
	empty := createBackupOwnershipUser(t, server, "backup-source-empty")
	sources := booksources.New(server.db)

	ownerSource, err := sources.Create(owner.ID, models.BookSource{
		Name: "用户甲活动源", BaseURL: "https://backup-owner.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sources.Create(other.ID, models.BookSource{
		Name: "用户乙私有源", BaseURL: "https://backup-other.example", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	detached, err := sources.Create(owner.ID, models.BookSource{
		Name: "用户甲已分离源", BaseURL: "https://backup-detached.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND source_id = ?", owner.ID, detached.ID).
		Update("detached", true).Error; err != nil {
		t.Fatal(err)
	}

	ownerPath, err := server.backupSvc.RunNowForUser(owner.ID, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	ownerEntries := readFixedBaselineBackupEntries(t, ownerPath)
	exported := decodeBackupOwnershipSources(t, ownerEntries["bookSource.json"])
	if len(exported) != 1 || exported[0].ID != 0 ||
		exported[0].Name != ownerSource.Name || exported[0].BaseURL != ownerSource.BaseURL {
		t.Fatalf("owner backup sources = %+v, want only active owner source", exported)
	}

	uninitializedPath, err := server.backupSvc.RunNowForUser(uninitialized.ID, uninitialized.Username)
	if err != nil {
		t.Fatal(err)
	}
	uninitializedEntries := readFixedBaselineBackupEntries(t, uninitializedPath)
	if _, exists := uninitializedEntries["bookSource.json"]; exists {
		t.Fatalf("uninitialized namespace must omit bookSource.json: %v", fixedBaselineEntryNames(uninitializedEntries))
	}
	var namespaceCount int64
	if err := server.db.Model(&models.BookSourceNamespace{}).
		Where("user_id = ?", uninitialized.ID).
		Count(&namespaceCount).Error; err != nil {
		t.Fatal(err)
	}
	if namespaceCount != 0 {
		t.Fatalf("backup initialized source namespace: %d", namespaceCount)
	}

	if err := server.db.Create(&models.BookSourceNamespace{UserID: empty.ID}).Error; err != nil {
		t.Fatal(err)
	}
	emptyPath, err := server.backupSvc.RunNowForUser(empty.ID, empty.Username)
	if err != nil {
		t.Fatal(err)
	}
	emptyEntries := readFixedBaselineBackupEntries(t, emptyPath)
	emptySources, exists := emptyEntries["bookSource.json"]
	if !exists {
		t.Fatalf("explicit empty namespace must include bookSource.json: %v", fixedBaselineEntryNames(emptyEntries))
	}
	if decoded := decodeBackupOwnershipSources(t, emptySources); len(decoded) != 0 {
		t.Fatalf("explicit empty namespace exported sources: %+v", decoded)
	}
}

func TestP2S4BackupFailsClosedForCrossOwnerBookSourceReferences(t *testing.T) {
	_, server := setupTestServer(t)
	owner := createBackupOwnershipUser(t, server, "backup-corrupt-owner")
	other := createBackupOwnershipUser(t, server, "backup-corrupt-other")
	foreign, err := booksources.New(server.db).Create(other.ID, models.BookSource{
		Name: "不能泄露的乙源", BaseURL: "https://foreign-source.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: owner.ID, SourceID: foreign.ID, Title: "损坏跨用户引用",
		URL: foreign.BaseURL + "/book/1", Variable: `{"secret":"owner-b"}`,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{
		BookID: book.ID, Index: 0, Title: "第一章",
		URL: foreign.BaseURL + "/chapter/1", Variable: `{"chapterSecret":"owner-b"}`,
	}).Error; err != nil {
		t.Fatal(err)
	}

	backupPath, err := server.backupSvc.RunNowForUser(owner.ID, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	entries := readFixedBaselineBackupEntries(t, backupPath)
	shelf := string(entries["bookshelf.json"])
	if strings.Contains(shelf, foreign.Name) ||
		strings.Contains(shelf, `"origin": "`+foreign.BaseURL+`"`) ||
		strings.Contains(shelf, "owner-b") {
		t.Fatalf("cross-owner source metadata leaked into bookshelf.json: %s", shelf)
	}
	if data, exists := entries["chapterVariables.json"]; exists {
		t.Fatalf("cross-owner chapter variables leaked into backup: %s", data)
	}
}

func TestP2S4AdminTriggerKeepsLegacyRootButFiltersAdminContent(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	var admin models.User
	if err := server.db.Where("username = ?", "testuser").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&admin).Updates(map[string]any{
		"role": "admin", "can_edit_sources": true, "can_access_store": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	other := createBackupOwnershipUser(t, server, "backup-trigger-other")
	sources := booksources.New(server.db)
	if _, err := sources.Create(admin.ID, models.BookSource{
		Name: "管理员自己的源", BaseURL: "https://admin-backup.example", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sources.Create(other.ID, models.BookSource{
		Name: "其他账号私有源", BaseURL: "https://other-backup.example", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.UserSetting{
		UserID: other.ID, Key: "shelf", Value: `{"private":"other-user"}`,
	}).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/backup/trigger", nil)
	request.Header.Set("Authorization", adminAuth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("admin trigger = %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Name == "" {
		t.Fatalf("admin trigger response missing backup name: %s", response.Body.String())
	}
	backupPath := filepath.Join(server.cfg.DataDir, "webdav", result.Name)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("administrator backup left the legacy root: %v", err)
	}
	entries := readFixedBaselineBackupEntries(t, backupPath)
	exported := decodeBackupOwnershipSources(t, entries["bookSource.json"])
	if len(exported) != 1 || exported[0].Name != "管理员自己的源" {
		t.Fatalf("administrator backup sources = %+v", exported)
	}
	if strings.Contains(string(entries["userSettings.json"]), "other-user") {
		t.Fatalf("administrator backup exported another user's settings: %s", entries["userSettings.json"])
	}
}

func TestP2S4RestoreReconcilesOnlyTargetSourceNamespace(t *testing.T) {
	_, server := setupTestServer(t)
	owner := createBackupOwnershipUser(t, server, "restore-source-owner")
	other := createBackupOwnershipUser(t, server, "restore-source-other")
	sources := booksources.New(server.db)
	oldOwnerSource, err := sources.Create(owner.ID, models.BookSource{
		Name: "甲旧源", BaseURL: "https://restore-owner-old.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherSource, err := sources.Create(other.ID, models.BookSource{
		Name: "乙私有源", BaseURL: "https://restore-other.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Book{
		UserID: owner.ID, SourceID: oldOwnerSource.ID,
		Title: "仍使用旧源的书", URL: oldOwnerSource.BaseURL + "/book/1",
	}).Error; err != nil {
		t.Fatal(err)
	}

	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookSource.json": `[{
			"bookSourceName":"甲恢复源",
			"bookSourceUrl":"https://restore-owner-new.example",
			"enabled":true
		}]`,
	})
	result, err := server.restoreLegadoBackupData(archive, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result["sources"] != 1 {
		t.Fatalf("restored source count = %#v", result)
	}
	if result["sourceDetached"] != 1 {
		t.Fatalf("used source detach diagnostics = %#v", result)
	}
	ownerActive, err := sources.ListExistingActive(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerActive) != 1 ||
		ownerActive[0].Name != "甲恢复源" ||
		ownerActive[0].BaseURL != "https://restore-owner-new.example" {
		t.Fatalf("owner active sources after restore = %+v", ownerActive)
	}
	var oldAssociation models.UserBookSource
	if err := server.db.Where("user_id = ? AND source_id = ?", owner.ID, oldOwnerSource.ID).
		First(&oldAssociation).Error; err != nil {
		t.Fatal(err)
	}
	if !oldAssociation.Detached {
		t.Fatalf("used source missing from archive must remain detached: %+v", oldAssociation)
	}
	otherActive, err := sources.ListExistingActive(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherActive) != 1 || otherActive[0].ID != otherSource.ID {
		t.Fatalf("other user sources changed by owner restore: %+v", otherActive)
	}
}

func TestP2S4RestoreCopyOnWritesSharedSourceSnapshots(t *testing.T) {
	_, server := setupTestServer(t)
	owner := createBackupOwnershipUser(t, server, "restore-cow-owner")
	other := createBackupOwnershipUser(t, server, "restore-cow-other")
	sources := booksources.New(server.db)
	shared, err := sources.Create(owner.ID, models.BookSource{
		Name: "共享旧源", BaseURL: "https://restore-shared.example", Charset: "utf-8", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSourceNamespace{UserID: other.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.UserBookSource{UserID: other.ID, SourceID: shared.ID}).Error; err != nil {
		t.Fatal(err)
	}
	ownerBook := models.Book{
		UserID: owner.ID, SourceID: shared.ID, Title: "甲共享源书",
		URL: shared.BaseURL + "/owner",
	}
	otherBook := models.Book{
		UserID: other.ID, SourceID: shared.ID, Title: "乙共享源书",
		URL: shared.BaseURL + "/other",
	}
	if err := server.db.Create(&ownerBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&otherBook).Error; err != nil {
		t.Fatal(err)
	}

	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookSource.json": `[{
			"bookSourceName":"甲恢复后的共享源",
			"bookSourceUrl":"https://restore-shared.example",
			"charset":"gbk",
			"enabled":true
		}]`,
	})
	if _, err := server.restoreLegadoBackupData(archive, owner.ID); err != nil {
		t.Fatal(err)
	}
	ownerActive, err := sources.ListExistingActive(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerActive) != 1 || ownerActive[0].ID == shared.ID ||
		ownerActive[0].Name != "甲恢复后的共享源" || ownerActive[0].Charset != "gbk" {
		t.Fatalf("owner restore did not copy-on-write shared source: %+v", ownerActive)
	}
	otherActive, err := sources.ListExistingActive(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherActive) != 1 || otherActive[0].ID != shared.ID ||
		otherActive[0].Name != "共享旧源" || otherActive[0].Charset != "utf-8" {
		t.Fatalf("owner restore changed other user's shared snapshot: %+v", otherActive)
	}
	if err := server.db.First(&ownerBook, ownerBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.First(&otherBook, otherBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ownerBook.SourceID != ownerActive[0].ID || otherBook.SourceID != shared.ID {
		t.Fatalf("copy-on-write book references: owner=%d other=%d shared=%d", ownerBook.SourceID, otherBook.SourceID, shared.ID)
	}
}

func TestP2S4RestoreExplicitEmptySourceListAndMissingArtifactDiffer(t *testing.T) {
	t.Run("explicit empty clears only active sources", func(t *testing.T) {
		_, server := setupTestServer(t)
		owner := createBackupOwnershipUser(t, server, "restore-empty-owner")
		other := createBackupOwnershipUser(t, server, "restore-empty-other")
		sources := booksources.New(server.db)
		if _, err := sources.Create(owner.ID, models.BookSource{
			Name: "应被清空的甲源", BaseURL: "https://restore-empty-owner.example", Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
		otherSource, err := sources.Create(other.ID, models.BookSource{
			Name: "必须保留的乙源", BaseURL: "https://restore-empty-other.example", Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ownerClient := server.hub.AddClient(owner.ID, nil)
		otherClient := server.hub.AddClient(other.ID, nil)
		defer server.hub.RemoveClient(ownerClient)
		defer server.hub.RemoveClient(otherClient)

		result, err := server.restoreLegadoBackupData(
			makeBackupRestoreZIP(t, map[string]string{"bookSource.json": `[]`}),
			owner.ID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result["sources"] != 0 || result["sourceRemoved"] != 1 {
			t.Fatalf("explicit empty restore diagnostics = %#v", result)
		}
		ownerActive, err := sources.ListExistingActive(owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(ownerActive) != 0 {
			t.Fatalf("explicit empty source restore kept owner active sources: %+v", ownerActive)
		}
		otherActive, err := sources.ListExistingActive(other.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(otherActive) != 1 || otherActive[0].ID != otherSource.ID {
			t.Fatalf("explicit empty owner restore changed other user: %+v", otherActive)
		}
		if !queuedPayloadContains(ownerClient.Send, `"type":"sources_update"`) {
			t.Fatal("explicit empty restore did not notify the target user")
		}
		if queuedPayloadContains(otherClient.Send, `"type":"sources_update"`) {
			t.Fatal("explicit empty restore notified another user")
		}
	})

	t.Run("missing artifact does not initialize namespace", func(t *testing.T) {
		_, server := setupTestServer(t)
		owner := createBackupOwnershipUser(t, server, "restore-missing-owner")
		if _, err := server.restoreLegadoBackupData(
			makeBackupRestoreZIP(t, map[string]string{
				"userSettings.json": `[{"key":"shelf","value":"{}"}]`,
			}),
			owner.ID,
		); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := server.db.Model(&models.BookSourceNamespace{}).
			Where("user_id = ?", owner.ID).
			Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("restore without source artifact initialized namespace: %d", count)
		}
	})
}

func TestP2S4BookshelfRestoreDoesNotBindAnotherUsersSource(t *testing.T) {
	_, server := setupTestServer(t)
	owner := createBackupOwnershipUser(t, server, "restore-shelf-owner")
	other := createBackupOwnershipUser(t, server, "restore-shelf-other")
	foreign, err := booksources.New(server.db).Create(other.ID, models.BookSource{
		Name: "乙同名私有源", BaseURL: "https://restore-shelf-foreign.example", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"bookshelf.json": `[{
			"name":"不能绑定乙源的书",
			"bookUrl":"https://restore-shelf.example/book/1",
			"sourceName":"乙同名私有源",
			"origin":"https://restore-shelf-foreign.example",
			"variable":"{\"secret\":\"must-clear\"}"
		}]`,
	})
	if _, err := server.restoreLegadoBackupData(archive, owner.ID); err != nil {
		t.Fatal(err)
	}
	var book models.Book
	if err := server.db.Where("user_id = ? AND title = ?", owner.ID, "不能绑定乙源的书").
		First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if book.SourceID != 0 || book.Variable != "" {
		t.Fatalf("restored owner book bound foreign source %d (%d): %+v", foreign.ID, book.SourceID, book)
	}
}

func createBackupOwnershipUser(t *testing.T, server *Server, username string) models.User {
	t.Helper()
	user := models.User{
		Username: username, PasswordHash: "hash",
		CanEditSources: true, CanAccessStore: true,
	}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func decodeBackupOwnershipSources(t *testing.T, data []byte) []models.BookSource {
	t.Helper()
	if len(data) == 0 {
		t.Fatal("bookSource.json is missing or empty")
	}
	sources, err := decodeBookSources(data)
	if err != nil {
		t.Fatal(err)
	}
	return sources
}
