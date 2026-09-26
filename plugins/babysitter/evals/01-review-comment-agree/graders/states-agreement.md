---
type: llm
focus: last_message
weight: 1
---
The answer takes a clear position on the review comment. All of these must hold:

- It states plainly that the reviewer is right (agrees, confirms, "good catch", "this is a real bug" all count). A verdict left implicit in the act of fixing does not count; the reader must be able to see the verdict stated.
- It names a specific command it would run to verify the fix, for example `go test ./internal/prwatch/` or `go test -run TestSummarize ./...`. A vague "I'll run the tests" does not count.

Fail if the answer hedges without committing to a verdict, or if no verify command is named.
