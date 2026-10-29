package app

import "testing"

// TestPlainCaptionTags checks only the tags Pixiv's captions carry are taken
// out of the text: a sentence that merely uses angle brackets is the author's
// words and stays, and a line break tag becomes a line break at any case.
func TestPlainCaptionTags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"A<B より C>D", "A<B より C>D"},
		{"一<BR />二", "一\n二"},
		{"一<br>二", "一\n二"},
		{"<b>太字</b>と<i>斜体</i>", "太字と斜体"},
		{"<STRONG>強調</STRONG>", "強調"},
		{"<u>下線</u>と<del>消し</del>", "下線と消し"},
		{"<ins>追加</ins>", "追加"},
		{"<blockquote>引用</blockquote>", "引用"},
		{"<center>中央</center>", "中央"},
		{`<iframe src="https://example.invalid/embed"></iframe>枠`, "枠"},
		{`<span style="color:#f00">赤</span>い`, "赤い"},
		{`<div class="x">箱</div>`, "箱"},
		{`<img src="https://example.invalid/a.png">絵`, "絵"},
		{"<custom>残る</custom>", "<custom>残る</custom>"},
		{"a < b かつ c > d", "a < b かつ c > d"},
		{"<3 は文字", "<3 は文字"},
	}
	for _, c := range cases {
		if got := plainCaption(c.in); got != c.want {
			t.Errorf("plainCaption(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
