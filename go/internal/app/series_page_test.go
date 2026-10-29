package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// seriesBodies is pageBodies with the series' author, works count, concluded
// state and dates, and the description a test gives it.
func seriesBodies(description string, concluded bool) map[string]string {
	bodies := pageBodies()
	bodies["/v1/illust/series/"] = `{"illust_series_detail":{"id":3,"title":"系列三",` +
		`"description":` + strconv.Quote(description) + `,` +
		`"user":{"id":7,"name":"画师七","account":"painter7"},` +
		`"work_illusts_count":12,"is_concluded":` + strconv.FormatBool(concluded) + `,` +
		`"create_date":"2024-01-02T03:04:05+09:00","update_date":"2024-05-06T07:08:09+09:00"},` +
		`"illusts":[{"id":21,"title":"系列作品","user":{"id":7,"name":"画师七"},` +
		`"width":480,"height":720}],"next_url":""}`
	return bodies
}

// openSeriesPage is a page app on a loaded series, with the times it hands the
// page.
func openSeriesPage(t *testing.T, bodies map[string]string) (*App, time.Time, time.Time) {
	t.Helper()
	a := newPageAppWithBodies(bodies)
	a.openSeries(3)
	waitSeries(t, a, func(v seriesPageView) bool { return !v.loading })
	tokyo := time.FixedZone("JST", 9*3600)
	return a, time.Date(2024, 1, 2, 3, 4, 5, 0, tokyo),
		time.Date(2024, 5, 6, 7, 8, 9, 0, tokyo)
}

// TestSeriesPageShowsInfo checks the series header shows its author, works
// count, concluded state and dates.
func TestSeriesPageShowsInfo(t *testing.T) {
	for _, concluded := range []bool{true, false} {
		a, created, updated := openSeriesPage(t, seriesBodies("系列简介", concluded))
		tt := ui.NewTester(a.View, 1000, 700)
		tt.Frame()
		state := a.t("series.ongoing")
		if concluded {
			state = a.t("series.concluded")
		}
		for _, want := range []string{
			"画师七",
			a.tf("series.worksCount", 12),
			state,
			detailDate(created) + " → " + detailDate(updated),
		} {
			if !tt.HasText(want) {
				t.Errorf("concluded=%v: %q missing: %q", concluded, want, tt.Texts())
			}
		}
	}
}

// TestSeriesPageAuthorOpensUser checks the author line opens the author's
// page.
func TestSeriesPageAuthorOpensUser(t *testing.T) {
	a, _, _ := openSeriesPage(t, seriesBodies("系列简介", true))
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if err := tt.Click(a.t("series.author")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.route != routeUser {
		t.Fatalf("route %q, want %q", a.route, routeUser)
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	opened := false
	for _, u := range tr.requestedURLs() {
		if strings.Contains(u, "/v1/user/detail") && strings.Contains(u, "user_id=7") {
			opened = true
		}
	}
	if !opened {
		t.Errorf("no detail request for user 7: %v", tr.requestedURLs())
	}
}

// TestSeriesPageDescriptionToggle checks a long description folds to a few
// lines and opens on the toggle.
func TestSeriesPageDescriptionToggle(t *testing.T) {
	desc := strings.Repeat("这是一段很长的系列简介。", 40)
	a, _, _ := openSeriesPage(t, seriesBodies(desc, true))
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	collapsed, ok := tt.Find(desc)
	if !ok {
		t.Fatalf("the description is missing: %q", tt.Texts())
	}
	if err := tt.Click(a.t("series.showMore")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	expanded, ok := tt.Find(desc)
	if !ok {
		t.Fatalf("the description is missing after expanding: %q", tt.Texts())
	}
	if expanded.H <= collapsed.H {
		t.Errorf("the description is %v high expanded, %v folded", expanded.H, collapsed.H)
	}
	if !tt.HasText(a.t("series.showLess")) {
		t.Fatalf("no fold control: %q", tt.Texts())
	}
	if err := tt.Click(a.t("series.showLess")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	folded, ok := tt.Find(desc)
	if !ok {
		t.Fatalf("the description is missing after folding: %q", tt.Texts())
	}
	if folded.H != collapsed.H {
		t.Errorf("the description is %v high folded again, %v at first", folded.H, collapsed.H)
	}
}

// TestSeriesPageShortDescriptionHasNoToggle checks a description that fits
// the folded lines offers no toggle.
func TestSeriesPageShortDescriptionHasNoToggle(t *testing.T) {
	a, _, _ := openSeriesPage(t, seriesBodies("系列简介", true))
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if !tt.HasText("系列简介") {
		t.Fatalf("the description is missing: %q", tt.Texts())
	}
	if tt.HasText(a.t("series.showMore")) {
		t.Errorf("a short description offers a toggle: %q", tt.Texts())
	}
}

// TestDescOverflows checks the wrap estimate the toggle follows.
func TestDescOverflows(t *testing.T) {
	if descOverflows("一行简介", 400, 13, seriesDescLines) {
		t.Error("a short line needs no toggle")
	}
	if !descOverflows(strings.Repeat("很长的一行。", 30), 400, 13, seriesDescLines) {
		t.Error("a long text needs the toggle")
	}
	if !descOverflows("一\n二\n三\n四", 400, 13, seriesDescLines) {
		t.Error("four paragraphs need the toggle")
	}
}

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
