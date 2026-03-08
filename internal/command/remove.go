package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type RemoveCommand struct {
	cmd        *cobra.Command
	removeData bool
}

func NewRemoveCommand() *RemoveCommand {
	r := &RemoveCommand{}
	r.cmd = &cobra.Command{
		Use:     "remove <app>",
		Aliases: []string{"rm"},
		Short:   "Remove an application",
		Args:    cobra.ExactArgs(1),
		RunE:    WithNamespace(r.run),
	}
	r.cmd.Flags().BoolVar(&r.removeData, "remove-data", false, "Also remove application data volume")
	return r
}

func (r *RemoveCommand) Command() *cobra.Command {
	return r.cmd
}

// Private

func (r *RemoveCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	appName := args[0]

	err := withApplication(ns, appName, "removing", func(app *docker.Application) error {
		return app.Remove(ctx, r.removeData)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Removed %s\n", appName)
	return nil
}
