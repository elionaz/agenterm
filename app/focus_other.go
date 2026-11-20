//go:build !linux

package main

func setWindowFocused(bool) {} // macOS asks NSApp (desktop.IsActive)
