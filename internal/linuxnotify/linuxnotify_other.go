//go:build !linux

package linuxnotify

// Notifier is unavailable outside Linux.
type Notifier struct{}

// New always fails outside Linux.
func New(string, func(string)) (*Notifier, error) { return nil, ErrUnavailable }

func (*Notifier) Post(Note) error { return ErrUnavailable }
func (*Notifier) Withdraw(string) {}
func (*Notifier) Close()          {}
