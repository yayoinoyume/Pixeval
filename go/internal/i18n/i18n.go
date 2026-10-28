// Package i18n holds the interface messages: one JSON file per language,
// embedded in the binary, with English as the fallback for a missing key.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// The languages the app ships. LangAuto follows the system's locale.
const (
	LangAuto   = "auto"
	LangZhHans = "zh-Hans"
	LangEnUS   = "en-US"
)

//go:embed locales/*.json
var locales embed.FS

// Bundle is one language's messages, with English behind them.
type Bundle struct {
	lang     string
	messages map[string]string
	fallback map[string]string
}

// Load reads the messages of lang, which may be LangAuto for the system's
// language. A language the app does not ship reads as English.
func Load(lang string) (*Bundle, error) {
	en, err := loadFile(LangEnUS)
	if err != nil {
		return nil, err
	}
	lang = Normalize(lang)
	if lang == LangEnUS {
		return &Bundle{lang: lang, messages: en, fallback: en}, nil
	}
	msgs, err := loadFile(lang)
	if err != nil {
		return nil, err
	}
	return &Bundle{lang: lang, messages: msgs, fallback: en}, nil
}

// Normalize maps a language tag to the one the app ships, or to the system's
// language when it is LangAuto. Every other zh tag reads as Simplified
// Chinese: the app ships only these two languages for now.
func Normalize(lang string) string {
	switch {
	case lang == "" || lang == LangAuto:
		return Detect()
	case strings.HasPrefix(strings.ToLower(lang), "zh"):
		return LangZhHans
	default:
		return LangEnUS
	}
}

// Detect is the language the environment asks for: Chinese when the locale
// names one, English otherwise.
func Detect() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" {
			if strings.Contains(strings.ToLower(v), "zh") {
				return LangZhHans
			}
			return LangEnUS
		}
	}
	return LangEnUS
}

// loadFile reads one language's messages. The files group keys by module, so
// a key is its group and name joined by a dot.
func loadFile(lang string) (map[string]string, error) {
	data, err := locales.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return nil, fmt.Errorf("i18n: %w", err)
	}
	return parseMessages(lang, data)
}

// parseMessages flattens one locale file's JSON into messages.
func parseMessages(lang string, data []byte) (map[string]string, error) {
	var groups map[string]map[string]string
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("i18n %s: %w", lang, err)
	}
	msgs := make(map[string]string)
	for group, entries := range groups {
		for key, text := range entries {
			msgs[group+"."+key] = text
		}
	}
	return msgs, nil
}

// Lang is the language the bundle speaks.
func (b *Bundle) Lang() string {
	if b == nil {
		return LangEnUS
	}
	return b.lang
}

// T is the message for key, English when this language lacks it, and the key
// itself when no language has it.
func (b *Bundle) T(key string) string {
	if b != nil {
		if s, ok := b.messages[key]; ok {
			return s
		}
		if s, ok := b.fallback[key]; ok {
			return s
		}
	}
	return key
}

// Tf is T with the format's arguments filled in.
func (b *Bundle) Tf(key string, args ...any) string {
	return fmt.Sprintf(b.T(key), args...)
}
