package agent

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/deividfortuna/babysitter/internal/session"
)

func Sanitize(s string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsControl(r) || unicode.In(r, invisible...):
			return -1
		}
		return r
	}, lineBreaks.Replace(session.Strip(s)))
	for _, marker := range []string{BeginData, EndData} {
		clean = strings.ReplaceAll(clean, marker, markerRemoved)
	}
	return clean
}

const markerRemoved = "[data marker removed]"

var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n", " ", "\n", " ", "\n")

var invisible = []*unicode.RangeTable{
	unicode.Cf,
	unicode.Other_Default_Ignorable_Code_Point,
	unicode.Variation_Selector,
	{R32: []unicode.Range32{
		{Lo: 0x1107f, Hi: 0x1107f, Stride: 1},
		{Lo: 0x16fe4, Hi: 0x16fe4, Stride: 1},
	}},
}

func Fence(s string) string {
	longest := 0
	for line := range strings.SplitSeq(s, "\n") {
		n := 0
		for _, r := range line {
			if r != '`' {
				break
			}
			n++
		}
		if n > longest {
			longest = n
		}
	}
	if longest < 3 {
		longest = 3
	}
	return strings.Repeat("`", longest+1)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var bareWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func ShellWord(s string) string {
	if bareWord.MatchString(s) {
		return s
	}
	return shellQuote(s)
}

func ShellJoin(argv []string) string {
	words := make([]string, 0, len(argv))
	for _, a := range argv {
		words = append(words, shellQuote(a))
	}
	return strings.Join(words, " ")
}

func PowerShellJoin(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	words := make([]string, 0, len(argv))
	for _, a := range argv {
		words = append(words, "'"+strings.ReplaceAll(a, "'", "''")+"'")
	}
	return "& " + strings.Join(words, " ")
}
