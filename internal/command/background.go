package command

import "github.com/spf13/cobra"

type BackgroundCommand struct {
	cmd *cobra.Command
}

func NewBackgroundCommand() *BackgroundCommand {
	b := &BackgroundCommand{}
	b.cmd = &cobra.Command{
		Use:   "background",
		Short: "Manage background tasks (automatic backups and updates)",
	}

	b.cmd.AddCommand(NewBackgroundInstallCommand().Command())
	b.cmd.AddCommand(NewBackgroundUninstallCommand().Command())
	b.cmd.AddCommand(NewBackgroundRunCommand().Command())

	return b
}

func (b *BackgroundCommand) Command() *cobra.Command {
	return b.cmd
}
