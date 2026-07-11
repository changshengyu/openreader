package api

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"openreader/backend/models"
)

func replaceRuleContractBool(value bool) *bool {
	return &value
}

func replaceRuleContractUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func replaceRuleContractRequest(t *testing.T, router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func replaceRuleContractChapter(t *testing.T, server *Server, userID uint, content string) models.Book {
	t.Helper()
	cachePath := filepath.Join("replace-contract", "chapter.txt")
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: userID, Title: "替换契约书", URL: "local://replace-contract"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return book
}

func TestReplaceRuleDefaultsAndNameUpsertMatchReaderDev(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)

	created := replaceRuleContractRequest(t, router, http.MethodPost, "/api/replace-rules", `{
		"name":"同名规则","pattern":"广告","replacement":"","scope":"*"
	}`, token)
	if created.Code != http.StatusCreated {
		t.Fatalf("create rule: expected 201, got %d: %s", created.Code, created.Body.String())
	}
	var first models.ReplaceRule
	if err := json.Unmarshal(created.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.IsRegex == nil || *first.IsRegex {
		t.Fatalf("missing isRegex must use upstream plain-text default, got %+v", first)
	}

	replaced := replaceRuleContractRequest(t, router, http.MethodPost, "/api/replace-rules", `{
		"name":"同名规则","pattern":"广告位","replacement":"","scope":"*","isRegex":false
	}`, token)
	if replaced.Code != http.StatusOK {
		t.Fatalf("same-name add must upsert in place with 200, got %d: %s", replaced.Code, replaced.Body.String())
	}
	var second models.ReplaceRule
	if err := json.Unmarshal(replaced.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Pattern != "广告位" {
		t.Fatalf("same-name rule should replace in place, first=%+v replacement=%+v", first, second)
	}

	for _, body := range []string{
		`{"name":"","pattern":"广告","scope":"*"}`,
		`{"name":"缺范围","pattern":"广告","scope":""}`,
	} {
		response := replaceRuleContractRequest(t, router, http.MethodPost, "/api/replace-rules", body, token)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("upstream form-required field must be rejected, got %d: %s", response.Code, response.Body.String())
		}
	}

	_ = server
}

func TestReplaceRuleReaderSemanticsPreserveOrderAndRegexFlags(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := replaceRuleContractUser(t, server)

	plain := false
	regex := true
	first := models.ReplaceRule{
		UserID: user.ID, Name: "先替换", Pattern: "ad", Replacement: "ONE", Scope: "*", IsRegex: &plain, Enabled: true,
	}
	second := models.ReplaceRule{
		UserID: user.ID, Name: "后替换", Pattern: "one", Replacement: "TWO", Scope: "*", IsRegex: &regex, Enabled: true,
	}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&second).Where("id = ?", second.ID).Update("updated_at", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	listed := replaceRuleContractRequest(t, router, http.MethodGet, "/api/replace-rules", "", token)
	if listed.Code != http.StatusOK {
		t.Fatalf("list replace rules: %d: %s", listed.Code, listed.Body.String())
	}
	var rules []models.ReplaceRule
	if err := json.Unmarshal(listed.Body.Bytes(), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].ID != first.ID || rules[1].ID != second.ID {
		t.Fatalf("rule list must retain stable insertion order after updates, got %+v", rules)
	}

	book := replaceRuleContractChapter(t, server, user.ID, "Ad ad ONE one")
	content := replaceRuleContractRequest(t, router, http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", "", token)
	if content.Code != http.StatusOK {
		t.Fatalf("chapter content: %d: %s", content.Code, content.Body.String())
	}
	if !strings.Contains(content.Body.String(), "Ad TWO TWO TWO") {
		t.Fatalf("plain first-match and regex global/case-insensitive semantics diverged: %s", content.Body.String())
	}
}

func TestReplaceRuleInvalidRegexIsRejectedWithoutLiteralFallback(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	response := replaceRuleContractRequest(t, router, http.MethodPost, "/api/replace-rules/test", `{
		"pattern":"[broken","replacement":"changed","isRegex":true,"text":"[broken"
	}`, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid regular expression must be a client error, got %d: %s", response.Code, response.Body.String())
	}
}

func TestLegacyBlankReplaceRuleScopeRemainsGlobalUntilEdited(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := replaceRuleContractUser(t, server)
	plain := false
	if err := server.db.Create(&models.ReplaceRule{
		UserID: user.ID, Name: "旧空范围", Pattern: "旧广告", Replacement: "", Scope: "", IsRegex: &plain, Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	book := replaceRuleContractChapter(t, server, user.ID, "旧广告 正文")
	content := replaceRuleContractRequest(t, router, http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", "", token)
	if content.Code != http.StatusOK || strings.Contains(content.Body.String(), "旧广告") {
		t.Fatalf("legacy blank scope must remain readable as global before edit: %d %s", content.Code, content.Body.String())
	}
}

func TestReplaceRuleBackupPreservesReaderPipelineOrder(t *testing.T) {
	_, server := setupTestServer(t)
	plain := false
	first := models.ReplaceRule{UserID: 1, Name: "先执行", Pattern: "A", Replacement: "B", Scope: "*", IsRegex: &plain, Enabled: true}
	second := models.ReplaceRule{UserID: 1, Name: "后执行", Pattern: "B", Replacement: "C", Scope: "*", IsRegex: &plain, Enabled: true}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&second).Where("id = ?", second.ID).Update("updated_at", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	path, err := server.backupSvc.RunNow()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "replaceRules.json" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		var rules []models.ReplaceRule
		if err := json.Unmarshal(data, &rules); err != nil {
			t.Fatal(err)
		}
		if len(rules) != 2 || rules[0].ID != first.ID || rules[1].ID != second.ID {
			t.Fatalf("backup must retain reader insertion pipeline, got %+v", rules)
		}
		return
	}
	t.Fatal("replaceRules.json not found in backup")
}
