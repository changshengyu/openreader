package instancesecret

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"

	"openreader/backend/config"
)

var createdFormat = regexp.MustCompile(`^[0-9a-f]{64}\n$`)

func randomHex(t *testing.T) string {
	t.Helper()
	random := make([]byte, secretBytes)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(random)
}

func requireNoStage(t *testing.T, directory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".instance-secret-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("staging files remain: %v", matches)
	}
}

func TestPublished(t *testing.T) {
	for secret, want := range map[string]bool{
		"":                              true,
		" \t\n":                         true,
		"change-this-before-deploy":     true,
		" change-this-before-deploy\n":  true,
		"change-me-in-production":       true,
		"change-this-before-deploy-now": false,
		"CHANGE-ME-IN-PRODUCTION":       false,
		"a-custom-secret":               false,
	} {
		if got := Published(secret); got != want {
			t.Errorf("Published(%q) = %v, want %v", secret, got, want)
		}
	}
}

func TestApplyKeepsCustomSecretAndTouchesNothing(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, FileName)
	// Even an unusable file is never read under a custom secret.
	if err := os.WriteFile(path, []byte("not a secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: directory, JWTSecret: "a-custom-secret"}
	applied, err := Apply(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(applied, cfg) {
		t.Fatalf("custom secret changed the config: %+v", applied)
	}
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("custom secret touched the instance secret file")
	}

	empty := t.TempDir()
	if _, err := Apply(config.Config{DataDir: empty, JWTSecret: "a-custom-secret"}); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(empty); err != nil || len(entries) != 0 {
		t.Fatalf("custom secret created files: %v %v", entries, err)
	}
}

func TestApplyCreatesOnePrivateInstanceSecret(t *testing.T) {
	for name, configured := range map[string]string{
		"current-default":  "change-this-before-deploy",
		"previous-default": "change-me-in-production",
		"empty":            "",
		"whitespace":       "  ",
	} {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "data")
			applied, err := Apply(config.Config{DataDir: directory, JWTSecret: configured})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, FileName)
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
				t.Fatalf("instance secret mode = %v, want a 0600 regular file", info.Mode())
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !createdFormat.Match(content) {
				t.Fatalf("instance secret is not 64 lowercase hex characters and a newline (%d bytes)", len(content))
			}
			if applied.JWTSecret != strings.TrimSuffix(string(content), "\n") {
				t.Fatal("config does not sign with the stored instance secret")
			}
			if !reflect.DeepEqual(applied.JWTPreviousSecrets, []string{configured}) {
				t.Fatalf("previous secrets = %q, want the configured value", applied.JWTPreviousSecrets)
			}
			requireNoStage(t, directory)

			again, err := Apply(config.Config{DataDir: directory, JWTSecret: configured})
			if err != nil {
				t.Fatal(err)
			}
			if again.JWTSecret != applied.JWTSecret {
				t.Fatal("a second start replaced the instance secret")
			}
			if later, err := os.Lstat(path); err != nil || !os.SameFile(info, later) || !later.ModTime().Equal(info.ModTime()) {
				t.Fatalf("a second start rewrote the instance secret: %v", err)
			}
			if Published(applied.JWTSecret) {
				t.Fatal("the instance secret counts as published")
			}
		})
	}
}

func TestApplyConcurrentFirstStartsShareOneSecret(t *testing.T) {
	directory := t.TempDir()
	const starts = 16
	secrets := make([]string, starts)
	errs := make([]error, starts)
	var ready, done sync.WaitGroup
	ready.Add(1)
	for index := range starts {
		done.Add(1)
		go func() {
			defer done.Done()
			ready.Wait()
			applied, err := Apply(config.Config{DataDir: directory, JWTSecret: "change-this-before-deploy"})
			secrets[index], errs[index] = applied.JWTSecret, err
		}()
	}
	ready.Done()
	done.Wait()
	for index := range starts {
		if errs[index] != nil {
			t.Fatalf("start %d: %v", index, errs[index])
		}
		if secrets[index] != secrets[0] {
			t.Fatalf("start %d signs with a different secret", index)
		}
	}
	content, err := os.ReadFile(filepath.Join(directory, FileName))
	if err != nil || strings.TrimSpace(string(content)) != secrets[0] {
		t.Fatalf("stored secret differs from the one in use: %v", err)
	}
	requireNoStage(t, directory)
}

func TestApplyRefusesUnusableFilesWithoutChangingThem(t *testing.T) {
	valid := randomHex(t)
	for name, content := range map[string]string{
		"empty":           "",
		"short":           "abc\n",
		"63-characters":   valid[:63] + "\n",
		"65-characters":   valid + "0\n",
		"not-hexadecimal": strings.Repeat("zz", secretBytes) + "\n",
		"two-lines":       valid + "\n" + valid + "\n",
		"blank-line":      valid + "\n\n",
		"padded":          " " + valid + "\n",
		"oversized":       strings.Repeat("a", 4096),
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, FileName)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{DataDir: directory, JWTSecret: "change-this-before-deploy"}
			applied, err := Apply(cfg)
			if err == nil {
				t.Fatal("an unusable instance secret was accepted")
			}
			if content != "" && strings.Contains(err.Error(), strings.TrimSpace(content)) {
				t.Fatal("the error carries the file content")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the error does not name the file: %v", err)
			}
			if !reflect.DeepEqual(applied, cfg) {
				t.Fatal("a failed start changed the config")
			}
			if current, err := os.ReadFile(path); err != nil || !bytes.Equal(current, []byte(content)) {
				t.Fatalf("a failed start changed the file: %v", err)
			}
			requireNoStage(t, directory)
		})
	}

	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(t.TempDir(), "elsewhere")
		if err := os.WriteFile(target, []byte(valid+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(directory, FileName)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := Apply(config.Config{DataDir: directory, JWTSecret: ""}); err == nil {
			t.Fatal("a symlinked instance secret was followed")
		}
	})

	t.Run("directory", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Mkdir(filepath.Join(directory, FileName), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := Apply(config.Config{DataDir: directory, JWTSecret: ""}); err == nil {
			t.Fatal("a directory was accepted as the instance secret")
		}
	})

	t.Run("fifo", func(t *testing.T) {
		directory := t.TempDir()
		if err := syscall.Mkfifo(filepath.Join(directory, FileName), 0o600); err != nil {
			t.Skipf("fifo unavailable: %v", err)
		}
		// Refused before any open, so this cannot block on a missing writer.
		if _, err := Apply(config.Config{DataDir: directory, JWTSecret: ""}); err == nil {
			t.Fatal("a FIFO was accepted as the instance secret")
		}
	})

	t.Run("no-data-directory", func(t *testing.T) {
		if _, err := Apply(config.Config{JWTSecret: ""}); err == nil {
			t.Fatal("an instance secret was prepared without a data directory")
		}
	})
}

func TestApplyAcceptsUppercaseAndCRLFAndNarrowsPermissions(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, FileName)
	stored := strings.ToUpper(randomHex(t))
	if err := os.WriteFile(path, []byte(stored+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := Apply(config.Config{DataDir: directory, JWTSecret: "change-me-in-production"})
	if err != nil {
		t.Fatal(err)
	}
	if applied.JWTSecret != stored {
		t.Fatal("the stored secret was not used verbatim")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != stored+"\r\n" {
		t.Fatalf("narrowing permissions changed the content: %v", err)
	}
}
