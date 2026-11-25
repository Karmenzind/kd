package decorate

import "testing"

func TestApplyThemeAndConfig(t *testing.T) {
	ApplyConfig(true)
	if got := Emo("star %s", "🌟"); got != "star 🌟" {
		t.Fatalf("Emo with emoji enabled = %q", got)
	}
	ApplyConfig(false)
	if got := Emo("plain %s", "text"); got != "plain text" {
		t.Fatalf("Emo with emoji disabled = %q", got)
	}

	for _, theme := range Themes {
		t.Run(theme, func(t *testing.T) {
			ApplyTheme(theme)
			if Title == nil || Text == nil {
				t.Fatalf("ApplyTheme(%q) left nil renderers", theme)
			}
			if got := Title("word"); got == "" {
				t.Fatal("Title renderer returned empty string")
			}
		})
	}
}
