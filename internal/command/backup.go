package command

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type BackupCommand struct {
	cmd *cobra.Command
}

func NewBackupCommand() *BackupCommand {
	b := &BackupCommand{}
	b.cmd = &cobra.Command{
		Use:   "backup <app> <filename>",
		Short: "Backup an application to a file",
		Args:  cobra.ExactArgs(2),
		RunE:  WithNamespace(b.run),
	}
	return b
}

func (b *BackupCommand) Command() *cobra.Command {
	return b.cmd
}

// Private

func (b *BackupCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	appName := args[0]
	filename := args[1]

	dir := filepath.Dir(filename)
	name := filepath.Base(filename)

	err := withApplication(ns, appName, "backing up", func(app *docker.Application) error {
		return app.BackupToFile(ctx, dir, name)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Backed up %s to %s\n", appName, filename)
	return nil
}
