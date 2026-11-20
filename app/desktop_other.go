//go:build !darwin && !linux

package main

import (
	"context"
	"errors"

	"github.com/elionaz/panal/internal/desktopnotify"
	"github.com/elionaz/panal/internal/macnotify"
)

type otherDesktop struct{}

func newDesktop(context.Context, func(string)) desktop { return otherDesktop{} }

func (otherDesktop) Notify(n note) error {
	if !n.Osascript {
		return nil
	}
	return desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}
func (otherDesktop) Withdraw(string)   {}
func (otherDesktop) SetBadge(int)      {}
func (otherDesktop) RequestAttention() {}
func (otherDesktop) IsActive() bool    { return true }
func (otherDesktop) Activate()         {}
func (otherDesktop) Refresh() (macnotify.Status, string) {
	return macnotify.StatusUnknown, "no native notifications on this platform"
}
func (otherDesktop) Post(n note) error {
	if !n.Osascript {
		return errors.New("no native notifications on this platform")
	}
	return desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}
