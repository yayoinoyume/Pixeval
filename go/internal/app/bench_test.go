package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// BenchmarkBrowseLayout1000 measures one frame of a feed of 1000 works. The
// covers are marked failed so the frame builds placeholders instead of
// starting the asynchronous image loads a single frame never settles.
func BenchmarkBrowseLayout1000(b *testing.B) {
	st := &browseState{started: true}
	st.items = fakeWorks(0, 1000)
	st.failed = map[string]bool{"": true}
	_, tt := newBrowseTest(st)
	// The first frame lays the masonry out; the benchmark times the frames
	// after it, which build the cards in view.
	tt.Frame()
	b.ReportAllocs()
	for b.Loop() {
		tt.Frame()
	}
}

// BenchmarkShellView measures one frame of the empty shell: navigation, the
// header and the signed-out home page.
func BenchmarkShellView(b *testing.B) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	b.ReportAllocs()
	for b.Loop() {
		tt.Frame()
	}
}
