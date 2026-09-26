package copilot

import "github.com/deividfortuna/babysitter/internal/agent"

func authorRules(l agent.Launch) []string {
	var out []string
	for _, command := range append(agent.AuthorCommands(l), agent.AuthorPatterns()...) {
		out = append(out, "shell("+command+":*)")
	}
	return out
}

var denied = []string{
	"shell(git push --force:*)",
	"shell(git push * --force:*)",
	"shell(git push -f:*)",
	"shell(git push * -f:*)",
	"shell(git push --no-verify:*)",
	"shell(git push * --no-verify:*)",
	"shell(git -c core.hooksPath:*)",
	"shell(git --exec-path:*)",
	"shell(git push --mirror:*)",
	"shell(git push --delete:*)",
	"shell(git branch -D:*)",
	"shell(git worktree:*)",
	"shell(git config --global:*)",
	"shell(gh pr merge:*)",
	"shell(gh pr close:*)",
	"shell(gh pr create:*)",
	"shell(gh pr ready:*)",
	"shell(gh pr comment:*)",
	"shell(gh pr review:*)",
	"shell(gh pr edit:*)",
	"shell(gh pr reopen:*)",
	"shell(gh issue:*)",
	"shell(gh api --method:*)",
	"shell(gh api -X:*)",
	"shell(gh api * --method:*)",
	"shell(gh api * -X:*)",
	"shell(gh api -f:*)",
	"shell(gh api -F:*)",
	"shell(gh api --field:*)",
	"shell(gh api --raw-field:*)",
	"shell(gh api --input:*)",
	"shell(gh api * -f:*)",
	"shell(gh api * -F:*)",
	"shell(gh api * --field:*)",
	"shell(gh api * --raw-field:*)",
	"shell(gh api * --input:*)",
	"shell(gh workflow run:*)",
	"shell(gh repo:*)",
	"shell(gh release:*)",
	"shell(gh secret:*)",
	"shell(gh auth:*)",
	"shell(gh run cancel:*)",
	"shell(gh run delete:*)",
	"shell(curl:*)",
	"shell(wget:*)",
	"shell(sudo:*)",
	"shell(rm -rf:*)",
	"url",
}
