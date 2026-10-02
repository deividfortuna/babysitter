package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient"
)

type authOutput struct {
	Origin        string     `json:"origin"`
	AppAvailable  bool       `json:"appAvailable"`
	SignedIn      bool       `json:"signedIn"`
	Login         string     `json:"login,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	InstallURL    string     `json:"installUrl"`
	Installations []string   `json:"installations,omitempty"`
	Error         string     `json:"error,omitempty"`
}

var originWords = map[string]string{
	"":                        "none",
	string(ghauth.OriginFlag): "the --token flag",
	string(ghauth.OriginEnv):  "the GITHUB_TOKEN environment variable",
	string(ghauth.OriginApp):  "the babysitter GitHub App",
	string(ghauth.OriginGH):   "the gh CLI",
}

func (a authOutput) writeText(w io.Writer) error {
	fmt.Fprintf(w, "Token from:    %s\n", originWords[a.Origin])
	if a.SignedIn {
		fmt.Fprintf(w, "App account:   %s\n", a.Login)
		fmt.Fprintf(w, "Installed on:  %s\n", kindsWord(a.Installations))
	} else {
		fmt.Fprintln(w, "App account:   not signed in")
	}
	if a.SignedIn && a.Origin != string(ghauth.OriginApp) && a.Error == "" {
		fmt.Fprintf(w, "Note:          %s comes before the app\n", originWords[a.Origin])
	}
	if a.Error != "" {
		fmt.Fprintf(w, "Error:         %s\n", a.Error)
	}
	_, err := fmt.Fprintf(w, "Install:       %s\n", a.InstallURL)
	return err
}

func newAuthCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Give babysitter access to GitHub through the babysitter GitHub App",
		Long: `The babysitter GitHub App is another way to give babysitter access to
GitHub, next to a token and the gh CLI. The app acts as you, but only on the
repositories where you install it and with the permissions of the app.
'auth login' signs in with a code you enter on GitHub. The daemon and the
desktop app use the same sign in, and they renew its token on their own.

The --token flag and GITHUB_TOKEN come before the app; the gh CLI comes
after it. 'auth logout' goes back to the next source.`,
	}
	cmd.PersistentFlags().StringVar(&opts.authDir, "data-dir", "", "directory of the GitHub App sign in (overrides BABYSITTER_DATA_DIR)")
	cmd.AddCommand(newAuthLoginCmd(opts), newAuthStatusCmd(opts), newAuthLogoutCmd(opts))
	return cmd
}

func newAuthLoginCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in with the babysitter GitHub App",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			auth, err := opts.githubAuth()
			if err != nil {
				return err
			}
			code, err := auth.RequestCode(ctx)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Open %s and enter the code %s\n", code.VerificationURI, code.UserCode)
			fmt.Fprintln(out, "Waiting for GitHub...")
			creds, err := auth.Complete(ctx, code, opts.whoami)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Signed in to GitHub as %s with the babysitter GitHub App.\n", creds.Login)
			fmt.Fprintf(out, "Install the app on the repositories it may watch: %s\n", auth.App().InstallURL())
			if st := auth.Status(ctx); st.Origin != ghauth.OriginApp {
				fmt.Fprintf(out, "Note: %s comes before the app, so babysitter does not use the app yet.\n", originWords[string(st.Origin)])
			}
			return nil
		},
	}
}

func newAuthStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print where the GitHub token comes from and the GitHub App sign in",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			auth, err := opts.githubAuth()
			if err != nil {
				return err
			}
			st := auth.Status(ctx)
			out := authOutput{
				Origin:       string(st.Origin),
				AppAvailable: st.Available,
				SignedIn:     st.SignedIn,
				Login:        st.Login,
				InstallURL:   st.InstallURL,
			}
			if st.Err != nil {
				out.Error = st.Err.Error()
			}
			if st.SignedIn && !st.ExpiresAt.IsZero() {
				out.ExpiresAt = &st.ExpiresAt
			}
			if st.Origin == ghauth.OriginApp {
				out.Installations, err = installationAccounts(cmd, opts)
				if err != nil {
					return err
				}
			}
			return opts.print(cmd.OutOrStdout(), out)
		},
	}
}

func installationAccounts(cmd *cobra.Command, opts *options) ([]string, error) {
	client, err := opts.client(cmd.Context())
	if err != nil {
		return nil, err
	}
	installs, err := ghclient.UserInstallations(cmd.Context(), client)
	if err != nil {
		return nil, err
	}
	accounts := make([]string, 0, len(installs))
	for _, inst := range installs {
		accounts = append(accounts, inst.Account)
	}
	return accounts, nil
}

func newAuthLogoutCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out of the babysitter GitHub App",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			auth, err := opts.githubAuth()
			if err != nil {
				return err
			}
			if err := auth.SignOut(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Signed out of the babysitter GitHub App.")
			return err
		},
	}
}
