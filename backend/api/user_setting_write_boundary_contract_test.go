package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"openreader/backend/models"
)

func TestUserSettingWriteBoundaryRejectsOversizeBeforePersistence(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)
	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)

	for _, key := range []string{"reader", "shelf", "search"} {
		for _, transfer := range []string{"declared", "chunked"} {
			t.Run(key+"/"+transfer, func(t *testing.T) {
				baseline := replaceSettingContractRow(t, server, user.ID, key, `{"marker":"baseline"}`)
				drainSettingContractEvents(client.Send)
				body := makeSettingContractBody(t, int(maxUserSettingRequestBodyBytes)+1, "oversize", `"},"force":true}`)
				request := httptest.NewRequest(http.MethodPut, "/api/settings/"+key, strings.NewReader(body))
				request.Header.Set("Authorization", auth)
				request.Header.Set("Content-Type", "application/json")
				if transfer == "chunked" {
					request.ContentLength = -1
					request.TransferEncoding = []string{"chunked"}
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)

				assertSettingContractError(t, response, http.StatusRequestEntityTooLarge, `{"error":"request body too large"}`)
				assertSettingContractRowUnchanged(t, server, baseline)
				if events := drainSettingContractEvents(client.Send); len(events) != 0 {
					t.Errorf("oversize request broadcast events: %q", events)
				}
			})
		}
	}
}

func TestUserSettingWriteBoundaryRejectsTrailingValueAndGarbage(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)
	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)

	for _, key := range []string{"reader", "shelf", "search"} {
		for _, suffix := range []struct {
			name  string
			value string
		}{
			{name: "second-json", value: ` {"second":true}`},
			{name: "garbage", value: ` trailing-garbage`},
		} {
			t.Run(key+"/"+suffix.name, func(t *testing.T) {
				baseline := replaceSettingContractRow(t, server, user.ID, key, `{"marker":"baseline"}`)
				drainSettingContractEvents(client.Send)
				body := `{"value":{"marker":"attempt"},"force":true}` + suffix.value
				request := httptest.NewRequest(http.MethodPut, "/api/settings/"+key, strings.NewReader(body))
				request.Header.Set("Authorization", auth)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)

				assertSettingContractError(t, response, http.StatusBadRequest, `{"error":"setting value is required"}`)
				assertSettingContractRowUnchanged(t, server, baseline)
				if events := drainSettingContractEvents(client.Send); len(events) != 0 {
					t.Errorf("trailing data request broadcast events: %q", events)
				}
			})
		}
	}
}

func TestUserSettingWriteBoundaryAcceptsExactLimitWhitespaceAndUnknownEnvelope(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)
	client := server.hub.AddClient(user.ID, nil)
	defer server.hub.RemoveClient(client)

	suffix := `"},"force":true,"unknownEnvelope":{"ignored":true}}` + "\n \t"
	body := makeSettingContractBody(t, int(maxUserSettingRequestBodyBytes), "exact-limit", suffix)
	request := httptest.NewRequest(http.MethodPut, "/api/settings/shelf", strings.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("exact-limit setting: expected 200, got %s", compactSettingContractResponse(response))
	}

	var setting models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", user.ID, "shelf").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	var value struct {
		Marker  string `json:"marker"`
		Padding string `json:"padding"`
	}
	if err := json.Unmarshal([]byte(setting.Value), &value); err != nil {
		t.Fatal(err)
	}
	wantPadding := int(maxUserSettingRequestBodyBytes) - len(`{"value":{"marker":"exact-limit","padding":"`) - len(suffix)
	if value.Marker != "exact-limit" || len(value.Padding) != wantPadding {
		t.Fatalf("exact-limit setting was not persisted intact: marker=%q padding=%d want=%d", value.Marker, len(value.Padding), wantPadding)
	}
	events := drainSettingContractEvents(client.Send)
	if len(events) != 1 || !strings.Contains(events[0], `"type":"settings_update"`) || !strings.Contains(events[0], `"key":"shelf"`) {
		t.Fatalf("exact-limit write events = %q, want one shelf settings_update", events)
	}
}

func TestUserSettingWriteBoundaryPreservesAuthAndKeyPriority(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)
	other := models.User{Username: "setting-boundary-other", PasswordHash: "hash"}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherBaseline := replaceSettingContractRow(t, server, other.ID, "reader", `{"marker":"other-user"}`)
	body := makeSettingContractBody(t, int(maxUserSettingRequestBodyBytes)+1, "priority", `"},"force":true}`)

	t.Run("authentication-before-body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/settings/reader", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertSettingContractError(t, response, http.StatusUnauthorized, `{"error":"missing bearer token"}`)
	})

	t.Run("key-before-body", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/settings/not-a-setting", strings.NewReader(body))
		request.Header.Set("Authorization", auth)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertSettingContractError(t, response, http.StatusBadRequest, `{"error":"invalid setting key"}`)
	})

	assertSettingContractRowUnchanged(t, server, otherBaseline)
	var currentUserRows int64
	if err := server.db.Model(&models.UserSetting{}).Where("user_id = ?", user.ID).Count(&currentUserRows).Error; err != nil {
		t.Fatal(err)
	}
	if currentUserRows != 0 {
		t.Fatalf("priority failures created %d current-user settings", currentUserRows)
	}
}

func TestUserSettingWriteBoundaryPreservesJSONValueKindsAndReaderSanitizing(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)

	values := []struct {
		name  string
		value string
	}{
		{name: "object", value: `{"kind":"object"}`},
		{name: "array", value: `["array",1]`},
		{name: "string", value: `"string"`},
		{name: "number", value: `42`},
		{name: "boolean", value: `true`},
		{name: "null", value: `null`},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			body := `{"value":` + test.value + `,"force":true}`
			response := putSettingBoundaryContract(t, router, auth, "search", body)
			assertSettingContractResponseValue(t, response.Body.Bytes(), test.value)

			request := httptest.NewRequest(http.MethodGet, "/api/settings/search", nil)
			request.Header.Set("Authorization", auth)
			loaded := httptest.NewRecorder()
			router.ServeHTTP(loaded, request)
			if loaded.Code != http.StatusOK {
				t.Fatalf("get %s setting: %s", test.name, compactSettingContractResponse(loaded))
			}
			assertSettingContractResponseValue(t, loaded.Body.Bytes(), test.value)
		})
	}

	readerValue := `{"pageMode":"device","miniInterface":true,"nested":{"pageMode":"keep","miniInterface":true},"custom":"keep"}`
	response := putSettingBoundaryContract(t, router, auth, "reader", `{"value":`+readerValue+`,"force":true}`)
	var payload struct {
		Value map[string]any `json:"value"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	nested, _ := payload.Value["nested"].(map[string]any)
	if payload.Value["pageMode"] != nil || payload.Value["miniInterface"] != nil || payload.Value["custom"] != "keep" || nested["pageMode"] != "keep" || nested["miniInterface"] != true {
		t.Fatalf("reader sanitizing changed the compatibility boundary: %#v", payload.Value)
	}

	var rows int64
	if err := server.db.Model(&models.UserSetting{}).Where("user_id = ?", user.ID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("value-kind round trips created %d rows, want search and reader", rows)
	}
}

func TestUserSettingWriteBoundaryPreservesHistoricalLargeRowsAcrossGetBackupRestore(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := loadSettingContractUser(t, server)
	value := `{"marker":"historical","padding":"` + strings.Repeat("h", int(maxUserSettingRequestBodyBytes)) + `"}`
	if err := server.db.Create(&models.UserSetting{UserID: user.ID, Key: "shelf", Value: value}).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/settings/shelf", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get historical large setting: %s", compactSettingContractResponse(response))
	}
	assertSettingContractLargeValue(t, response.Body.Bytes(), "historical", int(maxUserSettingRequestBodyBytes))

	path, err := server.backupSvc.RunNowForUser(user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	entries := readFixedBaselineBackupEntries(t, path)
	settingsData := entries["userSettings.json"]
	var exported []models.UserSetting
	if err := json.Unmarshal(settingsData, &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Key != "shelf" || exported[0].Value != value {
		t.Fatalf("logical backup changed historical large setting: rows=%d key=%q value-bytes=%d", len(exported), firstSettingContractKey(exported), firstSettingContractValueLength(exported))
	}

	destination := models.User{Username: "setting-boundary-restore", PasswordHash: "hash"}
	if err := server.db.Create(&destination).Error; err != nil {
		t.Fatal(err)
	}
	count, err := server.restoreUserSettingsFromData(settingsData, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restored historical setting count = %d, want 1", count)
	}
	var restored models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", destination.ID, "shelf").First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.Value != value {
		t.Fatalf("restore changed historical large setting: got %d bytes, want %d", len(restored.Value), len(value))
	}
}

func makeSettingContractBody(t *testing.T, totalBytes int, marker string, suffix string) string {
	t.Helper()
	prefix := `{"value":{"marker":"` + marker + `","padding":"`
	paddingBytes := totalBytes - len(prefix) - len(suffix)
	if paddingBytes < 0 {
		t.Fatalf("setting contract body target %d is smaller than framing %d", totalBytes, len(prefix)+len(suffix))
	}
	body := prefix + strings.Repeat("x", paddingBytes) + suffix
	if len(body) != totalBytes {
		t.Fatalf("setting contract body bytes = %d, want %d", len(body), totalBytes)
	}
	return body
}

func loadSettingContractUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func replaceSettingContractRow(t *testing.T, server *Server, userID uint, key string, value string) models.UserSetting {
	t.Helper()
	if err := server.db.Where("user_id = ? AND key = ?", userID, key).Delete(&models.UserSetting{}).Error; err != nil {
		t.Fatal(err)
	}
	setting := models.UserSetting{
		UserID:    userID,
		Key:       key,
		Value:     value,
		UpdatedAt: time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC),
	}
	if err := server.db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	return setting
}

func assertSettingContractRowUnchanged(t *testing.T, server *Server, baseline models.UserSetting) {
	t.Helper()
	var current models.UserSetting
	if err := server.db.Where("user_id = ? AND key = ?", baseline.UserID, baseline.Key).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.ID != baseline.ID || current.Value != baseline.Value || !current.UpdatedAt.Equal(baseline.UpdatedAt) {
		t.Errorf(
			"setting row changed after rejected request: before={id:%d value-bytes:%d value-prefix:%q updated:%s} after={id:%d value-bytes:%d value-prefix:%q updated:%s}",
			baseline.ID,
			len(baseline.Value),
			settingContractValuePrefix(baseline.Value),
			baseline.UpdatedAt.Format(time.RFC3339Nano),
			current.ID,
			len(current.Value),
			settingContractValuePrefix(current.Value),
			current.UpdatedAt.Format(time.RFC3339Nano),
		)
	}
}

func assertSettingContractError(t *testing.T, response *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if response.Code != status || !bytes.Equal(response.Body.Bytes(), []byte(body)) {
		t.Errorf("setting boundary response: got %s, want status=%d body=%s", compactSettingContractResponse(response), status, body)
	}
}

func compactSettingContractResponse(response *httptest.ResponseRecorder) string {
	body := response.Body.Bytes()
	if len(body) > 160 {
		body = body[:160]
	}
	return fmt.Sprintf("status=%d body-bytes=%d body-prefix=%q", response.Code, response.Body.Len(), body)
}

func settingContractValuePrefix(value string) string {
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func drainSettingContractEvents(channel <-chan []byte) []string {
	var events []string
	for {
		select {
		case payload := <-channel:
			events = append(events, string(payload))
		default:
			return events
		}
	}
}

func putSettingBoundaryContract(t *testing.T, handler http.Handler, auth string, key string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/api/settings/"+key, strings.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("put %s setting: %s", key, compactSettingContractResponse(response))
	}
	return response
}

func assertSettingContractResponseValue(t *testing.T, data []byte, expectedJSON string) {
	t.Helper()
	var payload struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(payload.Value, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(expectedJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("setting response value = %#v, want %#v", got, want)
	}
}

func assertSettingContractLargeValue(t *testing.T, data []byte, marker string, paddingBytes int) {
	t.Helper()
	var payload struct {
		Value struct {
			Marker  string `json:"marker"`
			Padding string `json:"padding"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Value.Marker != marker || len(payload.Value.Padding) != paddingBytes {
		t.Fatalf("large setting response marker=%q padding=%d, want marker=%q padding=%d", payload.Value.Marker, len(payload.Value.Padding), marker, paddingBytes)
	}
}

func firstSettingContractKey(settings []models.UserSetting) string {
	if len(settings) == 0 {
		return ""
	}
	return settings[0].Key
}

func firstSettingContractValueLength(settings []models.UserSetting) int {
	if len(settings) == 0 {
		return 0
	}
	return len(settings[0].Value)
}
