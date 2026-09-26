package agent

import "strings"

var dependabotLogins = map[string]bool{
	"dependabot[bot]":         true,
	"dependabot-preview[bot]": true,
	"dependabot":              true,
}

func IsDependabot(login string) bool {
	return dependabotLogins[strings.ToLower(strings.TrimSpace(login))]
}
