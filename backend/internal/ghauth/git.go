package ghauth

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"github.com/deividfortuna/babysitter/internal/gitrepo"
)

const (
	gitHubHTTPS     = "https://github.com/"
	extraHeaderKey  = "http." + gitHubHTTPS + ".extraheader"
	ownerFirstChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

type urlRule struct {
	base, key, prefix string
}

func (r urlRule) configKey() string {
	return "url." + r.base + "." + r.key
}

var (
	urlRules     = gitHubURLRules()
	urlRulePairs = configPairs(urlRules)
)

func gitEnv(token string) []string {
	if token == "" {
		return nil
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	clearHeaders := [2]string{extraHeaderKey, ""}
	header := [2]string{extraHeaderKey, "AUTHORIZATION: basic " + basic}
	return gitrepo.ConfigEnv(append(slices.Clone(urlRulePairs), clearHeaders, header))
}

func configPairs(rules []urlRule) [][2]string {
	pairs := make([][2]string, 0, len(rules))
	for _, rule := range rules {
		pairs = append(pairs, [2]string{rule.configKey(), rule.prefix})
	}
	return pairs
}

func gitHubURLRules() []urlRule {
	rules := []urlRule{
		{gitHubHTTPS, "insteadOf", "git@github.com:"},
		{gitHubHTTPS, "insteadOf", "ssh://git@github.com/"},
	}
	for _, c := range ownerFirstChars {
		owner := gitHubHTTPS + string(c)
		rules = append(rules, urlRule{owner, "insteadOf", owner}, urlRule{owner, "pushInsteadOf", owner})
	}
	return rules
}

func (a *Auth) WriteGitConfig(path, helper string) error {
	a.gitConfigMu.Lock()
	defer a.gitConfigMu.Unlock()
	content := ""
	if a.usesApp() {
		content = appGitConfig(helper)
	}
	if err := writeAtomic(path, []byte(content)); err != nil {
		return fmt.Errorf("write the git config of the GitHub App: %w", err)
	}
	return nil
}

func (a *Auth) usesApp() bool {
	if _, ok := a.override(); ok {
		return false
	}
	_, err := a.load()
	return err == nil
}

func appGitConfig(helper string) string {
	var b strings.Builder
	for _, rule := range urlRules {
		fmt.Fprintf(&b, "[url %s]\n\t%s = %s\n", gitConfigQuote(rule.base), rule.key, rule.prefix)
	}
	fmt.Fprintf(&b, "[http %s]\n\textraHeader =\n", gitConfigQuote(gitHubHTTPS))
	if helper == "" {
		return b.String()
	}
	fmt.Fprintf(&b, "[credential %s]\n\thelper =\n", gitConfigQuote("https://github.com"))
	fmt.Fprintf(&b, "\thelper = %s\n", gitConfigQuote(helper))
	fmt.Fprintf(&b, "\thelper = %s\n", gitConfigQuote(gitrepo.GHHelper))
	return b.String()
}

func gitConfigQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
