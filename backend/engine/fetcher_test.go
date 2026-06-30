package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/traditionalchinese"
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

func TestDecodeBodySupportsExplicitUpstreamCharset(t *testing.T) {
	encoded, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte("繁體內容"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBody(encoded, "big5")
	if err != nil {
		t.Fatal(err)
	}
	if decoded != "繁體內容" {
		t.Fatalf("Big5 response decoded as %q", decoded)
	}
}

func TestDecodeBodyDetectsHTMLMetaCharsetWhenUnspecified(t *testing.T) {
	html := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=big5"></head><body>繁體內容</body></html>`
	encoded, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(html))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBody(encoded, "")
	if err != nil {
		t.Fatal(err)
	}
	if decoded != html {
		t.Fatalf("auto-detected HTML response decoded as %q", decoded)
	}
}

func TestSourceHTTPClientConfiguresAuthenticatedProxy(t *testing.T) {
	client, err := sourceHTTPClient(
		&http.Client{Timeout: time.Second},
		"http://127.0.0.1:18080@reader@secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatalf("proxy transport was not configured: %#v", client.Transport)
	}
	targetURL, err := url.Parse("https://source.example/content")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(&http.Request{URL: targetURL})
	if err != nil {
		t.Fatal(err)
	}
	password, _ := proxyURL.User.Password()
	if proxyURL.String() != "http://reader:secret@127.0.0.1:18080" ||
		proxyURL.User.Username() != "reader" ||
		password != "secret" {
		t.Fatalf("authenticated proxy = %v", proxyURL)
	}
}

func TestPerformSOCKS4HandshakeUsesSOCKS4aForDomain(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	serverErr := make(chan error, 1)
	go func() {
		expected := []byte{
			0x04, 0x01, 0x01, 0xbb,
			0x00, 0x00, 0x00, 0x01,
			'r', 'e', 'a', 'd', 'e', 'r', 0x00,
			'b', 'o', 'o', 'k', 's', '.', 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0x00,
		}
		actual := make([]byte, len(expected))
		if _, err := io.ReadFull(server, actual); err != nil {
			serverErr <- err
			return
		}
		if !bytes.Equal(actual, expected) {
			serverErr <- fmt.Errorf("SOCKS4a request = %x, want %x", actual, expected)
			return
		}
		_, err := server.Write([]byte{0x00, 0x5a, 0x01, 0xbb, 0x7f, 0x00, 0x00, 0x01})
		serverErr <- err
	}()

	if err := performSOCKS4Handshake(context.Background(), client, "books.example:443", "reader"); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestPerformSOCKS4HandshakeRejectsProxyFailure(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		request := make([]byte, 9)
		_, _ = io.ReadFull(server, request)
		_, _ = server.Write([]byte{0x00, 0x5b, 0x00, 0x50, 0x00, 0x00, 0x00, 0x00})
	}()

	err := performSOCKS4Handshake(context.Background(), client, "127.0.0.1:80", "")
	if err == nil || !strings.Contains(err.Error(), "0x5b") {
		t.Fatalf("expected SOCKS4 rejection, got %v", err)
	}
}

func TestFetchSourceTextHonorsConcurrentRateAndContext(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	request := SourceRequest{
		URL:            "https://source.example/content",
		SourceKey:      "rate-test-serial",
		ConcurrentRate: "80",
	}
	if _, _, err := FetchSourceTextWithURLContext(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, err := FetchSourceTextWithURLContext(ctx, request)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) < 15*time.Millisecond {
		t.Fatalf("rate wait did not honor context: elapsed=%v err=%v", time.Since(started), err)
	}

	windowRequest := SourceRequest{
		URL:            "https://source.example/content",
		SourceKey:      "rate-test-window",
		ConcurrentRate: "2/60",
	}
	if _, _, err := FetchSourceTextWithURLContext(context.Background(), windowRequest); err != nil {
		t.Fatal(err)
	}
	if _, _, err := FetchSourceTextWithURLContext(context.Background(), windowRequest); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	if _, _, err := FetchSourceTextWithURLContext(context.Background(), windowRequest); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 45*time.Millisecond {
		t.Fatalf("window rate did not delay third request: %v", time.Since(started))
	}
}
