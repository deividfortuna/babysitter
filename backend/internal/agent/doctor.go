package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/deividfortuna/babysitter/internal/execx"
)

func CheckVersion(ctx context.Context, run execx.DirRunner, bin string) error {
	if run == nil {
		run = execx.RunIn
	}
	out, err := run(ctx, "", "", nil, bin, "--version")
	if err != nil {
		return fmt.Errorf("%s is not usable: %w", bin, err)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("%s --version printed nothing", bin)
	}
	return nil
}
