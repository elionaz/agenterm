//go:build darwin && cgo

package macnotify

// The //export lives apart from the Objective-C definitions: cgo copies the
// preamble of files with exports into _cgo_export.c, so it may only hold
// declarations.

/*
#include <stdlib.h>
*/
import "C"

//export agtNotifyClicked
func agtNotifyClicked(sid *C.char) {
	id := C.GoString(sid)
	clickMu.Lock()
	f := onClick
	clickMu.Unlock()
	if f != nil {
		go func() {
			defer func() { _ = recover() }()
			f(id)
		}()
	}
}
