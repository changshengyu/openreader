package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceNetworkAllowlistGrammarAndFailClosedInstall(t *testing.T) {
	policy, err := parseSourceNetworkPolicy(" NAS.Home. , 192.168.50.20 , 192.168.60.0/24, ::ffff:192.168.70.30, fd00:1234::/48, ,")
	if err != nil {
		t.Fatal(err)
	}
	allowed := []struct {
		host string
		addr string
	}{
		{host: "nas.home", addr: "10.0.0.5"},
		{host: "NAS.HOME.", addr: "fd00::5"},
		{host: "other.home", addr: "192.168.50.20"},
		{host: "other.home", addr: "192.168.60.99"},
		{host: "other.home", addr: "192.168.70.30"},
		{host: "other.home", addr: "fd00:1234::99"},
	}
	for _, item := range allowed {
		if !policy.allowsAddress(item.host, netip.MustParseAddr(item.addr)) {
			t.Fatalf("allowlist rejected host=%q addr=%s", item.host, item.addr)
		}
	}
	if policy.allowsAddress("other.home", netip.MustParseAddr("192.168.50.21")) {
		t.Fatal("exact IP allowlist widened to an adjacent address")
	}
	if policy.allowsAddress("other.home", netip.MustParseAddr("10.0.0.5")) {
		t.Fatal("exact hostname allowlist widened to another hostname")
	}

	invalid := []string{
		"*.home",
		"http://nas.home",
		"nas.home:8080",
		"münich.example",
		"[fd00::1]",
		"10.0.0.1/33",
		"bad_label.home",
		"-bad.home",
		"bad-.home",
		strings.Repeat("a", 64) + ".home",
		strings.Repeat("same.example,", sourceNetworkAllowlistMaxEntries+1),
		strings.Repeat("a", sourceNetworkAllowlistMaxBytes+1),
	}
	for _, raw := range invalid {
		t.Run(raw[:min(len(raw), 40)], func(t *testing.T) {
			if _, err := parseSourceNetworkPolicy(raw); err == nil {
				t.Fatalf("invalid allowlist %q was accepted", raw)
			} else if strings.Contains(err.Error(), raw) {
				t.Fatalf("allowlist error leaked the invalid deployment value: %v", err)
			}
		})
	}

	restore, err := ConfigureSourceNetworkPolicy("nas.home")
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	installed := currentSourceNetworkPolicy()
	if _, err := ConfigureSourceNetworkPolicy("*.invalid.internal"); err == nil {
		t.Fatal("invalid runtime allowlist was accepted")
	}
	if currentSourceNetworkPolicy() != installed {
		t.Fatal("invalid allowlist replaced the last valid runtime policy")
	}
}

func TestSourceNetworkPolicyBlocksSpecialUseIPv4AndIPv6(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.169.254",
		"172.16.0.1", "192.0.0.1", "192.0.2.1", "192.168.1.1", "198.18.0.1",
		"198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255",
		"::", "::1", "::ffff:127.0.0.1", "100::1", "64:ff9b::7f00:1",
		"64:ff9b:1::1", "2001:2::1", "2001:db8::1", "2001:10::1", "2001:20::1",
		"2002:7f00:1::", "fc00::1", "fd00:ec2::254", "fe80::1", "ff02::1",
	}
	for _, raw := range blocked {
		if address := netip.MustParseAddr(raw); !sourceAddressForbidden(address) {
			t.Errorf("special-use address %s was allowed", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if address := netip.MustParseAddr(raw); sourceAddressForbidden(address) {
			t.Errorf("public address %s was blocked", raw)
		}
	}
}

func TestSourceNetworkPolicyRejectsMixedDNSUnlessEveryForbiddenAnswerIsAllowed(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("10.0.0.8")}, nil
	}
	strict, err := parseSourceNetworkPolicy("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (sourceNetworkRuntime{policy: strict, lookup: lookup}).resolveAllowed(context.Background(), "mixed.example"); !errors.Is(err, ErrUnsafeSourceNetwork) {
		t.Fatalf("mixed public/private DNS error = %v, want ErrUnsafeSourceNetwork", err)
	}

	for _, raw := range []string{"mixed.example", "10.0.0.8", "10.0.0.0/24"} {
		policy, err := parseSourceNetworkPolicy(raw)
		if err != nil {
			t.Fatal(err)
		}
		addresses, err := (sourceNetworkRuntime{policy: policy, lookup: lookup}).resolveAllowed(context.Background(), "mixed.example")
		if err != nil || len(addresses) != 2 {
			t.Fatalf("allowlist %q mixed DNS addresses=%v err=%v", raw, addresses, err)
		}
	}

	hostOnly, err := parseSourceNetworkPolicy("mixed.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (sourceNetworkRuntime{policy: hostOnly, lookup: lookup}).resolveAllowed(context.Background(), "redirected.example"); !errors.Is(err, ErrUnsafeSourceNetwork) {
		t.Fatalf("exact hostname leaked to redirect host: %v", err)
	}
}

func TestSourceDirectTransportRejectsDNSRebindingAtDial(t *testing.T) {
	installSourceNetworkPolicyForTest(t, "")
	var lookups atomic.Int32
	var dials atomic.Int32
	installSourceNetworkHooksForTest(t,
		func(context.Context, string, string) ([]netip.Addr, error) {
			if lookups.Add(1) == 1 {
				return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("must not dial rebound address")
		},
	)

	request, err := http.NewRequest(http.MethodGet, "http://rebind.example/content", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = defaultSourceHTTPClient().Do(request)
	if !errors.Is(err, ErrUnsafeSourceNetwork) {
		t.Fatalf("DNS rebinding error = %v, want ErrUnsafeSourceNetwork", err)
	}
	if lookups.Load() < 2 || dials.Load() != 0 {
		t.Fatalf("DNS rebinding lookups=%d dials=%d, want >=2/0", lookups.Load(), dials.Load())
	}
}

func TestSourceDirectTransportRejectsRedirectToMetadataBeforeDial(t *testing.T) {
	installSourceNetworkPolicyForTest(t, "")
	var dials atomic.Int32
	serverDone := make(chan error, 1)
	installSourceNetworkHooksForTest(t,
		func(_ context.Context, _ string, host string) ([]netip.Addr, error) {
			if host != "public.example" {
				return nil, fmt.Errorf("unexpected lookup host %q", host)
			}
			return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
		},
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			dials.Add(1)
			if address != "1.1.1.1:80" {
				return nil, fmt.Errorf("unexpected dial address %q", address)
			}
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				if _, err := http.ReadRequest(bufio.NewReader(server)); err != nil {
					serverDone <- err
					return
				}
				_, err := io.WriteString(server, "HTTP/1.1 302 Found\r\nLocation: http://169.254.169.254/latest/meta-data\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				serverDone <- err
			}()
			return client, nil
		},
	)

	request, _ := http.NewRequest(http.MethodGet, "http://public.example/start", nil)
	_, err := defaultSourceHTTPClient().Do(request)
	if !errors.Is(err, ErrUnsafeSourceNetwork) {
		t.Fatalf("metadata redirect error = %v, want ErrUnsafeSourceNetwork", err)
	}
	if dials.Load() != 1 {
		t.Fatalf("metadata redirect made %d dials, want only the public origin dial", dials.Load())
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestSourceDirectTransportAllowsExplicitLANHostAndIgnoresAmbientProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	installSourceNetworkPolicyForTest(t, "nas.home")
	serverDone := make(chan error, 1)
	installSourceNetworkHooksForTest(t,
		func(_ context.Context, _ string, host string) ([]netip.Addr, error) {
			if host != "nas.home" {
				return nil, fmt.Errorf("ambient proxy caused unexpected lookup %q", host)
			}
			return []netip.Addr{netip.MustParseAddr("192.168.50.20")}, nil
		},
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address != "192.168.50.20:8080" {
				return nil, fmt.Errorf("dial address = %q, want pinned LAN target", address)
			}
			client, server := net.Pipe()
			go serveSourceNetworkHTTPPipe(server, http.StatusOK, "ok", "", serverDone)
			return client, nil
		},
	)

	request, _ := http.NewRequest(http.MethodGet, "http://nas.home:8080/books", nil)
	response, err := defaultSourceHTTPClient().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(data) != "ok" {
		t.Fatalf("LAN response=%q err=%v", data, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestSourceHTTPProxyPinsAbsoluteTargetAndPreservesHost(t *testing.T) {
	installSourceNetworkPolicyForTest(t, "")
	proxyDone := make(chan error, 1)
	installSourceNetworkHooksForTest(t,
		testSourceNetworkLookup(map[string][]string{
			"target.example": {"8.8.8.8"},
			"proxy.example":  {"1.1.1.1"},
		}),
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address != "1.1.1.1:18080" {
				return nil, fmt.Errorf("proxy endpoint dial = %q", address)
			}
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				request, err := http.ReadRequest(bufio.NewReader(server))
				if err == nil && request.URL.String() != "http://8.8.8.8:8080/path?q=1" {
					err = fmt.Errorf("proxy absolute target = %q", request.URL.String())
				}
				if err == nil && request.Host != "target.example:8080" {
					err = fmt.Errorf("proxy Host = %q", request.Host)
				}
				if err == nil && !strings.HasPrefix(request.Header.Get("Proxy-Authorization"), "Basic ") {
					err = errors.New("proxy authorization header missing")
				}
				if err == nil {
					_, err = io.WriteString(server, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
				}
				proxyDone <- err
			}()
			return client, nil
		},
	)

	client, err := sourceHTTPClient(defaultSourceHTTPClient(), "http://proxy.example:18080@reader@secret")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://target.example:8080/path?q=1", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.Request == nil || response.Request.URL.Host != "target.example:8080" {
		t.Fatalf("public response URL was not restored: %#v", response.Request)
	}
	if err := <-proxyDone; err != nil {
		t.Fatal(err)
	}
}

func TestSourceHTTPSProxyPinsConnectButPreservesTLSNameAndHost(t *testing.T) {
	certificate, roots := sourceNetworkTLSCertificate(t, "target.example")
	installSourceNetworkPolicyForTest(t, "")
	proxyDone := make(chan error, 1)
	installSourceNetworkHooksForTest(t,
		testSourceNetworkLookup(map[string][]string{
			"target.example": {"8.8.4.4"},
			"proxy.example":  {"1.0.0.1"},
		}),
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address != "1.0.0.1:18081" {
				return nil, fmt.Errorf("HTTPS proxy endpoint dial = %q", address)
			}
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				reader := bufio.NewReader(server)
				connect, err := http.ReadRequest(reader)
				if err == nil && connect.Method != http.MethodConnect {
					err = fmt.Errorf("proxy method = %q", connect.Method)
				}
				if err == nil && connect.RequestURI != "8.8.4.4:443" {
					err = fmt.Errorf("CONNECT target = %q", connect.RequestURI)
				}
				if err == nil {
					_, err = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\n")
				}
				var sni string
				tlsServer := tls.Server(server, &tls.Config{
					Certificates: []tls.Certificate{certificate},
					GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
						sni = hello.ServerName
						return nil, nil
					},
				})
				if err == nil {
					err = tlsServer.Handshake()
				}
				if err == nil && sni != "target.example" {
					err = fmt.Errorf("TLS SNI = %q", sni)
				}
				var inner *http.Request
				if err == nil {
					inner, err = http.ReadRequest(bufio.NewReader(tlsServer))
				}
				if err == nil && inner.Host != "target.example" {
					err = fmt.Errorf("HTTPS Host = %q", inner.Host)
				}
				if err == nil {
					_, err = io.WriteString(tlsServer, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
				}
				proxyDone <- err
			}()
			return client, nil
		},
	)

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.Proxy = nil
	baseTransport.TLSClientConfig = &tls.Config{RootCAs: roots}
	client, err := sourceHTTPClient(&http.Client{Transport: baseTransport, Timeout: time.Second}, "http://proxy.example:18081")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://target.example/secure", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err := <-proxyDone; err != nil {
		t.Fatal(err)
	}
}

func TestSourceSOCKSProxiesSendPinnedTargetIPs(t *testing.T) {
	for _, version := range []string{"socks4", "socks5"} {
		t.Run(version, func(t *testing.T) {
			installSourceNetworkPolicyForTest(t, "")
			proxyDone := make(chan error, 1)
			installSourceNetworkHooksForTest(t,
				testSourceNetworkLookup(map[string][]string{
					"target.example": {"8.8.8.8"},
					"proxy.example":  {"1.1.1.1"},
				}),
				func(_ context.Context, _ string, address string) (net.Conn, error) {
					if address != "1.1.1.1:1080" {
						return nil, fmt.Errorf("SOCKS endpoint dial = %q", address)
					}
					client, server := net.Pipe()
					go serveSourceSOCKSProxy(server, version, proxyDone)
					return client, nil
				},
			)
			client, err := sourceHTTPClient(defaultSourceHTTPClient(), version+"://proxy.example:1080")
			if err != nil {
				t.Fatal(err)
			}
			request, _ := http.NewRequest(http.MethodGet, "http://target.example:8080/path", nil)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if err := <-proxyDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceProxyEndpointAndTargetUseIndependentNetworkChecks(t *testing.T) {
	for _, test := range []struct {
		name       string
		proxyIP    string
		targetIP   string
		allowlist  string
		wantDialed bool
	}{
		{name: "private target rejected", proxyIP: "1.1.1.1", targetIP: "127.0.0.1"},
		{name: "private endpoint rejected", proxyIP: "10.0.0.2", targetIP: "8.8.8.8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			installSourceNetworkPolicyForTest(t, test.allowlist)
			var dials atomic.Int32
			installSourceNetworkHooksForTest(t,
				testSourceNetworkLookup(map[string][]string{
					"target.example": {test.targetIP},
					"proxy.example":  {test.proxyIP},
				}),
				func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, errors.New("must not dial an unsafe proxy path")
				},
			)
			client, err := sourceHTTPClient(defaultSourceHTTPClient(), "http://proxy.example:18080")
			if err != nil {
				t.Fatal(err)
			}
			request, _ := http.NewRequest(http.MethodGet, "http://target.example/content", nil)
			_, err = client.Do(request)
			if !errors.Is(err, ErrUnsafeSourceNetwork) {
				t.Fatalf("unsafe proxy path error = %v", err)
			}
			if dials.Load() != 0 {
				t.Fatalf("unsafe proxy path made %d dials", dials.Load())
			}
		})
	}
}

func TestSourceProxyPolicyErrorsAreClassifiedAndRedacted(t *testing.T) {
	installSourceNetworkPolicyForTest(t, "")
	installSourceNetworkHooksForTest(t,
		testSourceNetworkLookup(map[string][]string{
			"target.example":        {"8.8.8.8"},
			"secret-proxy.internal": {"10.0.0.2"},
		}),
		func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("must not dial")
		},
	)
	_, _, err := FetchSourceTextWithURLContext(context.Background(), SourceRequest{
		URL:   "http://target.example/content?token=target-secret",
		Proxy: "socks5://secret-proxy.internal:1080@alice@proxy-secret",
	})
	if !errors.Is(err, ErrSourceRequest) || !errors.Is(err, ErrUnsafeSourceNetwork) {
		t.Fatalf("proxy policy classification = %v", err)
	}
	for _, forbidden := range []string{"target.example", "target-secret", "secret-proxy", "10.0.0.2", "alice", "proxy-secret"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("proxy policy error leaked %q: %v", forbidden, err)
		}
	}
}

func TestHTTPClientTestingOverrideBypassesOnlyNetworkDialPolicy(t *testing.T) {
	installSourceNetworkPolicyForTest(t, "")
	var requests atomic.Int32
	restore := SetHTTPClientForTesting(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fixture")),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	defer restore()

	body, err := FetchTextContext(context.Background(), "https://fixture.invalid/content", "utf-8")
	if err != nil || body != "fixture" || requests.Load() != 1 {
		t.Fatalf("test override body=%q requests=%d err=%v", body, requests.Load(), err)
	}
	if _, err := FetchTextContext(context.Background(), "file:///tmp/private", "utf-8"); !errors.Is(err, ErrUnsafeSourceURL) {
		t.Fatalf("test override bypassed N1 URL policy: %v", err)
	}
}

func installSourceNetworkPolicyForTest(t *testing.T, raw string) {
	t.Helper()
	restore, err := ConfigureSourceNetworkPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
}

func installSourceNetworkHooksForTest(t *testing.T, lookup sourceLookupNetIPFunc, dial sourceDialContextFunc) {
	t.Helper()
	restore := setSourceNetworkHooksForTesting(lookup, dial)
	t.Cleanup(restore)
}

func testSourceNetworkLookup(values map[string][]string) sourceLookupNetIPFunc {
	return func(_ context.Context, _ string, host string) ([]netip.Addr, error) {
		entries, ok := values[host]
		if !ok {
			return nil, fmt.Errorf("unexpected lookup host %q", host)
		}
		addresses := make([]netip.Addr, 0, len(entries))
		for _, entry := range entries {
			addresses = append(addresses, netip.MustParseAddr(entry))
		}
		return addresses, nil
	}
}

func serveSourceNetworkHTTPPipe(connection net.Conn, status int, body, location string, done chan<- error) {
	defer connection.Close()
	if _, err := http.ReadRequest(bufio.NewReader(connection)); err != nil {
		done <- err
		return
	}
	response := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\nConnection: close\r\n", status, http.StatusText(status), len(body))
	if location != "" {
		response += "Location: " + location + "\r\n"
	}
	_, err := io.WriteString(connection, response+"\r\n"+body)
	done <- err
}

func serveSourceSOCKSProxy(connection net.Conn, version string, done chan<- error) {
	defer connection.Close()
	var err error
	switch version {
	case "socks4":
		header := make([]byte, 8)
		if _, err = io.ReadFull(connection, header); err == nil {
			if !bytes.Equal(header, []byte{0x04, 0x01, 0x1f, 0x90, 0x08, 0x08, 0x08, 0x08}) {
				err = fmt.Errorf("SOCKS4 pinned target = %x", header)
			}
		}
		if err == nil {
			userTerminator := make([]byte, 1)
			_, err = io.ReadFull(connection, userTerminator)
			if err == nil && userTerminator[0] != 0 {
				err = fmt.Errorf("unexpected SOCKS4 user payload %x", userTerminator)
			}
		}
		if err == nil {
			_, err = connection.Write([]byte{0x00, 0x5a, 0x1f, 0x90, 0x08, 0x08, 0x08, 0x08})
		}
	case "socks5":
		greeting := make([]byte, 3)
		if _, err = io.ReadFull(connection, greeting); err == nil && !bytes.Equal(greeting, []byte{0x05, 0x01, 0x00}) {
			err = fmt.Errorf("SOCKS5 greeting = %x", greeting)
		}
		if err == nil {
			_, err = connection.Write([]byte{0x05, 0x00})
		}
		connect := make([]byte, 10)
		if err == nil {
			_, err = io.ReadFull(connection, connect)
		}
		if err == nil && !bytes.Equal(connect, []byte{0x05, 0x01, 0x00, 0x01, 0x08, 0x08, 0x08, 0x08, 0x1f, 0x90}) {
			err = fmt.Errorf("SOCKS5 pinned target = %x", connect)
		}
		if err == nil {
			_, err = connection.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		}
	default:
		err = fmt.Errorf("unknown SOCKS version %q", version)
	}
	if err == nil {
		_, err = http.ReadRequest(bufio.NewReader(connection))
	}
	if err == nil {
		_, err = io.WriteString(connection, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
	}
	done <- err
}

func sourceNetworkTLSCertificate(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey}, roots
}
