package filter

import (
	"strings"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// Expr is a parsed filter expression. Match reports whether one work passes it.
type Expr interface {
	Match(w *pixiv.Illustration) bool
}

// Parse parses an expression. An empty or whitespace-only string matches every
// work. Field names and keywords are case-insensitive.
func Parse(s string) (Expr, error) {
	if strings.TrimSpace(s) == "" {
		return matchAll{}, nil
	}

	tokens, err := lex(s)
	if err != nil {
		return nil, err
	}

	p := parser{tokens: tokens}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokenEOF {
		return nil, &parseError{pos: tok.pos, msg: "unexpected " + tok.describe()}
	}
	return expr, nil
}

// matchAll is the expression of an empty filter.
type matchAll struct{}

func (matchAll) Match(*pixiv.Illustration) bool { return true }

type orExpr struct{ left, right Expr }

func (e orExpr) Match(w *pixiv.Illustration) bool {
	return e.left.Match(w) || e.right.Match(w)
}

type andExpr struct{ left, right Expr }

func (e andExpr) Match(w *pixiv.Illustration) bool {
	return e.left.Match(w) && e.right.Match(w)
}

type notExpr struct{ inner Expr }

func (e notExpr) Match(w *pixiv.Illustration) bool { return !e.inner.Match(w) }

// numFields are the integer fields usable with = != > >= < <=.
var numFields = map[string]func(*pixiv.Illustration) int64{
	"id":        func(w *pixiv.Illustration) int64 { return w.ID },
	"bookmarks": func(w *pixiv.Illustration) int64 { return int64(w.TotalBookmarks) },
	"views":     func(w *pixiv.Illustration) int64 { return int64(w.TotalView) },
}

// textFields are the string fields usable with = != ~ !~. Tag values include
// both the original and the translated name.
var textFields = map[string]func(*pixiv.Illustration) []string{
	"title": func(w *pixiv.Illustration) []string { return []string{w.Title} },
	"user":  func(w *pixiv.Illustration) []string { return []string{w.User.Name} },
	"tag": func(w *pixiv.Illustration) []string {
		values := make([]string, 0, 2*len(w.Tags))
		for _, tag := range w.Tags {
			values = append(values, tag.Name, tag.TranslatedName)
		}
		return values
	},
}

// flagFields are predicate fields that stand alone, optionally negated with
// NOT. is_r18 follows the C# Pixeval macro and includes R-18G; pixiv reports
// illust_ai_type 1 for AI-generated works.
var flagFields = map[string]func(*pixiv.Illustration) bool{
	"is_r18":        func(w *pixiv.Illustration) bool { return w.XRestrict != 0 },
	"is_ai":         func(w *pixiv.Illustration) bool { return w.IllustAIType == 1 },
	"is_bookmarked": func(w *pixiv.Illustration) bool { return w.IsBookmarked },
	"is_manga":      func(w *pixiv.Illustration) bool { return w.Type == "manga" },
}

type numExpr struct {
	get func(*pixiv.Illustration) int64
	op  tokenKind
	val int64
}

func (e numExpr) Match(w *pixiv.Illustration) bool {
	got := e.get(w)
	switch e.op {
	case tokenEq:
		return got == e.val
	case tokenNe:
		return got != e.val
	case tokenGt:
		return got > e.val
	case tokenGe:
		return got >= e.val
	case tokenLt:
		return got < e.val
	case tokenLe:
		return got <= e.val
	}
	return false
}

type textExpr struct {
	get func(*pixiv.Illustration) []string
	op  tokenKind
	val string
}

// Match uses any-of semantics for multi-valued fields such as tag: a positive
// operator passes on the first hit, a negated one fails on the first hit.
func (e textExpr) Match(w *pixiv.Illustration) bool {
	for _, got := range e.get(w) {
		switch e.op {
		case tokenEq:
			if got == e.val {
				return true
			}
		case tokenNe:
			if got == e.val {
				return false
			}
		case tokenContains:
			if strings.Contains(got, e.val) {
				return true
			}
		case tokenNotContains:
			if strings.Contains(got, e.val) {
				return false
			}
		}
	}
	return e.op == tokenNe || e.op == tokenNotContains
}

type flagExpr struct {
	match func(*pixiv.Illustration) bool
}

func (e flagExpr) Match(w *pixiv.Illustration) bool { return e.match(w) }
