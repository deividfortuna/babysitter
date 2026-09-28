package logbook

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

func open(t *testing.T, o Options) *Book {
	t.Helper()
	b, err := Open(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func messages(records []Record) []string {
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.Msg
	}
	return out
}

func TestRecordsKeepTheAttributesOfTheLoggerAndTheCall(t *testing.T) {
	b := open(t, Options{})
	log := slog.New(b.Handler()).With("component", "prwatch").WithGroup("watch").With("id", 7)

	log.Warn("poll failed", "err", errors.New("timeout"), slog.Group("pr", "number", 3))

	got := b.Since(0, 0)
	if len(got) != 1 {
		t.Fatalf("records = %v, want one", got)
	}
	want := []Attr{{"component", "prwatch"}, {"watch.id", "7"}, {"watch.err", "timeout"}, {"watch.pr.number", "3"}}
	if got[0].Seq != 1 || got[0].Level != "warn" || got[0].Msg != "poll failed" || !slices.Equal(got[0].Attrs, want) {
		t.Fatalf("record = %+v, want warn poll failed with %v", got[0], want)
	}
}

func TestTheLevelDropsLowerRecordsAndChangesWhileTheBookRuns(t *testing.T) {
	b := open(t, Options{Level: slog.LevelInfo})
	log := slog.New(b.Handler())

	log.Debug("hidden")
	b.SetLevel(slog.LevelDebug)
	log.Debug("shown")

	if got := messages(b.Since(0, 0)); !slices.Equal(got, []string{"shown"}) {
		t.Fatalf("records = %v, want only the debug record after the change", got)
	}
}

func TestSinceAnswersTheNewerRecordsUpToTheLimit(t *testing.T) {
	b := open(t, Options{Keep: 3})
	log := slog.New(b.Handler())
	for _, msg := range []string{"a", "b", "c", "d", "e"} {
		log.Info(msg)
	}

	if got := messages(b.Since(0, 0)); !slices.Equal(got, []string{"c", "d", "e"}) {
		t.Fatalf("kept = %v, want the last three", got)
	}
	if got := messages(b.Since(3, 0)); !slices.Equal(got, []string{"d", "e"}) {
		t.Fatalf("after 3 = %v, want d and e", got)
	}
	if got := messages(b.Since(0, 1)); !slices.Equal(got, []string{"e"}) {
		t.Fatalf("limit 1 = %v, want the last one", got)
	}
}

func TestSubscribersSeeEachNewRecord(t *testing.T) {
	b := open(t, Options{})
	var seen []int64
	unsubscribe := b.Subscribe(func(r Record) { seen = append(seen, r.Seq) })
	log := slog.New(b.Handler())

	log.Info("one")
	unsubscribe()
	log.Info("two")

	if !slices.Equal(seen, []int64{1}) {
		t.Fatalf("seen = %v, want only the record before the unsubscribe", seen)
	}
}

func TestSubscribersSeeTheRecordsInOrderWhenManyGoroutinesLog(t *testing.T) {
	b := open(t, Options{})
	var seen []int64
	b.Subscribe(func(r Record) { seen = append(seen, r.Seq) })
	log := slog.New(b.Handler())

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 500 {
				log.Info("busy")
			}
		})
	}
	wg.Wait()

	for i, seq := range seen {
		if seq != int64(i+1) {
			t.Fatalf("record %d reached the subscriber as seq %d, want %d", i, seq, i+1)
		}
	}
}

func TestASubscriberCanUnsubscribeFromItsCallbackWhileOthersLog(t *testing.T) {
	b, err := Open(Options{})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(b.Handler())
	for range 50 {
		var once sync.Once
		var unsubscribe func()
		unsubscribe = b.Subscribe(func(Record) { once.Do(func() { unsubscribe() }) })
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 200 {
					log.Info("busy")
				}
			})
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("logging stopped: a subscriber that unsubscribed from its callback deadlocked the book")
	}
}

func TestTheFileKeepsTheRecordsAndRotatesPastTheSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "daemon.log")
	b := open(t, Options{Path: path, MaxBytes: 200})
	log := slog.New(b.Handler())
	for _, msg := range []string{"first", "second", "third", "fourth"} {
		log.Info(msg, "padding", "0123456789012345678901234567890123456789")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + BackupSuffix); err != nil {
		t.Fatalf("backup file: %v, want the rotated file", err)
	}
	got, err := ReadTail(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if msgs := messages(got); len(msgs) == 0 || msgs[len(msgs)-1] != "fourth" {
		t.Fatalf("tail = %v, want it to end with the last record", msgs)
	}
	last, err := ReadTail(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if msgs := messages(last); !slices.Equal(msgs, []string{"fourth"}) {
		t.Fatalf("tail 1 = %v, want the last record", msgs)
	}
}

func TestASecondRotationReplacesTheBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	b := open(t, Options{Path: path, MaxBytes: 200})
	log := slog.New(b.Handler())
	for _, msg := range []string{"first", "second", "third", "fourth"} {
		log.Info(msg, "padding", "0123456789012345678901234567890123456789")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	backup, err := readFile(path + BackupSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if got := messages(backup); !slices.Equal(got, []string{"third", "fourth"}) {
		t.Fatalf("backup = %v, want the records of the second rotation only", got)
	}
}

func TestReadTailOfNoFileIsEmpty(t *testing.T) {
	got, err := ReadTail(filepath.Join(t.TempDir(), "daemon.log"), 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("tail = %v, %v, want nothing and no error", got, err)
	}
}

func TestParseLevelTakesTheFourNames(t *testing.T) {
	for _, name := range LevelNames {
		l, err := ParseLevel(name)
		if err != nil || LevelName(l) != name {
			t.Fatalf("ParseLevel(%q) = %v, %v", name, l, err)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel(verbose) = nil error, want a refusal")
	}
}
