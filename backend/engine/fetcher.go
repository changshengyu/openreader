package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/proxy"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

var defaultClient = &http.Client{Timeout: 12 * time.Second}
var sourceProxyPattern = regexp.MustCompile(`^(http|socks4|socks5)://(.+):([0-9]{2,5})(?:@([^@]*)@([^@]*))?$`)
var sourceRateLimiters sync.Map

type sourceRateLimiter struct {
	serial     chan struct{}
	mu         sync.Mutex
	lastStart  time.Time
	windowFrom time.Time
	windowUsed int
}

func SetHTTPClient(client *http.Client) func() {
	previous := defaultClient
	if client == nil {
		defaultClient = &http.Client{Timeout: 12 * time.Second}
	} else {
		defaultClient = client
	}
	return func() {
		defaultClient = previous
	}
}

func FetchDocument(url, charset string) (*goquery.Document, error) {
	return FetchDocumentContext(context.Background(), url, charset)
}

func FetchDocumentContext(ctx context.Context, url, charset string) (*goquery.Document, error) {
	return FetchDocumentWithHeadersContext(ctx, url, charset, nil)
}

func FetchDocumentWithHeaders(url, charset string, headers map[string]string) (*goquery.Document, error) {
	return FetchDocumentWithHeadersContext(context.Background(), url, charset, headers)
}

func FetchDocumentWithHeadersContext(ctx context.Context, url, charset string, headers map[string]string) (*goquery.Document, error) {
	decoded, err := FetchTextRequestContext(ctx, http.MethodGet, url, "", charset, headers)
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(decoded))
}

func FetchDocumentRequestContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (*goquery.Document, error) {
	document, _, err := FetchDocumentRequestWithURLContext(ctx, method, url, body, charset, headers)
	return document, err
}

func FetchDocumentRequestWithURLContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (*goquery.Document, string, error) {
	decoded, responseURL, err := FetchTextRequestWithURLContext(ctx, method, url, body, charset, headers)
	if err != nil {
		return nil, responseURL, err
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(decoded))
	return document, responseURL, err
}

func FetchSourceDocumentWithURLContext(ctx context.Context, request SourceRequest) (*goquery.Document, string, error) {
	decoded, responseURL, err := FetchSourceTextWithURLContext(ctx, request)
	if err != nil {
		return nil, responseURL, err
	}
	document, err := goquery.NewDocumentFromReader(strings.NewReader(decoded))
	return document, responseURL, err
}

func FetchText(url, charset string) (string, error) {
	return FetchTextContext(context.Background(), url, charset)
}

func FetchTextContext(ctx context.Context, url, charset string) (string, error) {
	return FetchTextWithHeadersContext(ctx, url, charset, nil)
}

func FetchTextWithHeaders(url, charset string, headers map[string]string) (string, error) {
	return FetchTextWithHeadersContext(context.Background(), url, charset, headers)
}

func FetchTextWithHeadersContext(ctx context.Context, url, charset string, headers map[string]string) (string, error) {
	return FetchTextRequestContext(ctx, http.MethodGet, url, "", charset, headers)
}

func FetchTextRequestContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (string, error) {
	decoded, _, err := FetchTextRequestWithURLContext(ctx, method, url, body, charset, headers)
	return decoded, err
}

func FetchTextRequestWithURLContext(ctx context.Context, method, url, body, charset string, headers map[string]string) (string, string, error) {
	return fetchTextRequestWithURLContext(ctx, method, url, body, charset, headers, 0, "")
}

func FetchSourceTextWithURLContext(ctx context.Context, request SourceRequest) (string, string, error) {
	release, err := acquireSourceRate(ctx, request.SourceKey, request.ConcurrentRate)
	if err != nil {
		return "", request.URL, err
	}
	defer release()
	return fetchTextRequestWithURLContext(
		ctx,
		request.Method,
		request.URL,
		request.Body,
		request.Charset,
		request.Headers,
		request.Retry,
		request.Type,
		request.Proxy,
	)
}

func fetchTextRequestWithURLContext(
	ctx context.Context,
	method string,
	url string,
	body string,
	charset string,
	headers map[string]string,
	retry int,
	responseType string,
	sourceProxy ...string,
) (string, string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if retry < 0 {
		retry = 0
	}
	client := defaultClient
	if len(sourceProxy) > 0 && strings.TrimSpace(sourceProxy[0]) != "" {
		var err error
		client, err = sourceHTTPClient(defaultClient, sourceProxy[0])
		if err != nil {
			return "", url, err
		}
	}

	for attempt := 0; attempt <= retry; attempt++ {
		var requestBody io.Reader
		if body != "" {
			requestBody = strings.NewReader(body)
		}
		request, err := http.NewRequestWithContext(ctx, method, url, requestBody)
		if err != nil {
			return "", url, err
		}
		for name, value := range headers {
			name = strings.TrimSpace(name)
			if name == "" || strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") {
				continue
			}
			request.Header.Set(name, value)
		}
		if request.Header.Get("User-Agent") == "" {
			request.Header.Set("User-Agent", "OpenReader/0.1 (+self-hosted reader)")
		}

		response, err := client.Do(request)
		if err != nil {
			return "", url, err
		}

		responseURL := url
		if response.Request != nil && response.Request.URL != nil {
			responseURL = response.Request.URL.String()
		}
		responseBody, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return "", responseURL, readErr
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			if attempt < retry {
				continue
			}
		}

		if strings.TrimSpace(responseType) != "" {
			return hex.EncodeToString(responseBody), responseURL, nil
		}
		decoded, err := DecodeBody(responseBody, charset)
		if err != nil {
			return "", responseURL, err
		}
		return decoded, responseURL, nil
	}
	return "", url, nil
}

func acquireSourceRate(ctx context.Context, sourceKey, rate string) (func(), error) {
	sourceKey = strings.TrimSpace(sourceKey)
	rate = strings.TrimSpace(rate)
	if sourceKey == "" || rate == "" {
		return func() {}, nil
	}
	key := sourceKey + "\n" + rate
	value, _ := sourceRateLimiters.LoadOrStore(key, &sourceRateLimiter{
		serial: make(chan struct{}, 1),
	})
	limiter := value.(*sourceRateLimiter)

	if countText, windowText, found := strings.Cut(rate, "/"); found {
		count, countErr := strconv.Atoi(strings.TrimSpace(countText))
		windowMS, windowErr := strconv.Atoi(strings.TrimSpace(windowText))
		if countErr != nil || windowErr != nil || count <= 0 || windowMS <= 0 {
			return func() {}, nil
		}
		window := time.Duration(windowMS) * time.Millisecond
		for {
			limiter.mu.Lock()
			now := time.Now()
			if limiter.windowFrom.IsZero() || now.Sub(limiter.windowFrom) >= window {
				limiter.windowFrom = now
				limiter.windowUsed = 0
			}
			if limiter.windowUsed < count {
				limiter.windowUsed++
				limiter.mu.Unlock()
				return func() {}, nil
			}
			wait := window - now.Sub(limiter.windowFrom)
			limiter.mu.Unlock()
			if err := waitSourceRate(ctx, wait); err != nil {
				return func() {}, err
			}
		}
	}

	delayMS, err := strconv.Atoi(rate)
	if err != nil || delayMS <= 0 {
		return func() {}, nil
	}
	select {
	case limiter.serial <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
	release := func() { <-limiter.serial }
	limiter.mu.Lock()
	wait := time.Duration(delayMS)*time.Millisecond - time.Since(limiter.lastStart)
	limiter.mu.Unlock()
	if err := waitSourceRate(ctx, wait); err != nil {
		release()
		return func() {}, err
	}
	limiter.mu.Lock()
	limiter.lastStart = time.Now()
	limiter.mu.Unlock()
	return release, nil
}

func waitSourceRate(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sourceHTTPClient(base *http.Client, value string) (*http.Client, error) {
	match := sourceProxyPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 6 {
		return nil, fmt.Errorf("invalid source proxy %q", value)
	}
	port, err := strconv.Atoi(match[3])
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid source proxy port %q", match[3])
	}
	transport, err := cloneHTTPTransport(base.Transport)
	if err != nil {
		return nil, err
	}
	address := match[2] + ":" + match[3]
	switch match[1] {
	case "http":
		proxyURL := &url.URL{Scheme: "http", Host: address}
		if match[4] != "" || match[5] != "" {
			proxyURL.User = url.UserPassword(match[4], match[5])
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	default:
		var auth *proxy.Auth
		if match[4] != "" || match[5] != "" {
			auth = &proxy.Auth{User: match[4], Password: match[5]}
		}
		dialer, dialErr := proxy.SOCKS5("tcp", address, auth, proxy.Direct)
		if dialErr != nil {
			return nil, dialErr
		}
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
				return contextDialer.DialContext(ctx, network, address)
			}
			return dialer.Dial(network, address)
		}
	}
	client := *base
	client.Transport = transport
	return &client, nil
}

func cloneHTTPTransport(transport http.RoundTripper) (*http.Transport, error) {
	if transport == nil {
		return http.DefaultTransport.(*http.Transport).Clone(), nil
	}
	base, ok := transport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("source proxy requires an HTTP transport")
	}
	return base.Clone(), nil
}

func DecodeBody(body []byte, charset string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(charset))
	if normalized == "" || normalized == "utf-8" || normalized == "utf8" || normalized == "escape" {
		return string(body), nil
	}

	encoding, err := htmlindex.Get(normalized)
	if err != nil {
		return string(body), nil
	}
	reader := transform.NewReader(bytes.NewReader(body), encoding.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
