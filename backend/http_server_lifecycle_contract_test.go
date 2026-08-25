package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPServerUsesFixedHeaderBoundaryWithoutGlobalStreamTimeouts(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	if server.MaxHeaderBytes != 512*1024 {
		t.Fatalf("MaxHeaderBytes = %d, want 512 KiB", server.MaxHeaderBytes)
	}
	if server.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want 10s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 0 || server.WriteTimeout != 0 {
		t.Fatalf("global stream timeouts must stay disabled: read=%s write=%s", server.ReadTimeout, server.WriteTimeout)
	}
}

func TestHTTPServerRejectsSlowAndOversizedHeadersBeforeHandler(t *testing.T) {
	var handled atomic.Int32
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		handled.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	server.ReadHeaderTimeout = 40 * time.Millisecond
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(server, listener, signals, time.Second, nil)
	}()

	slow, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial slow client: %v", err)
	}
	if _, err := io.WriteString(slow, "GET / HTTP/1.1\r\nHost: reader.test\r\nX-Slow: "); err != nil {
		t.Fatalf("write partial header: %v", err)
	}
	_ = slow.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := bufio.NewReader(slow).ReadString('\n'); err != nil {
		var netError net.Error
		if errors.As(err, &netError) && netError.Timeout() {
			t.Fatal("server left an incomplete request header open past ReadHeaderTimeout")
		}
	}
	_ = slow.Close()

	oversized, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial oversized client: %v", err)
	}
	request := "GET / HTTP/1.1\r\nHost: reader.test\r\nX-Large: " + strings.Repeat("x", 600*1024) + "\r\n\r\n"
	if _, err := io.WriteString(oversized, request); err != nil {
		t.Fatalf("write oversized header: %v", err)
	}
	_ = oversized.SetReadDeadline(time.Now().Add(time.Second))
	response, err := http.ReadResponse(bufio.NewReader(oversized), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read oversized response: %v", err)
	}
	_ = response.Body.Close()
	_ = oversized.Close()
	if response.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("oversized header status = %d, want 431", response.StatusCode)
	}
	if handled.Load() != 0 {
		t.Fatalf("oversized/slow requests reached handler %d times", handled.Load())
	}

	signals <- os.Interrupt
	if err := <-done; err != nil {
		t.Fatalf("stop server: %v", err)
	}
}

func TestHTTPServerSignalDrainsStartedRequestAndRunsHookOnce(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		writer.WriteHeader(http.StatusNoContent)
	}))
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	signals := make(chan os.Signal, 1)
	var hookCalls atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(server, listener, signals, time.Second, func() { hookCalls.Add(1) })
	}()

	responseDone := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				requestErr = errors.New("unexpected response status")
			}
		}
		responseDone <- requestErr
	}()
	<-started
	signals <- os.Interrupt
	close(release)
	if err := <-responseDone; err != nil {
		t.Fatalf("started request did not drain: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("graceful stop: %v", err)
	}
	if hookCalls.Load() != 1 {
		t.Fatalf("shutdown hook calls = %d, want 1", hookCalls.Load())
	}
}

func TestHTTPServerSignalForcesCancellationAtDeadline(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(canceled)
	}))
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(server, listener, signals, 40*time.Millisecond, nil)
	}()
	go func() {
		_, _ = http.Get("http://" + listener.Addr().String())
	}()
	<-started
	signals <- os.Interrupt

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("forced shutdown did not cancel the request context")
	}
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("forced shutdown error = %v, want deadline exceeded", err)
	}
}

func TestHTTPServerSecondSignalForcesImmediateCancellation(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(canceled)
	}))
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	signals := make(chan os.Signal, 2)
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(server, listener, signals, 5*time.Second, nil)
	}()
	go func() {
		_, _ = http.Get("http://" + listener.Addr().String())
	}()
	<-started
	signals <- os.Interrupt
	signals <- os.Interrupt

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("second signal did not force immediate request cancellation")
	}
	if err := <-done; err == nil {
		t.Fatal("forced second-signal shutdown was reported as graceful")
	}
}

func TestHTTPServerReturnsListenFailureWithoutWaitingForSignal(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(server, listener, make(chan os.Signal), time.Second, nil)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed listener was reported as a normal shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("listen failure waited for a shutdown signal")
	}
}
