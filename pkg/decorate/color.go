package decorate

import (
	"fmt"

	fc "github.com/fatih/color"
	gc "github.com/gookit/color"
)

/*
https://zetcode.com/golang/terminal-colour/#google_vignette
*/

// type ColorStringFunc func(s string, mode ...string)
// func mix(value ...color.Attribute) ColorStringFunc {
// 	return func(s string, mode ...string) {
// 	}
// }

//  -----------------------------------------------------------------------------
//  fatih version
//  -----------------------------------------------------------------------------

// fg
var Yellow = fc.New(fc.FgYellow).SprintfFunc()
var Red = fc.New(fc.FgRed).SprintfFunc()
var Green = fc.New(fc.FgGreen).SprintfFunc()
var GreenBg = fc.New(fc.BgGreen, fc.FgBlack).SprintfFunc()
var Blue = fc.New(fc.FgBlue).SprintfFunc()
var BlueBg = fc.New(fc.BgBlue, fc.FgBlack).SprintfFunc()
var Cyan = fc.New(fc.FgCyan).SprintfFunc()
var Gray = fc.New(fc.FgHiBlack).SprintfFunc()

// with bg

// bold/italic
var B = fc.New(fc.Bold).SprintfFunc()
var I = fc.New(fc.Italic).SprintfFunc()
var U = fc.New(fc.Underline).SprintfFunc()
var F = fc.New(fc.Faint).SprintfFunc()

// special
var Info = fc.New(fc.FgWhite).SprintfFunc()

// var Error = fc.Error

//  -----------------------------------------------------------------------------
//  gookit version
//  -----------------------------------------------------------------------------

// assignColor
var a = gc.C256(132)

var Warn = fc.New(fc.FgYellow, fc.Italic).SprintFunc()
var WarnBg = fc.New(fc.BgYellow, fc.FgBlack, fc.Italic, fc.Faint).SprintFunc()
var Error = fc.New(fc.FgRed, fc.Italic).SprintFunc()
var ErrorBg = fc.New(fc.BgRed, fc.FgBlack, fc.Italic, fc.Faint).SprintFunc()


// Theme
type ColorStringFunc func(a ...interface{}) string

var Title, Nation, Line, Pron, Property, Idx, Addi, Para,
	CollinsPara, Eg, EgPref, EgEn, EgCh,
	Star, Rank ColorStringFunc

var Text = fc.New(fc.FgWhite).SprintFunc()

func applyTheme(colorscheme string) {
	switch colorscheme {
	case "", "temp":
		Title = fc.New(fc.FgHiMagenta, fc.Italic, fc.Bold, fc.Underline).SprintFunc()
		Line = fc.New(fc.FgHiBlack, fc.Faint).SprintFunc()
		Pron = fc.New(fc.Faint).SprintFunc()
		Property = fc.New(fc.FgGreen).SprintFunc()
		Idx = fc.New(fc.FgHiWhite).SprintFunc()
		Addi = fc.New(fc.FgCyan, fc.Italic).SprintFunc()
        Para = Text
        CollinsPara = fc.New(fc.FgYellow).SprintFunc()
		Eg = fc.New(fc.FgHiWhite, fc.Faint, fc.Italic).SprintFunc()
		EgPref = Eg
		// EgEn = Text
		EgEn = fc.New(fc.FgHiWhite, fc.BgBlack).SprintFunc()
		EgCh = fc.New(fc.FgHiWhite, fc.Faint, fc.Italic).SprintFunc()
		Star = fc.New(fc.FgYellow).SprintFunc()
		Rank = Eg

	case "wudao":
		Title = fc.New(fc.FgRed, fc.Italic, fc.Bold, fc.Underline).SprintFunc()
		Line = fc.New(fc.FgHiBlack, fc.Faint).SprintFunc()
		Pron = fc.New(fc.FgCyan).SprintFunc()
		Property = fmt.Sprint
		Idx = fc.New(fc.FgHiWhite).SprintFunc()
		Addi = fc.New(fc.FgGreen, fc.Italic).SprintFunc()
        Para = Text
        CollinsPara = fc.New(fc.FgHiWhite).SprintFunc()
		Eg = fc.New(fc.FgHiYellow, fc.Faint, fc.Italic).SprintFunc()
		EgPref = Addi
		EgEn = fc.New(fc.FgYellow, fc.Italic).SprintFunc()
		EgCh = Text
		Star = fc.New(fc.FgYellow).SprintFunc()
		Rank = fc.New(fc.FgRed, fc.Italic).SprintFunc()

	default:
		panic(fmt.Errorf("unknown theme: %s", colorscheme))
	}
}
