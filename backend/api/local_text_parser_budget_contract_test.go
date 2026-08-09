package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/services/localbook"
)

func TestTXTPreviewParserLimitRetainsCallerStageWithoutPersistence(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxParsedTextBytes = 8
		cfg.MaxParsedChapters = 10
	})
	auth := authHeader(t, router)
	response := directLocalBookMultipartRequest(
		t,
		router,
		auth,
		"/api/imports/books/preview",
		"decoded-limit.txt",
		[]byte("第一章\n正文超过限制"),
		map[string]string{"tocRule": `^第.+章$`},
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("TXT parser-budget preview = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Error       string `json:"error"`
		ImportToken string `json:"importToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload.Error, engine.ErrLocalBookParseLimit.Error()) || !validLocalImportToken(payload.ImportToken) {
		t.Fatalf("TXT parser-budget response = %+v", payload)
	}
	dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), payload.ImportToken)
	for _, path := range []string{dataPath, metadataPath} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("failed preview must retain retry stage %s: info=%v err=%v", filepath.Base(path), info, err)
		}
	}
	var books int64
	if err := server.db.Table("books").Count(&books).Error; err != nil || books != 0 {
		t.Fatalf("parser-budget preview persisted books=%d err=%v", books, err)
	}
}

func TestStorageTXTPreviewsShareParserBudgetAndRetainScopedStages(t *testing.T) {
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.MaxParsedChapters = 1
	})
	auth := authHeader(t, router)
	fixture := []byte("第一章\n正文一\n第二章\n正文二")
	for _, test := range []struct {
		name     string
		root     string
		endpoint string
	}{
		{name: "LocalStore", root: server.cfg.LocalStoreDir, endpoint: "/api/local-store/import-preview"},
		{name: "WebDAV", root: filepath.Join(server.cfg.DataDir, "webdav"), endpoint: "/api/webdav/import-preview"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.MkdirAll(test.root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(test.root, "budget.txt"), fixture, 0o600); err != nil {
				t.Fatal(err)
			}
			request := strings.NewReader(`{"items":[{"path":"budget.txt","tocRule":"^第.+章$"}]}`)
			httpRequest, err := http.NewRequest(http.MethodPost, test.endpoint, request)
			if err != nil {
				t.Fatal(err)
			}
			httpRequest.Header.Set("Authorization", auth)
			httpRequest.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httpRequest)
			if response.Code != http.StatusOK {
				t.Fatalf("storage parser-budget preview = %d: %s", response.Code, response.Body.String())
			}
			var preview stagedStoragePreview
			if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			if len(preview.Items) != 1 || preview.Items[0].Book != nil ||
				!strings.Contains(preview.Items[0].Error, engine.ErrLocalBookParseLimit.Error()) ||
				!validLocalImportToken(preview.Items[0].ImportToken) {
				t.Fatalf("storage parser-budget result = %+v", preview)
			}
			dataPath, metadataPath := localImportStagePaths(server.localImportStageDir(1), preview.Items[0].ImportToken)
			for _, path := range []string{dataPath, metadataPath} {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("storage failed preview must retain %s: %v", filepath.Base(path), err)
				}
			}
		})
	}
}

func TestPreparedImportUsesGenericChapterLimitInsteadOfUMDLimit(t *testing.T) {
	server := &Server{cfg: config.Config{MaxParsedChapters: 1, MaxUMDChapters: 10}}
	prepared := localbook.PreparedImport{
		Version:      localbook.PreparedImportVersion,
		Extension:    ".txt",
		TOCRule:      `^第.+章$`,
		SourceSHA256: strings.Repeat("a", 64),
		Book: engine.ParsedBook{Chapters: []engine.TXTChapter{
			{Index: 0, Title: "第一章", Content: "一"},
			{Index: 1, Title: "第二章", Content: "二"},
		}},
	}
	if server.validStagedPreparedImport(prepared) {
		t.Fatal("prepared TXT snapshot exceeded generic chapter limit")
	}
}

func TestReadBoundedLocalBookSourceRejectsLegacyInputBeforeReturningBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(path, []byte("123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := readBoundedLocalBookSource(path, 8)
	if !errors.Is(err, engine.ErrLocalBookParseLimit) || data != nil {
		t.Fatalf("bounded historical source = %q, %v", data, err)
	}
	valid, err := readBoundedLocalBookSource(path, 9)
	if err != nil || string(valid) != "123456789" {
		t.Fatalf("bounded historical source control = %q, %v", valid, err)
	}
}
