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
