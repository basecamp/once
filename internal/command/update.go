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
		RunE:  WithNamespace(u.run),
	}
	return u
}

// Private

func (u *updateCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return version.NewUpdater().UpdateBinary()
	}

	appName := args[0]
	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			fmt.Printf("Downloading: %d%%\n", p.Percentage)
		case docker.DeployStageStarting:
			fmt.Println("Starting...")
		case docker.DeployStageFinished:
			fmt.Println("Finished")
		}
	}

	var changed bool
	err := withApplication(ns, appName, "updating", func(app *docker.Application) error {
		var err error
		changed, err = app.Update(ctx, progress)
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
