package filter

import "testing"

func TestLexTokens(t *testing.T) {
	tokens, err := lex(`id >= 100 AND NOT title !~ "猫"`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}

	want := []struct {
		kind tokenKind
		text string
		pos  int
	}{
		{tokenIdent, "id", 1},
		{tokenGe, "", 4},
		{tokenNumber, "100", 7},
		{tokenAnd, "AND", 11},
		{tokenNot, "NOT", 15},
		{tokenIdent, "title", 19},
		{tokenNotContains, "", 25},
		{tokenString, "猫", 28},
		{tokenEOF, "", 31},
	}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(want))
	}
	for i, w := range want {
		if tokens[i].kind != w.kind || tokens[i].text != w.text || tokens[i].pos != w.pos {
			t.Errorf("token %d = {%v %q %d}, want {%v %q %d}",
				i, tokens[i].kind, tokens[i].text, tokens[i].pos, w.kind, w.text, w.pos)
		}
	}
}

func TestLexKeywordsAreCaseInsensitive(t *testing.T) {
	tokens, err := lex(`is_r18 aNd noT is_ai Or title = "x"`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}

	kinds := []tokenKind{
		tokenIdent, tokenAnd, tokenNot, tokenIdent, tokenOr,
		tokenIdent, tokenEq, tokenString, tokenEOF,
	}
	if len(tokens) != len(kinds) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(kinds))
	}
	for i, kind := range kinds {
		if tokens[i].kind != kind {
			t.Errorf("token %d = %v, want %v", i, tokens[i].kind, kind)
		}
	}
}

func TestLexStringEscapes(t *testing.T) {
	tokens, err := lex(`title = "a\"b\\c"`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if got := tokens[2].text; got != `a"b\c` {
		t.Fatalf("string = %q, want %q", got, `a"b\c`)
	}
}

func TestLexErrorsHavePosition(t *testing.T) {
	tests := []struct {
		expr string
		pos  int
		want string
	}{
		{`id $ 1`, 4, "unexpected character '$'"},
		{`title = "cat`, 9, "unterminated string"},
	}

	for _, tt := range tests {
		_, err := lex(tt.expr)
		if err == nil {
			t.Fatalf("lex(%q): got nil error", tt.expr)
		}
		pos, msg := parseErrorAt(t, err)
		if pos != tt.pos || msg != tt.want {
			t.Errorf("lex(%q) = position %d %q, want position %d %q", tt.expr, pos, msg, tt.pos, tt.want)
		}
	}
}
