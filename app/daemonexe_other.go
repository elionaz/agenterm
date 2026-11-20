//go:build !linux

package main

// daemonExe: on macOS the app runs the daemon itself (as agenterm.app, for
// native notifications and their clicks).
func daemonExe(app string) string { return app }
