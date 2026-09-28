package logbook

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultKeep     = 2000
	DefaultMaxBytes = 5 << 20
	BackupSuffix    = ".1"
)

type Attr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Record struct {
	Seq   int64     `json:"seq"`
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
	Attrs []Attr    `json:"attrs"`
}

type Options struct {
	Path     string
	Keep     int
	MaxBytes int64
	Level    slog.Level
}

type Book struct {
	level    slog.LevelVar
	keep     int
	maxBytes int64
	path     string

	mu      sync.Mutex
	seq     int64
	records []Record
	file    *os.File
	size    int64

	subsMu sync.Mutex
	next   int
	subs   map[int]func(Record)

	delivery sync.Mutex
}

func Open(o Options) (*Book, error) {
	b := &Book{
		keep:     o.Keep,
		maxBytes: o.MaxBytes,
		path:     o.Path,
		subs:     make(map[int]func(Record)),
	}
	if b.keep <= 0 {
		b.keep = DefaultKeep
	}
	if b.maxBytes <= 0 {
		b.maxBytes = DefaultMaxBytes
	}
	b.level.Set(o.Level)
	if b.path == "" {
		return b, nil
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o750); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	if err := b.openFile(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Book) Path() string { return b.path }

func (b *Book) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.file == nil {
		return nil
	}
	err := b.file.Close()
	b.file = nil
	return err
}

func (b *Book) Leveler() slog.Leveler { return &b.level }

func (b *Book) Level() slog.Level { return b.level.Level() }

func (b *Book) SetLevel(l slog.Level) { b.level.Set(l) }

func (b *Book) Handler() slog.Handler { return &handler{book: b} }

func (b *Book) Since(after int64, limit int) []Record {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.kept()
	start := len(kept)
	for start > 0 && kept[start-1].Seq > after {
		start--
	}
	newer := kept[start:]
	if limit > 0 && len(newer) > limit {
		newer = newer[len(newer)-limit:]
	}
	out := make([]Record, len(newer))
	copy(out, newer)
	return out
}

func (b *Book) Subscribe(fn func(Record)) func() {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	id := b.next
	b.next++
	b.subs[id] = fn
	return func() {
		b.subsMu.Lock()
		defer b.subsMu.Unlock()
		delete(b.subs, id)
	}
}

func (b *Book) subscribers() []func(Record) {
	b.subsMu.Lock()
	defer b.subsMu.Unlock()
	subs := make([]func(Record), 0, len(b.subs))
	for _, fn := range b.subs {
		subs = append(subs, fn)
	}
	return subs
}

func (b *Book) add(r Record) {
	b.mu.Lock()
	b.seq++
	r.Seq = b.seq
	b.records = append(b.records, r)
	if len(b.records) >= 2*b.keep {
		b.records = append([]Record(nil), b.kept()...)
	}
	b.write(r)
	subs := b.subscribers()
	b.delivery.Lock()
	b.mu.Unlock()
	defer b.delivery.Unlock()
	for _, fn := range subs {
		fn(r)
	}
}

func (b *Book) kept() []Record {
	return b.records[max(0, len(b.records)-b.keep):]
}

func (b *Book) write(r Record) {
	if b.file == nil {
		return
	}
	line, err := json.Marshal(r)
	if err != nil {
		return
	}
	n, _ := b.file.Write(append(line, '\n'))
	b.size += int64(n)
	if b.size >= b.maxBytes {
		b.rotate()
	}
}

func (b *Book) rotate() {
	_ = b.file.Close()
	b.file = nil
	backup := b.path + BackupSuffix
	_ = os.Remove(backup)
	moved := os.Rename(b.path, backup) == nil
	if b.openFile() != nil {
		return
	}
	if !moved {
		b.size = 0
	}
}

func (b *Book) openFile() error {
	f, err := os.OpenFile(b.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("open log file: %w", err)
	}
	b.file = f
	b.size = info.Size()
	return nil
}

var levelNames = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

var LevelNames = []string{"debug", "info", "warn", "error"}

func ParseLevel(name string) (slog.Level, error) {
	if l, ok := levelNames[strings.ToLower(name)]; ok {
		return l, nil
	}
	return 0, fmt.Errorf("unknown log level %q: use %s", name, strings.Join(LevelNames, ", "))
}

func LevelName(l slog.Level) string {
	return strings.ToLower(l.String())
}
