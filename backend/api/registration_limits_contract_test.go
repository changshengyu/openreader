package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"openreader/backend/config"
	"openreader/backend/models"
)

// Contract: docs/compat/registration-invite-code-user-limit-fixed-baseline-second-audit-p1-contract.md.
// The limits come from the real environment loader, so these cases also pin the
// OPENREADER_INVITE_CODE / OPENREADER_USER_LIMIT parsing.
func setupRegistrationLimitServer(t *testing.T, env map[string]string) (*gin.Engine, *Server) {
	t.Helper()
	for _, key := range []string{"OPENREADER_INVITE_CODE", "OPENREADER_USER_LIMIT"} {
		t.Setenv(key, "")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	loaded := config.Load()
	return setupTestServerWithConfig(t, func(cfg *config.Config) {
		isolated := *cfg
		*cfg = loaded
		cfg.DataDir = isolated.DataDir
		cfg.CacheDir = isolated.CacheDir
		cfg.LibraryDir = isolated.LibraryDir
		cfg.DatabasePath = isolated.DatabasePath
		cfg.JWTSecret = isolated.JWTSecret
		cfg.JWTPreviousSecrets = isolated.JWTPreviousSecrets
		cfg.LocalStoreDir = isolated.LocalStoreDir
		cfg.PublicDir = isolated.PublicDir
		cfg.RateLimitPerMinute = isolated.RateLimitPerMinute
	})
}

func registrationBody(t *testing.T, username, password string, code *string) string {
	t.Helper()
	payload := map[string]string{"username": username, "password": password}
	if code != nil {
		payload["code"] = *code
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func registerAccount(t *testing.T, router http.Handler, username string, code *string) *httptest.ResponseRecorder {
	t.Helper()
	return authBoundaryRequest(router, "/api/auth/register", registrationBody(t, username, "password8x", code), false)
}

func stringPointer(value string) *string {
	return &value
}

func registeredUserCount(t *testing.T, server *Server) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.User{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func assertRegistered(t *testing.T, response *httptest.ResponseRecorder, username string) string {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("register %s: status=%d, want 200: %s", username, response.Code, response.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if payload.Token == "" || payload.User.Username != username {
		t.Fatalf("register %s returned token=%t user=%q", username, payload.Token != "", payload.User.Username)
	}
	return payload.Token
}

var seededRegistrationHash = func() string {
	hash, err := bcrypt.GenerateFromPassword([]byte("seededpassword8"), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
}()

func seedRegisteredUsers(t *testing.T, server *Server, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		user := models.User{Username: fmt.Sprintf("seeded%03d", index), PasswordHash: seededRegistrationHash, Role: "user",
			CanEditSources: true, CanAccessStore: true, LastActiveAt: time.Now().UTC()}
		if err := server.db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistrationRequiresTheConfiguredInviteCode(t *testing.T) {
	const invite = "reader-invite-2026"
	router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_INVITE_CODE": "  " + invite + "  "})

	assertAuthBoundaryError(t, registerAccount(t, router, "inviteuser1", nil), http.StatusBadRequest, "invite code required", invite)
	assertAuthBoundaryError(t, registerAccount(t, router, "inviteuser1", stringPointer("   ")), http.StatusBadRequest, "invite code required", invite)
	assertAuthBoundaryError(t, registerAccount(t, router, "inviteuser1", stringPointer("reader-invite-2025")), http.StatusForbidden, "invalid invite code", invite)
	assertAuthBoundaryError(t, registerAccount(t, router, "inviteuser1", stringPointer(strings.ToUpper(invite))), http.StatusForbidden, "invalid invite code", invite)
	if count := registeredUserCount(t, server); count != 0 {
		t.Fatalf("rejected invite codes created %d users", count)
	}

	response := registerAccount(t, router, "inviteuser1", stringPointer(" "+invite+" "))
	assertRegistered(t, response, "inviteuser1")
	if strings.Contains(response.Body.String(), invite) {
		t.Fatal("register response leaked the configured invite code")
	}
	if count := registeredUserCount(t, server); count != 1 {
		t.Fatalf("users=%d after the accepted invite code, want 1", count)
	}
}

func TestRegistrationChecksTheInviteCodeBeforeTheUsernameIsTaken(t *testing.T) {
	const invite = "reader-invite-2026"
	router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_INVITE_CODE": invite})
	seedRegisteredUsers(t, server, 1) // seeded000

	// Without the invite code nobody learns whether a username exists.
	assertAuthBoundaryError(t, registerAccount(t, router, "seeded000", nil), http.StatusBadRequest, "invite code required", invite)
	assertAuthBoundaryError(t, registerAccount(t, router, "seeded000", stringPointer("wrong-code")), http.StatusForbidden, "invalid invite code", invite)
	assertAuthBoundaryError(t, registerAccount(t, router, "seeded000", stringPointer(invite)), http.StatusConflict, "username already exists", invite)
	// Field validation still comes first, as upstream checks it before the invite code.
	assertAuthBoundaryError(t, registerAccount(t, router, "abc", nil), http.StatusBadRequest, "username must be at least 5 characters", invite)
}

func TestRegistrationStaysOpenWithoutAnInviteCode(t *testing.T) {
	router, server := setupRegistrationLimitServer(t, nil)
	assertRegistered(t, registerAccount(t, router, "openuser01", nil), "openuser01")
	assertRegistered(t, registerAccount(t, router, "openuser02", stringPointer("any-code")), "openuser02")
	if count := registeredUserCount(t, server); count != 2 {
		t.Fatalf("users=%d, want 2", count)
	}
}

func TestRegistrationStopsAtTheUserLimit(t *testing.T) {
	router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_USER_LIMIT": "2"})
	assertRegistered(t, registerAccount(t, router, "limituser1", nil), "limituser1")
	assertRegistered(t, registerAccount(t, router, "limituser2", nil), "limituser2")

	assertAuthBoundaryError(t, registerAccount(t, router, "limituser3", nil), http.StatusForbidden, "user limit reached", "")
	if count := registeredUserCount(t, server); count != 2 {
		t.Fatalf("users=%d after the limit, want 2", count)
	}
	var sessions int64
	if err := server.db.Model(&models.UserSession{}).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if sessions != 2 {
		t.Fatalf("sessions=%d, want only the two registered accounts", sessions)
	}

	// Existing accounts keep signing in at the limit.
	login := authBoundaryRequest(router, "/api/auth/login", registrationBody(t, "limituser1", "password8x", nil), false)
	if login.Code != http.StatusOK {
		t.Fatalf("login at the limit: status=%d: %s", login.Code, login.Body.String())
	}
}

func TestExistingAccountsAboveTheUserLimitStillSignIn(t *testing.T) {
	router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_USER_LIMIT": "2"})
	seedRegisteredUsers(t, server, 5)
	login := authBoundaryRequest(router, "/api/auth/login", registrationBody(t, "seeded004", "seededpassword8", nil), false)
	if login.Code != http.StatusOK {
		t.Fatalf("login above the limit: status=%d: %s", login.Code, login.Body.String())
	}
}

func TestAdminCreateUserStopsAtTheUserLimitWithoutAnInviteCode(t *testing.T) {
	const invite = "reader-invite-2026"
	router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_INVITE_CODE": invite, "OPENREADER_USER_LIMIT": "2"})
	adminToken := assertRegistered(t, registerAccount(t, router, "limitadmin", stringPointer(invite)), "limitadmin")

	create := func(username string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(registrationBody(t, username, "password8x", nil)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+adminToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	// The administrator needs no invite code.
	if response := create("adminmade1"); response.Code != http.StatusCreated {
		t.Fatalf("admin create below the limit: status=%d: %s", response.Code, response.Body.String())
	}
	response := create("adminmade2")
	if response.Code != http.StatusForbidden {
		t.Fatalf("admin create at the limit: status=%d, want 403: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin limit response: %v", err)
	}
	if payload.Error.Code != "USER_LIMIT_REACHED" || payload.Error.Message != "user limit reached" {
		t.Fatalf("admin limit error=%+v", payload.Error)
	}
	if count := registeredUserCount(t, server); count != 2 {
		t.Fatalf("users=%d after the admin limit, want 2", count)
	}
}

func TestConcurrentRegistrationsNeverExceedTheUserLimit(t *testing.T) {
	router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_USER_LIMIT": "3"})
	const attempts = 8
	statuses := make([]int, attempts)
	bodies := make([]string, attempts)
	for index := range bodies {
		bodies[index] = registrationBody(t, fmt.Sprintf("parallel%02d", index), "password8x", nil)
	}
	var wait sync.WaitGroup
	for index := 0; index < attempts; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			statuses[index] = authBoundaryRequest(router, "/api/auth/register", bodies[index], false).Code
		}(index)
	}
	wait.Wait()
	accepted, limited := 0, 0
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			accepted++
		case http.StatusForbidden:
			limited++
		default:
			t.Fatalf("unexpected registration status %d in %v", status, statuses)
		}
	}
	if accepted != 3 || limited != attempts-3 {
		t.Fatalf("accepted=%d limited=%d, want 3 and %d: %v", accepted, limited, attempts-3, statuses)
	}
	if count := registeredUserCount(t, server); count != 3 {
		t.Fatalf("users=%d after concurrent registration, want 3", count)
	}
}

func TestUserLimitEnvironmentIsClampedLikeUpstream(t *testing.T) {
	for _, tc := range []struct {
		value string
		limit int
	}{
		{"", 50}, {"7", 7}, {"0", 1}, {"-5", 1}, {"99", 50}, {"abc", 50},
	} {
		t.Run(fmt.Sprintf("OPENREADER_USER_LIMIT=%q", tc.value), func(t *testing.T) {
			router, server := setupRegistrationLimitServer(t, map[string]string{"OPENREADER_USER_LIMIT": tc.value})
			seedRegisteredUsers(t, server, tc.limit-1)
			assertRegistered(t, registerAccount(t, router, "lastallowed", nil), "lastallowed")
			assertAuthBoundaryError(t, registerAccount(t, router, "oneover", nil), http.StatusForbidden, "user limit reached", "")
			if count := registeredUserCount(t, server); count != int64(tc.limit) {
				t.Fatalf("users=%d, want %d", count, tc.limit)
			}
		})
	}
}
