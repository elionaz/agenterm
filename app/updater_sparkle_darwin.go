//go:build darwin && sparkle

package main

// Built with -tags sparkle by scripts/release-local.sh, which also passes
// the flags to find and weak-link the framework (not allowed in #cgo
// directives): CGO_CFLAGS/CGO_LDFLAGS "-F <Sparkle dir>",
// "-Wl,-weak_framework,Sparkle", "-Wl,-rpath,@executable_path/../Frameworks".

/*
#include <stdlib.h>
int agtSparkleLoaded(void);
char *agtBundleValue(const char *key);
char *agtBundlePath(void);
char *agtSparkleStart(void);
void agtSparkleState(int *autoChecks, int *autoDownloads, int *canCheck, double *lastCheck);
void agtSparkleCheck(void);
int agtSparkleInstallNow(void);
void agtSparkleSetAutomaticChecks(int on);
void agtSparkleSetAutomaticDownloads(int on);
*/
import "C"

import (
	"errors"
	"log"
	"math"
	"sync"
	"time"
	"unsafe"
)

const sparkleBuilt = true

func goString(p *C.char) string {
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

func bundleValue(key string) string {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	return goString(C.agtBundleValue(k))
}

func sparkleBundle() bundleInfo {
	return bundleInfo{
		Path:        goString(C.agtBundlePath()),
		Version:     bundleValue("CFBundleShortVersionString"),
		PublicEDKey: bundleValue("SUPublicEDKey"),
		HasSparkle:  C.agtSparkleLoaded() != 0,
	}
}

// Sparkle's reports, handled in order off the main thread (emitting a Wails
// event from it could wait on the main thread itself).
var (
	sparkleUpdates     = make(chan *UpdateInfo, 16)
	sparkleUpdatesOnce sync.Once
)

// agtSparkleUpdate is called by the delegate (on the main thread) with the
// update found, or NULL when there is none any more.
//
//export agtSparkleUpdate
func agtSparkleUpdate(version *C.char, ready C.int) {
	var u *UpdateInfo
	if version != nil {
		u = &UpdateInfo{Version: C.GoString(version), Ready: ready != 0}
	}
	sparkleUpdatesOnce.Do(func() {
		go func() {
			for u := range sparkleUpdates {
				if f := onSparkleUpdate.Load(); f != nil {
					(*f)(u)
				}
			}
		}()
	})
	select {
	case sparkleUpdates <- u:
	default:
		log.Printf("agenterm: updater: dropped an update report")
	}
}

func sparkleStart() error {
	if msg := goString(C.agtSparkleStart()); msg != "" {
		return errors.New(msg)
	}
	return nil
}

func sparkleState() sparkleStatus {
	var a, d, c C.int
	var last C.double
	C.agtSparkleState(&a, &d, &c, &last)
	s := sparkleStatus{auto: a != 0, autoDownload: d != 0, canCheck: c != 0}
	if last > 0 {
		sec, frac := math.Modf(float64(last))
		s.lastCheck = time.Unix(int64(sec), int64(frac*1e9))
	}
	return s
}

func sparkleCheck() { C.agtSparkleCheck() }

// sparkleInstallNow is false while Sparkle is busy downloading.
func sparkleInstallNow() bool { return C.agtSparkleInstallNow() != 0 }

func sparkleSetAutomaticChecks(on bool)    { C.agtSparkleSetAutomaticChecks(cBool(on)) }
func sparkleSetAutomaticDownloads(on bool) { C.agtSparkleSetAutomaticDownloads(cBool(on)) }

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
