package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
)

func newAuthTokenCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:    "token",
		Short:  "Print the GitHub App token of the daemon",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := opts.daemonAppToken(cmd.Context())
			if err != nil || token == "" {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), token)
			return err
		},
	}
}

func newAuthGitCredentialCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:    "git-credential <operation>",
		Short:  "Give git the GitHub App token of the daemon",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "get" {
				return nil
			}
			if !readCredentialRequest(cmd.InOrStdin()).forGitHub() {
				return nil
			}
			token, err := opts.daemonAppToken(cmd.Context())
			if err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "quit=1")
				return err
			}
			if token == "" {
				return nil
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "username=x-access-token\npassword=%s\n", token)
			return err
		},
	}
}

type credentialRequest map[string]string

func readCredentialRequest(r io.Reader) credentialRequest {
	req := credentialRequest{}
	lines := bufio.NewScanner(r)
	for lines.Scan() {
		key, value, found := strings.Cut(lines.Text(), "=")
		if !found {
			break
		}
		req[key] = value
	}
	return req
}

func (r credentialRequest) forGitHub() bool {
	host := strings.TrimSuffix(strings.TrimSuffix(r["host"], ":443"), ".")
	return strings.EqualFold(r["protocol"], "https") && strings.EqualFold(host, "github.com")
}

func (o *options) daemonAppToken(ctx context.Context) (string, error) {
	c, err := o.daemonClient(o.authDir)
	if errors.Is(err, errNoDaemon) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var out httpd.AppToken
	err = c.get(ctx, "/auth/token", &out)
	if appNotInUse(err) {
		return "", nil
	}
	return out.Token, err
}

func appNotInUse(err error) bool {
	code := errorCode(err)
	return code == "app_not_in_use" || code == "auth_unavailable"
}
