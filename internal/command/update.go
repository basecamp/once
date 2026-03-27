package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/version"
)

type updateCommand struct {
	cmd *cobra.Command
}

func newUpdateCommand() *updateCommand {
	u := &updateCommand{}
	u.cmd = &cobra.Command{
		Use:   "update [app]",
		Short: "Update once to the latest version, or update a specific application",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return version.NewUpdater().UpdateBinary()
			}
			return WithNamespace(u.run)(cmd, args)
		},
	}
	return u
}

// Private

func (u *updateCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	appName := args[0]

	var changed bool
	err := withApplication(ns, appName, "updating", func(app *docker.Application) error {
		var err error
		changed, err = app.Update(ctx, printDeployProgress)
		return err
	})
	if err != nil {
		return err
	}

	if changed {
		fmt.Printf("Updated %s\n", appName)
	} else {
		fmt.Printf("%s is already up to date\n", appName)
	}
	return nil
}
