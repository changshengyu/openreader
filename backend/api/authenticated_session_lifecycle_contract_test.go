package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"openreader/backend/models"
)

type sessionContractAuthResponse struct {
	Token string `json:"token"`
	User  struct {
		ID uint `json:"id"`
	} `json:"user"`
}

func registerSessionContractUser(t *testing.T, router *gin.Engine, username, password string) sessionContractAuthResponse {
	t.Helper()
	response := sessionContractJSONRequest(
		router,
		http.MethodPost,
		"/api/auth/register",
		`{"username":"`+username+`","password":"`+password+`"}`,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("register %s: status=%d: %s", username, response.Code, response.Body.String())
	}
	var payload sessionContractAuthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if payload.Token == "" || payload.User.ID == 0 {
		t.Fatalf("register response missing identity: %+v", payload)
	}
	return payload
}

func loginSessionContractUser(t *testing.T, router *gin.Engine, username, password string) sessionContractAuthResponse {
	t.Helper()
	response := sessionContractJSONRequest(
		router,
		http.MethodPost,
		"/api/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("login %s: status=%d: %s", username, response.Code, response.Body.String())
	}
	var payload sessionContractAuthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return payload
}

func sessionContractJSONRequest(router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAuthenticatedSessionLifecycleRequiresAdditiveSchemaAndTokenIdentity(t *testing.T) {
	t.Run("schema", func(t *testing.T) {
		_, server := setupTestServer(t)
		if !server.db.Migrator().HasColumn(&models.User{}, "auth_version") {
			t.Error("users.auth_version is missing")
		}
		if !server.db.Migrator().HasTable("user_sessions") {
			t.Error("user_sessions table is missing")
		}
	})

	t.Run("claims and persisted hashed session", func(t *testing.T) {
		router, server := setupTestServer(t)
		issued := registerSessionContractUser(t, router, "sessionclaims", "password8")
		parsed, _, err := jwt.NewParser().ParseUnverified(issued.Token, jwt.MapClaims{})
		if err != nil {
			t.Fatalf("parse issued token: %v", err)
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatalf("claims type = %T", parsed.Claims)
		}
		identity, _ := claims["jti"].(string)
		if identity == "" {
			t.Error("issued token has no random jti")
		}
		if version, ok := claims["authVersion"].(float64); !ok || version < 1 {
			t.Errorf("issued token authVersion = %#v, want positive", claims["authVersion"])
		}

		if server.db.Migrator().HasTable("user_sessions") {
			var rows int64
			if err := server.db.Table("user_sessions").Where("user_id = ?", issued.User.ID).Count(&rows).Error; err != nil {
				t.Fatalf("count sessions: %v", err)
			}
			if rows != 1 {
				t.Errorf("persisted session count = %d, want 1", rows)
			}
			var leaked int64
			if err := server.db.Raw(
				"SELECT COUNT(*) FROM user_sessions WHERE CAST(id AS TEXT) = ? OR CAST(id AS TEXT) = ?",
				issued.Token,
				identity,
			).Scan(&leaked).Error; err != nil {
				t.Fatalf("check raw session identity: %v", err)
			}
			if leaked != 0 {
				t.Errorf("user_sessions retained %d raw token identities", leaked)
			}
		}
	})
}

func TestAuthenticatedSessionLogoutRevokesOnlyCurrentLogin(t *testing.T) {
	router, _ := setupTestServer(t)
	first := registerSessionContractUser(t, router, "sessionlogout", "password8")
	second := loginSessionContractUser(t, router, "sessionlogout", "password8")
	if first.Token == second.Token {
		t.Error("two logins returned the same token")
	}

	logout := sessionContractJSONRequest(router, http.MethodPost, "/api/auth/logout", "", first.Token)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d, want 204: %s", logout.Code, logout.Body.String())
	}
	firstAfterLogout := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", first.Token)
	if firstAfterLogout.Code != http.StatusUnauthorized {
		t.Errorf("logged-out token status=%d, want 401: %s", firstAfterLogout.Code, firstAfterLogout.Body.String())
	}
	secondAfterLogout := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", second.Token)
	if secondAfterLogout.Code != http.StatusOK {
		t.Errorf("other login status=%d, want 200: %s", secondAfterLogout.Code, secondAfterLogout.Body.String())
	}
}

func TestDeletedUserTokenFailsBeforeRESTAndWebDAVSideEffects(t *testing.T) {
	router, server := setupTestServer(t)
	admin := registerSessionContractUser(t, router, "sessionadmin", "password8")
	target := registerSessionContractUser(t, router, "sessiondeleted", "password8")

	deleted := sessionContractJSONRequest(
		router,
		http.MethodPost,
		"/api/admin/users/batch-delete",
		`{"ids":[`+strconv.FormatUint(uint64(target.User.ID), 10)+`]}`,
		admin.Token,
	)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete user: status=%d: %s", deleted.Code, deleted.Body.String())
	}

	list := sessionContractJSONRequest(router, http.MethodGet, "/api/sources", "", target.Token)
	if list.Code != http.StatusUnauthorized {
		t.Errorf("deleted-user REST read status=%d, want 401: %s", list.Code, list.Body.String())
	}
	setting := sessionContractJSONRequest(
		router,
		http.MethodPut,
		"/api/settings/reader",
		`{"value":{"fontSize":99}}`,
		target.Token,
	)
	if setting.Code != http.StatusUnauthorized {
		t.Errorf("deleted-user setting write status=%d, want 401: %s", setting.Code, setting.Body.String())
	}
	webdav := sessionContractJSONRequest(router, http.MethodGet, "/webdav/", "", target.Token)
	if webdav.Code != http.StatusUnauthorized {
		t.Errorf("deleted-user WebDAV Bearer status=%d, want 401: %s", webdav.Code, webdav.Body.String())
	}

	var orphanSettings int64
	if err := server.db.Model(&models.UserSetting{}).Where("user_id = ?", target.User.ID).Count(&orphanSettings).Error; err != nil {
		t.Fatalf("count orphan settings: %v", err)
	}
	if orphanSettings != 0 {
		t.Errorf("deleted token created %d orphan user settings", orphanSettings)
	}
	var orphanSessions int64
	if err := server.db.Model(&models.UserSession{}).Where("user_id = ?", target.User.ID).Count(&orphanSessions).Error; err != nil {
		t.Fatalf("count orphan sessions: %v", err)
	}
	if orphanSessions != 0 {
		t.Errorf("deleted user retained %d sessions", orphanSessions)
	}
}

func TestCanceledUserDeletionPreservesUserAndSession(t *testing.T) {
	router, server := setupTestServer(t)
	target := registerSessionContractUser(t, router, "sessioncanceldelete", "password8")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := server.deleteUserData(ctx, []uint{target.User.ID}, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled deletion error = %v, want context.Canceled", err)
	}
	var users, sessions int64
	if err := server.db.Model(&models.User{}).Where("id = ?", target.User.ID).Count(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.UserSession{}).Where("user_id = ?", target.User.ID).Count(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	if users != 1 || sessions != 1 {
		t.Fatalf("canceled deletion partially committed: users=%d sessions=%d", users, sessions)
	}
}

func TestPasswordResetSessionRevocationRollsBackWithPasswordHash(t *testing.T) {
	router, server := setupTestServer(t)
	admin := registerSessionContractUser(t, router, "rollbackadmin", "password8")
	target := registerSessionContractUser(t, router, "rollbackmember", "password8")
	var before models.User
	if err := server.db.First(&before, target.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Exec(`CREATE TRIGGER fail_session_revoke
		BEFORE DELETE ON user_sessions
		WHEN OLD.user_id = ` + strconv.FormatUint(uint64(target.User.ID), 10) + `
		BEGIN SELECT RAISE(ABORT, 'forced session revoke failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	reset := sessionContractJSONRequest(
		router,
		http.MethodPut,
		"/api/admin/users/"+strconv.FormatUint(uint64(target.User.ID), 10)+"/password",
		`{"password":"changed88"}`,
		admin.Token,
	)
	if reset.Code != http.StatusInternalServerError {
		t.Fatalf("forced reset failure status=%d, want 500: %s", reset.Code, reset.Body.String())
	}
	var after models.User
	if err := server.db.First(&after, target.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != before.PasswordHash || after.AuthVersion != before.AuthVersion {
		t.Fatalf("failed reset partially committed credentials: before=%+v after=%+v", before, after)
	}
	stillValid := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", target.Token)
	if stillValid.Code != http.StatusOK {
		t.Fatalf("rolled-back session status=%d, want 200: %s", stillValid.Code, stillValid.Body.String())
	}
}

func TestPasswordResetRevokesExistingSessions(t *testing.T) {
	router, _ := setupTestServer(t)
	admin := registerSessionContractUser(t, router, "resetadmin", "password8")
	target := registerSessionContractUser(t, router, "resetmember", "password8")

	reset := sessionContractJSONRequest(
		router,
		http.MethodPut,
		"/api/admin/users/"+strconv.FormatUint(uint64(target.User.ID), 10)+"/password",
		`{"password":"changed88"}`,
		admin.Token,
	)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset password: status=%d: %s", reset.Code, reset.Body.String())
	}
	stale := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", target.Token)
	if stale.Code != http.StatusUnauthorized {
		t.Errorf("pre-reset token status=%d, want 401: %s", stale.Code, stale.Body.String())
	}
	newLogin := loginSessionContractUser(t, router, "resetmember", "changed88")
	current := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", newLogin.Token)
	if current.Code != http.StatusOK {
		t.Errorf("post-reset login status=%d, want 200: %s", current.Code, current.Body.String())
	}
}

func TestExpiredSessionFailsAcrossRESTWebDAVAndWebSocket(t *testing.T) {
	router, server := setupTestServer(t)
	issued := registerSessionContractUser(t, router, "sessionexpired", "password8")
	expiredAt := time.Now().UTC().Add(-time.Hour)
	result := server.db.Model(&models.UserSession{}).Where("user_id = ?", issued.User.ID).
		UpdateColumn("expires_at", expiredAt)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("expire session: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	var stored models.UserSession
	if err := server.db.Where("user_id = ?", issued.User.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.ExpiresAt.Equal(expiredAt) {
		t.Fatalf("stored expiry=%s, want %s", stored.ExpiresAt, expiredAt)
	}

	rest := sessionContractJSONRequest(router, http.MethodGet, "/api/me", "", issued.Token)
	if rest.Code != http.StatusUnauthorized {
		t.Errorf("expired REST session status=%d, want 401: %s", rest.Code, rest.Body.String())
	}
	webdav := sessionContractJSONRequest(router, http.MethodGet, "/webdav/", "", issued.Token)
	if webdav.Code != http.StatusUnauthorized {
		t.Errorf("expired WebDAV session status=%d, want 401: %s", webdav.Code, webdav.Body.String())
	}

	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	connection, response, err := dialSyncWebSocket(httpServer.URL, issued.Token, "")
	if connection != nil {
		_ = connection.Close()
	}
	requireHandshakeStatus(t, response, err, http.StatusUnauthorized)
}
