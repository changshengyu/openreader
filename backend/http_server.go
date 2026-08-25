package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

var errForcedHTTPShutdown = errors.New("HTTP shutdown forced by second signal")

const (
	maxHTTPHeaderBytes    = 512 * 1024
	httpReadHeaderTimeout = 10 * time.Second
	httpShutdownTimeout   = 8 * time.Second
)

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		MaxHeaderBytes:    maxHTTPHeaderBytes,
	}
}

func serveHTTPServer(
	server *http.Server,
	listener net.Listener,
	signals <-chan os.Signal,
	shutdownTimeout time.Duration,
	beginShutdown func(),
) error {
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case received := <-signals:
		log.Printf("OpenReader shutdown requested: %s", received)
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if beginShutdown != nil {
		beginShutdown()
	}
	if err := shutdownContext.Err(); err != nil {
		_ = server.Close()
		<-serveResult
		return err
	}
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- server.Shutdown(shutdownContext)
	}()

	var shutdownErr error
	select {
	case shutdownErr = <-shutdownResult:
	case received := <-signals:
		log.Printf("OpenReader forced shutdown requested: %s", received)
		cancel()
		_ = server.Close()
		<-shutdownResult
		<-serveResult
		return fmt.Errorf("%w: %s", errForcedHTTPShutdown, received)
	}
	if shutdownErr != nil {
		_ = server.Close()
		<-serveResult
		return shutdownErr
	}

	if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
