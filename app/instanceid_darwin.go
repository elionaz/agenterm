//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

// agtBundleIdentifier returns the main bundle's CFBundleIdentifier (malloc'd,
// caller frees) or NULL when not running from a bundle.
static char *agtBundleIdentifier(void) {
	CFBundleRef b = CFBundleGetMainBundle();
	if (b == NULL) return NULL;
	CFStringRef id = CFBundleGetIdentifier(b);
	if (id == NULL) return NULL;
	CFIndex n = CFStringGetMaximumSizeForEncoding(CFStringGetLength(id), kCFStringEncodingUTF8) + 1;
	char *buf = malloc(n);
	if (buf == NULL) return NULL;
	if (!CFStringGetCString(id, buf, n, kCFStringEncodingUTF8)) { free(buf); return NULL; }
	return buf;
}
*/
import "C"

import "unsafe"

func bundleIdentifier() string {
	p := C.agtBundleIdentifier()
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}
