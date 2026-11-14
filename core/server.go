package nasc

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

//go:embed all:webdist
var webFS embed.FS

const cookieName = "nasc"

// Server is the loopback HTTP server: static web app, JSON API and the WebSocket
// that carries SSH sessions. Every request must carry the per-launch token.
type Server struct {
	token   string
	hosts   *HostStore
	known   *KnownHosts
	history *HistoryStore
	vault   *Vault
	ln      net.Listener
	http    *http.Server

	mu       sync.Mutex
	sessions map[*session]struct{}
}

func newServer(dir string, masterKey []byte) (*Server, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	hosts, err := openHostStore(dir)
	if err != nil {
		return nil, err
	}
	known, err := openKnownHosts(dir)
	if err != nil {
		return nil, err
	}
	history, err := openHistoryStore(dir)
	if err != nil {
		return nil, err
	}
	vault, err := openVault(dir, masterKey)
	if err != nil {
		return nil, err
	}
	return &Server{
		token:    hex.EncodeToString(raw),
		hosts:    hosts,
		known:    known,
		history:  history,
		vault:    vault,
		sessions: map[*session]struct{}{},
	}, nil
}

// URL is the address for the WebView to load; the token is swapped for a cookie.
func (s *Server) URL() string {
	return fmt.Sprintf("http://%s/?t=%s", s.ln.Addr().String(), s.token)
}

// Close stops listening and ends all sessions.
func (s *Server) Close() {
	if s.http != nil {
		_ = s.http.Close()
	}
	s.mu.Lock()
	for sess := range s.sessions {
		sess.close()
	}
	s.mu.Unlock()
}

func (s *Server) authorized(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) == 1
}

// Handler builds the routing table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /api/hosts", s.listHosts)
	mux.HandleFunc("POST /api/hosts", s.saveHost)
	mux.HandleFunc("PUT /api/hosts/{id}", s.saveHost)
	mux.HandleFunc("DELETE /api/hosts/{id}", s.deleteHost)
	mux.HandleFunc("GET /api/keys", s.listKeys)
	mux.HandleFunc("POST /api/keys", s.addKey)
	mux.HandleFunc("POST /api/keys/generate", s.generateKey)
	mux.HandleFunc("DELETE /api/keys/{id}", s.deleteKey)
	mux.HandleFunc("GET /api/history", s.getHistory)
	mux.HandleFunc("POST /api/history", s.addHistory)
	mux.HandleFunc("DELETE /api/history", s.clearHistory)
	mux.HandleFunc("/", s.static)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Trade the launch token for an HttpOnly cookie, then drop it from the URL.
		if t := r.URL.Query().Get("t"); t != "" {
			if subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: s.token, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			q := r.URL.Query()
			q.Del("t")
			target := r.URL.Path
			if enc := q.Encode(); enc != "" {
				target += "?" + enc
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		if !s.authorized(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

// static serves the embedded SvelteKit build, falling back to index.html for SPA routes.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		http.Error(w, "no web build", http.StatusInternalServerError)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if f, err := sub.Open(name); err == nil {
		st, statErr := f.Stat()
		f.Close()
		if statErr == nil && !st.IsDir() {
			http.ServeFileFS(w, r, sub, name)
			return
		}
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		http.Error(w, "web app not built: run `make web`", http.StatusNotFound)
		return
	}
	http.ServeFileFS(w, r, sub, "index.html")
}
