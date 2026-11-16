//go:build !darwin || !cgo

package i18n

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

// appleLanguages reads `defaults read -g AppleLanguages` (macOS without
// cgo); elsewhere there is no such preference.
func appleLanguages() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return nil
	}
	return parseDefaultsArray(string(out))
}
