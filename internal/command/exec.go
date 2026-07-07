package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/pkg/docker"
)

type execCommand struct {
	cmd *cobra.Command
}

func newExecCommand() *execCommand {
	e := &execCommand{}
	e.cmd = &cobra.Command{
		Use:   "exec <host> <command> [args...]",
		Short: "Execute a command in an application container",
		Args:  cobra.MinimumNArgs(2),
		RunE:  WithNamespace(e.run),
	}
	e.cmd.Flags().SetInterspersed(false)
	return e
}

// Private

func (e *execCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	host, commandArgs, err := splitExecArgs(args)
	if err != nil {
		return err
	}

	err = withApplication(ns, host, "executing command in", func(app *docker.Application) error {
		return app.Exec(ctx, commandArgs)
	})
	if IsExitError(err) {
		cmd.SilenceErrors = true
	}
	return err
}

// Helpers

func splitExecArgs(args []string) (string, []string, error) {
	commandArgs := args[1:]
	if commandArgs[0] == "--" {
		commandArgs = commandArgs[1:]
	}
	if len(commandArgs) == 0 {
		return "", nil, fmt.Errorf("command is required")
	}

	return args[0], commandArgs, nil
}
