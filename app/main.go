// Command agenterm (desktop) is the Wails app. The same executable doubles as
// the daemon (`agenterm daemon`, what client.StartDaemon runs) and as the
// hook callback (`agenterm notify`); neither opens a window, so the .app is
// self-contained.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/daemoncmd"
	"github.com/elionaz/agenterm/internal/notifycmd"
)

// dist/.gitkeep keeps this embed valid (and `go build ./...` green) before
// the frontend has been built.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == daemoncmd.Name {
		if err := daemoncmd.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "agenterm daemon:", err)
			os.Exit(1)
		}
		return
	}
	// Agent hooks call back the daemon's executable, which may be this one.
	if len(os.Args) > 1 && os.Args[1] == notifycmd.Name {
		if err := notifycmd.Run(os.Args[2:], os.Stdin, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, "agenterm notify:", err)
			os.Exit(1)
		}
		return
	}

	app := NewApp()

	// The Edit menu is what makes ⌘C/⌘V/⌘A reach the webview on macOS.
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	appMenu.Append(menu.EditMenu())
	appMenu.Append(menu.WindowMenu())

	err := wails.Run(&options.App{
		Title:            "agenterm",
		Width:            1280,
		Height:           800,
		MinWidth:         640,
		MinHeight:        400,
		BackgroundColour: &options.RGBA{R: 0x14, G: 0x15, B: 0x18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		Menu:             appMenu,
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.elionaz.agenterm",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				runtime.WindowUnminimise(app.ctx)
				runtime.Show(app.ctx)
			},
		},
		Bind: []any{app},
		Mac: &mac.Options{
			// agenterm://session/<id> (CFBundleURLTypes from wails.json
			// "protocols"): daemon notification clicks.
			OnUrlOpen:  app.openURL,
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
			About: &mac.AboutInfo{
				Title:   "agenterm",
				Message: "Workspace de escritorio para agentes de código",
			},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenterm:", err)
		os.Exit(1)
	}
}
