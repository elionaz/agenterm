//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

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
	"log"
	"strconv"
	"sync/atomic"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/macnotify"
)

// macDesktop implements desktop with UNUserNotificationCenter (posting via
// internal/macnotify, shared with the daemon; clicks through the delegate
// Wails installs) plus Dock badge/bounce, falling back to
// osascript when the app has no bundle or notifications are not
// authorized.
type macDesktop struct {
	native atomic.Bool // UNUserNotificationCenter usable and authorized
}

func newDesktop(ctx context.Context, onClick func(sessionID string)) desktop {
	d := &macDesktop{}
	if !macnotify.HasBundle() {
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

func (d *macDesktop) Notify(n note) error {
	if d.native.Load() {
		// Unlike Wails' SendNotification this lets us choose the sound.
		err := macnotify.Post(macnotify.Note{ID: macnotify.NotificationID(n.SessionID), Title: n.Title, Body: n.Body,
			SessionID: n.SessionID, Sound: n.Sound})
		if err == nil {
			return nil
		}
		log.Printf("agenterm: notification: %v; falling back to osascript", err)
	}
	return desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}

// Withdraw also removes what the daemon posted while the app was closed
// (same bundle, same identifier).
func (d *macDesktop) Withdraw(sessionID string) {
	if d.native.Load() {
		macnotify.RemoveDelivered(macnotify.NotificationID(sessionID))
	}
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
