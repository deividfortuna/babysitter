package textx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCount(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{0: "0 replies", 1: "1 reply", 2: "2 replies"} {
		if got := Count(n, "reply", "replies"); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestJoinAnd(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		words []string
		want  string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
	} {
		if got := JoinAnd(tc.words); got != tc.want {
			t.Errorf("JoinAnd(%q) = %q, want %q", tc.words, got, tc.want)
		}
	}
}

func TestFirstLineCutsOnACharacter(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("a", 116) + strings.Repeat("é", 20)
	got := FirstLine(body, 120)
	if !utf8.ValidString(got) {
		t.Fatalf("FirstLine() = %q, which is not valid UTF-8", got)
	}
	if n := utf8.RuneCountInString(got); n != 120 {
		t.Fatalf("FirstLine() has %d characters, want 120", n)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("FirstLine() = %q, want it to end in an ellipsis", got)
	}
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"one line", "fix the test", 120, "fix the test"},
		{"the first line only", "fix the test\nand the build", 120, "fix the test"},
		{"a carriage return ends it too", "fix the test\r\nand the build", 120, "fix the test"},
		{"the blank lines in front are skipped", "\n\n  fix the test\n", 120, "fix the test"},
		{"empty", "   \n  \n", 120, ""},
		{"no cut without a limit", strings.Repeat("a", 300), 0, strings.Repeat("a", 300)},
		{"short enough stays whole", "héllo", 5, "héllo"},
		{"a limit shorter than the ellipsis cuts without one", "héllo", 2, "hé"},
		{"a limit as long as the ellipsis cuts without one", "héllo", 3, "hél"},
		{"one character more than the ellipsis leaves an ellipsis", "héllo", 4, "h..."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FirstLine(c.in, c.max); got != c.want {
				t.Errorf("FirstLine(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
			}
		})
	}
}
