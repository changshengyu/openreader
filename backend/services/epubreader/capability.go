package epubreader

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const resourceCapabilityPurpose = "openreader:epub-resource:v1"

type resourceClaims struct {
	UserID              uint   `json:"u"`
	BookID              uint   `json:"b"`
	Fingerprint         string `json:"f"`
	Purpose             string `json:"p"`
	ExpiresAt           int64  `json:"e"`
	DocumentPath        string `json:"d,omitempty"`
	ResourceFragment    string `json:"s,omitempty"`
	ResourceEndFragment string `json:"z,omitempty"`
}

func signResourceCapability(secret string, claims resourceClaims) (string, error) {
	if err := validateResourceClaims(claims); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := capabilitySignature(secret, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func verifyResourceCapability(secret, token string, now time.Time) (resourceClaims, error) {
	var claims resourceClaims
	if len(token) == 0 || len(token) > 2048 {
		return claims, ErrMalformedCapability
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, ErrMalformedCapability
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrMalformedCapability
	}
	if !hmac.Equal(signature, capabilitySignature(secret, parts[0])) {
		return claims, ErrInvalidCapability
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, ErrMalformedCapability
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return resourceClaims{}, ErrMalformedCapability
	}
	if err := validateResourceClaims(claims); err != nil {
		return resourceClaims{}, ErrInvalidCapability
	}
	if now.Unix() >= claims.ExpiresAt {
		return resourceClaims{}, ErrExpiredCapability
	}
	return claims, nil
}

func validateResourceClaims(claims resourceClaims) error {
	if claims.UserID == 0 || claims.BookID == 0 ||
		claims.Purpose != resourceCapabilityPurpose ||
		claims.ExpiresAt <= 0 ||
		len(claims.Fingerprint) != sha256.Size*2 {
		return ErrInvalidCapability
	}
	if _, err := hex.DecodeString(claims.Fingerprint); err != nil {
		return ErrInvalidCapability
	}
	if claims.DocumentPath == "" {
		if claims.ResourceFragment != "" || claims.ResourceEndFragment != "" {
			return ErrInvalidCapability
		}
		return nil
	}
	canonical, err := normalizeArchivePath(claims.DocumentPath)
	if err != nil || canonical != claims.DocumentPath {
		return ErrInvalidCapability
	}
	if fragment, err := normalizeResourceFragment(claims.ResourceFragment); err != nil || fragment != claims.ResourceFragment {
		return ErrInvalidCapability
	}
	if fragment, err := normalizeResourceFragment(claims.ResourceEndFragment); err != nil || fragment != claims.ResourceEndFragment {
		return ErrInvalidCapability
	}
	return nil
}

func capabilitySignature(secret, payload string) []byte {
	derivation := hmac.New(sha256.New, []byte(secret))
	_, _ = derivation.Write([]byte(resourceCapabilityPurpose))
	key := derivation.Sum(nil)

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
