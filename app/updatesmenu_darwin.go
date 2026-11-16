//go:build darwin

package main

/*
#include <stdlib.h>
void agtInstallUpdatesItem(const char *title, const char *hint);
void agtSetUpdatesItem(const char *title, const char *hint);
*/
import "C"

import "unsafe"

//export agtCheckUpdatesClicked
func agtCheckUpdatesClicked() { sparkleCheck() }

// installUpdatesMenu adds "Check for Updates…" (title) below "About
// agenterm". A non-empty hint means the updater is off: clicking shows it.
func installUpdatesMenu(title, hint string) {
	t, h := C.CString(title), C.CString(hint)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(h))
	C.agtInstallUpdatesItem(t, h)
}

// setUpdatesMenu retitles that item (UI language changed).
func setUpdatesMenu(title, hint string) {
	t, h := C.CString(title), C.CString(hint)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(h))
	C.agtSetUpdatesItem(t, h)
}
