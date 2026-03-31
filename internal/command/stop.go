package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type stopCommand struct {
	cmd *cobra.Command
}

func newStopCommand() *stopCommand {
	s := &stopCommand{}
	s.cmd = &cobra.Command{
		Use:   "stop <host>",
		Short: "Stop an application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(s.run),
	}
	return s
}

// Private

func (s *stopCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	host := args[0]

	err := withApplication(ns, host, "stopping", func(app *docker.Application) error {
		return app.Stop(ctx)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Stopped %s\n", host)
	return nil
}
