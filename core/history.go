package nasc

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxHistoryPerHost = 500
	maxCommandLen     = 1000
)

// HistoryStore keeps the commands typed per connection key ("user@host:port"), newest first.
// It is what the terminal's autocomplete suggests from.
type HistoryStore struct {
	mu   sync.Mutex
	path string
	data map[string][]string
}

func openHistoryStore(dir string) (*HistoryStore, error) {
	h := &HistoryStore{path: filepath.Join(dir, "history.json"), data: map[string][]string{}}
	b, err := os.ReadFile(h.path)
	if errors.Is(err, os.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return nil, err
	}
	return h, json.Unmarshal(b, &h.data)
}

func (h *HistoryStore) flush() error {
	b, err := json.Marshal(h.data)
	if err != nil {
		return err
	}
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, h.path)
}

func (h *HistoryStore) Get(key string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.data[key]...)
}

// Add records cmd as the most recent entry. Like HISTCONTROL=ignorespace, commands that start
// with a space are not recorded, so people can keep sensitive one-liners out of history.
func (h *HistoryStore) Add(key, cmd string) error {
	cmd = strings.TrimRight(cmd, " \t\r\n")
	if key == "" || strings.TrimSpace(cmd) == "" || strings.HasPrefix(cmd, " ") || len(cmd) > maxCommandLen {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	list := []string{cmd}
	for _, old := range h.data[key] {
		if old != cmd {
			list = append(list, old)
		}
	}
	if len(list) > maxHistoryPerHost {
		list = list[:maxHistoryPerHost]
	}
	h.data[key] = list
	return h.flush()
}

func (h *HistoryStore) Clear(key string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.data, key)
	return h.flush()
}

func (s *Server) getHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.history.Get(r.URL.Query().Get("key")))
}

func (s *Server) addHistory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
		Cmd string `json:"cmd"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if err := s.history.Add(body.Key, body.Cmd); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	if err := s.history.Clear(r.URL.Query().Get("key")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
