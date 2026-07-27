package coverimage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"openreader/backend/services/assets"
)

type lookupIPFunc func(context.Context, string) ([]net.IP, error)

type requestPolicy struct {
	LookupIP     lookupIPFunc
	MaxRedirects int
}

func (policy requestPolicy) CheckRedirect(request *http.Request, via []*http.Request) error {
	if len(via) > policy.MaxRedirects {
		return ErrUnavailable
	}
	return policy.validateURL(request.Context(), request.URL)
}

func (policy requestPolicy) validateURL(ctx context.Context, parsed *url.URL) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return ErrUnsafeURL
	}
	ips, err := policy.resolve(ctx, parsed.Hostname())
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
	ips, err := policy.resolve(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, ErrUnsafeURL
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

var blockedNetworks = mustNetworks(
	"0.0.0.0/8",
	"100.64.0.0/10",
	"192.0.0.0/24",
	"198.18.0.0/15",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
)

func mustNetworks(values ...string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err == nil {
			networks = append(networks, network)
		}
	}
	return networks
}

func fetchImage(ctx context.Context, client *http.Client, policy requestPolicy, rawURL string, maxBytes int64) ([]byte, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || policy.validateURL(ctx, parsed) != nil {
		return nil, "", ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", ErrUnsafeURL
	}
	request.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif,image/bmp;q=0.9,*/*;q=0.1")
	request.Header.Set("User-Agent", "OpenReader/0.1 (+self-hosted reader)")
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", ErrUnavailable
	}
	if maxBytes <= 0 || response.ContentLength > maxBytes {
		return nil, "", ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", ErrUnavailable
	}
	if int64(len(data)) > maxBytes {
		return nil, "", ErrUnavailable
	}
	contentType, ok := detectImageType(data)
	if !ok || !validImageBytes(data, contentType) {
		return nil, "", ErrUnavailable
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
	brand := string(data[8:12])
	return brand == "avif" || brand == "avis"
}

func validImageBytes(data []byte, contentType string) bool {
	extension := ""
	switch contentType {
	case "image/png":
		extension = ".png"
	case "image/jpeg":
		extension = ".jpg"
	case "image/gif":
		extension = ".gif"
	case "image/webp":
		extension = ".webp"
	case "image/bmp":
		return validBMP(data)
	case "image/avif":
		return validAVIF(data)
	default:
		return false
	}
	return assets.ValidateUpload(bytes.NewReader(data), int64(len(data)), "cover", extension) == nil
}

func validBMP(data []byte) bool {
	if len(data) < 54 || string(data[:2]) != "BM" {
		return false
	}
	fileSize := int64(binary.LittleEndian.Uint32(data[2:6]))
	pixelOffset := int64(binary.LittleEndian.Uint32(data[10:14]))
	dibSize := int64(binary.LittleEndian.Uint32(data[14:18]))
	if fileSize != int64(len(data)) || pixelOffset < 18 || pixelOffset >= fileSize ||
		dibSize < 40 || 14+dibSize > fileSize {
		return false
	}
	width := int64(int32(binary.LittleEndian.Uint32(data[18:22])))
	height := int64(int32(binary.LittleEndian.Uint32(data[22:26])))
	if width < 0 {
		width = -width
	}
	if height < 0 {
		height = -height
	}
	return validDimensions(width, height)
}

func validAVIF(data []byte) bool {
	if len(data) < 24 {
		return false
	}
	var foundFTYP, foundBrand, foundMeta, foundMedia bool
	for offset := 0; offset+8 <= len(data); {
		size := int64(binary.BigEndian.Uint32(data[offset : offset+4]))
		boxType := string(data[offset+4 : offset+8])
		headerSize := int64(8)
		if size == 1 {
			if offset+16 > len(data) {
				return false
			}
			size = int64(binary.BigEndian.Uint64(data[offset+8 : offset+16]))
			headerSize = 16
		} else if size == 0 {
			size = int64(len(data) - offset)
		}
		if size < headerSize || size > int64(len(data)-offset) {
			return false
		}
		box := data[offset : offset+int(size)]
		switch boxType {
		case "ftyp":
			foundFTYP = true
			for index := int(headerSize); index+4 <= len(box); index += 4 {
				brand := string(box[index : index+4])
				if brand == "avif" || brand == "avis" {
					foundBrand = true
				}
			}
		case "meta":
			foundMeta = size > headerSize+4
		case "mdat":
			foundMedia = size > headerSize
		}
		offset += int(size)
		if offset == len(data) {
			break
		}
	}
	return foundFTYP && foundBrand && foundMeta && foundMedia
}

func validDimensions(width, height int64) bool {
	return width > 0 && height > 0 && width <= 32_768 && height <= 32_768 &&
		width*height <= 64_000_000
}

func normalizeRemoteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	if raw == "" || len(raw) > 4096 {
		return "", ErrUnsafeURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		strings.TrimSpace(parsed.Hostname()) == "" {
		return "", ErrUnsafeURL
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && forbiddenIP(ip) {
		return "", ErrUnsafeURL
	}
	if parsed.Port() != "" {
		if _, err := net.LookupPort("tcp", parsed.Port()); err != nil {
			return "", ErrUnsafeURL
		}
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func unavailableError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w", ErrUnavailable)
}
