{{- with .Decision -}}
{{- if .Rejected -}}
The author rejected proposal {{.Proposal}} of {{.PR.Identity}}. Nothing was pushed or posted.
{{- if .Discarded}} The daemon reset your work branch to {{sanitize .Head}}: the commits of that turn are gone.
{{- else}} Your commits stay on your work branch.
{{- end}}
{{- if .Reason}}

What the author said:
{{sanitize .Reason}}
{{- end}}
{{- else -}}
The author approved proposal {{.Proposal}} of {{.PR.Identity}}.
{{- if .PushRejected}} They kept your commits off the pull request branch: the daemon pushed nothing, and your commits stay on your work branch.{{end}}
{{- range .Edited}}

The author changed your {{if .InReplyTo}}reply to comment {{.InReplyTo}}{{else}}comment on the pull request{{end}} before it went out. It now says:
{{sanitize .Text}}
{{- end}}
{{- range .Dropped}}

{{if .InReplyTo}}Your reply to comment {{.InReplyTo}} was dropped: it was not posted, and the comment comes back to you with a later message.{{else}}Your comment on the pull request was dropped: it was not posted.{{end}}
{{- end}}
{{- end}}
{{- if not .Reason}}

Nothing to do now. Change nothing and wait for the next message.
{{- end}}

PR: {{sanitize .PR.URL}}
{{- end}}
