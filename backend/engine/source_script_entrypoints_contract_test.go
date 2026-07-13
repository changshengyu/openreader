package engine

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"openreader/backend/models"
)

func TestSourceScriptEntryPointsRejectBeforeRemoteRequests(t *testing.T) {
	operations := []struct {
		name string
		run  func(models.BookSource) error
	}{
		{name: "search", run: func(source models.BookSource) error {
			_, err := SearchBooks(source, "脚本")
			return err
		}},
		{name: "explore", run: func(source models.BookSource) error {
			_, err := ExploreBooksPage(source, 1)
			return err
		}},
		{name: "book info and toc", run: func(source models.BookSource) error {
			_, _, err := FetchBookInfoAndTOC("/book/1", source)
			return err
		}},
		{name: "toc", run: func(source models.BookSource) error {
			_, err := ParseTOC("/book/1", source)
			return err
		}},
		{name: "content", run: func(source models.BookSource) error {
			_, err := FetchChapterContent("/chapter/1", source)
			return err
		}},
	}

	entryPoints := []struct {
		name  string
		apply func(*models.BookSource)
	}{
		{name: "dynamic @js header", apply: func(source *models.BookSource) {
			source.Header = `@js:return JSON.stringify({"X-Session":"secret"})`
		}},
		{name: "dynamic xml header", apply: func(source *models.BookSource) {
			source.Header = `<js>JSON.stringify({"X-Session":"secret"})</js>`
		}},
		{name: "login check", apply: func(source *models.BookSource) {
			source.LoginCheckJS = `return result`
		}},
	}

	for _, entryPoint := range entryPoints {
		t.Run(entryPoint.name, func(t *testing.T) {
			var requests atomic.Int32
			restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return nil, errors.New("script-configured source must not reach transport")
			})})
			defer restore()

			source := sourceScriptEntryPointFixture(t)
			entryPoint.apply(&source)
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					err := operation.run(source)
					if !errors.Is(err, ErrUnsupportedSourceRule) {
						t.Fatalf("%s error = %v, want ErrUnsupportedSourceRule", operation.name, err)
					}
				})
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("script entry point reached remote transport %d times, want 0", got)
			}
		})
	}
}

func sourceScriptEntryPointFixture(t *testing.T) models.BookSource {
	t.Helper()
	source := models.BookSource{
		ID:      88,
		Name:    "脚本入口契约源",
		BaseURL: "https://script-entry.example",
		Charset: "utf-8",
		Enabled: true,
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:       "https://script-entry.example/search?q={keyword}",
		ExploreURL:      "https://script-entry.example/explore?page={page}",
		BookListRule:    ".book",
		BookNameRule:    ".name|text",
		BookURLRule:     "a|attr:href",
		ChapterListRule: ".chapter",
		ChapterNameRule: ".title|text",
		ChapterURLRule:  "a|attr:href",
		ContentRule:     ".content|text",
	}); err != nil {
		t.Fatal(err)
	}
	return source
}
