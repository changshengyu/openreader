package chaptercache

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunPreparesExistingItemsWithoutChangingTextCacheCounters(t *testing.T) {
	prepared := make([]int, 0, 2)
	result, err := Run(context.Background(), []Item{
		{Index: 3, Existing: true},
		{Index: 4, Existing: false},
	}, 7, false, func(_ context.Context, item Item) error {
		prepared = append(prepared, item.Index)
		if item.Existing {
			return errors.New("optional derived-image preparation failed")
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prepared, []int{3, 4}) {
		t.Fatalf("prepared indexes=%v, want existing and uncached items", prepared)
	}
	if result.SelectedCached != 2 || result.SuccessCount != 1 || result.FailedCount != 0 || result.CachedCount != 8 {
		t.Fatalf("derived preparation changed text-cache counters: %+v", result)
	}
}

func TestRunPropagatesExistingItemPreparationCancellation(t *testing.T) {
	result, err := Run(context.Background(), []Item{{Index: 9, Existing: true}}, 1, false, func(context.Context, Item) error {
		return context.Canceled
	}, nil)
	if !errors.Is(err, context.Canceled) || result.Processed != 0 || result.SelectedCached != 0 {
		t.Fatalf("existing preparation cancellation result=%+v err=%v", result, err)
	}
}
