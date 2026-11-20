//go:build linux

package linuxnotify

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName = "org.freedesktop.Notifications"
	busPath = "/org/freedesktop/Notifications"
)

// Notifier holds the session-bus connection.
type Notifier struct {
	app  string
	conn *dbus.Conn
	obj  dbus.BusObject

	mu    sync.Mutex
	byKey map[string]uint32
	byID  map[uint32]string
}

// New connects to the session bus; onClick (may be nil) gets the key of a
// clicked notification.
func New(app string, onClick func(key string)) (*Notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	n := &Notifier{app: app, conn: conn, obj: conn.Object(busName, busPath), byKey: map[string]uint32{}, byID: map[uint32]string{}}
	if err := n.obj.Call("org.freedesktop.Notifications.GetServerInformation", 0).Err; err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if onClick != nil {
		_ = conn.AddMatchSignal(dbus.WithMatchInterface(busName), dbus.WithMatchMember("ActionInvoked"))
		_ = conn.AddMatchSignal(dbus.WithMatchInterface(busName), dbus.WithMatchMember("NotificationClosed"))
		ch := make(chan *dbus.Signal, 16)
		conn.Signal(ch)
		go func() {
			for sig := range ch {
				if len(sig.Body) < 1 {
					continue
				}
				id, _ := sig.Body[0].(uint32)
				n.mu.Lock()
				key := n.byID[id]
				if sig.Name == busName+".NotificationClosed" {
					delete(n.byID, id)
					if n.byKey[key] == id {
						delete(n.byKey, key)
					}
				}
				n.mu.Unlock()
				if sig.Name == busName+".ActionInvoked" && key != "" {
					onClick(key)
				}
			}
		}()
	}
	return n, nil
}

// Post shows note, replacing the previous one with its key.
func (n *Notifier) Post(note Note) error {
	n.mu.Lock()
	replaces := n.byKey[note.Key]
	n.mu.Unlock()
	hints := map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant("agenterm")}
	if note.Sound {
		hints["sound-name"] = dbus.MakeVariant("message-new-instant")
	} else {
		hints["suppress-sound"] = dbus.MakeVariant(true)
	}
	var id uint32
	err := n.obj.Call("org.freedesktop.Notifications.Notify", 0, n.app, replaces, "agenterm", note.Title, note.Body,
		[]string{"default", "Open"}, hints, int32(-1)).Store(&id)
	if err != nil {
		return err
	}
	if note.Key != "" {
		n.mu.Lock()
		n.byKey[note.Key] = id
		n.byID[id] = note.Key
		n.mu.Unlock()
	}
	return nil
}

// Withdraw closes the notification with key, if any.
func (n *Notifier) Withdraw(key string) {
	n.mu.Lock()
	id, ok := n.byKey[key]
	delete(n.byKey, key)
	n.mu.Unlock()
	if ok {
		_ = n.obj.Call("org.freedesktop.Notifications.CloseNotification", 0, id).Err
	}
}

// Close drops the bus connection.
func (n *Notifier) Close() { n.conn.Close() }
