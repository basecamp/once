package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/logging"
	"github.com/basecamp/once/internal/ui"
)

type RootCommand struct {
	cmd         *cobra.Command
	namespace   string
	closeLogger func()

	installImageRef string
}

func NewRootCommand() *RootCommand {
	r := &RootCommand{}
	r.cmd = &cobra.Command{
		Use:          "once",
		Short:        "Manage web applications from Docker images",
		SilenceUsage: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			closeLogger, err := logging.SetupFile()
			if err != nil {
				return fmt.Errorf("setting up logging: %w", err)
			}
			r.closeLogger = closeLogger
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if r.closeLogger != nil {
				r.closeLogger()
			}
		},
		RunE: WithNamespace(func(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
			return ui.Run(ns, r.installImageRef)
		}),
	}
	r.cmd.PersistentFlags().StringVarP(&r.namespace, "namespace", "n", docker.DefaultNamespace, "namespace for containers")

	r.cmd.Flags().StringVar(&r.installImageRef, "install", "", "Path to Docker image to install")

	r.cmd.AddCommand(NewBackgroundCommand().Command())
	r.cmd.AddCommand(NewBackupCommand().Command())
	r.cmd.AddCommand(NewDeployCommand().Command())
	r.cmd.AddCommand(NewListCommand().Command())
	r.cmd.AddCommand(NewRemoveCommand().Command())
	r.cmd.AddCommand(NewRestoreCommand().Command())
	r.cmd.AddCommand(NewStartCommand().Command())
	r.cmd.AddCommand(NewStopCommand().Command())
	r.cmd.AddCommand(NewTeardownCommand().Command())
	r.cmd.AddCommand(NewUpdateCommand().Command())
	r.cmd.AddCommand(NewVersionCommand().Command())

	return r
}

func (r *RootCommand) Execute() error {
	return r.cmd.Execute()
}

// Helpers

type NamespaceRunE func(ns *docker.Namespace, cmd *cobra.Command, args []string) error

func withApplication(ns *docker.Namespace, name string, action string, fn func(*docker.Application) error) error {
	app := ns.Application(name)
	if app == nil {
		return fmt.Errorf("application %q not found", name)
	}

	if err := fn(app); err != nil {
		return fmt.Errorf("%s application: %w", action, err)
	}

	return nil
}

func WithNamespace(fn NamespaceRunE) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		namespace, _ := cmd.Root().PersistentFlags().GetString("namespace")

		ns, err := docker.RestoreNamespace(ctx, namespace)
		if err != nil {
			return fmt.Errorf("restoring namespace: %w", err)
		}

		return fn(ns, cmd, args)
	}
}
