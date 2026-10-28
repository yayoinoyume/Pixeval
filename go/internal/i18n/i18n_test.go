package i18n

import "testing"

// TestLanguagesDiffer checks the same key reads differently in the two
// languages, so a copy cannot pass for a translation.
func TestLanguagesDiffer(t *testing.T) {
	zh, err := Load(LangZhHans)
	if err != nil {
		t.Fatalf("Load zh-Hans: %v", err)
	}
	en, err := Load(LangEnUS)
	if err != nil {
		t.Fatalf("Load en-US: %v", err)
	}
	for _, key := range []string{"nav.home", "common.retry", "settings.language"} {
		if zh.T(key) == en.T(key) {
			t.Errorf("%s reads %q in both languages", key, zh.T(key))
		}
	}
	if got := en.T("nav.home"); got != "Home" {
		t.Errorf("en nav.home %q, want Home", got)
	}
	if got := zh.T("nav.home"); got != "首页" {
		t.Errorf("zh nav.home %q, want 首页", got)
	}
	if got := en.Tf("download.page", 2); got != "Page 2" {
		t.Errorf("Tf %q, want Page 2", got)
	}
}

// TestMissingKeyFallsBackToEnglish checks a key only English has reads in
// English under Chinese, and a key no language has reads as itself.
func TestMissingKeyFallsBackToEnglish(t *testing.T) {
	b := &Bundle{
		lang:     LangZhHans,
		messages: map[string]string{},
		fallback: map[string]string{"common.retry": "Retry"},
	}
	if got := b.T("common.retry"); got != "Retry" {
		t.Errorf("fallback %q, want Retry", got)
	}
	if got := b.T("common.missing"); got != "common.missing" {
		t.Errorf("unknown key %q, want the key", got)
	}
}

// TestUnknownLanguageFallsBackToEnglish checks a language the app does not
// ship reads as English.
func TestUnknownLanguageFallsBackToEnglish(t *testing.T) {
	b, err := Load("de-DE")
	if err != nil {
		t.Fatalf("Load de-DE: %v", err)
	}
	if got := b.Lang(); got != LangEnUS {
		t.Errorf("lang %q, want %q", got, LangEnUS)
	}
	if got := b.T("nav.home"); got != "Home" {
		t.Errorf("nav.home %q, want Home", got)
	}
}

// TestLocalesMatch checks every embedded file parses, carries the same keys
// as English, and has a text for each: a missing file fails Load, and a key
// added to one language alone fails here.
func TestLocalesMatch(t *testing.T) {
	en, err := Load(LangEnUS)
	if err != nil {
		t.Fatalf("Load en-US: %v", err)
	}
	if len(en.messages) < 70 {
		t.Fatalf("too few messages: en %d", len(en.messages))
	}
	for key, text := range en.messages {
		if text == "" {
			t.Errorf("%s is empty in en-US", key)
		}
	}
	for _, lang := range []string{LangZhHans, LangFrFR, LangRuRU} {
		b, err := Load(lang)
		if err != nil {
			t.Fatalf("Load %s: %v", lang, err)
		}
		if got := b.Lang(); got != lang {
			t.Errorf("lang %q, want %q", got, lang)
		}
		for key := range en.messages {
			if _, ok := b.messages[key]; !ok {
				t.Errorf("%s missing from %s", key, lang)
				continue
			}
			if b.messages[key] == "" {
				t.Errorf("%s is empty in %s", key, lang)
			}
		}
		for key := range b.messages {
			if _, ok := en.messages[key]; !ok {
				t.Errorf("%s missing from en-US", key)
			}
		}
	}
}

// TestParseRejectsBadJSON checks a locale file that does not parse fails
// rather than leaving the app with a half understood language.
func TestParseRejectsBadJSON(t *testing.T) {
	if _, err := parseMessages(LangZhHans, []byte("{not json")); err == nil {
		t.Fatal("parseMessages accepted bad JSON")
	}
	if _, err := parseMessages(LangZhHans, []byte(`[]`)); err == nil {
		t.Fatal("parseMessages accepted a non-object")
	}
}

// TestTraditionalChineseReadsAsSimplified checks another zh tag lands on the
// one Chinese the app ships.
func TestTraditionalChineseReadsAsSimplified(t *testing.T) {
	for _, tag := range []string{"zh-Hant", "zh_TW.UTF-8"} {
		if got := Normalize(tag); got != LangZhHans {
			t.Errorf("Normalize(%q) = %q, want %q", tag, got, LangZhHans)
		}
		b, err := Load(tag)
		if err != nil {
			t.Fatalf("Load(%q): %v", tag, err)
		}
		if got := b.T("nav.home"); got != "首页" {
			t.Errorf("Load(%q) nav.home %q, want 首页", tag, got)
		}
	}
}

// TestNormalize checks a tag lands on the language the app ships for it, and
// anything else lands on English.
func TestNormalize(t *testing.T) {
	cases := []struct {
		tag  string
		want string
	}{
		{LangZhHans, LangZhHans},
		{"zh-Hant", LangZhHans},
		{"zh_TW.UTF-8", LangZhHans},
		{LangEnUS, LangEnUS},
		{"en-GB", LangEnUS},
		{LangFrFR, LangFrFR},
		{"fr", LangFrFR},
		{"fr_CA.UTF-8", LangFrFR},
		{LangRuRU, LangRuRU},
		{"ru", LangRuRU},
		{"ru_RU.UTF-8", LangRuRU},
		{"de-DE", LangEnUS},
	}
	for _, c := range cases {
		if got := Normalize(c.tag); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.tag, got, c.want)
		}
	}
}

// TestDetect checks the locale variables choose the language, LC_MESSAGES
// first, and that a French or Russian system gets its language.
func TestDetect(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if got := Detect(); got != LangZhHans {
		t.Errorf("zh LANG %q, want %q", got, LangZhHans)
	}
	t.Setenv("LC_MESSAGES", "en_US.UTF-8")
	if got := Detect(); got != LangEnUS {
		t.Errorf("en LC_MESSAGES over zh LANG %q, want %q", got, LangEnUS)
	}
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := Detect(); got != LangEnUS {
		t.Errorf("en LANG %q, want %q", got, LangEnUS)
	}
	t.Setenv("LC_MESSAGES", "fr_FR.UTF-8")
	if got := Detect(); got != LangFrFR {
		t.Errorf("fr LC_MESSAGES %q, want %q", got, LangFrFR)
	}
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	if got := Detect(); got != LangRuRU {
		t.Errorf("ru LANG %q, want %q", got, LangRuRU)
	}
	t.Setenv("LANG", "")
	if got := Detect(); got != LangEnUS {
		t.Errorf("unset LANG %q, want %q", got, LangEnUS)
	}
}
