package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

const (
	releaseOwner        = "deividfortuna"
	releaseRepo         = "babysitter"
	releaseCheckTimeout = 3 * time.Second
	appDaemonDir        = ".app/Contents/Resources/daemon/"
	appUpgradeHint      = "The desktop app updates itself, or run: brew upgrade --cask babysitter"
)

type versionOutput struct {
	Version         string `json:"version"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Upgrade         string `json:"upgrade,omitempty"`
}

func (v versionOutput) writeText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "babysitter version %s\n", v.Version); err != nil {
		return err
	}
	if !v.UpdateAvailable {
		return nil
	}
	_, err := fmt.Fprintf(w, "babysitter %s is out. %s\n", v.Latest, v.Upgrade)
	return err
}

func newVersionCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, and the upgrade when a newer release is out",
		Long: `version prints the version of this binary. It also asks GitHub for the
latest stable release. When that release is newer, it prints how to upgrade.
It never upgrades by itself. When GitHub does not answer in 3 seconds, it
prints the version only.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.print(cmd.OutOrStdout(), opts.versionReport(cmd.Context()))
		},
	}
}

func (o *options) versionReport(ctx context.Context) versionOutput {
	report := versionOutput{Version: o.version}
	if !semver.IsValid(tagOf(o.version)) {
		return report
	}
	release, ok := o.latestRelease(ctx)
	if !ok {
		return report
	}
	if !isNewer(o.version, release.Tag) {
		return report
	}
	report.Latest = strings.TrimPrefix(release.Tag, "v")
	report.UpdateAvailable = true
	report.Upgrade = upgradeHint(o.executablePath(), release.URL)
	return report
}

func (o *options) latestRelease(ctx context.Context) (ghclient.Release, bool) {
	client, err := o.newReleaseClient()
	if err != nil {
		return ghclient.Release{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, releaseCheckTimeout)
	defer cancel()
	release, err := ghclient.LatestRelease(ctx, client, releaseOwner, releaseRepo)
	return release, err == nil
}

func (o *options) executablePath() string {
	path, err := o.executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func tagOf(version string) string {
	return "v" + strings.TrimPrefix(version, "v")
}

func isNewer(current, tag string) bool {
	return semver.IsValid(tag) && semver.Compare(tag, tagOf(current)) > 0
}

func upgradeHint(executable, releaseURL string) string {
	if strings.Contains(filepath.ToSlash(executable), appDaemonDir) {
		return appUpgradeHint
	}
	return "Download it from " + releaseURL
}
