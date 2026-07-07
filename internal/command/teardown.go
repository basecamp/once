package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/pkg/docker"
)

type teardownCommand struct {
	cmd        *cobra.Command
	removeData bool
}

func newTeardownCommand() *teardownCommand {
	t := &teardownCommand{}
	t.cmd = &cobra.Command{
		Use:   "teardown",
		Short: "Remove all applications and the proxy",
		RunE:  WithNamespace(t.run),
	}
	t.cmd.Flags().BoolVar(&t.removeData, "remove-data", false, "Also remove application data volumes")
	return t
}

// Private

func (t *teardownCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	if err := ns.Teardown(ctx, t.removeData); err != nil {
		return fmt.Errorf("teardown failed: %w", err)
	}

	fmt.Println("Teardown complete")
	return nil
}
