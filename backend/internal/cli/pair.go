package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/remote"
	"github.com/deividfortuna/babysitter/internal/runfile"
)

type pairOutput struct {
	Port  int      `json:"port"`
	Token string   `json:"token"`
	Links []string `json:"links"`
}

func (p pairOutput) writeText(w io.Writer) error {
	fmt.Fprintf(w, "Port:  %d\n", p.Port)
	fmt.Fprintf(w, "Token: %s\n\n", p.Token)
	fmt.Fprintln(w, "Paste one of these links in the app, under Settings > Connections:")
	for _, link := range p.Links {
		fmt.Fprintf(w, "  %s\n", link)
	}
	return nil
}

func newDaemonPairCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var rotate bool
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Print the link that connects the app of another machine to this daemon",
		Long: `Print the pairing link of a daemon that runs with --remote. The link
holds the token that the daemon wants from other machines. Keep it
secret: who has it can read and drive every watch.

--rotate makes a new token. The app of each other machine must pair
again, and the daemon takes the new token when it starts again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir, err := opts.dataDir(*dataDirFlag)
			if err != nil {
				return err
			}
			info, err := runfile.Live(runfile.Path(dataDir))
			if err != nil {
				return err
			}
			if info == nil || info.RemotePort == 0 {
				return errors.New("no daemon with remote access runs; start one with 'babysitter daemon start --remote :7420'")
			}
			token, err := pairingToken(dataDir, rotate)
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), pairOutput{
				Port:  info.RemotePort,
				Token: token,
				Links: remote.PairingLinks(info.RemotePort, token),
			})
		},
	}
	cmd.Flags().BoolVar(&rotate, "rotate", false, "make a new token, so the links printed before stop working")
	return cmd
}

func pairingToken(dataDir string, rotate bool) (string, error) {
	if rotate {
		return remote.RotateToken(dataDir)
	}
	return remote.Token(dataDir)
}
