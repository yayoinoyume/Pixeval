package app

import "testing"

func TestPlainCaption(t *testing.T) {
	cases := []struct{ in, want string }{
		{"夏の終わりに。<br />青い光", "夏の終わりに。\n青い光"},
		{`<p>一行目</p><p>二行目</p>`, "一行目\n\n二行目"},
		{`<a href="https://example.com" target="_blank">リンク</a> &amp; more`, "リンク & more"},
		{"<3 は文字", "<3 は文字"},
		{"", ""},
	}
	for _, c := range cases {
		if got := plainCaption(c.in); got != c.want {
			t.Errorf("plainCaption(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
