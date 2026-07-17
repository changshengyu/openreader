package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

func registerBookInfoAssetUser(t *testing.T, router http.Handler, username string) (string, models.User) {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/register",
		strings.NewReader(`{"username":"`+username+`","password":"bookinfo-asset"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register %s: expected 200, got %d: %s", username, response.Code, response.Body.String())
	}
	var result struct {
		Token string      `json:"token"`
		User  models.User `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.User.ID == 0 {
		t.Fatalf("register %s returned incomplete identity: %+v", username, result)
	}
	return "Bearer " + result.Token, result.User
}

func uploadBookInfoCover(t *testing.T, router http.Handler, token string, filename string) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "cover"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("cover-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload cover: expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.URL == "" {
		t.Fatalf("upload cover returned no URL: %s", response.Body.String())
	}
	return result.URL
}

func TestBookInfoAssetsAreUserScopedAndCannotCrossMutate(t *testing.T) {
	router, server := setupTestServer(t)
	aliceToken, alice := registerBookInfoAssetUser(t, router, "assetalice")
	bobToken, bob := registerBookInfoAssetUser(t, router, "assetbob")

	aliceURL := uploadBookInfoCover(t, router, aliceToken, "alice.png")
	bobURL := uploadBookInfoCover(t, router, bobToken, "bob.png")
	for user, url := range map[models.User]string{alice: aliceURL, bob: bobURL} {
		prefix := "/uploads/users/" + strconv.FormatUint(uint64(user.ID), 10) + "/covers/"
		if !strings.HasPrefix(url, prefix) {
			t.Fatalf("user %d cover url = %q, want prefix %q", user.ID, url, prefix)
		}
		path := filepath.Join(server.cfg.DataDir, "uploads", strings.TrimPrefix(url, "/uploads/"))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("user %d uploaded file missing at %s: %v", user.ID, path, err)
		}
	}
	readAlice := httptest.NewRequest(http.MethodGet, aliceURL, nil)
	readAliceResponse := httptest.NewRecorder()
	router.ServeHTTP(readAliceResponse, readAlice)
	if readAliceResponse.Code != http.StatusOK || readAliceResponse.Body.String() != "cover-bytes" {
		t.Fatalf("user-scoped cover must remain directly readable: %d %q", readAliceResponse.Code, readAliceResponse.Body.String())
	}

	book := models.Book{UserID: alice.ID, SourceID: 1, Title: "资产归属书", CanUpdate: true}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	ownerCover := httptest.NewRequest(
		http.MethodPut,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10),
		strings.NewReader(`{"customCoverUrl":`+strconv.Quote(aliceURL)+`}`),
	)
	ownerCover.Header.Set("Content-Type", "application/json")
	ownerCover.Header.Set("Authorization", aliceToken)
	ownerCoverResponse := httptest.NewRecorder()
	router.ServeHTTP(ownerCoverResponse, ownerCover)
	if ownerCoverResponse.Code != http.StatusOK {
		t.Fatalf("owner cover assignment: expected 200, got %d: %s", ownerCoverResponse.Code, ownerCoverResponse.Body.String())
	}

	deleteReferenced := httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":`+strconv.Quote(aliceURL)+`}`))
	deleteReferenced.Header.Set("Content-Type", "application/json")
	deleteReferenced.Header.Set("Authorization", aliceToken)
	deleteReferencedResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteReferencedResponse, deleteReferenced)
	if deleteReferencedResponse.Code != http.StatusConflict {
		t.Fatalf("referenced upload delete: expected 409, got %d: %s", deleteReferencedResponse.Code, deleteReferencedResponse.Body.String())
	}
	if err := server.db.Create(&models.UserSetting{UserID: bob.ID, Key: "reader", Value: `{"customBgImage":` + strconv.Quote(bobURL) + `}`}).Error; err != nil {
		t.Fatal(err)
	}
	deleteSettingReferenced := httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":`+strconv.Quote(bobURL)+`}`))
	deleteSettingReferenced.Header.Set("Content-Type", "application/json")
	deleteSettingReferenced.Header.Set("Authorization", bobToken)
	deleteSettingReferencedResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteSettingReferencedResponse, deleteSettingReferenced)
	if deleteSettingReferencedResponse.Code != http.StatusConflict {
		t.Fatalf("settings-referenced upload delete: expected 409, got %d: %s", deleteSettingReferencedResponse.Code, deleteSettingReferencedResponse.Body.String())
	}

	foreignCover := httptest.NewRequest(
		http.MethodPut,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10),
		strings.NewReader(`{"customCoverUrl":`+strconv.Quote(bobURL)+`}`),
	)
	foreignCover.Header.Set("Content-Type", "application/json")
	foreignCover.Header.Set("Authorization", aliceToken)
	foreignCoverResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignCoverResponse, foreignCover)
	if foreignCoverResponse.Code != http.StatusBadRequest {
		t.Fatalf("foreign cover assignment: expected 400, got %d: %s", foreignCoverResponse.Code, foreignCoverResponse.Body.String())
	}
	for _, invalidURL := range []string{
		"https://example.invalid/new-cover.png",
		aliceURL + "?cache=1",
		"/uploads/users/" + strconv.FormatUint(uint64(alice.ID), 10) + "/covers/../other.png",
	} {
		invalidCover := httptest.NewRequest(
			http.MethodPut,
			"/api/books/"+strconv.FormatUint(uint64(book.ID), 10),
			strings.NewReader(`{"customCoverUrl":`+strconv.Quote(invalidURL)+`}`),
		)
		invalidCover.Header.Set("Content-Type", "application/json")
		invalidCover.Header.Set("Authorization", aliceToken)
		invalidCoverResponse := httptest.NewRecorder()
		router.ServeHTTP(invalidCoverResponse, invalidCover)
		if invalidCoverResponse.Code != http.StatusBadRequest {
			t.Fatalf("invalid cover %q: expected 400, got %d: %s", invalidURL, invalidCoverResponse.Code, invalidCoverResponse.Body.String())
		}
	}

	deleteVariant := httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":`+strconv.Quote(aliceURL+"#preview")+`}`))
	deleteVariant.Header.Set("Content-Type", "application/json")
	deleteVariant.Header.Set("Authorization", aliceToken)
	deleteVariantResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteVariantResponse, deleteVariant)
	if deleteVariantResponse.Code != http.StatusBadRequest {
		t.Fatalf("asset URL fragment: expected 400, got %d: %s", deleteVariantResponse.Code, deleteVariantResponse.Body.String())
	}

	deleteForeign := httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":`+strconv.Quote(aliceURL)+`}`))
	deleteForeign.Header.Set("Content-Type", "application/json")
	deleteForeign.Header.Set("Authorization", bobToken)
	deleteForeignResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteForeignResponse, deleteForeign)
	if deleteForeignResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-user asset delete: expected 404, got %d: %s", deleteForeignResponse.Code, deleteForeignResponse.Body.String())
	}
	if _, err := os.Stat(filepath.Join(server.cfg.DataDir, "uploads", strings.TrimPrefix(aliceURL, "/uploads/"))); err != nil {
		t.Fatalf("cross-user delete removed owner file: %v", err)
	}
}

func TestBookInfoLegacyCoverRemainsReadableAndCannotBeDeletedAsNewAsset(t *testing.T) {
	router, server := setupTestServer(t)
	token, user := registerBookInfoAssetUser(t, router, "assetlegacy")
	legacyPath := filepath.Join(server.cfg.DataDir, "uploads", "covers", "legacy-cover.png")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy-cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyURL := "/uploads/covers/legacy-cover.png"
	book := models.Book{UserID: user.ID, SourceID: 1, Title: "旧封面书", CustomCoverURL: legacyURL}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	readLegacy := httptest.NewRequest(http.MethodGet, legacyURL, nil)
	readLegacyResponse := httptest.NewRecorder()
	router.ServeHTTP(readLegacyResponse, readLegacy)
	if readLegacyResponse.Code != http.StatusOK || readLegacyResponse.Body.String() != "legacy-cover" {
		t.Fatalf("legacy cover must remain readable: %d %q", readLegacyResponse.Code, readLegacyResponse.Body.String())
	}

	deleteLegacy := httptest.NewRequest(http.MethodDelete, "/api/uploads", strings.NewReader(`{"url":"`+legacyURL+`"}`))
	deleteLegacy.Header.Set("Content-Type", "application/json")
	deleteLegacy.Header.Set("Authorization", token)
	deleteLegacyResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteLegacyResponse, deleteLegacy)
	if deleteLegacyResponse.Code != http.StatusBadRequest {
		t.Fatalf("legacy asset delete: expected 400, got %d: %s", deleteLegacyResponse.Code, deleteLegacyResponse.Body.String())
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy cover was deleted: %v", err)
	}
}
