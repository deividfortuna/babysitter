{{- with .Handback -}}
The author gave the session of {{.PR.Identity}} back to you. While they had it, they worked in the worktree in their own terminal: in this conversation, or in a shell that this conversation did not see.

Your work branch {{sanitize .WorkBranch}} stands at {{sanitize .Work}}.
{{- if .Commits}} It has {{plural (len .Commits) "commit"}} that the pull request branch does not have:
{{range .Commits}}
{{sanitize .SHA}} {{sanitize .Subject}}
{{- end}}

The daemon pushes them with the work of your next turn.
{{- else}} It has no commits that the pull request branch does not have.
{{- end}}
{{- if .Files}}

The worktree has {{plural (len .Files) "change"}} that {{if eq (len .Files) 1}}is{{else}}are{{end}} not committed:
{{range .Files}}
{{sanitize .}}
{{- end}}
{{- end}}

Read what the author did before you go on: `git log`, `git show` and `git status` in the worktree. Then wait for the next message.

PR: {{sanitize .PR.URL}}
{{- end}}
