package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSeriesPageDownloadLoaded checks the series' header button queues the
// works its feed has loaded.
func TestSeriesPageDownloadLoaded(t *testing.T) {
	a := newPageApp()
	a.downloadsDir = t.TempDir()
	a.openSeries(3)
	waitSeries(t, a, func(v seriesPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.seriesView.feed, "load the works", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	label := a.tf("download.allLoaded", 1)
	if !tt.HasText(label) {
		t.Fatalf("%q missing: %q", label, tt.Texts())
	}
	if err := tt.Click(label); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 1 {
		t.Errorf("%d tasks queued, want 1", len(tasks))
	}
}
