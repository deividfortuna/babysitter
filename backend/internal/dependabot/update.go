package dependabot

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Level string

const (
	Patch Level = "patch"
	Minor Level = "minor"
	Major Level = "major"
)

var Levels = []Level{Patch, Minor, Major}

func (l Level) Valid() bool { return slices.Contains(Levels, l) }

func (l Level) rank() int { return slices.Index(Levels, l) }

func Within(l, scope Level) bool {
	return l.Valid() && scope.Valid() && l.rank() <= scope.rank()
}

var (
	details     = regexp.MustCompile(`(?is)<details>.*?</details>`)
	versionPair = regexp.MustCompile(`(?i)\bfrom\s+((?:[~^<>=]+\s*)?\S+)\s+to\s+((?:[~^<>=]+\s*)?\S+)`)
	version     = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:[-+][\w.]+)?$`)
)

func UpdateType(title, body string) Level {
	text := title + "\n" + details.ReplaceAllString(body, "")
	pairs := versionPair.FindAllStringSubmatch(text, -1)
	if len(pairs) == 0 {
		return Major
	}
	highest := Patch
	for _, pair := range pairs {
		l := levelOf(pair[1], pair[2])
		if l.rank() > highest.rank() {
			highest = l
		}
	}
	return highest
}

func levelOf(from, to string) Level {
	a, okA := parts(from)
	b, okB := parts(to)
	switch {
	case !okA || !okB:
		return Major
	case a[0] != b[0]:
		return Major
	case a[1] != b[1]:
		return Minor
	default:
		return Patch
	}
}

func parts(s string) ([3]int, bool) {
	s = strings.TrimLeft(s, "~^<>= ")
	s = strings.Trim(s, "`.,;:()")
	m := version.FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range m[1:] {
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
