package stripansi

import (
	"regexp"
)

var (
	pattern = `(?i)\\u001B\[.*?m`
	// AnsiRegex holds the regex expression to interact with ansi escape sequences
	AnsiRegex = regexp.MustCompile(pattern)
)

// Strip removes ansi escape sequences from string
func Strip(text string) string {
	return AnsiRegex.ReplaceAllString(text, "")
}
