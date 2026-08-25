package config

import "testing"

func TestLoadTrustedProxiesPreservesExplicitValueAndDefaultsEmpty(t *testing.T) {
	t.Setenv("OPENREADER_TRUSTED_PROXIES", " 127.0.0.1/32 , ::1 ")
	if got := Load().TrustedProxies; got != " 127.0.0.1/32 , ::1 " {
		t.Fatalf("TrustedProxies = %q, want administrator value unchanged", got)
	}

	t.Setenv("OPENREADER_TRUSTED_PROXIES", "")
	if got := Load().TrustedProxies; got != "" {
		t.Fatalf("default TrustedProxies = %q, want empty", got)
	}
}

func TestLoadMaxImportBytesUsesPositiveEnvironmentValue(t *testing.T) {
	t.Setenv("OPENREADER_MAX_IMPORT_BYTES", "4096")
	if got := Load().MaxImportBytes; got != 4096 {
		t.Fatalf("MaxImportBytes = %d, want 4096", got)
	}
}

func TestLoadMaxImportBytesFallsBackForInvalidValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-a-number"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("OPENREADER_MAX_IMPORT_BYTES", value)
			if got := Load().MaxImportBytes; got != 128*1024*1024 {
				t.Fatalf("MaxImportBytes = %d, want default for %q", got, value)
			}
		})
	}
}

func TestLoadSourceFetchLimitsUsesConfiguredValuesAndSafeDefaults(t *testing.T) {
	t.Setenv("OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS", "7")
	t.Setenv("OPENREADER_MAX_SOURCE_RESPONSE_BYTES", "2048")
	t.Setenv("OPENREADER_MAX_SOURCE_REDIRECTS", "2")
	t.Setenv("OPENREADER_MAX_SOURCE_RETRIES", "1")

	configured := Load()
	if configured.SourceRequestTimeoutSeconds != 7 || configured.MaxSourceResponseBytes != 2048 ||
		configured.MaxSourceRedirects != 2 || configured.MaxSourceRetries != 1 {
		t.Fatalf("configured source fetch limits = %+v", configured)
	}

	t.Setenv("OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS", "0")
	t.Setenv("OPENREADER_MAX_SOURCE_RESPONSE_BYTES", "invalid")
	t.Setenv("OPENREADER_MAX_SOURCE_REDIRECTS", "-1")
	t.Setenv("OPENREADER_MAX_SOURCE_RETRIES", "0")

	defaults := Load()
	if defaults.SourceRequestTimeoutSeconds != 15 || defaults.MaxSourceResponseBytes != 16*1024*1024 ||
		defaults.MaxSourceRedirects != 5 || defaults.MaxSourceRetries != 3 {
		t.Fatalf("safe source fetch defaults = %+v", defaults)
	}
}

func TestLoadSourceNetworkAllowlistPreservesAdministratorValue(t *testing.T) {
	t.Setenv("OPENREADER_SOURCE_NETWORK_ALLOWLIST", "nas.home,192.168.50.0/24")
	if got := Load().SourceNetworkAllowlist; got != "nas.home,192.168.50.0/24" {
		t.Fatalf("SourceNetworkAllowlist = %q", got)
	}
	t.Setenv("OPENREADER_SOURCE_NETWORK_ALLOWLIST", "")
	if got := Load().SourceNetworkAllowlist; got != "" {
		t.Fatalf("default SourceNetworkAllowlist = %q, want empty", got)
	}
}

func TestLoadParserLimitsUsesConfiguredValuesAndSafeDefaults(t *testing.T) {
	t.Setenv("OPENREADER_MAX_ARCHIVE_ENTRIES", "12")
	t.Setenv("OPENREADER_MAX_ARCHIVE_ENTRY_BYTES", "2048")
	t.Setenv("OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES", "4096")
	t.Setenv("OPENREADER_MAX_PDF_PAGES", "13")
	t.Setenv("OPENREADER_MAX_PARSED_TEXT_BYTES", "8192")
	t.Setenv("OPENREADER_MAX_PARSED_CHAPTERS", "15")
	t.Setenv("OPENREADER_MAX_UMD_CHAPTERS", "14")

	configured := Load()
	if configured.MaxArchiveEntries != 12 || configured.MaxArchiveEntryBytes != 2048 ||
		configured.MaxArchiveExpandedBytes != 4096 || configured.MaxPDFPages != 13 ||
		configured.MaxParsedTextBytes != 8192 || configured.MaxParsedChapters != 15 || configured.MaxUMDChapters != 14 {
		t.Fatalf("configured parser limits = %+v", configured)
	}

	t.Setenv("OPENREADER_MAX_ARCHIVE_ENTRIES", "0")
	t.Setenv("OPENREADER_MAX_ARCHIVE_ENTRY_BYTES", "not-a-number")
	t.Setenv("OPENREADER_MAX_ARCHIVE_EXPANDED_BYTES", "-1")
	t.Setenv("OPENREADER_MAX_PDF_PAGES", "0")
	t.Setenv("OPENREADER_MAX_PARSED_TEXT_BYTES", "not-a-number")
	t.Setenv("OPENREADER_MAX_PARSED_CHAPTERS", "not-a-number")
	t.Setenv("OPENREADER_MAX_UMD_CHAPTERS", "-1")

	defaults := Load()
	if defaults.MaxArchiveEntries != 20_000 || defaults.MaxArchiveEntryBytes != 128*1024*1024 ||
		defaults.MaxArchiveExpandedBytes != 512*1024*1024 || defaults.MaxPDFPages != 10_000 ||
		defaults.MaxParsedTextBytes != 256*1024*1024 || defaults.MaxParsedChapters != 100_000 ||
		defaults.MaxUMDChapters != 100_000 {
		t.Fatalf("safe parser defaults = %+v", defaults)
	}
}

func TestLoadParsedChapterLimitFallsBackToExplicitLegacyUMDSetting(t *testing.T) {
	t.Setenv("OPENREADER_MAX_PARSED_CHAPTERS", "")
	t.Setenv("OPENREADER_MAX_UMD_CHAPTERS", "321")
	configured := Load()
	if configured.MaxParsedChapters != 321 || configured.MaxUMDChapters != 321 {
		t.Fatalf("parsed/UMD chapter compatibility fallback = %+v", configured)
	}
}

func TestLoadBackupRestoreLimitsUsesConfiguredValuesAndSafeDefaults(t *testing.T) {
	t.Setenv("OPENREADER_MAX_BACKUP_RESTORE_BYTES", "2048")
	t.Setenv("OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES", "12")
	t.Setenv("OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES", "1024")
	t.Setenv("OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES", "4096")

	configured := Load()
	if configured.MaxBackupRestoreBytes != 2048 || configured.MaxBackupArchiveEntries != 12 ||
		configured.MaxBackupArchiveBytes != 1024 || configured.MaxBackupArchiveTotal != 4096 {
		t.Fatalf("configured backup restore limits = %+v", configured)
	}

	t.Setenv("OPENREADER_MAX_BACKUP_RESTORE_BYTES", "0")
	t.Setenv("OPENREADER_MAX_BACKUP_ARCHIVE_ENTRIES", "not-a-number")
	t.Setenv("OPENREADER_MAX_BACKUP_ARCHIVE_ENTRY_BYTES", "-1")
	t.Setenv("OPENREADER_MAX_BACKUP_ARCHIVE_EXPANDED_BYTES", "0")

	defaults := Load()
	if defaults.MaxBackupRestoreBytes != 128*1024*1024 || defaults.MaxBackupArchiveEntries != 5_000 ||
		defaults.MaxBackupArchiveBytes != 16*1024*1024 || defaults.MaxBackupArchiveTotal != 128*1024*1024 {
		t.Fatalf("safe backup restore defaults = %+v", defaults)
	}
}

func TestLoadPortableBackupLimitsUsesConfiguredValuesAndSafeDefaults(t *testing.T) {
	t.Setenv("OPENREADER_MAX_PORTABLE_BACKUP_BYTES", "8192")
	t.Setenv("OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES", "12")
	t.Setenv("OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES", "4096")
	t.Setenv("OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES", "16384")

	configured := Load()
	if configured.MaxPortableBackupBytes != 8192 || configured.MaxPortableArchiveEntries != 12 ||
		configured.MaxPortableArchiveBytes != 4096 || configured.MaxPortableArchiveTotal != 16384 {
		t.Fatalf("configured portable backup limits = %+v", configured)
	}

	t.Setenv("OPENREADER_MAX_PORTABLE_BACKUP_BYTES", "0")
	t.Setenv("OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRIES", "not-a-number")
	t.Setenv("OPENREADER_MAX_PORTABLE_ARCHIVE_ENTRY_BYTES", "-1")
	t.Setenv("OPENREADER_MAX_PORTABLE_ARCHIVE_EXPANDED_BYTES", "0")

	defaults := Load()
	if defaults.MaxPortableBackupBytes != 512*1024*1024 || defaults.MaxPortableArchiveEntries != 10_000 ||
		defaults.MaxPortableArchiveBytes != 256*1024*1024 || defaults.MaxPortableArchiveTotal != 512*1024*1024 {
		t.Fatalf("safe portable backup defaults = %+v", defaults)
	}
}

func TestLoadChapterImageLimitsUsesConfiguredValuesAndSafeDefaults(t *testing.T) {
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGES", "7")
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGE_BYTES", "2048")
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGE_TOTAL_BYTES", "8192")
	t.Setenv("OPENREADER_CHAPTER_IMAGE_TIMEOUT_SECONDS", "4")
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGE_REDIRECTS", "2")

	configured := Load()
	if configured.MaxChapterImages != 7 || configured.MaxChapterImageBytes != 2048 ||
		configured.MaxChapterImageTotalBytes != 8192 || configured.ChapterImageTimeoutSeconds != 4 ||
		configured.MaxChapterImageRedirects != 2 {
		t.Fatalf("configured chapter image limits = %+v", configured)
	}

	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGES", "0")
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGE_BYTES", "invalid")
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGE_TOTAL_BYTES", "-1")
	t.Setenv("OPENREADER_CHAPTER_IMAGE_TIMEOUT_SECONDS", "0")
	t.Setenv("OPENREADER_MAX_CHAPTER_IMAGE_REDIRECTS", "invalid")

	defaults := Load()
	if defaults.MaxChapterImages != 64 || defaults.MaxChapterImageBytes != 8*1024*1024 ||
		defaults.MaxChapterImageTotalBytes != 32*1024*1024 || defaults.ChapterImageTimeoutSeconds != 12 ||
		defaults.MaxChapterImageRedirects != 3 {
		t.Fatalf("safe chapter image defaults = %+v", defaults)
	}
}
