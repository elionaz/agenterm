package hooks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Limits of Prompts: a transcript is read up to maxTranscript bytes, a
// line longer than maxPromptLine (a big tool result) is skipped unread.
const (
	maxTranscript = 64 << 20
	maxPromptLine = 1 << 20
)

// Prompts are the prompts the user typed in a Claude Code transcript
// (JSONL at path), oldest first: user messages whose content is plain
// text, without tool results, sidechains (subagents), meta messages or
// slash-command wrappers. Consecutive duplicates are dropped. Nil when
// the transcript cannot be read.
func Prompts(path string) []string {
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".jsonl" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	r := bufio.NewReaderSize(io.LimitReader(f, maxTranscript), 64<<10)
	var out []string
	for {
		line, err := readLine(r)
		if p := prompt(line); p != "" && (len(out) == 0 || out[len(out)-1] != p) {
			out = append(out, p)
		}
		if err != nil {
			return out
		}
	}
}

// readLine reads one line; a line over maxPromptLine comes back empty
// (the rest of it is discarded).
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, err := r.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > maxPromptLine {
				tooLong, buf = true, nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return buf, err
	}
}

func prompt(line []byte) string {
	// Cheap filter before decoding: most lines are assistant messages and
	// tool results.
	if !bytes.Contains(line, []byte(`"user"`)) || !bytes.Contains(line, []byte(`"content":"`)) {
		return ""
	}
	var l struct {
		Type      string `json:"type"`
		Sidechain bool   `json:"isSidechain"`
		Meta      bool   `json:"isMeta"`
		Origin    *struct {
			Kind string `json:"kind"`
		} `json:"origin"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &l) != nil || l.Type != "user" || l.Message.Role != "user" || l.Sidechain || l.Meta {
		return ""
	}
	if l.Origin != nil && l.Origin.Kind != "" && l.Origin.Kind != "human" {
		return ""
	}
	var text string
	if json.Unmarshal(l.Message.Content, &text) != nil {
		return "" // an array: tool results, images
	}
	text = strings.TrimSpace(text)
	// <command-name>, <local-command-stdout>, <system-reminder>…: what
	// Claude Code wraps around slash commands and its own notes.
	if text == "" || strings.HasPrefix(text, "<") {
		return ""
	}
	return text
}
