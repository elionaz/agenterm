// Objective-C half of prefs_darwin.go (a separate file because cgo
// preambles of files with //export may not hold definitions).
#import <Cocoa/Cocoa.h>
#include "_cgo_export.h"

@interface AgtPrefsTarget : NSObject
- (void)openPreferences:(id)sender;
@end

@implementation AgtPrefsTarget
- (void)openPreferences:(id)sender {
	agtPreferencesClicked();
}
@end

static AgtPrefsTarget *agtPrefsTarget;
static const NSInteger agtPrefsTag = 0x61677470;
// The item's title; only touched on the main queue.
static NSString *agtPrefsTitle = @"Preferences…";

// The menu bar is built asynchronously by Wails: retry for a few seconds.
static void agtInsertPreferences(int attempts) {
	NSMenu *bar = [NSApp mainMenu];
	NSMenu *appMenu = bar.numberOfItems > 0 ? [[bar itemAtIndex:0] submenu] : nil;
	if (appMenu == nil) {
		if (attempts > 0) {
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 200 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
				agtInsertPreferences(attempts - 1);
			});
		}
		return;
	}
	NSInteger existing = [appMenu indexOfItemWithTag:agtPrefsTag];
	if (existing >= 0) {
		[[appMenu itemAtIndex:existing] setTitle:agtPrefsTitle];
		return;
	}
	if (agtPrefsTarget == nil) {
		agtPrefsTarget = [AgtPrefsTarget new];
	}
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:agtPrefsTitle action:@selector(openPreferences:) keyEquivalent:@","];
	item.keyEquivalentModifierMask = NSEventModifierFlagCommand;
	item.target = agtPrefsTarget;
	item.tag = agtPrefsTag;
	// Right after "About agenterm" (and "Check for Updates…") + separator,
	// as in every Mac app.
	NSInteger at = 0;
	for (NSInteger i = 1; i < appMenu.numberOfItems && i <= 2; i++) {
		if ([[appMenu itemAtIndex:i] isSeparatorItem]) {
			at = i + 1;
			break;
		}
	}
	[appMenu insertItem:item atIndex:at];
	[appMenu insertItem:[NSMenuItem separatorItem] atIndex:at + 1];
}

void agtInstallPreferencesItem(const char *title) {
	NSString *t = [NSString stringWithUTF8String:title];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (t.length > 0) {
			agtPrefsTitle = t;
		}
		agtInsertPreferences(25);
	});
}

// agtSetPreferencesTitle retitles the item (or sets the title it will get
// once inserted).
void agtSetPreferencesTitle(const char *title) {
	NSString *t = [NSString stringWithUTF8String:title];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (t.length == 0) {
			return;
		}
		agtPrefsTitle = t;
		NSMenu *bar = [NSApp mainMenu];
		NSMenu *appMenu = bar.numberOfItems > 0 ? [[bar itemAtIndex:0] submenu] : nil;
		NSInteger i = appMenu ? [appMenu indexOfItemWithTag:agtPrefsTag] : -1;
		if (i >= 0) {
			[[appMenu itemAtIndex:i] setTitle:t];
		}
	});
}
