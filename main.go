package main

import (
	"embed"
	"fmt"
	"os"

	"chit/internal/store"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

// Windows takes its icon from build/windows/icon.ico at build time and macOS
// from build/appicon.png, but Linux only gets one if it is handed over here.
//
//go:embed build/appicon.png
var appIcon []byte

func main() {
	st, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	app := NewApp(st)

	// The store is bound directly: the frontend reads and writes its own
	// namespaced documents (settings, favourites, per-tool data).
	err = wails.Run(&options.App{
		Title:     "CHIT",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 620,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 16, G: 19, B: 26, A: 1},
		OnStartup:        app.startup,
		Linux: &linux.Options{
			Icon: appIcon,
			// Wails forces WebviewGpuPolicyNever when options.Linux is nil, and
			// the zero value of this field is OnDemand, so passing a Linux block
			// at all silently turns hardware acceleration on. Setting it keeps
			// the behaviour every previous build shipped with.
			// See wailsapp/wails#2977.
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
		Bind: []interface{}{
			app,
			st,
		},
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
