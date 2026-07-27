package coverimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

var contractPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
)

type contractRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn contractRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newContractService(t *testing.T, configure func(*config.Config)) (*Service, models.User, models.BookSource) {
	t.Helper()
	cfg := config.Config{
		CacheDir:                 t.TempDir(),
		JWTSecret:                "cover-contract-secret",
		MaxCoverImageBytes:       1024,
		MaxCoverCacheBytes:       4096,
		CoverImageTimeoutSeconds: 3,
		MaxCoverImageRedirects:   3,
	}
	if configure != nil {
		configure(&cfg)
	}
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cover.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.BookSource{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "cover-owner", PasswordHash: "unused", Role: "user"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "cover-source", BaseURL: "https://books.example", Enabled: true}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	service := New(cfg, database)
	service.lookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "cover.example", "redirect.example":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "private.example":
			return []net.IP{net.ParseIP("10.0.0.2")}, nil
		default:
			return nil, errors.New("unknown host")
		}
	}
	return service, user, source
}

func TestRemoteCoverCapabilityCacheAndIsolation(t *testing.T) {
	service, user, source := newContractService(t, nil)
	fixedNow := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	var requests atomic.Int32
	service.clientFactory = func(policy requestPolicy) *http.Client {
		return &http.Client{
			Transport: contractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				time.Sleep(10 * time.Millisecond)
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"text/html"}},
					Body:          io.NopCloser(bytes.NewReader(contractPNG)),
					ContentLength: int64(len(contractPNG)),
					Request:       request,
				}, nil
			}),
			CheckRedirect: policy.CheckRedirect,
		}
	}

	rawURL := "https://cover.example/art.png?credential=never-expose"
	resourceURL, err := service.Project(user.ID, source.ID, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resourceURL, "/api/cover/") ||
		strings.Contains(resourceURL, "cover.example") ||
		strings.Contains(resourceURL, "credential") {
		t.Fatalf("cover projection leaked source URL: %q", resourceURL)
	}
	reusedProjection, err := service.Project(user.ID, source.ID, rawURL)
	if err != nil || reusedProjection != resourceURL {
		t.Fatalf("same cover projection changed before expiry: first=%q second=%q err=%v", resourceURL, reusedProjection, err)
	}
	capability, err := url.PathUnescape(strings.TrimPrefix(resourceURL, "/api/cover/"))
	if err != nil {
		t.Fatal(err)
	}

	const readers = 8
	var wait sync.WaitGroup
	wait.Add(readers)
	for range readers {
		go func() {
			defer wait.Done()
			resource, openErr := service.Open(context.Background(), capability)
			if openErr != nil {
				t.Errorf("open cover: %v", openErr)
				return
			}
			if resource.ContentType != "image/png" || !bytes.Equal(resource.Data, contractPNG) {
				t.Errorf("unexpected resource: type=%q bytes=%x", resource.ContentType, resource.Data)
			}
		}()
	}
	wait.Wait()
	if requests.Load() != 1 {
		t.Fatalf("concurrent cache miss made %d requests, want 1", requests.Load())
	}

	if _, err := service.Open(context.Background(), capability); err != nil || requests.Load() != 1 {
		t.Fatalf("cache hit refetched: requests=%d err=%v", requests.Load(), err)
	}
	stats, err := service.StatsUser(user.ID)
	if err != nil || stats.Files != 1 || stats.Bytes != int64(len(contractPNG)) {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}

	tampered := tamperCapabilityBytes(t, capability)
	if _, err := service.Open(context.Background(), tampered); !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("tampered capability error=%v", err)
	}
	service.now = func() time.Time { return fixedNow.Add(capabilityTTL + time.Second) }
	if _, err := service.Open(context.Background(), capability); !errors.Is(err, ErrExpiredCapability) {
		t.Fatalf("expired capability error=%v", err)
	}

	if _, err := service.RemoveUser(user.ID); err != nil {
		t.Fatal(err)
	}
	stats, err = service.StatsUser(user.ID)
	if err != nil || stats.Files != 0 || stats.Bytes != 0 {
		t.Fatalf("removed user cache stats=%+v err=%v", stats, err)
	}
}

func tamperCapabilityBytes(t *testing.T, capability string) string {
	t.Helper()
	parts := strings.Split(capability, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected capability shape: %q", capability)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) == 0 {
		t.Fatalf("decode capability: bytes=%d err=%v", len(payload), err)
	}
	payload[len(payload)-1] ^= 0x01
	return parts[0] + "." + base64.RawURLEncoding.EncodeToString(payload)
}

func TestRemoteCoverProjectionCacheIsBoundedAndPrunesExpiredEntries(t *testing.T) {
	service, user, source := newContractService(t, nil)
	now := time.Date(2026, 7, 27, 8, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	for index := 0; index < maxProjectionEntriesPerUser+8; index++ {
		rawURL := "https://cover.example/projected-" + strconv.Itoa(index) + ".png"
		if _, err := service.Project(user.ID, source.ID, rawURL); err != nil {
			t.Fatal(err)
		}
	}
	if got := projectionCountForUser(service, user.ID); got != maxProjectionEntriesPerUser {
		t.Fatalf("projection cache entries=%d, want %d", got, maxProjectionEntriesPerUser)
	}

	now = now.Add(capabilityTTL + time.Second)
	if _, err := service.Project(user.ID, source.ID, "https://cover.example/fresh.png"); err != nil {
		t.Fatal(err)
	}
	if got := projectionCountForUser(service, user.ID); got != 1 {
		t.Fatalf("expired projections were not pruned: entries=%d", got)
	}
}

func projectionCountForUser(service *Service, userID uint) int {
	service.mu.Lock()
	defer service.mu.Unlock()
	count := 0
	for _, entry := range service.projections {
		if entry.userID == userID {
			count++
		}
	}
	return count
}

func TestRemoteCoverCacheEvictionAndUnsafeRoots(t *testing.T) {
	service, user, source := newContractService(t, func(cfg *config.Config) {
		cfg.MaxCoverImageBytes = int64(len(contractPNG))
		cfg.MaxCoverCacheBytes = int64(len(contractPNG) * 2)
	})
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	var requests atomic.Int32
	service.clientFactory = func(policy requestPolicy) *http.Client {
		return &http.Client{
			Transport: contractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        make(http.Header),
					Body:          io.NopCloser(bytes.NewReader(contractPNG)),
					ContentLength: int64(len(contractPNG)),
					Request:       request,
				}, nil
			}),
			CheckRedirect: policy.CheckRedirect,
		}
	}

	capabilities := make([]string, 0, 3)
	for index := range 3 {
		rawURL := "https://cover.example/cover-" + strconv.Itoa(index) + ".png"
		projected, err := service.Project(user.ID, source.ID, rawURL)
		if err != nil {
			t.Fatal(err)
		}
		capability, _ := url.PathUnescape(strings.TrimPrefix(projected, "/api/cover/"))
		if _, err := service.Open(context.Background(), capability); err != nil {
			t.Fatal(err)
		}
		capabilities = append(capabilities, capability)
		now = now.Add(time.Second)
	}
	stats, err := service.StatsUser(user.ID)
	if err != nil || stats.Files != 2 || stats.Bytes != int64(len(contractPNG)*2) {
		t.Fatalf("bounded cache stats=%+v err=%v", stats, err)
	}
	if _, err := service.Open(context.Background(), capabilities[0]); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 4 {
		t.Fatalf("oldest cover was not evicted: requests=%d", requests.Load())
	}

	if _, err := service.RemoveUser(user.ID); err != nil {
		t.Fatal(err)
	}
	cacheRoot, err := service.cacheRoot()
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	userRoot := filepath.Join(cacheRoot, "user-"+strconv.FormatUint(uint64(user.ID), 10))
	if err := os.Symlink(outside, userRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StatsUser(user.ID); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink user root error=%v", err)
	}
	if _, err := service.RemoveUser(user.ID); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink cleanup error=%v", err)
	}
	if info, err := os.Stat(outside); err != nil || !info.IsDir() {
		t.Fatalf("unsafe cleanup touched outside root: info=%v err=%v", info, err)
	}
}

func TestRemoteCoverPolicyRevalidatesDNSAndRedirects(t *testing.T) {
	var lookups atomic.Int32
	policy := requestPolicy{
		MaxRedirects: 3,
		LookupIP: func(context.Context, string) ([]net.IP, error) {
			if lookups.Add(1) == 1 {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			}
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		},
	}
	parsed, _ := url.Parse("https://rebind.example/cover.png")
	if err := policy.validateURL(context.Background(), parsed); err != nil {
		t.Fatalf("first DNS validation failed: %v", err)
	}
	if _, err := policy.allowedDialIPs(context.Background(), parsed.Hostname()); !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("dial did not reject rebound private address: %v", err)
	}
	request := &http.Request{URL: parsed}
	if err := policy.CheckRedirect(request, make([]*http.Request, 4)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("redirect limit error=%v", err)
	}
}

func TestRemoteCoverFetchRejectsUnsafeOrInvalidImages(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		response   []byte
		status     int
		configure  func(*config.Config)
		projectErr error
	}{
		{
			name:       "literal loopback",
			rawURL:     "http://127.0.0.1/metadata",
			projectErr: ErrUnsafeURL,
		},
		{
			name:       "userinfo",
			rawURL:     "https://user:secret@cover.example/art.png",
			projectErr: ErrUnsafeURL,
		},
		{
			name:     "private DNS",
			rawURL:   "https://private.example/art.png",
			response: contractPNG,
			status:   http.StatusOK,
		},
		{
			name:     "non image",
			rawURL:   "https://cover.example/art.png",
			response: []byte("<html>not an image</html>"),
			status:   http.StatusOK,
		},
		{
			name:     "truncated image",
			rawURL:   "https://cover.example/art.png",
			response: contractPNG[:12],
			status:   http.StatusOK,
		},
		{
			name:     "non success",
			rawURL:   "https://cover.example/art.png",
			response: contractPNG,
			status:   http.StatusForbidden,
		},
		{
			name:     "over limit",
			rawURL:   "https://cover.example/art.png",
			response: append(append([]byte(nil), contractPNG...), bytes.Repeat([]byte{0}, 64)...),
			status:   http.StatusOK,
			configure: func(cfg *config.Config) {
				cfg.MaxCoverImageBytes = int64(len(contractPNG))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, user, source := newContractService(t, tt.configure)
			service.clientFactory = func(policy requestPolicy) *http.Client {
				return &http.Client{
					Transport: contractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode:    tt.status,
							Header:        make(http.Header),
							Body:          io.NopCloser(bytes.NewReader(tt.response)),
							ContentLength: int64(len(tt.response)),
							Request:       request,
						}, nil
					}),
					CheckRedirect: policy.CheckRedirect,
				}
			}
			projected, err := service.Project(user.ID, source.ID, tt.rawURL)
			if tt.projectErr != nil {
				if !errors.Is(err, tt.projectErr) {
					t.Fatalf("project error=%v, want %v", err, tt.projectErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			capability, _ := url.PathUnescape(strings.TrimPrefix(projected, "/api/cover/"))
			if _, err := service.Open(context.Background(), capability); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("open error=%v, want unavailable", err)
			}
		})
	}
}
