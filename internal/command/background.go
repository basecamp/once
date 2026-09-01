package command

import "github.com/spf13/cobra"

type backgroundCommand struct {
	cmd *cobra.Command
}

func newBackgroundCommand() *backgroundCommand {
	b := &backgroundCommand{}
	b.cmd = &cobra.Command{
		Use:   "background",
		Short: "Manage background tasks (automatic backups and updates)",
	}

	b.cmd.AddCommand(newBackgroundInstallCommand().cmd)
	b.cmd.AddCommand(newBackgroundUninstallCommand().cmd)
	b.cmd.AddCommand(newBackgroundStatusCommand().cmd)
	b.cmd.AddCommand(newBackgroundRunCommand().cmd)

	return b
}
