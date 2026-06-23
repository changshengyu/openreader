package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type contextRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn contextRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestFetchTextContextCancelsHTTPRequest(t *testing.T) {
	requestCanceled := make(chan struct{}, 1)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			requestCanceled <- struct{}{}
			return nil, request.Context().Err()
		}),
	})
	defer restore()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := FetchTextContext(ctx, "https://slow.example/content", "utf-8")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline error, got %v", err)
	}
	select {
	case <-requestCanceled:
	default:
		t.Fatal("expected HTTP transport to receive request cancellation")
	}
}

func TestFetchSourceTextRetriesHTTPFailuresAndReturnsBinaryHex(t *testing.T) {
	attempts := 0
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			attempts++
			status := http.StatusServiceUnavailable
			body := "temporary"
			if attempts == 3 {
				status = http.StatusOK
				body = string([]byte{0x00, 0x0f, 0xa5, 0xff})
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	body, responseURL, err := FetchSourceTextWithURLContext(context.Background(), SourceRequest{
		URL:     "https://source.example/binary",
		Method:  http.MethodGet,
		Charset: "gbk",
		Retry:   2,
		Type:    "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || body != "000fa5ff" || responseURL != "https://source.example/binary" {
		t.Fatalf("unexpected retry/type result: attempts=%d body=%q responseURL=%q", attempts, body, responseURL)
	}
}

func TestFetchSourceTextDoesNotRetryTransportErrors(t *testing.T) {
	attempts := 0
	expected := errors.New("network unavailable")
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			attempts++
			return nil, expected
		}),
	})
	defer restore()

	_, _, err := FetchSourceTextWithURLContext(context.Background(), SourceRequest{
		URL:   "https://source.example/content",
		Retry: 4,
	})
	if !errors.Is(err, expected) || attempts != 1 {
		t.Fatalf("transport error retry behavior: attempts=%d err=%v", attempts, err)
	}
}
