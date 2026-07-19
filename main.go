package main

import (
	"embed"
	"log"

	"metro-wars/game"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed assets/maps/*.json
var MapsFS embed.FS

//go:embed assets/fonts/*.ttf
var FontsFS embed.FS

func main() {
	g, err := game.New(MapsFS, FontsFS)
	if err != nil {
		log.Fatal("Failed to create game:", err)
	}

	ebiten.SetWindowSize(1920, 1080)
	ebiten.SetWindowTitle("Metro Wars")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
