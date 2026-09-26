package redact

import "regexp"

const mark = "[redacted]"

var secretPatterns = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`), mark},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`), mark},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), mark},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{16,}`), mark},
	{regexp.MustCompile(`(?i)\b((?:[\w-]*[_-])?(?:api[_-]?key|secret[_-]?access[_-]?key|private[_-]?key|secret|token|password|key)"?\s*[:=]\s*['"]?)[A-Za-z0-9._~+/=-]{12,}`), "${1}" + mark},
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), mark},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`), mark},
}

func Text(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.with)
	}
	return s
}

func Err(err error) error {
	if err == nil {
		return nil
	}
	msg := Text(err.Error())
	if msg == err.Error() {
		return err
	}
	return redacted{err: err, msg: msg}
}

type redacted struct {
	err error
	msg string
}

func (r redacted) Error() string { return r.msg }
func (r redacted) Unwrap() error { return r.err }
