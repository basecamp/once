package command

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type backupCommand struct {
	cmd *cobra.Command
}

func newBackupCommand() *backupCommand {
	b := &backupCommand{}
	b.cmd = &cobra.Command{
		Use:   "backup <host> <filename>",
		Short: "Backup an application to a file",
		Args:  cobra.ExactArgs(2),
		RunE:  WithNamespace(b.run),
	}
	return b
}

// Private

func (b *backupCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	host := args[0]
	filename := args[1]

	dir := filepath.Dir(filename)
	name := filepath.Base(filename)

	err := withApplication(ns, host, "backing up", func(app *docker.Application) error {
		return app.BackupToFile(ctx, dir, name)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Backed up %s to %s\n", host, filename)
	return nil
}
