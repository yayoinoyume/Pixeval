package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/egoist/mygo"
	"github.com/yayoinoyume/pixeval-go/internal/app"
	"github.com/yayoinoyume/pixeval-go/internal/diag"
)

func main() {
	dir := logDir()
	if err := diag.Install(dir); err != nil {
		log.Printf("crash log: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			if err := diag.Report(dir, r, debug.Stack()); err != nil {
				log.Printf("crash log: %v", err)
				return
			}
			log.Println("crash log:", dir)
		}
	}()

	a := app.New()
	mygo.App.WhenReady(a.Start)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// logDir is the crash log directory under the user's cache directory.
func logDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		log.Printf("crash log: %v", err)
		return ""
	}
	return filepath.Join(base, "pixeval-go", "logs")
}
