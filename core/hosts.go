package nasc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Host is a saved connection. Secrets (passwords, key passphrases) are never stored here.
type Host struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	User  string `json:"user"`
	Group string `json:"group,omitempty"`
}

// HostStore persists hosts as a JSON file, written atomically.
type HostStore struct {
	mu    sync.Mutex
	path  string
	hosts []Host
}

func openHostStore(dir string) (*HostStore, error) {
	s := &HostStore{path: filepath.Join(dir, "hosts.json"), hosts: []Host{}}
	if err := readJSONFile(s.path, &s.hosts); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *HostStore) flush() error {
	b, err := json.MarshalIndent(s.hosts, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, b)
}

func (s *HostStore) List() []Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Host{}, s.hosts...)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Save inserts a host (empty ID) or replaces the one with the same ID.
func (s *HostStore) Save(h Host) (Host, error) {
	h.Name = strings.TrimSpace(h.Name)
	h.Host = strings.TrimSpace(h.Host)
	h.User = strings.TrimSpace(h.User)
	if h.Host == "" {
		return h, errors.New("host is required")
	}
	if h.Port == 0 {
		h.Port = 22
	}
	if h.Port < 1 || h.Port > 65535 {
		return h, errors.New("port out of range")
	}
	if h.Name == "" {
		h.Name = h.Host
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if h.ID == "" {
		raw := make([]byte, 8)
		_, _ = rand.Read(raw)
		h.ID = hex.EncodeToString(raw)
		s.hosts = append(s.hosts, h)
	} else {
		found := false
		for i := range s.hosts {
			if s.hosts[i].ID == h.ID {
				s.hosts[i] = h
				found = true
			}
		}
		if !found {
			return h, errors.New("no such host")
		}
	}
	return h, s.flush()
}

func (s *HostStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.hosts {
		if s.hosts[i].ID == id {
			s.hosts = append(s.hosts[:i], s.hosts[i+1:]...)
			return s.flush()
		}
	}
	return errors.New("no such host")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.hosts.List())
}

func (s *Server) saveHost(w http.ResponseWriter, r *http.Request) {
	var h Host
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&h); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if id := r.PathValue("id"); id != "" {
		h.ID = id
	}
	saved, err := s.hosts.Save(h)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	if err := s.hosts.Delete(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
