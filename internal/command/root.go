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
	cmd       *cobra.Command
	namespace string

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
		RunE: WithNamespace(func(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
			return logging.ToLogFile(func() error {
				return ui.Run(ns, r.installImageRef)
			})
		}),
	}
	r.cmd.PersistentFlags().StringVarP(&r.namespace, "namespace", "n", docker.DefaultNamespace, "namespace for containers")

	r.cmd.Flags().StringVar(&r.installImageRef, "install", "", "Path to Docker image to install")

	r.cmd.AddCommand(newBackgroundCommand().cmd)
	r.cmd.AddCommand(newBackupCommand().cmd)
	r.cmd.AddCommand(newDeployCommand().cmd)
	r.cmd.AddCommand(newListCommand().cmd)
	r.cmd.AddCommand(newRemoveCommand().cmd)
	r.cmd.AddCommand(newRestoreCommand().cmd)
	r.cmd.AddCommand(newStartCommand().cmd)
	r.cmd.AddCommand(newStopCommand().cmd)
	r.cmd.AddCommand(newTeardownCommand().cmd)
	r.cmd.AddCommand(newUpdateCommand().cmd)
	r.cmd.AddCommand(newSelfUpdateCommand().cmd)
	r.cmd.AddCommand(newVersionCommand().cmd)

	return r
}

func (r *RootCommand) Execute() error {
	return r.cmd.Execute()
}

// Helpers

type NamespaceRunE func(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error

func withApplication(ns *docker.Namespace, host string, action string, fn func(*docker.Application) error) error {
	app := ns.ApplicationByHost(host)
	if app == nil {
		return fmt.Errorf("no application found at host %q", host)
	}

	if err := fn(app); err != nil {
		return fmt.Errorf("%s application: %w", action, err)
	}

	return nil
}

func WithNamespace(fn NamespaceRunE) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		ns, err := docker.RestoreNamespace(ctx, namespaceFlag(cmd))
		if err != nil {
			return fmt.Errorf("restoring namespace: %w", err)
		}

		return fn(ctx, ns, cmd, args)
	}
}

func namespaceFlag(cmd *cobra.Command) string {
	namespace, _ := cmd.Root().PersistentFlags().GetString("namespace")
	return namespace
}
