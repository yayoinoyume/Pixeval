package filter

import (
	"fmt"
	"strconv"
	"strings"
)

// parseError reports a syntax error at a 1-based rune position, so non-ASCII
// expressions point at the rune the user sees.
type parseError struct {
	pos int
	msg string
}

func (e *parseError) Error() string {
	return fmt.Sprintf("filter: %s at position %d", e.msg, e.pos)
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token { return p.tokens[p.pos] }

// next never moves past the EOF token, so a missing operand is reported at the
// end of the input.
func (p *parser) next() token {
	tok := p.tokens[p.pos]
	if tok.kind != tokenEOF {
		p.pos++
	}
	return tok
}

func (p *parser) expect(kind tokenKind) (token, error) {
	tok := p.next()
	if tok.kind != kind {
		return token{}, &parseError{
			pos: tok.pos,
			msg: fmt.Sprintf("expected %s, got %s", kind, tok.describe()),
		}
	}
	return tok, nil
}

// parseOr is the lowest precedence level: comparisons, then AND, then OR.
func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orExpr{left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = andExpr{left: left, right: right}
	}
	return left, nil
}

// parseUnary binds NOT tighter than AND and OR, so "NOT id > 1" negates the
// comparison rather than the whole expression.
func (p *parser) parseUnary() (Expr, error) {
	if p.peek().kind == tokenNot {
		p.next()
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return notExpr{inner: inner}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	tok := p.next()
	switch tok.kind {
	case tokenLParen:
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, err
		}
		return inner, nil
	case tokenIdent:
		return p.parsePredicate(tok)
	default:
		return nil, &parseError{pos: tok.pos, msg: "expected a field, got " + tok.describe()}
	}
}

// parsePredicate parses one field reference: a standalone flag or
// "field operator literal".
func (p *parser) parsePredicate(name token) (Expr, error) {
	field := strings.ToLower(name.text)
	flagFn, isFlag := flagFields[field]
	numFn, isNum := numFields[field]
	textFn, isText := textFields[field]
	if !isFlag && !isNum && !isText {
		return nil, &parseError{pos: name.pos, msg: fmt.Sprintf("unknown field %q", name.text)}
	}

	next := p.peek()
	if !next.kind.isCompare() {
		if isFlag {
			return flagExpr{match: flagFn}, nil
		}
		return nil, &parseError{pos: next.pos, msg: fmt.Sprintf("field %q needs an operator", field)}
	}
	op := p.next()

	if isFlag {
		return nil, &parseError{pos: op.pos, msg: fmt.Sprintf("field %q is a flag and takes no operator", field)}
	}
	if isNum {
		if !op.kind.isNumeric() {
			return nil, &parseError{pos: op.pos, msg: fmt.Sprintf("field %q does not support %s", field, op.kind)}
		}
		lit, err := p.expect(tokenNumber)
		if err != nil {
			return nil, err
		}
		val, err := strconv.ParseInt(lit.text, 10, 64)
		if err != nil {
			return nil, &parseError{pos: lit.pos, msg: "invalid number " + lit.text}
		}
		return numExpr{get: numFn, op: op.kind, val: val}, nil
	}

	if !op.kind.isText() {
		return nil, &parseError{pos: op.pos, msg: fmt.Sprintf("field %q does not support %s", field, op.kind)}
	}
	lit, err := p.expect(tokenString)
	if err != nil {
		return nil, err
	}
	return textExpr{get: textFn, op: op.kind, val: lit.text}, nil
}
