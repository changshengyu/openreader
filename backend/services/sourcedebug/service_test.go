package sourcedebug

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBoundedEmitterCapsEventCountAndBytes(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		emitter := &boundedEmitter{ctx: context.Background(), target: func(Event) error { return nil }}
		for index := 0; index < maxEvents; index++ {
			if err := emitter.Emit(Event{Name: "log", Data: map[string]any{"message": "safe"}}); err != nil {
				t.Fatalf("event %d: %v", index, err)
			}
		}
		if err := emitter.Emit(Event{Name: "log", Data: map[string]any{"message": "overflow"}}); !errors.Is(err, ErrEventLimit) {
			t.Fatalf("event count overflow = %v", err)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		emitter := &boundedEmitter{ctx: context.Background(), target: func(Event) error { return nil }}
		err := emitter.Emit(Event{Name: "log", Data: map[string]any{"message": strings.Repeat("x", maxEventBytes)}})
		if !errors.Is(err, ErrEventLimit) {
			t.Fatalf("event byte overflow = %v", err)
		}
	})
}

func TestBoundedEmitterStopsBeforeTargetAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	emitter := &boundedEmitter{ctx: ctx, target: func(Event) error {
		called = true
		return nil
	}}
	if err := emitter.Emit(Event{Name: "log", Data: map[string]any{"message": "safe"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled emitter = %v", err)
	}
	if called {
		t.Fatal("canceled emitter reached target")
	}
}
