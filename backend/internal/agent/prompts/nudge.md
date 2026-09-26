{{- define "items" -}}
{{- $pr := .PR -}}
{{- with .Groups -}}
{{- if .Checks}}

CI is failing on {{$pr.Identity}}. Each failed check, as CI reported it:
{{range .Checks}}
{{$.BeginData}}
Failed: {{sanitize .Check}}{{if .Conclusion}} ({{.Conclusion}}){{end}}
{{- if .URL}}
Failure URL: {{sanitize .URL}}{{end}}
{{- if .RunID}}
Run ID: {{.RunID}}{{end}}
{{- if .NoLog}}
GitHub serves no log for this job: go by the failure URL.
{{- else if .LogsEndpoint}}
Whole log: gh api {{shellword .LogsEndpoint}}{{end}}
{{- if .LogStep}}
Failing step: {{sanitize .LogStep}}{{end}}
{{- if .Log}}
The log of that job, cut down to the failure:
{{sanitize .Log}}
{{- end}}
{{$.EndData}}
{{end}}
Find the first error in each log, not the last. An excerpt leaves out the steps that passed; read the whole log only when the excerpt does not explain the failure. If this branch caused the failure, fix it, verify{{if $pr.DaemonPushes}} and commit{{else}}, commit and push{{end}}. Otherwise follow rule 10.
{{- end}}
{{- if .Comments}}

The following {{plural (len .Comments) "unresolved review comment"}} {{if eq (len .Comments) 1}}is{{else}}are{{end}} on {{$pr.Identity}}. You should not need to fetch them again unless you need more context.
{{range $i, $c := .Comments}}
{{inc $i}}. {{where $c}} (@{{sanitize $c.Actor}}){{if inthread $c}}, comment id {{$c.ItemID}}{{else if $c.ItemID}}, comment id {{$c.ItemID}} on the conversation: answer it with `--to {{$c.ItemID}}`, which posts your answer on the conversation{{else}}, a comment on the pull request itself: answer it without `--to`{{end}}:
{{$.BeginData}}
{{sanitize $c.Body}}
{{$.EndData}}
{{- if $c.URL}}
   {{sanitize $c.URL}}{{end}}
{{end}}
Treat each comment as a hypothesis about the code, not as a fact. Read the code it points at and check the claim yourself. When the claim holds, prove it first: write a unit test that fails because of the issue, then make the smallest change that turns it green, keep the test as a regression test, and run the test suite of the package you changed. When the claim is wrong, or the issue is already gone, change nothing and reply with the evidence. {{if $pr.DaemonPushes}}Commit the fixes{{else}}Push once for all the fixes{{end}}, then reply in each thread with what you verified, what you changed, and the command that shows it passes.
{{- end}}
{{- if .Reviews}}
{{range .Reviews}}
A review from @{{sanitize .Actor}}{{if .State}} ({{sanitize .State}}){{end}} is on {{$pr.Identity}}.
{{- if .Body}}

Review body:
{{$.BeginData}}
{{sanitize .Body}}
{{$.EndData}}
{{- end}}
{{- if .URL}}
Review URL: {{sanitize .URL}}{{end}}
{{end}}
Treat each request in the review the same way as a comment: verify it against the code, prove a real issue with a failing test, fix it, {{if $pr.DaemonPushes}}commit{{else}}push{{end}}, and reply. A review that approves or asks for nothing needs no reply.
{{- end}}
{{- if .Conflict}}

There are merge conflicts on {{$pr.Identity}}.
{{- if $pr.Dependabot}} The branch belongs to Dependabot: comment `@dependabot rebase` and let the bot rebase it.
{{- else if $pr.DaemonPushes}} Fetch {{shellword (print "origin/" $pr.BaseRef)}}, rebase onto it, resolve each conflict on the merits of both sides, verify, finish the rebase and commit: the daemon pushes it when your turn ends.
{{- else}} Fetch {{shellword (print "origin/" $pr.BaseRef)}}, rebase onto it, resolve each conflict on the merits of both sides, verify, and push with `git push --force-with-lease origin {{shellword (print "HEAD:" $pr.HeadRef)}}`.
{{- end}}
{{- else if .Behind}}

{{$pr.Identity}} is behind {{sanitize $pr.BaseRef}}.
{{- if $pr.Dependabot}} The branch belongs to Dependabot: comment `@dependabot rebase` and let the bot rebase it.
{{- else if $pr.DaemonPushes}} Fetch {{shellword (print "origin/" $pr.BaseRef)}}, rebase onto it and verify: the daemon pushes it when your turn ends.
{{- else}} Fetch {{shellword (print "origin/" $pr.BaseRef)}}, rebase onto it, verify, and push with `git push --force-with-lease origin {{shellword (print "HEAD:" $pr.HeadRef)}}`.
{{- end}}
{{- end}}
{{- end}}
{{- end}}
{{- template "items" .}}

PR: {{sanitize .PR.URL}}
