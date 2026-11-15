//go:build darwin && cgo

package macnotify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications -framework CoreFoundation
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

extern void agtNotifyClicked(char *sessionID);

static int agtHasBundle(void) {
	@try {
		return [[NSBundle mainBundle] bundleIdentifier] != nil;
	} @catch (NSException *e) {
		return 0;
	}
}

// agtAuthStatus: -1 unknown/timeout, 0 not determined, 1 denied, 2 authorized.
static int agtAuthStatus(double timeout) {
	if (@available(macOS 10.14, *)) {
		@try {
			dispatch_semaphore_t sem = dispatch_semaphore_create(0);
			__block int st = -1;
			[[UNUserNotificationCenter currentNotificationCenter] getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
				switch (s.authorizationStatus) {
				case UNAuthorizationStatusNotDetermined: st = 0; break;
				case UNAuthorizationStatusDenied: st = 1; break;
				default: st = 2; break; // authorized, provisional, ephemeral
				}
				dispatch_semaphore_signal(sem);
			}];
			if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeout * NSEC_PER_SEC))) != 0) {
				return -1;
			}
			return st;
		} @catch (NSException *e) {
			return -1;
		}
	}
	return -1;
}

// agtRequestAuth asks for alert+sound+badge. Returns 1 if granted; *err is
// NULL or a malloc'd message.
static int agtRequestAuth(double timeout, char **err) {
	*err = NULL;
	if (@available(macOS 10.14, *)) {
		@try {
			dispatch_semaphore_t sem = dispatch_semaphore_create(0);
			__block int ok = 0;
			__block char *msg = NULL;
			UNAuthorizationOptions opts = UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge;
			[[UNUserNotificationCenter currentNotificationCenter] requestAuthorizationWithOptions:opts completionHandler:^(BOOL granted, NSError *e) {
				ok = granted ? 1 : 0;
				if (e) {
					msg = strdup([[e localizedDescription] UTF8String]);
				}
				dispatch_semaphore_signal(sem);
			}];
			if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeout * NSEC_PER_SEC))) != 0) {
				*err = strdup("timed out waiting for authorization");
				return 0;
			}
			*err = msg;
			return ok;
		} @catch (NSException *e) {
			*err = strdup([[e reason] UTF8String] ?: "exception");
			return 0;
		}
	}
	*err = strdup("notifications need macOS 10.14+");
	return 0;
}

// agtPost adds a UNNotificationRequest (bounded to 3 s). Returns NULL or a
// malloc'd error message.
static char *agtPost(const char *ident, const char *title, const char *body, const char *sessionID, int sound) {
	if (@available(macOS 10.14, *)) {
		@try {
			UNMutableNotificationContent *c = [[UNMutableNotificationContent alloc] init];
			c.title = [NSString stringWithUTF8String:title];
			c.body = [NSString stringWithUTF8String:body];
			NSString *sid = [NSString stringWithUTF8String:sessionID];
			c.userInfo = @{@"sessionId": sid};
			c.threadIdentifier = sid;
			if (sound) {
				c.sound = [UNNotificationSound defaultSound];
			}
			UNNotificationRequest *req = [UNNotificationRequest requestWithIdentifier:[NSString stringWithUTF8String:ident] content:c trigger:nil];
			dispatch_semaphore_t sem = dispatch_semaphore_create(0);
			__block char *err = NULL;
			[[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:req withCompletionHandler:^(NSError *e) {
				if (e) {
					err = strdup([[e localizedDescription] UTF8String]);
				}
				dispatch_semaphore_signal(sem);
			}];
			if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC)) != 0) {
				return strdup("timed out posting notification");
			}
			return err;
		} @catch (NSException *e) {
			return strdup([[e reason] UTF8String] ?: "exception");
		}
	}
	return strdup("notifications need macOS 10.14+");
}

static void agtRemoveDelivered(const char *ident) {
	if (@available(macOS 10.14, *)) {
		@try {
			NSString *i = [NSString stringWithUTF8String:ident];
			[[UNUserNotificationCenter currentNotificationCenter] removeDeliveredNotificationsWithIdentifiers:@[i]];
		} @catch (NSException *e) {
		}
	}
}

// AgtNotifyDelegate forwards clicks (default action) to Go.
API_AVAILABLE(macos(10.14))
@interface AgtNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation AgtNotifyDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
	if (@available(macOS 11.0, *)) {
		completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
	} else {
		completionHandler(UNNotificationPresentationOptionSound);
	}
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))completionHandler {
	@try {
		id sid = response.notification.request.content.userInfo[@"sessionId"];
		if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier] &&
			[sid isKindOfClass:[NSString class]]) {
			agtNotifyClicked((char *)[(NSString *)sid UTF8String]);
		}
	} @catch (NSException *e) {
	}
	completionHandler();
}
@end

static id agtDelegate; // strong: the center only keeps a weak reference

static int agtInstallDelegate(void) {
	if (@available(macOS 10.14, *)) {
		@try {
			agtDelegate = [[AgtNotifyDelegate alloc] init];
			[UNUserNotificationCenter currentNotificationCenter].delegate = agtDelegate;
			return 1;
		} @catch (NSException *e) {
			return 0;
		}
	}
	return 0;
}

static int agtIsMainThread(void) { return pthread_main_np(); }

static volatile int agtStop;

static void agtNoop(CFRunLoopTimerRef t, void *info) {}

// agtRunMain services the main run loop (and so the main dispatch queue,
// where UserNotifications delivers delegate callbacks) until agtStopMain.
static void agtRunMain(void) {
	// A run loop without sources returns at once: keep a dormant timer.
	CFRunLoopTimerRef t = CFRunLoopTimerCreate(NULL, CFAbsoluteTimeGetCurrent() + 1e9, 1e9, 0, 0, agtNoop, NULL);
	CFRunLoopAddTimer(CFRunLoopGetCurrent(), t, kCFRunLoopDefaultMode);
	while (!__atomic_load_n(&agtStop, __ATOMIC_SEQ_CST)) {
		@autoreleasepool {
			CFRunLoopRunInMode(kCFRunLoopDefaultMode, 1.0, false);
		}
	}
	CFRunLoopRemoveTimer(CFRunLoopGetCurrent(), t, kCFRunLoopDefaultMode);
	CFRelease(t);
}

static void agtStopMain(void) {
	__atomic_store_n(&agtStop, 1, __ATOMIC_SEQ_CST);
	CFRunLoopStop(CFRunLoopGetMain());
	CFRunLoopWakeUp(CFRunLoopGetMain());
}
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// Keep the main goroutine on the main OS thread, so RunMainLoop (called
// from it) can serve the Cocoa main run loop. Harmless for every other use
// (the Wails app does the same).
func init() { runtime.LockOSThread() }

// HasBundle reports whether this process has a bundle identifier, which
// UNUserNotificationCenter requires (it raises otherwise).
func HasBundle() bool { return C.agtHasBundle() != 0 }

// AuthStatus queries the authorization state, waiting at most timeout.
func AuthStatus(timeout time.Duration) Status {
	if !HasBundle() {
		return StatusUnknown
	}
	switch C.agtAuthStatus(C.double(timeout.Seconds())) {
	case 0:
		return StatusNotDetermined
	case 1:
		return StatusDenied
	case 2:
		return StatusAuthorized
	}
	return StatusUnknown
}

// RequestAuth asks the user for permission (the system prompt shows only
// the first time), waiting at most timeout.
func RequestAuth(timeout time.Duration) (bool, error) {
	if !HasBundle() {
		return false, ErrUnavailable
	}
	var cerr *C.char
	ok := C.agtRequestAuth(C.double(timeout.Seconds()), &cerr) != 0
	if cerr != nil {
		err := errors.New(C.GoString(cerr))
		C.free(unsafe.Pointer(cerr))
		return ok, err
	}
	return ok, nil
}

// Post shows n. It blocks for at most ~3 s.
func Post(n Note) error {
	if !HasBundle() {
		return ErrUnavailable
	}
	id, title, body, sid := C.CString(n.ID), C.CString(n.Title), C.CString(n.Body), C.CString(n.SessionID)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(body))
	defer C.free(unsafe.Pointer(sid))
	sound := C.int(0)
	if n.Sound {
		sound = 1
	}
	if cerr := C.agtPost(id, title, body, sid, sound); cerr != nil {
		err := errors.New(C.GoString(cerr))
		C.free(unsafe.Pointer(cerr))
		return err
	}
	return nil
}

// RemoveDelivered withdraws delivered notifications with that identifier.
func RemoveDelivered(id string) {
	if !HasBundle() {
		return
	}
	c := C.CString(id)
	defer C.free(unsafe.Pointer(c))
	C.agtRemoveDelivered(c)
}

var (
	clickMu sync.Mutex
	onClick func(sessionID string)
)

// SetClickHandler installs this package's notification center delegate,
// calling f (on its own goroutine) with the session id of a clicked
// notification. Not for the Wails app, whose own delegate it would replace.
// Delegate callbacks arrive on the main queue: the process must serve it
// (RunMainLoop). It reports whether the delegate was installed.
func SetClickHandler(f func(sessionID string)) bool {
	if !HasBundle() {
		return false
	}
	clickMu.Lock()
	onClick = f
	clickMu.Unlock()
	return C.agtInstallDelegate() != 0
}

// RunMainLoop runs work on another goroutine while the calling (main)
// thread serves the Cocoa main run loop, and returns when work does. Called
// off the main thread it just runs work.
func RunMainLoop(work func()) {
	if C.agtIsMainThread() == 0 {
		work()
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer C.agtStopMain()
		work()
	}()
	C.agtRunMain()
	<-done
}
