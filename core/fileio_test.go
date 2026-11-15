package nasc

import (
	"os"
	"path/filepath"
	"testing"
)

// A zero-length data file (left by a crash on filesystems that persist a rename before the data)
// must not stop the app from starting.
func TestStoresSurviveEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"hosts.json", "keys.json", "known_hosts.json", "history.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if s, err := openHostStore(dir); err != nil || len(s.List()) != 0 {
		t.Fatalf("hosts: %v", err)
	}
	if _, err := openKnownHosts(dir); err != nil {
		t.Fatalf("known hosts: %v", err)
	}
	if _, err := openHistoryStore(dir); err != nil {
		t.Fatalf("history: %v", err)
	}
	if _, err := openVault(dir, make([]byte, 32)); err != nil {
		t.Fatalf("vault: %v", err)
	}
}

func TestCorruptFileStillErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openHostStore(dir); err == nil {
		t.Fatal("expected an error for a corrupt, non-empty file")
	}
}

func TestWriteFileAtomicReplaces(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	for _, v := range []string{"one", "two"} {
		if err := writeFileAtomic(p, []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Fatalf("got %q", b)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
}
