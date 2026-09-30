package agent

import "strings"

const shellSeparators = ";&|\n()`"

const escapedInDoubleQuotes = "$`\"\\\n"

type shellWord struct {
	text      string
	separator bool
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
		case r == '\'':
			i = s.addSingleQuoted(runes, i+1)
		case r == '"':
			i = s.addDoubleQuoted(runes, i+1)
		case r == ' ' || r == '\t':
			s.flush()
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
