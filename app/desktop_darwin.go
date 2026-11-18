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
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/macnotify"
)

// macDesktop implements desktop with UNUserNotificationCenter (posting via
// internal/macnotify, shared with the daemon; clicks through the delegate
// Wails installs) plus Dock badge/bounce. When the app has no bundle or
// notifications are not authorized, notifications are dropped (logged once
// per reason) or, with notify.fallback: osascript, sent through osascript.
// AGENTERM_NOTIFY=off disables them entirely (the Dock badge stays).
type macDesktop struct {
	off    bool         // AGENTERM_NOTIFY=off
	native atomic.Bool  // UNUserNotificationCenter usable and authorized
	inited atomic.Bool  // UNUserNotificationCenter initialized (Refresh may enable native)
	reason atomic.Value // string: why native is not usable
	// checked is when a dropped notification last re-read the permission
	// (unix nanos), so a permission granted later is picked up without a
	// relaunch, at most once per recheckEvery.
	checked atomic.Int64
	logged  sync.Map // reasons already logged for dropped notifications
}

func newDesktop(ctx context.Context, onClick func(sessionID string)) desktop {
	d := &macDesktop{off: desktopnotify.Disabled()}
	if d.off {
		log.Printf("agenterm: notifications disabled (%s=off)", desktopnotify.EnvNotify)
		return d
	}
	d.reason.Store("not authorized yet")
	if !macnotify.HasBundle() {
		d.reason.Store("no bundle identifier")
		return d
	}
	if err := runtime.InitializeNotifications(ctx); err != nil {
		d.reason.Store("notifications: " + err.Error())
		return d
	}
	d.inited.Store(true)
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
	// and if denied, notifications go to the fallback (notify.fallback).
	go func() {
		ok, err := runtime.RequestNotificationAuthorization(ctx)
		if err != nil || !ok {
			if !d.native.Load() { // a Refresh may have found it granted meanwhile
				d.reason.Store(fmt.Sprintf("not authorized (%v)", err))
			}
			return
		}
		d.native.Store(true)
		d.reason.Store("")
	}()
	return d
}

// recheckEvery bounds how often a dropped notification re-reads the
// permission.
const recheckEvery = 30 * time.Second

// Refresh re-reads the permission: granted in System Settings after launch,
// notifications start working without a relaunch (and stop if revoked).
func (d *macDesktop) Refresh() (macnotify.Status, string) {
	if d.off {
		return macnotify.StatusUnknown, "off (" + desktopnotify.EnvNotify + "=off)"
	}
	if !d.inited.Load() {
		r, _ := d.reason.Load().(string)
		return macnotify.StatusUnknown, r
	}
	d.checked.Store(time.Now().UnixNano())
	st := macnotify.AuthStatus(2 * time.Second)
	switch st {
	case macnotify.StatusAuthorized:
		d.native.Store(true)
		d.reason.Store("")
	case macnotify.StatusUnknown: // a failed query leaves things as they are
	default:
		d.native.Store(false)
		d.reason.Store("not authorized (" + st.String() + ")")
	}
	if d.native.Load() {
		return st, ""
	}
	r, _ := d.reason.Load().(string)
	return st, r
}

// Post shows n now (native, else the osascript fallback when n asks for
// it) and reports what failed.
func (d *macDesktop) Post(n note) error {
	if d.off {
		return errors.New("notifications are off (" + desktopnotify.EnvNotify + "=off)")
	}
	if _, reason := d.Refresh(); reason == "" {
		return macnotify.Post(macnotify.Note{ID: macnotify.NotificationID(n.SessionID), Title: n.Title, Body: n.Body,
			SessionID: n.SessionID, Sound: n.Sound})
	} else if !n.Osascript {
		return errors.New(reason)
	}
	return desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}

func (d *macDesktop) Notify(n note) error {
	if d.off {
		return nil
	}
	if !d.native.Load() && d.inited.Load() {
		// One goroutine claims the re-check; a burst doesn't pile up.
		prev, now := d.checked.Load(), time.Now().UnixNano()
		if now-prev > int64(recheckEvery) && d.checked.CompareAndSwap(prev, now) {
			d.Refresh()
		}
	}
	reason, _ := d.reason.Load().(string)
	if d.native.Load() {
		// Unlike Wails' SendNotification this lets us choose the sound.
		err := macnotify.Post(macnotify.Note{ID: macnotify.NotificationID(n.SessionID), Title: n.Title, Body: n.Body,
			SessionID: n.SessionID, Sound: n.Sound})
		if err == nil {
			return nil
		}
		reason = "native notification failed: " + err.Error()
	}
	if !n.Osascript {
		if _, seen := d.logged.LoadOrStore(reason, true); !seen {
			log.Printf("agenterm: notifications not shown (%s); set notify.fallback: osascript to use AppleScript (shows as Script Editor)", reason)
		}
		return nil
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
