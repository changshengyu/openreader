package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"openreader/backend/services/importstage"
	"openreader/backend/services/localbook"
)

const (
	localImportStageLifetime              = importstage.Lifetime
	localImportStageCleanupInterval       = importstage.CleanupInterval
	defaultMaxLocalImportBytes      int64 = 128 * 1024 * 1024
)

var localImportStageLifecycleTestHook func(phase, directory, token, path string)

func localImportStageLifecycleTestPhase(phase, directory, token, path string) {
	if localImportStageLifecycleTestHook != nil {
		localImportStageLifecycleTestHook(phase, directory, token, path)
	}
}

var errInvalidLocalImportToken = importstage.ErrInvalidToken
var errLocalImportTooLarge = importstage.ErrTooLarge

type localImportStageMetadata = importstage.Metadata

func (s *Server) localImportStages() *importstage.Service {
	return importstage.New(s.cfg.CacheDir, importstage.Limits{
		Source: s.maxLocalImportBytes(), Prepared: s.maxLocalPreparedImportBytes(),
		ValidPrepared: s.validStagedPreparedImport,
	}, localImportStageLifecycleTestPhase)
}

// Legacy internal/test adapters. HTTP flows retain one session through handoff.
func (s *Server) stageLocalImport(userID uint, name, extension string, data []byte) (string, error) {
	stage, err := s.localImportStages().Create(context.Background(), userID, name, extension, data)
	if err != nil {
		return "", err
	}
	defer stage.Close()
	return stage.Token, nil
}
func (s *Server) loadStagedLocalImport(userID uint, token string) (localImportStageMetadata, []byte, error) {
	stage, err := s.localImportStages().Open(context.Background(), userID, token)
	if err != nil {
		return localImportStageMetadata{}, nil, err
	}
	defer stage.Close()
	return stage.Metadata, stage.Data, nil
}
func (s *Server) saveStagedPreparedImport(userID uint, token string, prepared localbook.PreparedImport) error {
	stage, err := s.localImportStages().Open(context.Background(), userID, token)
	if err != nil {
		return err
	}
	defer stage.Close()
	return stage.SavePrepared(prepared)
}
func (s *Server) loadStagedPreparedImport(userID uint, token string, request localbook.ImportRequest) (localbook.PreparedImport, bool) {
	stage, err := s.localImportStages().Open(context.Background(), userID, token)
	if err != nil {
		return localbook.PreparedImport{}, false
	}
	defer stage.Close()
	prepared, matched, err := stage.Prepared(request)
	return prepared, matched && err == nil
}

func (s *Server) maxLocalImportBytes() int64 {
	if s.cfg.MaxImportBytes > 0 {
		return s.cfg.MaxImportBytes
	}
	return defaultMaxLocalImportBytes
}
func (s *Server) readBoundedLocalImport(reader io.Reader) ([]byte, error) {
	return importstage.ReadBounded(context.Background(), reader, s.maxLocalImportBytes(), nil, nil)
}
func (s *Server) copyBoundedLocalImport(destination io.Writer, source io.Reader) error {
	limit := s.maxLocalImportBytes()
	probe := limit
	if probe < math.MaxInt64 {
		probe++
	}
	written, err := io.Copy(destination, io.LimitReader(source, probe))
	if err != nil {
		return err
	}
	if written > limit {
		return errLocalImportTooLarge
	}
	return nil
}

// Only budget controls use this adapter; live stage files use native sessions.
func (s *Server) readBoundedLocalImportFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return s.readBoundedLocalImport(file)
}
func (s *Server) localImportStageDir(userID uint) string {
	return filepath.Join(s.cfg.CacheDir, "import-previews", strconv.FormatUint(uint64(userID), 10))
}
func localImportStagePaths(dir, token string) (string, string) {
	return filepath.Join(dir, token+".book"), filepath.Join(dir, token+".json")
}
func localImportPreparedStagePath(dir, token string) string {
	return filepath.Join(dir, token+".parsed.json")
}
func (s *Server) maxLocalPreparedImportBytes() int64 {
	parsedLimit := s.cfg.MaxParsedTextBytes
	if parsedLimit <= 0 {
		parsedLimit = 256 * 1024 * 1024
	}
	inputLimit := s.maxLocalImportBytes()
	if inputLimit > math.MaxInt64-8*1024*1024 {
		return math.MaxInt64
	}
	overhead := inputLimit + 8*1024*1024
	if parsedLimit > (math.MaxInt64-overhead)/2 {
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
	if len(prepared.Book.Chapters) > s.cfg.ParsedChapterLimit() {
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
		if !consume(chapter.Title, chapter.Content, chapter.ResourcePath, chapter.ResourceFragment, chapter.ResourceEndFragment) {
			return false
		}
	}
	return true
}
func validLocalImportToken(token string) bool { return importstage.ValidToken(token) }
func StartLocalImportStageCleanup(ctx context.Context, cacheDir string) {
	importstage.New(cacheDir, importstage.Limits{}, localImportStageLifecycleTestPhase).StartCleanup(ctx)
}
func CleanupExpiredLocalImportStages(cacheDir string) {
	importstage.New(cacheDir, importstage.Limits{}, localImportStageLifecycleTestPhase).Cleanup(context.Background())
}
