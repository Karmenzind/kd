package tts

import (
	"fmt"
	"net/url"
)

var googleTTSURLFormat = "https://translate.google.com/translate_tts?ie=UTF-8&q=%s&tl=en&client=tw-ob"

func buildAudioUrl(word string) string {
	encodedWord := url.QueryEscape(word)
	return fmt.Sprintf(googleTTSURLFormat, encodedWord)
}
