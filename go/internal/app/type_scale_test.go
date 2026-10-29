package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// literalFontSize matches a text size written as a number instead of a token.
var literalFontSize = regexp.MustCompile(`FontSize\(\s*-?[0-9]`)

// TestNoLiteralFontSizes checks every view sizes its text with the design's
// tokens. A literal does not follow the desktop's text scale, so a reader who
// asks for larger text gets these lines unchanged.
func TestNoLiteralFontSizes(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if literalFontSize.MatchString(line) {
				t.Errorf("%s:%d sizes text with a literal: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
}
