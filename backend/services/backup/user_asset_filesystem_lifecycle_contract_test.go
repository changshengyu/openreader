package backup

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestPortableBackupRejectsAssetThroughCallerRootSymlink(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	libraryDir := filepath.Join(root, "library")
	webdavDir := filepath.Join(root, "webdav")
	database := portableBackupTestDB(t)
	service := New(database, webdavDir, config.Config{DataDir: dataDir, LibraryDir: libraryDir})
	owner := models.User{Username: "portable-rooted-asset", PasswordHash: "hash"}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}

	uploadsRoot := filepath.Join(dataDir, "uploads")
	usersRoot := filepath.Join(uploadsRoot, "users")
	if err := os.MkdirAll(usersRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-user")
	outsideFile := filepath.Join(outside, "backgrounds", "paper.png")
	if err := os.MkdirAll(filepath.Dir(outsideFile), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	callerRoot := filepath.Join(usersRoot, strconv.FormatUint(uint64(owner.ID), 10))
	if err := os.Symlink(outside, callerRoot); err != nil {
		t.Fatal(err)
	}
	assetURL := fmt.Sprintf("/uploads/users/%d/backgrounds/paper.png", owner.ID)
	settingValue, err := json.Marshal(map[string]any{"contentBGImg": assetURL})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.UserSetting{UserID: owner.ID, Key: "reader", Value: string(settingValue)}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := service.RunPortableV2ForUser(owner.ID, owner.Username, filepath.Join(webdavDir, "users", owner.Username))
	if err == nil {
		t.Fatalf("portable backup accepted caller-root symlink: %+v", result)
	}
	if result.Path != "" {
		if _, statErr := os.Stat(result.Path); !os.IsNotExist(statErr) {
			t.Fatalf("rejected portable backup left package %q: %v", result.Path, statErr)
		}
	}
}
