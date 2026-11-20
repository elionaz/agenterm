//go:build darwin

// Objective-C half of updatesmenu_darwin.go: "Check for Updates…" in the
// app menu, right below "About panal" (as Sparkle apps do).
#import <Cocoa/Cocoa.h>
#include "_cgo_export.h"

// Set by updater_sparkle_darwin.m once Sparkle runs (nil otherwise): whether
// a check may start now (not while one is in progress).
int (*agtUpdatesCanCheck)(void);

@interface AgtUpdatesTarget : NSObject <NSMenuItemValidation>
- (void)checkForUpdates:(id)sender;
@end

static NSString *agtUpdatesTitle = @"Check for Updates…";
// Non-empty: the updater is off and a click explains why instead.
static NSString *agtUpdatesHint = @"";
static AgtUpdatesTarget *agtUpdatesTarget;
static const NSInteger agtUpdatesTag = 0x61677475;

@implementation AgtUpdatesTarget
- (void)checkForUpdates:(id)sender {
	if (agtUpdatesHint.length > 0) {
		NSAlert *a = [NSAlert new];
		a.messageText = agtUpdatesTitle;
		a.informativeText = agtUpdatesHint;
		[a runModal];
		return;
	}
	agtCheckUpdatesClicked();
}

- (BOOL)validateMenuItem:(NSMenuItem *)item {
	if (agtUpdatesHint.length > 0 || agtUpdatesCanCheck == NULL) {
		return YES;
	}
	return agtUpdatesCanCheck() != 0;
}
@end

static NSMenu *agtAppMenu(void) {
	NSMenu *bar = [NSApp mainMenu];
	return bar.numberOfItems > 0 ? [[bar itemAtIndex:0] submenu] : nil;
}

// The menu bar is built asynchronously by Wails: retry for a few seconds.
static void agtInsertUpdates(int attempts) {
	NSMenu *appMenu = agtAppMenu();
	if (appMenu == nil) {
		if (attempts > 0) {
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 200 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
				agtInsertUpdates(attempts - 1);
			});
		}
		return;
	}
	NSInteger existing = [appMenu indexOfItemWithTag:agtUpdatesTag];
	if (existing >= 0) {
		[[appMenu itemAtIndex:existing] setTitle:agtUpdatesTitle];
		return;
	}
	if (agtUpdatesTarget == nil) {
		agtUpdatesTarget = [AgtUpdatesTarget new];
	}
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:agtUpdatesTitle action:@selector(checkForUpdates:) keyEquivalent:@""];
	item.target = agtUpdatesTarget;
	item.tag = agtUpdatesTag;
	[appMenu insertItem:item atIndex:appMenu.numberOfItems > 0 ? 1 : 0];
}

static void agtApplyUpdates(NSString *title, NSString *hint, BOOL insert) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (title.length > 0) {
			agtUpdatesTitle = title;
		}
		agtUpdatesHint = hint ?: @"";
		if (insert) {
			agtInsertUpdates(25);
			return;
		}
		NSMenu *appMenu = agtAppMenu();
		NSInteger i = appMenu ? [appMenu indexOfItemWithTag:agtUpdatesTag] : -1;
		if (i >= 0) {
			[[appMenu itemAtIndex:i] setTitle:agtUpdatesTitle];
		}
	});
}

void agtInstallUpdatesItem(const char *title, const char *hint) {
	agtApplyUpdates([NSString stringWithUTF8String:title], [NSString stringWithUTF8String:hint], YES);
}

// agtSetUpdatesItem retitles the item (UI language changed).
void agtSetUpdatesItem(const char *title, const char *hint) {
	agtApplyUpdates([NSString stringWithUTF8String:title], [NSString stringWithUTF8String:hint], NO);
}
