package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

func adminContractRequest(router *gin.Engine, method, path, body, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, request)
	return writer
}

func assertAdminContractError(t *testing.T, writer *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if writer.Code != status {
		t.Fatalf("status = %d, want %d: %s", writer.Code, status, writer.Body.String())
	}
	var response struct {
		Error apiError `json:"error"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q: %s", response.Error.Code, code, writer.Body.String())
	}
}

func TestAdminUserContractProtectsAdministratorRowsAndCreatesOnlyOrdinaryUsers(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)
	var administrator models.User
	if err := server.db.Where("username = ?", "testuser").First(&administrator).Error; err != nil {
		t.Fatalf("load administrator: %v", err)
	}
	if err := server.db.Model(&administrator).Update("role", "admin").Error; err != nil {
		t.Fatalf("promote administrator: %v", err)
	}

	memberAuth := registerStorageTestUser(t, router, "admincontractmember")
	for _, request := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/admin/users"},
		{name: "create", method: http.MethodPost, path: "/api/admin/users", body: `{"username":"blocked-create","password":"secret123"}`},
		{name: "update", method: http.MethodPut, path: "/api/admin/users/" + strconv.FormatUint(uint64(administrator.ID), 10), body: `{"canAccessStore":false}`},
		{name: "password", method: http.MethodPut, path: "/api/admin/users/" + strconv.FormatUint(uint64(administrator.ID), 10) + "/password", body: `{"password":"secret123"}`},
		{name: "delete", method: http.MethodPost, path: "/api/admin/users/batch-delete", body: `{"ids":[` + strconv.FormatUint(uint64(administrator.ID), 10) + `]}`},
		{name: "cleanup", method: http.MethodPost, path: "/api/admin/cleanup-inactive", body: `{}`},
	} {
		t.Run("non-admin "+request.name, func(t *testing.T) {
			writer := adminContractRequest(router, request.method, request.path, request.body, memberAuth)
			assertAdminContractError(t, writer, http.StatusForbidden, "FORBIDDEN")
		})
	}

	roleEscalation := adminContractRequest(router, http.MethodPost, "/api/admin/users", `{
		"username":"forbidden-admin",
		"password":"secret123",
		"role":"admin"
	}`, adminAuth)
	assertAdminContractError(t, roleEscalation, http.StatusBadRequest, "BAD_REQUEST")

	created := adminContractRequest(router, http.MethodPost, "/api/admin/users", `{
		"username":"ordinarymanaged",
		"password":"secret123",
		"canEditSources":false,
		"canAccessStore":true
	}`, adminAuth)
	if created.Code != http.StatusCreated {
		t.Fatalf("create ordinary user: %d %s", created.Code, created.Body.String())
	}
	var ordinary models.User
	if err := json.Unmarshal(created.Body.Bytes(), &ordinary); err != nil {
		t.Fatalf("decode ordinary user: %v", err)
	}
	if ordinary.Role != "user" || ordinary.CanEditSources {
		t.Fatalf("ordinary create result: %+v", ordinary)
	}

	protectedUpdate := adminContractRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(administrator.ID), 10), `{"canAccessStore":false}`, adminAuth)
	assertAdminContractError(t, protectedUpdate, http.StatusForbidden, "FORBIDDEN")
	protectedPassword := adminContractRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(administrator.ID), 10)+"/password", `{"password":"changed123"}`, adminAuth)
	assertAdminContractError(t, protectedPassword, http.StatusForbidden, "FORBIDDEN")

	ordinaryUpdate := adminContractRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(ordinary.ID), 10), `{"canAccessStore":false}`, adminAuth)
	if ordinaryUpdate.Code != http.StatusOK {
		t.Fatalf("update ordinary user: %d %s", ordinaryUpdate.Code, ordinaryUpdate.Body.String())
	}
}
