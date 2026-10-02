package ghauth

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	gitHubHTTPS     = "https://github.com/"
	ownerFirstChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

type urlRule struct {
	base, key, prefix string
}

func (r urlRule) configKey() string {
	return "url." + r.base + "." + r.key
}

func gitEnv(token string) []string {
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	var pairs [][2]string
	for _, rule := range urlRules() {
		pairs = append(pairs, [2]string{rule.configKey(), rule.prefix})
	}
	pairs = append(pairs, [2]string{"http.https://github.com/.extraheader", "AUTHORIZATION: basic " + basic})
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(pairs))}
	for i, p := range pairs {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, p[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, p[1]))
	}
	return env
}

func urlRules() []urlRule {
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
	for _, rule := range urlRules() {
		fmt.Fprintf(&b, "[url %s]\n\t%s = %s\n", gitConfigQuote(rule.base), rule.key, rule.prefix)
	}
	if helper == "" {
		return b.String()
	}
	fmt.Fprintf(&b, "[credential %s]\n\thelper =\n", gitConfigQuote("https://github.com"))
	fmt.Fprintf(&b, "\thelper = %s\n", gitConfigQuote(helper))
	fmt.Fprintf(&b, "\thelper = %s\n", gitConfigQuote(ghHelper))
	return b.String()
}

const ghHelper = "!gh auth git-credential"

func gitConfigQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
