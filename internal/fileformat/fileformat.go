// Package fileformat versions agenterm's on-disk files.
//
// Every file agenterm writes carries a format version: a top-level
// `version` key in settings.yaml and profiles.yaml, and a JSON envelope
// {"version": N, "<items>": [...]} in projects.json, worktrees.json and
// sessions.json. Files written before 0.4.0 have no version; they are format
// 1 and are still read (and rewritten with the version on the next save).
//
// A file whose version is newer than the build understands was written by a
// newer agenterm: it is refused with a TooNewError and never overwritten, so
// running an older build after a downgrade cannot destroy data.
package fileformat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// TooNewError: path was written in a format newer than this build reads.
type TooNewError struct {
	Path    string
	Version int
	Max     int
}

func (e *TooNewError) Error() string {
	return fmt.Sprintf("%s was written by a newer agenterm (format %d; this build reads up to %d): update agenterm",
		filepath.Base(e.Path), e.Version, e.Max)
}

// IsTooNew reports whether err is (or wraps) a TooNewError.
func IsTooNew(err error) bool {
	var t *TooNewError
	return errors.As(err, &t)
}

// Check validates a version read from path. 0 means the key was absent: a
// file from before formats were versioned, which is format 1.
func Check(path string, v, max int) error {
	switch {
	case v < 0:
		return fmt.Errorf("%s: invalid format version %d", filepath.Base(path), v)
	case v > max:
		return &TooNewError{Path: path, Version: v, Max: max}
	}
	return nil
}

// DecodeList decodes a JSON list file: either the versioned envelope
// {"version": N, key: [...]} or, for files from before 0.4.0, a bare array.
// It returns the format version (1 for a bare array).
func DecodeList[T any](path string, data []byte, key string, max int) ([]T, int, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var list []T
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, 0, fmt.Errorf("parse %s: %w", path, err)
		}
		return list, 1, nil
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", path, err)
	}
	var v int
	if raw, ok := env["version"]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, 0, fmt.Errorf("parse %s: version: %w", path, err)
		}
	}
	if v == 0 {
		return nil, 0, fmt.Errorf("parse %s: missing format version", path)
	}
	if err := Check(path, v, max); err != nil {
		return nil, v, err
	}
	var list []T
	if raw, ok := env[key]; ok {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, v, fmt.Errorf("parse %s: %s: %w", path, key, err)
		}
	}
	return list, v, nil
}

// EncodeList encodes list in the versioned envelope, version first,
// indented with two spaces and ending in a newline.
func EncodeList[T any](key string, version int, list []T) ([]byte, error) {
	if list == nil {
		list = []T{}
	}
	items, err := json.MarshalIndent(list, "  ", "  ")
	if err != nil {
		return nil, err
	}
	k, _ := json.Marshal(key)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{\n  \"version\": %d,\n  %s: ", version, k)
	buf.Write(items)
	buf.WriteString("\n}\n")
	return buf.Bytes(), nil
}
