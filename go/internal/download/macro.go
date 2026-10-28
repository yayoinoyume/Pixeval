package download

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Context is the work information a macro template can read. The caller is
// responsible for path-safe output: Render never strips or rewrites any text.
type Context struct {
	ID          int64
	Title       string
	ArtistID    int64
	ArtistName  string
	PublishTime time.Time
	SeriesID    int64
	SeriesTitle string
	Ext         string
	PicSetIndex int
	IsPicGif    bool
	IsPicSet    bool
	IsPicOne    bool
	IsNovel     bool
	IsSeries    bool
	IsR18       bool
	IsR18G      bool
	IsAI        bool
	IsGroup     bool
	GroupID     int64
}

// DefaultMacro is the C# Pixeval default download path macro. Its group prefix
// is empty for a work that is not in a group, so the rendered text still starts
// with "/": callers must filepath.Join it under the download base instead of
// using the text as a path.
const DefaultMacro = "@{is_group?@{group_id}:}/@{is_pic_set?[@{artist_name}] @{title}:}/[@{artist_name}] @{id}@{is_pic_set?p@{pic_set_index}:}.@{ext}"

// defaultTimeFormat is the C# Pixeval fallback for publish_time without format.
const defaultTimeFormat = "yyyy-M-d"

// macroValues is the closed set of keys that render a value. The dynamic type
// returned by each getter picks the formatter: string, macroID, int64 or
// time.Time.
var macroValues = map[string]func(Context) any{
	"id":            func(c Context) any { return macroID(c.ID) },
	"artist_id":     func(c Context) any { return macroID(c.ArtistID) },
	"series_id":     func(c Context) any { return c.SeriesID },
	"group_id":      func(c Context) any { return c.GroupID },
	"pic_set_index": func(c Context) any { return int64(c.PicSetIndex) },
	"title":         func(c Context) any { return c.Title },
	"artist_name":   func(c Context) any { return c.ArtistName },
	"series_title":  func(c Context) any { return c.SeriesTitle },
	"ext":           func(c Context) any { return c.Ext },
	"publish_time":  func(c Context) any { return c.PublishTime },
}

// macroConds is the closed set of keys usable as a condition.
var macroConds = map[string]func(Context) bool{
	"is_pic_gif": func(c Context) bool { return c.IsPicGif },
	"is_pic_set": func(c Context) bool { return c.IsPicSet },
	"is_pic_one": func(c Context) bool { return c.IsPicOne },
	"is_novel":   func(c Context) bool { return c.IsNovel },
	"is_series":  func(c Context) bool { return c.IsSeries },
	"is_r18":     func(c Context) bool { return c.IsR18 },
	"is_r18g":    func(c Context) bool { return c.IsR18G },
	"is_ai":      func(c Context) bool { return c.IsAI },
	"is_group":   func(c Context) bool { return c.IsGroup },
}

// Template is a parsed macro template, ready to be rendered with a Context.
type Template struct {
	nodes []macroNode
}

type macroNode interface {
	render(ctx Context, b *strings.Builder) error
}

type textNode string

func (n textNode) render(_ Context, b *strings.Builder) error {
	b.WriteString(string(n))
	return nil
}

type valueNode struct {
	key    string
	format string
	get    func(Context) any
	offset int // byte offset of the key, reported by format errors
}

// macroID is an id that the C# macro context holds as a string: it takes the
// string formatters u and l, and the digit formats pad it like a number.
type macroID int64

func (n valueNode) render(ctx Context, b *strings.Builder) error {
	var (
		s   string
		err error
	)

	switch v := n.get(ctx).(type) {
	case string:
		s, err = formatString(v, n.format)
	case macroID:
		s, err = formatID(int64(v), n.format)
	case int64:
		s, err = formatInt(v, n.format)
	case time.Time:
		s, err = formatTime(v, n.format)
	default:
		err = fmt.Errorf("macro %q has no value", n.key)
	}

	if err != nil {
		return fmt.Errorf("macro %q at offset %d: %w", n.key, n.offset, err)
	}

	b.WriteString(s)
	return nil
}

type condNode struct {
	key             string
	test            func(Context) bool
	then, otherwise []macroNode
}

func (n condNode) render(ctx Context, b *strings.Builder) error {
	branch := n.otherwise
	if n.test(ctx) {
		branch = n.then
	}

	for _, node := range branch {
		if err := node.render(ctx, b); err != nil {
			return err
		}
	}

	return nil
}

// Parse turns a macro template into a Template. Syntax errors, unknown keys and
// conditional misuse are reported with the byte offset where they occur.
func Parse(template string) (*Template, error) {
	p := &parser{src: template}
	nodes, err := p.parseSequence(false, false)
	if err != nil {
		return nil, err
	}

	return &Template{nodes: nodes}, nil
}

// Render evaluates the template. It returns an error for an unknown key or a
// formatter the key does not support.
func (t *Template) Render(ctx Context) (string, error) {
	var b strings.Builder
	for _, node := range t.nodes {
		if err := node.render(ctx, &b); err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

type parser struct {
	src string
	pos int
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("macro template at offset %d: %s", p.pos, fmt.Sprintf(format, args...))
}

// parseSequence reads plain text and macros until EOF, or until the byte the
// caller asked to stop at when parsing a conditional branch.
func (p *parser) parseSequence(stopAtColon, stopAtRightBrace bool) ([]macroNode, error) {
	var nodes []macroNode
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			nodes = append(nodes, textNode(text.String()))
			text.Reset()
		}
	}

	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case stopAtRightBrace && c == '}':
			flush()
			return nodes, nil
		case stopAtColon && c == ':':
			flush()
			return nodes, nil
		case c == '@':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == '@' {
				text.WriteByte('@')
				p.pos += 2
				continue
			}

			flush()
			node, err := p.parseMacro()
			if err != nil {
				return nil, err
			}

			nodes = append(nodes, node)
		default:
			text.WriteByte(c)
			p.pos++
		}
	}

	flush()
	return nodes, nil
}

func (p *parser) parseMacro() (macroNode, error) {
	start := p.pos
	p.pos++ // '@'
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		p.pos = start
		return nil, p.errorf("expected '{' after '@'")
	}

	p.pos++
	nameStart := p.pos
	for p.pos < len(p.src) && strings.IndexByte(macroNameBytes, p.src[p.pos]) >= 0 {
		p.pos++
	}

	if p.pos == nameStart {
		return nil, p.errorf("expected macro name")
	}

	key := p.src[nameStart:p.pos]
	get, isValue := macroValues[key]
	test, isCond := macroConds[key]
	if !isValue && !isCond {
		p.pos = nameStart
		return nil, p.errorf("unknown macro %q", key)
	}

	var format string
	hasFormat := false
	if p.pos < len(p.src) && p.src[p.pos] == ':' {
		p.pos++
		formatStart := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '?' && p.src[p.pos] != '}' {
			p.pos++
		}

		format, hasFormat = p.src[formatStart:p.pos], true
	}

	var then, otherwise []macroNode
	hasBranches := false
	if p.pos < len(p.src) && p.src[p.pos] == '?' {
		hasBranches = true
		p.pos++
		var err error
		if then, err = p.parseSequence(true, true); err != nil {
			return nil, err
		}

		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return nil, p.errorf("missing ':' in conditional for macro %q", key)
		}

		p.pos++
		if otherwise, err = p.parseSequence(false, true); err != nil {
			return nil, err
		}
	}

	if p.pos >= len(p.src) || p.src[p.pos] != '}' {
		p.pos = start
		return nil, p.errorf("missing '}' for macro %q", key)
	}

	p.pos++

	switch {
	case !isCond:
		if hasBranches {
			return nil, p.errorf("macro %q is not a condition and takes no branches", key)
		}

		return valueNode{key: key, format: format, get: get, offset: nameStart}, nil
	case hasFormat:
		return nil, p.errorf("macro %q is a condition and takes no format", key)
	case !hasBranches:
		return nil, p.errorf("macro %q is a condition and needs branches", key)
	default:
		return condNode{key: key, test: test, then: then, otherwise: otherwise}, nil
	}
}

const macroNameBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"

func formatString(value, format string) (string, error) {
	switch format {
	case "":
		return value, nil
	case "u":
		return strings.ToUpper(value), nil
	case "l":
		return strings.ToLower(value), nil
	default:
		return "", fmt.Errorf("invalid string format %q", format)
	}
}

// formatID formats an id: u and l as the string the C# context holds, any other
// format as a padded number.
func formatID(value int64, format string) (string, error) {
	switch format {
	case "u", "l":
		return formatString(strconv.FormatInt(value, 10), format)
	default:
		return formatInt(value, format)
	}
}

// formatInt implements the digit part of the C# custom number format: '0' is a
// required digit, so its count is the minimum width, and '#' is optional, so it
// adds no width. Grouping and the other number specifiers are not supported.
func formatInt(value int64, format string) (string, error) {
	width := 0
	for i := 0; i < len(format); i++ {
		switch format[i] {
		case '0':
			width++
		case '#':
		default:
			return "", fmt.Errorf("invalid integer format %q", format)
		}
	}

	s := strconv.FormatInt(value, 10)
	digits := strings.TrimPrefix(s, "-")
	if len(digits) < width {
		digits = strings.Repeat("0", width-len(digits)) + digits
	}

	if strings.HasPrefix(s, "-") {
		return "-" + digits, nil
	}

	return digits, nil
}

func formatTime(value time.Time, format string) (string, error) {
	if format == "" {
		format = defaultTimeFormat
	}

	layout := goTimeLayout(format)
	if layout == "" {
		return "", fmt.Errorf("invalid time format %q", format)
	}

	return value.Format(layout), nil
}

// timeTokens maps a C# custom date token to the Go layout text for each run
// length it models. A length missing from the map is not supported.
var timeTokens = map[rune]map[int]string{
	'y': {1: "2006", 2: "06", 4: "2006"},
	'M': {1: "1", 2: "01"},
	'd': {1: "2", 2: "02"},
	'H': {2: "15"},
	'h': {1: "3", 2: "03"},
	'm': {1: "4", 2: "04"},
	's': {1: "5", 2: "05"},
}

// goTimeLayout rewrites the C# custom date tokens in format as Go layout text.
// An unmodelled token returns "", so the caller reports the format rather than
// printing text that would mean something else in Go.
func goTimeLayout(format string) string {
	runes := []rune(format)
	var b strings.Builder
	for i := 0; i < len(runes); {
		c := runes[i]
		j := i
		for j < len(runes) && runes[j] == c {
			j++
		}

		if layouts, ok := timeTokens[c]; ok {
			layout, ok := layouts[j-i]
			if !ok {
				return ""
			}

			b.WriteString(layout)
		} else if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			// A letter run that is not a modelled token is one C# would
			// format; printing it as Go layout text would be a wrong date.
			return ""
		} else {
			b.WriteString(string(runes[i:j]))
		}

		i = j
	}

	return b.String()
}
