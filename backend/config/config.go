package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Address            string
	DataDir            string
	CacheDir           string
	LibraryDir         string
	DatabasePath       string
	JWTSecret          string
	CORSOrigin         string
	PublicDir          string
	CheckInterval      string
	LocalStoreDir      string
	RateLimitPerMinute int
	MaxImportBytes     int64
}

func Load() Config {
	dataDir := env("OPENREADER_DATA_DIR", "data")
	cacheDir := env("OPENREADER_CACHE_DIR", "cache")

	return Config{
		Address:            env("OPENREADER_ADDR", ":8080"),
		DataDir:            dataDir,
		CacheDir:           cacheDir,
		LibraryDir:         env("OPENREADER_LIBRARY_DIR", "library"),
		DatabasePath:       env("OPENREADER_DB", filepath.Join(dataDir, "openreader.db")),
		JWTSecret:          env("OPENREADER_JWT_SECRET", "change-this-before-deploy"),
		CORSOrigin:         env("OPENREADER_CORS_ORIGIN", "http://localhost:5173"),
		PublicDir:          env("OPENREADER_PUBLIC_DIR", "public"),
		CheckInterval:      env("OPENREADER_CHECK_INTERVAL", "30m"),
		LocalStoreDir:      env("OPENREADER_LOCAL_STORE_DIR", filepath.Join("library", "localStore")),
		RateLimitPerMinute: envInt("OPENREADER_RATE_LIMIT_PER_MINUTE", 6000),
		MaxImportBytes:     envInt64("OPENREADER_MAX_IMPORT_BYTES", 128*1024*1024),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
