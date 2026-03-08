package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type StartCommand struct {
	cmd *cobra.Command
}

func NewStartCommand() *StartCommand {
	s := &StartCommand{}
	s.cmd = &cobra.Command{
		Use:   "start <app>",
		Short: "Start an application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(s.run),
	}
	return s
}

func (s *StartCommand) Command() *cobra.Command {
	return s.cmd
}

// Private

func (s *StartCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	appName := args[0]

	err := withApplication(ns, appName, "starting", func(app *docker.Application) error {
		return app.Start(ctx)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Started %s\n", appName)
	return nil
}
