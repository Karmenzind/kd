package decorate

import (
    "fmt"
    "os"
)

/*
✓ 2713 Check mark
✔ 2714 Heavy check mark
✕ 2715 Multiplication
✖ 2716 Heavy multiplication
✗ 2717 Ballot X
✘ 2718 Heavy ballot X
🉑  📥  ℹ  🇺🇸  🇬🇧   🗣  👄  👀  🎈
*/

func f(s string, a ...any) string {
    if len(a) > 0 {
        return fmt.Sprintf(s, a...)
    }
    return s
}

// Abnormal
// --------------------------------------------

func EchoWarn(content string, a ...any) {
    fmt.Fprintln(os.Stderr, WarnBg("⚠ WARNING:"), Warn(f(content, a...)))
}

func EchoError(content string, a ...any) {
    fmt.Fprintln(os.Stderr, ErrorBg("☣ ERROR:"), Error(f(content, a...)))
}

func EchoFatal(content string, a ...any) {
    fmt.Fprintln(os.Stderr, ErrorBg("☣ ERROR:"), Error(f(content, a...)))
    os.Exit(1)
}

func EchoWrong(content string, a ...any) {
    fmt.Fprintln(os.Stderr, Red("✘ "), Red(f(content, a...)))
}

// Normal
// --------------------------------------------

func EchoRun(content string, a ...any) {
    fmt.Println(Blue("≫ "), Blue(f(content, a...)))
}

func EchoOkay(content string, a ...any) {
    fmt.Println(Green("✔ "), Green(f(content, a...)))
}

func EchoFine(content string, a ...any) {
    fmt.Println(Green("☺ "), Green(f(content, a...)))
}

func EchoWeakNotice(content string, a ...any) {
    fmt.Println(Gray("☺ "), Gray(f(content, a...)))
}
