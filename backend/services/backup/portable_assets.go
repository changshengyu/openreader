package backup

import (
	"archive/zip"
	"bytes"
	"context"
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
	file      *os.File
	size      int64
	sha256    string
}

func (s *Service) collectPortableAssetBundle(ctx context.Context, userID uint) (map[string][]byte, []portableAssetInput, int, error) {
	logicalEntries, err := s.portableLogicalEntries(ctx, userID)
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
	complete := false
	defer func() {
		if !complete {
			closePortableAssetInputs(assets)
		}
	}()
	for _, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return nil, nil, 0, err
		}
		reference, err := s.validatePortableAssetReference(ctx, userID, rawURL)
		if err != nil {
			return nil, nil, 0, err
		}
		dedupKey := reference.kind + "\x00" + reference.extension + "\x00" + reference.sha256
		if existing, ok := deduplicated[dedupKey]; ok {
			_ = reference.file.Close()
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
		input := portableAssetInput{manifest: manifest, file: reference.file}
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
	complete = true
	return logicalEntries, assets, len(legacy), nil
}

func (s *Service) portableLogicalEntries(ctx context.Context, userID uint) (map[string][]byte, error) {
	var output bytes.Buffer
	writer := zip.NewWriter(contextWriter{ctx: ctx, writer: &output})
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.writeLogicalEntries(ctx, tx, writer, &userID); err != nil {
			_ = writer.Close()
			return err
		}
		if err := ctx.Err(); err != nil {
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		opened, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(contextReader{ctx: ctx, reader: opened})
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

func (s *Service) validatePortableAssetReference(ctx context.Context, userID uint, rawURL string) (portableAssetReference, error) {
	if err := ctx.Err(); err != nil {
		return portableAssetReference{}, err
	}
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
	store := assetservice.NewStore(s.cfg.DataDir)
	opened, err := store.Open(userID, kind, parts[4])
	if err != nil || opened.Info.Size() <= 0 {
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	if opened.Info.Size() > assetservice.SizeLimitForKind(kind) {
		_ = opened.File.Close()
		return portableAssetReference{}, ErrPortableBackupLimit
	}
	file := opened.File
	validationErr := assetservice.ValidateUpload(contextReader{ctx: ctx, reader: file}, opened.Info.Size(), kind, extension)
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return portableAssetReference{}, err
	}
	if validationErr != nil {
		_ = file.Close()
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	digest, size, err := portableAssetDigestFile(ctx, file, assetservice.SizeLimitForKind(kind))
	if contextErr := ctx.Err(); contextErr != nil {
		_ = file.Close()
		return portableAssetReference{}, contextErr
	}
	if err != nil || size != opened.Info.Size() {
		_ = file.Close()
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return portableAssetReference{}, ErrPortableAssetUnavailable
	}
	return portableAssetReference{
		kind:      kind,
		extension: extension,
		file:      file,
		size:      size,
		sha256:    digest,
	}, nil
}

func writePortableAssetEntry(ctx context.Context, writer *zip.Writer, asset portableAssetInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file := asset.file
	if file == nil {
		return ErrPortableAssetUnavailable
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.manifest.Size ||
		info.Size() > assetservice.SizeLimitForKind(asset.manifest.Kind) {
		return ErrPortableAssetUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ErrPortableAssetUnavailable
	}
	if err := assetservice.ValidateUpload(contextReader{ctx: ctx, reader: file}, info.Size(), asset.manifest.Kind, asset.manifest.Extension); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return ErrPortableAssetUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ErrPortableAssetUnavailable
	}
	entry, err := writer.Create(asset.manifest.Entry)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(entry, hash),
		contextReader{ctx: ctx, reader: io.LimitReader(file, asset.manifest.Size+1)},
	)
	if err := ctx.Err(); err != nil {
		return err
	}
	if copyErr != nil || written != asset.manifest.Size ||
		hex.EncodeToString(hash.Sum(nil)) != asset.manifest.SHA256 {
		return ErrPortableAssetUnavailable
	}
	return nil
}

func portableAssetDigestFile(ctx context.Context, file *os.File, limit int64) (string, int64, error) {
	hash := sha256.New()
	written, err := io.Copy(hash, contextReader{ctx: ctx, reader: io.LimitReader(file, limit+1)})
	if err != nil || written > limit {
		return "", 0, ErrPortableBackupLimit
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func closePortableAssetInputs(assets []portableAssetInput) {
	for _, asset := range assets {
		if asset.file != nil {
			_ = asset.file.Close()
		}
	}
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
