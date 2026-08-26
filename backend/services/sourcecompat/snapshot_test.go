package sourcecompat

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateSnapshotBoundsRawEntries(t *testing.T) {
	entries := make([]map[string]string, DefaultSnapshotMaxEntries+1)
	for index := range entries {
		entries[index] = map[string]string{"bookSourceName": "source"}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapshot(data); !errors.Is(err, ErrSnapshotTooManyEntries) {
		t.Fatalf("301-entry snapshot error = %v, want ErrSnapshotTooManyEntries", err)
	}
	if err := ValidateSnapshot([]byte(`{"bookSources":[]}`)); err != nil {
		t.Fatalf("explicit empty wrapper rejected: %v", err)
	}
	if err := ValidateSnapshot([]byte(`[null]`)); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("non-object entry error = %v, want ErrInvalidSnapshot", err)
	}
	if err := ValidateSnapshot([]byte{'[', '"', 0xff, '"', ']'}); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrInvalidSnapshot", err)
	}
}
