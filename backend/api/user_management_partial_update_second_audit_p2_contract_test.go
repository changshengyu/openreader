package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"openreader/backend/models"
)

func TestAdminPartialUserUpdateDoesNotOverwriteConcurrentLoginOrPasswordWrite(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)

	oldHash, err := bcrypt.GenerateFromPassword([]byte("password8"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	target := models.User{
		Username:        "partialupdateuser",
		PasswordHash:    string(oldHash),
		Role:            "user",
		CanEditSources:  true,
		CanAccessStore:  true,
		CanAccessWebDAV: boolValue(true),
		LastActiveAt:    time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := server.db.Create(&target).Error; err != nil {
		t.Fatalf("create target user: %v", err)
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("changed88"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	newLoginAt := time.Date(2035, time.June, 7, 8, 9, 10, 0, time.UTC)
	trigger := fmt.Sprintf(`
		CREATE TRIGGER user_permission_concurrent_security_write
		BEFORE UPDATE OF can_access_store ON users
		WHEN OLD.id = %d
		BEGIN
			UPDATE users
			SET password_hash = '%s', last_active_at = '%s'
			WHERE id = OLD.id;
		END;
	`, target.ID, newHash, newLoginAt.Format("2006-01-02 15:04:05-07:00"))
	if err := server.db.Exec(trigger).Error; err != nil {
		t.Fatalf("create concurrent-write trigger: %v", err)
	}

	writer := adminContractRequest(
		router,
		http.MethodPut,
		"/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10),
		`{"canAccessStore":false}`,
		adminAuth,
	)
	if writer.Code != http.StatusOK {
		t.Fatalf("partial user update: status=%d body=%s", writer.Code, writer.Body.String())
	}

	var stored models.User
	if err := server.db.First(&stored, target.ID).Error; err != nil {
		t.Fatalf("reload target user: %v", err)
	}
	if stored.PasswordHash != string(newHash) {
		t.Fatal("permission update overwrote the concurrently reset password hash")
	}
	if !stored.LastActiveAt.Equal(newLoginAt) {
		t.Fatalf("permission update overwrote concurrent login time: got %s want %s", stored.LastActiveAt, newLoginAt)
	}
	if stored.CanAccessStore {
		t.Fatal("requested permission field was not updated")
	}
}

func TestAdminPartialUserUpdateValidatesPatchAndProjectsLegacyWebDAV(t *testing.T) {
	router, server := setupTestServer(t)
	adminAuth := authHeader(t, router)

	target := models.User{
		Username:        "legacypatchuser",
		PasswordHash:    "hash",
		Role:            "user",
		BookLimit:       7,
		SourceLimit:     9,
		CanEditSources:  true,
		CanAccessStore:  true,
		CanAccessWebDAV: nil,
	}
	if err := server.db.Create(&target).Error; err != nil {
		t.Fatalf("create legacy target: %v", err)
	}
	path := "/api/admin/users/" + strconv.FormatUint(uint64(target.ID), 10)

	for _, body := range []string{`{}`, `{"ignored":true}`, `{"bookLimit":-1}`, `{"sourceLimit":-1}`} {
		writer := adminContractRequest(router, http.MethodPut, path, body, adminAuth)
		assertAdminContractError(t, writer, http.StatusBadRequest, "BAD_REQUEST")
	}

	writer := adminContractRequest(
		router,
		http.MethodPut,
		path,
		`{"canAccessStore":false,"bookLimit":0}`,
		adminAuth,
	)
	if writer.Code != http.StatusOK {
		t.Fatalf("valid partial update: status=%d body=%s", writer.Code, writer.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode partial update response: %v", err)
	}
	if value, exists := response["canAccessWebdav"]; !exists || value != false {
		t.Fatalf("effective legacy WebDAV projection missing: %+v", response)
	}

	var stored models.User
	if err := server.db.First(&stored, target.ID).Error; err != nil {
		t.Fatalf("reload legacy target: %v", err)
	}
	if stored.CanAccessStore || stored.BookLimit != 0 || stored.SourceLimit != 9 || !stored.CanEditSources {
		t.Fatalf("partial update changed the wrong fields: %+v", stored)
	}
	if stored.CanAccessWebDAV != nil {
		t.Fatalf("effective response projection persisted the nullable migration field: %+v", stored.CanAccessWebDAV)
	}
}
