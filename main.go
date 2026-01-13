package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// 关键修改：直接 embed 整个 frontend 文件夹
//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "SecureChat Pro v5",
		Width:  1100,
		Height: 750,
		AssetServer: &assetserver.Options{
			Assets: assets, // 这里的 assets 指向了整个 frontend 文件夹
		},
		BackgroundColour: &options.RGBA{R: 14, G: 22, B: 33, A: 255},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}