package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type upgradeCommand struct {
	cmd *cobra.Command
}

func newUpgradeCommand() *upgradeCommand {
	u := &upgradeCommand{}
	u.cmd = &cobra.Command{
		Use:   "upgrade <host>",
		Short: "Check for and apply updates to a deployed application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(u.run),
	}
	return u
}

// Private

func (u *upgradeCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	host := args[0]

	app := ns.ApplicationByHost(host)
	if app == nil {
		return fmt.Errorf("no application found at host %q", host)
	}

	var changed bool
	err := runWithProgress("Checking for updates to "+host, func(progress docker.DeployProgressCallback) error {
		var err error
		changed, err = app.Update(ctx, progress)
		return err
	})
	if err != nil {
		return err
	}

	if changed {
		fmt.Printf("Updated %s\n", host)
	} else {
		fmt.Printf("%s is already running the latest version\n", host)
	}
	return nil
}
