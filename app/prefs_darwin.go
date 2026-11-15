//go:build darwin

package main

/*
void agtInstallPreferencesItem(void);
*/
import "C"

var onPreferences func()

//export agtPreferencesClicked
func agtPreferencesClicked() {
	if f := onPreferences; f != nil {
		go f()
	}
}

// installPreferencesMenu adds "Preferencias… ⌘," to the app menu that Wails
// builds from menu.AppMenu() (whose items cannot be extended from Go).
func installPreferencesMenu(f func()) {
	onPreferences = f
	C.agtInstallPreferencesItem()
}
