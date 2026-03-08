package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type TeardownCommand struct {
	cmd        *cobra.Command
	removeData bool
}

func NewTeardownCommand() *TeardownCommand {
	t := &TeardownCommand{}
	t.cmd = &cobra.Command{
		Use:   "teardown",
		Short: "Remove all applications and the proxy",
		RunE:  WithNamespace(t.run),
	}
	t.cmd.Flags().BoolVar(&t.removeData, "remove-data", false, "Also remove application data volumes")
	return t
}

func (t *TeardownCommand) Command() *cobra.Command {
	return t.cmd
}

// Private

func (t *TeardownCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if err := ns.Teardown(ctx, t.removeData); err != nil {
		return fmt.Errorf("teardown failed: %w", err)
	}

	fmt.Println("Teardown complete")
	return nil
}
