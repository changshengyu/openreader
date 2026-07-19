package chaptercache

import (
	"context"
	"errors"
)

// Item is the persistence-independent input for one selected catalogue row.
// Existing means a non-empty cache file was verified before the job started.
type Item struct {
	Index    int
	Existing bool
}

// Progress separates the complete usable cache count from work performed by
// this request. SelectedCached retains the legacy bounded-window meaning.
type Progress struct {
	Processed      int
	Total          int
	CachedCount    int
	SelectedCached int
	SuccessCount   int
	FailedCount    int
	ChapterIndex   int
}

type Fetch func(context.Context, Item) error
type OnProgress func(Progress) error

// Run executes a cancellable, ordered cache plan. Ordered execution is
// deliberate: source rules can persist book/chapter variables whose updates
// must not race one another.
func Run(
	ctx context.Context,
	items []Item,
	initialCached int,
	refresh bool,
	fetch Fetch,
	onProgress OnProgress,
) (Progress, error) {
	progress := Progress{Total: len(items), CachedCount: initialCached}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return progress, err
		}
		if !refresh && item.Existing {
			// A valid text cache remains successful even when optional derived
			// work (for example embedded-image hydration) fails. It still runs so
			// old mounted text caches can gain newly supported derived artifacts
			// without refetching chapter text from the source.
			if err := fetch(ctx, item); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return progress, err
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return progress, ctxErr
				}
			}
			progress.SelectedCached++
		} else if err := fetch(ctx, item); err != nil {
			progress.FailedCount++
		} else {
			progress.SuccessCount++
			progress.SelectedCached++
			if !item.Existing {
				progress.CachedCount++
			}
		}
		progress.Processed++
		progress.ChapterIndex = item.Index
		if onProgress != nil {
			if err := onProgress(progress); err != nil {
				return progress, err
			}
		}
	}
	return progress, nil
}
