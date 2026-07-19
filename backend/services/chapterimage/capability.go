package chapterimage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const capabilityPurpose = "openreader:chapter-image:v1"

type capabilityClaims struct {
	UserID      uint   `json:"u"`
	BookID      uint   `json:"b"`
	SourceID    uint   `json:"s"`
	Key         string `json:"k"`
	Fingerprint string `json:"f"`
	Purpose     string `json:"p"`
	ExpiresAt   int64  `json:"e"`
}

func signCapability(secret string, claims capabilityClaims) (string, error) {
	if err := validateClaims(claims); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := capabilitySignature(secret, encoded)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func verifyCapability(secret, token string, now time.Time) (capabilityClaims, error) {
	var claims capabilityClaims
	if token == "" || len(token) > 2048 {
		return claims, ErrMalformedCapability
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, ErrMalformedCapability
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != parts[1] ||
		!hmac.Equal(signature, capabilitySignature(secret, parts[0])) {
		return claims, ErrInvalidCapability
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || base64.RawURLEncoding.EncodeToString(payload) != parts[0] {
		return claims, ErrMalformedCapability
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return capabilityClaims{}, ErrMalformedCapability
	}
	if err := validateClaims(claims); err != nil {
		return capabilityClaims{}, ErrInvalidCapability
	}
	if now.Unix() >= claims.ExpiresAt {
		return capabilityClaims{}, ErrExpiredCapability
	}
	return claims, nil
}

func validateClaims(claims capabilityClaims) error {
	if claims.UserID == 0 || claims.BookID == 0 || claims.SourceID == 0 ||
		claims.Purpose != capabilityPurpose || claims.ExpiresAt <= 0 ||
		!validImageKey(claims.Key) || len(claims.Fingerprint) != sha256.Size*2 {
		return ErrInvalidCapability
	}
	if _, err := hex.DecodeString(claims.Fingerprint); err != nil {
		return ErrInvalidCapability
	}
	return nil
}

func capabilitySignature(secret, payload string) []byte {
	derivation := hmac.New(sha256.New, []byte(secret))
	_, _ = derivation.Write([]byte(capabilityPurpose))
	key := derivation.Sum(nil)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
