---
type: llm
focus: { source: file, path: helper_test.go }
weight: 1
---
Judge the substance of the test, not its style. Pass if both hold:

- The file is Go test code that calls `summarize` (directly, or through a helper in the same file) with a `Snapshot` whose `HeadSHA` field is nil.
- Run against the implementation in the prompt, which dereferences `*snap.HeadSHA` with no nil check, that nil case does not pass: it panics, or it fails an assertion because the current code cannot produce the expected value.

Do not fail the grader for any of these, which are all acceptable:

- a table test that covers the non-nil case alongside the nil case
- a helper that wraps `summarize` in `recover` so the panic is reported as a test failure instead of taking the binary down
- extra helper functions, fixtures or subtests in the file
- any choice of expected output for the nil case, such as an empty string, a placeholder, or an error

Fail the grader only if no nil `HeadSHA` case is exercised at all, or if the file redefines `summarize` with the nil guard already applied, so that nothing in it would fail against the branch as it stands.
