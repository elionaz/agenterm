package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/elionaz/panal/internal/paths"
)

// defaultInstanceID is the bundle identifier. It keeps the name from before
// the rename to panal on purpose: Sparkle won't update an app to another
// identifier, and macOS keys notification permission, preferences and the
// web view's storage on it.
const defaultInstanceID = "com.elionaz.agenterm"

// singleInstanceID is the Wails SingleInstanceLock id. A second launch with
// the same id hands off to the running window and exits, so it is derived
// from the bundle id (a renamed test copy is its own app) and, when
// PANAL_HOME is set, from that home too (a test instance with its own
// daemon doesn't hand off to the user's app).
func singleInstanceID() string {
	return instanceID(bundleIdentifier(), os.Getenv(paths.EnvHome))
}

func instanceID(bundle, home string) string {
	id := bundle
	if id == "" {
		id = defaultInstanceID
	}
	if home != "" {
		sum := sha256.Sum256([]byte(home))
		id += "." + hex.EncodeToString(sum[:6])
	}
	return id
}
