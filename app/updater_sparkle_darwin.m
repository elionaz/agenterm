//go:build darwin && sparkle

// Objective-C half of updater_sparkle_darwin.go. Sparkle is weak-linked
// (see scripts/release-local.sh): without Sparkle.framework in the bundle
// its classes are nil and the updater stays off instead of crashing.
#import <Cocoa/Cocoa.h>
#import <Sparkle/Sparkle.h>
#include <stdlib.h>
#include <string.h>
#include "_cgo_export.h"

extern int (*agtUpdatesCanCheck)(void);

// Only touched on the main thread (Sparkle's API is main-thread only).
static SPUStandardUpdaterController *agtUpdater;

// The update Sparkle downloaded and will install on quit: running this
// installs it now and relaunches the app (Sparkle ≥ 2.3 allows repeated calls).
static void (^agtInstallNowBlock)(void);

// agtReport tells Go about the update found (nil: none any more).
static void agtReport(SUAppcastItem *item) {
	NSString *v = item.displayVersionString.length > 0 ? item.displayVersionString : item.versionString;
	agtSparkleUpdate(v.length > 0 ? (char *)v.UTF8String : NULL, agtInstallNowBlock != nil ? 1 : 0);
}

// Feeds the in-app banner (updates.available) and implements gentle
// reminders: a scheduled update shows Sparkle's window only when it would be
// in immediate focus (just launched, or idle); otherwise the banner is the
// reminder and its button brings that window up (checkForUpdates).
@interface AgtSparkleDelegate : NSObject <SPUUpdaterDelegate, SPUStandardUserDriverDelegate>
@end

@implementation AgtSparkleDelegate

- (void)updater:(SPUUpdater *)updater didFindValidUpdate:(SUAppcastItem *)item {
	agtReport(item);
}

- (void)updaterDidNotFindUpdate:(SPUUpdater *)updater {
	// A downloaded update is still installed on quit.
	if (agtInstallNowBlock == nil) {
		agtReport(nil);
	}
}

// Automatic downloads: the update is ready. Returning YES keeps the block
// usable ("Restart to update"); Sparkle still installs it on quit.
- (BOOL)updater:(SPUUpdater *)updater willInstallUpdateOnQuit:(SUAppcastItem *)item immediateInstallationBlock:(void (^)(void))immediateInstallHandler {
	agtInstallNowBlock = [immediateInstallHandler copy];
	agtReport(item);
	return YES;
}

- (void)updater:(SPUUpdater *)updater userDidMakeChoice:(SPUUserUpdateChoice)choice forUpdate:(SUAppcastItem *)updateItem state:(SPUUserUpdateState *)state {
	if (choice == SPUUserUpdateChoiceSkip) {
		agtReport(nil);
	}
}

- (BOOL)supportsGentleScheduledUpdateReminders {
	return YES;
}

- (BOOL)standardUserDriverShouldHandleShowingScheduledUpdate:(SUAppcastItem *)update andInImmediateFocus:(BOOL)immediateFocus {
	return immediateFocus;
}

- (void)standardUserDriverWillHandleShowingUpdate:(BOOL)handleShowingUpdate forUpdate:(SUAppcastItem *)update state:(SPUUserUpdateState *)state {
	if (!handleShowingUpdate) {
		agtReport(update);
	}
}

@end

// Sparkle references its delegates weakly.
static AgtSparkleDelegate *agtDelegate;

static void agtOnMain(dispatch_block_t b) {
	if ([NSThread isMainThread]) {
		b();
	} else {
		dispatch_sync(dispatch_get_main_queue(), b);
	}
}

static char *agtDup(NSString *s) {
	return s ? strdup(s.UTF8String) : NULL;
}

int agtSparkleLoaded(void) {
	return NSClassFromString(@"SPUStandardUpdaterController") != nil ? 1 : 0;
}

// agtBundleValue returns an Info.plist string (malloc'd; NULL if absent).
char *agtBundleValue(const char *key) {
	id v = [[NSBundle mainBundle] objectForInfoDictionaryKey:[NSString stringWithUTF8String:key]];
	return [v isKindOfClass:[NSString class]] ? agtDup(v) : NULL;
}

// agtBundlePath is the .app being run (malloc'd; NULL outside a bundle).
char *agtBundlePath(void) {
	NSString *p = [[NSBundle mainBundle] bundlePath];
	return [p hasSuffix:@".app"] ? agtDup(p) : NULL;
}

static int agtCanCheck(void) {
	__block BOOL ok = NO;
	agtOnMain(^{
		ok = agtUpdater.updater.canCheckForUpdates;
	});
	return ok ? 1 : 0;
}

// agtSparkleStart creates the updater controller on the main thread and
// starts it (scheduled checks per SUEnableAutomaticChecks /
// SUScheduledCheckInterval). It returns an error message (malloc'd) or NULL.
char *agtSparkleStart(void) {
	__block NSString *msg = nil;
	agtOnMain(^{
		Class cls = NSClassFromString(@"SPUStandardUpdaterController");
		if (cls == nil) {
			msg = @"Sparkle.framework not loaded";
			return;
		}
		agtDelegate = [AgtSparkleDelegate new];
		SPUStandardUpdaterController *c = [[cls alloc] initWithStartingUpdater:NO updaterDelegate:agtDelegate userDriverDelegate:agtDelegate];
		NSError *err = nil;
		if (![c.updater startUpdater:&err]) {
			msg = err.localizedDescription ?: @"startUpdater failed";
			return;
		}
		agtUpdater = c;
		agtUpdatesCanCheck = agtCanCheck;
	});
	return agtDup(msg);
}

void agtSparkleState(int *autoChecks, int *autoDownloads, int *canCheck, double *lastCheck) {
	__block BOOL a = NO, d = NO, c = NO;
	__block double last = 0;
	agtOnMain(^{
		SPUUpdater *u = agtUpdater.updater;
		a = u.automaticallyChecksForUpdates;
		d = u.automaticallyDownloadsUpdates;
		c = u.canCheckForUpdates;
		NSDate *l = u.lastUpdateCheckDate;
		last = l ? l.timeIntervalSince1970 : 0;
	});
	*autoChecks = a;
	*autoDownloads = d;
	*canCheck = c;
	*lastCheck = last;
}

void agtSparkleCheck(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (agtUpdater != nil) {
			[NSApp activateIgnoringOtherApps:YES];
			[agtUpdater checkForUpdates:nil];
		}
	});
}

// agtSparkleInstallNow installs the downloaded update and relaunches, or
// else shows Sparkle's window for the update found (or a fresh check).
void agtSparkleInstallNow(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (agtInstallNowBlock != nil) {
			agtInstallNowBlock();
			return;
		}
		if (agtUpdater != nil) {
			[NSApp activateIgnoringOtherApps:YES];
			[agtUpdater checkForUpdates:nil];
		}
	});
}

void agtSparkleSetAutomaticChecks(int on) {
	agtOnMain(^{
		agtUpdater.updater.automaticallyChecksForUpdates = on != 0;
	});
}

void agtSparkleSetAutomaticDownloads(int on) {
	agtOnMain(^{
		agtUpdater.updater.automaticallyDownloadsUpdates = on != 0;
	});
}
