package middleware

import "testing"

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
