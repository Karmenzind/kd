package decorate

import (
	"testing"

	fc "github.com/fatih/color"
)

func TestThemesPreserveTextWhenColorIsDisabled(t *testing.T) {
	originalNoColor := fc.NoColor
	fc.NoColor = true
	t.Cleanup(func() {
		fc.NoColor = originalNoColor
		applyTheme("temp")
	})

	// Every name in Themes must be accepted: applyTheme exits on unknown names.
	for _, theme := range Themes {
		t.Run(theme, func(t *testing.T) {
			applyTheme(theme)
			for name, render := range map[string]ColorStringFunc{
				"text":      Text,
				"title":     Title,
				"pronounce": Pron,
				"property":  Property,
				"example":   Eg,
			} {
				if got, want := render("visible text"), "visible text"; got != want {
					t.Fatalf("%s %s renderer = %q, want %q", theme, name, got, want)
				}
			}
		})
	}
}
