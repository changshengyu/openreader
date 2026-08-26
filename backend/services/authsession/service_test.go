package authsession

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

type sequenceReader struct {
	next byte
}

func (r *sequenceReader) Read(data []byte) (int, error) {
	r.next++
	for index := range data {
		data[index] = r.next
	}
	return len(data), nil
}

type serviceFixture struct {
	db      *gorm.DB
	service *Service
	user    models.User
	now     *time.Time
	marker  time.Time
}

func newServiceFixture(t *testing.T) serviceFixture {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sessions.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.UserSession{}, &models.SchemaMigration{}); err != nil {
		t.Fatal(err)
	}
	marker := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	if err := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.SchemaMigration{
		Key:       MigrationKey,
		AppliedAt: marker,
	}).Error; err != nil {
		t.Fatal(err)
	}
	user := models.User{
		Username:     "sessionfixture",
		PasswordHash: "hash",
		Role:         "user",
		AuthVersion:  1,
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := marker.Add(time.Hour)
	service := New(database, "session-test-secret")
	service.now = func() time.Time { return now }
	service.random = &sequenceReader{}
	return serviceFixture{db: database, service: service, user: user, now: &now, marker: marker}
}

func TestIssueAuthenticateRenewAndRevokeSession(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	first, err := fixture.service.Issue(ctx, fixture.user)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.Issue(ctx, fixture.user)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("independent logins produced the same token")
	}

	claims, err := parseClaims(fixture.service.secret, first)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ID == "" || claims.AuthVersion != 1 || len(claims.ID) < 40 {
		t.Fatalf("issued claims = %+v", claims)
	}
	var row models.UserSession
	if err := fixture.db.Where("user_id = ?", fixture.user.ID).Order("created_at ASC").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ID == first || row.ID == claims.ID || row.Legacy || row.RevokedAt != nil {
		t.Fatalf("persisted raw or invalid session row: %+v", row)
	}

	*fixture.now = fixture.now.Add(24 * time.Hour)
	identity, err := fixture.service.Authenticate(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != fixture.user.ID || identity.Key != row.ID || identity.Legacy {
		t.Fatalf("authenticated identity = %+v", identity)
	}
	if err := fixture.db.First(&row, "id = ?", identity.Key).Error; err != nil {
		t.Fatal(err)
	}
	if !row.LastSeenAt.Equal(*fixture.now) || !row.ExpiresAt.Equal(fixture.now.Add(SessionTTL)) {
		t.Fatalf("renewed row = %+v, now=%s", row, *fixture.now)
	}

	if err := fixture.service.Revoke(ctx, identity); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := fixture.db.Model(&models.UserSession{}).Where("id = ?", identity.Key).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("modern revoked session rows = %d, want 0", count)
	}
	if _, err := fixture.service.Authenticate(ctx, first); err != ErrInvalidSession {
		t.Fatalf("revoked session error = %v, want ErrInvalidSession", err)
	}
}

func TestLegacySessionAdoptionWindowAndRevocationTombstone(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	legacy := signLegacyToken(t, fixture.service.secret, fixture.user.ID, fixture.marker.Add(-time.Hour))
	identity, err := fixture.service.Authenticate(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !identity.Legacy {
		t.Fatalf("legacy identity = %+v", identity)
	}
	var row models.UserSession
	if err := fixture.db.First(&row, "id = ?", identity.Key).Error; err != nil {
		t.Fatal(err)
	}
	if !row.Legacy || row.ID == legacy || row.RevokedAt != nil {
		t.Fatalf("legacy row = %+v", row)
	}
	if err := fixture.service.Revoke(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Authenticate(ctx, legacy); err != ErrInvalidSession {
		t.Fatalf("revoked legacy token was re-adopted: %v", err)
	}
	if err := fixture.db.First(&row, "id = ?", identity.Key).Error; err != nil || row.RevokedAt == nil {
		t.Fatalf("legacy revocation tombstone = %+v, err=%v", row, err)
	}

	unadopted := signLegacyToken(t, fixture.service.secret, fixture.user.ID, fixture.marker.Add(-2*time.Hour))
	*fixture.now = fixture.marker.Add(SessionTTL + time.Second)
	if _, err := fixture.service.Authenticate(ctx, unadopted); err != ErrInvalidSession {
		t.Fatalf("legacy token after adoption window error = %v", err)
	}

	*fixture.now = fixture.marker.Add(time.Hour)
	if err := fixture.db.Model(&models.User{}).Where("id = ?", fixture.user.ID).Update("auth_version", 2).Error; err != nil {
		t.Fatal(err)
	}
	postResetLegacy := signLegacyToken(t, fixture.service.secret, fixture.user.ID, fixture.marker.Add(-3*time.Hour))
	if _, err := fixture.service.Authenticate(ctx, postResetLegacy); err != ErrInvalidSession {
		t.Fatalf("legacy token after auth-version change error = %v", err)
	}
}

func TestSessionRetentionCapAndExpiry(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	var first, latest string
	for index := 0; index < MaxUserSessions+1; index++ {
		token, err := fixture.service.Issue(ctx, fixture.user)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			first = token
		}
		latest = token
		*fixture.now = fixture.now.Add(time.Second)
	}
	var count int64
	if err := fixture.db.Model(&models.UserSession{}).
		Where("user_id = ? AND revoked_at IS NULL", fixture.user.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != MaxUserSessions {
		t.Fatalf("active sessions = %d, want %d", count, MaxUserSessions)
	}
	if _, err := fixture.service.Authenticate(ctx, first); err != ErrInvalidSession {
		t.Fatalf("oldest over-cap session error = %v", err)
	}
	if _, err := fixture.service.Authenticate(ctx, latest); err != nil {
		t.Fatalf("latest session: %v", err)
	}
	*fixture.now = fixture.now.Add(SessionTTL + time.Second)
	if _, err := fixture.service.Authenticate(ctx, latest); err != ErrInvalidSession {
		t.Fatalf("expired session error = %v", err)
	}
}

func signLegacyToken(t *testing.T, secret string, userID uint, issuedAt time.Time) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(issuedAt),
		},
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestSessionIdentityUsesFullRandomInput(t *testing.T) {
	raw := make([]byte, 32)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) < 40 || sessionDigest("jti", encoded) == encoded {
		t.Fatal("session identity did not preserve 256-bit input before hashing")
	}
}

func TestConcurrentRenewCannotResurrectRevokedSession(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	token, err := fixture.service.Issue(ctx, fixture.user)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := fixture.service.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for index := 0; index < 16; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, _ = fixture.service.Authenticate(ctx, token)
		}()
	}
	close(start)
	if err := fixture.service.Revoke(ctx, identity); err != nil && err != ErrInvalidSession {
		t.Fatal(err)
	}
	workers.Wait()

	if _, err := fixture.service.Authenticate(ctx, token); err != ErrInvalidSession {
		t.Fatalf("revoked session was resurrected after concurrent renewals: %v", err)
	}
	var count int64
	if err := fixture.db.Model(&models.UserSession{}).Where("id = ?", identity.Key).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("revoked session rows = %d, want 0", count)
	}
}
