//go:build fyne || android

package main

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	zopapp "github.com/peterwwillis/zop/internal/app"
	"github.com/peterwwillis/zop/internal/mobileui"
)

func main() {
	application := fyneapp.NewWithID("com.zop.app")
	controller, err := zopapp.NewController("", "", "")
	if err != nil {
		window := application.NewWindow("zop")
		dialog.NewError(err, window).Show()
		window.SetContent(widget.NewLabel("Unable to start zop mobile UI."))
		window.Resize(fyne.NewSize(520, 420))
		window.ShowAndRun()
		return
	}

	window := mobileui.NewWindow(application, controller)
	window.ShowAndRun()
}
