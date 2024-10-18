package decorate

var emojiEnabled = false

func ApplyConfig(enableEmoji bool) {
    emojiEnabled = enableEmoji
}

func ApplyTheme(theme string) {
    applyTheme(theme)
}
