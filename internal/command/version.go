package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/pkg/version"
)

type versionCommand struct {
	cmd *cobra.Command
}

func newVersionCommand() *versionCommand {
	v := &versionCommand{}
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
