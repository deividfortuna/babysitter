package checks

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultLogBefore = 60
	defaultLogAfter  = 20
	defaultLogLines  = 200
)

const (
	groupMarker    = "##[group]"
	endGroupMarker = "##[endgroup]"
	errorMarker    = "##[error]"
)

const (
	escape = 0x1b
	bell   = 0x07
	del    = 0x7f
)

type LogOptions struct {
	Before   int
	After    int
	MaxLines int
}

func (o LogOptions) withDefaults() LogOptions {
	if o.Before <= 0 {
		o.Before = defaultLogBefore
	}
	if o.After <= 0 {
		o.After = defaultLogAfter
	}
	if o.MaxLines <= 0 {
		o.MaxLines = defaultLogLines
	}
	return o
}

type LogExcerpt struct {
	Step string
	Text string
}

func TrimLog(raw string, o LogOptions) LogExcerpt {
	o = o.withDefaults()
	lines := trimBlankLines(readLog(raw))
	if len(lines) == 0 {
		return LogExcerpt{}
	}
	spans := fitBudget(errorWindows(lines, o), o.MaxLines)
	if len(spans) == 0 {
		spans = []span{{from: max(0, len(lines)-o.MaxLines), to: len(lines)}}
	}
	return LogExcerpt{Step: failingStep(lines), Text: renderLines(lines, spans)}
}

type logLine struct {
	text  string
	group string
	fail  bool
}

func readLog(raw string) []logLine {
	var out []logLine
	group := ""
	for text := range strings.SplitSeq(stripEscapes(raw), "\n") {
		text = dropTimestamp(strings.TrimRight(text, " \t"))
		switch {
		case strings.HasPrefix(text, groupMarker):
			group = strings.TrimPrefix(text, groupMarker)
			text = group
		case strings.HasPrefix(text, endGroupMarker):
			continue
		}
		out = append(out, logLine{text: text, group: group, fail: strings.HasPrefix(text, errorMarker)})
	}
	return out
}

func stripEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == escape {
			i = endOfSequence(s, i)
			continue
		}
		if keepsInLog(s[i]) {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func keepsInLog(c byte) bool {
	switch c {
	case '\n', '\t':
		return true
	case del:
		return false
	default:
		return c >= 0x20
	}
}

func endOfSequence(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[':
		for j := i + 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j
			}
		}
		return len(s)
	case ']':
		for j := i + 2; j < len(s); j++ {
			if s[j] == bell {
				return j
			}
			if s[j] == escape && j+1 < len(s) && s[j+1] == '\\' {
				return j + 1
			}
		}
		return len(s)
	default:
		return i + 1
	}
}

func dropTimestamp(s string) string {
	end := strings.IndexByte(s, ' ')
	if end < 0 {
		end = len(s)
	}
	stamp := s[:end]
	if len(stamp) < 20 || stamp[10] != 'T' || stamp[len(stamp)-1] != 'Z' {
		return s
	}
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		return s
	}
	return strings.TrimPrefix(s[end:], " ")
}

func trimBlankLines(lines []logLine) []logLine {
	for len(lines) > 0 && lines[0].text == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1].text == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func failingStep(lines []logLine) string {
	for _, l := range lines {
		if l.fail {
			return l.group
		}
	}
	return lines[len(lines)-1].group
}

type span struct{ from, to int }

func errorWindows(lines []logLine, o LogOptions) []span {
	var out []span
	for i, l := range lines {
		if !l.fail {
			continue
		}
		s := span{from: max(0, i-o.Before), to: min(len(lines), i+o.After+1)}
		if last := len(out) - 1; last >= 0 && s.from <= out[last].to {
			out[last].to = max(out[last].to, s.to)
			continue
		}
		out = append(out, s)
	}
	return out
}

func fitBudget(spans []span, n int) []span {
	var out []span
	for _, s := range spans {
		if n <= 0 {
			break
		}
		if s.to-s.from > n {
			s.to = s.from + n
		}
		n -= s.to - s.from
		out = append(out, s)
	}
	return out
}

func renderLines(lines []logLine, spans []span) string {
	var b strings.Builder
	end := 0
	for _, s := range spans {
		writeOmitted(&b, s.from-end)
		for _, l := range lines[s.from:s.to] {
			b.WriteString(l.text)
			b.WriteByte('\n')
		}
		end = s.to
	}
	writeOmitted(&b, len(lines)-end)
	return b.String()
}

func writeOmitted(b *strings.Builder, n int) {
	switch {
	case n <= 0:
	case n == 1:
		b.WriteString("[1 line omitted]\n")
	default:
		fmt.Fprintf(b, "[%d lines omitted]\n", n)
	}
}
