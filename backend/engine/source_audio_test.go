package engine

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestFetchAudioChapterContentReturnsChapterURLWhenContentRuleEmpty(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			t.Fatalf("audio source with empty content rule should not fetch content page, requested %s", request.URL.String())
			return nil, nil
		}),
	})
	defer restore()

	source := models.BookSource{
		Name:       "空正文规则音频源",
		BaseURL:    "https://audio.example",
		SourceType: 1,
		Charset:    "utf-8",
	}
	if err := source.SetRules(models.BookSourceRule{}); err != nil {
		t.Fatal(err)
	}

	content, err := FetchChapterContent("/media/001.mp3", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "https://audio.example/media/001.mp3" {
		t.Fatalf("audio empty content rule returned %q", content)
	}
}

func TestFetchAudioChapterContentResolvesRelativeMediaURL(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != "https://audio.example/chapter/1" {
				t.Fatalf("unexpected content request: %s", request.URL.String())
			}
			body := `<html><body><audio src="../media/001.mp3"></audio></body></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{
		Name:       "相对音频源",
		BaseURL:    "https://audio.example/books/book-1/",
		SourceType: 1,
		Charset:    "utf-8",
	}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule: "audio|attr:src",
	}); err != nil {
		t.Fatal(err)
	}

	content, err := FetchChapterContent("https://audio.example/chapter/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "https://audio.example/media/001.mp3" {
		t.Fatalf("audio content rule returned %q", content)
	}
}

func TestFetchAudioChapterContentExtractsSourceAndAnchorMediaURLs(t *testing.T) {
	tests := []struct {
		name string
		rule string
		body string
		want string
	}{
		{
			name: "source src",
			rule: "source|attr:src",
			body: `<audio><source src="/media/source-001.m4a"></audio>`,
			want: "https://audio.example/media/source-001.m4a",
		},
		{
			name: "anchor href",
			rule: "a.play|attr:href",
			body: `<a class="play" href="//cdn.audio.example/track/001.ogg">播放</a>`,
			want: "https://cdn.audio.example/track/001.ogg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := SetHTTPClient(&http.Client{
				Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tt.body)),
						Header:     make(http.Header),
						Request:    request,
					}, nil
				}),
			})
			defer restore()

			source := models.BookSource{
				Name:       "音频媒体规则源",
				BaseURL:    "https://audio.example/books/book-1/",
				SourceType: 1,
				Charset:    "utf-8",
			}
			if err := source.SetRules(models.BookSourceRule{
				ContentRule: tt.rule,
			}); err != nil {
				t.Fatal(err)
			}
			content, err := FetchChapterContent("/chapter/1", source)
			if err != nil {
				t.Fatal(err)
			}
			if content != tt.want {
				t.Fatalf("%s content = %q, want %q", tt.name, content, tt.want)
			}
		})
	}
}
