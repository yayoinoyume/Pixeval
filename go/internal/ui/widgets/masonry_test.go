package widgets

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testItems(n int) []MasonryItem {
	items := make([]MasonryItem, n)
	for i := range items {
		items[i] = MasonryItem{Key: i, Aspect: 0.5 + float64(i%7)/7}
	}
	return items
}

func TestMasonryBuildsOnlyVisible(t *testing.T) {
	const n = 10000
	items := testItems(n)
	st := &MasonryState{}
	built := 0
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 200, 8, func(i int) { built++ })
	}, 800, 600)
	built = 0
	tt.Frame()
	if built == 0 || built >= 200 {
		t.Fatalf("a frame built %d items of %d", built, n)
	}
}

func TestMasonryBuildsWhatIsInView(t *testing.T) {
	items := testItems(2000)
	st := &MasonryState{}
	built := map[int]bool{}
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 150, 8, func(i int) { built[i] = true })
	}, 800, 600)
	st.Scroll.Y = 5000
	built = map[int]bool{}
	tt.Frame()
	top := st.Scroll.Y
	if top != 5000 {
		t.Fatalf("scrolled to %v", top)
	}
	for i, r := range st.rects {
		if r.y+r.h > top && r.y < top+600 && !built[i] {
			t.Errorf("item %d at %v..%v is in view, not built", i, r.y, r.y+r.h)
		}
	}
}

func TestMasonryColumnsAndRects(t *testing.T) {
	items := testItems(300)
	st := &MasonryState{}
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 200, 8, func(i int) {})
	}, 800, 600)
	for _, tc := range []struct {
		width, height int
		cols          int
	}{
		{800, 600, 3},
		{1000, 600, 4},
		{400, 600, 1},
		{1200, 600, 5},
	} {
		tt.SetSize(tc.width, tc.height)
		if st.width != float32(tc.width) {
			t.Fatalf("laid out for %v wide, not %d", st.width, tc.width)
		}
		checkMasonry(t, st, float32(tc.width), tc.cols)
	}
}

// checkMasonry checks that st placed every item in the shortest column, at
// its width, without overlapping.
func checkMasonry(t *testing.T, st *MasonryState, width float32, cols int) {
	t.Helper()
	if st.cols != cols {
		t.Fatalf("%d columns at %v wide, not %d", st.cols, width, cols)
	}
	colW := (width - st.gap*float32(cols-1)) / float32(cols)
	bottoms := make([]float32, cols)
	used := make([]bool, cols)
	for i, r := range st.rects {
		if math.Abs(float64(r.w-colW)) > 0.01 {
			t.Fatalf("item %d is %v wide, not %v", i, r.w, colW)
		}
		col := int(math.Round(float64(r.x / (colW + st.gap))))
		if col < 0 || col >= cols || math.Abs(float64(r.x-float32(col)*(colW+st.gap))) > 0.01 {
			t.Fatalf("item %d at x %v is in no column", i, r.x)
		}
		used[col] = true
		want := bottoms[0]
		for _, b := range bottoms[1:] {
			want = min(want, b)
		}
		if math.Abs(float64(r.y-want)) > 0.01 {
			t.Fatalf("item %d at y %v, not the shortest column at %v", i, r.y, want)
		}
		bottoms[col] = r.y + r.h
		if r.x < 0 || r.x+r.w > width+0.01 {
			t.Fatalf("item %d at x %v..%v leaves %v wide", i, r.x, r.x+r.w, width)
		}
		if r.y+r.h > st.total+0.01 {
			t.Fatalf("item %d ends at %v, past %v", i, r.y+r.h, st.total)
		}
	}
	for col := 0; col < min(cols, len(st.rects)); col++ {
		if !used[col] {
			t.Fatalf("column %d is empty", col)
		}
	}
	var tallest float32
	for _, b := range bottoms {
		tallest = max(tallest, b)
	}
	if math.Abs(float64(st.total-tallest)) > 0.01 {
		t.Fatalf("content is %v high, not %v", st.total, tallest)
	}
}

func TestMasonryPlacesItems(t *testing.T) {
	items := testItems(40)
	st := &MasonryState{}
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 200, 8, func(i int) {
			ui.Text(c, fmt.Sprintf("tile %d", i))
		})
	}, 800, 600)
	for i, r := range st.rects {
		if r.y+r.h <= st.Scroll.Y || r.y >= st.Scroll.Y+600 {
			continue
		}
		got, ok := tt.Find(fmt.Sprintf("tile %d", i))
		if !ok {
			t.Fatalf("item %d is in view, not found", i)
		}
		if math.Abs(float64(got.X-r.x)) > 1 || math.Abs(float64(got.Y-r.y)) > 1 {
			t.Errorf("item %d at %v,%v, laid out at %v,%v", i, got.X, got.Y, r.x, r.y)
		}
	}
}

func TestMasonryAppendKeepsScroll(t *testing.T) {
	items := testItems(500)
	st := &MasonryState{}
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 200, 8, func(i int) {})
	}, 800, 600)
	st.Scroll.Y = 500
	tt.Frame()
	if st.Scroll.Y != 500 {
		t.Fatalf("scrolled to %v", st.Scroll.Y)
	}
	for i := 0; i < 100; i++ {
		k := len(items)
		items = append(items, MasonryItem{Key: k, Aspect: 0.5 + float64(k%7)/7})
	}
	tt.Frame()
	if st.Scroll.Y != 500 {
		t.Fatalf("adding items scrolled to %v", st.Scroll.Y)
	}
	if len(st.rects) != len(items) {
		t.Fatalf("laid out %d of %d items", len(st.rects), len(items))
	}
	checkMasonry(t, st, 800, 3)
}

func TestMasonryAtEnd(t *testing.T) {
	items := testItems(500)
	st := &MasonryState{}
	tt := ui.NewTester(func(c *ui.Context) {
		Masonry(c, st, items, 200, 8, func(i int) {})
	}, 800, 600)
	if st.AtEnd() {
		t.Fatal("at the end at the top")
	}
	st.Scroll.Y = math.MaxFloat32
	tt.Frame()
	if !st.AtEnd() {
		t.Fatalf("not at the end at %v of %v", st.Scroll.Y, st.Scroll.MaxY)
	}
}

func BenchmarkLayout10000(b *testing.B) {
	items := testItems(10000)
	st := &MasonryState{}
	b.ReportAllocs()
	for b.Loop() {
		st.compute(items, 1200, 200, 8)
	}
}
