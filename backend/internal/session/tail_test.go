package session

import (
	"io"
	"strings"
	"testing"
)

type countingReaderAt struct {
	r    io.ReaderAt
	read int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.read += int64(n)
	return n, err
}

func TestTailAtAnswersTheSameAsLastLines(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 3*tailChunk)
	cases := []struct {
		name string
		in   string
		n    int
	}{
		{"the last lines", "one\ntwo\nthree\n", 2},
		{"more lines than the log has", "one\ntwo\n", 5},
		{"a log that ends without a line end", "one\ntwo\nthree", 2},
		{"every line", "one\ntwo\n", 0},
		{"an empty log", "", 3},
		{"one line longer than a chunk", long + "\n", 1},
		{"lines on both sides of a chunk", strings.Repeat("line\n", tailChunk), 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := tailAt(strings.NewReader(c.in), int64(len(c.in)), c.n)
			if err != nil {
				t.Fatalf("tailAt() error = %v", err)
			}
			if want := lastLines([]byte(c.in), c.n); got != want {
				t.Fatalf("tailAt() = %q, want %q", got, want)
			}
		})
	}
}

func TestTailAtReadsOnlyTheEndOfTheLog(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for b.Len() < 8<<20 {
		b.WriteString("the agent of a long watch printed this\n")
	}
	b.WriteString("the last line\n")
	log := b.String()

	c := &countingReaderAt{r: strings.NewReader(log)}
	got, err := tailAt(c, int64(len(log)), 2)
	if err != nil || !strings.HasSuffix(got, "the last line\n") {
		t.Fatalf("tailAt() = %q, %v", got, err)
	}
	if c.read > tailChunk {
		t.Fatalf("the end of a log of %d bytes took %d bytes to read", len(log), c.read)
	}
}

func TestTailCountsAnUnfinishedLastLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"the last line, finished", "one\ntwo\nthree\n", 1, "three\n"},
		{"the last line, unfinished", "one\ntwo\nthree", 1, "three"},
		{"two lines, the last unfinished", "one\ntwo\nthree", 2, "two\nthree"},
		{"every line, the last unfinished", "one\ntwo\nthree", 3, "one\ntwo\nthree"},
		{"more lines than an unfinished log has", "one\ntwo\nthree", 4, "one\ntwo\nthree"},
		{"one unfinished line", "three", 1, "three"},
		{"an empty last line", "one\n\n", 1, "\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := lastLines([]byte(c.in), c.n); got != c.want {
				t.Fatalf("lastLines(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
			}
			got, err := tailAt(strings.NewReader(c.in), int64(len(c.in)), c.n)
			if err != nil || got != c.want {
				t.Fatalf("tailAt(%q, %d) = %q, %v, want %q", c.in, c.n, got, err, c.want)
			}
		})
	}
}
