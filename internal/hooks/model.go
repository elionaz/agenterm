package hooks

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// transcriptTail is how much of a transcript Model reads (from its end).
const transcriptTail = 256 << 10

// Model is the model Claude Code reports in payload: its "model" field
// (a string, or an object with an id), else the last model an assistant
// message used in the transcript (transcript_path). Only for the events
// where it can change (start, a new prompt, end of turn); "" otherwise.
//
// Other agents: Cursor sends "model" with every hook; the opencode plugin
// sends it when it changes; Gemini CLI only puts it in its transcript
// (JSONL, "type":"gemini" messages), read at start and at the end of a
// turn.
func Model(agent string, payload []byte) string {
	switch agent {
	case AgentCursor, AgentOpencode:
		var p struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(payload, &p) != nil || len(p.Model) > 80 {
			return ""
		}
		return p.Model
	case AgentGemini:
		var p struct {
			Event      string `json:"hook_event_name"`
			Transcript string `json:"transcript_path"`
		}
		if json.Unmarshal(payload, &p) != nil || (p.Event != "SessionStart" && p.Event != "AfterAgent") {
			return ""
		}
		return transcriptModelOf(p.Transcript, `"type":"gemini"`, true)
	case AgentClaude:
	default:
		return ""
	}
	var p struct {
		Event      string          `json:"hook_event_name"`
		Model      json.RawMessage `json:"model"`
		Transcript string          `json:"transcript_path"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	switch p.Event {
	case "SessionStart", "UserPromptSubmit", "Stop":
	default:
		return ""
	}
	if m := modelField(p.Model); m != "" {
		return m
	}
	return transcriptModel(p.Transcript)
}

func modelField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.ID
	}
	return ""
}

var modelRe = regexp.MustCompile(`"model":"([^"]{1,80})"`)

// transcriptModel reads the end of a Claude Code transcript (JSONL) and
// returns the model of its last assistant message.
func transcriptModel(path string) string { return transcriptModelOf(path, `"assistant"`, false) }

// transcriptModelOf reads the end of a JSONL transcript and returns the
// model of its last line containing marker (the agent's own messages):
// that line's first "model" field, or its last one with lastField.
func transcriptModelOf(path, marker string, lastField bool) string {
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".jsonl" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	off := max(fi.Size()-transcriptTail, 0)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return ""
	}
	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if !bytes.Contains(l, []byte(marker)) {
			continue
		}
		ms := modelRe.FindAllSubmatch(l, -1)
		if len(ms) == 0 {
			continue
		}
		// Claude Code writes the model before the message content, Gemini
		// CLI after it (content may hold tool arguments named "model").
		m := ms[0]
		if lastField {
			m = ms[len(ms)-1]
		}
		if !strings.HasPrefix(string(m[1]), "<") {
			return string(m[1])
		}
	}
	return ""
}
