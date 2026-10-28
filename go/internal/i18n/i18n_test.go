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
	b, err := Load("fr-FR")
	if err != nil {
		t.Fatalf("Load fr-FR: %v", err)
	}
	if got := b.Lang(); got != LangEnUS {
		t.Errorf("lang %q, want %q", got, LangEnUS)
	}
	if got := b.T("nav.home"); got != "Home" {
		t.Errorf("nav.home %q, want Home", got)
	}
}

// TestLocalesMatch checks both embedded files parse and carry the same keys: a
// missing file fails Load, and a key added to one language alone fails here.
func TestLocalesMatch(t *testing.T) {
	zh, err := Load(LangZhHans)
	if err != nil {
		t.Fatalf("Load zh-Hans: %v", err)
	}
	en, err := Load(LangEnUS)
	if err != nil {
		t.Fatalf("Load en-US: %v", err)
	}
	if len(en.messages) < 70 || len(zh.messages) < 70 {
		t.Fatalf("too few messages: zh %d, en %d", len(zh.messages), len(en.messages))
	}
	for key := range en.messages {
		if _, ok := zh.messages[key]; !ok {
			t.Errorf("%s missing from zh-Hans", key)
		}
	}
	for key := range zh.messages {
		if _, ok := en.messages[key]; !ok {
			t.Errorf("%s missing from en-US", key)
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

// TestDetect checks the locale variables choose the language, LC_MESSAGES
// first.
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
	t.Setenv("LANG", "")
	if got := Detect(); got != LangEnUS {
		t.Errorf("unset LANG %q, want %q", got, LangEnUS)
	}
}
