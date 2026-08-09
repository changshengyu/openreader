package remotereader

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"openreader/backend/models"
)

var (
	ErrMissing  = errors.New("remote reader session missing")
	ErrExpired  = errors.New("remote reader session expired")
	ErrTooLarge = errors.New("remote reader session exceeds retention limit")
)

type Limits struct {
	IdleTTL         time.Duration
	MaxTTL          time.Duration
	MaxSessionBytes int64
	MaxUserSessions int
	MaxUserBytes    int64
	MaxSessions     int
	MaxBytes        int64
}

func DefaultLimits() Limits {
	return Limits{
		IdleTTL:         30 * time.Minute,
		MaxTTL:          4 * time.Hour,
		MaxSessionBytes: 8 << 20,
		MaxUserSessions: 8,
		MaxUserBytes:    32 << 20,
		MaxSessions:     128,
		MaxBytes:        128 << 20,
	}
}

type Session struct {
	ID              string
	UserID          uint
	Source          models.BookSource
	Book            models.Book
	Chapters        []models.Chapter
	CreatedAt       time.Time
	ExpiresAt       time.Time
	MaxExpiresAt    time.Time
	LastAccessedAt  time.Time
	RetainedBytes   int64
	lastAccessOrder uint64
}

type Store struct {
	mu          sync.Mutex
	sessions    map[string]*Session
	expired     map[string]expiredSession
	limits      Limits
	now         func() time.Time
	accessOrder uint64
}

type expiredSession struct {
	UserID    uint
	ExpiredAt time.Time
	RemoveAt  time.Time
}

const maxExpiredSessionMarkers = 1024

func NewStore(limits Limits, now func() time.Time) *Store {
	defaults := DefaultLimits()
	if limits.IdleTTL <= 0 {
		limits.IdleTTL = defaults.IdleTTL
	}
	if limits.MaxTTL <= 0 {
		limits.MaxTTL = defaults.MaxTTL
	}
	if limits.MaxSessionBytes <= 0 {
		limits.MaxSessionBytes = defaults.MaxSessionBytes
	}
	if limits.MaxUserSessions <= 0 {
		limits.MaxUserSessions = defaults.MaxUserSessions
	}
	if limits.MaxUserBytes <= 0 {
		limits.MaxUserBytes = defaults.MaxUserBytes
	}
	if limits.MaxSessions <= 0 {
		limits.MaxSessions = defaults.MaxSessions
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = defaults.MaxBytes
	}
	if now == nil {
		now = time.Now
	}
	return &Store{
		sessions: make(map[string]*Session),
		expired:  make(map[string]expiredSession),
		limits:   limits,
		now:      now,
	}
}

func (s *Store) Create(userID uint, source models.BookSource, book models.Book, chapters []models.Chapter) (Session, error) {
	retainedBytes, err := retainedBytesFor(source, book, chapters)
	if err != nil {
		return Session{}, err
	}
	if retainedBytes > s.limits.MaxSessionBytes || retainedBytes > s.limits.MaxUserBytes || retainedBytes > s.limits.MaxBytes {
		return Session{}, ErrTooLarge
	}
	identifier, err := randomSessionID()
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	session := Session{
		ID:             identifier,
		UserID:         userID,
		Source:         cloneBookSource(source),
		Book:           book,
		Chapters:       cloneChapters(chapters),
		CreatedAt:      now,
		ExpiresAt:      now.Add(s.limits.IdleTTL),
		MaxExpiresAt:   now.Add(s.limits.MaxTTL),
		LastAccessedAt: now,
		RetainedBytes:  retainedBytes,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	for s.userSessionCountLocked(userID)+1 > s.limits.MaxUserSessions || s.userBytesLocked(userID)+retainedBytes > s.limits.MaxUserBytes {
		if !s.evictOldestLocked(userID, identifier) {
			return Session{}, ErrTooLarge
		}
	}
	for len(s.sessions)+1 > s.limits.MaxSessions || s.totalBytesLocked()+retainedBytes > s.limits.MaxBytes {
		if !s.evictOldestLocked(0, identifier) {
			return Session{}, ErrTooLarge
		}
	}
	s.touchLocked(&session, now)
	s.sessions[identifier] = &session
	return cloneSession(session), nil
}

func (s *Store) Get(userID uint, identifier string) (Session, error) {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	identifier = strings.TrimSpace(identifier)
	session, ok := s.sessions[identifier]
	if !ok {
		s.purgeExpiredLocked(now)
		if marker, exists := s.expired[identifier]; exists && marker.UserID == userID {
			return Session{}, ErrExpired
		}
		return Session{}, ErrMissing
	}
	if session.UserID != userID {
		s.purgeExpiredLocked(now)
		return Session{}, ErrMissing
	}
	if s.expiredLocked(session, now) {
		s.markExpiredLocked(session, now)
		return Session{}, ErrExpired
	}
	session.ExpiresAt = minExpiry(now.Add(s.limits.IdleTTL), session.MaxExpiresAt)
	s.touchLocked(session, now)
	return cloneSession(*session), nil
}

func (s *Store) UpdateVariables(userID uint, identifier, bookVariable string, chapterIndex int, chapterVariable string) error {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	identifier = strings.TrimSpace(identifier)
	session, ok := s.sessions[identifier]
	if !ok {
		if marker, exists := s.expired[identifier]; exists && marker.UserID == userID && now.Before(marker.RemoveAt) {
			return ErrExpired
		}
		return ErrMissing
	}
	if session.UserID != userID {
		return ErrMissing
	}
	if s.expiredLocked(session, now) {
		s.markExpiredLocked(session, now)
		return ErrExpired
	}

	updated := cloneSession(*session)
	updated.Book.Variable = bookVariable
	chapterFound := false
	for index := range updated.Chapters {
		if updated.Chapters[index].Index == chapterIndex {
			updated.Chapters[index].Variable = chapterVariable
			chapterFound = true
			break
		}
	}
	if !chapterFound {
		return ErrMissing
	}
	retainedBytes, err := retainedBytesFor(updated.Source, updated.Book, updated.Chapters)
	if err != nil {
		return err
	}
	if retainedBytes > s.limits.MaxSessionBytes || retainedBytes > s.limits.MaxUserBytes || retainedBytes > s.limits.MaxBytes {
		return ErrTooLarge
	}
	for s.userBytesLocked(userID)-session.RetainedBytes+retainedBytes > s.limits.MaxUserBytes {
		if !s.evictOldestLocked(userID, identifier) {
			return ErrTooLarge
		}
	}
	for s.totalBytesLocked()-session.RetainedBytes+retainedBytes > s.limits.MaxBytes {
		if !s.evictOldestLocked(0, identifier) {
			return ErrTooLarge
		}
	}
	updated.RetainedBytes = retainedBytes
	s.touchLocked(&updated, now)
	s.sessions[identifier] = &updated
	return nil
}

func (s *Store) touchLocked(session *Session, now time.Time) {
	s.accessOrder++
	session.LastAccessedAt = now
	session.lastAccessOrder = s.accessOrder
}

func (s *Store) expiredLocked(session *Session, now time.Time) bool {
	return !now.Before(session.ExpiresAt) || !now.Before(session.MaxExpiresAt)
}

func (s *Store) purgeExpiredLocked(now time.Time) {
	for identifier, marker := range s.expired {
		if !now.Before(marker.RemoveAt) {
			delete(s.expired, identifier)
		}
	}
	for _, session := range s.sessions {
		if s.expiredLocked(session, now) {
			s.markExpiredLocked(session, now)
		}
	}
}

func (s *Store) markExpiredLocked(session *Session, now time.Time) {
	delete(s.sessions, session.ID)
	s.expired[session.ID] = expiredSession{
		UserID:    session.UserID,
		ExpiredAt: now,
		RemoveAt:  now.Add(s.limits.MaxTTL),
	}
	for len(s.expired) > maxExpiredSessionMarkers {
		var oldestID string
		var oldest expiredSession
		for identifier, marker := range s.expired {
			if oldestID == "" || marker.ExpiredAt.Before(oldest.ExpiredAt) || marker.ExpiredAt.Equal(oldest.ExpiredAt) && identifier < oldestID {
				oldestID = identifier
				oldest = marker
			}
		}
		delete(s.expired, oldestID)
	}
}

func (s *Store) evictOldestLocked(userID uint, excludeID string) bool {
	var candidate *Session
	for _, session := range s.sessions {
		if session.ID == excludeID || userID != 0 && session.UserID != userID {
			continue
		}
		if candidate == nil || session.lastAccessOrder < candidate.lastAccessOrder || session.lastAccessOrder == candidate.lastAccessOrder && (session.CreatedAt.Before(candidate.CreatedAt) || session.CreatedAt.Equal(candidate.CreatedAt) && session.ID < candidate.ID) {
			candidate = session
		}
	}
	if candidate == nil {
		return false
	}
	delete(s.sessions, candidate.ID)
	return true
}

func (s *Store) userSessionCountLocked(userID uint) int {
	count := 0
	for _, session := range s.sessions {
		if session.UserID == userID {
			count++
		}
	}
	return count
}

func (s *Store) userBytesLocked(userID uint) int64 {
	var total int64
	for _, session := range s.sessions {
		if session.UserID == userID {
			total += session.RetainedBytes
		}
	}
	return total
}

func (s *Store) totalBytesLocked() int64 {
	var total int64
	for _, session := range s.sessions {
		total += session.RetainedBytes
	}
	return total
}

func retainedBytesFor(source models.BookSource, book models.Book, chapters []models.Chapter) (int64, error) {
	projection := struct {
		Source   models.BookSource `json:"source"`
		Book     models.Book       `json:"book"`
		Chapters []models.Chapter  `json:"chapters"`
	}{
		Source:   source,
		Book:     book,
		Chapters: chapters,
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return 0, err
	}
	return int64(len(encoded)), nil
}

func randomSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func cloneSession(value Session) Session {
	value.Source = cloneBookSource(value.Source)
	value.Chapters = cloneChapters(value.Chapters)
	return value
}

func cloneBookSource(value models.BookSource) models.BookSource {
	if value.EnabledExplore != nil {
		enabled := *value.EnabledExplore
		value.EnabledExplore = &enabled
	}
	return value
}

func cloneChapters(value []models.Chapter) []models.Chapter {
	return append([]models.Chapter(nil), value...)
}

func minExpiry(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
