package chapterimage

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "chapter-images.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.BookSource{}, &models.Book{}, &models.Chapter{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		JWTSecret: "chapter-image-contract-secret",
	}
	service := New(cfg, database)
	service.limits = Limits{
		MaxImages:     64,
		MaxImageBytes: 8 * 1024 * 1024,
		MaxTotalBytes: 32 * 1024 * 1024,
		Timeout:       2 * time.Second,
		MaxRedirects:  3,
	}
	return service, database
}

func createImageFixture(t *testing.T, database *gorm.DB, baseURL string) (models.User, models.BookSource, models.Book, models.Chapter) {
	t.Helper()
	user := models.User{Username: "imageowner", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{
		Name:    "图片缓存源",
		BaseURL: baseURL,
		Header:  `{"Referer":"https://reader.example/chapter","Cookie":"sid=source-secret","Authorization":"Bearer source-secret","X-Source":"kept"}`,
		Enabled: true,
	}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "图片书", URL: baseURL + "/book"}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: baseURL + "/chapter/1"}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return user, source, book, chapter
}

func TestExtractImageReferencesNormalizesAndDeduplicatesInOrder(t *testing.T) {
	content := strings.Join([]string{
		`正文<img src="../images/a.png" alt="A" data-image-style="FULL"><img data-src="/images/b">`,
		`<img src="https://cdn.example/c.webp?token=hidden"><img src="../images/a.png">`,
		`<img src="javascript:alert(1)">`,
	}, "\n")
	references := extractImageReferences(content, "https://books.example/novel/chapter/1", 64)
	want := []string{
		"https://books.example/novel/images/a.png",
		"https://books.example/images/b",
		"https://cdn.example/c.webp?token=hidden",
	}
	if len(references) != len(want) {
		t.Fatalf("references = %+v, want %v", references, want)
	}
	for index, value := range want {
		if references[index].URL != value || references[index].Key != imageKey(value) {
			t.Fatalf("reference %d = %+v, want URL=%q", index, references[index], value)
		}
	}
}

func TestCacheChapterWithoutImagesDoesNotCreateEmptyDerivedFiles(t *testing.T) {
	service, database := testService(t)
	_, source, book, chapter := createImageFixture(t, database, "https://source.example")
	result, err := service.CacheChapter(context.Background(), source, book, chapter, "只有正文，没有图片。")
	if err != nil || result.Found != 0 {
		t.Fatalf("plain chapter result=%+v err=%v", result, err)
	}
	stats, err := service.StatsUser(book.UserID)
	if err != nil || stats.Files != 0 || stats.Bytes != 0 {
		t.Fatalf("plain chapter created derived files: stats=%+v err=%v", stats, err)
	}
}

func TestUncachedReadPathsDoNotCreateEmptyBookRoots(t *testing.T) {
	service, database := testService(t)
	_, _, book, chapter := createImageFixture(t, database, "https://source.example")
	root := filepath.Join(
		service.cfg.CacheDir,
		"chapter-images",
		"user-"+strconv.FormatUint(uint64(book.UserID), 10),
		"book-"+strconv.FormatUint(uint64(book.ID), 10),
	)
	mapping, _, err := service.CachedImages(book, chapter, `<img src="https://source.example/image">`)
	if err != nil || len(mapping) != 0 {
		t.Fatalf("uncached mapping=%v err=%v", mapping, err)
	}
	files, err := service.CachedFiles(book, chapter, `<img src="https://source.example/image">`)
	if err != nil || len(files) != 0 {
		t.Fatalf("uncached files=%v err=%v", files, err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only cache lookup created book root %s: %v", root, err)
	}
}

func TestCacheChapterUsesSameOriginHeadersDeduplicatesAndIssuesCapability(t *testing.T) {
	png := testPNG(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("Cookie") != "sid=source-secret" ||
			request.Header.Get("Authorization") != "Bearer source-secret" ||
			request.Header.Get("X-Source") != "kept" ||
			request.Header.Get("Referer") != "https://reader.example/chapter" {
			t.Errorf("same-origin source headers were not preserved: %v", request.Header)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer server.Close()

	service, database := testService(t)
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	content := `<img src="` + server.URL + `/no-extension?secret=one"><img src="` + server.URL + `/no-extension?secret=one">`
	result, err := service.CacheChapter(context.Background(), source, book, chapter, content)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found != 1 || result.Downloaded != 1 || result.Failed != 0 || requests.Load() != 1 {
		t.Fatalf("cache result=%+v requests=%d", result, requests.Load())
	}

	mapping, expiresAt, err := service.CachedImages(book, chapter, content)
	if err != nil {
		t.Fatal(err)
	}
	remoteURL := server.URL + "/no-extension?secret=one"
	resourceURL := mapping[remoteURL]
	if !strings.HasPrefix(resourceURL, "/api/chapter-image/") || !expiresAt.After(time.Now()) {
		t.Fatalf("mapping=%+v expiresAt=%v", mapping, expiresAt)
	}
	if strings.Contains(resourceURL, "secret=one") || strings.Contains(resourceURL, "source-secret") {
		t.Fatalf("capability leaked source URL or credentials: %q", resourceURL)
	}
	token, err := url.PathUnescape(strings.TrimPrefix(resourceURL, "/api/chapter-image/"))
	if err != nil {
		t.Fatal(err)
	}
	resource, err := service.OpenResource(token)
	if err != nil {
		t.Fatal(err)
	}
	if resource.ContentType != "image/png" {
		t.Fatalf("resource content type = %q", resource.ContentType)
	}
	data, err := os.ReadFile(resource.Path)
	if err != nil || string(data) != string(png) {
		t.Fatalf("resource bytes changed: len=%d err=%v", len(data), err)
	}
	root := filepath.Dir(filepath.Dir(resource.Path))
	if _, err := service.RemoveBook(book); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenResource(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed capability err=%v, want ErrNotFound", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale capability recreated removed root %s: %v", root, err)
	}
}

func TestCrossOriginHeadersAndPrivateTargetsFailClosed(t *testing.T) {
	service, database := testService(t)
	_, source, book, chapter := createImageFixture(t, database, "https://source.example")
	png := testPNG(t)
	var requestHeaders http.Header
	service.lookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "cdn.example", "redirect.example":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "metadata.example":
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		default:
			return nil, errors.New("unexpected host")
		}
	}
	service.clientFactory = func(policy requestPolicy) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requestHeaders = request.Header.Clone()
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/png"}},
					Body:       io.NopCloser(strings.NewReader(string(png))),
					Request:    request,
				}, nil
			}),
			CheckRedirect: policy.CheckRedirect,
		}
	}

	result, err := service.CacheChapter(context.Background(), source, book, chapter, `<img src="https://cdn.example/image">`)
	if err != nil || result.Downloaded != 1 {
		t.Fatalf("public cross-origin image failed: result=%+v err=%v", result, err)
	}
	if requestHeaders.Get("Cookie") != "" || requestHeaders.Get("Authorization") != "" || requestHeaders.Get("X-Source") != "" {
		t.Fatalf("cross-origin credentials/custom headers leaked: %v", requestHeaders)
	}
	if requestHeaders.Get("Referer") == "" {
		t.Fatalf("cross-origin image lost safe Referer header: %v", requestHeaders)
	}

	requestHeaders = nil
	result, err = service.CacheChapter(context.Background(), source, book, chapter, `<img src="https://source.example:8443/other-port">`)
	if err != nil || result.Downloaded != 1 {
		t.Fatalf("same-host different-port image failed: result=%+v err=%v", result, err)
	}
	if requestHeaders.Get("Cookie") != "" || requestHeaders.Get("Authorization") != "" || requestHeaders.Get("X-Source") != "" {
		t.Fatalf("source credentials leaked across origins on the same hostname: %v", requestHeaders)
	}
	if requestHeaders.Get("Referer") != "https://reader.example/" {
		t.Fatalf("cross-origin Referer leaked chapter path/query: %q", requestHeaders.Get("Referer"))
	}

	called := false
	service.clientFactory = func(policy requestPolicy) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("must not request private target")
		}), CheckRedirect: policy.CheckRedirect}
	}
	result, err = service.CacheChapter(context.Background(), source, book, chapter, `<img src="https://metadata.example/latest/meta-data">`)
	if err != nil || result.Failed != 1 || called {
		t.Fatalf("private cross-origin target was not rejected before fetch: result=%+v called=%v err=%v", result, called, err)
	}
}

func TestCacheChapterRejectsOversizeAndNonImageWithoutPartialFiles(t *testing.T) {
	service, database := testService(t)
	service.limits.MaxImageBytes = 16
	service.limits.MaxTotalBytes = 16
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/html":
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "<html>not an image</html>")
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, 17))
		}
	}))
	defer server.Close()
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	content := `<img src="` + server.URL + `/oversize"><img src="` + server.URL + `/html">`
	result, err := service.CacheChapter(context.Background(), source, book, chapter, content)
	if err != nil || result.Failed != 2 || result.Downloaded != 0 {
		t.Fatalf("invalid image result=%+v err=%v", result, err)
	}
	bookRoot, err := service.bookRoot(book)
	if err != nil {
		t.Fatal(err)
	}
	_ = filepath.WalkDir(bookRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr == nil && entry != nil && !entry.IsDir() && (strings.Contains(entry.Name(), ".tmp") || strings.Contains(path, string(filepath.Separator)+"blobs"+string(filepath.Separator))) {
			t.Errorf("rejected image left a partial blob: %s", path)
		}
		return nil
	})
}

func TestCacheChapterRefreshFailureKeepsOldReferencesUntilSuccessfulReplacement(t *testing.T) {
	png := testPNG(t)
	newImageFails := atomic.Bool{}
	newImageFails.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/new" && newImageFails.Load() {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer server.Close()
	service, database := testService(t)
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	oldURL := server.URL + "/old"
	newURL := server.URL + "/new"
	oldContent := `<img src="` + oldURL + `">`
	newContent := `<img src="` + newURL + `">`
	if _, err := service.CacheChapter(context.Background(), source, book, chapter, oldContent); err != nil {
		t.Fatal(err)
	}

	result, err := service.CacheChapter(context.Background(), source, book, chapter, newContent)
	if err != nil || result.Failed != 1 {
		t.Fatalf("failed refresh result=%+v err=%v", result, err)
	}
	oldMapping, _, err := service.CachedImages(book, chapter, oldContent)
	if err != nil || oldMapping[oldURL] == "" {
		t.Fatalf("failed refresh discarded old image: mapping=%v err=%v", oldMapping, err)
	}
	newMapping, _, err := service.CachedImages(book, chapter, newContent)
	if err != nil || len(newMapping) != 0 {
		t.Fatalf("failed refresh published partial replacement: mapping=%v err=%v", newMapping, err)
	}

	newImageFails.Store(false)
	result, err = service.CacheChapter(context.Background(), source, book, chapter, newContent)
	if err != nil || result.Downloaded != 1 || result.Failed != 0 {
		t.Fatalf("successful replacement result=%+v err=%v", result, err)
	}
	oldMapping, _, _ = service.CachedImages(book, chapter, oldContent)
	newMapping, _, err = service.CachedImages(book, chapter, newContent)
	if len(oldMapping) != 0 || err != nil || newMapping[newURL] == "" {
		t.Fatalf("successful refresh did not replace references: old=%v new=%v err=%v", oldMapping, newMapping, err)
	}
}

func TestImageTypeAllowlistAndLimits(t *testing.T) {
	cases := map[string][]byte{
		"image/jpeg": {0xff, 0xd8, 0xff},
		"image/png":  []byte("\x89PNG\r\n\x1a\n"),
		"image/gif":  []byte("GIF89a"),
		"image/webp": []byte("RIFF1234WEBP"),
		"image/bmp":  []byte("BM"),
		"image/avif": append([]byte{0, 0, 0, 20}, []byte("ftypavif0000")...),
	}
	for want, data := range cases {
		if got, ok := detectImageType(data); !ok || got != want {
			t.Fatalf("detectImageType(%q)=(%q,%v), want %q", data, got, ok, want)
		}
	}
	for _, data := range [][]byte{nil, []byte("<svg></svg>"), []byte("<html></html>")} {
		if got, ok := detectImageType(data); ok || got != "" {
			t.Fatalf("unsafe image bytes accepted as %q", got)
		}
	}

	service, database := testService(t)
	service.limits.MaxImages = 2
	service.limits.MaxImageBytes = 8
	service.limits.MaxTotalBytes = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer server.Close()
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	content := `<img src="` + server.URL + `/one"><img src="` + server.URL + `/two"><img src="` + server.URL + `/three">`
	result, err := service.CacheChapter(context.Background(), source, book, chapter, content)
	if err != nil || result.Found != 2 || result.Downloaded != 1 || result.Failed != 1 {
		t.Fatalf("count/total image limits result=%+v err=%v", result, err)
	}
}

func TestImageFetchTimeoutAndRedirectLimitAreBestEffortFailures(t *testing.T) {
	service, database := testService(t)
	service.limits.Timeout = 30 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/slow" {
			select {
			case <-time.After(200 * time.Millisecond):
				_, _ = w.Write(testPNG(t))
			case <-request.Context().Done():
			}
			return
		}
		step, _ := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/redirect-"))
		if step < 4 {
			http.Redirect(w, request, "/redirect-"+strconv.Itoa(step+1), http.StatusFound)
			return
		}
		_, _ = w.Write(testPNG(t))
	}))
	defer server.Close()
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	content := `<img src="` + server.URL + `/slow"><img src="` + server.URL + `/redirect-0">`
	result, err := service.CacheChapter(context.Background(), source, book, chapter, content)
	if err != nil || result.Failed != 2 || result.Downloaded != 0 {
		t.Fatalf("timeout/redirect result=%+v err=%v", result, err)
	}
}

func TestCacheChapterRejectsExistingBlobThatExceedsCurrentLimit(t *testing.T) {
	service, database := testService(t)
	service.limits.MaxImageBytes = 16
	service.limits.MaxTotalBytes = 16
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(t)[:8])
	}))
	defer server.Close()
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	remoteURL := server.URL + "/existing"
	root, err := service.bookRoot(book)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeBlobAtomic(root, imageKey(remoteURL), testPNG(t)); err != nil {
		t.Fatal(err)
	}

	result, err := service.CacheChapter(context.Background(), source, book, chapter, `<img src="`+remoteURL+`">`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Downloaded != 0 || result.Reused != 0 {
		t.Fatalf("oversized existing blob result=%+v", result)
	}
	if requests.Load() != 0 {
		t.Fatalf("oversized existing blob was fetched again: requests=%d", requests.Load())
	}
	if mapping, _, err := service.CachedImages(book, chapter, `<img src="`+remoteURL+`">`); err != nil || len(mapping) != 0 {
		t.Fatalf("oversized existing blob was published: mapping=%v err=%v", mapping, err)
	}
}

func TestPruneUnreferencedBlobsFailsClosedForMalformedManifest(t *testing.T) {
	service, database := testService(t)
	_, _, book, chapter := createImageFixture(t, database, "https://source.example")
	root, err := service.bookRoot(book)
	if err != nil {
		t.Fatal(err)
	}
	key := imageKey("https://source.example/image.png")
	blob, err := writeBlobAtomic(root, key, testPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestPath(root, chapter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"version":1,"chapterId":`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := pruneUnreferencedBlobs(root); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("malformed manifest prune err=%v, want unsafe path", err)
	}
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("fail-closed prune removed blob: %v", err)
	}
}

func TestCapabilityRechecksFingerprintOwnershipAndReferenceLifecycle(t *testing.T) {
	png := testPNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer server.Close()
	service, database := testService(t)
	user, source, book, chapterA := createImageFixture(t, database, server.URL)
	chapterB := models.Chapter{BookID: book.ID, Index: 1, Title: "第二章", URL: server.URL + "/chapter/2"}
	if err := database.Create(&chapterB).Error; err != nil {
		t.Fatal(err)
	}
	content := `<img src="` + server.URL + `/shared">`
	if _, err := service.CacheChapter(context.Background(), source, book, chapterA, content); err != nil {
		t.Fatal(err)
	}
	if result, err := service.CacheChapter(context.Background(), source, book, chapterB, content); err != nil || result.Reused != 1 {
		t.Fatalf("second chapter did not reuse image: result=%+v err=%v", result, err)
	}
	mapping, _, err := service.CachedImages(book, chapterA, content)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := url.PathUnescape(strings.TrimPrefix(mapping[server.URL+"/shared"], "/api/chapter-image/"))
	resource, err := service.OpenResource(token)
	if err != nil {
		t.Fatal(err)
	}

	if stats, err := service.RemoveChapterReferences(book, []uint{chapterA.ID}); err != nil || stats.Files != 1 {
		t.Fatalf("first reference removal must remove only its manifest: stats=%+v err=%v", stats, err)
	}
	if _, err := service.OpenResource(token); err != nil {
		t.Fatalf("shared blob disappeared while chapter B still references it: %v", err)
	}
	if stats, err := service.RemoveChapterReferences(book, []uint{chapterB.ID}); err != nil || stats.Files < 2 {
		t.Fatalf("last reference removal must remove manifest and blob: stats=%+v err=%v", stats, err)
	}
	if _, err := service.OpenResource(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed blob capability err=%v, want not found", err)
	}

	if _, err := service.CacheChapter(context.Background(), source, book, chapterA, content); err != nil {
		t.Fatal(err)
	}
	mapping, _, _ = service.CachedImages(book, chapterA, content)
	token, _ = url.PathUnescape(strings.TrimPrefix(mapping[server.URL+"/shared"], "/api/chapter-image/"))
	resource, err = service.OpenResource(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resource.Path, append(png, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenResource(token); !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("mutated blob capability err=%v, want invalid capability", err)
	}
	if err := database.Model(&models.Book{}).Where("id = ?", book.ID).Update("user_id", user.ID+100).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenResource(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ownership-changed capability err=%v, want not found", err)
	}
}

func TestCacheChapterCancellationStopsImageWorkAndDoesNotPublishManifest(t *testing.T) {
	service, database := testService(t)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	_, source, book, chapter := createImageFixture(t, database, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.CacheChapter(ctx, source, book, chapter, `<img src="`+server.URL+`/slow"><img src="`+server.URL+`/later">`)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled image cache err=%v", err)
	}
	if mapping, _, err := service.CachedImages(book, chapter, `<img src="`+server.URL+`/slow">`); err != nil || len(mapping) != 0 {
		t.Fatalf("cancelled image cache published references: mapping=%v err=%v", mapping, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
