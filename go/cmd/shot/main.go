// Command shot opens the app, waits for it to draw, and writes a PNG. It is a
// development aid for machines with a display but no screenshot tool.
package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/yayoinoyume/pixeval-go/internal/app"
)

func main() {
	a := app.New()
	out := os.Getenv("SHOT")
	if out == "" {
		out = "/tmp/pixevalgo.png"
	}
	if os.Getenv("SHOT_MODE") == "login" {
		// The login window is its own window, so the caller screenshots the
		// whole display instead of capturing this one.
		mygo.App.WhenReady(func() { a.ShowLogin() })
		if err := mygo.App.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Pixeval Go",
			Width:     1280,
			Height:    820,
			MinWidth:  960,
			MinHeight: 600,
			Content:   ui.View(a.View),
		})
		if os.Getenv("SHOT_MODE") == "hold" {
			return
		}
		go func() {
			time.Sleep(3 * time.Second)
			png, err := w.CapturePage()
			if err != nil {
				log.Println("capture:", err)
				w.Close()
				return
			}
			if err := os.WriteFile(out, png, 0o644); err != nil {
				log.Println("write:", err)
			} else {
				log.Println("saved", out, len(png), "bytes")
			}
			w.Close()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
