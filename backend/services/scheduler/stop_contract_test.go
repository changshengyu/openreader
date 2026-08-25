package scheduler

import "testing"

func TestStopIsIdempotent(t *testing.T) {
	scheduler := New(nil, 0)
	scheduler.Stop()
	scheduler.Stop()
}
