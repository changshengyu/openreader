package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const contractMaxSourceResponseBytes = 16 * 1024 * 1024

type policyTrackingBody struct {
	reader io.Reader
	reads  atomic.Int32
	closed atomic.Bool
}

func (body *policyTrackingBody) Read(buffer []byte) (int, error) {
	body.reads.Add(1)
	return body.reader.Read(buffer)
}

func (body *policyTrackingBody) Close() error {
	body.closed.Store(true)
	return nil
}

func TestSharedFetcherRejectsUnsafeURLBeforeTransport(t *testing.T) {
	var requests atomic.Int32
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("unexpected")),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	defer restore()

	for _, rawURL := range []string{
		"file:///etc/passwd",
		"data:text/plain,secret",
		"ftp://source.example/feed",
		"//source.example/feed",
		"https://alice:secret@source.example/feed",
		"https:///missing-host",
		"https://source.example:bad/feed",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := FetchTextContext(context.Background(), rawURL, "utf-8")
			if !errors.Is(err, ErrUnsafeSourceURL) {
				t.Fatalf("FetchTextContext(%q) error = %v, want ErrUnsafeSourceURL", rawURL, err)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("unsafe URLs reached the transport %d times", got)
	}
}

func TestSharedFetcherBoundsResponseBeforeDecodeAndClosesBody(t *testing.T) {
	t.Run("content length", func(t *testing.T) {
		body := &policyTrackingBody{reader: strings.NewReader("must not be read")}
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: contractMaxSourceResponseBytes + 1,
				Body:          body,
				Header:        make(http.Header),
				Request:       request,
			}, nil
		})})
		defer restore()

		_, err := FetchTextContext(context.Background(), "https://source.example/oversized", "utf-8")
		if !errors.Is(err, ErrSourceResponseLimit) {
			t.Fatalf("oversized Content-Length error = %v, want ErrSourceResponseLimit", err)
		}
		if body.reads.Load() != 0 || !body.closed.Load() {
			t.Fatalf("oversized Content-Length body reads=%d closed=%v", body.reads.Load(), body.closed.Load())
		}
	})

	t.Run("chunked body", func(t *testing.T) {
		body := &policyTrackingBody{reader: io.LimitReader(zeroReader{}, contractMaxSourceResponseBytes+1)}
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: -1,
				Body:          body,
				Header:        make(http.Header),
				Request:       request,
			}, nil
		})})
		defer restore()

		_, _, err := FetchSourceTextWithURLContext(context.Background(), SourceRequest{
			URL:  "https://source.example/oversized-chunked",
			Type: "application/octet-stream",
		})
		if !errors.Is(err, ErrSourceResponseLimit) {
			t.Fatalf("oversized chunked error = %v, want ErrSourceResponseLimit", err)
		}
		if !body.closed.Load() {
			t.Fatal("oversized chunked body was not closed")
		}
	})
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 0
	}
	return len(buffer), nil
}

func TestSharedFetcherCapsSourceRetryWithoutChangingFinalBody(t *testing.T) {
	var attempts atomic.Int32
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempt := attempts.Add(1)
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("attempt-" + strconv.Itoa(int(attempt)))),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	defer restore()

	body, _, err := FetchSourceTextWithURLContext(context.Background(), SourceRequest{
		URL:   "https://source.example/retry",
		Retry: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 4 || body != "attempt-4" {
		t.Fatalf("bounded retry attempts=%d body=%q, want 4 and final body", attempts.Load(), body)
	}
}

func TestSharedFetcherRedirectLimitAndHeaderIsolation(t *testing.T) {
	t.Run("allows five redirects and rejects sixth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			step, _ := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/"))
			final, _ := strconv.Atoi(request.URL.Query().Get("final"))
			if step < final {
				http.Redirect(writer, request, "/"+strconv.Itoa(step+1)+"?final="+strconv.Itoa(final), http.StatusFound)
				return
			}
			_, _ = writer.Write([]byte("ok"))
		}))
		defer server.Close()

		body, err := FetchTextContext(context.Background(), server.URL+"/0?final=5", "utf-8")
		if err != nil || body != "ok" {
			t.Fatalf("five redirects body=%q err=%v", body, err)
		}
		_, err = FetchTextContext(context.Background(), server.URL+"/0?final=6", "utf-8")
		if !errors.Is(err, ErrSourceRedirectLimit) {
			t.Fatalf("six redirects error = %v, want ErrSourceRedirectLimit", err)
		}
	})

	t.Run("same origin keeps source headers", func(t *testing.T) {
		seen := make(chan http.Header, 1)
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/start" {
				http.Redirect(writer, request, "/final", http.StatusFound)
				return
			}
			seen <- request.Header.Clone()
			_, _ = writer.Write([]byte("ok"))
		}))
		defer server.Close()

		_, err := FetchTextWithHeadersContext(context.Background(), server.URL+"/start", "utf-8", map[string]string{
			"Authorization":  "Bearer same-origin-secret",
			"Cookie":         "session=same-origin-secret",
			"X-Source-Token": "same-origin-secret",
		})
		if err != nil {
			t.Fatal(err)
		}
		headers := <-seen
		if headers.Get("Authorization") == "" || headers.Get("Cookie") == "" || headers.Get("X-Source-Token") == "" {
			t.Fatalf("same-origin source headers were dropped: %v", headers)
		}
	})

	t.Run("cross origin drops credentials and custom source headers", func(t *testing.T) {
		seen := make(chan http.Header, 1)
		target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			seen <- request.Header.Clone()
			_, _ = writer.Write([]byte("ok"))
		}))
		defer target.Close()
		origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, target.URL+"/final", http.StatusFound)
		}))
		defer origin.Close()

		_, err := FetchTextWithHeadersContext(context.Background(), origin.URL+"/start", "utf-8", map[string]string{
			"Authorization":       "Bearer redirect-secret",
			"Cookie":              "session=redirect-secret",
			"Proxy-Authorization": "Basic redirect-secret",
			"X-Source-Token":      "redirect-secret",
			"Accept-Language":     "zh-CN",
		})
		if err != nil {
			t.Fatal(err)
		}
		headers := <-seen
		for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "X-Source-Token"} {
			if headers.Get(name) != "" {
				t.Fatalf("cross-origin redirect retained %s: %v", name, headers)
			}
		}
		if headers.Get("Accept-Language") != "zh-CN" || headers.Get("User-Agent") == "" {
			t.Fatalf("safe redirect headers were lost: %v", headers)
		}
	})
}

func TestSharedFetcherRedactsTransportFailureWithoutBreakingErrorsIs(t *testing.T) {
	secretFailure := errors.New("dial https://alice:password@secret.example/path?token=private via socks5://bob:proxy-secret@127.0.0.1")
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, secretFailure
	})})
	defer restore()

	_, _, err := FetchSourceTextWithURLContext(context.Background(), SourceRequest{URL: "https://source.example/content"})
	if !errors.Is(err, ErrSourceRequest) || !errors.Is(err, secretFailure) {
		t.Fatalf("source transport classification = %v", err)
	}
	for _, forbidden := range []string{"alice", "password", "secret.example", "token=private", "bob", "proxy-secret", "127.0.0.1"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("transport error leaked %q: %v", forbidden, err)
		}
	}
}
