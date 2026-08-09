package remotereader

import (
	"errors"
	"strings"
	"testing"
	"time"

	"openreader/backend/models"
)

func TestStoreUsesIdleAndAbsoluteExpiryWithoutResurrection(t *testing.T) {
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	limits := DefaultLimits()
	limits.IdleTTL = 10 * time.Minute
	limits.MaxTTL = 25 * time.Minute
	store := NewStore(limits, func() time.Time { return now })

	session, err := store.Create(1, models.BookSource{Name: "expiry"}, models.Book{Title: "expiry"}, []models.Chapter{{Index: 0, Title: "one"}})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(9 * time.Minute)
	first, err := store.Get(1, session.ID)
	if err != nil || !first.ExpiresAt.Equal(time.Date(2026, 8, 9, 8, 19, 0, 0, time.UTC)) {
		t.Fatalf("first renewal = %v, %v", first.ExpiresAt, err)
	}
	now = now.Add(9 * time.Minute)
	second, err := store.Get(1, session.ID)
	if err != nil || !second.ExpiresAt.Equal(session.MaxExpiresAt) {
		t.Fatalf("absolute-capped renewal = %v, %v; max=%v", second.ExpiresAt, err, session.MaxExpiresAt)
	}
	now = session.MaxExpiresAt
	if _, err := store.Get(1, session.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("absolute deadline error = %v, want ErrExpired", err)
	}
	if _, err := store.Get(1, session.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("remembered natural expiry error = %v, want ErrExpired", err)
	}

	other, err := store.Create(1, models.BookSource{Name: "purge"}, models.Book{Title: "purge"}, []models.Chapter{{Index: 0, Title: "one"}})
	if err != nil {
		t.Fatal(err)
	}
	now = other.ExpiresAt
	if _, err := store.Create(1, models.BookSource{Name: "trigger purge"}, models.Book{Title: "trigger purge"}, []models.Chapter{{Index: 0, Title: "one"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(1, other.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("purged natural expiry error = %v, want ErrExpired", err)
	}
}

func TestStoreEvictsLeastRecentlyUsedWithinUserAndProcessBudgets(t *testing.T) {
	now := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	limits := DefaultLimits()
	limits.MaxUserSessions = 2
	limits.MaxSessions = 3
	store := NewStore(limits, func() time.Time { return now })

	create := func(userID uint, name string) Session {
		t.Helper()
		session, err := store.Create(userID, models.BookSource{Name: name}, models.Book{Title: name}, []models.Chapter{{Index: 0, Title: name}})
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
		return session
	}

	a := create(1, "a")
	b := create(1, "b")
	if _, err := store.Get(1, a.ID); err != nil {
		t.Fatal(err)
	}
	c := create(1, "c")
	if _, err := store.Get(1, b.ID); !errors.Is(err, ErrMissing) {
		t.Fatalf("least-recently-used user session error = %v, want ErrMissing", err)
	}
	for _, id := range []string{a.ID, c.ID} {
		if _, err := store.Get(1, id); err != nil {
			t.Fatalf("retained user session %s: %v", id, err)
		}
	}

	d := create(2, "d")
	if _, err := store.Get(2, d.ID); err != nil {
		t.Fatal(err)
	}
	e := create(3, "e")
	if _, err := store.Get(1, a.ID); !errors.Is(err, ErrMissing) {
		t.Fatalf("global least-recently-used session error = %v, want ErrMissing", err)
	}
	for _, item := range []struct {
		userID uint
		id     string
	}{{1, c.ID}, {2, d.ID}, {3, e.ID}} {
		if _, err := store.Get(item.userID, item.id); err != nil {
			t.Fatalf("retained global session %+v: %v", item, err)
		}
	}
}

func TestStoreEnforcesRetainedByteBudgetsBeforeEviction(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	source := models.BookSource{Name: "sized", Rules: strings.Repeat("r", 512)}
	book := models.Book{Title: "sized"}
	chapters := []models.Chapter{{Index: 0, Title: "one", URL: strings.Repeat("u", 128)}}
	retained, err := retainedBytesFor(source, book, chapters)
	if err != nil {
		t.Fatal(err)
	}

	limits := DefaultLimits()
	limits.MaxSessionBytes = retained - 1
	store := NewStore(limits, func() time.Time { return now })
	if _, err := store.Create(1, source, book, chapters); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("single-session byte error = %v, want ErrTooLarge", err)
	}
	if len(store.sessions) != 0 {
		t.Fatalf("oversized create inserted %d sessions", len(store.sessions))
	}

	limits = DefaultLimits()
	limits.MaxSessionBytes = retained + 1
	limits.MaxUserBytes = retained*2 - 1
	limits.MaxBytes = retained*4 + 1
	store = NewStore(limits, func() time.Time { return now })
	first, err := store.Create(1, source, book, chapters)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	second, err := store.Create(1, source, book, chapters)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(1, first.ID); !errors.Is(err, ErrMissing) {
		t.Fatalf("user byte-budget eviction error = %v, want ErrMissing", err)
	}
	if _, err := store.Get(1, second.ID); err != nil {
		t.Fatalf("new session after user byte eviction: %v", err)
	}

	limits = DefaultLimits()
	limits.MaxSessionBytes = retained + 1
	limits.MaxUserBytes = retained*2 + 1
	limits.MaxBytes = retained*2 - 1
	store = NewStore(limits, func() time.Time { return now })
	globalFirst, err := store.Create(1, source, book, chapters)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	globalSecond, err := store.Create(2, source, book, chapters)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(1, globalFirst.ID); !errors.Is(err, ErrMissing) {
		t.Fatalf("global byte-budget eviction error = %v, want ErrMissing", err)
	}
	if _, err := store.Get(2, globalSecond.ID); err != nil {
		t.Fatalf("new session after global byte eviction: %v", err)
	}
}

func TestStoreCommitsOnlyCurrentChapterVariablesAndClonesResults(t *testing.T) {
	store := NewStore(DefaultLimits(), time.Now)
	session, err := store.Create(7, models.BookSource{Name: "variables"}, models.Book{Title: "variables", Variable: `{"book":"before"}`}, []models.Chapter{
		{Index: 4, Title: "four", Variable: `{"chapter":"four"}`},
		{Index: 9, Title: "nine", Variable: `{"chapter":"nine"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVariables(7, session.ID, `{"book":"after"}`, 4, `{"chapter":"changed"}`); err != nil {
		t.Fatalf("expected variable update to commit: %v", err)
	}
	updated, err := store.Get(7, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Book.Variable != `{"book":"after"}` || updated.Chapters[0].Variable != `{"chapter":"changed"}` || updated.Chapters[1].Variable != `{"chapter":"nine"}` {
		t.Fatalf("updated variable scopes = book %q, chapters %#v", updated.Book.Variable, updated.Chapters)
	}
	updated.Chapters[0].Variable = "mutated-client-copy"
	again, err := store.Get(7, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Chapters[0].Variable != `{"chapter":"changed"}` {
		t.Fatalf("returned session aliased store state: %q", again.Chapters[0].Variable)
	}
}
