package readingprogress

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"openreader/backend/models"
)

func ingressFixture(t *testing.T) (*Service, models.Book, []models.Chapter, models.ReadingProgress) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Book{}, &models.Chapter{}, &models.ReadingProgress{}); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: 1, Title: "书", Author: "作者", URL: "https://example.test/book"}
	if err := db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{{BookID: book.ID, Index: 0, Title: "第一章"}, {BookID: book.ID, Index: 1, Title: "第二章"}}
	if err := db.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	previous := models.ReadingProgress{UserID: 1, BookID: book.ID, ChapterID: chapters[0].ID, ChapterTitle: chapters[0].Title, Offset: 5, ChapterPercent: .3, Mode: "scroll", UpdatedAt: time.Now().Add(-time.Minute).UTC()}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	return New(db, t.TempDir()), book, chapters, previous
}

func ingressBody(t *testing.T, book models.Book, alter func(map[string]any)) string {
	t.Helper()
	payload := map[string]any{"bookUrl": book.URL, "name": book.Title, "author": book.Author, "durChapterIndex": 1, "durChapterPos": 240, "durChapterTime": time.Now().UnixMilli(), "durChapterTitle": "错误标题"}
	if alter != nil {
		alter(payload)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func uploadForTest(t *testing.T, s *Service, userID uint, path, body string) (bool, int, string) {
	t.Helper()
	events := 0
	stored := ""
	failed, err := s.UploadWebDAV(context.Background(), userID, path, strings.NewReader(body), func(r io.Reader) error {
		data, err := io.ReadAll(r)
		stored = string(data)
		return err
	}, func(result Result) {
		events++
		durable, err := s.load(userID, result.Book.ID)
		if err != nil || durable.Offset != result.Progress.Offset || durable.ChapterID != result.Progress.ChapterID {
			t.Errorf("notification preceded commit: %+v %v", durable, err)
		}
		if result.MirrorStatus != MirrorSkipped {
			t.Error("ingress invoked outgoing mirror")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored != body {
		t.Fatal("raw upload bytes changed")
	}
	return failed, events, stored
}

func TestIngressCanonicalCommitAndReplay(t *testing.T) {
	s, book, chapters, _ := ingressFixture(t)
	body := ingressBody(t, book, nil)
	failed, events, _ := uploadForTest(t, s, 1, "bookProgress/book.json", body)
	if failed || events != 1 {
		t.Fatalf("failed=%v events=%d", failed, events)
	}
	p, _ := s.load(1, book.ID)
	if p.ChapterID != chapters[1].ID || p.ChapterTitle != chapters[1].Title || p.Offset != 240 || p.ChapterPercent != 0 || p.Percent != .5 {
		t.Fatalf("noncanonical progress %+v", p)
	}
	failed, events, _ = uploadForTest(t, s, 1, "legado/bookProgress/renamed.json", body)
	if failed || events != 0 {
		t.Fatalf("replay failed=%v events=%d", failed, events)
	}
	again, _ := s.load(1, book.ID)
	if again != p {
		t.Fatalf("replay changed version: %+v", again)
	}
}

func TestIngressInvalidOrUnrelatedFilesRemainOrdinary(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		alter func(map[string]any)
		raw   string
	}{
		{name: "missing index", alter: func(p map[string]any) { delete(p, "durChapterIndex") }},
		{name: "null index", alter: func(p map[string]any) { p["durChapterIndex"] = nil }},
		{name: "negative index", alter: func(p map[string]any) { p["durChapterIndex"] = -1 }},
		{name: "missing chapter", alter: func(p map[string]any) { p["durChapterIndex"] = 99 }},
		{name: "negative offset", alter: func(p map[string]any) { p["durChapterPos"] = -1 }},
		{name: "missing time", alter: func(p map[string]any) { delete(p, "durChapterTime") }},
		{name: "old time", alter: func(p map[string]any) { p["durChapterTime"] = time.Now().Add(-time.Hour).UnixMilli() }},
		{name: "future clock", alter: func(p map[string]any) { p["durChapterTime"] = time.Now().Add(time.Hour).UnixMilli() }},
		{name: "unknown book", alter: func(p map[string]any) { p["bookUrl"] = "unknown"; p["name"] = "unknown" }},
		{name: "blank identity", alter: func(p map[string]any) { p["bookUrl"] = " "; p["name"] = " " }},
		{name: "recognition overflow", alter: func(p map[string]any) { p["padding"] = strings.Repeat("x", progressRecognitionBytes) }},
		{name: "ordinary path", path: "backup/bookProgress/book.json"},
		{name: "nested file", path: "bookProgress/sub/book.json"},
		{name: "suffix", path: "bookProgress/book.json.bak"},
		{name: "multiple objects", raw: "{} {}"},
		{name: "array", raw: "[]"},
		{name: "null", raw: "null"},
		{name: "invalid UTF8", raw: "{\"name\":\"\xff\"}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, book, _, previous := ingressFixture(t)
			body := tc.raw
			if body == "" {
				body = ingressBody(t, book, tc.alter)
			}
			path := tc.path
			if path == "" {
				path = "bookProgress/book.json"
			}
			failed, events, _ := uploadForTest(t, s, 1, path, body)
			p, _ := s.load(1, book.ID)
			if failed || events != 0 || p != previous {
				t.Fatalf("invalid upload applied: failed=%v events=%d p=%+v", failed, events, p)
			}
		})
	}
}

func TestIngressMatchingIsolationAndAmbiguity(t *testing.T) {
	for _, kind := range []string{"foreign", "ambiguous URL", "ambiguous name", "URL priority", "name fallback"} {
		t.Run(kind, func(t *testing.T) {
			s, book, _, previous := ingressFixture(t)
			caller := uint(1)
			if kind == "foreign" {
				caller = 2
			}
			if strings.HasPrefix(kind, "ambiguous") {
				duplicate := models.Book{UserID: 1, Title: book.Title, Author: book.Author, URL: book.URL}
				if err := s.db.Create(&duplicate).Error; err != nil {
					t.Fatal(err)
				}
			}
			body := ingressBody(t, book, func(p map[string]any) {
				if kind == "ambiguous name" || kind == "name fallback" {
					delete(p, "bookUrl")
				}
				if kind == "URL priority" {
					p["name"] = "not this name"
					p["author"] = "not this author"
				}
			})
			failed, events, _ := uploadForTest(t, s, caller, "bookProgress/book.json", body)
			p, _ := s.load(1, book.ID)
			want := 0
			if kind == "URL priority" || kind == "name fallback" {
				want = 1
			}
			if failed || events != want || (want == 0 && p != previous) {
				t.Fatalf("matching failed=%v events=%d progress=%+v", failed, events, p)
			}
		})
	}
}

func TestIngressSamePositionPreservesMeasuredFractionAndResetIsExplicit(t *testing.T) {
	s, book, _, previous := ingressFixture(t)
	body := ingressBody(t, book, func(p map[string]any) { p["durChapterIndex"] = 0; p["durChapterPos"] = 5 })
	_, events, _ := uploadForTest(t, s, 1, "bookProgress/book.json", body)
	p, _ := s.load(1, book.ID)
	if events != 0 || p != previous {
		t.Fatalf("same position discarded fraction %+v", p)
	}
	body = ingressBody(t, book, func(p map[string]any) { p["durChapterIndex"] = 0; delete(p, "durChapterPos") })
	failed, events, _ := uploadForTest(t, s, 1, "bookProgress/book.json", body)
	p, _ = s.load(1, book.ID)
	if failed || events != 1 || p.Offset != 0 || p.ChapterIndex != 0 || p.ChapterPercent != 0 {
		t.Fatalf("explicit reset %+v failed=%v events=%d", p, failed, events)
	}
}

func TestIngressFailureAndPostCommitCancellation(t *testing.T) {
	for _, kind := range []string{"file failure", "post-file cancel", "SQL failure", "catalog change"} {
		t.Run(kind, func(t *testing.T) {
			s, book, chapters, previous := ingressFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			events := 0
			if kind == "SQL failure" {
				if err := s.db.Callback().Update().Before("gorm:update").Register("ingress-fail", func(tx *gorm.DB) {
					if tx.Statement.Table == "reading_progresses" {
						tx.AddError(errors.New("injected"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "catalog change" {
				if err := s.db.Callback().Update().Before("gorm:update").Register("ingress-catalog", func(tx *gorm.DB) {
					if tx.Statement.Table == "reading_progresses" {
						tx.Exec("DELETE FROM chapters WHERE id = ?", chapters[1].ID)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			failed, err := s.UploadWebDAV(ctx, 1, "bookProgress/book.json", strings.NewReader(ingressBody(t, book, nil)), func(r io.Reader) error {
				_, readErr := io.Copy(io.Discard, r)
				if kind == "file failure" {
					return errors.New("file rejected")
				}
				if kind == "post-file cancel" {
					cancel()
				}
				return readErr
			}, func(Result) { events++ })
			p, _ := s.load(1, book.ID)
			if events != 0 || p != previous {
				t.Fatalf("failed publication modified progress: %+v events=%d", p, events)
			}
			if kind == "file failure" {
				if err == nil || failed {
					t.Fatalf("file error erased: %v %v", failed, err)
				}
			} else if err != nil || !failed {
				t.Fatalf("post-file diagnostic missing: %v %v", failed, err)
			}
		})
	}
}

func TestIngressGateCancellationCleanupAndOrdinarySaveWins(t *testing.T) {
	s, book, _, _ := ingressFixture(t)
	release, err := s.acquireUpload(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	published := false
	_, err = s.UploadWebDAV(ctx, 1, "legado/bookProgress/book.json", strings.NewReader(ingressBody(t, book, nil)), func(io.Reader) error { published = true; return nil }, nil)
	if !errors.Is(err, context.Canceled) || published {
		t.Fatal("cancelled queued upload published")
	}
	release()
	if len(s.uploads) != 0 {
		t.Fatal("unreferenced gate leaked")
	}
	body := ingressBody(t, book, nil)
	events := 0
	failed, err := s.UploadWebDAV(context.Background(), 1, "bookProgress/book.json", strings.NewReader(body), func(r io.Reader) error {
		_, err := io.Copy(io.Discard, r)
		if err != nil {
			return err
		}
		winner, err := s.Save(Input{UserID: 1, BookID: book.ID, ChapterIndex: 0, Offset: 333, ChapterPercent: .6})
		if err != nil || winner.Conflict {
			t.Fatalf("ordinary save failed: %+v %v", winner, err)
		}
		return nil
	}, func(Result) { events++ })
	p, _ := s.load(1, book.ID)
	if failed || err != nil || events != 0 || p.Offset != 333 || p.ChapterPercent != .6 {
		t.Fatalf("late upload beat ordinary save: %+v %v", p, err)
	}
}

func TestIngressInitialProgressAndExactRecognitionBudget(t *testing.T) {
	s, book, _, _ := ingressFixture(t)
	if err := s.db.Where("user_id = ? AND book_id = ?", 1, book.ID).Delete(&models.ReadingProgress{}).Error; err != nil {
		t.Fatal(err)
	}
	base := ingressBody(t, book, nil)
	prefix := strings.TrimSuffix(base, "}") + `,"padding":"`
	body := prefix + strings.Repeat("x", progressRecognitionBytes-len(prefix)-2) + `"}`
	if len(body) != progressRecognitionBytes {
		t.Fatal("fixture size")
	}
	failed, events, _ := uploadForTest(t, s, 1, "bookProgress/first.json", body)
	p, err := s.load(1, book.ID)
	if failed || events != 1 || err != nil || p.Offset != 240 || p.Mode != "scroll" {
		t.Fatalf("initial ingress: %+v %v failed=%v events=%d", p, err, failed, events)
	}
	var capture recognitionBuffer
	_, _ = capture.Write([]byte(strings.Repeat("x", progressRecognitionBytes*20)))
	if len(capture.data) != progressRecognitionBytes+1 {
		t.Fatal("unbounded recognition capture")
	}
}

func TestIngressCancelledWaiterNeverPublishesAndOtherUserIsIndependent(t *testing.T) {
	s, book, _, _ := ingressFixture(t)
	release, err := s.acquireUpload(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.UploadWebDAV(ctx, 1, "legado/bookProgress/queued.json", strings.NewReader(ingressBody(t, book, nil)), func(io.Reader) error { return errors.New("unexpected publication") }, nil)
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.uploadMu.Lock()
		refs := s.uploads[1].refs
		s.uploadMu.Unlock()
		if refs == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter never entered gate")
		}
		time.Sleep(time.Millisecond)
	}
	otherRelease, err := s.acquireUpload(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	otherRelease()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not cancel")
	}
	s.uploadMu.Lock()
	refs := s.uploads[1].refs
	s.uploadMu.Unlock()
	if refs != 1 {
		t.Fatal("cancelled waiter retained gate reference")
	}
}
