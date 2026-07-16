package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Address                   string
	DataDir                   string
	CacheDir                  string
	LibraryDir                string
	DatabasePath              string
	JWTSecret                 string
	CORSOrigin                string
	PublicDir                 string
	CheckInterval             string
	LocalStoreDir             string
	RateLimitPerMinute        int
	MaxImportBytes            int64
	MaxArchiveEntries         int
	MaxArchiveEntryBytes      int64
	MaxArchiveExpandedBytes   int64
	MaxPDFPages               int
	MaxParsedTextBytes        int64
	MaxUMDChapters            int
	MaxBackupRestoreBytes     int64
	MaxBackupArchiveEntries   int
	MaxBackupArchiveBytes     int64
	MaxBackupArchiveTotal     int64
	MaxPortableBackupBytes    int64
	MaxPortableArchiveEntries int
	MaxPortableArchiveBytes   int64
	MaxPortableArchiveTotal   int64
}

func Load() Config {
	dataDir := env("OPENREADER_DATA_DIR", "data")
	cacheDir := env("OPENREADER_CACHE_DIR", "cache")

	return Config{
		Address:                   env("OPENREADER_ADDR", ":8080"),
		DataDir:                   dataDir,
		CacheDir:                  cacheDir,
		LibraryDir:                env("OPENREADER_LIBRARY_DIR", "library"),
		DatabasePath:              env("OPENREADER_DB", filepath.Join(dataDir, "openreader.db")),
		JWTSecret:                 env("OPENREADER_JWT_SECRET", "change-this-before-deploy"),
		CORSOrigin:                env("OPENREADER_CORS_ORIGIN", "http://localhost:5173"),
		PublicDir:                 env("OPENREADER_PUBLIC_DIR", "public"),
		CheckInterval:             env("OPENREADER_CHECK_INTERVAL", "30m"),
		LocalStoreDir:             env("OPENREADER_LOCAL_STORE_DIR", filepath.Join("library", "localStore")),
		RateLimitPerMinute:        envInt("OPENREADER_RATE_LIMIT_PER_MINUTE", 6000),
		MaxImportBytes:            envInt64("OPENREADER_MAX_IMPORT_BYTES", 128*1024*1024),
		MaxArchiveEntries:         envPositiveInt("OPENREADER_MAX_ARCHIVE_ENTRIES", 20_000),
		MaxArchiveEntryBytes:      envInt64("OPENREADER_MAX_ARCHIVE_ENTRY_BYTES", 128*1024*1024),
		MaxArchiveExpandedBytes:   envInt64("OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES", 512*1024*1024),
		MaxPDFPages:               envPositiveInt("OPENREADER_MAX_PDF_PAGES", 10_000),
		MaxParsedTextBytes:        envInt64("OPENREADER_MAX_PARSED_TEXT_BYTES", 256*1024*1024),
		MaxUMDChapters:            envPositiveInt("OPENREADER_MAX_UMD_CHAPTERS", 100_000),
		MaxBackupRestoreBytes:     envInt64("OPENREADER_MAX_BACKUP_RESTORE_BYTES", 128*1024*1024),
		MaxBackupArchiveEntries:   envPositiveInt("OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES", 5_000),
		MaxBackupArchiveBytes:     envInt64("OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES", 16*1024*1024),
		MaxBackupArchiveTotal:     envInt64("OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES", 128*1024*1024),
		MaxPortableBackupBytes:    envInt64("OPENREADER_MAX_PORTABLE_BACKUP_BYTES", 512*1024*1024),
		MaxPortableArchiveEntries: envPositiveInt("OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES", 10_000),
		MaxPortableArchiveBytes:   envInt64("OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES", 256*1024*1024),
		MaxPortableArchiveTotal:   envInt64("OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES", 512*1024*1024),
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

func envPositiveInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
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
