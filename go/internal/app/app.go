package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// App holds the state of the window. Views are rebuilt from it on the main
// thread, so state lives here and is changed from goroutines only through
// Window.Update.
type App struct {
	win *mygo.Window
}

func New() *App { return &App{} }

func (a *App) Start() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "Pixeval Go",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 600,
		Content:   ui.View(a.View),
	})
}

// View builds the interface. M0 placeholder: the shell lands here.
func (a *App) View(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		ui.Text(c, "Pixeval Go").FontSize(28).Bold()
		ui.Text(c, "M0 skeleton").TextColor(t.TextMuted)
	})
}
