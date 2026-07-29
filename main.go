package main

import (
	"embed"
	"fmt"
	"os"

	"chit/internal/store"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

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
