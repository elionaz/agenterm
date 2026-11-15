//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>
#include <stdlib.h>
#include <string.h>

static int agtHasBundle(void) {
	return [[NSBundle mainBundle] bundleIdentifier] != nil;
}

// agtPost adds a UNNotificationRequest. Unlike Wails' SendNotification it
// lets us choose the sound. Clicks still reach Wails' notification center
// delegate (and so runtime.OnNotificationResponse). Returns NULL or a
// malloc'd error message.
static char *agtPost(const char *ident, const char *title, const char *body, const char *sessionID, int sound) {
	if (@available(macOS 10.14, *)) {
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
	}
	return strdup("notifications need macOS 10.14+");
}

// agtRemoveDelivered withdraws the notifications of a session (on ack).
static void agtRemoveDelivered(const char *ident) {
	if (@available(macOS 10.14, *)) {
		NSString *i = [NSString stringWithUTF8String:ident];
		[[UNUserNotificationCenter currentNotificationCenter] removeDeliveredNotificationsWithIdentifiers:@[i]];
	}
}

static void agtSetBadge(const char *label) {
	NSString *l = label[0] ? [NSString stringWithUTF8String:label] : nil;
	dispatch_async(dispatch_get_main_queue(), ^{
		[[NSApp dockTile] setBadgeLabel:l];
	});
}

static void agtRequestAttention(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp requestUserAttention:NSInformationalRequest];
	});
}

static int agtIsActive(void) {
	return [NSApp isActive] ? 1 : 0;
}

static void agtActivate(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp activateIgnoringOtherApps:YES];
	});
}
*/
import "C"

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync/atomic"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/desktopnotify"
)

// macDesktop implements desktop with UNUserNotificationCenter (through the
// delegate Wails installs) plus Dock badge/bounce, falling back to
// osascript when the app has no bundle or notifications are not
// authorized.
type macDesktop struct {
	native atomic.Bool // UNUserNotificationCenter usable and authorized
}

func newDesktop(ctx context.Context, onClick func(sessionID string)) desktop {
	d := &macDesktop{}
	if C.agtHasBundle() == 0 {
		log.Printf("agenterm: no bundle identifier; using osascript notifications")
		return d
	}
	if err := runtime.InitializeNotifications(ctx); err != nil {
		log.Printf("agenterm: notifications: %v; using osascript", err)
		return d
	}
	runtime.OnNotificationResponse(ctx, func(r runtime.NotificationResult) {
		if r.Error != nil {
			return
		}
		if sid, ok := r.Response.UserInfo["sessionId"].(string); ok && sid != "" {
			onClick(sid)
		}
	})
	// First launch shows the system prompt (which may wait on the user);
	// later launches return the stored decision immediately. Until then,
	// and if denied, notifications go through osascript.
	go func() {
		ok, err := runtime.RequestNotificationAuthorization(ctx)
		if err != nil || !ok {
			log.Printf("agenterm: notifications not authorized (%v); using osascript", err)
			return
		}
		d.native.Store(true)
	}()
	return d
}

func notificationID(sessionID string) string { return "agenterm-" + sessionID }

func (d *macDesktop) Notify(n note) error {
	if d.native.Load() {
		id, title, body, sid := C.CString(notificationID(n.SessionID)), C.CString(n.Title), C.CString(n.Body), C.CString(n.SessionID)
		defer C.free(unsafe.Pointer(id))
		defer C.free(unsafe.Pointer(title))
		defer C.free(unsafe.Pointer(body))
		defer C.free(unsafe.Pointer(sid))
		sound := C.int(0)
		if n.Sound {
			sound = 1
		}
		cerr := C.agtPost(id, title, body, sid, sound)
		if cerr == nil {
			return nil
		}
		err := errors.New(C.GoString(cerr))
		C.free(unsafe.Pointer(cerr))
		log.Printf("agenterm: notification: %v; falling back to osascript", err)
	}
	return desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}

func (d *macDesktop) Withdraw(sessionID string) {
	if !d.native.Load() {
		return
	}
	id := C.CString(notificationID(sessionID))
	defer C.free(unsafe.Pointer(id))
	C.agtRemoveDelivered(id)
}

func (d *macDesktop) SetBadge(n int) {
	label := ""
	if n > 0 {
		label = strconv.Itoa(n)
	}
	l := C.CString(label)
	defer C.free(unsafe.Pointer(l))
	C.agtSetBadge(l)
}

func (d *macDesktop) RequestAttention() { C.agtRequestAttention() }
func (d *macDesktop) IsActive() bool    { return C.agtIsActive() != 0 }
func (d *macDesktop) Activate()         { C.agtActivate() }
