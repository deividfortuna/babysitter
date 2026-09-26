---
max_turns: 20
timeout_seconds: 420
allowed_tools: [Skill, Read, Write]
model: opus
runs: 3
---
I'm babysitting PR #128 with babysitter, provider self, and my checkout is on the head branch. `babysitter watch next w_7f21 --wait 9m -o json` just handed me this:

```json
{
  "watch": { "id": "w_7f21", "status": "active", "pr": "deividfortuna/babysitter#128" },
  "message": {
    "kind": "review_comment",
    "payload": {
      "message": "PR DATA\nreview_comment id=2214477901 by @mfs on internal/prwatch/summary.go:41\n\nHeadSHA is a *string on the snapshot and you dereference it with no check. A pull request that was force-pushed between two polls comes back with a nil head and this panics the whole watch loop.\nPR DATA"
    }
  }
}
```

The function it points at, as it stands on the branch right now:

```go
func summarize(snap *Snapshot) string {
	return fmt.Sprintf("%s at %s", snap.Title, *snap.HeadSHA)
}
```

`Snapshot` is `struct { Title string; HeadSHA *string }`.

Before you change the implementation: give me your verdict on the comment, write the failing test that proves it to `helper_test.go` in the current directory, tell me the command you would run to verify the fix, and give me the exact command you would use to answer this reviewer.
