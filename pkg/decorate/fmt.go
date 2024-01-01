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
*/
func EchoWarn(content string) {
	fmt.Println(WarnBg("⚠ WARNING:"), Warn(content))
}

func EchoError(content string) {
	fmt.Println(ErrorBg("☣ ERROR:"), Error(content))
}

func EchoFatal(content string) {
	fmt.Println(ErrorBg("☣ ERROR:"), Error(content))
	os.Exit(1)
}

func EchoRun(content string) {
	fmt.Println(Blue("≫ "), Blue(content))
}

func EchoOkay(content string) {
	fmt.Println(Green("✔ "), Green(content))
}

func EchoFine(content string) {
	fmt.Println(Green("☺ "), Green(content))
}

func EchoWrong(content string) {
	fmt.Println(Red("✘ "), Red(content))
}

func EchoWeakNotice(content string) {
	fmt.Println(Gray("☺ "), Gray(content))
}
