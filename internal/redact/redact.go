// Package redact hides what looks like a secret (API keys, tokens,
// private keys, password assignments) in text that is about to leave the
// machine, such as the agent prompts added to a pull request. It is a
// best-effort filter, not a guarantee: the user still sees the text
// before it is sent.
package redact

import "regexp"

// Mask replaces each secret.
const Mask = "[redacted]"

type rule struct {
	re   *regexp.Regexp
	repl string // ReplaceAllString template
}

var rules = []rule{
	// PEM private keys (the whole block, or to the end when cut short).
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`), Mask},
	// Provider tokens with a recognisable prefix.
	{regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{10,}`), Mask},
	{regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}`), Mask},
	{regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`), Mask},
	{regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`), Mask},
	{regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`), Mask},
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), Mask},
	{regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{35}`), Mask},
	{regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}`), Mask},
	// JSON Web Tokens.
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), Mask},
	// Credentials in URLs: scheme://user:password@host keeps the user.
	{regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^\s/:@]+:)[^\s/@]+@`), "${1}" + Mask + "@"},
	// Bearer tokens.
	{regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]{16,}`), "${1}" + Mask},
	// key = value / key: value for secret-sounding keys: only the value.
	{regexp.MustCompile(`(?i)(\b[A-Za-z0-9_.-]*(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)[A-Za-z0-9_.-]*["']?\s*[:=]\s*)(?:"[^"\n]+"|'[^'\n]+'|[^\s,;]+)`), "${1}" + Mask},
}

// String returns s with every secret replaced by Mask.
func String(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}
