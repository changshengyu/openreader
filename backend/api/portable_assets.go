package api

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	assetservice "openreader/backend/services/assets"
)

func portableManifestVersion(name string) (int, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	const prefix = "openreader-portable-v"
	const suffix = ".json"
	if strings.Contains(name, "/") || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if raw == "" || strings.HasPrefix(raw, "0") {
		return 0, false
	}
	version, err := strconv.Atoi(raw)
	return version, err == nil && version > 0
}

func canonicalPortableManifestName(version int) string {
	return fmt.Sprintf("openreader-portable-v%d.json", version)
}

func decodePortableManifest(data []byte) (portableBackupManifest, error) {
	var manifest portableBackupManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return portableBackupManifest{}, errInvalidPortableBackup
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return portableBackupManifest{}, errInvalidPortableBackup
	}
	return manifest, nil
}

func validatePortableAssetManifest(manifest portableBackupManifest, files map[string]*zip.File) error {
	declared := make(map[string]struct{}, len(manifest.Assets))
	digests := make(map[string]struct{}, len(manifest.Assets))
	for index, asset := range manifest.Assets {
		wantID := fmt.Sprintf("a%04d", index+1)
		if asset.ID != wantID || asset.Kind != strings.TrimSpace(asset.Kind) ||
			asset.Kind != strings.ToLower(asset.Kind) ||
			!assetservice.IsKindDirectory(asset.Kind) ||
			!assetservice.AllowedExtension(asset.Kind, asset.Extension) ||
			asset.Extension != strings.TrimSpace(asset.Extension) ||
			asset.Extension != strings.ToLower(asset.Extension) ||
			asset.Size <= 0 || asset.Size > assetservice.SizeLimitForKind(asset.Kind) ||
			len(asset.SHA256) != sha256.Size*2 || asset.SHA256 != strings.ToLower(asset.SHA256) {
			return errInvalidPortableBackup
		}
		if _, err := hex.DecodeString(asset.SHA256); err != nil {
			return errInvalidPortableBackup
		}
		wantEntry := "appearance-assets/" + asset.ID + asset.Extension
		entry, err := normalizeBackupArchivePath(asset.Entry)
		if err != nil || entry != asset.Entry || entry != wantEntry {
			return errInvalidPortableBackup
		}
		key := strings.ToLower(entry)
		file := files[key]
		if file == nil {
			return errInvalidPortableBackup
		}
		actualEntry, err := normalizeBackupArchivePath(file.Name)
		if err != nil || actualEntry != entry {
			return errInvalidPortableBackup
		}
		if _, exists := declared[key]; exists {
			return errInvalidPortableBackup
		}
		digestKey := asset.Kind + "\x00" + asset.Extension + "\x00" + asset.SHA256
		if _, exists := digests[digestKey]; exists {
			return errInvalidPortableBackup
		}
		declared[key] = struct{}{}
		digests[digestKey] = struct{}{}
	}
	for key := range files {
		if strings.HasPrefix(key, "appearance-assets/") {
			if _, ok := declared[key]; !ok {
				return errInvalidPortableBackup
			}
		}
	}
	return nil
}

func copyAndValidatePortableAsset(
	file *zip.File,
	target string,
	manifest portableBackupManifestAsset,
	maxBytes int64,
) error {
	kindLimit := assetservice.SizeLimitForKind(manifest.Kind)
	if file == nil || manifest.Size <= 0 || manifest.Size > maxBytes || manifest.Size > kindLimit {
		return errPortableBackupLimit
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = output.Close()
		if cleanup {
			_ = os.Remove(target)
		}
	}()
	input, err := file.Open()
	if err != nil {
		return errInvalidPortableBackup
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, manifest.Size+1))
	closeInputErr := input.Close()
	if copyErr != nil || closeInputErr != nil || written > manifest.Size {
		return errInvalidPortableBackup
	}
	if written != manifest.Size || hex.EncodeToString(hash.Sum(nil)) != manifest.SHA256 {
		return errInvalidPortableBackup
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	validationInput, err := os.Open(target)
	if err != nil {
		return err
	}
	validationErr := assetservice.ValidateUpload(
		validationInput,
		manifest.Size,
		manifest.Kind,
		manifest.Extension,
	)
	_ = validationInput.Close()
	if validationErr != nil {
		return errInvalidPortableBackup
	}
	cleanup = false
	return nil
}

func (s *Server) allocatePortableAssetTarget(
	userID uint,
	manifest portableBackupManifestAsset,
) (string, string, error) {
	if userID == 0 || strings.TrimSpace(s.cfg.DataDir) == "" {
		return "", "", errInvalidPortableBackup
	}
	dir := filepath.Join(
		s.cfg.DataDir,
		"uploads",
		"users",
		strconv.FormatUint(uint64(userID), 10),
		manifest.Kind,
	)
	for attempt := 0; attempt < 32; attempt++ {
		name := "portable-" + randomHex(12) + manifest.Extension
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			url := fmt.Sprintf("/uploads/users/%d/%s/%s", userID, manifest.Kind, name)
			return url, path, nil
		} else if err != nil {
			return "", "", err
		}
	}
	return "", "", errInvalidPortableBackup
}

func rewritePortableAssetLogicalEntries(entries map[string][]byte, destinations map[string]string) (int, error) {
	settingsData, ok := entries["userSettings.json"]
	if !ok {
		return 0, errInvalidPortableBackup
	}
	shelfData, ok := entries["bookshelf.json"]
	if !ok {
		return 0, errInvalidPortableBackup
	}
	references := make(map[string]int, len(destinations))
	legacy := make(map[string]struct{})

	var settings []map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return 0, errInvalidPortableBackup
	}
	for _, row := range settings {
		for key, field := range row {
			if key == "value" {
				continue
			}
			if portableContainsPlaceholder(field) {
				return 0, errInvalidPortableBackup
			}
		}
		rawValue, ok := row["value"].(string)
		if !ok || !json.Valid([]byte(rawValue)) {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(rawValue), &value); err != nil {
			return 0, errInvalidPortableBackup
		}
		rewritten, err := rewritePortablePlaceholders(value, destinations, references, legacy)
		if err != nil {
			return 0, err
		}
		encoded, err := json.Marshal(rewritten)
		if err != nil {
			return 0, err
		}
		row["value"] = string(encoded)
	}
	rewrittenSettings, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return 0, err
	}

	var shelf []map[string]any
	if err := json.Unmarshal(shelfData, &shelf); err != nil {
		return 0, errInvalidPortableBackup
	}
	for _, row := range shelf {
		for key, field := range row {
			if key == "customCoverUrl" {
				continue
			}
			if portableContainsPlaceholder(field) {
				return 0, errInvalidPortableBackup
			}
		}
		rawURL, _ := row["customCoverUrl"].(string)
		rewritten, err := rewritePortablePlaceholderString(rawURL, destinations, references, legacy)
		if err != nil {
			return 0, err
		}
		if rewritten != rawURL {
			row["customCoverUrl"] = rewritten
		}
	}
	rewrittenShelf, err := json.MarshalIndent(shelf, "", "  ")
	if err != nil {
		return 0, err
	}

	for name, data := range entries {
		if name == "userSettings.json" || name == "bookshelf.json" {
			continue
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return 0, errInvalidPortableBackup
		}
		if portableContainsPlaceholder(value) {
			return 0, errInvalidPortableBackup
		}
	}
	for id := range destinations {
		if references[id] == 0 {
			return 0, errInvalidPortableBackup
		}
	}
	entries["userSettings.json"] = rewrittenSettings
	entries["bookshelf.json"] = rewrittenShelf
	return len(legacy), nil
}

func rewritePortablePlaceholders(
	value any,
	destinations map[string]string,
	references map[string]int,
	legacy map[string]struct{},
) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			rewritten, err := rewritePortablePlaceholders(child, destinations, references, legacy)
			if err != nil {
				return nil, err
			}
			typed[key] = rewritten
		}
	case []any:
		for index, child := range typed {
			rewritten, err := rewritePortablePlaceholders(child, destinations, references, legacy)
			if err != nil {
				return nil, err
			}
			typed[index] = rewritten
		}
	case string:
		return rewritePortablePlaceholderString(typed, destinations, references, legacy)
	}
	return value, nil
}

func rewritePortablePlaceholderString(
	value string,
	destinations map[string]string,
	references map[string]int,
	legacy map[string]struct{},
) (string, error) {
	if strings.HasPrefix(value, portableAssetPlaceholder) {
		id := strings.TrimPrefix(value, portableAssetPlaceholder)
		target, ok := destinations[id]
		if !ok || value != portableAssetPlaceholder+id {
			return "", errInvalidPortableBackup
		}
		references[id]++
		return target, nil
	}
	if portableLegacyAssetURL(value) {
		legacy[value] = struct{}{}
	}
	return value, nil
}

func portableContainsPlaceholder(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if portableContainsPlaceholder(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if portableContainsPlaceholder(child) {
				return true
			}
		}
	case string:
		return strings.HasPrefix(typed, portableAssetPlaceholder)
	}
	return false
}

func portableLegacyAssetURL(rawURL string) bool {
	if strings.ContainsAny(rawURL, "?#") || strings.Contains(rawURL, `\`) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(rawURL, "/"), "/")
	return len(parts) == 3 && parts[0] == "uploads" &&
		assetservice.IsKindDirectory(parts[1]) &&
		parts[2] != "" && parts[2] == filepath.Base(parts[2])
}

func (s *Server) promotePortableAssets(assets []portableStagedAsset, userID uint) ([]string, error) {
	if len(assets) == 0 {
		return nil, nil
	}
	promoted := make([]string, 0, len(assets))
	cleanup := func(err error) ([]string, error) {
		removePortablePromotedAssets(promoted)
		return nil, err
	}
	userRoot := filepath.Join(s.cfg.DataDir, "uploads", "users", strconv.FormatUint(uint64(userID), 10))
	if err := os.MkdirAll(userRoot, 0o700); err != nil {
		return nil, err
	}
	userResolved, err := filepath.EvalSymlinks(userRoot)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		dir := filepath.Dir(asset.finalPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return cleanup(err)
		}
		dirResolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return cleanup(err)
		}
		relative, err := filepath.Rel(userResolved, dirResolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return cleanup(errInvalidPortableBackup)
		}
		if filepath.Base(asset.finalPath) == "." || filepath.Dir(asset.finalPath) != dir {
			return cleanup(errInvalidPortableBackup)
		}
		if _, err := os.Lstat(asset.finalPath); !os.IsNotExist(err) {
			if err == nil {
				err = errInvalidPortableBackup
			}
			return cleanup(err)
		}
		input, err := os.Open(asset.path)
		if err != nil {
			return cleanup(err)
		}
		temporary, err := os.CreateTemp(dir, ".portable-asset-*.tmp")
		if err != nil {
			_ = input.Close()
			return cleanup(err)
		}
		temporaryPath := temporary.Name()
		linked := false
		func() {
			defer func() {
				_ = input.Close()
				_ = temporary.Close()
				_ = os.Remove(temporaryPath)
			}()
			if err = temporary.Chmod(0o600); err != nil {
				return
			}
			var written int64
			written, err = io.Copy(temporary, io.LimitReader(input, asset.manifest.Size+1))
			if err != nil || written != asset.manifest.Size {
				if err == nil {
					err = errInvalidPortableBackup
				}
				return
			}
			if err = temporary.Sync(); err != nil {
				return
			}
			if err = temporary.Close(); err != nil {
				return
			}
			if err = os.Link(temporaryPath, asset.finalPath); err != nil {
				return
			}
			linked = true
		}()
		if err != nil || !linked {
			return cleanup(err)
		}
		promoted = append(promoted, asset.finalPath)
	}
	return promoted, nil
}

func removePortablePromotedAssets(paths []string) {
	for index := len(paths) - 1; index >= 0; index-- {
		_ = os.Remove(paths[index])
	}
}

type portableAssetRestoreJournal struct {
	Version int      `json:"version"`
	UserID  uint     `json:"userId"`
	URLs    []string `json:"urls"`
}

func (s *Server) writePortableAssetRestoreJournal(assets []portableStagedAsset, userID uint) (string, error) {
	if len(assets) == 0 {
		return "", nil
	}
	journal := portableAssetRestoreJournal{
		Version: 1,
		UserID:  userID,
		URLs:    make([]string, 0, len(assets)),
	}
	for _, asset := range assets {
		parsed, err := s.userUploadAsset(asset.finalURL)
		if err != nil || parsed.UserID != userID || parsed.Path != asset.finalPath {
			return "", errInvalidPortableBackup
		}
		journal.URLs = append(journal.URLs, asset.finalURL)
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return "", err
	}
	root := filepath.Join(s.cfg.DataDir, ".portable-restore-journals")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(root, ".portable-assets-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	completed := false
	defer func() {
		_ = temporary.Close()
		if !completed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	finalPath := strings.TrimSuffix(temporaryPath, ".tmp") + ".json"
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return "", err
	}
	completed = true
	return finalPath, nil
}

func (s *Server) cleanupPortableAssetRestoreJournals() {
	root := filepath.Join(s.cfg.DataDir, ".portable-restore-journals")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasPrefix(entry.Name(), ".portable-assets-") ||
			!strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 64*1024+1))
		_ = file.Close()
		if readErr != nil || len(data) > 64*1024 {
			_ = os.Remove(path)
			continue
		}
		var journal portableAssetRestoreJournal
		if err := json.Unmarshal(data, &journal); err != nil ||
			journal.Version != 1 || journal.UserID == 0 ||
			len(journal.URLs) == 0 || len(journal.URLs) > 10_000 {
			_ = os.Remove(path)
			continue
		}
		removable := make([]string, 0, len(journal.URLs))
		valid := true
		for _, rawURL := range journal.URLs {
			asset, err := s.userUploadAsset(rawURL)
			if err != nil || asset.UserID != journal.UserID {
				valid = false
				break
			}
			referenced, err := s.userUploadAssetReferenced(journal.UserID, asset.URL)
			if err != nil {
				valid = false
				break
			}
			if !referenced {
				removable = append(removable, asset.Path)
			}
		}
		if !valid {
			continue
		}
		cleaned := true
		for _, assetPath := range removable {
			if err := os.Remove(assetPath); err != nil && !os.IsNotExist(err) {
				cleaned = false
			}
		}
		if cleaned {
			_ = os.Remove(path)
		}
	}
}

func portableBackupFormatFromFile(path string) string {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "portable-invalid"
	}
	defer reader.Close()
	var manifest *zip.File
	version := 0
	for _, file := range reader.File {
		name, err := normalizeBackupArchivePath(file.Name)
		if err != nil {
			return "portable-invalid"
		}
		current, ok := portableManifestVersion(name)
		if !ok {
			continue
		}
		if name != canonicalPortableManifestName(current) {
			return "portable-invalid"
		}
		if manifest != nil {
			return "portable-invalid"
		}
		manifest = file
		version = current
	}
	if manifest == nil || (version != 1 && version != 2) {
		return "portable-invalid"
	}
	data, err := readPortableZipEntry(manifest, 1024*1024)
	if err != nil {
		return "portable-invalid"
	}
	decoded, err := decodePortableManifest(data)
	if err != nil ||
		decoded.Format != "openreader-portable-backup" || decoded.Version != version {
		return "portable-invalid"
	}
	return fmt.Sprintf("openreader-portable-v%d", version)
}
