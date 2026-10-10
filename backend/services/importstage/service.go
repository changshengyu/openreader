package importstage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"openreader/backend/models"
	"openreader/backend/services/localbook"
	"openreader/backend/services/rootedfs"
)

const Lifetime = 24 * time.Hour
const CleanupInterval = time.Hour
const MetadataLimit int64 = 1 << 20

var (
	ErrInvalidToken  = errors.New("invalid or expired local import token")
	ErrTooLarge      = errors.New("local book exceeds maximum import size")
	ErrStageWrite    = errors.New("failed to stage import")
	ErrPreparedWrite = errors.New("failed to stage parsed import")
)

type Metadata struct {
	FileName  string    `json:"fileName"`
	Extension string    `json:"extension"`
	CreatedAt time.Time `json:"createdAt"`
}
type Hook func(phase, directory, token, path string)
type Limits struct {
	Source, Prepared int64
	ValidPrepared    func(localbook.PreparedImport) bool
}
type Service struct {
	cache  string
	limits Limits
	hook   Hook
}
type Session struct {
	service  *Service
	root     *rootedfs.PrivateScope
	ctx      context.Context
	release  func()
	Token    string
	Metadata Metadata
	Data     []byte
	entries  map[string]*rootedfs.PrivateEntry
}

func New(cache string, limits Limits, hook Hook) *Service {
	return &Service{cache: cache, limits: limits, hook: hook}
}
func ValidToken(token string) bool {
	if len(token) != 48 || token != strings.ToLower(token) {
		return false
	}
	b, err := hex.DecodeString(token)
	return err == nil && len(b) == 24
}
func (s *Service) phase(phase, dir, token, path string) {
	if s.hook != nil {
		s.hook(phase, dir, token, path)
	}
}
func tokenError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrInvalidToken
}
func (s *Service) begin(ctx context.Context, user uint, token string, create bool) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !ValidToken(token) {
		return nil, ErrInvalidToken
	}
	key := leaseKey(s.cache, strconv.FormatUint(uint64(user), 10), token)
	release, err := acquire(ctx, key, false)
	if err != nil {
		return nil, err
	}
	root, err := rootedfs.OpenPrivateScope(ctx, s.cache, filepath.Join("import-previews", strconv.FormatUint(uint64(user), 10)), create)
	if err != nil {
		release()
		return nil, err
	}
	x := &Session{service: s, root: root, ctx: ctx, release: release, Token: token, entries: make(map[string]*rootedfs.PrivateEntry)}
	for _, suffix := range []string{".book", ".json", ".parsed.json"} {
		entry, err := root.Snapshot(token + suffix)
		if err != nil {
			x.Close()
			return nil, err
		}
		x.entries[suffix] = entry
	}
	return x, nil
}

func (s *Service) Create(ctx context.Context, user uint, name, extension string, data []byte) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > s.limits.Source {
		return nil, ErrTooLarge
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, ErrStageWrite
	}
	x, err := s.begin(ctx, user, hex.EncodeToString(b), true)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStageWrite
	}
	complete := false
	defer func() {
		if !complete {
			x.Close()
		}
	}()
	s.phase("create-admitted", x.root.PathLabel(), x.Token, "")
	if err := x.Validate(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStageWrite
	}
	s.cleanupUser(ctx, x.root, strconv.FormatUint(uint64(user), 10), time.Now())
	if err := x.Validate(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStageWrite
	}
	x.Metadata = Metadata{FileName: filepath.Base(name), Extension: strings.ToLower(strings.TrimSpace(extension)), CreatedAt: time.Now().UTC()}
	encoded, err := json.Marshal(x.Metadata)
	if err != nil || int64(len(encoded)) > MetadataLimit {
		return nil, ErrStageWrite
	}
	if err := x.root.WriteNew(x.entries[".book"], x.writer(data, "create-write")); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStageWrite
	}
	defer func() {
		if !complete {
			_ = x.root.RemoveOwned(x.entries[".book"])
			_ = x.root.RemoveOwned(x.entries[".json"])
		}
	}()
	s.phase("create-data-written", x.root.PathLabel(), x.Token, x.path(".book"))
	if err := x.root.WriteNew(x.entries[".json"], x.writer(encoded, "metadata-write")); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStageWrite
	}
	// The two writes form one bundle handoff. Checking only the last file's
	// inode would miss a raw-file replacement during the metadata write.
	if err := x.Validate(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStageWrite
	}
	x.Data = data
	x.root.KeepDirectories()
	complete = true
	return x, nil
}

func (s *Service) Open(ctx context.Context, user uint, token string) (*Session, error) {
	x, err := s.begin(ctx, user, token, false)
	if err != nil {
		return nil, tokenError(err)
	}
	complete := false
	defer func() {
		if !complete {
			x.Close()
		}
	}()
	encoded, err := x.read(".json", MetadataLimit, "metadata-read")
	if err != nil {
		return nil, tokenError(err)
	}
	if json.Unmarshal(encoded, &x.Metadata) != nil || x.Metadata.CreatedAt.IsZero() || time.Since(x.Metadata.CreatedAt) > Lifetime {
		_ = x.Consume()
		return nil, ErrInvalidToken
	}
	s.phase("load-metadata", x.root.PathLabel(), token, x.path(".book"))
	x.Data, err = x.read(".book", s.limits.Source, "load-raw-read")
	if err != nil {
		return nil, tokenError(err)
	}
	s.phase("raw-loaded", x.root.PathLabel(), token, x.path(".book"))
	if err := x.Validate(); err != nil {
		return nil, tokenError(err)
	}
	complete = true
	return x, nil
}

func (x *Session) path(suffix string) string {
	return filepath.Join(x.root.PathLabel(), x.Token+suffix)
}
func (x *Session) Close() {
	if x.root != nil {
		_ = x.root.Close()
		x.root = nil
	}
	if x.release != nil {
		x.release()
		x.release = nil
	}
}
func (x *Session) Validate() error {
	if x.root == nil {
		return ErrInvalidToken
	}
	if err := x.root.Validate(); err != nil {
		return tokenError(err)
	}
	for _, e := range x.entries {
		if !e.Read {
			if err := e.Validate(); err != nil {
				return tokenError(err)
			}
		}
	}
	return nil
}
func (x *Session) read(suffix string, limit int64, phase string) ([]byte, error) {
	e := x.entries[suffix]
	file, err := e.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ReadBounded(x.ctx, file, limit, x.root.Validate, func() { x.service.phase(phase, x.root.PathLabel(), x.Token, x.path(suffix)) })
}
func (x *Session) writer(data []byte, phase string) func(io.Writer) error {
	return func(output io.Writer) error {
		for len(data) > 0 {
			if err := x.root.Validate(); err != nil {
				return err
			}
			chunk := data
			if len(chunk) > 64<<10 {
				chunk = chunk[:64<<10]
			}
			n, err := output.Write(chunk)
			x.service.phase(phase, x.root.PathLabel(), x.Token, "")
			if err != nil {
				return err
			}
			if err := x.root.Validate(); err != nil {
				return err
			}
			if n != len(chunk) {
				return io.ErrShortWrite
			}
			data = data[n:]
		}
		return x.root.Validate()
	}
}

func (x *Session) SavePrepared(prepared localbook.PreparedImport) error {
	if err := x.Validate(); err != nil {
		return err
	}
	if x.service.limits.ValidPrepared != nil && !x.service.limits.ValidPrepared(prepared) {
		return ErrTooLarge
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(prepared); err != nil {
		return ErrPreparedWrite
	}
	if int64(buffer.Len()) > x.service.limits.Prepared {
		return ErrTooLarge
	}
	err := x.root.Replace(x.entries[".parsed.json"], x.Token+".parsed-", x.writer(buffer.Bytes(), "prepared-write"), func(path string) { x.service.phase("prepared-ready", x.root.PathLabel(), x.Token, path) })
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// This is a prepared publication failure, not token-read admission.
		// Keep the deployed write-stage wire error/status distinction.
		return ErrPreparedWrite
	}
	if err := x.Validate(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrPreparedWrite
	}
	return nil
}

func (x *Session) Prepared(request localbook.ImportRequest) (localbook.PreparedImport, bool, error) {
	if err := x.Validate(); err != nil {
		return localbook.PreparedImport{}, false, err
	}
	e := x.entries[".parsed.json"]
	if e.Info == nil {
		return localbook.PreparedImport{}, false, nil
	}
	encoded, err := x.read(".parsed.json", x.service.limits.Prepared, "prepared-read")
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, rootedfs.ErrUnsafePath) {
			return localbook.PreparedImport{}, false, tokenError(err)
		}
		if err := x.Validate(); err != nil {
			return localbook.PreparedImport{}, false, err
		}
		if errors.Is(err, ErrTooLarge) {
			_ = x.root.Remove(e)
		}
		return localbook.PreparedImport{}, false, nil
	}
	if err := x.Validate(); err != nil {
		return localbook.PreparedImport{}, false, err
	}
	var prepared localbook.PreparedImport
	if json.Unmarshal(encoded, &prepared) != nil || (x.service.limits.ValidPrepared != nil && !x.service.limits.ValidPrepared(prepared)) {
		if err := x.root.Remove(e); err != nil {
			return localbook.PreparedImport{}, false, tokenError(err)
		}
		return localbook.PreparedImport{}, false, nil
	}
	return prepared, prepared.Matches(request), nil
}

func (x *Session) Consume() error {
	if x.root == nil {
		return ErrInvalidToken
	}
	x.service.phase("consume-admitted", x.root.PathLabel(), x.Token, x.path(".book"))
	var result error
	for _, suffix := range []string{".book", ".json", ".parsed.json"} {
		result = errors.Join(result, x.root.Remove(x.entries[suffix]))
	}
	return result
}

// ImportBook keeps the same admitted session through cached/fallback parsing
// and the final stage check before durability. Consumption is a separate,
// best-effort post-durable action: its failure must never roll back the Book.
// Parser CPU and all SQL/archive internals are not an atomic context boundary.
func (x *Session) ImportBook(importer localbook.Importer, request localbook.ImportRequest) (models.Book, error) {
	prepared, matched, err := x.Prepared(request)
	if err != nil {
		return models.Book{}, err
	}
	if !matched {
		_, prepared, err = importer.Prepare(request)
		if err != nil {
			return models.Book{}, err
		}
		if err := x.SavePrepared(prepared); err != nil {
			return models.Book{}, err
		}
	}
	if err := x.Validate(); err != nil {
		return models.Book{}, err
	}
	return importer.ImportPrepared(request, prepared)
}

type contextRead struct {
	ctx     context.Context
	reader  io.Reader
	check   func() error
	observe func()
}

func (r contextRead) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.check != nil {
		if err := r.check(); err != nil {
			return 0, err
		}
	}
	n, err := r.reader.Read(p)
	if r.observe != nil {
		r.observe()
	}
	if cancel := r.ctx.Err(); cancel != nil {
		return 0, cancel
	}
	if r.check != nil {
		if changed := r.check(); changed != nil {
			return 0, changed
		}
	}
	return n, err
}
func ReadBounded(ctx context.Context, reader io.Reader, limit int64, check func() error, observe func()) ([]byte, error) {
	probe := limit
	if probe < math.MaxInt64 {
		probe++
	}
	data, err := io.ReadAll(io.LimitReader(contextRead{ctx, reader, check, observe}, probe))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	return data, nil
}

type gate struct {
	slot chan struct{}
	refs int
}

var leases = struct {
	sync.Mutex
	values map[string]*gate
}{values: make(map[string]*gate)}

func leaseKey(cache, user, token string) string {
	absolute, _ := filepath.Abs(cache)
	return absolute + "\x00" + user + "\x00" + token
}
func acquire(ctx context.Context, key string, try bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	leases.Lock()
	g := leases.values[key]
	if g == nil {
		g = &gate{slot: make(chan struct{}, 1)}
		g.slot <- struct{}{}
		leases.values[key] = g
	}
	g.refs++
	leases.Unlock()
	drop := func() {
		leases.Lock()
		g.refs--
		if g.refs == 0 {
			delete(leases.values, key)
		}
		leases.Unlock()
	}
	if try {
		select {
		case <-g.slot:
		default:
			drop()
			return nil, nil
		}
	} else {
		select {
		case <-g.slot:
		case <-ctx.Done():
			drop()
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		g.slot <- struct{}{}
		drop()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { g.slot <- struct{}{}; drop() }) }, nil
}

func (s *Service) StartCleanup(ctx context.Context) {
	s.Cleanup(ctx)
	go func() {
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Cleanup(ctx)
			}
		}
	}()
}
func (s *Service) Cleanup(ctx context.Context) {
	root, err := rootedfs.OpenPrivateScope(ctx, s.cache, "import-previews", false)
	if err != nil {
		return
	}
	defer root.Close()
	entries, err := root.Entries()
	if err != nil {
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		name := entry.RelativePath
		if _, err := strconv.ParseUint(name, 10, 64); err != nil || !entry.Info.IsDir() || entry.Info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		child, err := root.Child(name, entry.Info)
		if err != nil {
			continue
		}
		s.cleanupUser(ctx, child, name, time.Now())
		_ = child.Close()
	}
}
func (s *Service) cleanupUser(ctx context.Context, root *rootedfs.PrivateScope, user string, now time.Time) {
	entries, err := root.Entries()
	if err != nil {
		return
	}
	byName := make(map[string]os.FileInfo, len(entries))
	tokens := make(map[string]bool)
	for _, entry := range entries {
		byName[entry.RelativePath] = entry.Info
		for _, suffix := range []string{".book", ".json", ".parsed.json"} {
			if strings.HasSuffix(entry.RelativePath, suffix) {
				token := strings.TrimSuffix(entry.RelativePath, suffix)
				if ValidToken(token) {
					tokens[token] = true
				}
			}
		}
	}
	cutoff := now.Add(-Lifetime)
	for token := range tokens {
		if ctx.Err() != nil {
			return
		}
		release, err := acquire(ctx, leaseKey(s.cache, user, token), true)
		if err != nil {
			return
		}
		if release == nil {
			continue
		}
		s.cleanupBundle(ctx, root, token, byName, cutoff)
		release()
	}
	for name, info := range byName {
		if ctx.Err() != nil {
			return
		}
		if !info.Mode().IsRegular() || info.ModTime().After(cutoff) || !strings.Contains(name, ".parsed-") {
			continue
		}
		token := strings.SplitN(name, ".parsed-", 2)[0]
		release, err := acquire(ctx, leaseKey(s.cache, user, token), true)
		if err != nil {
			return
		}
		if release == nil {
			continue
		}
		entry, err := root.SnapshotOriginal(name, info)
		if err == nil {
			s.phase("cleanup-unlink", root.PathLabel(), name, filepath.Join(root.PathLabel(), name))
			_ = root.Remove(entry)
		}
		release()
	}
}
func (s *Service) cleanupBundle(ctx context.Context, root *rootedfs.PrivateScope, token string, byName map[string]os.FileInfo, cutoff time.Time) {
	selected := make(map[string]*rootedfs.PrivateEntry)
	for _, suffix := range []string{".book", ".json", ".parsed.json"} {
		name := token + suffix
		entry, err := root.SnapshotOriginal(name, byName[name])
		if err != nil {
			return
		}
		selected[suffix] = entry
	}
	meta := selected[".json"]
	expired := false
	if meta.Info != nil {
		file, err := meta.Open()
		if err != nil {
			return
		}
		encoded, err := ReadBounded(ctx, file, MetadataLimit, root.Validate, func() {
			s.phase("cleanup-metadata-read", root.PathLabel(), token, filepath.Join(root.PathLabel(), token+".json"))
		})
		_ = file.Close()
		if err != nil {
			return
		}
		var metadata Metadata
		expired = json.Unmarshal(encoded, &metadata) != nil || metadata.CreatedAt.IsZero() || metadata.CreatedAt.Before(cutoff) || selected[".book"].Info == nil
		if !expired {
			return
		}
	} else {
		book := selected[".book"]
		expired = book.Info != nil && !book.Info.ModTime().After(cutoff)
	}
	if expired {
		s.phase("cleanup-remove", root.PathLabel(), token, filepath.Join(root.PathLabel(), token+".json"))
		for _, suffix := range []string{".book", ".json", ".parsed.json"} {
			_ = root.Remove(selected[suffix])
		}
		return
	}
	parsed := selected[".parsed.json"]
	if meta.Info == nil && parsed.Info != nil && !parsed.Info.ModTime().After(cutoff) {
		s.phase("cleanup-unlink", root.PathLabel(), parsed.Name, filepath.Join(root.PathLabel(), parsed.Name))
		_ = root.Remove(parsed)
	}
}
