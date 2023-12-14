package str

import (
	"regexp"
	"strings"
)

var SPACE_OR_TABLE = regexp.MustCompile("[ \t]+")

// trim spaces, remove \n and
// replace /[ \t]+/ with a single ' '
func Simplify(s string) string {
	s = strings.Trim(s, "\n\t ")
	s = strings.ReplaceAll(s, "\n", "")
	s = SPACE_OR_TABLE.ReplaceAllString(s, " ")
	return s
}
