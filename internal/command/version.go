package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/version"
)

type VersionCommand struct {
	root *RootCommand
	cmd  *cobra.Command
}

func NewVersionCommand(root *RootCommand) *VersionCommand {
	v := &VersionCommand{root: root}
	v.cmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Version)
		},
	}
	return v
}

func (v *VersionCommand) Command() *cobra.Command {
	return v.cmd
}
