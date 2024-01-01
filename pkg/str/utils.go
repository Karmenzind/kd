package str

import (
	"regexp"
	"strings"
)

var SPACE_OR_TABLE = regexp.MustCompile("[ \t]+")

// trim spaces, remove \n and
// replace /[ \t]+/ with a single ' '
func Simplify(s string) string {
	s = strings.Trim(s, " \n\t ") // 结尾包含不可见unicode
	s = strings.ReplaceAll(s, "\n", "")
	s = SPACE_OR_TABLE.ReplaceAllString(s, " ")
	return s
}


func InSlice(s string, l []string) bool {
    for i := 0; i < len(s); i++ {
        if s == l[i] {
            return true
        }
    }
    return false
}
