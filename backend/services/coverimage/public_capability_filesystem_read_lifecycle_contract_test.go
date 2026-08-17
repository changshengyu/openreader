package coverimage

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachedCoverDoesNotTouchReplacementAfterVerification(t *testing.T) {
	service, user, source := newContractService(t, nil)
	fixedNow := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	rawURL := "https://cover.example/replacement.png"
	resourceURL, err := service.Project(user.ID, source.ID, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := url.PathUnescape(strings.TrimPrefix(resourceURL, "/api/cover/"))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.writeCached(user.ID, rawURL, contractPNG); err != nil {
		t.Fatal(err)
	}
	cachePath, err := service.cachePath(user.ID, rawURL, false)
	if err != nil {
		t.Fatal(err)
	}

	replacement := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(replacement, contractPNG, 0o600); err != nil {
		t.Fatal(err)
	}
	originalTime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(replacement, originalTime, originalTime); err != nil {
		t.Fatal(err)
	}

	reached := make(chan struct{})
	release := make(chan struct{})
	var nowCalls atomic.Int32
	service.now = func() time.Time {
		if nowCalls.Add(1) == 2 {
			close(reached)
			<-release
		}
		return fixedNow.Add(time.Minute)
	}
	done := make(chan error, 1)
	go func() {
		_, openErr := service.Open(context.Background(), capability)
		done <- openErr
	}()

	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		close(release)
		<-done
		t.Fatal("cached cover read did not reach the post-verification touch boundary")
	}
	backup := cachePath + ".verified-open"
	if err := os.Rename(cachePath, backup); err != nil {
		close(release)
		<-done
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, cachePath); err != nil {
		_ = os.Rename(backup, cachePath)
		close(release)
		<-done
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("cached cover open failed before identity assertion: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(cachePath)
		_ = os.Rename(backup, cachePath)
	})

	info, err := os.Stat(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(originalTime) {
		t.Fatalf("cached cover touched the replacement mounted object: got %s want %s", info.ModTime(), originalTime)
	}
}
