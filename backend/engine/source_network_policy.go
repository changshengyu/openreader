package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

const (
	sourceNetworkAllowlistMaxEntries = 256
	sourceNetworkAllowlistMaxBytes   = 16 * 1024
)

var ErrUnsafeSourceNetwork = errors.New("unsafe source network target")

type sourceLookupNetIPFunc func(context.Context, string, string) ([]netip.Addr, error)
type sourceDialContextFunc func(context.Context, string, string) (net.Conn, error)

type sourceNetworkPolicy struct {
	hosts    map[string]struct{}
	prefixes []netip.Prefix
}

type sourceNetworkHooks struct {
	lookup sourceLookupNetIPFunc
	dial   sourceDialContextFunc
}

type sourceNetworkRuntime struct {
	policy *sourceNetworkPolicy
	lookup sourceLookupNetIPFunc
	dial   sourceDialContextFunc
}

var configuredSourceNetworkPolicy atomic.Pointer[sourceNetworkPolicy]
var configuredSourceNetworkHooks atomic.Pointer[sourceNetworkHooks]

func parseSourceNetworkPolicy(raw string) (*sourceNetworkPolicy, error) {
	if len(raw) > sourceNetworkAllowlistMaxBytes {
		return nil, errors.New("source network allowlist exceeds 16384 bytes")
	}
	policy := &sourceNetworkPolicy{hosts: make(map[string]struct{})}
	prefixSeen := make(map[string]struct{})
	entryCount := 0
	for index, part := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		entryCount++
		if entryCount > sourceNetworkAllowlistMaxEntries {
			return nil, errors.New("source network allowlist exceeds 256 entries")
		}
		entryError := func() (*sourceNetworkPolicy, error) {
			return nil, fmt.Errorf("invalid source network allowlist entry %d", index+1)
		}

		if strings.Contains(entry, "/") {
			prefix, err := netip.ParsePrefix(entry)
			if err != nil || !prefix.Addr().IsValid() || prefix.Addr().Zone() != "" {
				return entryError()
			}
			prefix, err = normalizeSourcePrefix(prefix)
			if err != nil {
				return entryError()
			}
			key := prefix.String()
			if _, exists := prefixSeen[key]; !exists {
				prefixSeen[key] = struct{}{}
				policy.prefixes = append(policy.prefixes, prefix)
			}
			continue
		}

		if address, err := netip.ParseAddr(entry); err == nil {
			if address.Zone() != "" {
				return entryError()
			}
			address = address.Unmap()
			bits := 128
			if address.Is4() {
				bits = 32
			}
			prefix := netip.PrefixFrom(address, bits)
			key := prefix.String()
			if _, exists := prefixSeen[key]; !exists {
				prefixSeen[key] = struct{}{}
				policy.prefixes = append(policy.prefixes, prefix)
			}
			continue
		}

		host, err := canonicalSourceDNSName(entry)
		if err != nil {
			return entryError()
		}
		policy.hosts[host] = struct{}{}
	}
	return policy, nil
}

func normalizeSourcePrefix(prefix netip.Prefix) (netip.Prefix, error) {
	address := prefix.Addr()
	if address.Is4In6() {
		if prefix.Bits() < 96 {
			return netip.Prefix{}, ErrUnsafeSourceNetwork
		}
		address = address.Unmap()
		prefix = netip.PrefixFrom(address, prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}

func canonicalSourceDNSName(value string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if host == "" || len(host) > 253 {
		return "", ErrUnsafeSourceNetwork
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrUnsafeSourceNetwork
		}
		for _, character := range []byte(label) {
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", ErrUnsafeSourceNetwork
		}
	}
	return host, nil
}

func canonicalSourceNetworkHost(value string) (string, netip.Addr, bool, error) {
	trimmed := strings.TrimSpace(value)
	if address, err := netip.ParseAddr(trimmed); err == nil {
		if address.Zone() != "" {
			return "", netip.Addr{}, false, ErrUnsafeSourceNetwork
		}
		address = address.Unmap()
		return address.String(), address, true, nil
	}
	host, err := canonicalSourceDNSName(trimmed)
	return host, netip.Addr{}, false, err
}

func (policy *sourceNetworkPolicy) allowsAddress(host string, address netip.Addr) bool {
	if policy == nil || !address.IsValid() {
		return false
	}
	canonicalHost, _, _, err := canonicalSourceNetworkHost(host)
	if err == nil {
		if _, allowed := policy.hosts[canonicalHost]; allowed {
			return true
		}
	}
	address = address.Unmap()
	for _, prefix := range policy.prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func ConfigureSourceNetworkPolicy(raw string) (func(), error) {
	policy, err := parseSourceNetworkPolicy(raw)
	if err != nil {
		return nil, err
	}
	installed := policy
	previous := configuredSourceNetworkPolicy.Swap(installed)
	closeSourceNetworkIdleConnections()
	var restored atomic.Bool
	return func() {
		if !restored.CompareAndSwap(false, true) {
			return
		}
		if configuredSourceNetworkPolicy.CompareAndSwap(installed, previous) {
			closeSourceNetworkIdleConnections()
		}
	}, nil
}

func currentSourceNetworkPolicy() *sourceNetworkPolicy {
	if policy := configuredSourceNetworkPolicy.Load(); policy != nil {
		return policy
	}
	policy, _ := parseSourceNetworkPolicy("")
	if configuredSourceNetworkPolicy.CompareAndSwap(nil, policy) {
		return policy
	}
	return configuredSourceNetworkPolicy.Load()
}

func defaultSourceNetworkHooks() *sourceNetworkHooks {
	dialer := &net.Dialer{KeepAlive: 30 * time.Second}
	return &sourceNetworkHooks{
		lookup: net.DefaultResolver.LookupNetIP,
		dial:   dialer.DialContext,
	}
}

func currentSourceNetworkRuntime() sourceNetworkRuntime {
	hooks := configuredSourceNetworkHooks.Load()
	if hooks == nil {
		hooks = defaultSourceNetworkHooks()
	}
	return sourceNetworkRuntime{
		policy: currentSourceNetworkPolicy(),
		lookup: hooks.lookup,
		dial:   hooks.dial,
	}
}

func setSourceNetworkHooksForTesting(lookup sourceLookupNetIPFunc, dial sourceDialContextFunc) func() {
	defaults := defaultSourceNetworkHooks()
	if lookup == nil {
		lookup = defaults.lookup
	}
	if dial == nil {
		dial = defaults.dial
	}
	installed := &sourceNetworkHooks{lookup: lookup, dial: dial}
	previous := configuredSourceNetworkHooks.Swap(installed)
	closeSourceNetworkIdleConnections()
	var restored atomic.Bool
	return func() {
		if !restored.CompareAndSwap(false, true) {
			return
		}
		if configuredSourceNetworkHooks.CompareAndSwap(installed, previous) {
			closeSourceNetworkIdleConnections()
		}
	}
}

func (runtime sourceNetworkRuntime) resolveAllowed(ctx context.Context, host string) ([]netip.Addr, error) {
	canonicalHost, literal, isLiteral, err := canonicalSourceNetworkHost(host)
	if err != nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}
	var addresses []netip.Addr
	if isLiteral {
		addresses = []netip.Addr{literal}
	} else {
		lookup := runtime.lookup
		if lookup == nil {
			lookup = net.DefaultResolver.LookupNetIP
		}
		addresses, err = lookup(ctx, "ip", canonicalHost)
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				err = ctx.Err()
			}
			return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
		}
	}
	if len(addresses) == 0 {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}

	allowed := make([]netip.Addr, 0, len(addresses))
	seen := make(map[netip.Addr]struct{})
	for _, address := range addresses {
		if !address.IsValid() || address.Zone() != "" {
			return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
		}
		address = address.Unmap()
		if sourceAddressForbidden(address) && (runtime.policy == nil || !runtime.policy.allowsAddress(canonicalHost, address)) {
			return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
		}
		if _, exists := seen[address]; !exists {
			seen[address] = struct{}{}
			allowed = append(allowed, address)
		}
	}
	return allowed, nil
}

func (runtime sourceNetworkRuntime) validateURL(ctx context.Context, parsed *url.URL) error {
	if parsed == nil || parsed.User != nil || parsed.Host == "" {
		return redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}
	if _, err := sourceNetworkPort(parsed); err != nil {
		return err
	}
	_, err := runtime.resolveAllowed(ctx, parsed.Hostname())
	return err
}

func (runtime sourceNetworkRuntime) dialAllowed(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}
	addresses, err := runtime.resolveAllowed(ctx, host)
	if err != nil {
		return nil, err
	}
	dial := runtime.dial
	if dial == nil {
		dial = defaultSourceNetworkHooks().dial
	}
	var lastErr error
	for _, candidate := range addresses {
		if network == "tcp4" && !candidate.Is4() {
			continue
		}
		if network == "tcp6" && !candidate.Is6() {
			continue
		}
		connection, dialErr := dial(ctx, network, net.JoinHostPort(candidate.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = ErrUnsafeSourceNetwork
	}
	return nil, redactSourceFetchError(errSourceTransport, lastErr)
}

func sourceAddressForbidden(address netip.Addr) bool {
	if !address.IsValid() {
		return true
	}
	address = address.Unmap()
	if address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsInterfaceLocalMulticast() || address.IsMulticast() {
		return true
	}
	for _, prefix := range sourceBlockedNetworkPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var sourceBlockedNetworkPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func sourceNetworkPort(parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}
	port := parsed.Port()
	if port == "" {
		if strings.EqualFold(parsed.Scheme, "https") {
			return "443", nil
		}
		return "80", nil
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}
	return port, nil
}

type sourceValidatedRoundTripper struct {
	transport     *http.Transport
	proxyEndpoint string
}

func newDefaultSourceRoundTripper() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	prepareSourceNetworkTransport(transport)
	return &sourceValidatedRoundTripper{transport: transport}
}

func prepareSourceNetworkTransport(transport *http.Transport) {
	transport.Proxy = nil
	transport.DialTLSContext = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return currentSourceNetworkRuntime().dialAllowed(ctx, network, address)
	}
}

func (roundTripper *sourceValidatedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}
	runtime := currentSourceNetworkRuntime()
	if err := runtime.validateURL(request.Context(), request.URL); err != nil {
		return nil, err
	}
	if roundTripper.proxyEndpoint != "" {
		if _, err := runtime.resolveAllowed(request.Context(), roundTripper.proxyEndpoint); err != nil {
			return nil, err
		}
	}
	return roundTripper.transport.RoundTrip(request)
}

func (roundTripper *sourceValidatedRoundTripper) CloseIdleConnections() {
	if roundTripper != nil && roundTripper.transport != nil {
		roundTripper.transport.CloseIdleConnections()
	}
}

type sourceHTTPProxyRoundTripper struct {
	template      *http.Transport
	proxyURL      *url.URL
	endpointHost  string
	transportLock sync.Mutex
	transports    map[string]*http.Transport
}

func (roundTripper *sourceHTTPProxyRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, nil)
	}
	runtime := currentSourceNetworkRuntime()
	if _, err := runtime.resolveAllowed(request.Context(), roundTripper.endpointHost); err != nil {
		return nil, err
	}
	targetAddresses, err := runtime.resolveAllowed(request.Context(), request.URL.Hostname())
	if err != nil {
		return nil, err
	}
	port, err := sourceNetworkPort(request.URL)
	if err != nil {
		return nil, err
	}
	targetAddress := targetAddresses[0]
	originalHost := request.URL.Host
	serverName, _, _, err := canonicalSourceNetworkHost(request.URL.Hostname())
	if err != nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}

	clonedRequest := request.Clone(request.Context())
	clonedURL := *request.URL
	clonedURL.Host = net.JoinHostPort(targetAddress.String(), port)
	if strings.EqualFold(clonedURL.Scheme, "http") {
		path := clonedURL.EscapedPath()
		if path == "" {
			path = "/"
		}
		// Request.WriteProxy normally reuses Request.Host for both the
		// absolute-form authority and Host header. An opaque absolute URI
		// keeps the pinned IP in the request target while preserving the
		// original virtual-host header.
		clonedURL.Opaque = "//" + clonedURL.Host + path
	}
	clonedRequest.URL = &clonedURL
	if request.Host != "" {
		clonedRequest.Host = request.Host
	} else {
		clonedRequest.Host = originalHost
	}

	transport := roundTripper.transportFor(request.URL.Scheme, serverName, targetAddress, port)
	response, err := transport.RoundTrip(clonedRequest)
	if response != nil {
		response.Request = request
	}
	return response, err
}

func (roundTripper *sourceHTTPProxyRoundTripper) transportFor(scheme, serverName string, target netip.Addr, port string) *http.Transport {
	key := strings.ToLower(scheme) + "|" + serverName + "|" + target.String() + "|" + port
	roundTripper.transportLock.Lock()
	defer roundTripper.transportLock.Unlock()
	if transport := roundTripper.transports[key]; transport != nil {
		return transport
	}
	transport := roundTripper.template.Clone()
	prepareSourceNetworkTransport(transport)
	transport.Proxy = http.ProxyURL(roundTripper.proxyURL)
	if strings.EqualFold(scheme, "https") {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		} else {
			transport.TLSClientConfig = transport.TLSClientConfig.Clone()
		}
		transport.TLSClientConfig.ServerName = serverName
	}
	if roundTripper.transports == nil {
		roundTripper.transports = make(map[string]*http.Transport)
	}
	roundTripper.transports[key] = transport
	return transport
}

func (roundTripper *sourceHTTPProxyRoundTripper) CloseIdleConnections() {
	if roundTripper == nil {
		return
	}
	roundTripper.transportLock.Lock()
	defer roundTripper.transportLock.Unlock()
	for _, transport := range roundTripper.transports {
		transport.CloseIdleConnections()
	}
}

type sourceProxyEndpointDialer struct{}

func (sourceProxyEndpointDialer) Dial(network, address string) (net.Conn, error) {
	return currentSourceNetworkRuntime().dialAllowed(context.Background(), network, address)
}

func (sourceProxyEndpointDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return currentSourceNetworkRuntime().dialAllowed(ctx, network, address)
}

func dialSourceSOCKS5Context(ctx context.Context, dialer proxy.Dialer, network, targetAddress string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}
	addresses, err := currentSourceNetworkRuntime().resolveAllowed(ctx, host)
	if err != nil {
		return nil, err
	}
	contextDialer, supportsContext := dialer.(proxy.ContextDialer)
	var lastErr error
	for _, address := range addresses {
		pinnedTarget := net.JoinHostPort(address.String(), port)
		var connection net.Conn
		if supportsContext {
			connection, err = contextDialer.DialContext(ctx, network, pinnedTarget)
		} else {
			connection, err = dialer.Dial(network, pinnedTarget)
		}
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	return nil, redactSourceFetchError(errSourceTransport, lastErr)
}

func dialSourceSOCKS4Context(ctx context.Context, proxyAddress, targetAddress, userID string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return nil, redactSourceFetchError(ErrUnsafeSourceNetwork, err)
	}
	addresses, err := currentSourceNetworkRuntime().resolveAllowed(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, address := range addresses {
		if !address.Is4() {
			continue
		}
		connection, dialErr := currentSourceNetworkRuntime().dialAllowed(ctx, "tcp", proxyAddress)
		if dialErr != nil {
			return nil, dialErr
		}
		if handshakeErr := performSOCKS4Handshake(ctx, connection, net.JoinHostPort(address.String(), port), userID); handshakeErr == nil {
			return connection, nil
		} else {
			_ = connection.Close()
			lastErr = handshakeErr
		}
	}
	if lastErr == nil {
		lastErr = ErrUnsafeSourceNetwork
	}
	return nil, redactSourceFetchError(errSourceTransport, lastErr)
}

func closeSourceNetworkIdleConnections() {
	if client := defaultClient.Load(); client != nil {
		client.CloseIdleConnections()
	}
}
