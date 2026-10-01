package agent

import (
	"strings"
	"unicode"
)

const shellSeparators = ";&|\n()`"

const escapedInDoubleQuotes = "$`\"\\\n"

const redirectionOperators = "<>&"

type shellWord struct {
	text      string
	separator bool
	redirect  bool
}

type shellSplitter struct {
	words  []shellWord
	word   strings.Builder
	inWord bool
}

func splitShellWords(command string) []shellWord {
	var s shellSplitter
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			i++
			if i < len(runes) && runes[i] != '\n' {
				s.add(runes[i])
			}
		case r == '$' && followedBy(runes, i, '\''):
			i = s.addANSIQuoted(runes, i+2)
		case r == '$' && followedBy(runes, i, '"'):
			i = s.addDoubleQuoted(runes, i+2)
		case r == '\'':
			i = s.addSingleQuoted(runes, i+1)
		case r == '"':
			i = s.addDoubleQuoted(runes, i+1)
		case r == ' ' || r == '\t':
			s.flush()
		case startsRedirection(runes, i):
			i = s.addRedirection(runes, i)
		case strings.ContainsRune(shellSeparators, r):
			s.flush()
			s.words = append(s.words, shellWord{text: string(r), separator: true})
		default:
			s.add(r)
		}
	}
	s.flush()
	return s.words
}

func (s *shellSplitter) add(r rune) {
	s.word.WriteRune(r)
	s.inWord = true
}

func (s *shellSplitter) flush() {
	if !s.inWord {
		return
	}
	s.words = append(s.words, shellWord{text: s.word.String()})
	s.word.Reset()
	s.inWord = false
}

func startsRedirection(runes []rune, i int) bool {
	if runes[i] == '<' || runes[i] == '>' {
		return true
	}
	return runes[i] == '&' && i+1 < len(runes) && runes[i+1] == '>'
}

func (s *shellSplitter) addRedirection(runes []rune, from int) int {
	if isFileDescriptor(s.word.String()) {
		s.word.Reset()
		s.inWord = false
	}
	s.flush()
	end := from + 1
	for end < len(runes) && strings.ContainsRune(redirectionOperators, runes[end]) {
		end++
	}
	if end < len(runes) && runes[end-1] == '>' && runes[end] == '|' {
		end++
	}
	s.words = append(s.words, shellWord{text: string(runes[from:end]), redirect: true})
	return end - 1
}

func isFileDescriptor(word string) bool {
	if word == "" {
		return false
	}
	named := len(word) > 2 && strings.HasPrefix(word, "{") && strings.HasSuffix(word, "}")
	return named || strings.Trim(word, "0123456789") == ""
}

func (s *shellSplitter) addSingleQuoted(runes []rune, from int) int {
	s.inWord = true
	for i := from; i < len(runes); i++ {
		if runes[i] == '\'' {
			return i
		}
		s.word.WriteRune(runes[i])
	}
	return len(runes)
}

func followedBy(runes []rune, i int, next rune) bool {
	return i+1 < len(runes) && runes[i+1] == next
}

var ansiEscapes = map[rune]rune{
	'a': '\a', 'b': '\b', 'e': 0x1b, 'E': 0x1b, 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v',
	'\\': '\\', '\'': '\'', '"': '"', '?': '?',
}

func (s *shellSplitter) addANSIQuoted(runes []rune, from int) int {
	s.inWord = true
	for i := from; i < len(runes); i++ {
		switch runes[i] {
		case '\'':
			return i
		case '\\':
			i = s.addANSIEscape(runes, i+1)
		default:
			s.word.WriteRune(runes[i])
		}
	}
	return len(runes)
}

func (s *shellSplitter) addANSIEscape(runes []rune, at int) int {
	if at >= len(runes) {
		s.word.WriteRune('\\')
		return at
	}
	r := runes[at]
	switch {
	case r == 'x':
		return s.addEscapedNumber(runes, at, at+1, 2, 16, true)
	case r == 'u':
		return s.addEscapedNumber(runes, at, at+1, 4, 16, false)
	case r == 'U':
		return s.addEscapedNumber(runes, at, at+1, 8, 16, false)
	case r >= '0' && r <= '7':
		return s.addEscapedNumber(runes, at, at, 3, 8, true)
	case r == 'c' && at+1 < len(runes):
		s.word.WriteRune(runes[at+1] & 0x1f)
		return at + 1
	}
	if mapped, ok := ansiEscapes[r]; ok {
		s.word.WriteRune(mapped)
		return at
	}
	s.word.WriteRune('\\')
	s.word.WriteRune(r)
	return at
}

func (s *shellSplitter) addEscapedNumber(runes []rune, at, from, most, base int, asByte bool) int {
	value, end := 0, from
	for end < len(runes) && end-from < most {
		digit, ok := digitValue(runes[end], base)
		if !ok {
			break
		}
		value = value*base + digit
		end++
	}
	if end == from {
		s.word.WriteRune('\\')
		s.word.WriteRune(runes[at])
		return at
	}
	if asByte {
		s.word.WriteByte(byte(value))
	} else {
		s.word.WriteRune(rune(value))
	}
	return end - 1
}

func digitValue(r rune, base int) (int, bool) {
	value := strings.IndexRune("0123456789abcdef", unicode.ToLower(r))
	return value, value >= 0 && value < base
}

func (s *shellSplitter) addDoubleQuoted(runes []rune, from int) int {
	s.inWord = true
	for i := from; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '"':
			return i
		case r == '\\' && escapesNext(runes, i):
			i++
			if runes[i] != '\n' {
				s.word.WriteRune(runes[i])
			}
		default:
			s.word.WriteRune(r)
		}
	}
	return len(runes)
}

func escapesNext(runes []rune, i int) bool {
	return i+1 < len(runes) && strings.ContainsRune(escapedInDoubleQuotes, runes[i+1])
}
