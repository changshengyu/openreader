package chapterimage

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"openreader/backend/models"
)

type lookupIPFunc func(context.Context, string) ([]net.IP, error)

type requestPolicy struct {
	TrustedHosts      map[string]struct{}
	CredentialOrigins map[string]struct{}
	Headers           map[string]string
	Referer           string
	LookupIP          lookupIPFunc
	MaxRedirects      int
}

func (policy requestPolicy) CheckRedirect(request *http.Request, via []*http.Request) error {
	if len(via) > policy.MaxRedirects {
		return ErrImageLimit
	}
	if err := policy.validateURL(request.Context(), request.URL); err != nil {
		return err
	}
	applyRequestHeaders(request, policy)
	return nil
}

func (policy requestPolicy) validateURL(ctx context.Context, parsed *url.URL) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil {
		return ErrUnsafeURL
	}
	host := normalizeHost(parsed.Hostname())
	if host == "" {
		return ErrUnsafeURL
	}
	if _, trusted := policy.TrustedHosts[host]; trusted {
		return nil
	}
	ips, err := policy.resolve(ctx, host)
	if err != nil || len(ips) == 0 {
		return ErrUnsafeURL
	}
	for _, ip := range ips {
		if forbiddenIP(ip) {
			return ErrUnsafeURL
		}
	}
	return nil
}

func (policy requestPolicy) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if policy.LookupIP != nil {
		return policy.LookupIP(ctx, host)
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, address.IP)
	}
	return ips, nil
}

func (policy requestPolicy) allowedDialIPs(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := policy.resolve(ctx, normalizeHost(host))
	if err != nil || len(ips) == 0 {
		return nil, ErrUnsafeURL
	}
	if _, trusted := policy.TrustedHosts[normalizeHost(host)]; trusted {
		return ips, nil
	}
	for _, ip := range ips {
		if forbiddenIP(ip) {
			return nil, ErrUnsafeURL
		}
	}
	return ips, nil
}

func defaultClientForPolicy(policy requestPolicy, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrUnsafeURL
		}
		ips, err := policy.allowedDialIPs(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = ErrUnsafeURL
		}
		return nil, lastErr
	}
	transport.ResponseHeaderTimeout = timeout
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: policy.CheckRedirect}
}

func buildRequestPolicy(source models.BookSource, book models.Book, chapter models.Chapter, lookup lookupIPFunc, redirects int) requestPolicy {
	trusted := make(map[string]struct{})
	credentialOrigins := make(map[string]struct{})
	for _, value := range []string{source.BaseURL, book.URL, chapter.URL} {
		if parsed, err := url.Parse(strings.TrimSpace(value)); err == nil {
			if host := normalizeHost(parsed.Hostname()); host != "" {
				trusted[host] = struct{}{}
			}
			if origin := normalizeOrigin(parsed); origin != "" {
				credentialOrigins[origin] = struct{}{}
			}
		}
	}
	rules, _ := source.ParsedRules()
	return requestPolicy{
		TrustedHosts:      trusted,
		CredentialOrigins: credentialOrigins,
		Headers:           rules.Headers,
		Referer:           strings.TrimSpace(chapter.URL),
		LookupIP:          lookup,
		MaxRedirects:      redirects,
	}
}

func applyRequestHeaders(request *http.Request, policy requestPolicy) {
	request.Header = make(http.Header)
	_, credentialOrigin := policy.CredentialOrigins[normalizeOrigin(request.URL)]
	referer := ""
	for name, value := range policy.Headers {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || strings.EqualFold(trimmed, "Host") ||
			strings.EqualFold(trimmed, "Content-Length") || strings.EqualFold(trimmed, "proxy") {
			continue
		}
		if !credentialOrigin && !safeCrossOriginHeader(trimmed) {
			continue
		}
		if strings.EqualFold(trimmed, "Referer") {
			referer = value
			continue
		}
		request.Header.Set(trimmed, value)
	}
	if strings.TrimSpace(referer) == "" {
		referer = policy.Referer
	}
	if referer = sanitizedReferer(referer, credentialOrigin); referer != "" {
		request.Header.Set("Referer", referer)
	}
	if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif,image/bmp;q=0.9,*/*;q=0.1")
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", "OpenReader/0.1 (+self-hosted reader)")
	}
}

func safeCrossOriginHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "accept", "accept-language", "cache-control", "pragma", "referer", "user-agent":
		return true
	default:
		return false
	}
}

func normalizeHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func normalizeOrigin(parsed *url.URL) string {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return ""
	}
	host := normalizeHost(parsed.Hostname())
	if host == "" {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(parsed.Scheme) + "://" + net.JoinHostPort(host, port)
}

func sanitizedReferer(raw string, preservePath bool) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || normalizeOrigin(parsed) == "" {
		return ""
	}
	parsed.User = nil
	parsed.Fragment = ""
	if !preservePath {
		parsed.Path = "/"
		parsed.RawPath = ""
		parsed.RawQuery = ""
		parsed.ForceQuery = false
	}
	return parsed.String()
}

var blockedNetworks = mustNetworks(
	"0.0.0.0/8",
	"100.64.0.0/10",
	"192.0.0.0/24",
	"198.18.0.0/15",
	"240.0.0.0/4",
	"::/128",
	"2001:db8::/32",
)

func mustNetworks(values ...string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			panic(err)
		}
		networks = append(networks, network)
	}
	return networks
}

func forbiddenIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, network := range blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func fetchImageBytes(ctx context.Context, client *http.Client, policy requestPolicy, rawURL string, maxBytes int64) ([]byte, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || policy.validateURL(ctx, parsed) != nil {
		return nil, "", ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", ErrUnsafeURL
	}
	applyRequestHeaders(request, policy)
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", fmt.Errorf("chapter image request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("chapter image response status %d", response.StatusCode)
	}
	if maxBytes <= 0 {
		return nil, "", ErrImageLimit
	}
	if response.ContentLength > maxBytes {
		return nil, "", ErrImageLimit
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", fmt.Errorf("read chapter image response")
	}
	if int64(len(data)) > maxBytes {
		return nil, "", ErrImageLimit
	}
	contentType, ok := detectImageType(data)
	if !ok {
		return nil, "", ErrUnsupportedImage
	}
	return data, contentType, nil
}

func detectImageType(data []byte) (string, bool) {
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png", true
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg", true
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "image/gif", true
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp", true
	case len(data) >= 2 && string(data[:2]) == "BM":
		return "image/bmp", true
	case isAVIF(data):
		return "image/avif", true
	default:
		return "", false
	}
}

func isAVIF(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	limit := len(data)
	if limit > 32 {
		limit = 32
	}
	brands := string(data[8:limit])
	return strings.Contains(brands, "avif") || strings.Contains(brands, "avis")
}
