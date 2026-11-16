//go:build darwin && sparkle

// Objective-C half of updater_sparkle_darwin.go. Sparkle is weak-linked
// (see scripts/release-local.sh): without Sparkle.framework in the bundle
// its classes are nil and the updater stays off instead of crashing.
#import <Cocoa/Cocoa.h>
#import <Sparkle/Sparkle.h>
#include <stdlib.h>
#include <string.h>

extern int (*agtUpdatesCanCheck)(void);

// Only touched on the main thread (Sparkle's API is main-thread only).
static SPUStandardUpdaterController *agtUpdater;

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
		SPUStandardUpdaterController *c = [[cls alloc] initWithStartingUpdater:NO updaterDelegate:nil userDriverDelegate:nil];
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
