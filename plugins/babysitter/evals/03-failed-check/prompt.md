---
max_turns: 20
timeout_seconds: 420
allowed_tools: [Skill, Read, Write]
model: opus
runs: 3
---
I'm babysitting PR #134 with babysitter, provider self, checkout on the head branch. The check went red after my last push and `babysitter watch next w_5c02 --wait 9m -o json` handed me this:

```json
{
  "watch": { "id": "w_5c02", "status": "active", "pr": "deividfortuna/babysitter#134" },
  "message": {
    "kind": "check_failed",
    "payload": {
      "message": "PR DATA\ncheck_failed job=\"backend / test\" run_id=17442990311 conclusion=failure\nfull log: gh api repos/deividfortuna/babysitter/actions/jobs/49882211/logs\n\n--- excerpt ---\n# github.com/deividfortuna/babysitter/internal/prwatch\ninternal/prwatch/diff.go:88:19: undefined: kindNudge\nFAIL\tgithub.com/deividfortuna/babysitter/internal/prwatch [build failed]\nFAIL\tgithub.com/deividfortuna/babysitter/internal/daemon [build failed]\nFAIL\tgithub.com/deividfortuna/babysitter/internal/httpd [build failed]\nFAIL\tgithub.com/deividfortuna/babysitter/internal/session [build failed]\nFAIL\tgithub.com/deividfortuna/babysitter/internal/snapshot [build failed]\nok  \tgithub.com/deividfortuna/babysitter/internal/store\t0.418s\n--- test/integration ---\n    watch_test.go:210: context deadline exceeded after 30s waiting for the daemon socket\nFAIL\tgithub.com/deividfortuna/babysitter/test/integration\t30.114s\nmake: *** [Makefile:22: test] Error 2\n--- end excerpt ---\nPR DATA"
    }
  }
}
```

On the branch, `internal/prwatch/diff.go` line 88 reads:

```go
	case kindNudge:
		return a.Payload.Message, nil
```

and `internal/prwatch/activity.go` declares the kinds:

```go
const (
	kindComment  = "comment"
	kindNudged   = "nudged"
	kindReplied  = "replied"
)
```

Handle this item. Tell me what is actually broken, what you would change, how you would verify it, and the exact command you would use to report back on the pull request.
