package nasc

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
)

// testSSHServer accepts password "pw" for user "bob" and echoes input upper-cased.
// It records the last window size it was told about.
type testSSHServer struct {
	ln   net.Listener
	cols uint32
	rows uint32
	mu   sync.Mutex
	pub  ssh.PublicKey // if set, this key may log in as bob
}

func newTestSSHServer(t *testing.T) *testSSHServer {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	srv := &testSSHServer{}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			srv.mu.Lock()
			defer srv.mu.Unlock()
			if c.User() == "bob" && srv.pub != nil && bytes.Equal(k.Marshal(), srv.pub.Marshal()) {
				return nil, nil
			}
			return nil, ssh.ErrNoAuth
		},
		PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if c.User() == "bob" && string(p) == "pw" {
				return nil, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.ln = ln
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(c, cfg)
		}
	}()
	return srv
}

func (s *testSSHServer) serve(c net.Conn, cfg *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				switch r.Type {
				case "pty-req":
					// uint32 termLen, term, cols, rows ...
					p := r.Payload
					tl := int(p[3])
					s.mu.Lock()
					s.cols = uint32(p[4+tl])<<24 | uint32(p[5+tl])<<16 | uint32(p[6+tl])<<8 | uint32(p[7+tl])
					s.rows = uint32(p[8+tl])<<24 | uint32(p[9+tl])<<16 | uint32(p[10+tl])<<8 | uint32(p[11+tl])
					s.mu.Unlock()
					_ = r.Reply(true, nil)
				case "shell":
					_ = r.Reply(true, nil)
					go func() {
						buf := make([]byte, 1024)
						for {
							n, err := ch.Read(buf)
							if n > 0 {
								_, _ = ch.Write(bytes.ToUpper(buf[:n]))
							}
							if err != nil {
								return
							}
						}
					}()
				default:
					_ = r.Reply(false, nil)
				}
			}
		}()
	}
}

func newTestApp(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, err := newServer(t.TempDir(), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func dialWS(t *testing.T, ts *httptest.Server, srv *Server) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	h := http.Header{}
	h.Set("Cookie", cookieName+"="+srv.token)
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	return ws, ctx
}

func writeMsg(t *testing.T, ctx context.Context, ws *websocket.Conn, m message) {
	t.Helper()
	b, _ := json.Marshal(m)
	if err := ws.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func readMsg(t *testing.T, ctx context.Context, ws *websocket.Conn) message {
	t.Helper()
	for {
		typ, data, err := ws.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if typ == websocket.MessageText {
			var m message
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			return m
		}
	}
}

func connectMsg(ssh *testSSHServer, password string) message {
	addr := ssh.ln.Addr().(*net.TCPAddr)
	return message{Type: "connect", Host: "127.0.0.1", Port: addr.Port, User: "bob", Password: password, Cols: 100, Rows: 30}
}

func TestSessionTOFUAndEcho(t *testing.T) {
	sshd := newTestSSHServer(t)
	srv, ts := newTestApp(t)

	ws, ctx := dialWS(t, ts, srv)
	writeMsg(t, ctx, ws, connectMsg(sshd, "pw"))

	hk := readMsg(t, ctx, ws)
	if hk.Type != "hostkey" || hk.Status != KeyNew || !strings.HasPrefix(hk.Fingerprint, "SHA256:") {
		t.Fatalf("expected new hostkey prompt, got %+v", hk)
	}
	writeMsg(t, ctx, ws, message{Type: "trust", Accept: true})
	if m := readMsg(t, ctx, ws); m.Type != "ready" {
		t.Fatalf("expected ready, got %+v", m)
	}

	if err := ws.Write(ctx, websocket.MessageBinary, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	typ, data, err := ws.Read(ctx)
	if err != nil || typ != websocket.MessageBinary || string(data) != "HELLO" {
		t.Fatalf("echo: typ=%v data=%q err=%v", typ, data, err)
	}

	sshd.mu.Lock()
	cols, rows := sshd.cols, sshd.rows
	sshd.mu.Unlock()
	if cols != 100 || rows != 30 {
		t.Fatalf("pty size = %dx%d, want 100x30", cols, rows)
	}

	// Second connection: key is now known, so no prompt.
	ws2, ctx2 := dialWS(t, ts, srv)
	writeMsg(t, ctx2, ws2, connectMsg(sshd, "pw"))
	if m := readMsg(t, ctx2, ws2); m.Type != "ready" {
		t.Fatalf("expected ready without prompt, got %+v", m)
	}
}

func TestSessionRejectedHostKey(t *testing.T) {
	sshd := newTestSSHServer(t)
	srv, ts := newTestApp(t)
	ws, ctx := dialWS(t, ts, srv)
	writeMsg(t, ctx, ws, connectMsg(sshd, "pw"))
	readMsg(t, ctx, ws) // hostkey
	writeMsg(t, ctx, ws, message{Type: "trust", Accept: false})
	if m := readMsg(t, ctx, ws); m.Type != "error" || !strings.Contains(m.Message, "rejected") {
		t.Fatalf("expected rejection error, got %+v", m)
	}
}

func TestSessionChangedHostKey(t *testing.T) {
	first := newTestSSHServer(t)
	srv, ts := newTestApp(t)

	ws, ctx := dialWS(t, ts, srv)
	writeMsg(t, ctx, ws, connectMsg(first, "pw"))
	readMsg(t, ctx, ws)
	writeMsg(t, ctx, ws, message{Type: "trust", Accept: true})
	readMsg(t, ctx, ws) // ready

	// A different server answering on the same host:port presents another key.
	second := newTestSSHServer(t)
	port := first.ln.Addr().(*net.TCPAddr).Port
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	secondKey := hostKeyOf(t, second)
	if got := srv.known.Check(addr, secondKey); got != KeyChanged {
		t.Fatalf("Check = %v, want changed", got)
	}
}

func TestWrongPassword(t *testing.T) {
	sshd := newTestSSHServer(t)
	srv, ts := newTestApp(t)
	ws, ctx := dialWS(t, ts, srv)
	writeMsg(t, ctx, ws, connectMsg(sshd, "nope"))
	readMsg(t, ctx, ws) // hostkey
	writeMsg(t, ctx, ws, message{Type: "trust", Accept: true})
	if m := readMsg(t, ctx, ws); m.Type != "error" {
		t.Fatalf("expected auth error, got %+v", m)
	}
}

func TestAuthRequired(t *testing.T) {
	_, ts := newTestApp(t)
	for _, p := range []string{"/", "/api/hosts", "/ws"} {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without cookie = %d, want 403", p, resp.StatusCode)
		}
	}
}

func TestTokenBecomesCookie(t *testing.T) {
	srv, ts := newTestApp(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ts.URL + "/?t=" + srv.token)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	var got *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			got = c
		}
	}
	if got == nil || !got.HttpOnly || got.Value != srv.token {
		t.Fatalf("bad cookie: %+v", got)
	}
	if r, _ := http.Get(ts.URL + "/?t=wrong"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token = %d", r.StatusCode)
	}
}

func TestHostsCRUD(t *testing.T) {
	srv, ts := newTestApp(t)
	do := func(method, path, body string) (*http.Response, []byte) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: srv.token})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp, buf.Bytes()
	}

	resp, body := do("POST", "/api/hosts", `{"host":"example.com","user":"me"}`)
	var h Host
	_ = json.Unmarshal(body, &h)
	if resp.StatusCode != 200 || h.ID == "" || h.Port != 22 || h.Name != "example.com" {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do("POST", "/api/hosts", `{"host":""}`); resp.StatusCode != 400 {
		t.Fatalf("empty host = %d", resp.StatusCode)
	}
	do("PUT", "/api/hosts/"+h.ID, `{"name":"Prod","host":"example.com","port":2222,"user":"me"}`)
	_, body = do("GET", "/api/hosts", "")
	var list []Host
	_ = json.Unmarshal(body, &list)
	if len(list) != 1 || list[0].Name != "Prod" || list[0].Port != 2222 {
		t.Fatalf("list: %s", body)
	}

	// Survives a restart.
	reopened, err := openHostStore(srv.hosts.path[:strings.LastIndex(srv.hosts.path, "/")])
	if err != nil || len(reopened.List()) != 1 {
		t.Fatalf("reopen: %v %v", err, reopened.List())
	}

	if resp, _ := do("DELETE", "/api/hosts/"+h.ID, ""); resp.StatusCode != 204 {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
	if resp, _ := do("DELETE", "/api/hosts/"+h.ID, ""); resp.StatusCode != 404 {
		t.Fatalf("second delete = %d", resp.StatusCode)
	}
}

func hostKeyOf(t *testing.T, s *testSSHServer) ssh.PublicKey {
	t.Helper()
	var key ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "x", Auth: []ssh.AuthMethod{ssh.Password("x")},
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error { key = k; return nil },
		Timeout:         5 * time.Second,
	}
	c, err := ssh.Dial("tcp", s.ln.Addr().String(), cfg)
	if err == nil {
		c.Close()
	}
	if key == nil {
		t.Fatal("no host key captured")
	}
	return key
}

func TestHistory(t *testing.T) {
	srv, ts := newTestApp(t)
	do := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: srv.token})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, strings.TrimSpace(buf.String())
	}
	add := func(cmd string) {
		b, _ := json.Marshal(map[string]string{"key": "me@h:22", "cmd": cmd})
		if code, _ := do("POST", "/api/history", string(b)); code != 204 {
			t.Fatalf("add %q = %d", cmd, code)
		}
	}
	add("ls -la")
	add("docker ps")
	add("  secret thing") // leading space: not recorded
	add("ls -la")         // duplicate moves to front
	add("")               // empty: ignored

	_, body := do("GET", "/api/history?key=me@h:22", "")
	if body != `["ls -la","docker ps"]` {
		t.Fatalf("history = %s", body)
	}
	if _, body := do("GET", "/api/history?key=other", ""); body != "[]" {
		t.Fatalf("other host = %s", body)
	}

	// Survives restart, and clear works.
	h2, err := openHistoryStore(filepath.Dir(srv.history.path))
	if err != nil || len(h2.Get("me@h:22")) != 2 {
		t.Fatalf("reopen: %v %v", err, h2.Get("me@h:22"))
	}
	do("DELETE", "/api/history?key=me@h:22", "")
	if _, body := do("GET", "/api/history?key=me@h:22", ""); body != "[]" {
		t.Fatalf("after clear = %s", body)
	}
}

func newPEMKey(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(priv)
	return string(pem.EncodeToMemory(block)), signer.PublicKey()
}

func TestVaultEncryptsAtRest(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{1}, 32)
	v, err := openVault(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	pem, _ := newPEMKey(t)
	info, err := v.Add("laptop", pem)
	if err != nil || info.Type != "ssh-ed25519" || !strings.HasPrefix(info.Fingerprint, "SHA256:") || info.Encrypted {
		t.Fatalf("add: %+v %v", info, err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "keys.json"))
	if bytes.Contains(raw, []byte("PRIVATE KEY")) || bytes.Contains(raw, []byte("openssh-key-v1")) {
		t.Fatal("private key stored in plaintext")
	}
	if st, _ := os.Stat(filepath.Join(dir, "keys.json")); st.Mode().Perm() != 0o600 {
		t.Fatalf("keys.json mode = %v", st.Mode().Perm())
	}

	// Same master key can read it back after a restart; a different one cannot.
	again, _ := openVault(dir, key)
	if got, err := again.Private(info.ID); err != nil || strings.TrimSpace(got) != strings.TrimSpace(pem) {
		t.Fatalf("roundtrip: %v", err)
	}
	other, _ := openVault(dir, bytes.Repeat([]byte{2}, 32))
	if _, err := other.Private(info.ID); err == nil {
		t.Fatal("decrypted with the wrong master key")
	}

	// Moving a ciphertext under another id must not decrypt (id is authenticated).
	again.entries[0].ID = "forged"
	if _, err := again.Private("forged"); err == nil {
		t.Fatal("ciphertext accepted under a different id")
	}

	if _, err := openVault(dir, []byte("short")); err == nil {
		t.Fatal("accepted a short master key")
	}
	if _, err := v.Add("junk", "not a key"); err == nil {
		t.Fatal("accepted junk as a key")
	}
	if err := v.Delete(info.ID); err != nil || len(v.List()) != 0 {
		t.Fatalf("delete: %v", err)
	}
}

func TestKeysAPINeverReturnsPrivateKey(t *testing.T) {
	srv, ts := newTestApp(t)
	pem, _ := newPEMKey(t)
	body, _ := json.Marshal(map[string]string{"name": "k", "privateKey": pem})
	do := func(method, path string, b []byte) (int, string) {
		req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(b))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: srv.token})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}
	code, out := do("POST", "/api/keys", body)
	if code != 200 || strings.Contains(out, "PRIVATE") || strings.Contains(out, "openssh-key") {
		t.Fatalf("add: %d %s", code, out)
	}
	_, out = do("GET", "/api/keys", nil)
	if strings.Contains(out, "PRIVATE") || strings.Contains(out, "privateKey") || !strings.Contains(out, `"name":"k"`) {
		t.Fatalf("list: %s", out)
	}
	if code, _ := do("POST", "/api/keys", []byte(`{"name":"x","privateKey":"garbage"}`)); code != 400 {
		t.Fatalf("garbage key = %d", code)
	}
}

func TestConnectWithSavedKey(t *testing.T) {
	sshd := newTestSSHServer(t)
	srv, ts := newTestApp(t)
	pem, pub := newPEMKey(t)
	sshd.mu.Lock()
	sshd.pub = pub
	sshd.mu.Unlock()
	info, err := srv.vault.Add("k", pem)
	if err != nil {
		t.Fatal(err)
	}

	ws, ctx := dialWS(t, ts, srv)
	m := connectMsg(sshd, "")
	m.KeyID = info.ID
	writeMsg(t, ctx, ws, m)
	readMsg(t, ctx, ws) // hostkey
	writeMsg(t, ctx, ws, message{Type: "trust", Accept: true})
	if got := readMsg(t, ctx, ws); got.Type != "ready" {
		t.Fatalf("expected ready via saved key, got %+v", got)
	}

	// An unknown key id fails cleanly.
	ws2, ctx2 := dialWS(t, ts, srv)
	m.KeyID = "nope"
	writeMsg(t, ctx2, ws2, m)
	if got := readMsg(t, ctx2, ws2); got.Type != "error" {
		t.Fatalf("expected error, got %+v", got)
	}
}

func TestGenerateKey(t *testing.T) {
	sshd := newTestSSHServer(t)
	srv, ts := newTestApp(t)

	for _, kind := range []string{"ed25519", "rsa"} {
		if kind == "rsa" && testing.Short() {
			continue
		}
		info, err := srv.vault.Generate("my phone", kind, "")
		if err != nil {
			t.Fatal(kind, err)
		}
		pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(info.PublicKey))
		if err != nil || comment != "my-phone" || ssh.FingerprintSHA256(pub) != info.Fingerprint {
			t.Fatalf("%s public line: %q (%v) comment=%q", kind, info.PublicKey, err, comment)
		}
		pemText, err := srv.vault.Private(info.ID)
		if err != nil || strings.Contains(info.PublicKey, "PRIVATE") {
			t.Fatalf("%s private: %v", kind, err)
		}
		signer, err := ssh.ParsePrivateKey([]byte(pemText))
		if err != nil || !bytes.Equal(signer.PublicKey().Marshal(), pub.Marshal()) {
			t.Fatalf("%s generated private key does not match its public key (%v)", kind, err)
		}
	}

	// A generated key can log in once its public half is authorised on the server.
	info, _ := srv.vault.Generate("login", "ed25519", "")
	pub, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(info.PublicKey))
	sshd.mu.Lock()
	sshd.pub = pub
	sshd.mu.Unlock()
	ws, ctx := dialWS(t, ts, srv)
	m := connectMsg(sshd, "")
	m.KeyID = info.ID
	writeMsg(t, ctx, ws, m)
	readMsg(t, ctx, ws)
	writeMsg(t, ctx, ws, message{Type: "trust", Accept: true})
	if got := readMsg(t, ctx, ws); got.Type != "ready" {
		t.Fatalf("login with generated key: %+v", got)
	}

	// With a passphrase the stored key is itself encrypted and flagged.
	enc, err := srv.vault.Generate("locked", "ed25519", "hunter2")
	if err != nil || !enc.Encrypted || enc.PublicKey == "" {
		t.Fatalf("passphrase key: %+v %v", enc, err)
	}
	pemText, _ := srv.vault.Private(enc.ID)
	if _, err := ssh.ParsePrivateKey([]byte(pemText)); err == nil {
		t.Fatal("passphrase-protected key parsed without a passphrase")
	}
	if _, err := ssh.ParsePrivateKeyWithPassphrase([]byte(pemText), []byte("hunter2")); err != nil {
		t.Fatal(err)
	}

	if _, err := srv.vault.Generate("x", "dsa", ""); err == nil {
		t.Fatal("accepted unsupported key type")
	}
}
