package nasc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
)

// Wire protocol on /ws (one WebSocket per terminal tab):
//
//	client -> server  text   {"type":"connect", host, port, user, password?, privateKey?, passphrase?, cols, rows}
//	server -> client  text   {"type":"hostkey", status:"new"|"changed", keyType, fingerprint}   (client must answer)
//	client -> server  text   {"type":"trust", accept:bool}
//	server -> client  text   {"type":"ready"}
//	client -> server  text   {"type":"resize", cols, rows}
//	both ways         binary terminal bytes
//	server -> client  text   {"type":"error", message} | {"type":"exit", message}
type message struct {
	Type string `json:"type"`

	// connect
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
	KeyID      string `json:"keyId,omitempty"`      // a key saved in the vault
	PrivateKey string `json:"privateKey,omitempty"` // an unsaved, pasted key
	Passphrase string `json:"passphrase,omitempty"`

	// connect + resize
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`

	// trust
	Accept bool `json:"accept,omitempty"`

	// hostkey
	Status      HostKeyStatus `json:"status,omitempty"`
	KeyType     string        `json:"keyType,omitempty"`
	Fingerprint string        `json:"fingerprint,omitempty"`

	// error / exit
	Message string `json:"message,omitempty"`
}

type session struct {
	srv    *Server
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	client *ssh.Client
	id     string
}

func (s *session) logf(format string, args ...any) {
	log.Printf("nasc[%s]: %s", s.id, fmt.Sprintf(format, args...))
}

func (s *session) close() { s.closeWith(websocket.StatusNormalClosure, "") }

// closeWith ends the session; the reason (max ~120 bytes) reaches the browser in the close frame.
func (s *session) closeWith(code websocket.StatusCode, reason string) {
	s.once.Do(func() {
		s.logf("closing: code=%d reason=%q", code, reason)
		s.cancel()
		if s.client != nil {
			_ = s.client.Close()
		}
		if len(reason) > 120 {
			reason = reason[:120]
		}
		_ = s.ws.Close(code, reason)
	})
}

func (s *session) send(m message) error {
	b, _ := json.Marshal(m)
	return s.ws.Write(s.ctx, websocket.MessageText, b)
}

func (s *session) fail(err error) {
	s.logf("failed: %v", err)
	_ = s.send(message{Type: "error", Message: err.Error()})
	s.closeWith(websocket.StatusInternalError, err.Error())
}

// readControl reads the next text frame as a control message.
func (s *session) readControl() (message, error) {
	for {
		typ, data, err := s.ws.Read(s.ctx)
		if err != nil {
			return message{}, err
		}
		if typ != websocket.MessageText {
			continue // stray terminal bytes before the shell exists
		}
		var m message
		if err := json.Unmarshal(data, &m); err != nil {
			return message{}, err
		}
		return m, nil
	}
}

func (srv *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil) // rejects cross-origin by default
	if err != nil {
		return
	}
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{srv: srv, ws: ws, ctx: ctx, cancel: cancel, id: r.RemoteAddr}
	s.logf("websocket opened")
	srv.mu.Lock()
	srv.sessions[s] = struct{}{}
	srv.mu.Unlock()
	defer func() {
		s.close()
		srv.mu.Lock()
		delete(srv.sessions, s)
		srv.mu.Unlock()
	}()
	s.run()
}

func (s *session) run() {
	defer func() {
		if r := recover(); r != nil {
			s.fail(fmt.Errorf("internal error: %v", r))
		}
	}()
	first, err := s.readControl()
	if err != nil {
		s.logf("no connect message: %v", err)
		return
	}
	s.logf("connect %s@%s:%d (key=%t saved=%t password=%t)", first.User, first.Host, first.Port, first.PrivateKey != "", first.KeyID != "", first.Password != "")
	if first.Type != "connect" {
		s.fail(errors.New("expected connect"))
		return
	}
	if first.Cols < 1 || first.Rows < 1 {
		first.Cols, first.Rows = 80, 24
	}
	if first.Port == 0 {
		first.Port = 22
	}

	client, err := s.dial(first)
	if err != nil {
		s.fail(err)
		return
	}
	s.client = client
	s.logf("ssh connected, server version %q", string(client.ServerVersion()))

	sess, err := client.NewSession()
	if err != nil {
		s.fail(err)
		return
	}
	defer sess.Close()

	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}
	if err := sess.RequestPty("xterm-256color", first.Rows, first.Cols, modes); err != nil {
		s.fail(fmt.Errorf("pty: %w", err))
		return
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		s.fail(err)
		return
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		s.fail(err)
		return
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		s.fail(err)
		return
	}
	if err := sess.Shell(); err != nil {
		s.fail(fmt.Errorf("shell: %w", err))
		return
	}
	s.logf("shell started")
	_ = s.send(message{Type: "ready"})

	var out sync.WaitGroup
	for _, r := range []io.Reader{stdout, stderr} {
		out.Add(1)
		go func(r io.Reader) {
			defer out.Done()
			buf := make([]byte, 32*1024)
			for {
				n, err := r.Read(buf)
				if n > 0 {
					if werr := s.ws.Write(s.ctx, websocket.MessageBinary, buf[:n]); werr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}(r)
	}

	done := make(chan struct{})
	go func() {
		out.Wait()
		werr := sess.Wait()
		s.logf("remote session ended: %v", werr)
		msg := "remote shell exited"
		var exit *ssh.ExitError
		var missing *ssh.ExitMissingError
		switch {
		case errors.As(werr, &exit):
			msg = fmt.Sprintf("remote shell exited with status %d", exit.ExitStatus())
		case errors.As(werr, &missing):
			msg = "server closed the connection without an exit status"
		case werr != nil:
			msg = "session ended: " + werr.Error()
		}
		_ = s.send(message{Type: "exit", Message: msg})
		close(done)
		s.close()
	}()

	go s.keepAlive(done)

	// Browser -> remote.
	for {
		typ, data, err := s.ws.Read(s.ctx)
		if err != nil {
			s.logf("browser read ended: %v (status %d)", err, websocket.CloseStatus(err))
			return
		}
		if typ == websocket.MessageBinary {
			if _, err := stdin.Write(data); err != nil {
				s.logf("stdin write: %v", err)
				return
			}
			continue
		}
		var m message
		if json.Unmarshal(data, &m) == nil && m.Type == "resize" && m.Cols > 0 && m.Rows > 0 {
			_ = sess.WindowChange(m.Rows, m.Cols)
		}
	}
}

func (s *session) keepAlive(done <-chan struct{}) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-s.ctx.Done():
			return
		case <-t.C:
			if _, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				s.logf("keepalive failed: %v", err)
				_ = s.send(message{Type: "error", Message: "connection lost: keepalive failed (" + err.Error() + ")"})
				s.close()
				return
			}
		}
	}
}

func (s *session) authMethods(m message) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if m.KeyID != "" {
		pem, err := s.srv.vault.Private(m.KeyID)
		if err != nil {
			return nil, err
		}
		m.PrivateKey = pem
	}
	if m.PrivateKey != "" {
		var signer ssh.Signer
		var err error
		if m.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(m.PrivateKey), []byte(m.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(m.PrivateKey))
		}
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, errors.New("private key is encrypted: passphrase required")
		}
		if err != nil {
			return nil, fmt.Errorf("private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if m.Password != "" {
		methods = append(methods,
			ssh.Password(m.Password),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				answers := make([]string, len(qs))
				for i := range answers {
					answers[i] = m.Password
				}
				return answers, nil
			}),
		)
	}
	if len(methods) == 0 {
		return nil, errors.New("no password or key provided")
	}
	return methods, nil
}

func (s *session) dial(m message) (*ssh.Client, error) {
	if m.Host == "" || m.User == "" {
		return nil, errors.New("host and user are required")
	}
	auth, err := s.authMethods(m)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))

	cfg := &ssh.ClientConfig{
		User:    m.User,
		Auth:    auth,
		Timeout: 15 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			return s.verifyHostKey(addr, key)
		},
	}
	d := net.Dialer{Timeout: cfg.Timeout}
	conn, err := d.DialContext(s.ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// Bound the handshake too, including the time the user spends on the trust prompt.
	sc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(sc, chans, reqs), nil
}

// verifyHostKey implements trust-on-first-use: unknown and changed keys are put to the
// user over the WebSocket and the handshake waits for the answer.
func (s *session) verifyHostKey(addr string, key ssh.PublicKey) error {
	status := s.srv.known.Check(addr, key)
	if status == KeyKnown {
		return nil
	}
	err := s.send(message{
		Type: "hostkey", Status: status,
		KeyType: key.Type(), Fingerprint: ssh.FingerprintSHA256(key),
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	for {
		typ, data, err := s.ws.Read(ctx)
		if err != nil {
			return errors.New("host key prompt not answered")
		}
		if typ != websocket.MessageText {
			continue
		}
		var m message
		if json.Unmarshal(data, &m) != nil || m.Type != "trust" {
			continue
		}
		if !m.Accept {
			return errors.New("host key rejected")
		}
		return s.srv.known.Trust(addr, key)
	}
}
