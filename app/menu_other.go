//go:build !darwin

package main

// The native menu items (Preferences…, Check for Updates…) are macOS's.

func installPreferencesMenu(string, func()) {}
func setPreferencesTitle(string)            {}
func installUpdatesMenu(string, string)     {}
func setUpdatesMenu(string, string)         {}
