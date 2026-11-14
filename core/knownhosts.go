package nasc

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
)

// HostKeyStatus is the result of checking a server key against what we have stored.
type HostKeyStatus string

const (
	KeyKnown   HostKeyStatus = "known"
	KeyNew     HostKeyStatus = "new"     // never seen this host: ask the user (TOFU)
	KeyChanged HostKeyStatus = "changed" // different key than before: warn loudly
)

type knownEntry struct {
	Type string `json:"type"`
	Key  []byte `json:"key"` // ssh wire format
}

// KnownHosts is a trust-on-first-use store keyed by "host:port".
type KnownHosts struct {
	mu      sync.Mutex
	path    string
	entries map[string]knownEntry
}

func openKnownHosts(dir string) (*KnownHosts, error) {
	k := &KnownHosts{path: filepath.Join(dir, "known_hosts.json"), entries: map[string]knownEntry{}}
	b, err := os.ReadFile(k.path)
	if errors.Is(err, os.ErrNotExist) {
		return k, nil
	}
	if err != nil {
		return nil, err
	}
	return k, json.Unmarshal(b, &k.entries)
}

func (k *KnownHosts) Check(addr string, key ssh.PublicKey) HostKeyStatus {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.entries[addr]
	if !ok {
		return KeyNew
	}
	if e.Type == key.Type() && bytes.Equal(e.Key, key.Marshal()) {
		return KeyKnown
	}
	return KeyChanged
}

// Trust stores (or replaces) the key for addr.
func (k *KnownHosts) Trust(addr string, key ssh.PublicKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.entries[addr] = knownEntry{Type: key.Type(), Key: key.Marshal()}
	b, err := json.MarshalIndent(k.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}
