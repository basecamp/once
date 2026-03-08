package command

import (
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/background"
	"github.com/basecamp/once/internal/logging"
)

type BackgroundRunCommand struct {
	cmd *cobra.Command
}

func NewBackgroundRunCommand() *BackgroundRunCommand {
	b := &BackgroundRunCommand{}
	b.cmd = &cobra.Command{
		Use:    "run",
		Short:  "Run background tasks (automatic backups and updates)",
		Args:   cobra.NoArgs,
		Hidden: true,

		// Note: override parent PersistentPreRunE here, so we skip the default logging setup.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logging.SetupStderr()
			return nil
		},
		RunE: b.run,
	}
	return b
}

func (b *BackgroundRunCommand) Command() *cobra.Command {
	return b.cmd
}

// Private

func (b *BackgroundRunCommand) run(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	namespace, _ := cmd.Root().PersistentFlags().GetString("namespace")

	runner := background.NewRunner(namespace)

	return runner.Run(ctx)
}
