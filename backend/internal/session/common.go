package session

import (
	"bytes"
	"context"
	"errors"
	"time"
)

const RotatedSuffix = ".1"

type Spec struct {
	Dir        string
	Argv       []string
	Env        []string
	LogPath    string
	Rows, Cols uint16
}

type Handle interface {
	Send(ctx context.Context, text string) error
	Ready(ctx context.Context) error
	Interrupt() error
	Output(n int) string
	Done() <-chan struct{}
	Err() error
	PID() int
	Stop(ctx context.Context) error
}

type Host interface {
	Start(ctx context.Context, spec Spec) (Handle, error)
}

type Timing struct {
	ChunkRunes  int
	ChunkDelay  time.Duration
	EnterDelay  time.Duration
	ReadyWait   time.Duration
	ReadySettle time.Duration
}

var DefaultTiming = Timing{
	ChunkRunes: 512, ChunkDelay: 15 * time.Millisecond, EnterDelay: 300 * time.Millisecond,
	ReadyWait: 20 * time.Second, ReadySettle: 750 * time.Millisecond,
}

var ErrExited = errors.New("the agent process exited")

func chunks(text string, n int) []string {
	if n <= 0 {
		return []string{text}
	}
	var out []string
	runes := []rune(text)
	for len(runes) > n {
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return append(out, string(runes))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func lastLines(b []byte, n int) string {
	if n <= 0 {
		return string(b)
	}
	end := len(b)
	if end > 0 && b[end-1] != '\n' {
		n--
	}
	for i := end - 1; i >= 0; i-- {
		if b[i] == '\n' {
			n--
			if n < 0 {
				return string(b[i+1 : end])
			}
		}
	}
	return string(b)
}

func Strip(s string) string {
	b := []byte(s)
	var out bytes.Buffer
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c != 0x1b {
			if c == '\r' || c >= 0x20 || c == '\n' || c == '\t' {
				out.WriteByte(c)
			}
			continue
		}
		i++
		if i >= len(b) {
			break
		}
		switch b[i] {
		case '[':
			for i++; i < len(b); i++ {
				if b[i] >= 0x40 && b[i] <= 0x7e {
					break
				}
			}
		case ']':
			for i++; i < len(b); i++ {
				if b[i] == 0x07 {
					break
				}
				if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
					i++
					break
				}
			}
		}
	}
	return out.String()
}
