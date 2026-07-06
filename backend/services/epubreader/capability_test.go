package epubreader

import (
	"strings"
	"testing"
	"time"
)

func TestResourceCapabilityIsPurposeScopedAndExpires(t *testing.T) {
	now := time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)
	claims := resourceClaims{
		UserID:      11,
		BookID:      23,
		Fingerprint: strings.Repeat("a", 64),
		Purpose:     resourceCapabilityPurpose,
		ExpiresAt:   now.Add(time.Hour).Unix(),
	}
	token, err := signResourceCapability("test-secret", claims)
	if err != nil {
		t.Fatal(err)
	}
	got, err := verifyResourceCapability("test-secret", token, now)
	if err != nil {
		t.Fatal(err)
	}
	if got != claims {
		t.Fatalf("claims = %#v, want %#v", got, claims)
	}

	if _, err := verifyResourceCapability("test-secret", token+"x", now); err == nil {
		t.Fatal("modified capability unexpectedly verified")
	}
	if _, err := verifyResourceCapability("other-secret", token, now); err == nil {
		t.Fatal("capability unexpectedly verified with another secret")
	}
	if _, err := verifyResourceCapability("test-secret", token, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired capability unexpectedly verified")
	}

	claims.Purpose = "login"
	if _, err := signResourceCapability("test-secret", claims); err == nil {
		t.Fatal("wrong-purpose capability unexpectedly signed")
	}
}
