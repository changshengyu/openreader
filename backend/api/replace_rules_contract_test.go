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
	"openreader/backend/services/replacerules"
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

func TestReplaceRuleAcceptedFieldsPreserveExactWhitespace(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	response := replaceRuleContractRequest(t, router, http.MethodPost, "/api/replace-rules", `{
		"name":"  空白名称  ",
		"pattern":" 广告 ",
		"replacement":"替换",
		"scope":" 替换契约书;local://replace-contract "
	}`, token)
	if response.Code != http.StatusCreated {
		t.Fatalf("whitespace is valid upstream rule data: %d %s", response.Code, response.Body.String())
	}
	var rule models.ReplaceRule
	if err := json.Unmarshal(response.Body.Bytes(), &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Name != "  空白名称  " || rule.Pattern != " 广告 " ||
		rule.Scope != " 替换契约书;local://replace-contract " {
		t.Fatalf("accepted strings were normalized instead of preserved: %+v", rule)
	}

	whitespaceOnly := replaceRuleContractRequest(t, router, http.MethodPost, "/api/replace-rules", `{
		"name":" ","pattern":" ","replacement":"","scope":"*"
	}`, token)
	if whitespaceOnly.Code != http.StatusCreated {
		t.Fatalf("upstream only rejects exact empty strings, got %d: %s", whitespaceOnly.Code, whitespaceOnly.Body.String())
	}
}

func TestReplaceRuleBatchWithOnlySkippedRowsDoesNotBroadcast(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := replaceRuleContractUser(t, server)
	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)

	response := replaceRuleContractRequest(
		t,
		router,
		http.MethodPost,
		"/api/replace-rules/batch",
		`[{"name":"","pattern":"广告"},{"name":"无规则","pattern":""}]`,
		token,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("all-skipped batch: %d %s", response.Code, response.Body.String())
	}
	var summary struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Created != 0 || summary.Updated != 0 || summary.Skipped != 2 {
		t.Fatalf("unexpected all-skipped summary: %+v", summary)
	}
	select {
	case payload := <-client.Send:
		t.Fatalf("a request with no durable mutation broadcast an update: %s", payload)
	default:
	}
}

func TestReplaceRuleBatchRejectsExcessiveRowsBeforeMutation(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := replaceRuleContractUser(t, server)
	rows := make([]replaceRuleRequest, 0, maxReplaceRuleBatchItems+1)
	for index := 0; index <= maxReplaceRuleBatchItems; index++ {
		enabled := true
		rows = append(rows, replaceRuleRequest{
			Name:      "rule-" + strconv.Itoa(index),
			Pattern:   "a",
			Scope:     "*",
			IsEnabled: &enabled,
		})
	}
	body, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	response := replaceRuleContractRequest(
		t,
		router,
		http.MethodPost,
		"/api/replace-rules/batch",
		string(body),
		token,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized batch must be rejected, got %d: %s", response.Code, response.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.ReplaceRule{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("oversized batch mutated %d rules", count)
	}
}

func TestReplaceRuleReaderSemanticsPreserveOrderAndRegexFlags(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := replaceRuleContractUser(t, server)

	plain := false
	regex := true
	first := models.ReplaceRule{
		UserID: user.ID, Name: "先替换", Pattern: "ad", Replacement: "ONE", Scope: "*", IsRegex: &plain, Enabled: true, Order: 99,
	}
	second := models.ReplaceRule{
		UserID: user.ID, Name: "后替换", Pattern: "one", Replacement: "TWO", Scope: "*", IsRegex: &regex, Enabled: true, Order: -99,
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

func TestReplaceRuleScopeUsesExactSecondSegment(t *testing.T) {
	book := models.Book{Title: "目标书", URL: "https://book.example/1"}
	if replaceRuleAppliesToBook("目标书;", book) {
		t.Fatal("an explicit empty URL segment must not mean any non-empty book URL")
	}
	if !replaceRuleAppliesToBook("目标书;https://book.example/1", book) {
		t.Fatal("the exact title and URL scope must match")
	}
	if replaceRuleAppliesToBook(" 目标书;https://book.example/1", book) {
		t.Fatal("scope title whitespace must remain significant")
	}
	if replaceRuleAppliesToBook("目标书;https://book.example/1 ", book) {
		t.Fatal("scope URL whitespace must remain significant")
	}
}

func TestReplaceRuleReplacementStringMatchesJavaScript(t *testing.T) {
	plain, err := applyReaderReplaceRule(
		"left ad right ad",
		"ad",
		"[$$][$&][$`][$']",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "left [$][ad][left ][ right ad] right ad" {
		t.Fatalf("plain JavaScript replacement tokens diverged: %q", plain)
	}

	regex, err := applyReaderReplaceRule(
		"Ad1 ad2",
		`(ad)(\d)`,
		"$2-$1-$&-$$",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if regex != "1-Ad-Ad1-$ 2-ad-ad2-$" {
		t.Fatalf("regex JavaScript replacement tokens diverged: %q", regex)
	}
}

func TestReplaceRuleTestEndpointRejectsOversizedInput(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)
	body := `{"pattern":"a","replacement":"b","text":"` +
		strings.Repeat("a", maxReplaceRuleTestTextBytes+1) +
		`"}`
	response := replaceRuleContractRequest(
		t,
		router,
		http.MethodPost,
		"/api/replace-rules/test",
		body,
		token,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized test input must be rejected, got %d: %s", response.Code, response.Body.String())
	}
}

func TestReplaceRuleTestEndpointRejectsExecutionLimitOverflows(t *testing.T) {
	router, _ := setupTestServer(t)
	token := authHeader(t, router)

	matchLimitBody := `{"pattern":"a","replacement":"b","isRegex":true,"text":"` +
		strings.Repeat("a", replacerules.DefaultMaxMatches+1) +
		`"}`
	response := replaceRuleContractRequest(
		t,
		router,
		http.MethodPost,
		"/api/replace-rules/test",
		matchLimitBody,
		token,
	)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "execution limit") {
		t.Fatalf("match overflow must fail closed, got %d: %s", response.Code, response.Body.String())
	}

	outputLimitBody := `{"pattern":"a","replacement":"` +
		strings.Repeat("x", maxReplaceRuleReplacementBytes) +
		`","isRegex":true,"text":"` +
		strings.Repeat("a", maxReplaceRuleTestOutputBytes/maxReplaceRuleReplacementBytes+1) +
		`"}`
	response = replaceRuleContractRequest(
		t,
		router,
		http.MethodPost,
		"/api/replace-rules/test",
		outputLimitBody,
		token,
	)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "execution limit") {
		t.Fatalf("output overflow must fail closed, got %d: %s", response.Code, response.Body.String())
	}
}

func TestReplaceRuleExecutionLimitPreservesPriorPipelineOutput(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	user := replaceRuleContractUser(t, server)
	plain := false
	regex := true
	rules := []models.ReplaceRule{
		{
			UserID: user.ID, Name: "first", Pattern: "prefix", Replacement: "done",
			Scope: "*", IsRegex: &plain, Enabled: true,
		},
		{
			UserID: user.ID, Name: "bounded", Pattern: "a", Replacement: "b",
			Scope: "*", IsRegex: &regex, Enabled: true,
		},
		{
			UserID: user.ID, Name: "must not run", Pattern: "suffix", Replacement: "after",
			Scope: "*", IsRegex: &plain, Enabled: true,
		},
	}
	if err := server.db.Create(&rules).Error; err != nil {
		t.Fatal(err)
	}
	input := "prefix " + strings.Repeat("a", replacerules.DefaultMaxMatches+1) + " suffix"
	got := server.applyUserReplaceRules(models.Book{UserID: user.ID}, input)
	want := "done " + strings.Repeat("a", replacerules.DefaultMaxMatches+1) + " suffix"
	if got != want {
		t.Fatalf("execution overflow must preserve prior output and stop later rules")
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
	first := models.ReplaceRule{UserID: 1, Name: "先执行", Pattern: "A", Replacement: "B", Scope: "*", IsRegex: &plain, Enabled: true, Order: 99}
	second := models.ReplaceRule{UserID: 1, Name: "后执行", Pattern: "B", Replacement: "C", Scope: "*", IsRegex: &plain, Enabled: true, Order: -99}
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

func TestReplaceRuleRestoreUsesExactNameAndArchiveOrder(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "replace-restore-contract", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	plain := false
	byName := models.ReplaceRule{
		UserID: user.ID, Name: "按名称更新", Pattern: "旧 pattern", Scope: "*",
		IsRegex: &plain, Enabled: true,
	}
	samePattern := models.ReplaceRule{
		UserID: user.ID, Name: "不可误覆盖", Pattern: "目标 pattern", Scope: "*",
		IsRegex: &plain, Enabled: true,
	}
	if err := server.db.Create(&byName).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&samePattern).Error; err != nil {
		t.Fatal(err)
	}

	count, err := server.restoreReplaceRulesFromData([]byte(`[
		{"name":"按名称更新","pattern":"目标 pattern","replacement":"新","scope":"*","isEnabled":true,"order":99},
		{"name":"","pattern":"无名 pattern","replacement":"不得创建","scope":"*"},
		{"name":"后追加一","pattern":"A","replacement":"B","scope":"*","order":50},
		{"name":"后追加二","pattern":"B","replacement":"C","scope":"*","order":-50}
	]`), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("restore must skip the unnamed row, count=%d", count)
	}

	var rules []models.ReplaceRule
	if err := server.db.Where("user_id = ?", user.ID).Order("id asc").Find(&rules).Error; err != nil {
		t.Fatal(err)
	}
	if len(rules) != 4 {
		t.Fatalf("restore changed the wrong identity or created an unnamed row: %+v", rules)
	}
	if rules[0].ID != byName.ID || rules[0].Pattern != "目标 pattern" || rules[0].Replacement != "新" {
		t.Fatalf("same-name row was not updated in place: %+v", rules)
	}
	if rules[1].ID != samePattern.ID || rules[1].Name != "不可误覆盖" {
		t.Fatalf("same-pattern different-name row was overwritten: %+v", rules)
	}
	if rules[2].Name != "后追加一" || rules[3].Name != "后追加二" {
		t.Fatalf("new archive rows did not append in archive order: %+v", rules)
	}
}

func TestReplaceRuleRestorePrevalidatesEveryRuleBeforeMutation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "replace-restore-validation", PasswordHash: "hash"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	count, err := server.restoreReplaceRulesFromData([]byte(`[
		{"name":"would-write","pattern":"a","replacement":"b","scope":"*","isRegex":false},
		{"name":"invalid-regex","pattern":"(?=unsupported)","replacement":"","scope":"*","isRegex":true}
	]`), user.ID)
	if err == nil {
		t.Fatal("unsupported restored regex must fail validation")
	}
	if count != 0 {
		t.Fatalf("failed prevalidation reported restored rows: %d", count)
	}
	var stored int64
	if err := server.db.Model(&models.ReplaceRule{}).Where("user_id = ?", user.ID).Count(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("restore wrote %d rows before discovering an invalid rule", stored)
	}
}
