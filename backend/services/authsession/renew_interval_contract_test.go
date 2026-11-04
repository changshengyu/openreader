package authsession

import (
	"context"
	"sync"
	"testing"
	"time"

	"openreader/backend/models"
)

func loadSessionRow(t *testing.T, fixture serviceFixture, key string) models.UserSession {
	t.Helper()
	var row models.UserSession
	if err := fixture.db.First(&row, "id = ?", key).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func issuedSessionKey(t *testing.T, fixture serviceFixture, token string) string {
	t.Helper()
	claims, err := parseClaims(fixture.service.secret, token)
	if err != nil {
		t.Fatal(err)
	}
	return sessionDigest("jti", claims.ID)
}

func TestAuthenticateRenewsSessionOnlyOnceAMinute(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	token, err := fixture.service.Issue(ctx, fixture.user)
	if err != nil {
		t.Fatal(err)
	}
	key := issuedSessionKey(t, fixture, token)
	issuedAt := *fixture.now
	authenticateAt := func(at time.Time) models.UserSession {
		t.Helper()
		*fixture.now = at
		if _, err := fixture.service.Authenticate(ctx, token); err != nil {
			t.Fatalf("authenticate %s after issue: %v", at.Sub(issuedAt), err)
		}
		return loadSessionRow(t, fixture, key)
	}

	row := authenticateAt(issuedAt.Add(59 * time.Second))
	if !row.LastSeenAt.Equal(issuedAt) || !row.ExpiresAt.Equal(issuedAt.Add(SessionTTL)) {
		t.Errorf("authentication 59s after issue renewed the session: last_seen_at=+%s expires_at=+%s, want +0s and +%s",
			row.LastSeenAt.Sub(issuedAt), row.ExpiresAt.Sub(issuedAt), SessionTTL)
	}

	renewedAt := issuedAt.Add(time.Minute)
	row = authenticateAt(renewedAt)
	if !row.LastSeenAt.Equal(renewedAt) || !row.ExpiresAt.Equal(renewedAt.Add(SessionTTL)) {
		t.Errorf("authentication a minute after issue: last_seen_at=+%s expires_at=+%s, want +1m0s and +%s",
			row.LastSeenAt.Sub(issuedAt), row.ExpiresAt.Sub(issuedAt), time.Minute+SessionTTL)
	}

	// The seven-day sliding rule holds to within the renew interval after
	// every successful authentication.
	for at := renewedAt.Add(30 * time.Second); !at.After(renewedAt.Add(10 * time.Minute)); at = at.Add(30 * time.Second) {
		row = authenticateAt(at)
		if row.ExpiresAt.Before(at.Add(SessionTTL-time.Minute)) || row.ExpiresAt.After(at.Add(SessionTTL)) {
			t.Errorf("after authenticating at +%s expires_at=+%s, want within [+%s, +%s]",
				at.Sub(issuedAt), row.ExpiresAt.Sub(issuedAt), at.Add(SessionTTL-time.Minute).Sub(issuedAt), at.Add(SessionTTL).Sub(issuedAt))
		}
		if !row.ExpiresAt.Equal(row.LastSeenAt.Add(SessionTTL)) {
			t.Errorf("after authenticating at +%s expires_at=+%s, want last_seen_at (+%s) + %s",
				at.Sub(issuedAt), row.ExpiresAt.Sub(issuedAt), row.LastSeenAt.Sub(issuedAt), SessionTTL)
		}
	}
}

func TestLegacySessionRenewsOnlyOnceAMinute(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	legacy := signLegacyToken(t, fixture.service.secret, fixture.user.ID, fixture.marker.Add(-time.Hour))
	adoptedAt := *fixture.now
	identity, err := fixture.service.Authenticate(ctx, legacy)
	if err != nil || !identity.Legacy {
		t.Fatalf("adopt legacy token: identity=%+v err=%v", identity, err)
	}

	*fixture.now = adoptedAt.Add(59 * time.Second)
	if _, err := fixture.service.Authenticate(ctx, legacy); err != nil {
		t.Fatalf("legacy authentication 59s after adoption: %v", err)
	}
	row := loadSessionRow(t, fixture, identity.Key)
	if !row.LastSeenAt.Equal(adoptedAt) || !row.ExpiresAt.Equal(adoptedAt.Add(SessionTTL)) {
		t.Errorf("legacy authentication 59s after adoption renewed the session: last_seen_at=+%s expires_at=+%s, want +0s and +%s",
			row.LastSeenAt.Sub(adoptedAt), row.ExpiresAt.Sub(adoptedAt), SessionTTL)
	}

	renewedAt := adoptedAt.Add(time.Minute)
	*fixture.now = renewedAt
	if _, err := fixture.service.Authenticate(ctx, legacy); err != nil {
		t.Fatalf("legacy authentication a minute after adoption: %v", err)
	}
	row = loadSessionRow(t, fixture, identity.Key)
	if !row.LastSeenAt.Equal(renewedAt) || !row.ExpiresAt.Equal(renewedAt.Add(SessionTTL)) {
		t.Errorf("legacy authentication a minute after adoption: last_seen_at=+%s expires_at=+%s, want +1m0s and +%s",
			row.LastSeenAt.Sub(adoptedAt), row.ExpiresAt.Sub(adoptedAt), time.Minute+SessionTTL)
	}

	*fixture.now = renewedAt.Add(time.Second)
	if err := fixture.service.Revoke(ctx, identity); err != nil {
		t.Fatal(err)
	}
	*fixture.now = renewedAt.Add(2 * time.Second)
	if _, err := fixture.service.Authenticate(ctx, legacy); err != ErrInvalidSession {
		t.Fatalf("revoked legacy token a second after its revocation: %v, want ErrInvalidSession", err)
	}
}

func TestAuthenticateWithinAMinuteStillRejectsRevokedResetExpiredAndDeletedSessions(t *testing.T) {
	cases := []struct {
		name       string
		invalidate func(t *testing.T, fixture serviceFixture, identity Identity)
	}{
		{
			name: "logout",
			invalidate: func(t *testing.T, fixture serviceFixture, identity Identity) {
				if err := fixture.service.Revoke(context.Background(), identity); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "password reset auth version",
			invalidate: func(t *testing.T, fixture serviceFixture, identity Identity) {
				if err := fixture.db.Model(&models.User{}).Where("id = ?", fixture.user.ID).
					Update("auth_version", 2).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "session row from another auth version",
			invalidate: func(t *testing.T, fixture serviceFixture, identity Identity) {
				if err := fixture.db.Model(&models.UserSession{}).Where("id = ?", identity.Key).
					Update("auth_version", 2).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "expired",
			invalidate: func(t *testing.T, fixture serviceFixture, identity Identity) {
				if err := fixture.db.Model(&models.UserSession{}).Where("id = ?", identity.Key).
					UpdateColumn("expires_at", fixture.now.Add(time.Second)).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "deleted user",
			invalidate: func(t *testing.T, fixture serviceFixture, identity Identity) {
				if err := fixture.db.Delete(&models.User{}, fixture.user.ID).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			ctx := context.Background()
			token, err := fixture.service.Issue(ctx, fixture.user)
			if err != nil {
				t.Fatal(err)
			}
			*fixture.now = fixture.now.Add(10 * time.Second)
			identity, err := fixture.service.Authenticate(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			testCase.invalidate(t, fixture, identity)
			*fixture.now = fixture.now.Add(10 * time.Second)
			if _, err := fixture.service.Authenticate(ctx, token); err != ErrInvalidSession {
				t.Fatalf("authentication 20s after issue, after %s: %v, want ErrInvalidSession", testCase.name, err)
			}
		})
	}
}

func TestConcurrentRenewAfterAMinuteCannotResurrectRevokedSession(t *testing.T) {
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
	// Two minutes on, the first authentications find the session due for
	// renewal and race their conditional updates against the logout.
	*fixture.now = fixture.now.Add(2 * time.Minute)

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
