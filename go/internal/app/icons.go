package app

import "github.com/egoist/mygo/ui"

// The icons are authored here as one-stroke SVGs, so the app carries no icon
// dependency and every glyph follows the text color (currentColor).
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconLogo      = icon(`<rect x="8.5" y="3.5" width="12" height="12" rx="3"/><rect x="3.5" y="8.5" width="12" height="12" rx="3"/>`)
	iconHome      = icon(`<path d="M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-4.5v-6h-5v6H5a1 1 0 0 1-1-1Z"/>`)
	iconSearch    = icon(`<circle cx="11" cy="11" r="6.5"/><path d="m16 16 4.5 4.5"/>`)
	iconBell      = icon(`<path d="M6.5 9.5a5.5 5.5 0 0 1 11 0c0 4 1.5 5.5 2 6.5H4.5c.5-1 2-2.5 2-6.5Z"/><path d="M10 19a2.2 2.2 0 0 0 4 0"/>`)
	iconDownload  = icon(`<path d="M12 4v11"/><path d="m7.5 10.5 4.5 4.5 4.5-4.5"/><path d="M5 20h14"/>`)
	iconHistory   = icon(`<circle cx="12" cy="12" r="8"/><path d="M12 8v4l2.5 1.5"/>`)
	iconSettings  = icon(`<path d="M4 7h16M4 12h16M4 17h16"/><circle cx="9" cy="7" r="2.2"/><circle cx="15" cy="12" r="2.2"/><circle cx="8" cy="17" r="2.2"/>`)
	iconUser      = icon(`<circle cx="12" cy="8" r="3.5"/><path d="M5 20a7 7 0 0 1 14 0"/>`)
	iconClose     = icon(`<path d="m6 6 12 12M18 6 6 18"/>`)
	iconMinus     = icon(`<path d="M6 12h12"/>`)
	iconPlus      = icon(`<path d="M12 6v12M6 12h12"/>`)
	iconLeft      = icon(`<path d="m14.5 6-6 6 6 6"/>`)
	iconRight     = icon(`<path d="m9.5 6 6 6-6 6"/>`)
	iconDown      = icon(`<path d="m6 9.5 6 6 6-6"/>`)
	iconHeart     = icon(`<path d="M12 20.3 4.9 13.2a4.9 4.9 0 0 1 0-6.9 4.8 4.8 0 0 1 6.8 0l.3.3.3-.3a4.8 4.8 0 0 1 6.8 0 4.9 4.9 0 0 1 0 6.9Z"/>`)
	iconHeartFill = icon(`<path d="M12 20.3 4.9 13.2a4.9 4.9 0 0 1 0-6.9 4.8 4.8 0 0 1 6.8 0l.3.3.3-.3a4.8 4.8 0 0 1 6.8 0 4.9 4.9 0 0 1 0 6.9Z" fill="currentColor" stroke="none"/>`)
	iconRefresh   = icon(`<path d="M20 4.5v6h-6"/><path d="M19.2 10.5A7.5 7.5 0 1 0 17 17.4"/>`)
	iconComment   = icon(`<path d="M20 14.5a3 3 0 0 1-3 3H8.5L4 21V6.5a3 3 0 0 1 3-3h10a3 3 0 0 1 3 3Z"/>`)
	iconLogout    = icon(`<path d="M14.5 4H19a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4.5"/><path d="M9 8l-4 4 4 4"/><path d="M5 12h10"/>`)
	iconSeries    = icon(`<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5"/><path d="m3 16 9 5 9-5"/>`)
	iconImage     = icon(`<rect x="3.5" y="4.5" width="17" height="15" rx="2.5"/><circle cx="9" cy="10" r="1.6"/><path d="m5 18 4.5-4.5 3.5 3.5 2.5-2.5L20 19"/>`)
	iconAlert     = icon(`<path d="M12 4 3.5 19.5h17Z"/><path d="M12 10v4"/><circle cx="12" cy="17" r=".8" fill="currentColor" stroke="none"/>`)
)

// navIcon is the glyph of a navigation page.
func navIcon(r route) *ui.SVG {
	switch r {
	case routeSearch:
		return iconSearch
	case routeSubs:
		return iconBell
	case routeDownloads:
		return iconDownload
	case routeHistory:
		return iconHistory
	case routeSettings:
		return iconSettings
	default:
		return iconHome
	}
}
