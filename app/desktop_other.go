//go:build !darwin

package main

import (
	"context"

	"github.com/elionaz/agenterm/internal/desktopnotify"
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

func installPreferencesMenu(string, func()) {}
func setPreferencesTitle(string)            {}
func installUpdatesMenu(string, string)     {}
func setUpdatesMenu(string, string)         {}
