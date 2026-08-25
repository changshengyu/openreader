package backup

import "testing"

func TestStopIsIdempotent(t *testing.T) {
	service := New(nil, t.TempDir())
	service.Stop()
	service.Stop()
}
