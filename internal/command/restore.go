package command

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type RestoreCommand struct {
	cmd *cobra.Command
}

func NewRestoreCommand() *RestoreCommand {
	r := &RestoreCommand{}
	r.cmd = &cobra.Command{
		Use:   "restore <filename>",
		Short: "Restore an application from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(r.run),
	}
	return r
}

func (r *RestoreCommand) Command() *cobra.Command {
	return r.cmd
}

// Private

func (r *RestoreCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()
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
