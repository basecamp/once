package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type DeployCommand struct {
	cmd  *cobra.Command
	host string
}

func NewDeployCommand() *DeployCommand {
	d := &DeployCommand{}
	d.cmd = &cobra.Command{
		Use:   "deploy <image>",
		Short: "Deploy an application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(d.run),
	}
	d.cmd.Flags().StringVar(&d.host, "host", "", "hostname for the application (defaults to <name>.localhost)")
	return d
}

func (d *DeployCommand) Command() *cobra.Command {
	return d.cmd
}

// Private

func (d *DeployCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	imageRef := args[0]

	if err := ns.Setup(ctx); err != nil {
		return fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)
	}

	baseName := docker.NameFromImageRef(imageRef)
	name, err := ns.UniqueName(baseName)
	if err != nil {
		return fmt.Errorf("generating app name: %w", err)
	}

	host := d.host
	if host == "" {
		host = baseName + ".localhost"
	}

	if ns.HostInUse(host) {
		return docker.ErrHostnameInUse
	}

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:       name,
		Image:      imageRef,
		Host:       host,
		AutoUpdate: true,
	})

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

	if err := app.Deploy(ctx, progress); err != nil {
		return fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)
	}

	fmt.Println("Verifying...")
	if err := app.VerifyHTTP(ctx); err != nil {
		app.Destroy(ctx, true)
		return err
	}

	fmt.Printf("Deployed %s\n", name)
	return nil
}
