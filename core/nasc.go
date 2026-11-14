// Package nasc is the Go core of the Nasc SSH client. It is bound to Android with
// gomobile: Kotlin calls Start once, then loads the returned URL in a WebView.
// Everything else (terminal I/O, host storage) happens over that loopback URL.
package nasc

import (
	"errors"
	"net"
	"net/http"
	"sync"
)

var (
	mu      sync.Mutex
	running *Server
)

// Start launches the loopback server, storing data under dataDir, and returns the
// URL the WebView should load (it carries a one-time token that becomes a cookie).
// masterKey (32 bytes) encrypts saved private keys; the host app must keep it in secure storage.
func Start(dataDir string, masterKey []byte) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if running != nil {
		return running.URL(), nil
	}
	s, err := newServer(dataDir, masterKey)
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	s.ln = ln
	s.http = &http.Server{Handler: s.Handler()}
	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = err
		}
	}()
	running = s
	return s.URL(), nil
}

// Stop shuts the server down and drops every open SSH session.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if running != nil {
		running.Close()
		running = nil
	}
}
