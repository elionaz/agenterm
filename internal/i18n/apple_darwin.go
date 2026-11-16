//go:build darwin && cgo

package i18n

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

// agtAppleLanguages returns the global AppleLanguages preference as a
// newline-separated, malloc'd C string (NULL if unset). The preference is
// re-synchronized first, so a long-running process sees later changes.
static char *agtAppleLanguages(void) {
	CFPreferencesAppSynchronize(kCFPreferencesAnyApplication);
	CFPropertyListRef v = CFPreferencesCopyAppValue(CFSTR("AppleLanguages"), kCFPreferencesAnyApplication);
	if (v == NULL) {
		return NULL;
	}
	if (CFGetTypeID(v) != CFArrayGetTypeID()) {
		CFRelease(v);
		return NULL;
	}
	CFArrayRef arr = (CFArrayRef)v;
	size_t cap = 1024, len = 0;
	char *out = malloc(cap);
	if (out == NULL) {
		CFRelease(v);
		return NULL;
	}
	out[0] = 0;
	for (CFIndex i = 0; i < CFArrayGetCount(arr) && i < 16; i++) {
		CFTypeRef s = CFArrayGetValueAtIndex(arr, i);
		if (CFGetTypeID(s) != CFStringGetTypeID()) {
			continue;
		}
		char buf[64];
		if (!CFStringGetCString((CFStringRef)s, buf, sizeof buf, kCFStringEncodingUTF8)) {
			continue;
		}
		size_t n = strlen(buf);
		if (len + n + 2 > cap) {
			break;
		}
		memcpy(out + len, buf, n);
		len += n;
		out[len++] = '\n';
		out[len] = 0;
	}
	CFRelease(v);
	return out;
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

func appleLanguages() []string {
	cs := C.agtAppleLanguages()
	if cs == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cs))
	return strings.Fields(C.GoString(cs))
}
