package textx

import (
	"fmt"
	"strings"
)

const ellipsis = "..."

func FirstLine(s string, limit int) string {
	line := ""
	for raw := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(strings.TrimSuffix(raw, "\r")); line != "" {
			break
		}
	}
	if limit <= 0 {
		return line
	}
	runes := []rune(line)
	if len(runes) <= limit {
		return line
	}
	if limit <= len(ellipsis) {
		return string(runes[:limit])
	}
	return string(runes[:limit-len(ellipsis)]) + ellipsis
}

func Plural(n int, word string) string {
	return Count(n, word, word+"s")
}

func Count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func JoinAnd(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
