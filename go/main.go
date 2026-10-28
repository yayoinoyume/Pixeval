package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/yayoinoyume/pixeval-go/internal/app"
)

func main() {
	a := app.New()
	mygo.App.WhenReady(a.Start)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
