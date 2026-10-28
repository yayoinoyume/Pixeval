package widgets

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// BenchmarkBuildVisible measures a frame that builds the 10 items of a small
// masonry, all of them in view.
func BenchmarkBuildVisible(b *testing.B) {
	items := testItems(10)
	st := &MasonryState{}
	built := 0
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 150, 8, func(i int) {
			built++
			ui.Column(c).Fill().Gap(4).Children(func() {
				ui.Text(c, fmt.Sprintf("作品 %d", i)).MaxLines(1)
				ui.Text(c, "作者").FontSize(12).MaxLines(1)
			})
		})
	}, 800, 600)
	built = 0
	tt.Frame()
	if built != len(items) {
		b.Fatalf("built %d of %d items", built, len(items))
	}
	b.ReportAllocs()
	for b.Loop() {
		tt.Frame()
	}
}
