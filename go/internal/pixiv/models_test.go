package pixiv

import "testing"

// TestThumbURLPrefersMedium checks a card gets the smaller Medium cover even
// when Large is present.
func TestThumbURLPrefersMedium(t *testing.T) {
	w := Illustration{ImageUrls: ImageUrls{
		SquareMedium: "https://i/square.jpg",
		Medium:       "https://i/medium.jpg",
		Large:        "https://i/large.jpg",
	}}
	if got := w.ThumbURL(); got != "https://i/medium.jpg" {
		t.Errorf("ThumbURL = %q, want the Medium cover", got)
	}
}

// TestThumbURLFallsBack checks works without a Medium cover still get one.
func TestThumbURLFallsBack(t *testing.T) {
	cases := []struct {
		name string
		urls ImageUrls
		want string
	}{
		{"large", ImageUrls{Large: "https://i/large.jpg", SquareMedium: "https://i/square.jpg"}, "https://i/large.jpg"},
		{"square", ImageUrls{SquareMedium: "https://i/square.jpg"}, "https://i/square.jpg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := Illustration{ImageUrls: tc.urls}
			if got := w.ThumbURL(); got != tc.want {
				t.Errorf("ThumbURL = %q, want %q", got, tc.want)
			}
		})
	}
}
