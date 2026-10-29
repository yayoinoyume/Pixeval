package app

import (
	"html"
	"regexp"
	"strings"
)

// captionTag matches one of the tags Pixiv's captions carry, with its
// attributes. Anything else between angle brackets is the author's own text:
// "<B より C>" in a caption is a sentence, not a tag, and dropping it would
// delete words the work's author wrote. The longer names come first, so a
// shorter one that starts the same way cannot shadow them.
var captionTag = regexp.MustCompile(`(?is)^</?(?:blockquote|center|iframe|strong|span|del|div|ins|img|br|em|a|b|i|p|u)(?:\s+[a-z-]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+))?)*\s*/?>`)

// plainCaption turns the HTML fragment Pixiv calls a caption into the plain
// text the rail shows. Pixiv allows <br>, <p>, links and entities; the
// editorial page has no rich-text renderer, and showing the tags themselves
// is worse than dropping them.
func plainCaption(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '<' {
			if tag := captionTag.FindString(s[i:]); tag != "" {
				lower := strings.ToLower(tag)
				switch {
				case strings.HasPrefix(lower, "<br") || strings.HasPrefix(lower, "</br"):
					b.WriteByte('\n')
				case strings.HasPrefix(lower, "</p"):
					b.WriteString("\n\n")
				}
				i += len(tag)
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	out := html.UnescapeString(b.String())
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}
