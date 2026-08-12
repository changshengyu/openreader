package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"openreader/backend/models"
)

type adminUserWriteFixture struct {
	router        *gin.Engine
	server        *Server
	adminAuth     string
	administrator models.User
	target        models.User
	method        string
	path          string
	body          string
	assertStable  func(*testing.T)
	adminEvents   <-chan []byte
	targetEvents  <-chan []byte
}

func newAdminUserWriteFixture(t *testing.T, route string) adminUserWriteFixture {
	t.Helper()
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	var administrator models.User
	if err := server.db.Where("username = ?", "testuser").First(&administrator).Error; err != nil {
		t.Fatalf("load administrator: %v", err)
	}
	target := createUserManagementP2User(t, server, "boundarytarget", time.Now())
	fixture := adminUserWriteFixture{
		router:        router,
		server:        server,
		adminAuth:     adminAuth,
		administrator: administrator,
		target:        target,
	}

	switch route {
	case "create":
		fixture.method = http.MethodPost
		fixture.path = "/api/admin/users"
		fixture.body = `{"username":"boundarycreated","password":"password8"}`
		fixture.assertStable = func(t *testing.T) {
			var count int64
			if err := server.db.Model(&models.User{}).Where("username = ?", "boundarycreated").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("failed create persisted %d user rows", count)
			}
		}
	case "update":
		fixture.method = http.MethodPut
		fixture.path = "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10)
		fixture.body = `{"bookLimit":7}`
		fixture.assertStable = func(t *testing.T) {
			var reloaded models.User
			if err := server.db.First(&reloaded, target.ID).Error; err != nil {
				t.Fatal(err)
			}
			if reloaded.BookLimit != target.BookLimit {
				t.Errorf("failed update changed book limit to %d", reloaded.BookLimit)
			}
		}
	case "password":
		fixture.method = http.MethodPut
		fixture.path = "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10) + "/password"
		fixture.body = `{"password":"changed88"}`
		fixture.assertStable = func(t *testing.T) {
			var reloaded models.User
			if err := server.db.First(&reloaded, target.ID).Error; err != nil {
				t.Fatal(err)
			}
			if reloaded.PasswordHash != target.PasswordHash {
				t.Errorf("failed password reset changed the stored hash")
			}
		}
	case "reset-sources":
		fixture.method = http.MethodPost
		fixture.path = "/api/admin/users/sources/reset"
		fixture.body = fmt.Sprintf(`{"ids":[%d]}`, target.ID)
		if err := server.db.Create(&models.BookSource{Name: "boundary-default", BaseURL: "https://default.example", Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
		saved := adminContractRequest(router, http.MethodPost, "/api/sources/default/save", "", adminAuth)
		if saved.Code != http.StatusOK {
			t.Fatalf("save default sources: %d %s", saved.Code, saved.Body.String())
		}
		if err := server.db.Create(&models.BookSource{Name: "boundary-extra", BaseURL: "https://extra.example", Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
		var before int64
		if err := server.db.Model(&models.UserBookSource{}).Where("user_id = ?", target.ID).Count(&before).Error; err != nil {
			t.Fatal(err)
		}
		fixture.assertStable = func(t *testing.T) {
			var after int64
			if err := server.db.Model(&models.UserBookSource{}).Where("user_id = ?", target.ID).Count(&after).Error; err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Errorf("failed source reset changed target associations from %d to %d", before, after)
			}
		}
	case "batch-delete":
		fixture.method = http.MethodPost
		fixture.path = "/api/admin/users/batch-delete"
		fixture.body = fmt.Sprintf(`{"ids":[%d]}`, target.ID)
		fixture.assertStable = func(t *testing.T) {
			var count int64
			if err := server.db.Model(&models.User{}).Where("id = ?", target.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Errorf("failed batch delete removed target user")
			}
		}
	default:
		t.Fatalf("unknown fixture route %q", route)
	}

	fixture.adminEvents = server.hub.AddClient(administrator.ID, nil).Send
	fixture.targetEvents = server.hub.AddClient(target.ID, nil).Send
	return fixture
}

func adminUserWriteRequest(fixture adminUserWriteFixture, body string, chunked bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(fixture.method, fixture.path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fixture.adminAuth)
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func assertAdminUserWriteError(t *testing.T, response *httptest.ResponseRecorder, status int, code, message string, secrets ...string) {
	t.Helper()
	if response.Code != status {
		t.Errorf("status = %d, want %d: %s", response.Code, status, response.Body.String())
		return
	}
	var payload struct {
		Error apiError `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Errorf("decode error response: %v: %s", err, response.Body.String())
		return
	}
	if payload.Error.Code != code || payload.Error.Message != message {
		t.Errorf("error = %+v, want code %q message %q", payload.Error, code, message)
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(response.Body.String(), secret) {
			t.Errorf("error response leaked submitted secret")
		}
	}
}

func assertAdminUserWriteFailureStable(t *testing.T, fixture adminUserWriteFixture) {
	t.Helper()
	fixture.assertStable(t)
	if queuedPayloadContains(fixture.adminEvents, "users_update") || queuedPayloadContains(fixture.adminEvents, "sources_update") {
		t.Errorf("failed request broadcast an administrator sync event")
	}
	if queuedPayloadContains(fixture.targetEvents, "users_update") || queuedPayloadContains(fixture.targetEvents, "sources_update") {
		t.Errorf("failed request broadcast a target sync event")
	}
}

func assertAdminUserWriteUserMissing(t *testing.T, server *Server, username string) {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("failed request persisted username %q", username)
	}
}

func withAdminUserWritePadding(body string, total int) string {
	prefix := strings.TrimSuffix(body, "}") + `,"padding":"`
	suffix := `"}`
	return prefix + strings.Repeat("x", total-len(prefix)-len(suffix)) + suffix
}

func TestAdminUserWriteBoundaryRejectsDeclaredAndChunkedOversizedBodies(t *testing.T) {
	for _, route := range []string{"create", "update", "password", "reset-sources", "batch-delete"} {
		for _, chunked := range []bool{false, true} {
			name := "declared"
			if chunked {
				name = "chunked"
			}
			t.Run(route+"/"+name, func(t *testing.T) {
				fixture := newAdminUserWriteFixture(t, route)
				body := withAdminUserWritePadding(fixture.body, maxAdminUserRequestBodyBytes+1)
				response := adminUserWriteRequest(fixture, body, chunked)
				assertAdminUserWriteError(t, response, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large")
				assertAdminUserWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestAdminUserWriteBoundaryRejectsTrailingJSONValuesAndGarbage(t *testing.T) {
	for _, suffix := range []struct {
		name  string
		value string
	}{
		{name: "second-json", value: `{"ignored":true}`},
		{name: "garbage", value: `garbage`},
	} {
		for _, route := range []string{"create", "update", "password", "reset-sources", "batch-delete"} {
			t.Run(route+"/"+suffix.name, func(t *testing.T) {
				fixture := newAdminUserWriteFixture(t, route)
				response := adminUserWriteRequest(fixture, fixture.body+suffix.value, false)
				assertAdminUserWriteError(t, response, http.StatusBadRequest, "BAD_REQUEST", "invalid payload")
				assertAdminUserWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestAdminUserWriteBoundaryAcceptsExactLimitAndTrailingWhitespace(t *testing.T) {
	fixture := newAdminUserWriteFixture(t, "update")
	body := withAdminUserWritePadding(fixture.body, maxAdminUserRequestBodyBytes)
	response := adminUserWriteRequest(fixture, body+"\r\n\t", false)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("wire body including trailing whitespace must exceed the limit: %d %s", response.Code, response.Body.String())
	}

	fixture = newAdminUserWriteFixture(t, "update")
	body = withAdminUserWritePadding(fixture.body, maxAdminUserRequestBodyBytes-len("\r\n\t")) + "\r\n\t"
	response = adminUserWriteRequest(fixture, body, false)
	if response.Code != http.StatusOK {
		t.Fatalf("exact-limit body with trailing whitespace = %d, want 200: %s", response.Code, response.Body.String())
	}
}

func TestAdminUserWriteBoundaryLimitsRawBatchCardinality(t *testing.T) {
	for _, route := range []string{"reset-sources", "batch-delete"} {
		t.Run(route, func(t *testing.T) {
			fixture := newAdminUserWriteFixture(t, route)
			ids := make([]uint, 2001)
			for index := range ids {
				ids[index] = fixture.target.ID
			}
			encoded, err := json.Marshal(gin.H{"ids": ids})
			if err != nil {
				t.Fatal(err)
			}
			response := adminUserWriteRequest(fixture, string(encoded), false)
			assertAdminUserWriteError(t, response, http.StatusBadRequest, "BAD_REQUEST", "too many users selected")
			assertAdminUserWriteFailureStable(t, fixture)
		})
	}
}

func TestAdminUserWriteBoundaryAllowsMaximumRawBatchCardinality(t *testing.T) {
	for _, route := range []string{"reset-sources", "batch-delete"} {
		t.Run(route, func(t *testing.T) {
			fixture := newAdminUserWriteFixture(t, route)
			ids := make([]uint, 2000)
			for index := range ids {
				ids[index] = fixture.target.ID
			}
			encoded, err := json.Marshal(gin.H{"ids": ids})
			if err != nil {
				t.Fatal(err)
			}
			response := adminUserWriteRequest(fixture, string(encoded), false)
			if response.Code != http.StatusOK {
				t.Fatalf("2,000 raw ids = %d, want 200: %s", response.Code, response.Body.String())
			}
			if route == "reset-sources" {
				var count int64
				if err := fixture.server.db.Model(&models.UserBookSource{}).Where("user_id = ?", fixture.target.ID).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("2,000-id reset left %d target source associations, want 1", count)
				}
				return
			}
			var count int64
			if err := fixture.server.db.Model(&models.User{}).Where("id = ?", fixture.target.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("2,000-id delete left target user")
			}
		})
	}
}

func TestAdminUserWriteBoundaryAuthenticatesBeforeReadingOversizedBody(t *testing.T) {
	router, server := setupTestServer(t)
	authHeader(t, router)
	target := createUserManagementP2User(t, server, "prioritytarget", time.Now())
	memberAuth := registerStorageTestUser(t, router, "prioritymember")
	body := withAdminUserWritePadding(`{"ids":[]}`, maxAdminUserRequestBodyBytes+1)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/users"},
		{http.MethodPut, fmt.Sprintf("/api/admin/users/%d", target.ID)},
		{http.MethodPut, fmt.Sprintf("/api/admin/users/%d/password", target.ID)},
		{http.MethodPost, "/api/admin/users/sources/reset"},
		{http.MethodPost, "/api/admin/users/batch-delete"},
	}
	for _, route := range routes {
		t.Run(route.path+"/unauthenticated", func(t *testing.T) {
			response := adminContractRequest(router, route.method, route.path, body, "")
			if response.Code != http.StatusUnauthorized || response.Body.String() != `{"error":"missing bearer token"}` {
				t.Errorf("unauthenticated priority = %d %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), body) {
				t.Errorf("unauthenticated error leaked request body")
			}
		})
		t.Run(route.path+"/non-admin", func(t *testing.T) {
			response := adminContractRequest(router, route.method, route.path, body, memberAuth)
			assertAdminUserWriteError(t, response, http.StatusForbidden, "FORBIDDEN", "admin access required", memberAuth)
		})
	}
}

func TestNewPasswordBoundaryUsesUTF16CodeUnits(t *testing.T) {
	t.Run("public-register-rejects-six-code-units", func(t *testing.T) {
		router, server := setupTestServer(t)
		password := strings.Repeat("密码", 3)
		response := adminContractRequest(router, http.MethodPost, "/api/auth/register", fmt.Sprintf(`{"username":"unicodeuser","password":%q}`, password), "")
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "password must be at least 8 characters") {
			t.Errorf("register six UTF-16 units = %d: %s", response.Code, response.Body.String())
		}
		var count int64
		if err := server.db.Model(&models.User{}).Where("username = ?", "unicodeuser").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("short multibyte registration persisted a user")
		}
		if strings.Contains(response.Body.String(), password) {
			t.Errorf("registration error leaked password")
		}
	})

	t.Run("public-register-accepts-eight-code-units", func(t *testing.T) {
		router, _ := setupTestServer(t)
		password := strings.Repeat("密码", 4)
		response := adminContractRequest(router, http.MethodPost, "/api/auth/register", fmt.Sprintf(`{"username":"unicodeuser","password":%q}`, password), "")
		if response.Code != http.StatusOK {
			t.Fatalf("register eight UTF-16 units = %d: %s", response.Code, response.Body.String())
		}
	})

	for _, action := range []string{"create", "password"} {
		t.Run(action+"-rejects-six-code-units", func(t *testing.T) {
			fixture := newAdminUserWriteFixture(t, action)
			password := strings.Repeat("密码", 3)
			if action == "create" {
				fixture.body = fmt.Sprintf(`{"username":"shortunicode","password":%q}`, password)
			} else {
				fixture.body = fmt.Sprintf(`{"password":%q}`, password)
			}
			response := adminUserWriteRequest(fixture, fixture.body, false)
			assertAdminUserWriteError(t, response, http.StatusBadRequest, "BAD_REQUEST", "password must be at least 8 characters", password)
			if action == "create" {
				assertAdminUserWriteUserMissing(t, fixture.server, "shortunicode")
			} else {
				assertAdminUserWriteFailureStable(t, fixture)
			}
		})

		t.Run(action+"-accepts-eight-code-units", func(t *testing.T) {
			fixture := newAdminUserWriteFixture(t, action)
			password := strings.Repeat("密码", 4)
			if action == "create" {
				fixture.body = fmt.Sprintf(`{"username":"validunicode","password":%q}`, password)
			} else {
				fixture.body = fmt.Sprintf(`{"password":%q}`, password)
			}
			response := adminUserWriteRequest(fixture, fixture.body, false)
			want := http.StatusCreated
			if action == "password" {
				want = http.StatusOK
			}
			if response.Code != want {
				t.Fatalf("%s eight UTF-16 units = %d: %s", action, response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminPasswordBoundaryMapsBcryptLimitWithoutMutation(t *testing.T) {
	for _, action := range []string{"create", "password"} {
		for _, size := range []int{72, 73} {
			t.Run(fmt.Sprintf("%s/%d-bytes", action, size), func(t *testing.T) {
				fixture := newAdminUserWriteFixture(t, action)
				password := strings.Repeat("p", size)
				if action == "create" {
					fixture.body = fmt.Sprintf(`{"username":"bcryptboundary","password":%q}`, password)
				} else {
					fixture.body = fmt.Sprintf(`{"password":%q}`, password)
				}
				response := adminUserWriteRequest(fixture, fixture.body, false)
				if size == 73 {
					assertAdminUserWriteError(t, response, http.StatusBadRequest, "BAD_REQUEST", "password must be at most 72 bytes", password)
					if action == "create" {
						assertAdminUserWriteUserMissing(t, fixture.server, "bcryptboundary")
					} else {
						assertAdminUserWriteFailureStable(t, fixture)
					}
					return
				}
				want := http.StatusCreated
				if action == "password" {
					want = http.StatusOK
				}
				if response.Code != want {
					t.Fatalf("%s 72-byte password = %d: %s", action, response.Code, response.Body.String())
				}
				if action == "password" {
					var reloaded models.User
					if err := fixture.server.db.First(&reloaded, fixture.target.ID).Error; err != nil {
						t.Fatal(err)
					}
					if err := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(password)); err != nil {
						t.Fatalf("stored reset password does not match: %v", err)
					}
				}
			})
		}
	}
}

func TestAdminUserCreateRejectsNegativeLimitsWithoutPersisting(t *testing.T) {
	for _, field := range []string{"bookLimit", "sourceLimit"} {
		t.Run(field, func(t *testing.T) {
			fixture := newAdminUserWriteFixture(t, "create")
			fixture.body = fmt.Sprintf(`{"username":"negativelimit","password":"password8",%q:-1}`, field)
			response := adminUserWriteRequest(fixture, fixture.body, false)
			message := "book limit cannot be negative"
			if field == "sourceLimit" {
				message = "source limit cannot be negative"
			}
			assertAdminUserWriteError(t, response, http.StatusBadRequest, "BAD_REQUEST", message)
			assertAdminUserWriteUserMissing(t, fixture.server, "negativelimit")
		})
	}
}

func TestAdminPasswordResetPreservesTargetErrorsWithoutMutation(t *testing.T) {
	fixture := newAdminUserWriteFixture(t, "password")
	password := strings.Repeat("p", 72)
	fixture.path = "/api/admin/users/999999/password"
	response := adminUserWriteRequest(fixture, fmt.Sprintf(`{"password":%q}`, password), false)
	assertAdminUserWriteError(t, response, http.StatusNotFound, "NOT_FOUND", "user not found", password)
	assertAdminUserWriteFailureStable(t, fixture)

	fixture = newAdminUserWriteFixture(t, "password")
	fixture.path = fmt.Sprintf("/api/admin/users/%d/password", fixture.administrator.ID)
	administratorHash := fixture.administrator.PasswordHash
	response = adminUserWriteRequest(fixture, fmt.Sprintf(`{"password":%q}`, password), false)
	assertAdminUserWriteError(t, response, http.StatusForbidden, "FORBIDDEN", "protected administrator password cannot be reset", password)
	var administrator models.User
	if err := fixture.server.db.First(&administrator, fixture.administrator.ID).Error; err != nil {
		t.Fatal(err)
	}
	if administrator.PasswordHash != administratorHash {
		t.Errorf("protected administrator hash changed")
	}
	if queuedPayloadContains(fixture.adminEvents, "users_update") {
		t.Errorf("protected password reset broadcast an event")
	}
}
