package authsession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openreader/backend/models"
)

const (
	SessionTTL        = 7 * 24 * time.Hour
	MaxUserSessions   = 64
	MigrationKey      = "authenticated-session-v1"
	legacyDefaultKey  = "change-me-in-production"
	currentDefaultKey = "change-this-before-deploy"
)

var ErrInvalidSession = errors.New("invalid authenticated session")

type Claims struct {
	UserID      uint   `json:"userId"`
	AuthVersion uint64 `json:"authVersion,omitempty"`
	jwt.RegisteredClaims
}

type Identity struct {
	UserID uint
	Key    string
	Legacy bool
}

type Service struct {
	db     *gorm.DB
	secret string
	now    func() time.Time
	random io.Reader
}

func New(database *gorm.DB, secret string) *Service {
	return &Service{
		db:     database,
		secret: secret,
		now:    time.Now,
		random: rand.Reader,
	}
}

func (s *Service) Issue(ctx context.Context, user models.User) (string, error) {
	if user.ID == 0 {
		return "", ErrInvalidSession
	}

	random := make([]byte, 32)
	if _, err := io.ReadFull(s.random, random); err != nil {
		return "", fmt.Errorf("generate session identity: %w", err)
	}
	jti := base64.RawURLEncoding.EncodeToString(random)
	now := s.now().UTC()
	sessionKey := sessionDigest("jti", jti)
	var authVersion uint64
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.User{}).Where("id = ?", user.ID).Pluck("auth_version", &authVersion)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 || authVersion == 0 {
			return ErrInvalidSession
		}
		if err := tx.Where("user_id = ? AND expires_at <= ?", user.ID, now).
			Delete(&models.UserSession{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.UserSession{
			ID:          sessionKey,
			UserID:      user.ID,
			AuthVersion: authVersion,
			CreatedAt:   now,
			LastSeenAt:  now,
			ExpiresAt:   now.Add(SessionTTL),
		}).Error; err != nil {
			return err
		}
		return pruneUserSessions(tx, user.ID)
	}); err != nil {
		return "", fmt.Errorf("persist session: %w", err)
	}
	claims := Claims{
		UserID:      user.ID,
		AuthVersion: authVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:       jti,
			IssuedAt: jwt.NewNumericDate(now),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.secret))
	if err != nil {
		_ = s.db.WithContext(ctx).Where("id = ? AND user_id = ?", sessionKey, user.ID).Delete(&models.UserSession{}).Error
		return "", fmt.Errorf("sign session token: %w", err)
	}
	return token, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	claims, err := parseClaims(s.secret, token)
	if err != nil || claims.UserID == 0 || claims.IssuedAt == nil {
		return Identity{}, ErrInvalidSession
	}
	now := s.now().UTC()
	if claims.IssuedAt.Time.After(now.Add(time.Minute)) {
		return Identity{}, ErrInvalidSession
	}

	var user models.User
	if err := s.db.WithContext(ctx).Select("id", "auth_version").First(&user, claims.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Identity{}, ErrInvalidSession
		}
		return Identity{}, err
	}
	if user.AuthVersion == 0 {
		return Identity{}, ErrInvalidSession
	}

	if claims.ID != "" || claims.AuthVersion != 0 {
		if claims.ID == "" || claims.AuthVersion == 0 || claims.AuthVersion != user.AuthVersion {
			return Identity{}, ErrInvalidSession
		}
		key := sessionDigest("jti", claims.ID)
		if err := s.renew(ctx, key, user.ID, user.AuthVersion, now); err != nil {
			return Identity{}, err
		}
		return Identity{UserID: user.ID, Key: key}, nil
	}

	if user.AuthVersion != 1 {
		return Identity{}, ErrInvalidSession
	}
	marker, err := s.migrationTime(ctx)
	if err != nil {
		return Identity{}, err
	}
	if claims.IssuedAt.Time.After(marker) || now.After(marker.Add(SessionTTL)) {
		return Identity{}, ErrInvalidSession
	}
	key := sessionDigest("legacy", token)
	if err := s.adoptLegacy(ctx, key, user.ID, now); err != nil {
		return Identity{}, err
	}
	return Identity{UserID: user.ID, Key: key, Legacy: true}, nil
}

func (s *Service) Revoke(ctx context.Context, identity Identity) error {
	if identity.UserID == 0 || identity.Key == "" {
		return ErrInvalidSession
	}
	now := s.now().UTC()
	query := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", identity.Key, identity.UserID, now)
	var result *gorm.DB
	if identity.Legacy {
		result = query.Model(&models.UserSession{}).Updates(map[string]any{"revoked_at": now, "last_seen_at": now})
	} else {
		result = query.Delete(&models.UserSession{})
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidSession
	}
	return nil
}

func (s *Service) RevokeUser(ctx context.Context, tx *gorm.DB, userID uint) error {
	database := tx
	if database == nil {
		database = s.db
	}
	return database.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.UserSession{}).Error
}

func (s *Service) renew(ctx context.Context, key string, userID uint, version uint64, now time.Time) error {
	result := s.db.WithContext(ctx).Model(&models.UserSession{}).
		Where("id = ? AND user_id = ? AND auth_version = ? AND revoked_at IS NULL AND expires_at > ?", key, userID, version, now).
		Updates(map[string]any{"last_seen_at": now, "expires_at": now.Add(SessionTTL)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidSession
	}
	return nil
}

func (s *Service) adoptLegacy(ctx context.Context, key string, userID uint, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session := models.UserSession{
			ID:          key,
			UserID:      userID,
			AuthVersion: 1,
			Legacy:      true,
			CreatedAt:   now,
			LastSeenAt:  now,
			ExpiresAt:   now.Add(SessionTTL),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&session).Error; err != nil {
			return err
		}
		result := tx.Model(&models.UserSession{}).
			Where("id = ? AND user_id = ? AND auth_version = 1 AND revoked_at IS NULL AND expires_at > ?", key, userID, now).
			Updates(map[string]any{"last_seen_at": now, "expires_at": now.Add(SessionTTL)})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvalidSession
		}
		return pruneUserSessions(tx, userID)
	})
}

func (s *Service) migrationTime(ctx context.Context) (time.Time, error) {
	var migration models.SchemaMigration
	if err := s.db.WithContext(ctx).Where("key = ?", MigrationKey).First(&migration).Error; err != nil {
		return time.Time{}, err
	}
	return migration.AppliedAt.UTC(), nil
}

func pruneUserSessions(tx *gorm.DB, userID uint) error {
	var stale []string
	if err := tx.Model(&models.UserSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("last_seen_at DESC, created_at DESC, id DESC").
		Offset(MaxUserSessions).Pluck("id", &stale).Error; err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	return tx.Where("user_id = ? AND id IN ?", userID, stale).Delete(&models.UserSession{}).Error
}

func parseClaims(secret, token string) (Claims, error) {
	var lastErr error
	for _, candidate := range compatibleSecrets(secret) {
		claims := Claims{}
		parsed, err := jwt.ParseWithClaims(token, &claims, func(parsed *jwt.Token) (any, error) {
			if parsed.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidSession
			}
			return []byte(candidate), nil
		}, jwt.WithoutClaimsValidation())
		if err == nil && parsed.Valid {
			return claims, nil
		}
		lastErr = err
	}
	return Claims{}, lastErr
}

func compatibleSecrets(secret string) []string {
	switch secret {
	case legacyDefaultKey:
		return []string{legacyDefaultKey, currentDefaultKey}
	case currentDefaultKey:
		return []string{currentDefaultKey, legacyDefaultKey}
	default:
		return []string{secret}
	}
}

func sessionDigest(domain, value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(domain) + "\x00" + value))
	return hex.EncodeToString(digest[:])
}
