package session

import (
	"errors"
	"io"
	"os"
	"strings"
)

const tailChunk = 64 << 10

func Tail(path string, n int) (string, error) {
	out, err := tailOne(path, n)
	if err != nil {
		return "", err
	}
	if n <= 0 || strings.Count(out, "\n") > n {
		return out, nil
	}
	older, err := tailOne(path+RotatedSuffix, n)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return "", err
	}
	return lastLines([]byte(older+out), n), nil
}

func tailOne(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	return tailAt(f, info.Size(), n)
}

func tailAt(r io.ReaderAt, size int64, n int) (string, error) {
	if n <= 0 {
		return readAt(r, 0, size)
	}
	out, lines, end := "", 0, size
	for end > 0 {
		start := max(end-tailChunk, 0)
		chunk, err := readAt(r, start, end-start)
		if err != nil {
			return "", err
		}
		out, lines, end = chunk+out, lines+strings.Count(chunk, "\n"), start
		if lines > n {
			break
		}
	}
	return lastLines([]byte(out), n), nil
}

func readAt(r io.ReaderAt, off, n int64) (string, error) {
	buf := make([]byte, n)
	_, err := r.ReadAt(buf, off)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
