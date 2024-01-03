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

func EchoWarn(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(WarnBg("⚠ WARNING:"), Warn(content))
}

func EchoError(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(ErrorBg("☣ ERROR:"), Error(content))
}

func EchoFatal(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(ErrorBg("☣ ERROR:"), Error(content))
	os.Exit(1)
}

func EchoRun(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(Blue("≫ "), Blue(content))
}

func EchoOkay(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(Green("✔ "), Green(content))
}

func EchoFine(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(Green("☺ "), Green(content))
}

func EchoWrong(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(Red("✘ "), Red(content))
}

func EchoWeakNotice(content string, a ...any) {
	if len(a) > 0 {
		content = fmt.Sprintf(content, a...)
	}
	fmt.Println(Gray("☺ "), Gray(content))
}
