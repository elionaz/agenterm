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
	if ([appMenu indexOfItemWithTag:agtPrefsTag] >= 0) {
		return;
	}
	if (agtPrefsTarget == nil) {
		agtPrefsTarget = [AgtPrefsTarget new];
	}
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:@"Preferencias…" action:@selector(openPreferences:) keyEquivalent:@","];
	item.keyEquivalentModifierMask = NSEventModifierFlagCommand;
	item.target = agtPrefsTarget;
	item.tag = agtPrefsTag;
	// Right after "About agenterm" + separator, as in every Mac app.
	NSInteger at = (appMenu.numberOfItems >= 2 && [[appMenu itemAtIndex:1] isSeparatorItem]) ? 2 : 0;
	[appMenu insertItem:item atIndex:at];
	[appMenu insertItem:[NSMenuItem separatorItem] atIndex:at + 1];
}

void agtInstallPreferencesItem(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		agtInsertPreferences(25);
	});
}
