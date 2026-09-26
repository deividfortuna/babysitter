package httpd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestActivityKindEnumMatchesTheStore(t *testing.T) {
	t.Parallel()
	field, ok := reflect.TypeFor[Activity]().FieldByName("Kind")
	if !ok {
		t.Fatal("Activity has no Kind field")
	}
	got := strings.Split(field.Tag.Get("enum"), ",")
	want := make([]string, 0, len(store.ActivityKinds))
	for _, kind := range store.ActivityKinds {
		want = append(want, string(kind))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("enum tag = %v,\nwant      %v", got, want)
	}
}

func TestAWatchTakenOverReadsWithItsTimeAndWorkBranch(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	w := store.Watch{ID: 1, Owner: "octo", Name: "hello", Number: 3, WorkBranch: "babysitter/fix", TakenOverAt: &at, TakenOverPID: 4242}

	raw, err := json.Marshal(watchFromStore(w, prwatch.SessionInfo{}, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["takenOverAt"] != "2026-09-26T12:00:00Z" || wire["workBranch"] != "babysitter/fix" {
		t.Fatalf("watch = %s", raw)
	}

	w.TakenOverAt = nil
	if raw, _ = json.Marshal(watchFromStore(w, prwatch.SessionInfo{}, nil, nil)); strings.Contains(string(raw), "takenOverAt") {
		t.Fatalf("a watch not taken over = %s", raw)
	}
}

func TestMergeMethodEnumsMatchTheClient(t *testing.T) {
	t.Parallel()
	fields := map[reflect.Type]string{
		reflect.TypeFor[Watch]():             "MergeMethod",
		reflect.TypeFor[StartWatchRequest](): "MergeMethod",
		reflect.TypeFor[MergeWatchRequest](): "Method",
		reflect.TypeFor[Settings]():          "MergeMethod",
	}
	want := "," + strings.Join(ghclient.MergeMethodsKnown, ",")
	for typ, name := range fields {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("%s has no %s field", typ, name)
		}
		if got := field.Tag.Get("enum"); got != want {
			t.Fatalf("%s.%s enum tag = %q, want %q", typ, name, got, want)
		}
	}
}

func TestAReplyTheDaemonDroppedReadsAsDropped(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	reply := store.ProposalReply{ID: 5, InReplyTo: 31, Body: "renamed it", Error: "no such review comment: 31", DroppedAt: &at}
	view := prwatch.ProposalView{Replies: []prwatch.ReplyView{{ProposalReply: reply}}}

	raw, err := json.Marshal(proposalFromView(view).Replies[0])
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["droppedAt"] != "2026-09-23T10:00:00Z" || wire["dropped"] != false {
		t.Fatalf("reply = %s", raw)
	}
}

func TestACommitTheAuthorKeptOffReadsAsHeldBack(t *testing.T) {
	t.Parallel()
	d := prwatch.ProposalDetail{
		Commits:  []gitrelease.Commit{{SHA: "e1197bc", Subject: "Strip punctuation in slug"}, {SHA: "472a1ca", Subject: "Guard null"}},
		HeldBack: []string{"e1197bc"},
	}
	got := proposalDetailFrom(d).Commits
	want := []ProposalCommit{{SHA: "e1197bc", Subject: "Strip punctuation in slug", HeldBack: true}, {SHA: "472a1ca", Subject: "Guard null"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commits = %+v, want %+v", got, want)
	}
}
