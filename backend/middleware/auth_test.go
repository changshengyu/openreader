package middleware

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestDefaultJWTSecretsRemainCompatible(t *testing.T) {
	for _, issuedWith := range []string{legacyDefaultJWTSecret, currentDefaultJWTSecret} {
		token, err := GenerateToken(issuedWith, 42)
		if err != nil {
			t.Fatalf("generate token with %q: %v", issuedWith, err)
		}
		for _, verifiedWith := range []string{legacyDefaultJWTSecret, currentDefaultJWTSecret} {
			userID, err := ParseToken(verifiedWith, token)
			if err != nil {
				t.Fatalf("token issued with %q was rejected by %q: %v", issuedWith, verifiedWith, err)
			}
			if userID != 42 {
				t.Fatalf("user id = %d, want 42", userID)
			}
		}
	}
}

func TestCustomJWTSecretDoesNotTrustDefaultSecrets(t *testing.T) {
	token, err := GenerateToken(legacyDefaultJWTSecret, 42)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken("custom-production-secret", token); err == nil {
		t.Fatal("custom secret unexpectedly accepted a token signed with a public default")
	}
}

func TestExpiredLegacyTokenRemainsUsable(t *testing.T) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: 42,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-45 * 24 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-15 * 24 * time.Hour)),
		},
	}).SignedString([]byte(currentDefaultJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	userID, err := ParseToken(currentDefaultJWTSecret, token)
	if err != nil {
		t.Fatalf("expired legacy token was rejected: %v", err)
	}
	if userID != 42 {
		t.Fatalf("user id = %d, want 42", userID)
	}
}

func TestTokenRejectsUnexpectedAlgorithmAndInvalidUser(t *testing.T) {
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{UserID: 42}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(currentDefaultJWTSecret, unsigned); err == nil {
		t.Fatal("unsigned token was accepted")
	}

	zeroUser, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{}).SignedString([]byte(currentDefaultJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(currentDefaultJWTSecret, zeroUser); err == nil {
		t.Fatal("token without a user id was accepted")
	}
}
