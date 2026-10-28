package filter

import (
	"fmt"
	"strings"
	"unicode"
)

// tokenKind is the lexical class of one piece of a filter expression.
type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenIdent
	tokenString
	tokenNumber
	tokenLParen
	tokenRParen
	tokenAnd
	tokenOr
	tokenNot
	tokenEq
	tokenNe
	tokenGt
	tokenGe
	tokenLt
	tokenLe
	tokenContains
	tokenNotContains
)

func (k tokenKind) String() string {
	switch k {
	case tokenEOF:
		return "end of input"
	case tokenIdent:
		return "field"
	case tokenString:
		return "string"
	case tokenNumber:
		return "number"
	case tokenLParen:
		return "'('"
	case tokenRParen:
		return "')'"
	case tokenAnd:
		return "AND"
	case tokenOr:
		return "OR"
	case tokenNot:
		return "NOT"
	case tokenEq:
		return "'='"
	case tokenNe:
		return "'!='"
	case tokenGt:
		return "'>'"
	case tokenGe:
		return "'>='"
	case tokenLt:
		return "'<'"
	case tokenLe:
		return "'<='"
	case tokenContains:
		return "'~'"
	case tokenNotContains:
		return "'!~'"
	}
	return "unknown token"
}

func (k tokenKind) isCompare() bool {
	return k >= tokenEq && k <= tokenNotContains
}

func (k tokenKind) isNumeric() bool {
	switch k {
	case tokenEq, tokenNe, tokenGt, tokenGe, tokenLt, tokenLe:
		return true
	}
	return false
}

func (k tokenKind) isText() bool {
	switch k {
	case tokenEq, tokenNe, tokenContains, tokenNotContains:
		return true
	}
	return false
}

// token is one lexed piece with its 1-based rune position.
type token struct {
	kind tokenKind
	text string
	pos  int
}

func (t token) describe() string {
	if t.text != "" {
		return fmt.Sprintf("%q", t.text)
	}
	return t.kind.String()
}

// lex turns a filter expression into tokens. Comments are not part of the
// language, so every character must belong to a token.
func lex(s string) ([]token, error) {
	runes := []rune(s)
	tokens := make([]token, 0, len(runes)/2+1)

	for i := 0; i < len(runes); {
		c := runes[i]
		if unicode.IsSpace(c) {
			i++
			continue
		}

		pos := i + 1
		switch {
		case c == '(':
			tokens = append(tokens, token{kind: tokenLParen, pos: pos})
			i++
		case c == ')':
			tokens = append(tokens, token{kind: tokenRParen, pos: pos})
			i++
		case c == '"':
			text, next, err := lexString(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenString, text: text, pos: pos})
			i = next
		case unicode.IsDigit(c):
			j := i
			for j < len(runes) && unicode.IsDigit(runes[j]) {
				j++
			}
			tokens = append(tokens, token{kind: tokenNumber, text: string(runes[i:j]), pos: pos})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(runes) && isIdentPart(runes[j]) {
				j++
			}
			word := string(runes[i:j])
			kind := tokenIdent
			switch strings.ToLower(word) {
			case "and":
				kind = tokenAnd
			case "or":
				kind = tokenOr
			case "not":
				kind = tokenNot
			}
			tokens = append(tokens, token{kind: kind, text: word, pos: pos})
			i = j
		case c == '=':
			tokens = append(tokens, token{kind: tokenEq, pos: pos})
			i++
		case c == '~':
			tokens = append(tokens, token{kind: tokenContains, pos: pos})
			i++
		case c == '!':
			switch {
			case i+1 < len(runes) && runes[i+1] == '=':
				tokens = append(tokens, token{kind: tokenNe, pos: pos})
				i += 2
			case i+1 < len(runes) && runes[i+1] == '~':
				tokens = append(tokens, token{kind: tokenNotContains, pos: pos})
				i += 2
			default:
				tokens = append(tokens, token{kind: tokenNot, pos: pos})
				i++
			}
		case c == '>':
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, token{kind: tokenGe, pos: pos})
				i += 2
			} else {
				tokens = append(tokens, token{kind: tokenGt, pos: pos})
				i++
			}
		case c == '<':
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, token{kind: tokenLe, pos: pos})
				i += 2
			} else {
				tokens = append(tokens, token{kind: tokenLt, pos: pos})
				i++
			}
		default:
			return nil, &parseError{pos: pos, msg: fmt.Sprintf("unexpected character %q", c)}
		}
	}

	return append(tokens, token{kind: tokenEOF, pos: len(runes) + 1}), nil
}

// lexString reads a double-quoted string. Backslash makes the next rune
// literal, so \" and \\ work without inventing escape sequences.
func lexString(runes []rune, start int) (string, int, error) {
	var b strings.Builder
	for i := start + 1; i < len(runes); i++ {
		switch runes[i] {
		case '"':
			return b.String(), i + 1, nil
		case '\\':
			if i+1 >= len(runes) {
				return "", 0, &parseError{pos: start + 1, msg: "unterminated string"}
			}
			b.WriteRune(runes[i+1])
			i++
		default:
			b.WriteRune(runes[i])
		}
	}
	return "", 0, &parseError{pos: start + 1, msg: "unterminated string"}
}

func isIdentStart(c rune) bool {
	return c == '_' || unicode.IsLetter(c)
}

func isIdentPart(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}
