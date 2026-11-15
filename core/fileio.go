package nasc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// writeFileAtomic replaces path with data so that a crash or power loss at any point leaves either
// the old file or the new one, never an empty or partial file. Write-then-rename alone is not
// enough: without fsync, filesystems like ext4 can persist the rename before the data and leave a
// zero-length file, which used to make the app fail to start.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	// Persist the rename itself. Best effort: some platforms can't fsync a directory.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// readJSONFile loads path into v. A missing file, or an empty one left behind by an unclean
// shutdown, leaves v untouched so the app starts with no data instead of refusing to start.
// A non-empty file that isn't valid JSON is still an error.
func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}
