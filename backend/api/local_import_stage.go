package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const localImportStageLifetime = 24 * time.Hour

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
	s.cleanupExpiredLocalImportStages(dir)

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
}

func (s *Server) localImportStageDir(userID uint) string {
	return filepath.Join(s.cfg.CacheDir, "import-previews", strconv.FormatUint(uint64(userID), 10))
}

func localImportStagePaths(dir string, token string) (string, string) {
	return filepath.Join(dir, token+".book"), filepath.Join(dir, token+".json")
}

func validLocalImportToken(token string) bool {
	if len(token) != 48 {
		return false
	}
	decoded, err := hex.DecodeString(token)
	return err == nil && len(decoded) == 24
}

func (s *Server) cleanupExpiredLocalImportStages(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-localImportStageLifetime)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		token := strings.TrimSuffix(entry.Name(), ".json")
		s.removeStagedLocalImportFromDir(dir, token)
	}
}

func (s *Server) removeStagedLocalImportFromDir(dir string, token string) {
	if !validLocalImportToken(token) {
		return
	}
	dataPath, metadataPath := localImportStagePaths(dir, token)
	_ = os.Remove(dataPath)
	_ = os.Remove(metadataPath)
}
