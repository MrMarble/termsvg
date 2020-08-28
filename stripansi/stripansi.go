package stripansi

import (
	"regexp"
)

var (
	pattern = `[\\u001B\\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[-a-zA-Z\\d\\/#&.:=?%@~_]*)*)?\\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))`
	// AnsiRegex holds the regex expression to interact with ansi escape sequences
	AnsiRegex = regexp.MustCompile(pattern)
)

// Strip removes ansi escape sequences from string
func Strip(text string) string {
	return AnsiRegex.ReplaceAllString(text, "")
}
