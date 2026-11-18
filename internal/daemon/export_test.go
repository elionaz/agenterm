package daemon

import "time"

// SetAutoTiming shortens the automatic-delivery delays for a test; the
// returned func restores them.
func SetAutoTiming(delay, quiet, retry time.Duration) func() {
	d, q, r := autoDelay, autoQuiet, autoRetry
	autoDelay, autoQuiet, autoRetry = delay, quiet, retry
	return func() { autoDelay, autoQuiet, autoRetry = d, q, r }
}

// AutoChainMax is the automatic-delivery chain limit.
const AutoChainMax = autoChainMax
