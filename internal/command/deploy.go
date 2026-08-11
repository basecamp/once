package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type deployCommand struct {
	cmd   *cobra.Command
	flags settingsFlags
}

func newDeployCommand() *deployCommand {
	d := &deployCommand{}
	d.cmd = &cobra.Command{
		Use:   "deploy <image>",
		Short: "Deploy an application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(d.run),
	}

	d.flags.register(d.cmd)

	return d
}

// Private

func (d *deployCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	imageRef := args[0]

	if err := ns.Setup(ctx); err != nil {
		return fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)
	}

	hosts := d.flags.host
	if len(hosts) == 0 {
		hosts = []string{docker.NameFromImageRef(imageRef) + ".localhost"}
	}

	for _, host := range hosts {
		if ns.HostInUse(host) {
			return docker.ErrHostnameInUse
		}
	}

	settings, err := d.flags.buildSettings(imageRef, strings.Join(hosts, ","))
	if err != nil {
		return err
	}

	baseName := docker.NameFromImageRef(imageRef)
	name, err := ns.UniqueName(baseName)
	if err != nil {
		return fmt.Errorf("generating app name: %w", err)
	}
	settings.Name = name

	app := docker.NewApplication(ns, settings)

	return runWithProgress("Deploying "+hosts[0], func(progress docker.DeployProgressCallback) error {
		if err := app.Deploy(ctx, progress); err != nil {
			if cleanupErr := app.Destroy(context.Background(), true); cleanupErr != nil {
				slog.Error("Failed to clean up after deploy failure", "app", name, "error", cleanupErr)
			}
			return fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)
		}

		if err := app.VerifyHTTPOrRemove(ctx); err != nil {
			return err
		}

		return nil
	})
}
