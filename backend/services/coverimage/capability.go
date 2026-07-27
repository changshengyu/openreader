package coverimage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	capabilityPurpose = "openreader:cover-image:v1"
	capabilityVersion = 1
	capabilityTTL     = 48 * time.Hour
)

type capabilityClaims struct {
	Version   int    `json:"v"`
	Purpose   string `json:"p"`
	UserID    uint   `json:"u"`
	SourceID  uint   `json:"s,omitempty"`
	URL       string `json:"r"`
	ExpiresAt int64  `json:"e"`
}

func sealCapability(secret string, claims capabilityClaims) (string, error) {
	if strings.TrimSpace(secret) == "" || validateClaims(claims) != nil {
		return "", ErrInvalidCapability
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	aead, err := capabilityAEAD(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, payload, []byte(capabilityPurpose))
	token := append(nonce, sealed...)
	return "v1." + base64.RawURLEncoding.EncodeToString(token), nil
}

func openCapability(secret, token string, now time.Time) (capabilityClaims, error) {
	var claims capabilityClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] != "v1" || strings.TrimSpace(parts[1]) == "" {
		return claims, ErrMalformedCapability
	}
	encoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrMalformedCapability
	}
	aead, err := capabilityAEAD(secret)
	if err != nil {
		return claims, err
	}
	if len(encoded) <= aead.NonceSize() {
		return claims, ErrMalformedCapability
	}
	payload, err := aead.Open(nil, encoded[:aead.NonceSize()], encoded[aead.NonceSize():], []byte(capabilityPurpose))
	if err != nil {
		return claims, ErrInvalidCapability
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return capabilityClaims{}, ErrInvalidCapability
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return capabilityClaims{}, ErrInvalidCapability
	}
	if err := validateClaims(claims); err != nil {
		return capabilityClaims{}, err
	}
	if !now.Before(time.Unix(claims.ExpiresAt, 0)) {
		return capabilityClaims{}, ErrExpiredCapability
	}
	return claims, nil
}

func capabilityAEAD(secret string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(capabilityPurpose + "\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func validateClaims(claims capabilityClaims) error {
	if claims.Version != capabilityVersion || claims.Purpose != capabilityPurpose ||
		claims.UserID == 0 || strings.TrimSpace(claims.URL) == "" || claims.ExpiresAt <= 0 {
		return ErrInvalidCapability
	}
	return nil
}
