package command

import (
	"github.com/spf13/cobra"

	"github.com/basecamp/once/pkg/version"
)

type selfUpdateCommand struct {
	cmd *cobra.Command
}

func newSelfUpdateCommand() *selfUpdateCommand {
	u := &selfUpdateCommand{}
	u.cmd = &cobra.Command{
		Use:   "self-update",
		Short: "Update once to the latest version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return version.NewUpdater().UpdateBinary()
		},
	}
	return u
}
