package nasc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// KeyInfo is everything the UI is allowed to know about a saved key. The private part never
// leaves the vault except to be handed to the SSH client at connect time.
type KeyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`        // e.g. ssh-ed25519
	Fingerprint string `json:"fingerprint,omitempty"` // SHA256:... of the public key
	PublicKey   string `json:"publicKey,omitempty"`   // authorized_keys line: safe to show and copy
	Encrypted   bool   `json:"encrypted"`             // the key itself needs a passphrase to use
}

type vaultEntry struct {
	KeyInfo
	Nonce []byte `json:"nonce"`
	Data  []byte `json:"data"` // AES-256-GCM(private key PEM), AAD = ID
}

// Vault stores private keys encrypted at rest. The 32-byte master key is supplied by the host
// app (Android: a random key wrapped by the hardware-backed Android Keystore) and lives only in
// memory here, so keys.json on its own is useless.
type Vault struct {
	mu      sync.Mutex
	path    string
	gcm     cipher.AEAD
	entries []vaultEntry
}

func openVault(dir string, masterKey []byte) (*Vault, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	v := &Vault{path: filepath.Join(dir, "keys.json"), gcm: gcm, entries: []vaultEntry{}}
	b, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	return v, json.Unmarshal(b, &v.entries)
}

func (v *Vault) flush() error {
	b, err := json.Marshal(v.entries)
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

func (v *Vault) List() []KeyInfo {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]KeyInfo, 0, len(v.entries))
	for _, e := range v.entries {
		out = append(out, e.KeyInfo)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Add validates and stores a PEM/OpenSSH private key. Passphrase-protected keys are accepted:
// the passphrase is not stored and is asked for at connect time.
func (v *Vault) Add(name, pem string) (KeyInfo, error) {
	pem = strings.TrimSpace(pem) + "\n"
	info := KeyInfo{Name: strings.TrimSpace(name)}
	signer, err := ssh.ParsePrivateKey([]byte(pem))
	var missing *ssh.PassphraseMissingError
	switch {
	case errors.As(err, &missing):
		info.Encrypted = true
		info.setPublic(missing.PublicKey)
	case err != nil:
		return info, fmt.Errorf("not a valid private key: %w", err)
	default:
		info.setPublic(signer.PublicKey())
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return info, err
	}
	info.ID = hex.EncodeToString(raw)
	if info.Name == "" {
		info.Name = "Key " + info.ID[:4]
	}
	if info.PublicKey != "" {
		info.PublicKey += " " + strings.Join(strings.Fields(info.Name), "-") // comment, like ssh-keygen -C
	}
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return info, err
	}
	entry := vaultEntry{KeyInfo: info, Nonce: nonce, Data: v.gcm.Seal(nil, nonce, []byte(pem), []byte(info.ID))}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = append(v.entries, entry)
	if err := v.flush(); err != nil {
		v.entries = v.entries[:len(v.entries)-1]
		return info, err
	}
	return info, nil
}

// Private decrypts a stored key. Fails if the file was tampered with or the master key differs.
func (v *Vault) Private(id string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, e := range v.entries {
		if e.ID == id {
			pem, err := v.gcm.Open(nil, e.Nonce, e.Data, []byte(e.ID))
			if err != nil {
				return "", errors.New("saved key could not be decrypted (secure storage key changed or data corrupted)")
			}
			return string(pem), nil
		}
	}
	return "", errors.New("no such saved key")
}

func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i, e := range v.entries {
		if e.ID == id {
			v.entries = append(v.entries[:i], v.entries[i+1:]...)
			return v.flush()
		}
	}
	return errors.New("no such saved key")
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.vault.List())
}

func (s *Server) addKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		PrivateKey string `json:"privateKey"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	info, err := s.vault.Add(body.Name, body.PrivateKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := s.vault.Delete(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (k *KeyInfo) setPublic(pub ssh.PublicKey) {
	if pub == nil {
		return
	}
	k.Type = pub.Type()
	k.Fingerprint = ssh.FingerprintSHA256(pub)
	k.PublicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
}

// Generate makes a new keypair on the device and stores it. kind is "ed25519" or "rsa" (4096 bits).
// The private half never leaves the vault; the returned KeyInfo carries the public line to install
// on servers. An optional passphrase additionally encrypts the key itself.
func (v *Vault) Generate(name, kind, passphrase string) (KeyInfo, error) {
	var priv any
	switch kind {
	case "", "ed25519":
		_, k, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return KeyInfo{}, err
		}
		priv = k
	case "rsa":
		k, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return KeyInfo{}, err
		}
		priv = k
	default:
		return KeyInfo{}, fmt.Errorf("unsupported key type %q", kind)
	}
	comment := strings.Join(strings.Fields(name), "-")
	var block *pem.Block
	var err error
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, comment, []byte(passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(priv, comment)
	}
	if err != nil {
		return KeyInfo{}, err
	}
	return v.Add(name, string(pem.EncodeToMemory(block)))
}

func (s *Server) generateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	info, err := s.vault.Generate(body.Name, body.Type, body.Passphrase)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}
