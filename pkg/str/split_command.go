package str

import "errors"

var (
	ErrUnclosedQuote     = errors.New("unclosed quote")
	ErrTrailingBackslash = errors.New("trailing backslash")
)

// SplitCommand splits s into shell-style words for pager command lines.
func SplitCommand(s string) ([]string, error) {
	var words []string
	var cur []rune
	inWord := false
	inSingle := false
	inDouble := false
	inComment := false
	escape := false
	dEscape := false

	commit := func() {
		words = append(words, string(cur))
		cur = cur[:0]
		inWord = false
	}

	for _, r := range s {
		if inComment {
			if r == '\n' {
				inComment = false
			}
			continue
		}

		if dEscape {
			dEscape = false
			cur = append(cur, r)
			continue
		}
		if escape {
			escape = false
			cur = append(cur, r)
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
			} else {
				cur = append(cur, r)
			}
			continue
		}

		if inDouble {
			switch r {
			case '\\':
				dEscape = true
			case '"':
				inDouble = false
			default:
				cur = append(cur, r)
			}
			continue
		}

		if !inWord {
			switch r {
			case ' ', '\t', '\r', '\n':
				continue
			case '#':
				inComment = true
				continue
			}
			inWord = true
		}

		switch r {
		case ' ', '\t', '\r', '\n':
			commit()
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			escape = true
		default:
			cur = append(cur, r)
		}
	}

	if dEscape || escape {
		return words, ErrTrailingBackslash
	}
	if inSingle || inDouble {
		return words, ErrUnclosedQuote
	}
	if inWord {
		commit()
	}
	return words, nil
}
