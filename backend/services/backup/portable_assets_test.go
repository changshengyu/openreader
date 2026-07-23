package backup

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

type portableV2ManifestFixture struct {
	Format  string                           `json:"format"`
	Version int                              `json:"version"`
	Assets  []portableV2ManifestAssetFixture `json:"assets"`
}

type portableV2ManifestAssetFixture struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Extension string `json:"extension"`
	Entry     string `json:"entry"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

func TestPortableBackupV2PackagesOnlyReferencedAssetsWithoutOwnerMetadata(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	libraryDir := filepath.Join(root, "library")
	webdavDir := filepath.Join(root, "webdav")
	database := portableBackupTestDB(t)
	service := New(database, webdavDir, config.Config{DataDir: dataDir, LibraryDir: libraryDir})

	owner := models.User{Username: "portable-asset-owner", PasswordHash: "hash"}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	ownerPrefix := fmt.Sprintf("/uploads/users/%d/backgrounds/", owner.ID)
	firstURL := ownerPrefix + "first.png"
	secondURL := ownerPrefix + "second.png"
	imageData := portableAssetPNG(t)
	for _, assetURL := range []string{firstURL, secondURL} {
		path := filepath.Join(dataDir, "uploads", strings.TrimPrefix(assetURL, "/uploads/"))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, imageData, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	legacyURL := "/uploads/backgrounds/legacy.png"
	settingValue, err := json.Marshal(map[string]any{
		"contentBGImg":     firstURL,
		"customBGImgList":  []string{firstURL, secondURL, legacyURL},
		"customConfigList": []any{map[string]any{"name": "纸张", "contentBGImg": secondURL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.UserSetting{
		UserID: owner.ID,
		Key:    "reader",
		Value:  string(settingValue),
	}).Error; err != nil {
		t.Fatal(err)
	}

	logicalPath, err := service.RunNowForUser(owner.ID, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	logicalEntries, err := sortedPortableEntries(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range logicalEntries {
		if name == "openreader-portable-v2.json" || strings.HasPrefix(name, "appearance-assets/") {
			t.Fatalf("ordinary logical backup unexpectedly contains %q", name)
		}
	}
	logicalSettings := string(portableEntryData(t, logicalPath, "userSettings.json"))
	if !strings.Contains(logicalSettings, firstURL) || strings.Contains(logicalSettings, "openreader-asset://") {
		t.Fatalf("ordinary logical settings changed portable semantics: %s", logicalSettings)
	}

	portablePath, localBooks, err := service.RunPortableForUser(
		owner.ID,
		owner.Username,
		filepath.Join(webdavDir, "users", owner.Username),
	)
	if err != nil {
		t.Fatal(err)
	}
	if localBooks != 0 {
		t.Fatalf("portable localBooks = %d, want 0", localBooks)
	}
	manifestData := portableEntryData(t, portablePath, "openreader-portable-v2.json")
	var manifest portableV2ManifestFixture
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Format != "openreader-portable-backup" || manifest.Version != 2 || len(manifest.Assets) != 1 {
		t.Fatalf("portable v2 manifest = %+v", manifest)
	}
	asset := manifest.Assets[0]
	if asset.ID != "a0001" || asset.Kind != "backgrounds" || asset.Extension != ".png" ||
		asset.Entry != "appearance-assets/a0001.png" || asset.Size != int64(len(imageData)) ||
		len(asset.SHA256) != 64 {
		t.Fatalf("portable v2 asset = %+v", asset)
	}
	manifestText := string(manifestData)
	if strings.Contains(manifestText, owner.Username) ||
		strings.Contains(manifestText, "/users/"+strconv.FormatUint(uint64(owner.ID), 10)+"/") ||
		strings.Contains(manifestText, "first.png") ||
		strings.Contains(manifestText, "second.png") {
		t.Fatalf("portable v2 manifest leaked owner metadata: %s", manifestText)
	}
	if got := portableEntryData(t, portablePath, asset.Entry); string(got) != string(imageData) {
		t.Fatalf("portable v2 asset bytes differ: got=%d want=%d", len(got), len(imageData))
	}
	portableSettings := string(portableEntryData(t, portablePath, "userSettings.json"))
	if strings.Contains(portableSettings, firstURL) || strings.Contains(portableSettings, secondURL) ||
		strings.Count(portableSettings, "openreader-asset://a0001") != 4 ||
		!strings.Contains(portableSettings, legacyURL) {
		t.Fatalf("portable v2 logical placeholder rewrite = %s", portableSettings)
	}
}

func portableAssetPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
