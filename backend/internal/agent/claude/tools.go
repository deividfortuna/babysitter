package claude

import "github.com/deividfortuna/babysitter/internal/agent"

func authorRules(l agent.Launch) []string {
	var out []string
	for _, command := range agent.AuthorCommands(l) {
		out = append(out, "Bash("+command+":*)", "Bash("+command+" *)")
	}
	for _, pattern := range agent.AuthorPatterns() {
		out = append(out, "Bash("+pattern+"*)")
	}
	return out
}

var allowedTools = []string{
	"Read", "Edit", "Write", "MultiEdit", "Grep", "Glob", "LS", "Bash",
}

var deniedTools = []string{
	"Bash(git push --force)", "Bash(git push --force *)", "Bash(git push * --force)", "Bash(git push * --force *)",
	"Bash(git push -f)", "Bash(git push -f *)", "Bash(git push * -f)", "Bash(git push * -f *)",
	"Bash(git push --no-verify:*)", "Bash(git push --no-verify *)", "Bash(git push * --no-verify)", "Bash(git push * --no-verify *)",
	"Bash(git -c core.hooksPath:*)", "Bash(git -c core.hooksPath*)", "Bash(git --exec-path:*)", "Bash(git --exec-path*)",
	"Bash(git push --mirror:*)", "Bash(git push --mirror *)",
	"Bash(git push --delete:*)", "Bash(git push --delete *)",
	"Bash(git branch -D:*)", "Bash(git branch -D *)",
	"Bash(git worktree:*)", "Bash(git worktree *)",
	"Bash(git config --global:*)", "Bash(git config --global *)",
	"Bash(gh pr merge:*)", "Bash(gh pr merge *)",
	"Bash(gh pr close:*)", "Bash(gh pr close *)",
	"Bash(gh pr create:*)", "Bash(gh pr create *)",
	"Bash(gh pr ready:*)", "Bash(gh pr ready *)",
	"Bash(gh pr comment:*)", "Bash(gh pr comment *)",
	"Bash(gh pr review:*)", "Bash(gh pr review *)",
	"Bash(gh pr edit:*)", "Bash(gh pr edit *)",
	"Bash(gh pr reopen:*)", "Bash(gh pr reopen *)",
	"Bash(gh issue:*)", "Bash(gh issue *)",
	"Bash(gh api --method:*)", "Bash(gh api --method *)", "Bash(gh api -X:*)", "Bash(gh api -X *)",
	"Bash(gh api * --method*)", "Bash(gh api * -X *)",
	"Bash(gh api -f:*)", "Bash(gh api -F:*)", "Bash(gh api --field:*)", "Bash(gh api --raw-field:*)", "Bash(gh api --input:*)",
	"Bash(gh api * -f *)", "Bash(gh api * -F *)", "Bash(gh api * --field *)", "Bash(gh api * --raw-field *)", "Bash(gh api * --input *)",
	"Bash(gh workflow run:*)", "Bash(gh workflow run *)",
	"Bash(gh repo:*)", "Bash(gh repo *)",
	"Bash(gh release:*)", "Bash(gh release *)",
	"Bash(gh secret:*)", "Bash(gh secret *)",
	"Bash(gh auth:*)", "Bash(gh auth *)",
	"Bash(gh run cancel:*)", "Bash(gh run cancel *)",
	"Bash(gh run delete:*)", "Bash(gh run delete *)",
	"Bash(curl:*)", "Bash(curl *)",
	"Bash(wget:*)", "Bash(wget *)",
	"Bash(sudo:*)", "Bash(sudo *)",
	"Bash(rm -rf:*)", "Bash(rm -rf *)",
	"WebFetch", "WebSearch", "Task", "Agent",
}
