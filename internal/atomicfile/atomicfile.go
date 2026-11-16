// Package atomicfile replaces configuration files safely.
package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
)

// Write replaces path with data atomically (temp file + fsync + rename in
// the same directory). If path already exists its previous content is
// copied to path+".bak" first.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := writeTemp(path+".bak", old, perm); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeTemp(path, data, perm)
}

// Replace is Write without the .bak copy, for frequently rewritten state
// (scrollback snapshots).
func Replace(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeTemp(path, data, perm)
}

func writeTemp(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
