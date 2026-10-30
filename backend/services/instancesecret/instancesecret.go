// Package instancesecret keeps the signing secret of an instance whose
// OPENREADER_JWT_SECRET is blank or a value published in this repository.
//
// Such a value signs nothing. The first start creates a random secret in the
// data directory instead, sessions and resource capabilities are signed with
// it from then on, and it moves with the data directory.
package instancesecret

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"openreader/backend/config"
)

// FileName is the instance secret's name inside the data directory.
const FileName = "instance-secret"

const (
	currentDefault  = "change-this-before-deploy"
	previousDefault = "change-me-in-production"
	secretBytes     = 32
	// maxFileBytes bounds what is read from an unexpected file.
	maxFileBytes = 1024
)

// Published reports whether secret is blank or one of the defaults published
// in this repository: the fallback of config.Load and that of older releases.
func Published(secret string) bool {
	switch strings.TrimSpace(secret) {
	case "", currentDefault, previousDefault:
		return true
	default:
		return false
	}
}

// Apply returns cfg with the secret that signs sessions and resource
// capabilities. A custom cfg.JWTSecret is returned unchanged and no file is
// read or written. A published one is replaced by the instance secret in
// cfg.DataDir, created on first use, and kept in cfg.JWTPreviousSecrets so
// that jti-backed sessions it signed before stay valid. Errors name the file,
// never its content.
func Apply(cfg config.Config) (config.Config, error) {
	if !Published(cfg.JWTSecret) {
		return cfg, nil
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return cfg, errors.New("instance secret: no data directory is configured")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return cfg, fmt.Errorf("instance secret: create data directory: %w", err)
	}
	path := filepath.Join(cfg.DataDir, FileName)
	secret, err := read(path)
	created := false
	if errors.Is(err, fs.ErrNotExist) {
		secret, err = create(cfg.DataDir, path)
		created = err == nil
		if errors.Is(err, fs.ErrExist) {
			// Another process published one first; use that.
			secret, err = read(path)
		}
	}
	if err != nil {
		return cfg, err
	}
	if created {
		log.Printf("OPENREADER_JWT_SECRET is unset or a published default; created a random instance secret in %s", path)
	} else {
		log.Printf("OPENREADER_JWT_SECRET is unset or a published default; signing with the instance secret in %s", path)
	}
	cfg.JWTPreviousSecrets = []string{cfg.JWTSecret}
	cfg.JWTSecret = secret
	return cfg, nil
}

// read returns the secret stored at path. Anything but a regular file holding
// 64 hexadecimal characters, optionally followed by one line ending, fails
// closed. Permissions wider than 0600 are narrowed.
func read(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("instance secret %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("instance secret %s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("instance secret %s: %w", path, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("instance secret %s: %w", path, err)
	}
	if !os.SameFile(info, opened) {
		return "", fmt.Errorf("instance secret %s changed while it was opened", path)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("instance secret %s: %w", path, err)
	}
	secret, ok := parse(content)
	if !ok {
		return "", fmt.Errorf("instance secret %s is not %d hexadecimal characters: restore it from a copy of the data directory, "+
			"set OPENREADER_JWT_SECRET, or move it away so a new one is created (every device then signs in again)", path, secretBytes*2)
	}
	if opened.Mode().Perm()&0o077 != 0 {
		if err := file.Chmod(0o600); err != nil {
			log.Printf("instance secret %s is readable by other users and could not be restricted to 0600: %v", path, err)
		} else {
			log.Printf("restricted instance secret %s to mode 0600", path)
		}
	}
	return secret, nil
}

func parse(content []byte) (string, bool) {
	text := string(content)
	if trimmed, ok := strings.CutSuffix(text, "\r\n"); ok {
		text = trimmed
	} else {
		text = strings.TrimSuffix(text, "\n")
	}
	if len(text) != secretBytes*2 {
		return "", false
	}
	if _, err := hex.DecodeString(text); err != nil {
		return "", false
	}
	return text, true
}

// create publishes a new secret at path without ever replacing a file there:
// a private staging file is written and synced first and then linked to the
// final name, so path is either absent or complete. It returns an error
// matching fs.ErrExist when another process published first.
func create(directory, path string) (string, error) {
	random := make([]byte, secretBytes)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return "", fmt.Errorf("instance secret: generate: %w", err)
	}
	secret := hex.EncodeToString(random)

	stage, err := os.CreateTemp(directory, ".instance-secret-*.tmp")
	if err != nil {
		return "", fmt.Errorf("instance secret: create staging file in %s: %w", directory, err)
	}
	defer os.Remove(stage.Name())
	if _, err := io.WriteString(stage, secret+"\n"); err != nil {
		_ = stage.Close()
		return "", fmt.Errorf("instance secret: write %s: %w", stage.Name(), err)
	}
	if err := stage.Sync(); err != nil {
		_ = stage.Close()
		return "", fmt.Errorf("instance secret: sync %s: %w", stage.Name(), err)
	}
	if err := stage.Close(); err != nil {
		return "", fmt.Errorf("instance secret: close %s: %w", stage.Name(), err)
	}

	if err := os.Link(stage.Name(), path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", err
		}
		// A filesystem without hard links gets a rename instead, which would
		// replace a file published meanwhile; concurrent first starts on such
		// a filesystem are not supported.
		if _, statErr := os.Lstat(path); statErr == nil {
			return "", fmt.Errorf("instance secret %s: %w", path, fs.ErrExist)
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return "", fmt.Errorf("instance secret %s: %w", path, statErr)
		}
		if err := os.Rename(stage.Name(), path); err != nil {
			return "", fmt.Errorf("instance secret: publish %s: %w", path, err)
		}
	}
	syncDirectory(directory)
	return secret, nil
}

// syncDirectory makes the new name durable where the platform allows it.
// Losing it in a crash costs only a new secret and one more sign-in.
func syncDirectory(path string) {
	directory, err := os.Open(path)
	if err != nil {
		return
	}
	_ = directory.Sync()
	_ = directory.Close()
}
