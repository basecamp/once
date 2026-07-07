package command

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/pkg/docker"
)

type restoreCommand struct {
	cmd *cobra.Command
}

func newRestoreCommand() *restoreCommand {
	r := &restoreCommand{}
	r.cmd = &cobra.Command{
		Use:   "restore <filename>",
		Short: "Restore an application from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(r.run),
	}
	return r
}

// Private

func (r *restoreCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	filename := args[0]

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("opening backup file: %w", err)
	}
	defer file.Close()

	if err := ns.Setup(ctx); err != nil {
		return fmt.Errorf("setting up namespace: %w", err)
	}

	app, err := ns.Restore(ctx, file)
	if err != nil {
		return fmt.Errorf("restoring application: %w", err)
	}

	fmt.Printf("Restored %s from %s\n", app.Settings.Name, filename)
	return nil
}
