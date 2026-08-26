package db

import (
	"testing"
	"time"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestAutoMigrateAddsAuthenticatedSessionStateWithoutRewritingHistoricalUser(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	cfg.DatabasePath = cfg.DataDir + "/historical-session.db"
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`CREATE TABLE users (
		id integer PRIMARY KEY AUTOINCREMENT,
		username text NOT NULL,
		password_hash text NOT NULL,
		role text,
		book_limit integer,
		source_limit integer,
		can_edit_sources numeric,
		can_access_store numeric,
		can_access_webdav numeric,
		last_active_at datetime,
		created_at datetime,
		updated_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	lastActive := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	created := time.Date(2019, time.June, 7, 8, 9, 10, 0, time.UTC)
	if err := database.Exec(`INSERT INTO users
		(username,password_hash,role,book_limit,source_limit,can_edit_sources,can_access_store,can_access_webdav,last_active_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		"historical-session-user", "historical-hash", "user", 12, 34, true, false, nil,
		lastActive, created, created,
	).Error; err != nil {
		t.Fatal(err)
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	var user models.User
	if err := database.Where("username = ?", "historical-session-user").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash != "historical-hash" || user.Role != "user" || user.BookLimit != 12 || user.SourceLimit != 34 ||
		!user.CanEditSources || user.CanAccessStore || user.CanAccessWebDAV != nil || user.AuthVersion != 1 ||
		!user.LastActiveAt.Equal(lastActive) || !user.CreatedAt.Equal(created) || !user.UpdatedAt.Equal(created) {
		t.Fatalf("historical user changed during session migration: %+v", user)
	}
	if !database.Migrator().HasTable(&models.UserSession{}) {
		t.Fatal("user_sessions table was not added")
	}
	var marker models.SchemaMigration
	if err := database.Where("key = ?", authenticatedSessionMigrationKey).First(&marker).Error; err != nil {
		t.Fatal(err)
	}
	if marker.AppliedAt.IsZero() {
		t.Fatal("session migration marker has no applied time")
	}
	firstAppliedAt := marker.AppliedAt
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	if err := database.Where("key = ?", authenticatedSessionMigrationKey).First(&marker).Error; err != nil {
		t.Fatal(err)
	}
	if !marker.AppliedAt.Equal(firstAppliedAt) {
		t.Fatalf("repeat migration reopened adoption window: first=%s second=%s", firstAppliedAt, marker.AppliedAt)
	}
}
