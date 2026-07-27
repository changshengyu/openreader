package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

type sourceContractAccount struct {
	ID     uint
	Auth   string
	Source models.BookSource
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
