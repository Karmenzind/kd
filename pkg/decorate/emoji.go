package decorate

import "fmt"

var fallbackMap = map[string]string{
	"🌟": "★",
}

// emojify sprintf
func Emo(format string, a ...any) string {
	if emojiEnabled {
		return fmt.Sprintf(format, a...)
	}
	return fmt.Sprintf(format, a...)
}
