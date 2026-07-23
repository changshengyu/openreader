package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestPortableBackupV2RestoresAppearanceAssetsAcrossUserIDs(t *testing.T) {
	sourceRouter, source := setupTestServer(t)
	sourceToken := authHeader(t, sourceRouter)
	var sourceUser models.User
	if err := source.db.Where("username = ?", "testuser").First(&sourceUser).Error; err != nil {
		t.Fatal(err)
	}
	backgroundURL := portableUploadURL(t, sourceRouter, sourceToken, "background", "paper.png", readerAppearancePNG(t, 2, 2))
	fontURL := portableUploadURL(t, sourceRouter, sourceToken, "font", "reading.woff2", readerAppearanceFont("woff2"))
	coverURL := portableUploadURL(t, sourceRouter, sourceToken, "cover", "cover.png", readerAppearancePNG(t, 1, 1))
	legacyURL := "/uploads/backgrounds/legacy-paper.png"

	readerValue, err := json.Marshal(map[string]any{
		"contentBGImg":    backgroundURL,
		"customBGImgList": []string{backgroundURL, legacyURL},
		"customFontsMap":  map[string]string{"hei": fontURL},
		"customConfigList": []any{
			map[string]any{"name": "自定义纸张", "contentBGImg": backgroundURL, "customFontsMap": map[string]string{"hei": fontURL}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.db.Create(&models.UserSetting{
		UserID: sourceUser.ID,
		Key:    "reader",
		Value:  string(readerValue),
	}).Error; err != nil {
		t.Fatal(err)
	}
	bookSource := models.BookSource{Name: "Portable source", BaseURL: "https://portable.example", Enabled: true}
	if err := source.db.Create(&bookSource).Error; err != nil {
		t.Fatal(err)
	}
	sourceBook := models.Book{
		UserID:         sourceUser.ID,
		SourceID:       bookSource.ID,
		Title:          "可移植封面书",
		Author:         "OpenReader",
		URL:            "https://portable.example/book/1",
		CustomCoverURL: coverURL,
		CanUpdate:      true,
	}
	if err := source.db.Create(&sourceBook).Error; err != nil {
		t.Fatal(err)
	}

	trigger := httptest.NewRequest(http.MethodPost, "/api/backup/portable/trigger", nil)
	trigger.Header.Set("Authorization", sourceToken)
	triggerResponse := httptest.NewRecorder()
	sourceRouter.ServeHTTP(triggerResponse, trigger)
	if triggerResponse.Code != http.StatusOK {
		t.Fatalf("portable v2 trigger: expected 200, got %d: %s", triggerResponse.Code, triggerResponse.Body.String())
	}
	var triggerResult struct {
		Name         string `json:"name"`
		Format       string `json:"format"`
		LocalBooks   int    `json:"localBooks"`
		Assets       int    `json:"assets"`
		LegacyAssets int    `json:"legacyAssets"`
	}
	if err := json.Unmarshal(triggerResponse.Body.Bytes(), &triggerResult); err != nil {
		t.Fatal(err)
	}
	if triggerResult.Format != "openreader-portable-v2" || triggerResult.LocalBooks != 0 ||
		triggerResult.Assets != 3 || triggerResult.LegacyAssets != 1 {
		t.Fatalf("portable v2 trigger payload = %+v", triggerResult)
	}
	backupPath := filepath.Join(source.cfg.DataDir, "webdav", triggerResult.Name)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		backupPath = filepath.Join(source.cfg.DataDir, "webdav", "users", sourceUser.Username, triggerResult.Name)
	}
	if format := portableBackupFormatFromFile(backupPath); format != "openreader-portable-v2" {
		t.Fatalf("portable v2 list format = %q", format)
	}
	_, destination := setupTestServer(t)
	filler := models.User{Username: "portable-target-filler", PasswordHash: "hash"}
	target := models.User{Username: "portable-target", PasswordHash: "hash"}
	if err := destination.db.Create(&filler).Error; err != nil {
		t.Fatal(err)
	}
	if err := destination.db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if sourceUser.ID == target.ID {
		t.Fatalf("fixture must restore across user IDs: source=%d target=%d", sourceUser.ID, target.ID)
	}
	result, err := destination.restoreBackupFile(backupPath, target.ID, target.Username)
	if err != nil {
		t.Fatal(err)
	}
	if result["assets"] != 3 || result["legacyAssets"] != 1 || result["books"] != 1 {
		t.Fatalf("portable v2 restore result = %#v", result)
	}

	var restoredSetting models.UserSetting
	if err := destination.db.Where("user_id = ? AND key = ?", target.ID, "reader").First(&restoredSetting).Error; err != nil {
		t.Fatal(err)
	}
	targetPrefix := fmt.Sprintf("/uploads/users/%d/", target.ID)
	sourcePrefix := fmt.Sprintf("/uploads/users/%d/", sourceUser.ID)
	if strings.Contains(restoredSetting.Value, sourcePrefix) ||
		strings.Count(restoredSetting.Value, targetPrefix) != 5 ||
		!strings.Contains(restoredSetting.Value, legacyURL) ||
		strings.Contains(restoredSetting.Value, "openreader-asset://") {
		t.Fatalf("restored reader setting URLs = %s", restoredSetting.Value)
	}
	var restoredBook models.Book
	if err := destination.db.Where("user_id = ? AND url = ?", target.ID, sourceBook.URL).First(&restoredBook).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(restoredBook.CustomCoverURL, targetPrefix+"covers/") ||
		restoredBook.CustomCoverURL == coverURL {
		t.Fatalf("restored custom cover URL = %q", restoredBook.CustomCoverURL)
	}

	restoredURLs := portableManagedURLs(t, restoredSetting.Value)
	restoredURLs = append(restoredURLs, restoredBook.CustomCoverURL)
	seen := map[string]bool{}
	for _, restoredURL := range restoredURLs {
		if seen[restoredURL] {
			continue
		}
		seen[restoredURL] = true
		path := filepath.Join(destination.cfg.DataDir, "uploads", strings.TrimPrefix(restoredURL, "/uploads/"))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("restored asset %q is missing: %v", restoredURL, err)
		}
		switch {
		case strings.Contains(restoredURL, "/backgrounds/"):
			if !bytes.Equal(data, readerAppearancePNG(t, 2, 2)) {
				t.Fatalf("restored background bytes differ")
			}
		case strings.Contains(restoredURL, "/fonts/"):
			if !bytes.Equal(data, readerAppearanceFont("woff2")) {
				t.Fatalf("restored font bytes differ")
			}
		case strings.Contains(restoredURL, "/covers/"):
			if !bytes.Equal(data, readerAppearancePNG(t, 1, 1)) {
				t.Fatalf("restored cover bytes differ")
			}
		default:
			t.Fatalf("unexpected restored asset URL %q", restoredURL)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("restored unique asset URLs = %v", seen)
	}
}

func TestPortableBackupV2RejectsInvalidAssetBeforeDatabaseOrFileMutation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "portable-v2-invalid-target", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	beforeValue := `{"fontSize":20,"contentBGImg":"/uploads/backgrounds/existing.png"}`
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: beforeValue}).Error; err != nil {
		t.Fatal(err)
	}
	invalidAsset := []byte("<html>not a png</html>")
	hash := sha256.Sum256(invalidAsset)
	manifest, err := json.Marshal(map[string]any{
		"format":    "openreader-portable-backup",
		"version":   2,
		"createdAt": time.Now().UTC(),
		"books":     []any{},
		"assets": []any{map[string]any{
			"id": "a0001", "kind": "backgrounds", "extension": ".png",
			"entry": "appearance-assets/a0001.png", "size": len(invalidAsset), "sha256": hex.EncodeToString(hash[:]),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"openreader-portable-v2.json": string(manifest),
		"userSettings.json":           `[{"key":"reader","value":"{\"fontSize\":24,\"contentBGImg\":\"openreader-asset://a0001\"}"}]`,
		"bookshelf.json":              `[]`,
		"appearance-assets/a0001.png": string(invalidAsset),
	})
	path := filepath.Join(t.TempDir(), "invalid-portable-v2.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if format := portableBackupFormatFromFile(path); format != "openreader-portable-v2" {
		t.Fatalf("portable v2 list format = %q", format)
	}
	if _, err := server.restoreBackupFile(path, user.ID, user.Username); err == nil {
		t.Fatal("invalid portable v2 asset unexpectedly restored")
	}
	var setting models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "reader").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != beforeValue {
		t.Fatalf("invalid portable v2 mutated setting: got %s want %s", setting.Value, beforeValue)
	}
	assertPortableAssetRootEmpty(t, server, user.ID)
}

func TestPortableBackupV2RejectsMissingAndCrossOwnerReferencesWithoutPackage(t *testing.T) {
	tests := []struct {
		name       string
		reference  func(models.User, models.User) string
		writeAsset bool
	}{
		{
			name: "missing caller asset",
			reference: func(owner, _ models.User) string {
				return fmt.Sprintf("/uploads/users/%d/backgrounds/missing.png", owner.ID)
			},
		},
		{
			name: "cross owner asset",
			reference: func(_, other models.User) string {
				return fmt.Sprintf("/uploads/users/%d/backgrounds/foreign.png", other.ID)
			},
			writeAsset: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			token := authHeader(t, router)
			var owner models.User
			if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
				t.Fatal(err)
			}
			other := models.User{Username: "portable-other-owner", PasswordHash: "hash"}
			if err := server.db.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			reference := tt.reference(owner, other)
			if tt.writeAsset {
				path := filepath.Join(server.cfg.DataDir, "uploads", strings.TrimPrefix(reference, "/uploads/"))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, readerAppearancePNG(t, 1, 1), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			value, err := json.Marshal(map[string]string{"contentBGImg": reference})
			if err != nil {
				t.Fatal(err)
			}
			if err := server.db.Create(&models.UserSetting{UserID: owner.ID, Key: "reader", Value: string(value)}).Error; err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/backup/portable/trigger", nil)
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusConflict ||
				!strings.Contains(response.Body.String(), "custom asset unavailable") ||
				strings.Contains(response.Body.String(), reference) {
				t.Fatalf("portable invalid reference: expected safe 409, got %d: %s", response.Code, response.Body.String())
			}
			err = filepath.WalkDir(filepath.Join(server.cfg.DataDir, "webdav"), func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), "portable_backup_") {
					t.Fatalf("rejected portable trigger left package %s", path)
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}
}

func TestPortableBackupV2DatabaseFailureRemovesPromotedAssetsAndRollsBackRows(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "portable-v2-db-failure", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	beforeValue := `{"fontSize":20}`
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: beforeValue}).Error; err != nil {
		t.Fatal(err)
	}
	assetData := readerAppearancePNG(t, 1, 1)
	hash := sha256.Sum256(assetData)
	manifest, err := json.Marshal(map[string]any{
		"format": "openreader-portable-backup", "version": 2, "createdAt": time.Now().UTC(), "books": []any{},
		"assets": []any{map[string]any{
			"id": "a0001", "kind": "backgrounds", "extension": ".png",
			"entry": "appearance-assets/a0001.png", "size": len(assetData), "sha256": hex.EncodeToString(hash[:]),
		}},
		"legacyAssets": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"openreader-portable-v2.json": string(manifest),
		"userSettings.json":           `[{"key":"reader","value":"{\"fontSize\":24,\"contentBGImg\":\"openreader-asset://a0001\"}"}]`,
		"bookshelf.json":              `[]`,
		"appearance-assets/a0001.png": string(assetData),
	})
	path := filepath.Join(t.TempDir(), "db-failure-portable-v2.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if format := portableBackupFormatFromFile(path); format != "openreader-portable-v2" {
		t.Fatalf("portable v2 list format = %q", format)
	}
	callbackName := "portable-v2-force-user-setting-failure"
	failSettings := func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "user_settings" {
			tx.AddError(errors.New("forced user setting failure"))
		}
	}
	if err := server.db.Callback().Create().Before("gorm:create").Register(callbackName, failSettings); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Callback().Update().Before("gorm:update").Register(callbackName, failSettings); err != nil {
		t.Fatal(err)
	}
	if _, err := server.restoreBackupFile(path, user.ID, user.Username); err == nil {
		t.Fatal("forced database failure unexpectedly restored portable v2")
	}
	var setting models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "reader").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != beforeValue {
		t.Fatalf("database failure did not roll back settings: got %s want %s", setting.Value, beforeValue)
	}
	assertPortableAssetRootEmpty(t, server, user.ID)
}

func TestPortableAssetRestoreJournalRemovesOnlyUnreferencedCrashFiles(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "portable-journal-user", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	makeAsset := func(name string) (string, string) {
		url := fmt.Sprintf("/uploads/users/%d/backgrounds/%s", user.ID, name)
		path := filepath.Join(server.cfg.DataDir, "uploads", strings.TrimPrefix(url, "/uploads/"))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, readerAppearancePNG(t, 1, 1), 0o600); err != nil {
			t.Fatal(err)
		}
		return url, path
	}

	orphanURL, orphanPath := makeAsset("orphan.png")
	orphanJournal, err := server.writePortableAssetRestoreJournal([]portableStagedAsset{{
		finalURL: orphanURL, finalPath: orphanPath,
	}}, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	server.cleanupPortableAssetRestoreJournals()
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("startup cleanup retained unreferenced crash file: %v", err)
	}
	if _, err := os.Stat(orphanJournal); !os.IsNotExist(err) {
		t.Fatalf("startup cleanup retained completed orphan journal: %v", err)
	}

	referencedURL, referencedPath := makeAsset("referenced.png")
	value, err := json.Marshal(map[string]string{"contentBGImg": referencedURL})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: string(value)}).Error; err != nil {
		t.Fatal(err)
	}
	referencedJournal, err := server.writePortableAssetRestoreJournal([]portableStagedAsset{{
		finalURL: referencedURL, finalPath: referencedPath,
	}}, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	server.cleanupPortableAssetRestoreJournals()
	if _, err := os.Stat(referencedPath); err != nil {
		t.Fatalf("startup cleanup removed a committed referenced asset: %v", err)
	}
	if _, err := os.Stat(referencedJournal); !os.IsNotExist(err) {
		t.Fatalf("startup cleanup retained completed referenced journal: %v", err)
	}
}

func TestPortableBackupV1RemainsRestorableWithoutInterpretingAssetLikeStrings(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "portable-v1-target", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	bookData := []byte("第一章\nportable v1 正文")
	hash := sha256.Sum256(bookData)
	manifest, err := json.Marshal(map[string]any{
		"format":  "openreader-portable-backup",
		"version": 1,
		"books": []any{map[string]any{
			"bookUrl": "local://portable-v1", "title": "Portable v1", "author": "OpenReader",
			"extension": ".txt", "entry": "local-books/b0001/original.txt",
			"size": len(bookData), "sha256": hex.EncodeToString(hash[:]),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyManagedURL := "/uploads/users/999/backgrounds/legacy-v1.png"
	archive := makeBackupRestoreZIP(t, map[string]string{
		"openreader-portable-v1.json":    string(manifest),
		"userSettings.json":              `[{"key":"reader","value":"{\"contentBGImg\":\"` + legacyManagedURL + `\"}"}]`,
		"bookshelf.json":                 `[{"title":"Portable v1","author":"OpenReader","url":"local://portable-v1","origin":"loc_book","sourceId":0}]`,
		"local-books/b0001/original.txt": string(bookData),
	})
	path := filepath.Join(t.TempDir(), "portable-v1.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if format := portableBackupFormatFromFile(path); format != "openreader-portable-v1" {
		t.Fatalf("portable v1 list format = %q", format)
	}
	result, err := server.restoreBackupFile(path, user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if result["localBooks"] != 1 || result["assets"] != 0 || result["legacyAssets"] != 0 {
		t.Fatalf("portable v1 restore result = %#v", result)
	}
	var setting models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "reader").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(setting.Value, legacyManagedURL) {
		t.Fatalf("portable v1 asset-like string was interpreted: %s", setting.Value)
	}
	var book models.Book
	if err := server.db.Where("user_id = ? AND url = ?", user.ID, "local://portable-v1").First(&book).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.LibraryDir, book.OriginalFile)); err != nil {
		t.Fatalf("portable v1 archive was not restored: %v", err)
	}
}

func TestUnknownPortableVersionFailsClosedBeforeLogicalMutation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "portable-v3-target", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	beforeValue := `{"fontSize":20}`
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: beforeValue}).Error; err != nil {
		t.Fatal(err)
	}
	archive := makeBackupRestoreZIP(t, map[string]string{
		"openreader-portable-v3.json": `{"format":"openreader-portable-backup","version":3,"books":[],"assets":[]}`,
		"userSettings.json":           `[{"key":"reader","value":"{\"fontSize\":99}"}]`,
		"bookshelf.json":              `[]`,
	})
	path := filepath.Join(t.TempDir(), "future-portable.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if format := portableBackupFormatFromFile(path); format != "portable-invalid" {
		t.Fatalf("future portable list format = %q", format)
	}
	if _, err := server.restoreBackupFile(path, user.ID, user.Username); err == nil {
		t.Fatal("future portable version unexpectedly fell through to logical restore")
	}
	var setting models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "reader").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != beforeValue {
		t.Fatalf("future portable version mutated setting: got %s want %s", setting.Value, beforeValue)
	}
}

func TestPortableV2RejectsNonCanonicalManifestAndUnknownFieldsBeforeMutation(t *testing.T) {
	tests := []struct {
		name         string
		manifestName string
		manifest     map[string]any
	}{
		{
			name:         "non canonical manifest case",
			manifestName: "OpenReader-portable-v2.json",
			manifest: map[string]any{
				"format": "openreader-portable-backup", "version": 2,
				"createdAt": time.Now().UTC(), "books": []any{}, "assets": []any{},
			},
		},
		{
			name:         "unknown manifest field",
			manifestName: "openreader-portable-v2.json",
			manifest: map[string]any{
				"format": "openreader-portable-backup", "version": 2,
				"createdAt": time.Now().UTC(), "books": []any{}, "assets": []any{},
				"sourceUserId": 99,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, server := setupTestServer(t)
			user := models.User{Username: "portable-v2-strict-target", PasswordHash: "hash"}
			if err := server.db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			beforeValue := `{"fontSize":20}`
			if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "reader", Value: beforeValue}).Error; err != nil {
				t.Fatal(err)
			}
			manifest, err := json.Marshal(tt.manifest)
			if err != nil {
				t.Fatal(err)
			}
			archive := makeBackupRestoreZIP(t, map[string]string{
				tt.manifestName:     string(manifest),
				"userSettings.json": `[{"key":"reader","value":"{\"fontSize\":99}"}]`,
				"bookshelf.json":    `[]`,
			})
			path := filepath.Join(t.TempDir(), "strict-portable-v2.zip")
			if err := os.WriteFile(path, archive, 0o600); err != nil {
				t.Fatal(err)
			}
			if format := portableBackupFormatFromFile(path); format != "portable-invalid" {
				t.Fatalf("strict portable list format = %q", format)
			}
			if _, err := server.restoreBackupFile(path, user.ID, user.Username); err == nil {
				t.Fatal("non-canonical portable v2 unexpectedly restored")
			}
			var setting models.UserSetting
			if err := server.db.Where("user_id = ? AND key = ?", user.ID, "reader").First(&setting).Error; err != nil {
				t.Fatal(err)
			}
			if setting.Value != beforeValue {
				t.Fatalf("strict portable v2 mutated setting: got %s want %s", setting.Value, beforeValue)
			}
		})
	}
}

func portableUploadURL(t *testing.T, router http.Handler, token, kind, filename string, data []byte) string {
	t.Helper()
	response := uploadReaderAppearanceAsset(t, router, token, kind, filename, data)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload %s: expected 201, got %d: %s", kind, response.Code, response.Body.String())
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.URL
}

func portableManagedURLs(t *testing.T, settingValue string) []string {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(settingValue), &value); err != nil {
		t.Fatal(err)
	}
	var urls []string
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case string:
			if strings.HasPrefix(typed, "/uploads/users/") {
				urls = append(urls, typed)
			}
		}
	}
	walk(value)
	return urls
}

func assertPortableAssetRootEmpty(t *testing.T, server *Server, userID uint) {
	t.Helper()
	root := filepath.Join(server.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(userID), 10))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			t.Fatalf("failed portable restore left asset file %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
