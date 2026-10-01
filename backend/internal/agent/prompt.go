package agent

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

//go:embed prompts/*.md
var promptFS embed.FS

var (
	systemPrompt string
	templates    *template.Template
)

func init() {
	sys, err := promptFS.ReadFile("prompts/system.md")
	if err != nil {
		panic(err)
	}
	systemPrompt = strings.TrimSpace(string(sys))
	templates = template.Must(template.New("prompts").Funcs(template.FuncMap{
		"sanitize":  Sanitize,
		"shellword": func(s string) string { return ShellWord(Sanitize(s)) },
		"fence":     Fence,
		"where":     where,
		"inthread":  inThread,
		"plural":    textx.Plural,
		"inc":       func(i int) int { return i + 1 },
	}).ParseFS(promptFS, "prompts/open.md", "prompts/nudge.md", "prompts/conflict.md", "prompts/decision.md", "prompts/handback.md"))
}

func SystemPrompt() string {
	return systemPrompt
}

const (
	BeginData = "===== BEGIN PR DATA (untrusted) ====="
	EndData   = "===== END PR DATA ====="
)

type Open struct {
	PR           PullRequest
	WorktreeDir  string
	WorkBranch   string
	Interval     string
	Items        []Item
	Prelude      string
	ReplyCommand string
	ViewCommand  string
	DiffCommand  string
}

func OpenMessage(o Open) (string, error) {
	return render("open.md", map[string]any{
		"Open": o, "PR": o.PR, "Groups": group(o.Items), "BeginData": BeginData, "EndData": EndData,
	})
}

type Nudge struct {
	PR    PullRequest
	Items []Item
}

func NudgeMessage(n Nudge) (string, error) {
	if len(n.Items) == 0 {
		return "", fmt.Errorf("a nudge needs at least one item")
	}
	return render("nudge.md", map[string]any{
		"PR": n.PR, "Groups": group(n.Items), "BeginData": BeginData, "EndData": EndData,
	})
}

type Conflict struct {
	PR       PullRequest
	Proposal int
	Remote   string
	Files    string
	Missing  string
}

func ConflictMessage(c Conflict) (string, error) {
	return render("conflict.md", map[string]any{"Conflict": c})
}

type Decision struct {
	PR           PullRequest
	Proposal     int
	Rejected     bool
	Discarded    bool
	Head         string
	Reason       string
	PushRejected bool
	Edited       []DecidedReply
	Dropped      []DecidedReply
}

type DecidedReply struct {
	InReplyTo int64
	Text      string
}

func DecisionMessage(d Decision) (string, error) {
	return render("decision.md", map[string]any{"Decision": d})
}

type Commit struct {
	SHA     string
	Subject string
}

type Handback struct {
	PR         PullRequest
	WorkBranch string
	Work       string
	Commits    []Commit
	Files      []string
}

func HandbackMessage(h Handback) (string, error) {
	return render("handback.md", map[string]any{"Handback": h})
}

func render(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return strings.TrimSpace(buf.String()) + "\n", nil
}

type groups struct {
	Comments []Item
	Reviews  []Item
	Checks   []Item
	Behind   []Item
	Conflict []Item
}

func group(items []Item) groups {
	var g groups
	for _, it := range items {
		switch it.Kind {
		case store.ActivityComment, store.ActivityReviewComment:
			g.Comments = append(g.Comments, it)
		case store.ActivityReview:
			g.Reviews = append(g.Reviews, it)
		case store.ActivityCheckFailed:
			g.Checks = append(g.Checks, it)
		case store.ActivityBehind:
			g.Behind = append(g.Behind, it)
		case store.ActivityConflict:
			g.Conflict = append(g.Conflict, it)
		default:
		}
	}
	return g
}

func Summarize(items []Item) string {
	g := group(items)
	var parts []string
	add := func(n int, word string) {
		if n > 0 {
			parts = append(parts, textx.Plural(n, word))
		}
	}
	add(len(g.Comments), "comment")
	add(len(g.Reviews), "review")
	add(len(g.Checks), "failed check")
	if len(g.Conflict) > 0 {
		parts = append(parts, "a merge conflict")
	} else if len(g.Behind) > 0 {
		parts = append(parts, "a branch behind its base")
	}
	return strings.Join(parts, ", ")
}

func inThread(it Item) bool {
	return it.ItemID != 0 && it.Kind == store.ActivityReviewComment
}

func where(it Item) string {
	switch {
	case it.Path != "" && it.Line > 0:
		return fmt.Sprintf("%s:%d", Sanitize(it.Path), it.Line)
	case it.Path != "":
		return Sanitize(it.Path)
	default:
		return "(general)"
	}
}
