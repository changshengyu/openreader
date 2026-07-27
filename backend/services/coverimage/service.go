package coverimage

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

type Limits struct {
	MaxImageBytes int64
	MaxCacheBytes int64
	Timeout       time.Duration
	MaxRedirects  int
}

type Resource struct {
	Data        []byte
	ContentType string
	Size        int64
}

type downloadCall struct {
	done     chan struct{}
	resource Resource
	err      error
}

type projectionEntry struct {
	userID    uint
	resource  string
	expiresAt time.Time
}

const maxProjectionEntriesPerUser = 4096

type Service struct {
	cfg           config.Config
	db            *gorm.DB
	now           func() time.Time
	limits        Limits
	lookupIP      lookupIPFunc
	clientFactory func(requestPolicy) *http.Client

	mu               sync.Mutex
	cacheMu          sync.Mutex
	downloads        map[string]*downloadCall
	projections      map[string]projectionEntry
	projectionCounts map[uint]int
}

func New(cfg config.Config, database *gorm.DB) *Service {
	maxImageBytes := positiveInt64(cfg.MaxCoverImageBytes, 8*1024*1024)
	maxCacheBytes := positiveInt64(cfg.MaxCoverCacheBytes, 256*1024*1024)
	if maxCacheBytes < maxImageBytes {
		maxCacheBytes = maxImageBytes
	}
	timeout := time.Duration(positiveInt(cfg.CoverImageTimeoutSeconds, 3)) * time.Second
	service := &Service{
		cfg: cfg,
		db:  database,
		now: time.Now,
		limits: Limits{
			MaxImageBytes: maxImageBytes,
			MaxCacheBytes: maxCacheBytes,
			Timeout:       timeout,
			MaxRedirects:  positiveInt(cfg.MaxCoverImageRedirects, 3),
		},
		downloads:        make(map[string]*downloadCall),
		projections:      make(map[string]projectionEntry),
		projectionCounts: make(map[uint]int),
	}
	service.lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		ips := make([]net.IP, 0, len(addresses))
		for _, address := range addresses {
			ips = append(ips, address.IP)
		}
		return ips, nil
	}
	service.clientFactory = func(policy requestPolicy) *http.Client {
		return defaultClientForPolicy(policy, timeout)
	}
	return service
}

func positiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func (s *Service) Project(userID, sourceID uint, rawURL string) (string, error) {
	if userID == 0 {
		return "", ErrInvalidCapability
	}
	normalized, err := normalizeRemoteURL(rawURL)
	if err != nil {
		return "", err
	}
	projectionKey := strings.Join([]string{
		strconv.FormatUint(uint64(userID), 10),
		strconv.FormatUint(uint64(sourceID), 10),
		coverCacheKey(normalized),
	}, ":")
	now := s.now().UTC()
	s.mu.Lock()
	if existing, ok := s.projections[projectionKey]; ok && existing.expiresAt.After(now.Add(time.Hour)) {
		s.mu.Unlock()
		return existing.resource, nil
	}
	if _, ok := s.projections[projectionKey]; ok {
		delete(s.projections, projectionKey)
		s.projectionCounts[userID]--
	}
	if s.projectionCounts[userID] >= maxProjectionEntriesPerUser {
		s.ensureProjectionCapacityLocked(userID, now)
	}
	token, err := sealCapability(s.cfg.JWTSecret, capabilityClaims{
		Version:   capabilityVersion,
		Purpose:   capabilityPurpose,
		UserID:    userID,
		SourceID:  sourceID,
		URL:       normalized,
		ExpiresAt: now.Add(capabilityTTL).Unix(),
	})
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	resource := "/api/cover/" + url.PathEscape(token)
	s.projections[projectionKey] = projectionEntry{
		userID:    userID,
		resource:  resource,
		expiresAt: now.Add(capabilityTTL),
	}
	s.projectionCounts[userID]++
	s.mu.Unlock()
	return resource, nil
}

func (s *Service) ensureProjectionCapacityLocked(userID uint, now time.Time) {
	activeEntries := 0
	oldestKey := ""
	var oldestExpiry time.Time
	for key, entry := range s.projections {
		if entry.userID != userID {
			continue
		}
		if !entry.expiresAt.After(now) {
			delete(s.projections, key)
			continue
		}
		activeEntries++
		if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = entry.expiresAt
		}
	}
	if activeEntries >= maxProjectionEntriesPerUser && oldestKey != "" {
		delete(s.projections, oldestKey)
		activeEntries--
	}
	s.projectionCounts[userID] = activeEntries
}

func (s *Service) Open(ctx context.Context, capability string) (Resource, error) {
	claims, err := openCapability(s.cfg.JWTSecret, capability, s.now().UTC())
	if err != nil {
		return Resource{}, err
	}
	normalized, err := normalizeRemoteURL(claims.URL)
	if err != nil || normalized != claims.URL {
		return Resource{}, ErrInvalidCapability
	}
	if s.db != nil {
		var count int64
		if err := s.db.Model(&models.User{}).Where("id = ?", claims.UserID).Count(&count).Error; err != nil {
			return Resource{}, err
		}
		if count == 0 {
			return Resource{}, ErrUnavailable
		}
	}
	resource, err := s.readCached(claims.UserID, claims.URL)
	if err == nil {
		return resource, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Resource{}, err
	}

	key := strings.Join([]string{strconv.FormatUint(uint64(claims.UserID), 10), coverCacheKey(claims.URL)}, ":")
	s.mu.Lock()
	if existing := s.downloads[key]; existing != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return Resource{}, ctx.Err()
		case <-existing.done:
			return existing.resource, existing.err
		}
	}
	call := &downloadCall{done: make(chan struct{})}
	s.downloads[key] = call
	s.mu.Unlock()

	resource, err = s.downloadAndCache(ctx, claims)
	call.resource = resource
	call.err = err
	close(call.done)
	s.mu.Lock()
	delete(s.downloads, key)
	s.mu.Unlock()
	return resource, err
}

func (s *Service) downloadAndCache(ctx context.Context, claims capabilityClaims) (Resource, error) {
	policy := requestPolicy{LookupIP: s.lookupIP, MaxRedirects: s.limits.MaxRedirects}
	client := s.clientFactory(policy)
	data, contentType, err := fetchImage(ctx, client, policy, claims.URL, s.limits.MaxImageBytes)
	if err != nil {
		return Resource{}, unavailableError(err)
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if exists, err := s.userExists(claims.UserID); err != nil {
		return Resource{}, err
	} else if !exists {
		return Resource{}, ErrUnavailable
	}
	if err := s.writeCached(claims.UserID, claims.URL, data); err != nil {
		return Resource{}, err
	}
	return Resource{Data: data, ContentType: contentType, Size: int64(len(data))}, nil
}

func (s *Service) userExists(userID uint) (bool, error) {
	if s.db == nil {
		return true, nil
	}
	var count int64
	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) RemoveUser(userID uint) (FileStats, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	stats, err := s.removeUserCache(userID)
	s.mu.Lock()
	for key, entry := range s.projections {
		if entry.userID == userID {
			delete(s.projections, key)
		}
	}
	delete(s.projectionCounts, userID)
	s.mu.Unlock()
	return stats, err
}
