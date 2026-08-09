package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

type sourceContractAccount struct {
	ID     uint
	Auth   string
	Source models.BookSource
}

func TestBookSourceListProjectsOnlyCallerBookshelfNamesInStableOrder(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "sourceusagealice")
	bob := registerSourceContractAccount(t, router, "sourceusagebob")
	shared := createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"共享引用源",
		"baseUrl":"https://source-usage.example",
		"enabled":true
	}`)
	if err := server.db.Create(&models.UserBookSource{UserID: bob.ID, SourceID: shared.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSourceNamespace{UserID: bob.ID}).Error; err != nil {
		t.Fatal(err)
	}
	books := []models.Book{
		{UserID: alice.ID, SourceID: shared.ID, Title: "Alice 第一册"},
		{UserID: alice.ID, SourceID: shared.ID, Title: "Alice 第二册"},
		{UserID: bob.ID, SourceID: shared.ID, Title: "Bob 私有册"},
	}
	if err := server.db.Create(&books).Error; err != nil {
		t.Fatal(err)
	}

	type sourceUsageProjection struct {
		ID            uint     `json:"id"`
		UsedBookCount int      `json:"usedBookCount"`
		UsedBookNames []string `json:"usedBookNames"`
	}
	load := func(auth string) []sourceUsageProjection {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
		request.Header.Set("Authorization", auth)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("list sources = %d %s", response.Code, response.Body.String())
		}
		var rows []sourceUsageProjection
		if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}

	aliceRows := load(alice.Auth)
	if len(aliceRows) != 1 || aliceRows[0].ID != shared.ID || aliceRows[0].UsedBookCount != 2 {
		t.Fatalf("alice usage projection = %#v", aliceRows)
	}
	if got, want := aliceRows[0].UsedBookNames, []string{"Alice 第一册", "Alice 第二册"}; !stringSlicesEqual(got, want) {
		t.Fatalf("alice usedBookNames = %#v, want %#v", got, want)
	}

	bobRows := load(bob.Auth)
	if len(bobRows) != 1 || bobRows[0].ID != shared.ID || bobRows[0].UsedBookCount != 1 {
		t.Fatalf("bob usage projection = %#v", bobRows)
	}
	if got, want := bobRows[0].UsedBookNames, []string{"Bob 私有册"}; !stringSlicesEqual(got, want) {
		t.Fatalf("bob usedBookNames = %#v, want %#v", got, want)
	}
}

func TestBookSourceImportEditorFieldsRoundTripWithoutSchemaMigration(t *testing.T) {
	router, server := setupTestServer(t)
	account := registerSourceContractAccount(t, router, "sourceextraroundtrip")
	response := importSourcesThroughAPI(t, router, account.Auth, `[
		{
			"bookSourceName":"扩展字段源",
			"bookSourceUrl":"https://source-extra.example",
			"enabled":true,
			"enabledExplore":true,
			"loginUi":[{"name":"账号","type":"text"}],
			"customExtension":{"mode":"preserve-only","script":"@js:never-execute()"},
			"ruleToc":{"chapterList":".chapter","preUpdateJs":"@js:preserve-toc()"},
			"ruleContent":{"content":".content","webJs":"@js:preserve-content()"}
		}
	]`)
	if response.Code != http.StatusOK {
		t.Fatalf("import extended source = %d %s", response.Code, response.Body.String())
	}

	var stored models.BookSource
	if err := server.db.Where("base_url = ?", "https://source-extra.example").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Rules, `"__openreaderSourceExtra"`) {
		t.Fatalf("stored rules did not preserve top-level extensions: %s", stored.Rules)
	}

	exportRequest := httptest.NewRequest(http.MethodGet, "/api/sources/export?sourceIds="+uintString(stored.ID), nil)
	exportRequest.Header.Set("Authorization", account.Auth)
	exportResponse := httptest.NewRecorder()
	router.ServeHTTP(exportResponse, exportRequest)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export extended source = %d %s", exportResponse.Code, exportResponse.Body.String())
	}
	var exported []map[string]any
	if err := json.Unmarshal(exportResponse.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 {
		t.Fatalf("exported extended sources = %#v", exported)
	}
	if _, ok := exported[0]["loginUi"].([]any); !ok {
		t.Fatalf("loginUi did not round-trip at top level: %#v", exported[0])
	}
	extension, ok := exported[0]["customExtension"].(map[string]any)
	if !ok || extension["script"] != "@js:never-execute()" {
		t.Fatalf("custom extension did not round-trip: %#v", exported[0])
	}
	if strings.Contains(fmt.Sprint(exported[0]["rules"]), "__openreaderSourceExtra") {
		t.Fatalf("internal preservation envelope leaked into reader-dev JSON: %#v", exported[0])
	}
}

func TestBookSourceImportRejectsOversizedFileBeforeJSONDecode(t *testing.T) {
	router, _ := setupTestServer(t)
	account := registerSourceContractAccount(t, router, "sourceoversize")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "oversized-bookSources.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, io.LimitReader(zeroBytesReader{}, maxBookSourceImportBytes+1)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sources/import", &body)
	request.Header.Set("Authorization", account.Auth)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge ||
		!strings.Contains(response.Body.String(), "source file is too large") {
		t.Fatalf("oversized source import = %d %s", response.Code, response.Body.String())
	}
}

type zeroBytesReader struct{}

func (zeroBytesReader) Read(data []byte) (int, error) {
	for index := range data {
		data[index] = 'x'
	}
	return len(data), nil
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestBookSourceCRUDAndReadsAreScopedToAuthenticatedUser(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "sourceapialice")
	bob := registerSourceContractAccount(t, router, "sourceapibob")

	alice.Source = createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Alice 私有源",
		"baseUrl":"https://alice-source.example",
		"searchUrl":"https://alice-source.example/search",
		"enabled":true
	}`)
	assertSourceListIDs(t, router, alice.Auth, []uint{alice.Source.ID})
	assertSourceListIDs(t, router, bob.Auth, nil)

	for _, endpoint := range []string{
		"/api/sources/" + uintString(alice.Source.ID),
		"/api/sources/" + uintString(alice.Source.ID) + "/test",
	} {
		method := http.MethodGet
		var body *strings.Reader
		if strings.HasSuffix(endpoint, "/test") {
			method = http.MethodPost
			body = strings.NewReader(`{"keyword":"测试"}`)
		} else {
			body = strings.NewReader("")
		}
		request := httptest.NewRequest(method, endpoint, body)
		request.Header.Set("Authorization", bob.Auth)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "source not found") {
			t.Fatalf("foreign %s %s = %d %s, want scoped 404", method, endpoint, response.Code, response.Body.String())
		}
	}

	update := httptest.NewRequest(
		http.MethodPut,
		"/api/sources/"+uintString(alice.Source.ID),
		strings.NewReader(`{"name":"越权修改","baseUrl":"https://foreign.example","enabled":true}`),
	)
	update.Header.Set("Authorization", bob.Auth)
	update.Header.Set("Content-Type", "application/json")
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign update = %d %s, want 404", updateResponse.Code, updateResponse.Body.String())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/sources/"+uintString(alice.Source.ID), nil)
	deleteRequest.Header.Set("Authorization", bob.Auth)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNotFound {
		t.Fatalf("foreign delete = %d %s, want 404", deleteResponse.Code, deleteResponse.Body.String())
	}

	var stored models.BookSource
	if err := server.db.First(&stored, alice.Source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Name != alice.Source.Name || stored.BaseURL != alice.Source.BaseURL {
		t.Fatalf("foreign mutations changed alice source: %+v", stored)
	}
}

func TestBookSourceSharedSnapshotUpdateAndDeleteUseCopyOnWrite(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "sourcecowapialice")
	bob := registerSourceContractAccount(t, router, "sourcecowapibob")
	shared := createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"迁移共享源",
		"baseUrl":"https://shared-api.example",
		"header":"{\"X-Owner\":\"shared\"}",
		"enabled":true
	}`)
	if err := server.db.Create(&models.UserBookSource{UserID: bob.ID, SourceID: shared.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookSourceNamespace{UserID: bob.ID}).Error; err != nil {
		t.Fatal(err)
	}
	aliceBook := models.Book{
		UserID: alice.ID, SourceID: shared.ID, Title: "Alice 书",
		Variable: `{"owner":"alice"}`,
	}
	bobBook := models.Book{
		UserID: bob.ID, SourceID: shared.ID, Title: "Bob 书",
		Variable: `{"owner":"bob"}`,
	}
	if err := server.db.Create(&[]*models.Book{&aliceBook, &bobBook}).Error; err != nil {
		t.Fatal(err)
	}

	update := httptest.NewRequest(
		http.MethodPut,
		"/api/sources/"+uintString(shared.ID),
		strings.NewReader(`{
			"name":"Alice 写时复制源",
			"baseUrl":"https://alice-cow.example",
			"header":"{\"X-Owner\":\"alice\"}",
			"charset":"utf-8",
			"enabled":true
		}`),
	)
	update.Header.Set("Authorization", alice.Auth)
	update.Header.Set("Content-Type", "application/json")
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("copy-on-write update = %d %s", updateResponse.Code, updateResponse.Body.String())
	}
	var aliceUpdated models.BookSource
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &aliceUpdated); err != nil {
		t.Fatal(err)
	}
	if aliceUpdated.ID == 0 || aliceUpdated.ID == shared.ID {
		t.Fatalf("shared update did not return a copied source: %+v", aliceUpdated)
	}
	assertSourceListIDs(t, router, alice.Auth, []uint{aliceUpdated.ID})
	assertSourceListIDs(t, router, bob.Auth, []uint{shared.ID})

	var aliceStoredBook, bobStoredBook models.Book
	if err := server.db.First(&aliceStoredBook, aliceBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.First(&bobStoredBook, bobBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if aliceStoredBook.SourceID != aliceUpdated.ID || aliceStoredBook.Variable != "" {
		t.Fatalf("alice book was not privately remapped/cleared: %+v", aliceStoredBook)
	}
	if bobStoredBook.SourceID != shared.ID || bobStoredBook.Variable != bobBook.Variable {
		t.Fatalf("bob book changed during alice update: %+v", bobStoredBook)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/sources/"+uintString(aliceUpdated.ID), nil)
	deleteRequest.Header.Set("Authorization", alice.Auth)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusConflict ||
		!strings.Contains(deleteResponse.Body.String(), `"usedBookCount":1`) {
		t.Fatalf("alice used-source delete = %d %s, want scoped 409", deleteResponse.Code, deleteResponse.Body.String())
	}
	assertSourceListIDs(t, router, bob.Auth, []uint{shared.ID})
}

func TestBookSourceExportBatchDebugAndBroadcastDoNotCrossAccounts(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "sourcetoolsalice")
	bob := registerSourceContractAccount(t, router, "sourcetoolsbob")
	aliceClient := server.hub.AddClient(alice.ID, nil)
	bobClient := server.hub.AddClient(bob.ID, nil)
	t.Cleanup(func() {
		server.hub.RemoveClient(aliceClient)
		server.hub.RemoveClient(bobClient)
	})

	alice.Source = createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Alice 工具源",
		"baseUrl":"https://alice-tools.example",
		"enabled":true
	}`)
	select {
	case payload := <-aliceClient.Send:
		if !strings.Contains(string(payload), `"type":"sources_update"`) {
			t.Fatalf("unexpected alice broadcast: %s", payload)
		}
	default:
		t.Fatal("alice did not receive her source update")
	}
	select {
	case payload := <-bobClient.Send:
		t.Fatalf("bob received alice source update: %s", payload)
	default:
	}

	exportRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/sources/export?sourceIds="+uintString(alice.Source.ID),
		nil,
	)
	exportRequest.Header.Set("Authorization", bob.Auth)
	exportResponse := httptest.NewRecorder()
	router.ServeHTTP(exportResponse, exportRequest)
	if exportResponse.Code != http.StatusOK || strings.TrimSpace(exportResponse.Body.String()) != "[]" {
		t.Fatalf("bob foreign export = %d %s, want 200 []", exportResponse.Code, exportResponse.Body.String())
	}

	batchRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/sources/batch",
		strings.NewReader(`{"action":"disable","sourceIds":[`+uintString(alice.Source.ID)+`]}`),
	)
	batchRequest.Header.Set("Authorization", bob.Auth)
	batchRequest.Header.Set("Content-Type", "application/json")
	batchResponse := httptest.NewRecorder()
	router.ServeHTTP(batchResponse, batchRequest)
	if batchResponse.Code != http.StatusOK ||
		!strings.Contains(batchResponse.Body.String(), `"affected":0`) {
		t.Fatalf("bob foreign batch = %d %s, want affected 0", batchResponse.Code, batchResponse.Body.String())
	}

	var stored models.BookSource
	if err := server.db.First(&stored, alice.Source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Enabled {
		t.Fatalf("bob disabled alice source: %+v", stored)
	}

	debugRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/sources/batch-test",
		strings.NewReader(`{"keyword":"测试","sourceIds":[`+uintString(alice.Source.ID)+`]}`),
	)
	debugRequest.Header.Set("Authorization", bob.Auth)
	debugRequest.Header.Set("Content-Type", "application/json")
	debugResponse := httptest.NewRecorder()
	router.ServeHTTP(debugResponse, debugRequest)
	if debugResponse.Code != http.StatusOK ||
		!strings.Contains(debugResponse.Body.String(), `"results":[]`) {
		t.Fatalf("bob foreign batch debug = %d %s, want empty results", debugResponse.Code, debugResponse.Body.String())
	}
}

func TestBookSourceClearImportAndDefaultRestoreAreScoped(t *testing.T) {
	router, _ := setupTestServer(t)
	admin := registerSourceContractAccount(t, router, "sourcedefaultadmin")
	bob := registerSourceContractAccount(t, router, "sourcedefaultbob")
	admin.Source = createSourceThroughAPI(t, router, admin.Auth, `{
		"name":"管理员默认源",
		"baseUrl":"https://default-api.example",
		"enabled":true
	}`)

	saveDefault := httptest.NewRequest(http.MethodPost, "/api/sources/default/save", nil)
	saveDefault.Header.Set("Authorization", admin.Auth)
	saveDefaultResponse := httptest.NewRecorder()
	router.ServeHTTP(saveDefaultResponse, saveDefault)
	if saveDefaultResponse.Code != http.StatusOK ||
		!strings.Contains(saveDefaultResponse.Body.String(), `"count":1`) {
		t.Fatalf("admin save default = %d %s", saveDefaultResponse.Code, saveDefaultResponse.Body.String())
	}

	assertSourceListIDs(t, router, bob.Auth, []uint{admin.Source.ID})
	importResponse := importSourcesThroughAPI(t, router, bob.Auth, `[
		{"bookSourceName":"Bob 覆盖默认源","bookSourceUrl":"https://default-api.example","enabled":true},
		{"bookSourceName":"Bob 新源","bookSourceUrl":"https://bob-import.example","enabled":true}
	]`)
	if importResponse.Code != http.StatusOK ||
		!strings.Contains(importResponse.Body.String(), `"imported":1`) ||
		!strings.Contains(importResponse.Body.String(), `"updated":1`) {
		t.Fatalf("bob import = %d %s", importResponse.Code, importResponse.Body.String())
	}
	adminSources := sourceList(t, router, admin.Auth)
	bobSources := sourceList(t, router, bob.Auth)
	if len(adminSources) != 1 || adminSources[0].Name != "管理员默认源" {
		t.Fatalf("bob import changed admin source: %+v", adminSources)
	}
	if len(bobSources) != 2 || bobSources[0].Name != "Bob 覆盖默认源" {
		t.Fatalf("bob import did not stay in bob namespace: %+v", bobSources)
	}

	bobSaveDefault := httptest.NewRequest(http.MethodPost, "/api/sources/default/save", nil)
	bobSaveDefault.Header.Set("Authorization", bob.Auth)
	bobSaveDefaultResponse := httptest.NewRecorder()
	router.ServeHTTP(bobSaveDefaultResponse, bobSaveDefault)
	if bobSaveDefaultResponse.Code != http.StatusForbidden {
		t.Fatalf("ordinary user saved global defaults: %d %s", bobSaveDefaultResponse.Code, bobSaveDefaultResponse.Body.String())
	}

	clearRequest := httptest.NewRequest(http.MethodDelete, "/api/sources", nil)
	clearRequest.Header.Set("Authorization", bob.Auth)
	clearResponse := httptest.NewRecorder()
	router.ServeHTTP(clearResponse, clearRequest)
	if clearResponse.Code != http.StatusOK ||
		!strings.Contains(clearResponse.Body.String(), `"affected":2`) {
		t.Fatalf("bob clear = %d %s", clearResponse.Code, clearResponse.Body.String())
	}
	assertSourceListIDs(t, router, bob.Auth, nil)
	adminSources = sourceList(t, router, admin.Auth)
	if len(adminSources) != 1 || adminSources[0].Name != "管理员默认源" {
		t.Fatalf("bob clear changed admin source: %+v", adminSources)
	}

	// An initialized empty namespace must remain empty until this explicit action.
	assertSourceListIDs(t, router, bob.Auth, nil)
	restoreRequest := httptest.NewRequest(http.MethodPost, "/api/sources/default/restore", nil)
	restoreRequest.Header.Set("Authorization", bob.Auth)
	restoreResponse := httptest.NewRecorder()
	router.ServeHTTP(restoreResponse, restoreRequest)
	if restoreResponse.Code != http.StatusOK {
		t.Fatalf("bob restore default = %d %s", restoreResponse.Code, restoreResponse.Body.String())
	}
	restored := sourceList(t, router, bob.Auth)
	if len(restored) != 1 || restored[0].Name != "管理员默认源" {
		t.Fatalf("bob restored sources = %+v", restored)
	}
}

func TestAdminBookSourceDefaultAndResetActionsAreTargetScoped(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	var admin models.User
	if err := server.db.Where("username = ?", "testuser").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&admin).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	alice := registerSourceContractAccount(t, router, "sourceadminalice")
	bob := registerSourceContractAccount(t, router, "sourceadminbob")
	empty := registerSourceContractAccount(t, router, "sourceadminempty")
	alice.Source = createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Alice 默认候选",
		"baseUrl":"https://admin-default-alice.example",
		"enabled":true
	}`)

	uninitializedRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/"+uintString(bob.ID)+"/sources/default",
		nil,
	)
	uninitializedRequest.Header.Set("Authorization", adminAuth)
	uninitializedResponse := httptest.NewRecorder()
	router.ServeHTTP(uninitializedResponse, uninitializedRequest)
	if uninitializedResponse.Code != http.StatusConflict ||
		!strings.Contains(uninitializedResponse.Body.String(), "user sources are not initialized") {
		t.Fatalf(
			"uninitialized target default = %d %s, want 409",
			uninitializedResponse.Code,
			uninitializedResponse.Body.String(),
		)
	}

	ordinaryRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/"+uintString(alice.ID)+"/sources/default",
		nil,
	)
	ordinaryRequest.Header.Set("Authorization", bob.Auth)
	ordinaryResponse := httptest.NewRecorder()
	router.ServeHTTP(ordinaryResponse, ordinaryRequest)
	if ordinaryResponse.Code != http.StatusForbidden {
		t.Fatalf("ordinary user set default = %d %s, want 403", ordinaryResponse.Code, ordinaryResponse.Body.String())
	}

	missingRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/99999999/sources/default",
		nil,
	)
	missingRequest.Header.Set("Authorization", adminAuth)
	missingResponse := httptest.NewRecorder()
	router.ServeHTTP(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("missing target default = %d %s, want 404", missingResponse.Code, missingResponse.Body.String())
	}

	setDefaultRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/"+uintString(alice.ID)+"/sources/default",
		nil,
	)
	setDefaultRequest.Header.Set("Authorization", adminAuth)
	setDefaultResponse := httptest.NewRecorder()
	router.ServeHTTP(setDefaultResponse, setDefaultRequest)
	if setDefaultResponse.Code != http.StatusOK ||
		!strings.Contains(setDefaultResponse.Body.String(), `"count":1`) {
		t.Fatalf("set alice sources as default = %d %s", setDefaultResponse.Code, setDefaultResponse.Body.String())
	}

	aliceClient := server.hub.AddClient(alice.ID, nil)
	bobClient := server.hub.AddClient(bob.ID, nil)
	t.Cleanup(func() {
		server.hub.RemoveClient(aliceClient)
		server.hub.RemoveClient(bobClient)
	})
	resetRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/sources/reset",
		strings.NewReader(`{"ids":[`+uintString(bob.ID)+`,`+uintString(bob.ID)+`]}`),
	)
	resetRequest.Header.Set("Authorization", adminAuth)
	resetRequest.Header.Set("Content-Type", "application/json")
	resetResponse := httptest.NewRecorder()
	router.ServeHTTP(resetResponse, resetRequest)
	if resetResponse.Code != http.StatusOK ||
		!strings.Contains(resetResponse.Body.String(), `"reset":1`) {
		t.Fatalf("reset bob sources = %d %s", resetResponse.Code, resetResponse.Body.String())
	}
	var bobActiveCount int64
	if err := server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND detached = ?", bob.ID, false).
		Count(&bobActiveCount).Error; err != nil {
		t.Fatal(err)
	}
	if bobActiveCount != 1 {
		t.Fatalf("bob active source count after reset = %d, want 1", bobActiveCount)
	}
	if !queuedPayloadContains(bobClient.Send, `"type":"sources_update"`) {
		t.Fatal("bob did not receive his reset sources_update")
	}
	if queuedPayloadContains(aliceClient.Send, `"type":"sources_update"`) {
		t.Fatal("alice received bob's reset sources_update")
	}

	createSourceThroughAPI(t, router, bob.Auth, `{
		"name":"Bob 重置前私有源",
		"baseUrl":"https://admin-reset-bob-private.example",
		"enabled":true
	}`)
	missingBatchRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/sources/reset",
		strings.NewReader(`{"ids":[`+uintString(bob.ID)+`,99999999]}`),
	)
	missingBatchRequest.Header.Set("Authorization", adminAuth)
	missingBatchRequest.Header.Set("Content-Type", "application/json")
	missingBatchResponse := httptest.NewRecorder()
	router.ServeHTTP(missingBatchResponse, missingBatchRequest)
	if missingBatchResponse.Code != http.StatusNotFound {
		t.Fatalf("reset batch with missing target = %d %s, want 404", missingBatchResponse.Code, missingBatchResponse.Body.String())
	}
	if err := server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND detached = ?", bob.ID, false).
		Count(&bobActiveCount).Error; err != nil {
		t.Fatal(err)
	}
	if bobActiveCount != 2 {
		t.Fatalf("missing-target batch partially reset bob: active=%d", bobActiveCount)
	}

	if err := server.db.Create(&models.BookSourceNamespace{UserID: empty.ID}).Error; err != nil {
		t.Fatal(err)
	}
	emptyDefaultRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/"+uintString(empty.ID)+"/sources/default",
		nil,
	)
	emptyDefaultRequest.Header.Set("Authorization", adminAuth)
	emptyDefaultResponse := httptest.NewRecorder()
	router.ServeHTTP(emptyDefaultResponse, emptyDefaultRequest)
	if emptyDefaultResponse.Code != http.StatusOK ||
		!strings.Contains(emptyDefaultResponse.Body.String(), `"count":0`) {
		t.Fatalf("set empty target as default = %d %s", emptyDefaultResponse.Code, emptyDefaultResponse.Body.String())
	}

	emptyResetRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/sources/reset",
		strings.NewReader(`{"ids":[`+uintString(bob.ID)+`]}`),
	)
	emptyResetRequest.Header.Set("Authorization", adminAuth)
	emptyResetRequest.Header.Set("Content-Type", "application/json")
	emptyResetResponse := httptest.NewRecorder()
	router.ServeHTTP(emptyResetResponse, emptyResetRequest)
	if emptyResetResponse.Code != http.StatusOK {
		t.Fatalf("reset to empty default = %d %s", emptyResetResponse.Code, emptyResetResponse.Body.String())
	}
	if err := server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND detached = ?", bob.ID, false).
		Count(&bobActiveCount).Error; err != nil {
		t.Fatal(err)
	}
	if bobActiveCount != 0 {
		t.Fatalf("bob active sources after empty reset = %d, want 0", bobActiveCount)
	}

	emptySelectionRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/users/sources/reset",
		strings.NewReader(`{"ids":[]}`),
	)
	emptySelectionRequest.Header.Set("Authorization", adminAuth)
	emptySelectionRequest.Header.Set("Content-Type", "application/json")
	emptySelectionResponse := httptest.NewRecorder()
	router.ServeHTTP(emptySelectionResponse, emptySelectionRequest)
	if emptySelectionResponse.Code != http.StatusBadRequest {
		t.Fatalf("empty reset selection = %d %s, want 400", emptySelectionResponse.Code, emptySelectionResponse.Body.String())
	}
}

func TestSearchAndExploreResolveOnlyCallerActiveSources(t *testing.T) {
	router, _ := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "sourcesearchalice")
	bob := registerSourceContractAccount(t, router, "sourcesearchbob")
	alice.Source = createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Alice 搜索探索源",
		"baseUrl":"https://alice-runtime.example",
		"searchUrl":"https://alice-runtime.example/search?q={keyword}",
		"rules":"{\"searchUrl\":\"https://alice-runtime.example/search?q={keyword}\",\"exploreUrl\":\"分类::https://alice-runtime.example/explore\",\"bookListRule\":\".book\",\"bookNameRule\":\".name|text\",\"bookURLRule\":\".name|attr:href\"}",
		"enabled":true,
		"enabledExplore":true
	}`)
	bob.Source = createSourceThroughAPI(t, router, bob.Auth, `{
		"name":"Bob 搜索探索源",
		"baseUrl":"https://bob-runtime.example",
		"searchUrl":"https://bob-runtime.example/search?q={keyword}",
		"rules":"{\"searchUrl\":\"https://bob-runtime.example/search?q={keyword}\",\"exploreUrl\":\"分类::https://bob-runtime.example/explore\",\"bookListRule\":\".book\",\"bookNameRule\":\".name|text\",\"bookURLRule\":\".name|attr:href\"}",
		"enabled":true,
		"enabledExplore":true
	}`)
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<div class="book"><a class="name" href="/book">结果</a></div>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	exploreListRequest := httptest.NewRequest(http.MethodGet, "/api/explore/sources", nil)
	exploreListRequest.Header.Set("Authorization", bob.Auth)
	exploreListResponse := httptest.NewRecorder()
	router.ServeHTTP(exploreListResponse, exploreListRequest)
	if exploreListResponse.Code != http.StatusOK {
		t.Fatalf("bob explore list = %d %s", exploreListResponse.Code, exploreListResponse.Body.String())
	}
	var exploreSources []exploreSourceResponse
	if err := json.Unmarshal(exploreListResponse.Body.Bytes(), &exploreSources); err != nil {
		t.Fatal(err)
	}
	if len(exploreSources) != 1 || exploreSources[0].ID != bob.Source.ID {
		t.Fatalf("bob explore list leaked alice: %+v", exploreSources)
	}

	foreignExploreRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/explore/"+uintString(alice.Source.ID)+"?url=https://alice-runtime.example/explore",
		nil,
	)
	foreignExploreRequest.Header.Set("Authorization", bob.Auth)
	foreignExploreResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignExploreResponse, foreignExploreRequest)
	if foreignExploreResponse.Code != http.StatusNotFound {
		t.Fatalf("bob explored alice source = %d %s, want 404", foreignExploreResponse.Code, foreignExploreResponse.Body.String())
	}

	foreignSearchRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/search",
		strings.NewReader(`{"keyword":"隔离","sourceIds":[`+uintString(alice.Source.ID)+`]}`),
	)
	foreignSearchRequest.Header.Set("Authorization", bob.Auth)
	foreignSearchRequest.Header.Set("Content-Type", "application/json")
	foreignSearchResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignSearchResponse, foreignSearchRequest)
	if foreignSearchResponse.Code != http.StatusBadRequest ||
		!strings.Contains(foreignSearchResponse.Body.String(), "未配置书源") {
		t.Fatalf("bob searched alice source = %d %s, want empty scoped selection", foreignSearchResponse.Code, foreignSearchResponse.Body.String())
	}
}

func TestRemoteBookReaderCandidatesAndRefreshRejectForeignSources(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "sourceruntimealice")
	bob := registerSourceContractAccount(t, router, "sourceruntimebob")
	alice.Source = createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Alice 运行时源",
		"baseUrl":"https://alice-book-runtime.example",
		"searchUrl":"https://alice-book-runtime.example/search?q={keyword}",
		"rules":"{\"searchUrl\":\"https://alice-book-runtime.example/search?q={keyword}\",\"bookListRule\":\".book\",\"bookNameRule\":\".name|text\",\"bookURLRule\":\".name|attr:href\",\"chapterListRule\":\".chapter\",\"chapterNameRule\":\".chapter|text\",\"chapterURLRule\":\".chapter|attr:href\",\"contentRule\":\".content|text\"}",
		"enabled":true
	}`)
	bob.Source = createSourceThroughAPI(t, router, bob.Auth, `{
		"name":"Bob 运行时源",
		"baseUrl":"https://bob-book-runtime.example",
		"searchUrl":"https://bob-book-runtime.example/search?q={keyword}",
		"rules":"{\"searchUrl\":\"https://bob-book-runtime.example/search?q={keyword}\",\"bookListRule\":\".book\",\"bookNameRule\":\".name|text\",\"bookURLRule\":\".name|attr:href\",\"chapterListRule\":\".chapter\",\"chapterNameRule\":\".chapter|text\",\"chapterURLRule\":\".chapter|attr:href\",\"contentRule\":\".content|text\"}",
		"enabled":true
	}`)
	var requestMu sync.Mutex
	requestHosts := make([]string, 0)
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestMu.Lock()
			requestHosts = append(requestHosts, request.URL.Host)
			requestMu.Unlock()
			body := `<div class="book"><a class="name" href="/book">候选书</a></div>
				<a class="chapter" href="/chapter/1">第一章</a>
				<div class="content">正文</div>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	bobBook := models.Book{UserID: bob.ID, Title: "Bob 候选书", URL: "local://candidate"}
	if err := server.db.Create(&bobBook).Error; err != nil {
		t.Fatal(err)
	}
	candidatesRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+uintString(bobBook.ID)+"/source-candidates?paged=true&limit=10",
		nil,
	)
	candidatesRequest.Header.Set("Authorization", bob.Auth)
	candidatesResponse := httptest.NewRecorder()
	router.ServeHTTP(candidatesResponse, candidatesRequest)
	if candidatesResponse.Code != http.StatusOK ||
		!strings.Contains(candidatesResponse.Body.String(), `"searched":1`) ||
		!strings.Contains(candidatesResponse.Body.String(), `"total":1`) {
		t.Fatalf("bob candidates used a foreign source: %d %s", candidatesResponse.Code, candidatesResponse.Body.String())
	}
	requestMu.Lock()
	hostsAfterCandidates := append([]string(nil), requestHosts...)
	requestMu.Unlock()
	if len(hostsAfterCandidates) != 1 || hostsAfterCandidates[0] != "bob-book-runtime.example" {
		t.Fatalf("candidate requests crossed source namespaces: %v", hostsAfterCandidates)
	}

	changeRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/books/"+uintString(bobBook.ID)+"/change-source",
		strings.NewReader(`{"sourceId":`+uintString(alice.Source.ID)+`,"bookUrl":"https://alice-book-runtime.example/book"}`),
	)
	changeRequest.Header.Set("Authorization", bob.Auth)
	changeRequest.Header.Set("Content-Type", "application/json")
	changeResponse := httptest.NewRecorder()
	router.ServeHTTP(changeResponse, changeRequest)
	if changeResponse.Code != http.StatusBadRequest ||
		!strings.Contains(changeResponse.Body.String(), "source not found") {
		t.Fatalf("bob changed to alice source = %d %s", changeResponse.Code, changeResponse.Body.String())
	}

	remoteBookRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/books/remote",
		strings.NewReader(`{"title":"越权书","bookUrl":"https://alice-book-runtime.example/book","sourceId":`+uintString(alice.Source.ID)+`}`),
	)
	remoteBookRequest.Header.Set("Authorization", bob.Auth)
	remoteBookRequest.Header.Set("Content-Type", "application/json")
	remoteBookResponse := httptest.NewRecorder()
	router.ServeHTTP(remoteBookResponse, remoteBookRequest)
	if remoteBookResponse.Code != http.StatusBadRequest ||
		!strings.Contains(remoteBookResponse.Body.String(), "source not found") {
		t.Fatalf("bob created book with alice source = %d %s", remoteBookResponse.Code, remoteBookResponse.Body.String())
	}

	remoteReaderRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/reader/remote-sessions",
		strings.NewReader(`{"title":"越权临时阅读","bookUrl":"https://alice-book-runtime.example/book","sourceId":`+uintString(alice.Source.ID)+`}`),
	)
	remoteReaderRequest.Header.Set("Authorization", bob.Auth)
	remoteReaderRequest.Header.Set("Content-Type", "application/json")
	remoteReaderResponse := httptest.NewRecorder()
	router.ServeHTTP(remoteReaderResponse, remoteReaderRequest)
	if remoteReaderResponse.Code != http.StatusNotFound {
		t.Fatalf("bob opened reader with alice source = %d %s", remoteReaderResponse.Code, remoteReaderResponse.Body.String())
	}

	corruptBook := models.Book{
		UserID: bob.ID, SourceID: alice.Source.ID, Title: "遗留越权引用",
		URL: "https://alice-book-runtime.example/book",
	}
	if err := server.db.Create(&corruptBook).Error; err != nil {
		t.Fatal(err)
	}
	contentSearchRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+uintString(corruptBook.ID)+"/search?q=%E6%AD%A3%E6%96%87&paged=1",
		nil,
	)
	contentSearchRequest.Header.Set("Authorization", bob.Auth)
	contentSearchResponse := httptest.NewRecorder()
	router.ServeHTTP(contentSearchResponse, contentSearchRequest)
	if contentSearchResponse.Code != http.StatusBadRequest ||
		!strings.Contains(contentSearchResponse.Body.String(), "未配置书源") {
		t.Fatalf(
			"bob searched content through alice source = %d %s",
			contentSearchResponse.Code,
			contentSearchResponse.Body.String(),
		)
	}

	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/books/"+uintString(corruptBook.ID)+"/refresh", nil)
	refreshRequest.Header.Set("Authorization", bob.Auth)
	refreshResponse := httptest.NewRecorder()
	router.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusBadRequest ||
		!strings.Contains(refreshResponse.Body.String(), "source not found") {
		t.Fatalf("bob refreshed through alice source = %d %s", refreshResponse.Code, refreshResponse.Body.String())
	}

	requestMu.Lock()
	defer requestMu.Unlock()
	if len(requestHosts) != len(hostsAfterCandidates) {
		t.Fatalf("foreign source endpoints performed remote requests: before=%v after=%v", hostsAfterCandidates, requestHosts)
	}
}

func registerSourceContractAccount(t *testing.T, router *gin.Engine, username string) sourceContractAccount {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/register",
		strings.NewReader(`{"username":"`+username+`","password":"source-contract-123"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register %s = %d %s", username, response.Code, response.Body.String())
	}
	var payload struct {
		Token string      `json:"token"`
		User  models.User `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Token == "" || payload.User.ID == 0 {
		t.Fatalf("register %s returned incomplete credentials: %+v", username, payload)
	}
	return sourceContractAccount{ID: payload.User.ID, Auth: "Bearer " + payload.Token}
}

func createSourceThroughAPI(t *testing.T, router *gin.Engine, auth, body string) models.BookSource {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create source = %d %s", response.Code, response.Body.String())
	}
	var source models.BookSource
	if err := json.Unmarshal(response.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	return source
}

func assertSourceListIDs(t *testing.T, router *gin.Engine, auth string, expected []uint) {
	t.Helper()
	sources := sourceList(t, router, auth)
	actual := make([]uint, 0, len(sources))
	for _, source := range sources {
		actual = append(actual, source.ID)
	}
	if len(actual) != len(expected) {
		t.Fatalf("source ids = %v, want %v", actual, expected)
	}
	for index := range actual {
		if actual[index] != expected[index] {
			t.Fatalf("source ids = %v, want %v", actual, expected)
		}
	}
}

func sourceList(t *testing.T, router *gin.Engine, auth string) []models.BookSource {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list sources = %d %s", response.Code, response.Body.String())
	}
	var sources []models.BookSource
	if err := json.Unmarshal(response.Body.Bytes(), &sources); err != nil {
		t.Fatal(err)
	}
	return sources
}

func importSourcesThroughAPI(t *testing.T, router *gin.Engine, auth, data string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bookSources.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/sources/import", &body)
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func queuedPayloadContains(channel <-chan []byte, needle string) bool {
	found := false
	for {
		select {
		case payload := <-channel:
			if strings.Contains(string(payload), needle) {
				found = true
			}
		default:
			return found
		}
	}
}
