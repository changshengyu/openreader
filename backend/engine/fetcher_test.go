package engine

import (
	"context"
	"errors"
	"net/http"
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
