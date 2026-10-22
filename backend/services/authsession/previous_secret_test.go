package authsession

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Under an instance secret the published value the instance used to sign
// with becomes a previous secret: it still verifies jti-backed sessions, whose
// row authenticates them, but signs nothing and adopts no legacy token.

const instanceTestSecret = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"

func TestPreviousSecretVerifiesSessionsButNeverSigns(t *testing.T) {
	for _, pair := range []struct{ name, previous, issuedWith string }{
		{"same-default", currentDefaultKey, currentDefaultKey},
		{"other-default", currentDefaultKey, legacyDefaultKey},
		{"previous-default", legacyDefaultKey, currentDefaultKey},
		{"blank", "", ""},
	} {
		t.Run(pair.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			ctx := context.Background()
			older := New(fixture.db, pair.issuedWith)
			older.now = fixture.service.now
			older.random = &sequenceReader{}
			published, err := older.Issue(ctx, fixture.user)
			if err != nil {
				t.Fatal(err)
			}

			service := New(fixture.db, instanceTestSecret, pair.previous)
			service.now = fixture.service.now
			service.random = &sequenceReader{next: 100}
			identity, err := service.Authenticate(ctx, published)
			if err != nil || identity.UserID != fixture.user.ID || identity.Legacy {
				t.Fatalf("session signed with a previous secret: identity=%+v err=%v", identity, err)
			}

			issued, err := service.Issue(ctx, fixture.user)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{currentDefaultKey, legacyDefaultKey, pair.previous} {
				if _, err := parseClaims(value, issued); err == nil {
					t.Errorf("a new session verifies with the previous value %q", value)
				}
			}
			if _, err := parseClaims(instanceTestSecret, issued); err != nil {
				t.Fatalf("a new session is not signed with the instance secret: %v", err)
			}
			if identity, err := service.Authenticate(ctx, issued); err != nil || identity.UserID != fixture.user.ID {
				t.Fatalf("new session: identity=%+v err=%v", identity, err)
			}
			if _, err := older.Authenticate(ctx, issued); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("an older binary accepted a session signed with the instance secret: %v", err)
			}
		})
	}
}

func TestPreviousSecretRefusesEveryLegacyTokenBeforeAccountLookup(t *testing.T) {
	for _, signer := range []string{instanceTestSecret, currentDefaultKey, legacyDefaultKey} {
		t.Run(signer[:8], func(t *testing.T) {
			fixture := newServiceFixture(t)
			ctx := context.Background()
			service := New(fixture.db, instanceTestSecret, currentDefaultKey)
			service.now = fixture.service.now
			// Account and token both predate the marker, inside the window:
			// a custom-secret instance would adopt this token.
			token := signLegacyToken(t, signer, fixture.user.ID, fixture.marker.Add(-time.Hour))

			if identity, err := service.Authenticate(ctx, token); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("legacy token under an instance secret: identity=%+v err=%v", identity, err)
			}
			if count := countLegacySessions(t, fixture); count != 0 {
				t.Fatalf("legacy token created %d legacy sessions", count)
			}
			sqlDB, err := fixture.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			if err := sqlDB.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Authenticate(ctx, token); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("legacy token under an instance secret reached the database: %v", err)
			}
		})
	}

	t.Run("custom-control", func(t *testing.T) {
		fixture := newServiceFixture(t)
		token := signLegacyToken(t, fixture.service.secret, fixture.user.ID, fixture.marker.Add(-time.Hour))
		if identity, err := fixture.service.Authenticate(context.Background(), token); err != nil || !identity.Legacy {
			t.Fatalf("custom secret without previous no longer adopts: identity=%+v err=%v", identity, err)
		}
	})
}
