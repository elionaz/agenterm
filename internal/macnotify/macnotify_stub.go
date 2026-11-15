//go:build !darwin || !cgo

package macnotify

import "time"

func HasBundle() bool                             { return false }
func AuthStatus(time.Duration) Status             { return StatusUnknown }
func RequestAuth(time.Duration) (bool, error)     { return false, ErrUnavailable }
func Post(Note) error                             { return ErrUnavailable }
func RemoveDelivered(string)                      {}
func SetClickHandler(func(sessionID string)) bool { return false }
func RunMainLoop(work func())                     { work() }
