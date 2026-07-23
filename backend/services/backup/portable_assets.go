package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	assetservice "openreader/backend/services/assets"
)

const portableAssetPlaceholderPrefix = "openreader-asset://"

type portableAssetReference struct {
	kind      string
	extension string
	path      string
	size      int64
	sha256    string
}

func (s *Service) collectPortableAssetBundle(userID uint) (map[string][]byte, []portableAssetInput, int, error) {
	logicalEntries, err := s.portableLogicalEntries(userID)
	if err != nil {
		return nil, nil, 0, err
	}
	settingsData, ok := logicalEntries["userSettings.json"]
	if !ok {
		return nil, nil, 0, ErrPortableAssetUnavailable
	}
	shelfData, ok := logicalEntries["bookshelf.json"]
	if !ok {
		return nil, nil, 0, ErrPortableAssetUnavailable
	}

	referenced := make(map[string]struct{})
	legacy := make(map[string]struct{})
	settingsRows, err := collectPortableSettingAssetReferences(settingsData, referenced, legacy)
	if err != nil {
		return nil, nil, 0, err
	}
	shelfRows, err := collectPortableShelfAssetReferences(shelfData, referenced, legacy)
	if err != nil {
		return nil, nil, 0, err
	}

	urls := make([]string, 0, len(referenced))
	for rawURL := range referenced {
		urls = append(urls, rawURL)
	}
	sort.Strings(urls)
	placeholderByURL := make(map[string]string, len(urls))
	deduplicated := make(map[string]portableAssetInput)
	assets := make([]portableAssetInput, 0, len(urls))
	for _, rawURL := range urls {
		reference, err := s.validatePortableAssetReference(userID, rawURL)
		if err != nil {
			return nil, nil, 0, err
		}
		dedupKey := reference.kind + "\x00" + reference.extension + "\x00" + reference.sha256
		if existing, ok := deduplicated[dedupKey]; ok {
			placeholderByURL[rawURL] = portableAssetPlaceholderPrefix + existing.manifest.ID
			continue
		}
		id := fmt.Sprintf("a%04d", len(assets)+1)
		manifest := portableManifestAsset{
			ID:        id,
			Kind:      reference.kind,
			Extension: reference.extension,
			Entry:     "appearance-assets/" + id + reference.extension,
			Size:      reference.size,
			SHA256:    reference.sha256,
		}
		input := portableAssetInput{manifest: manifest, path: reference.path}
		assets = append(assets, input)
		deduplicated[dedupKey] = input
		placeholderByURL[rawURL] = portableAssetPlaceholderPrefix + id
	}

	rewrittenSettings, err := rewritePortableSettingAssets(settingsRows, placeholderByURL)
	if err != nil {
		return nil, nil, 0, err
	}
	rewrittenShelf, err := rewritePortableShelfAssets(shelfRows, placeholderByURL)
	if err != nil {
		return nil, nil, 0, err
	}
	logicalEntries["userSettings.json"] = rewrittenSettings
	logicalEntries["bookshelf.json"] = rewrittenShelf
	return logicalEntries, assets, len(legacy), nil
}

func (s *Service) portableLogicalEntries(userID uint) (map[string][]byte, error) {
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.writeLogicalEntries(tx, writer, &userID); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	})
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		return nil, err
	}
	entries := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(opened)
		closeErr := opened.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		entries[file.Name] = data
	}
	return entries, nil
}

func collectPortableSettingAssetReferences(
	data []byte,
	referenced map[string]struct{},
	legacy map[string]struct{},
) ([]map[string]any, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, ErrPortableAssetUnavailable
	}
	for _, row := range rows {
		value, ok := row["value"].(string)
		if !ok || !json.Valid([]byte(value)) {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			continue
		}
		if err := walkPortableAssetStrings(decoded, func(rawURL string) error {
			return collectPortableAssetURL(rawURL, referenced, legacy)
		}); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func collectPortableShelfAssetReferences(
	data []byte,
	referenced map[string]struct{},
	legacy map[string]struct{},
) ([]map[string]any, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, ErrPortableAssetUnavailable
	}
	for _, row := range rows {
		rawURL, _ := row["customCoverUrl"].(string)
		if err := collectPortableAssetURL(rawURL, referenced, legacy); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func collectPortableAssetURL(rawURL string, referenced, legacy map[string]struct{}) error {
	if rawURL == "" {
		return nil
	}
	if strings.HasPrefix(rawURL, portableAssetPlaceholderPrefix) {
		return ErrPortableAssetUnavailable
	}
	if strings.HasPrefix(rawURL, "/uploads/users/") {
		referenced[rawURL] = struct{}{}
		return nil
	}
	if isPortableLegacyAssetURL(rawURL) {
		legacy[rawURL] = struct{}{}
	}
	return nil
}

func walkPortableAssetStrings(value any, visit func(string) error) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if err := walkPortableAssetStrings(child, visit); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := walkPortableAssetStrings(child, visit); err != nil {
				return err
			}
		}
	case string:
		return visit(typed)
	}
	return nil
}

func rewritePortableSettingAssets(rows []map[string]any, placeholders map[string]string) ([]byte, error) {
	for _, row := range rows {
		value, ok := row["value"].(string)
		if !ok || !json.Valid([]byte(value)) {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			continue
		}
		decoded = rewritePortableAssetStrings(decoded, placeholders)
		encoded, err := json.Marshal(decoded)
		if err != nil {
			return nil, err
		}
		row["value"] = string(encoded)
	}
	return json.MarshalIndent(rows, "", "  ")
}

func rewritePortableShelfAssets(rows []map[string]any, placeholders map[string]string) ([]byte, error) {
	for _, row := range rows {
		rawURL, _ := row["customCoverUrl"].(string)
		if placeholder := placeholders[rawURL]; placeholder != "" {
			row["customCoverUrl"] = placeholder
		}
	}
	return json.MarshalIndent(rows, "", "  ")
}

func rewritePortableAssetStrings(value any, placeholders map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			typed[key] = rewritePortableAssetStrings(child, placeholders)
		}
	case []any:
		for index, child := range typed {
			typed[index] = rewritePortableAssetStrings(child, placeholders)
		}
	case string:
		if placeholder := placeholders[typed]; placeholder != "" {
			return placeholder
		}
	}
	return value
}

func (s *Service) validatePortableAssetReference(userID uint, rawURL string) (portableAssetReference, error) {
	if strings.ContainsAny(rawURL, "?#") || strings.Contains(rawURL, `\`) {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	parts := strings.Split(strings.TrimPrefix(rawURL, "/"), "/")
	if len(parts) != 5 || parts[0] != "uploads" || parts[1] != "users" ||
		parts[2] != strconv.FormatUint(uint64(userID), 10) ||
		!assetservice.IsKindDirectory(parts[3]) ||
		parts[4] == "" || parts[4] != filepath.Base(parts[4]) {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	kind := parts[3]
	extension := strings.ToLower(filepath.Ext(parts[4]))
	if !assetservice.AllowedExtension(kind, extension) {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	root := filepath.Join(s.cfg.DataDir, "uploads", "users", parts[2])
	path := filepath.Join(root, kind, parts[4])
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	if info.Size() > assetservice.SizeLimitForKind(kind) {
		return portableAssetReference{}, ErrPortableBackupLimit
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	relative, err := filepath.Rel(rootResolved, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	file, err := os.Open(resolved)
	if err != nil {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	validationErr := assetservice.ValidateUpload(file, info.Size(), kind, extension)
	_ = file.Close()
	if validationErr != nil {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	digest, size, err := portableAssetDigest(resolved, assetservice.SizeLimitForKind(kind))
	if err != nil || size != info.Size() {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	return portableAssetReference{
		kind:      kind,
		extension: extension,
		path:      resolved,
		size:      size,
		sha256:    digest,
	}, nil
}

func writePortableAssetEntry(writer *zip.Writer, asset portableAssetInput) error {
	file, err := os.Open(asset.path)
	if err != nil {
		return ErrPortableAssetUnavailable
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.manifest.Size ||
		info.Size() > assetservice.SizeLimitForKind(asset.manifest.Kind) {
		_ = file.Close()
		return ErrPortableAssetUnavailable
	}
	if err := assetservice.ValidateUpload(file, info.Size(), asset.manifest.Kind, asset.manifest.Extension); err != nil {
		_ = file.Close()
		return ErrPortableAssetUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return ErrPortableAssetUnavailable
	}
	entry, err := writer.Create(asset.manifest.Entry)
	if err != nil {
		_ = file.Close()
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(entry, hash), io.LimitReader(file, asset.manifest.Size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written != asset.manifest.Size ||
		hex.EncodeToString(hash.Sum(nil)) != asset.manifest.SHA256 {
		return ErrPortableAssetUnavailable
	}
	return nil
}

func portableAssetDigest(path string, limit int64) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil || written > limit {
		return "", 0, ErrPortableBackupLimit
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func isPortableLegacyAssetURL(rawURL string) bool {
	if strings.ContainsAny(rawURL, "?#") || strings.Contains(rawURL, `\`) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(rawURL, "/"), "/")
	return len(parts) == 3 && parts[0] == "uploads" &&
		assetservice.IsKindDirectory(parts[1]) &&
		parts[2] != "" && parts[2] == filepath.Base(parts[2])
}
