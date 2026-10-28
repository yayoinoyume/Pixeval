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
	LangFrFR   = "fr-FR"
	LangRuRU   = "ru-RU"
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
// language when it is LangAuto. Any other zh tag reads as Simplified Chinese,
// any other fr tag as French, any other ru tag as Russian; the app ships no
// other language, so everything else reads as English.
func Normalize(lang string) string {
	if lang == "" || lang == LangAuto {
		return Detect()
	}
	return tagLanguage(lang)
}

// tagLanguage maps a locale tag to a shipped language, English for the rest.
func tagLanguage(tag string) string {
	tag = strings.ToLower(tag)
	switch {
	case strings.HasPrefix(tag, "zh"):
		return LangZhHans
	case strings.HasPrefix(tag, "fr"):
		return LangFrFR
	case strings.HasPrefix(tag, "ru"):
		return LangRuRU
	default:
		return LangEnUS
	}
}

// Detect is the language the environment asks for: the shipped language its
// locale names, English when it names none.
func Detect() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" {
			return tagLanguage(v)
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
