package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewRenders(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 480, 360)
	if !tt.HasText("Pixeval Go") {
		t.Errorf("texts %q", tt.Texts())
	}
}
