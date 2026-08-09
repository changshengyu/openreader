package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/proxy"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

const (
	defaultSourceRequestTimeout = 15 * time.Second
	defaultMaxSourceResponse    = int64(16 * 1024 * 1024)
	defaultMaxSourceRedirects   = 5
	defaultMaxSourceRetries     = 3
)

var defaultClient atomic.Pointer[http.Client]
var configuredSourceFetchLimits atomic.Pointer[SourceFetchLimits]
var sourceHTTPClientOverrides struct {
	sync.Mutex
	frames []*sourceHTTPClientOverride
}
var sourceProxyPattern = regexp.MustCompile(`^(http|socks4|socks5)://(.+):([0-9]{2,5})(?:@([^@]*)@([^@]*))?$`)
var sourceRateLimiters sync.Map

// ErrSourceRequest marks a failure that occurred while OpenReader was making
// a remote source request. Parser/configuration errors deliberately do not
// carry this marker, so callers do not suppress a source for a bad rule.
var ErrSourceRequest = errors.New("source request failed")

var (
	ErrUnsafeSourceURL     = errors.New("unsafe source URL")
	ErrSourceResponseLimit = errors.New("source response exceeds limit")
	ErrSourceRedirectLimit = errors.New("source redirect limit exceeded")
	errSourceTransport     = errors.New("source transport failed")
	errSourceResponseRead  = errors.New("failed to read source response")
)

// SourceFetchLimits configures the shared book-source and RSS transport. The
// values are process-local runtime policy and never enter persistent data.
type SourceFetchLimits struct {
	Timeout          time.Duration
	MaxResponseBytes int64
	MaxRedirects     int
	MaxRetries       int
}

type redactedSourceFetchError struct {
	kind  error
	cause error
}

type sourceHTTPClientOverride struct {
	client *http.Client
	active bool
}

func (err *redactedSourceFetchError) Error() string {
	return err.kind.Error()
}

func (err *redactedSourceFetchError) Unwrap() []error {
	if err.cause == nil || err.cause == err.kind {
		return []error{err.kind}
	}
	return []error{err.kind, err.cause}
}

func init() {
	defaultClient.Store(defaultSourceHTTPClient())
	limits := normalizeSourceFetchLimits(SourceFetchLimits{})
	configuredSourceFetchLimits.Store(&limits)
}

func defaultSourceHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultSourceRequestTimeout}
}

func normalizeSourceFetchLimits(limits SourceFetchLimits) SourceFetchLimits {
	if limits.Timeout <= 0 {
		limits.Timeout = defaultSourceRequestTimeout
	}
	if limits.MaxResponseBytes <= 0 {
		limits.MaxResponseBytes = defaultMaxSourceResponse
	}
	if limits.MaxRedirects <= 0 {
		limits.MaxRedirects = defaultMaxSourceRedirects
	}
	if limits.MaxRetries <= 0 {
		limits.MaxRetries = defaultMaxSourceRetries
	}
	return limits
}

func currentSourceFetchLimits() SourceFetchLimits {
	if limits := configuredSourceFetchLimits.Load(); limits != nil {
		return *limits
	}
	return normalizeSourceFetchLimits(SourceFetchLimits{})
}

// ConfigureSourceFetchLimits installs the runtime policy. The restore closure
// is useful for focused tests; production calls this once during startup.
func ConfigureSourceFetchLimits(limits SourceFetchLimits) func() {
	normalized := normalizeSourceFetchLimits(limits)
	installed := &normalized
	previous := configuredSourceFetchLimits.Swap(installed)
	return func() {
		configuredSourceFetchLimits.CompareAndSwap(installed, previous)
	}
}

type sourceRateLimiter struct {
	serial     chan struct{}
	mu         sync.Mutex
	lastStart  time.Time
	windowFrom time.Time
	windowUsed int
}

func SetHTTPClient(client *http.Client) func() {
	if client == nil {
		client = defaultSourceHTTPClient()
	}
	frame := &sourceHTTPClientOverride{client: client, active: true}
	sourceHTTPClientOverrides.Lock()
	sourceHTTPClientOverrides.frames = append(sourceHTTPClientOverrides.frames, frame)
	defaultClient.Store(client)
	sourceHTTPClientOverrides.Unlock()
	var restored atomic.Bool
	return func() {
		if !restored.CompareAndSwap(false, true) {
			return
		}
		sourceHTTPClientOverrides.Lock()
		frame.active = false
		frames := sourceHTTPClientOverrides.frames
		for len(frames) > 0 && !frames[len(frames)-1].active {
			frames = frames[:len(frames)-1]
		}
		sourceHTTPClientOverrides.frames = frames
		current := defaultSourceHTTPClient()
		for index := len(frames) - 1; index >= 0; index-- {
			if frames[index].active {
				current = frames[index].client
				break
			}
		}
		defaultClient.Store(current)
		sourceHTTPClientOverrides.Unlock()
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
		return "", request.URL, sourceRequestError(err)
	}
	defer release()
	text, responseURL, err := fetchTextRequestWithURLContext(
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
	if err != nil {
		return "", responseURL, sourceRequestError(err)
	}
	return text, responseURL, nil
}

func sourceRequestError(err error) error {
	if err == nil || errors.Is(err, ErrSourceRequest) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrSourceRequest, err)
}

// IsSourceRequestError reports whether err came from a remote source fetch,
// rather than a local rule/configuration/parser failure.
func IsSourceRequestError(err error) bool {
	return errors.Is(err, ErrSourceRequest)
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
	normalizedURL, err := normalizeSourceRequestURL(url)
	if err != nil {
		return "", "", err
	}
	url = normalizedURL
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if retry < 0 {
		retry = 0
	}
	limits := currentSourceFetchLimits()
	if retry > limits.MaxRetries {
		retry = limits.MaxRetries
	}
	baseClient := defaultClient.Load()
	if baseClient == nil {
		baseClient = defaultSourceHTTPClient()
	}
	client := baseClient
	if len(sourceProxy) > 0 && strings.TrimSpace(sourceProxy[0]) != "" {
		client, err = sourceHTTPClient(baseClient, sourceProxy[0])
		if err != nil {
			return "", "", redactSourceFetchError(errSourceTransport, err)
		}
	}
	client = sourceFetchClient(client, limits)

	for attempt := 0; attempt <= retry; attempt++ {
		var requestBody io.Reader
		if body != "" {
			requestBody = strings.NewReader(body)
		}
		request, err := http.NewRequestWithContext(ctx, method, url, requestBody)
		if err != nil {
			return "", "", redactSourceFetchError(ErrUnsafeSourceURL, err)
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
			return "", "", classifySourceClientError(err)
		}

		responseURL := url
		if response.Request != nil && response.Request.URL != nil {
			responseURL = response.Request.URL.String()
		}
		responseBody, readErr := readBoundedSourceResponse(response, limits.MaxResponseBytes)
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

func normalizeSourceRequestURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Host == "" || parsed.Hostname() == "" {
		return "", redactSourceFetchError(ErrUnsafeSourceURL, err)
	}
	parsed.Scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", redactSourceFetchError(ErrUnsafeSourceURL, nil)
	}
	if port := parsed.Port(); port != "" {
		value, parseErr := strconv.Atoi(port)
		if parseErr != nil || value < 1 || value > 65535 {
			return "", redactSourceFetchError(ErrUnsafeSourceURL, parseErr)
		}
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String(), nil
}

func sourceFetchClient(base *http.Client, limits SourceFetchLimits) *http.Client {
	if base == nil {
		base = defaultSourceHTTPClient()
	}
	client := *base
	if client.Timeout <= 0 || client.Timeout > limits.Timeout {
		client.Timeout = limits.Timeout
	}
	previousRedirect := base.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > limits.MaxRedirects {
			return ErrSourceRedirectLimit
		}
		if request == nil || request.URL == nil {
			return ErrUnsafeSourceURL
		}
		normalized, err := normalizeSourceRequestURL(request.URL.String())
		if err != nil {
			return ErrUnsafeSourceURL
		}
		parsed, err := url.Parse(normalized)
		if err != nil {
			return ErrUnsafeSourceURL
		}
		request.URL = parsed
		if len(via) > 0 && !sameSourceOrigin(via[len(via)-1].URL, request.URL) {
			request.Header = safeCrossOriginSourceHeaders(request.Header)
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		return nil
	}
	return &client
}

func sameSourceOrigin(left, right *url.URL) bool {
	return normalizedSourceOrigin(left) != "" && normalizedSourceOrigin(left) == normalizedSourceOrigin(right)
}

func normalizedSourceOrigin(parsed *url.URL) string {
	if parsed == nil || parsed.User != nil {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

func safeCrossOriginSourceHeaders(headers http.Header) http.Header {
	safe := make(http.Header)
	for name, values := range headers {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "accept", "accept-language", "cache-control", "pragma", "user-agent":
			for _, value := range values {
				safe.Add(name, value)
			}
		}
	}
	return safe
}

func readBoundedSourceResponse(response *http.Response, maxBytes int64) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, redactSourceFetchError(errSourceResponseRead, nil)
	}
	defer response.Body.Close()
	if maxBytes <= 0 || response.ContentLength > maxBytes {
		return nil, redactSourceFetchError(ErrSourceResponseLimit, nil)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, classifySourceReadError(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, redactSourceFetchError(ErrSourceResponseLimit, nil)
	}
	return data, nil
}

func classifySourceClientError(err error) error {
	switch {
	case errors.Is(err, ErrSourceRedirectLimit):
		return redactSourceFetchError(ErrSourceRedirectLimit, err)
	case errors.Is(err, ErrUnsafeSourceURL):
		return redactSourceFetchError(ErrUnsafeSourceURL, err)
	default:
		return redactSourceFetchError(errSourceTransport, err)
	}
}

func classifySourceReadError(err error) error {
	return redactSourceFetchError(errSourceResponseRead, err)
}

func redactSourceFetchError(kind, cause error) error {
	if kind == nil {
		kind = errSourceTransport
	}
	return &redactedSourceFetchError{kind: kind, cause: cause}
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
		return nil, errors.New("invalid source proxy")
	}
	port, err := strconv.Atoi(match[3])
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid source proxy port")
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
	case "socks5":
		var auth *proxy.Auth
		if match[4] != "" || match[5] != "" {
			auth = &proxy.Auth{User: match[4], Password: match[5]}
		}
		dialer, dialErr := proxy.SOCKS5("tcp", address, auth, proxy.Direct)
		if dialErr != nil {
			return nil, errors.New("invalid source proxy")
		}
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
				return contextDialer.DialContext(ctx, network, address)
			}
			return dialer.Dial(network, address)
		}
	case "socks4":
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, targetAddress string) (net.Conn, error) {
			return dialSOCKS4Context(ctx, address, targetAddress, match[4])
		}
	}
	client := *base
	client.Transport = transport
	return &client, nil
}

func dialSOCKS4Context(ctx context.Context, proxyAddress, targetAddress, userID string) (net.Conn, error) {
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, err
	}
	if err := performSOCKS4Handshake(ctx, connection, targetAddress, userID); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return connection, nil
}

func performSOCKS4Handshake(ctx context.Context, connection net.Conn, targetAddress, userID string) error {
	host, portText, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return fmt.Errorf("invalid SOCKS4 target %q: %w", targetAddress, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid SOCKS4 target port %q", portText)
	}
	if strings.IndexByte(userID, 0) >= 0 {
		return fmt.Errorf("invalid SOCKS4 user ID")
	}

	request := make([]byte, 0, 9+len(userID)+len(host))
	request = append(request, 0x04, 0x01, byte(port>>8), byte(port))
	ipv4 := net.ParseIP(host).To4()
	if ipv4 != nil {
		request = append(request, ipv4...)
	} else {
		if strings.IndexByte(host, 0) >= 0 {
			return fmt.Errorf("invalid SOCKS4 target host")
		}
		request = append(request, 0x00, 0x00, 0x00, 0x01)
	}
	request = append(request, userID...)
	request = append(request, 0x00)
	if ipv4 == nil {
		request = append(request, host...)
		request = append(request, 0x00)
	}

	cancelDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.SetDeadline(time.Now())
		case <-cancelDone:
		}
	}()
	defer close(cancelDone)
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	if _, err := connection.Write(request); err != nil {
		return err
	}
	response := make([]byte, 8)
	if _, err := io.ReadFull(connection, response); err != nil {
		return err
	}
	if response[0] != 0x00 || response[1] != 0x5a {
		return fmt.Errorf("SOCKS4 proxy rejected connection with code 0x%02x", response[1])
	}
	_ = connection.SetDeadline(time.Time{})
	return nil
}

func cloneHTTPTransport(transport http.RoundTripper) (*http.Transport, error) {
	if transport == nil {
		return http.DefaultTransport.(*http.Transport).Clone(), nil
	}
	base, ok := transport.(*http.Transport)
	if !ok {
		return nil, errors.New("source proxy requires an HTTP transport")
	}
	return base.Clone(), nil
}

func DecodeBody(body []byte, charset string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(charset))
	if normalized == "" || normalized == "auto" {
		if detected := detectHTMLCharset(body); detected != "" {
			return DecodeBody(body, detected)
		}
		decoded, _, err := detectAndDecodeText(body)
		return decoded, err
	}
	if normalized == "utf-8" || normalized == "utf8" || normalized == "escape" {
		return string(bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})), nil
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
