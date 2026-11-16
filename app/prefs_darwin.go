//go:build darwin

package main

/*
#include <stdlib.h>
void agtInstallPreferencesItem(const char *title);
void agtSetPreferencesTitle(const char *title);
*/
import "C"

import "unsafe"

var onPreferences func()

//export agtPreferencesClicked
func agtPreferencesClicked() {
	if f := onPreferences; f != nil {
		go f()
	}
}

// installPreferencesMenu adds "Preferences… ⌘," (title) to the app menu
// that Wails builds from menu.AppMenu() (whose items cannot be extended
// from Go).
func installPreferencesMenu(title string, f func()) {
	onPreferences = f
	cs := C.CString(title)
	defer C.free(unsafe.Pointer(cs))
	C.agtInstallPreferencesItem(cs)
}

// setPreferencesTitle retitles that item (UI language changed).
func setPreferencesTitle(title string) {
	cs := C.CString(title)
	defer C.free(unsafe.Pointer(cs))
	C.agtSetPreferencesTitle(cs)
}
