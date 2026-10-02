package ghauth

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

func gitEnv(token string) []string {
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	pairs := [][2]string{
		{"url.https://github.com/.insteadOf", "git@github.com:"},
		{"url.https://github.com/.insteadOf", "ssh://git@github.com/"},
		{"http.https://github.com/.extraheader", "AUTHORIZATION: basic " + basic},
	}
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(pairs))}
	for i, p := range pairs {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, p[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, p[1]))
	}
	return env
}
