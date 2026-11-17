//go:build !darwin || !sparkle

package main

// Builds without Sparkle (development, tests, other OSes): the updater is
// off with reason "unsupported".

const sparkleBuilt = false

func sparkleBundle() bundleInfo            { return bundleInfo{} }
func sparkleStart() error                  { return nil }
func sparkleState() sparkleStatus          { return sparkleStatus{} }
func sparkleCheck()                        {}
func sparkleInstallNow()                   {}
func sparkleSetAutomaticChecks(on bool)    {}
func sparkleSetAutomaticDownloads(on bool) {}
