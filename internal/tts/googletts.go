package tts

import (
	"fmt"
	"net/url"
)

func buildAudioUrl(word string) string {
	encodedWord := url.QueryEscape(word)
	return fmt.Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&q=%s&tl=en&client=tw-ob", encodedWord)
}

