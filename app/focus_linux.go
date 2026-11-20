//go:build linux

package main

func setWindowFocused(f bool) { windowFocused.Store(f) }
