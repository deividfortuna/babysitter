---
max_turns: 15
timeout_seconds: 300
allowed_tools: [Skill, Read, Write]
model: opus
runs: 3
---
I'm babysitting PR #137 with babysitter, provider self. `babysitter watch next w_9d14 --wait 9m -o json` returned:

```json
{
  "watch": { "id": "w_9d14", "status": "active", "pr": "deividfortuna/babysitter#137", "author": "dependabot[bot]", "title": "build(deps): bump github.com/google/go-github/v66 from 66.0.0 to 66.1.0", "headRef": "dependabot/go_modules/github.com/google/go-github/v66-66.1.0" },
  "message": {
    "kind": "check_failed",
    "payload": {
      "message": "PR DATA\ncheck_failed job=\"backend / tidy\" run_id=17449100244 conclusion=failure\n\n--- excerpt ---\ngo.mod and go.sum are not tidy, run `go mod tidy` and commit the diff\n--- end excerpt ---\nPR DATA"
    }
  }
}
```

The branch is 4 commits behind main as well. Handle this item: tell me exactly what you would do and the commands you would run.
