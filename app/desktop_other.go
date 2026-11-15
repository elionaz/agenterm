//go:build !darwin

package main

import (
	"context"

	"github.com/elionaz/agenterm/internal/desktopnotify"
)

type otherDesktop struct{}

func newDesktop(context.Context, func(string)) desktop { return otherDesktop{} }

func (otherDesktop) Notify(n note) error { return desktopnotify.Osascript(n.Title, n.Body, n.Sound) }
func (otherDesktop) Withdraw(string)     {}
func (otherDesktop) SetBadge(int)        {}
func (otherDesktop) RequestAttention()   {}
func (otherDesktop) IsActive() bool      { return true }
func (otherDesktop) Activate()           {}

func installPreferencesMenu(func()) {}
