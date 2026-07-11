package config

import "testing"

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
