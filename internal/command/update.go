package command

import (
	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/version"
)

type UpdateCommand struct {
	cmd *cobra.Command
}

func NewUpdateCommand() *UpdateCommand {
	u := &UpdateCommand{}
	u.cmd = &cobra.Command{
		Use:   "update",
		Short: "Update once to the latest version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return version.NewUpdater().UpdateBinary()
		},
	}
	return u
}

func (u *UpdateCommand) Command() *cobra.Command {
	return u.cmd
}
