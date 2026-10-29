package app

import (
	"html"
	"strings"
)

// plainCaption turns the HTML fragment Pixiv calls a caption into the plain
// text the rail shows. Pixiv allows <br>, <p>, links and entities; the
// editorial page has no rich-text renderer, and showing the tags themselves
// is worse than dropping them.
func plainCaption(s string) string {
	if s == "" {
		return ""
	}
	s = strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n", "<br  />", "\n",
		"<p>", "", "</p>", "\n\n",
	).Replace(s)

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			// Strip a tag only when it looks like one; a literal "<3" in a
			// caption is text, not the start of a tag.
			if i+1 < len(s) && (isTagStart(s[i+1]) || s[i+1] == '/') {
				if j := strings.IndexByte(s[i:], '>'); j >= 0 {
					i += j
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	out := html.UnescapeString(b.String())
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

func isTagStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '!'
}
