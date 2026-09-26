package checks

import (
	"fmt"
	"strings"
	"testing"
)

func stampedLog(lines ...string) string {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "2026-09-18T10:23:%02d.1234567Z %s\n", i%60, l)
	}
	return b.String()
}

func TestTrimLogKeepsTheFailingStepAndTheLinesAroundTheError(t *testing.T) {
	t.Parallel()
	raw := stampedLog(
		"##[group]Run actions/checkout@v4",
		"with:",
		"##[endgroup]",
		"Syncing repository",
		"##[group]Run go test ./...",
		"##[endgroup]",
		"ok  	example/one",
		"--- FAIL: TestThing",
		"    thing_test.go:12: want 3, got 4",
		"FAIL	example/two",
		"##[error]Process completed with exit code 1.",
	)

	got := TrimLog(raw, LogOptions{})

	if got.Step != "Run go test ./..." {
		t.Errorf("Step = %q, want the step of the error", got.Step)
	}
	for _, want := range []string{"--- FAIL: TestThing", "thing_test.go:12: want 3, got 4", "##[error]Process completed with exit code 1."} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("Text does not carry %q:\n%s", want, got.Text)
		}
	}
	if strings.Contains(got.Text, "##[endgroup]") {
		t.Errorf("Text keeps the group markers:\n%s", got.Text)
	}
	if strings.Contains(got.Text, "2026-09-18T10:23") {
		t.Errorf("Text keeps the timestamps:\n%s", got.Text)
	}
}

func TestTrimLogDropsEscapeSequences(t *testing.T) {
	t.Parallel()
	raw := stampedLog("\x1b[31mred\x1b[0m output", "##[error]it broke")

	got := TrimLog(raw, LogOptions{})

	if strings.Contains(got.Text, "\x1b") {
		t.Errorf("Text keeps an escape sequence: %q", got.Text)
	}
	if !strings.Contains(got.Text, "red output") {
		t.Errorf("Text = %q, want the line without its colors", got.Text)
	}
}

func TestTrimLogLeavesOutTheStepsThatPassed(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := range 500 {
		lines = append(lines, fmt.Sprintf("setup line %d", i))
	}
	lines = append(lines, "##[group]Run make build", "##[endgroup]", "main.go:9: undefined: x", "##[error]Process completed with exit code 2.")
	raw := stampedLog(lines...)

	got := TrimLog(raw, LogOptions{Before: 5, After: 2})

	if strings.Contains(got.Text, "setup line 0") {
		t.Errorf("Text keeps the first setup line:\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "[497 lines omitted]") {
		t.Errorf("Text does not say what it left out:\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "main.go:9: undefined: x") {
		t.Errorf("Text does not carry the error:\n%s", got.Text)
	}
	if n := strings.Count(got.Text, "\n"); n > 12 {
		t.Errorf("Text has %d lines, want the window and one marker", n)
	}
}

func TestTrimLogMergesTheWindowsOfNearbyErrors(t *testing.T) {
	t.Parallel()
	raw := stampedLog("##[error]first", "middle", "##[error]second")

	got := TrimLog(raw, LogOptions{Before: 2, After: 2})

	if strings.Contains(got.Text, "omitted") {
		t.Errorf("Text splits one window in two:\n%s", got.Text)
	}
	if got.Text != "##[error]first\nmiddle\n##[error]second\n" {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestTrimLogSeparatesTheWindowsOfDistantErrors(t *testing.T) {
	t.Parallel()
	lines := []string{"##[error]first"}
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("noise %d", i))
	}
	lines = append(lines, "##[error]second")
	raw := stampedLog(lines...)

	got := TrimLog(raw, LogOptions{Before: 1, After: 1})

	if !strings.Contains(got.Text, "##[error]first") || !strings.Contains(got.Text, "##[error]second") {
		t.Errorf("Text loses an error:\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "[38 lines omitted]") {
		t.Errorf("Text does not say what it left out between them:\n%s", got.Text)
	}
}

func TestTrimLogKeepsTheTailWhenTheLogMarksNoError(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	raw := stampedLog(lines...)

	got := TrimLog(raw, LogOptions{MaxLines: 3})

	if got.Text != "[97 lines omitted]\nline 97\nline 98\nline 99\n" {
		t.Errorf("Text = %q, want the last lines and what it left out", got.Text)
	}
}

func TestTrimLogHoldsTheWholeExcerptToTheBudget(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := range 300 {
		lines = append(lines, fmt.Sprintf("##[error]failure %d", i))
	}
	raw := stampedLog(lines...)

	got := TrimLog(raw, LogOptions{Before: 0, After: 0, MaxLines: 10})

	if n := strings.Count(got.Text, "failure "); n > 10 {
		t.Errorf("Text carries %d error lines, want at most the budget:\n%s", n, got.Text)
	}
}

func TestTrimLogTakesALogWithoutTimestamps(t *testing.T) {
	t.Parallel()
	got := TrimLog("plain output\n##[error]it broke\n", LogOptions{})

	if !strings.Contains(got.Text, "plain output") {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestTrimLogTakesAnEmptyLog(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "\n\n\n", stampedLog("", "")} {
		if got := TrimLog(raw, LogOptions{}); got.Text != "" || got.Step != "" {
			t.Errorf("TrimLog(%q) = %+v, want an empty excerpt", raw, got)
		}
	}
}

func TestDropTimestampInALogLeavesALineThatOnlyLooksStamped(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"not-a-time here", "2026-13-45T99:99:99Z broken", "short"} {
		if got := dropTimestamp(s); got != s {
			t.Errorf("dropTimestamp(%q) = %q, want it unchanged", s, got)
		}
	}
}

func TestStripEscapesDropsWhatWouldRedrawATerminal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, in, want string }{
		{"colors", "\x1b[31mred\x1b[0m text", "red text"},
		{"cursor move", "a\x1b[2Kb", "ab"},
		{"window title", "\x1b]0;a title\x07done", "done"},
		{"window title with a string terminator", "\x1b]0;a title\x1b\\done", "done"},
		{"a carriage return", "progress\rdone", "progressdone"},
		{"a lone escape at the end", "text\x1b", "text"},
		{"an unterminated sequence", "text\x1b[31", "text"},
		{"the delete byte", "a\x7fb", "ab"},
		{"the bell", "a\x07b", "ab"},
		{"the line breaks and the tabs", "a\n\tb", "a\n\tb"},
		{"characters beyond ascii", "é ok", "é ok"},
	} {
		if got := stripEscapes(c.in); got != c.want {
			t.Errorf("stripEscapes(%q) = %q, want %q (%s)", c.in, got, c.want, c.name)
		}
	}
}

func TestTrimLogDropsACarriageReturnFromTheExcerpt(t *testing.T) {
	t.Parallel()
	got := TrimLog(stampedLog("downloading 10%\rdownloading 100%", "##[error]it broke"), LogOptions{})

	if strings.Contains(got.Text, "\r") {
		t.Errorf("Text keeps a carriage return: %q", got.Text)
	}
}
