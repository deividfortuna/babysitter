package snapshot

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/deividfortuna/babysitter/internal/store"
)

type Target struct {
	Owner  string
	Name   string
	Number int
}

var ErrIncompleteTarget = errors.New("without a checkout, the target must name the repository and the number")

func (t Target) Complete() bool {
	return t.Owner != "" && t.Name != "" && t.Number > 0
}

var numberRE = regexp.MustCompile(`^\d+$`)

func ParseTarget(arg, repoFlag string) (Target, error) {
	var t Target
	if repoFlag != "" {
		var err error
		if t.Owner, t.Name, err = store.ParseFullName(repoFlag); err != nil {
			return Target{}, err
		}
	}
	arg = strings.TrimSpace(arg)
	switch {
	case arg == "":
		return t, nil
	case numberRE.MatchString(arg):
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 {
			return Target{}, fmt.Errorf("invalid pull request number %q", arg)
		}
		t.Number = n
		return t, nil
	case strings.Contains(arg, "://"):
		return parseURL(arg)
	}
	repo, num, ok := strings.Cut(arg, "#")
	if !ok {
		return Target{}, fmt.Errorf("invalid pull request %q, want a number, owner/name#number or a URL", arg)
	}
	owner, name, err := store.ParseFullName(repo)
	if err != nil {
		return Target{}, err
	}
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 {
		return Target{}, fmt.Errorf("invalid pull request number %q in %q", num, arg)
	}
	return Target{Owner: owner, Name: name, Number: n}, nil
}

func parseURL(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("invalid pull request URL %q: %w", raw, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	namesRepo := len(parts) >= 4 && parts[0] != "" && parts[1] != ""
	if !namesRepo || parts[2] != "pull" {
		return Target{}, fmt.Errorf("invalid pull request URL %q, want https://github.com/owner/name/pull/number", raw)
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return Target{}, fmt.Errorf("invalid pull request number %q in %q", parts[3], raw)
	}
	return Target{Owner: parts[0], Name: parts[1], Number: n}, nil
}

func (t Target) Repo() string {
	return t.Owner + "/" + t.Name
}
