//go:build fyne

package mobileui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	zopapp "github.com/peterwwillis/zop/internal/app"
	"github.com/peterwwillis/zop/internal/config"
)

func TestScreenshot(t *testing.T) {
	app := test.NewApp()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	_, err := config.EnsureConfigFile(configPath)
	if err != nil {
		t.Fatalf("ensure config file: %v", err)
	}

	controller, err := zopapp.NewController(configPath, "mobile", "")
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}

	window := NewWindow(app, controller)
	canvas, ok := window.Canvas().(interface{ Capture() image.Image })
	if !ok {
		t.Fatal("canvas does not support capture")
	}

	window.Canvas().Refresh(window.Content())
	imageFile := filepath.Join(os.TempDir(), "zop-mobile-ui.png")
	file, err := os.Create(imageFile)
	if err != nil {
		t.Fatalf("create screenshot: %v", err)
	}
	defer file.Close()

	if err := png.Encode(file, canvas.Capture()); err != nil {
		t.Fatalf("encode screenshot: %v", err)
	}

	t.Logf("wrote screenshot to %s", imageFile)
}
