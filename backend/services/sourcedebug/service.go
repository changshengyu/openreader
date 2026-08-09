package sourcedebug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	DefaultKeyword = "我的"
	maxEvents      = 128
	maxEventBytes  = 64 * 1024
)

var ErrEventLimit = errors.New("source debug event limit exceeded")

type Event struct {
	Name string
	Data map[string]any
}

type EmitFunc func(Event) error

type Result struct {
	ContentLength int
}

type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string {
	if e == nil || e.Err == nil {
		return "source debug failed"
	}
	return e.Err.Error()
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ErrorStage(err error) string {
	var target *StageError
	if errors.As(err, &target) {
		return target.Stage
	}
	return ""
}

// Run reproduces reader-dev's automatic source-debug dispatch while keeping
// all parser state in memory. It never writes a source, shelf row, cache row,
// sync event, or source-failure row.
func Run(ctx context.Context, source models.BookSource, keyword string, emit EmitFunc) (Result, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		keyword = DefaultKeyword
	}
	emitter := &boundedEmitter{ctx: ctx, target: emit}
	runner := debugRunner{ctx: ctx, source: source, emit: emitter.Emit}
	if err := emitter.Emit(Event{Name: "log", Data: map[string]any{
		"stage":   "dispatch",
		"status":  "start",
		"message": "开始书源调试",
	}}); err != nil {
		return Result{}, err
	}

	switch {
	case strings.HasPrefix(keyword, "++"):
		return runner.runTOC(strings.TrimSpace(strings.TrimPrefix(keyword, "++")), "", "")
	case strings.HasPrefix(keyword, "--"):
		return runner.runContent(strings.TrimSpace(strings.TrimPrefix(keyword, "--")), "", engine.SourceRuleVariableState{})
	case isAbsoluteHTTPURL(keyword):
		return runner.runBookChain(keyword)
	case strings.Contains(keyword, "::"):
		parts := strings.SplitN(keyword, "::", 2)
		return runner.runExplore(strings.TrimSpace(parts[1]))
	default:
		return runner.runSearch(keyword)
	}
}

type debugRunner struct {
	ctx    context.Context
	source models.BookSource
	emit   EmitFunc
}

func (r debugRunner) runSearch(keyword string) (Result, error) {
	if err := r.stage("search", "start", "开始搜索", -1); err != nil {
		return Result{}, err
	}
	result, err := engine.SearchBooksPageContext(r.ctx, r.source, keyword, 1)
	if err != nil {
		return Result{}, r.fail("search", err)
	}
	if len(result.Items) == 0 {
		if err := r.stage("search", "empty", "搜索结果为空", 0); err != nil {
			return Result{}, err
		}
		return Result{}, nil
	}
	if err := r.stage("search", "success", "搜索完成", len(result.Items)); err != nil {
		return Result{}, err
	}
	// reader-dev's infoDebug passes only bookUrl. Search-result variables are
	// deliberately not carried into the new BookInfo debug object.
	return r.runBookChain(result.Items[0].BookURL)
}

func (r debugRunner) runExplore(exploreURL string) (Result, error) {
	if err := r.stage("explore", "start", "开始发现", -1); err != nil {
		return Result{}, err
	}
	result, err := engine.ExploreBooksPageWithURLContext(r.ctx, r.source, exploreURL, 1)
	if err != nil {
		return Result{}, r.fail("explore", err)
	}
	if len(result.Items) == 0 {
		if err := r.stage("explore", "empty", "发现结果为空", 0); err != nil {
			return Result{}, err
		}
		return Result{}, nil
	}
	if err := r.stage("explore", "success", "发现完成", len(result.Items)); err != nil {
		return Result{}, err
	}
	return r.runBookChain(result.Items[0].BookURL)
}

func (r debugRunner) runBookChain(bookURL string) (Result, error) {
	if err := r.stage("book_info", "start", "开始解析书籍详情", -1); err != nil {
		return Result{}, err
	}
	tocStarted := false
	info, chapters, bookVariable, err := engine.FetchBookInfoAndTOCWithVariablesContext(
		r.ctx,
		bookURL,
		r.source,
		"",
		"",
		func(engine.RemoteBookInfo) error {
			if err := r.stage("book_info", "success", "书籍详情解析完成", -1); err != nil {
				return err
			}
			tocStarted = true
			return r.stage("toc", "start", "开始解析目录", -1)
		},
	)
	if err != nil {
		stage := "book_info"
		if tocStarted {
			stage = "toc"
		}
		if errors.Is(err, engine.ErrNoChapters) && stage == "toc" {
			if emitErr := r.stage("toc", "empty", "目录为空", 0); emitErr != nil {
				return Result{}, emitErr
			}
			return Result{}, nil
		}
		return Result{}, r.fail(stage, err)
	}
	if err := r.stage("toc", "success", "目录解析完成", len(chapters)); err != nil {
		return Result{}, err
	}
	if len(chapters) == 0 {
		return Result{}, nil
	}
	nextURL := ""
	if len(chapters) > 1 {
		nextURL = chapters[1].URL
	}
	return r.runContent(chapters[0].URL, nextURL, engine.SourceRuleVariableState{
		BookVariable:    bookVariable,
		ChapterVariable: chapters[0].Variable,
		BookName:        info.Title,
		ChapterTitle:    chapters[0].Title,
	})
}

func (r debugRunner) runTOC(bookURL, bookVariable, bookName string) (Result, error) {
	if err := r.stage("toc", "start", "开始解析目录", -1); err != nil {
		return Result{}, err
	}
	chapters, nextBookVariable, err := engine.ParseTOCWithVariablesContext(r.ctx, bookURL, r.source, bookVariable, bookName)
	if err != nil {
		if errors.Is(err, engine.ErrNoChapters) {
			if emitErr := r.stage("toc", "empty", "目录为空", 0); emitErr != nil {
				return Result{}, emitErr
			}
			return Result{}, nil
		}
		return Result{}, r.fail("toc", err)
	}
	if err := r.stage("toc", "success", "目录解析完成", len(chapters)); err != nil {
		return Result{}, err
	}
	if len(chapters) == 0 {
		return Result{}, nil
	}
	nextURL := ""
	if len(chapters) > 1 {
		nextURL = chapters[1].URL
	}
	return r.runContent(chapters[0].URL, nextURL, engine.SourceRuleVariableState{
		BookVariable:    nextBookVariable,
		ChapterVariable: chapters[0].Variable,
		BookName:        bookName,
		ChapterTitle:    chapters[0].Title,
	})
}

func (r debugRunner) runContent(chapterURL, nextChapterURL string, state engine.SourceRuleVariableState) (Result, error) {
	if err := r.stage("content", "start", "开始解析正文", -1); err != nil {
		return Result{}, err
	}
	content, _, err := engine.FetchChapterContentContextWithState(r.ctx, chapterURL, nextChapterURL, r.source, state)
	if err != nil {
		return Result{}, r.fail("content", err)
	}
	length := utf8.RuneCountInString(content)
	if err := r.stage("content", "success", "正文解析完成", length); err != nil {
		return Result{}, err
	}
	return Result{ContentLength: length}, nil
}

func (r debugRunner) stage(stage, status, message string, count int) error {
	data := map[string]any{
		"stage":   stage,
		"status":  status,
		"message": message,
	}
	if count >= 0 {
		data["count"] = count
	}
	return r.emit(Event{Name: "stage", Data: data})
}

func (r debugRunner) fail(stage string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrEventLimit) {
		return err
	}
	_ = r.stage(stage, "error", "调试阶段失败", -1)
	return &StageError{Stage: stage, Err: err}
}

type boundedEmitter struct {
	ctx    context.Context
	target EmitFunc
	count  int
	bytes  int
}

func (e *boundedEmitter) Emit(event Event) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("encode source debug event: %w", err)
	}
	if e.count >= maxEvents || e.bytes+len(encoded) > maxEventBytes {
		return ErrEventLimit
	}
	e.count++
	e.bytes += len(encoded)
	if e.target == nil {
		return nil
	}
	return e.target(event)
}

func isAbsoluteHTTPURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
