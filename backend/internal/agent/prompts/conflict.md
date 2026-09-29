{{- with .Conflict -}}
{{- if .Missing -}}
The daemon could not push your work on {{.PR.Identity}}: origin/{{sanitize .PR.HeadRef}} moved to {{sanitize .Remote}}, and it has commits your work lacks: {{sanitize .Missing}}. Your work is not only new commits on top of the branch: it rewrote the branch, merged another branch into it, or the branch was rewritten under it. So the daemon did not rebase it, and a push would drop those commits.

Run `git fetch origin {{shellword .PR.HeadRef}}`, bring those commits into your work without dropping any of yours, verify and commit. The daemon pushes it when your turn ends.
{{- else if .PR.MergesBase -}}
The daemon could not push your work on {{.PR.Identity}}: origin/{{sanitize .PR.HeadRef}} moved to {{sanitize .Remote}}, and it does not merge into your work without conflicts{{if .Files}} in {{sanitize .Files}}{{end}}. The daemon aborted its merge, so your branch is as you left it.

Run `git fetch origin {{shellword .PR.HeadRef}}` and `git merge {{shellword (print "origin/" .PR.HeadRef)}}`, resolve each conflict on the merits of both sides, verify, and commit the merge. Do not rebase. The daemon pushes it when your turn ends.
{{- else -}}
The daemon could not push your work on {{.PR.Identity}}: origin/{{sanitize .PR.HeadRef}} moved to {{sanitize .Remote}}, and your commits do not rebase onto it without conflicts{{if .Files}} in {{sanitize .Files}}{{end}}. The daemon aborted its rebase, so your branch is as you left it.

Run `git fetch origin {{shellword .PR.HeadRef}}` and `git rebase {{shellword (print "origin/" .PR.HeadRef)}}`, resolve each conflict on the merits of both sides, verify, finish the rebase and commit. The daemon pushes it when your turn ends.
{{- end}}

The replies of proposal {{.Proposal}} did not go out. They go out with the work of your next turn, as you recorded them. When a reply names a commit that your rebase replaces, record it again with the same --to and the new commit: the new reply takes the place of the old one.

PR: {{sanitize .PR.URL}}
{{- end}}
