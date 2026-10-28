// Package widgets holds Pixeval's own UI components, built from mygo's
// primitives where the framework has none.
package widgets

import (
	"math"
	"reflect"
	"sort"

	"github.com/egoist/mygo/ui"
)

// MasonryItem is one tile of a Masonry: its identity, and its width over its
// height, as 1 for a square and 2 for one twice as wide as it is high.
type MasonryItem struct {
	Key    any
	Aspect float64
}

// masonryRect is where a tile goes in the content of the scroll container.
type masonryRect struct {
	x, y, w, h float32
}

// MasonryState is a Masonry's place and the layout it computed, kept in the
// app's state. Give each Masonry a MasonryState of its own.
type MasonryState struct {
	// Scroll is where the user scrolled the masonry, which TrackScroll keeps.
	Scroll ui.ScrollState

	rects   []masonryRect
	bottoms []float32 // the height each column has so far, with gap
	cols    int
	// width, columnWidth and gap are what rects were laid out for: when one
	// changes, the layout is computed again.
	width       float32
	columnWidth float32
	gap         float32
	total       float32
	maxH        float32
	// vh is the height of the view in the last frame, which AtEnd tells a
	// content that fits from one whose layout is yet to come.
	vh float32
	// firstKey and lastKey are the keys rects were laid out for: when they
	// change, the items were replaced, and the layout is computed again.
	firstKey, lastKey any
}

// masonryFallback is the width a Masonry lays out for before its container
// has been laid out once, or while it has no width.
const masonryFallback = 400

// Masonry creates a scroll container of items in columns, each item placed
// in the shortest column, as Photos' justified grid and Pinterest's pages
// do: item i takes its place from items[i].Aspect, in a column columnWidth
// wide, gap apart, and build fills it. Only the items in view, with half a
// screen above and below, are built.
//
// st keeps the place and the layout, or nil for a masonry that keeps none.
// The layout is computed again when the number of items, their keys, or the
// width of the container changes; items added keep the scroll where it is.
func Masonry(c *ui.Context, st *MasonryState, items []MasonryItem, columnWidth, gap float64, build func(i int)) {
	if st == nil {
		st = &MasonryState{}
	}
	e := ui.Scroll(c).TrackScroll(&st.Scroll).Grow(1)
	// The container is laid out after the view that fills it: its width is
	// that of the last frame.
	b := e.LayoutBounds()
	width := b.W
	if width <= 0 {
		// A container yet to be laid out, or one squeezed to no width: lay
		// out for a width of its own without asking for another frame, as a
		// frame that keeps it at no width would come again and again.
		width = masonryFallback
	}
	if st.layout(items, width, float32(columnWidth), float32(gap)) {
		// The width laid out for was the one before this frame's layout, as
		// after a resize: one more frame lays out the real one.
		c.Invalidate()
	}
	vh := b.H
	if vh <= 0 {
		_, vh = c.Size()
	}
	if vh <= 0 {
		vh = masonryFallback
	}
	st.vh = vh
	top := min(max(st.Scroll.Y, 0), max(st.total-vh, 0))
	from, to := top-vh/2, top+vh+vh/2
	start := st.firstVisible(from)
	e.Children(func() {
		ui.Box(c).Height(st.total).Children(func() {
			for i := start; i < len(items) && st.rects[i].y <= to; i++ {
				r := st.rects[i]
				cell := ui.Box(c).Absolute().Top(r.y).Left(r.x).Width(r.w).Height(r.h)
				if items[i].Key != nil {
					cell.Key(items[i].Key)
				}
				cell.Children(func() { build(i) })
			}
		})
	})
}

// AtEnd reports whether the masonry shows its end: nothing to scroll, or the
// scroll is as far down as it goes.
func (st *MasonryState) AtEnd() bool {
	if st.Scroll.MaxY <= 0 {
		// Nothing scrolls: the content fits, or the layout is yet to find the
		// view's height.
		return st.total <= st.vh
	}
	return st.Scroll.Y >= st.Scroll.MaxY
}

// layout places items in columns width wide, and reports whether it computed
// the whole layout again rather than only the items added.
func (st *MasonryState) layout(items []MasonryItem, width, columnWidth, gap float32) bool {
	if columnWidth <= 0 {
		columnWidth = 1
	}
	if gap < 0 {
		gap = 0
	}
	if width <= 0 {
		width = masonryFallback
	}
	cols := masonryColumns(width, columnWidth, gap)
	if st.cols != cols || st.width != width || st.columnWidth != columnWidth || st.gap != gap {
		st.compute(items, width, columnWidth, gap)
		return true
	}
	switch {
	case len(items) > len(st.rects):
		st.extend(items)
	case len(items) < len(st.rects):
		st.compute(items, width, columnWidth, gap)
		return true
	case len(items) > 0 && (!sameKey(items[0].Key, st.firstKey) || !sameKey(items[len(items)-1].Key, st.lastKey)):
		// The items were replaced, not added to.
		st.compute(items, width, columnWidth, gap)
		return true
	}
	return false
}

// sameKey reports whether a and b are the same key. A key Go cannot compare,
// as a slice, is taken as unchanged, as such a key is no identity to follow.
func sameKey(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if !ta.Comparable() {
		return true
	}
	return a == b
}

// masonryColumns returns how many columns at least columnWidth wide, gap
// apart, fit width.
func masonryColumns(width, columnWidth, gap float32) int {
	return max(1, int(math.Floor(float64(width+gap)/float64(columnWidth+gap))))
}

// masonryHeight returns how high an item width wide with aspect goes, as wide
// as it is high. An aspect no height comes of, as one not above zero or
// beyond what a float holds, makes the item square.
func masonryHeight(width float32, aspect float64) float32 {
	if !(aspect > 0) || math.IsInf(aspect, 0) {
		return width
	}
	h := width / float32(aspect)
	if math.IsInf(float64(h), 0) || math.IsNaN(float64(h)) {
		return width
	}
	return h
}

// compute lays out all items again, from the first.
func (st *MasonryState) compute(items []MasonryItem, width, columnWidth, gap float32) {
	cols := masonryColumns(width, columnWidth, gap)
	colW := (width - gap*float32(cols-1)) / float32(cols)
	if colW < 0 {
		colW = width
	}
	st.cols, st.width, st.columnWidth, st.gap = cols, width, columnWidth, gap
	st.rects = st.rects[:0]
	if cap(st.bottoms) < cols {
		st.bottoms = make([]float32, cols)
	} else {
		st.bottoms = st.bottoms[:cols]
		clear(st.bottoms)
	}
	st.maxH = 0
	for _, item := range items {
		st.place(item, colW, gap)
	}
	st.total = st.columnHeight()
	st.firstKey, st.lastKey = nil, nil
	if len(items) > 0 {
		st.firstKey, st.lastKey = items[0].Key, items[len(items)-1].Key
	}
}

// extend lays out the items added since the last layout, from the columns as
// they were.
func (st *MasonryState) extend(items []MasonryItem) {
	start := len(st.rects)
	colW := (st.width - st.gap*float32(st.cols-1)) / float32(st.cols)
	for _, item := range items[start:] {
		st.place(item, colW, st.gap)
	}
	st.total = st.columnHeight()
	if start == 0 && len(items) > 0 {
		st.firstKey = items[0].Key
	}
	if len(items) > 0 {
		st.lastKey = items[len(items)-1].Key
	}
}

// columnHeight returns how high the tallest column makes the content.
func (st *MasonryState) columnHeight() float32 {
	var tallest float32
	for _, b := range st.bottoms {
		tallest = max(tallest, b)
	}
	return max(0, tallest-st.gap)
}

// place puts item at the top of the shortest column.
func (st *MasonryState) place(item MasonryItem, colW, gap float32) {
	col := 0
	for j := 1; j < len(st.bottoms); j++ {
		if st.bottoms[j] < st.bottoms[col] {
			col = j
		}
	}
	y := st.bottoms[col]
	h := masonryHeight(colW, item.Aspect)
	st.rects = append(st.rects, masonryRect{x: float32(col) * (colW + gap), y: y, w: colW, h: h})
	st.bottoms[col] = y + h + gap
	st.maxH = max(st.maxH, h)
}

// firstVisible returns the first item that may reach top, which is the first
// whose top is within a tallest item of it: the tops only go down with the
// index, so this is a search, not a scan of every item.
func (st *MasonryState) firstVisible(top float32) int {
	i := sort.Search(len(st.rects), func(i int) bool {
		return st.rects[i].y >= top-st.maxH
	})
	for i < len(st.rects) && st.rects[i].y < top && st.rects[i].y+st.rects[i].h <= top {
		i++
	}
	return i
}
