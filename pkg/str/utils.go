package str

import (
	"regexp"
	"strings"
)

var SPACE_OR_TABLE = regexp.MustCompile(`[ \t\p{Zs}]+`)

var lineBreaks = regexp.MustCompile(`\r\n|\r|\n`)

// Simplify 把文本归一成单行：换行按HTML的语义折成空格（否则`young\nsheep`
// 会被粘成`youngsheep`），连续空白（含不可见unicode空格）合并为一个，两端裁掉。
func Simplify(s string) string {
	s = lineBreaks.ReplaceAllString(s, " ")
	s = SPACE_OR_TABLE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// identifierTokenPat 匹配标识符式的 token（字母或下划线开头，后续为字母/数字/下划线）。
var identifierTokenPat = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// SplitIdentifier 把标识符式字符串拆成组成词，支持 camelCase/PascalCase 与下划线：
//
//	helloWorld      -> [hello World]
//	HTTPServer      -> [HTTP Server]
//	getHTTPResponse -> [get HTTP Response]
//	getUserByID     -> [get User By ID]
//	hello_world     -> [hello world]
//	_private        -> [private]
//	user2FA         -> [user2 FA]
//
// 不含任何边界的字符串会原样返回为单元素切片；空串、纯下划线会返回空切片。
func SplitIdentifier(s string) []string {
	segments := make([]string, 0, 2)
	for _, chunk := range strings.Split(s, "_") {
		if chunk == "" {
			continue
		}
		segments = append(segments, splitCamelCase(chunk)...)
	}
	return segments
}

// splitCamelCase 在大小写边界处切分，数字不单独成段而是跟随前一段。
func splitCamelCase(s string) []string {
	runes := []rune(s)
	parts := make([]string, 0, 2)
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		if !isUpperASCII(cur) {
			continue
		}
		switch {
		case isLowerASCII(prev) || isDigitASCII(prev):
			// helloWorld、user2FA
		case isUpperASCII(prev) && i+1 < len(runes) && isLowerASCII(runes[i+1]):
			// HTTPServer，把连续大写视为整体
		default:
			continue
		}
		parts = append(parts, string(runes[start:i]))
		start = i
	}
	return append(parts, string(runes[start:]))
}

func isUpperASCII(r rune) bool { return r >= 'A' && r <= 'Z' }

func isLowerASCII(r rune) bool { return r >= 'a' && r <= 'z' }

func isDigitASCII(r rune) bool { return r >= '0' && r <= '9' }

// IdentifierPhrase 返回标识符拆词后以空格拼接的小写形式，ok 表示结果与原始输入不同。
// 普通单词、词组、纯全大写词不会触发（ok=false）。
func IdentifierPhrase(s string) (string, bool) {
	segments := SplitIdentifier(s)
	if len(segments) == 0 {
		return "", false
	}
	phrase := strings.ToLower(strings.Join(segments, " "))
	if phrase == strings.ToLower(s) {
		return "", false
	}
	return phrase, true
}

// RewriteIdentifiers 把文本中每个标识符式 token 替换为空格分词后的小写形式，
// 其余内容保持不变。用于长文本翻译前的预处理。
func RewriteIdentifiers(text string) string {
	return identifierTokenPat.ReplaceAllStringFunc(text, func(token string) string {
		if phrase, ok := IdentifierPhrase(token); ok {
			return phrase
		}
		return token
	})
}

// SimplifyLines 按行拆分文本，逐行Simplify并丢弃空行。
// 用于处理缓存中残留的、未经清洗的HTML原始文本（含换行与大段缩进）。
func SimplifyLines(s string) []string {
	raw := lineBreaks.Split(s, -1)
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if line = Simplify(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
