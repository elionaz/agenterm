// Package i18n resolves the UI language (settings ui.language) and holds
// the few user-facing strings the Go side renders itself: notification
// fallbacks, the app menu, hook activity lines. English is the canonical
// language; CLI output and RPC errors are always English.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang is a supported UI language.
type Lang string

const (
	EN Lang = "en"
	ES Lang = "es"
)

// Settings values for ui.language.
const (
	Auto = "auto"
)

// NormalizeSetting maps a ui.language value to auto | en | es.
func NormalizeSetting(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(EN):
		return string(EN)
	case string(ES):
		return string(ES)
	}
	return Auto
}

// Match returns the first supported language in a preference list of
// BCP 47 / POSIX tags ("es-ES", "en_US.UTF-8", "fr").
func Match(tags []string) (Lang, bool) {
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		t, _, _ = strings.Cut(t, ".") // en_US.UTF-8
		prim := t
		if i := strings.IndexAny(t, "-_"); i >= 0 {
			prim = t[:i]
		}
		switch prim {
		case string(EN):
			return EN, true
		case string(ES):
			return ES, true
		}
	}
	return "", false
}

// Detector finds the system language: the macOS preferred languages first,
// then the POSIX locale variables. Its fields are injectable for tests.
type Detector struct {
	Getenv         func(string) string
	AppleLanguages func() []string
}

// System is the real environment.
var System = Detector{Getenv: os.Getenv, AppleLanguages: appleLanguages}

// Detect returns the first supported preferred language, else English.
func (d Detector) Detect() Lang {
	if d.AppleLanguages != nil {
		if l, ok := Match(d.AppleLanguages()); ok {
			return l
		}
	}
	if d.Getenv != nil {
		for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
			v := d.Getenv(k)
			if v == "" || v == "C" || v == "POSIX" {
				continue
			}
			if l, ok := Match(strings.Split(v, ":")); ok {
				return l
			}
			break // the first set variable wins, as in setlocale
		}
	}
	return EN
}

// Resolve turns a ui.language setting into a language.
func (d Detector) Resolve(setting string) Lang {
	switch NormalizeSetting(setting) {
	case string(EN):
		return EN
	case string(ES):
		return ES
	}
	return d.Detect()
}

// Resolve is System.Resolve.
func Resolve(setting string) Lang { return System.Resolve(setting) }

// Message keys.
const (
	NotifyPermission = "notify.permission"
	NotifyQuestion   = "notify.question"
	NotifyDone       = "notify.done"
	NotifyError      = "notify.error"
	NotifyStall      = "notify.stall"
	NotifyStallAgo   = "notify.stallAgo" // %s: duration
	NotifyAttention  = "notify.attention"

	MenuPreferences  = "menu.preferences"
	MenuCheckUpdates = "menu.checkUpdates"
	// Shown by "Check for Updates…" when the in-app updater is off.
	UpdatesHomebrew    = "updates.homebrew"
	UpdatesUnavailable = "updates.unavailable"
	DialogAddProject   = "dialog.addProject"
	AboutMessage       = "about.message"

	PermissionFor = "permission.for" // %s: tool

	// Lines the daemon writes into a restored session's terminal.
	TermRestored  = "term.restored"  // %s: time
	TermSuspended = "term.suspended" // %s: time
)

var messages = map[string]map[Lang]string{
	NotifyPermission: {EN: "Needs your approval", ES: "Necesita tu aprobación"},
	NotifyQuestion:   {EN: "Waiting for your answer", ES: "Está esperando tu respuesta"},
	NotifyDone:       {EN: "Finished", ES: "Terminó"},
	NotifyError:      {EN: "Finished with an error", ES: "Terminó con un error"},
	NotifyStall:      {EN: "Stalled? No activity", ES: "¿Trabado? Sin actividad"},
	NotifyStallAgo:   {EN: "for %s", ES: "hace %s"},
	NotifyAttention:  {EN: "Needs your attention", ES: "Necesita tu atención"},

	MenuPreferences:  {EN: "Preferences…", ES: "Preferencias…"},
	MenuCheckUpdates: {EN: "Check for Updates…", ES: "Buscar actualizaciones…"},
	UpdatesHomebrew: {EN: "Installed with Homebrew — update with `brew upgrade --cask agenterm`.",
		ES: "Instalado con Homebrew — actualízalo con `brew upgrade --cask agenterm`."},
	UpdatesUnavailable: {EN: "In-app updates are not available in this build.",
		ES: "Las actualizaciones integradas no están disponibles en esta compilación."},
	DialogAddProject: {EN: "Add project", ES: "Añadir proyecto"},
	AboutMessage:     {EN: "Desktop workspace for coding agents", ES: "Workspace de escritorio para agentes de código"},

	PermissionFor: {EN: "Permission for %s", ES: "Permiso para %s"},
	TermRestored:  {EN: "session restored · %s", ES: "sesión restaurada · %s"},
	TermSuspended: {EN: "suspended since %s (daemon restarted) · resume to continue", ES: "suspendida desde %s (el daemon se reinició) · reanúdala para continuar"},

	// Activity lines: "activity.<kind>" takes the target, ".bare" is used
	// without one. Keep in sync with the frontend dictionaries.
	"activity." + ActThinking:           {EN: "Thinking…", ES: "Pensando…"},
	"activity." + ActEdit:               {EN: "Editing %s", ES: "Editando %s"},
	"activity." + ActEdit + ".bare":     {EN: "Editing", ES: "Editando"},
	"activity." + ActRead:               {EN: "Reading %s", ES: "Leyendo %s"},
	"activity." + ActRead + ".bare":     {EN: "Reading", ES: "Leyendo"},
	"activity." + ActBash:               {EN: "Bash: %s", ES: "Bash: %s"},
	"activity." + ActBash + ".bare":     {EN: "Bash", ES: "Bash"},
	"activity." + ActSearch:             {EN: "Searching %s", ES: "Buscando %s"},
	"activity." + ActSearch + ".bare":   {EN: "Searching", ES: "Buscando"},
	"activity." + ActWeb:                {EN: "Web: %s", ES: "Web: %s"},
	"activity." + ActWeb + ".bare":      {EN: "Web", ES: "Web"},
	"activity." + ActSubagent:           {EN: "Subagent: %s", ES: "Subagente: %s"},
	"activity." + ActSubagent + ".bare": {EN: "Subagent", ES: "Subagente"},
	"activity." + ActTodo:               {EN: "Updating tasks", ES: "Actualizando tareas"},
	"activity." + ActMCP:                {EN: "MCP: %s", ES: "MCP: %s"},
	"activity." + ActMCP + ".bare":      {EN: "MCP", ES: "MCP"},
}

// Activity kinds (protocol activityKind): what a hook reported the agent
// doing. ActTool is any other tool (the target is its name, verbatim).
const (
	ActThinking = "thinking"
	ActEdit     = "edit"
	ActRead     = "read"
	ActBash     = "bash"
	ActSearch   = "search"
	ActWeb      = "web"
	ActSubagent = "subagent"
	ActTodo     = "todo"
	ActMCP      = "mcp"
	ActTool     = "tool"
)

// ActivityText renders a structured activity in l: "Editing main.go",
// "Leyendo go.mod". Unknown kinds (and ActTool) render the target as is.
func ActivityText(l Lang, kind, target string) string {
	key := "activity." + kind
	m, ok := messages[key]
	if !ok {
		return target
	}
	if !strings.Contains(m[EN], "%s") {
		return T(l, key)
	}
	if target == "" {
		return T(l, key+".bare")
	}
	return T(l, key, target)
}

// T renders key in l (English when l is unknown), formatting args with
// fmt verbs. Unknown keys render as the key itself.
func T(l Lang, key string, args ...any) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	s, ok := m[l]
	if !ok {
		s = m[EN]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// parseDefaultsArray parses `defaults` plist output: ( "es-ES", en ).
func parseDefaultsArray(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune("(),\n\t \"", r) }) {
		out = append(out, f)
	}
	return out
}
