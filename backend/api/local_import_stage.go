package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"openreader/backend/services/localbook"
)

const localImportStageLifetime = 24 * time.Hour
const localImportStageCleanupInterval = time.Hour

const defaultMaxLocalImportBytes int64 = 128 * 1024 * 1024

var (
	errInvalidLocalImportToken = errors.New("invalid or expired local import token")
	errLocalImportTooLarge     = errors.New("local book exceeds maximum import size")
)

type localImportStageMetadata struct {
	FileName  string    `json:"fileName"`
	Extension string    `json:"extension"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) stageLocalImport(userID uint, fileName string, extension string, data []byte) (string, error) {
	if int64(len(data)) > s.maxLocalImportBytes() {
		return "", errLocalImportTooLarge
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)
	dir := s.localImportStageDir(userID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	cleanupLocalImportStageDir(dir, time.Now())

	metadata := localImportStageMetadata{
		FileName:  filepath.Base(fileName),
		Extension: strings.ToLower(strings.TrimSpace(extension)),
		CreatedAt: time.Now().UTC(),
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	dataPath, metadataPath := localImportStagePaths(dir, token)
	if err := os.WriteFile(dataPath, data, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(metadataPath, encoded, 0o600); err != nil {
		_ = os.Remove(dataPath)
		return "", err
	}
	return token, nil
}

func (s *Server) loadStagedLocalImport(userID uint, token string) (localImportStageMetadata, []byte, error) {
	if !validLocalImportToken(token) {
		return localImportStageMetadata{}, nil, errInvalidLocalImportToken
	}
	dir := s.localImportStageDir(userID)
	dataPath, metadataPath := localImportStagePaths(dir, token)
	encoded, err := os.ReadFile(metadataPath)
	if err != nil {
		return localImportStageMetadata{}, nil, errInvalidLocalImportToken
	}
	var metadata localImportStageMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil ||
		metadata.CreatedAt.IsZero() ||
		time.Since(metadata.CreatedAt) > localImportStageLifetime {
		s.removeStagedLocalImport(userID, token)
		return localImportStageMetadata{}, nil, errInvalidLocalImportToken
	}
	data, err := s.readBoundedLocalImportFile(dataPath)
	if err != nil {
		s.removeStagedLocalImport(userID, token)
		return localImportStageMetadata{}, nil, errInvalidLocalImportToken
	}
	return metadata, data, nil
}

func (s *Server) maxLocalImportBytes() int64 {
	if s.cfg.MaxImportBytes > 0 {
		return s.cfg.MaxImportBytes
	}
	return defaultMaxLocalImportBytes
}

func (s *Server) readBoundedLocalImport(reader io.Reader) ([]byte, error) {
	limit := s.maxLocalImportBytes()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errLocalImportTooLarge
	}
	return data, nil
}

func (s *Server) copyBoundedLocalImport(destination io.Writer, source io.Reader) error {
	limit := s.maxLocalImportBytes()
	written, err := io.Copy(destination, io.LimitReader(source, limit+1))
	if err != nil {
		return err
	}
	if written > limit {
		return errLocalImportTooLarge
	}
	return nil
}

func (s *Server) readBoundedLocalImportFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return s.readBoundedLocalImport(file)
}

func (s *Server) removeStagedLocalImport(userID uint, token string) {
	if !validLocalImportToken(token) {
		return
	}
	dataPath, metadataPath := localImportStagePaths(s.localImportStageDir(userID), token)
	_ = os.Remove(dataPath)
	_ = os.Remove(metadataPath)
	_ = os.Remove(localImportPreparedStagePath(s.localImportStageDir(userID), token))
}

func (s *Server) localImportStageDir(userID uint) string {
	return filepath.Join(s.cfg.CacheDir, "import-previews", strconv.FormatUint(uint64(userID), 10))
}

func localImportStagePaths(dir string, token string) (string, string) {
	return filepath.Join(dir, token+".book"), filepath.Join(dir, token+".json")
}

func localImportPreparedStagePath(dir string, token string) string {
	return filepath.Join(dir, token+".parsed.json")
}

func (s *Server) saveStagedPreparedImport(userID uint, token string, prepared localbook.PreparedImport) error {
	if !validLocalImportToken(token) {
		return errInvalidLocalImportToken
	}
	if !s.validStagedPreparedImport(prepared) {
		return errLocalImportTooLarge
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(prepared); err != nil {
		return err
	}
	if int64(encoded.Len()) > s.maxLocalPreparedImportBytes() {
		return errLocalImportTooLarge
	}
	dir := s.localImportStageDir(userID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, token+".parsed-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded.Bytes()); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, localImportPreparedStagePath(dir, token))
}

func (s *Server) loadStagedPreparedImport(userID uint, token string, request localbook.ImportRequest) (localbook.PreparedImport, bool) {
	if !validLocalImportToken(token) {
		return localbook.PreparedImport{}, false
	}
	path := localImportPreparedStagePath(s.localImportStageDir(userID), token)
	file, err := os.Open(path)
	if err != nil {
		return localbook.PreparedImport{}, false
	}
	defer file.Close()
	limit := s.maxLocalPreparedImportBytes()
	encoded, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(encoded)) > limit {
		_ = os.Remove(path)
		return localbook.PreparedImport{}, false
	}
	var prepared localbook.PreparedImport
	if json.Unmarshal(encoded, &prepared) != nil || !s.validStagedPreparedImport(prepared) {
		_ = os.Remove(path)
		return localbook.PreparedImport{}, false
	}
	if !prepared.Matches(request) {
		return localbook.PreparedImport{}, false
	}
	return prepared, true
}

func (s *Server) maxLocalPreparedImportBytes() int64 {
	parsedLimit := s.cfg.MaxParsedTextBytes
	if parsedLimit <= 0 {
		parsedLimit = 256 * 1024 * 1024
	}
	inputLimit := s.maxLocalImportBytes()
	// Parser output consists primarily of extracted text plus bounded metadata
	// originating from the already bounded source archive. Keep JSON overhead
	// finite while allowing valid near-limit UTF-8 books.
	overhead := inputLimit + 8*1024*1024
	if overhead < inputLimit || parsedLimit > (math.MaxInt64-overhead)/2 {
		return math.MaxInt64
	}
	return parsedLimit*2 + overhead
}

func (s *Server) validStagedPreparedImport(prepared localbook.PreparedImport) bool {
	if prepared.Version != localbook.PreparedImportVersion || len(prepared.SourceSHA256) != sha256.Size*2 {
		return false
	}
	if _, err := hex.DecodeString(prepared.SourceSHA256); err != nil {
		return false
	}
	chapterLimit := s.cfg.MaxUMDChapters
	if chapterLimit <= 0 {
		chapterLimit = 100_000
	}
	if len(prepared.Book.Chapters) > chapterLimit {
		return false
	}
	remaining := s.cfg.MaxParsedTextBytes
	if remaining <= 0 {
		remaining = 256 * 1024 * 1024
	}
	consume := func(values ...string) bool {
		for _, value := range values {
			length := int64(len(value))
			if length > remaining {
				return false
			}
			remaining -= length
		}
		return true
	}
	if !consume(prepared.Extension, prepared.TOCRule, prepared.Book.Title, prepared.Book.Author, prepared.Book.CoverResourcePath) {
		return false
	}
	for _, chapter := range prepared.Book.Chapters {
		if !consume(
			chapter.Title,
			chapter.Content,
			chapter.ResourcePath,
			chapter.ResourceFragment,
			chapter.ResourceEndFragment,
		) {
			return false
		}
	}
	return true
}

func validLocalImportToken(token string) bool {
	if len(token) != 48 {
		return false
	}
	decoded, err := hex.DecodeString(token)
	return err == nil && len(decoded) == 24
}

func StartLocalImportStageCleanup(ctx context.Context, cacheDir string) {
	CleanupExpiredLocalImportStages(cacheDir)
	ticker := time.NewTicker(localImportStageCleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				CleanupExpiredLocalImportStages(cacheDir)
			}
		}
	}()
}

func CleanupExpiredLocalImportStages(cacheDir string) {
	root := filepath.Join(cacheDir, "import-previews")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() || !validLocalImportStageDirectoryName(entry.Name()) {
			continue
		}
		cleanupLocalImportStageDir(filepath.Join(root, entry.Name()), now)
	}
}

func validLocalImportStageDirectoryName(value string) bool {
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil && value != ""
}

func cleanupLocalImportStageDir(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := now.Add(-localImportStageLifetime)
	metadataTokens := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		token := strings.TrimSuffix(entry.Name(), ".json")
		if !validLocalImportToken(token) {
			continue
		}
		metadataPath := filepath.Join(dir, entry.Name())
		encoded, err := os.ReadFile(metadataPath)
		var metadata localImportStageMetadata
		if err != nil || json.Unmarshal(encoded, &metadata) != nil || metadata.CreatedAt.IsZero() || metadata.CreatedAt.Before(cutoff) {
			removeStagedLocalImportFromDir(dir, token)
			continue
		}
		dataPath, _ := localImportStagePaths(dir, token)
		if info, err := os.Stat(dataPath); err != nil || info.IsDir() {
			removeStagedLocalImportFromDir(dir, token)
			continue
		}
		metadataTokens[token] = true
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".book") {
			continue
		}
		token := strings.TrimSuffix(entry.Name(), ".book")
		if !validLocalImportToken(token) || metadataTokens[token] {
			continue
		}
		info, err := entry.Info()
		if err == nil && !info.ModTime().After(cutoff) {
			removeStagedLocalImportFromDir(dir, token)
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".parsed.json") {
			token := strings.TrimSuffix(name, ".parsed.json")
			if validLocalImportToken(token) && metadataTokens[token] {
				continue
			}
			info, err := entry.Info()
			if err == nil && !info.ModTime().After(cutoff) {
				_ = os.Remove(filepath.Join(dir, name))
			}
			continue
		}
		// Atomic snapshot writes use a token-prefixed temporary file. A crash
		// may leave one behind; only aged files inside the stage directory are
		// eligible for cleanup.
		if strings.Contains(name, ".parsed-") {
			info, err := entry.Info()
			if err == nil && !info.ModTime().After(cutoff) {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}
}

func removeStagedLocalImportFromDir(dir string, token string) {
	if !validLocalImportToken(token) {
		return
	}
	dataPath, metadataPath := localImportStagePaths(dir, token)
	_ = os.Remove(dataPath)
	_ = os.Remove(metadataPath)
	_ = os.Remove(localImportPreparedStagePath(dir, token))
}
