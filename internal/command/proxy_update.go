package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type proxyUpdateCommand struct {
	cmd *cobra.Command
}

func newProxyUpdateCommand() *proxyUpdateCommand {
	u := &proxyUpdateCommand{}
	u.cmd = &cobra.Command{
		Use:   "update",
		Short: "Update the proxy to the latest image",
		Args:  cobra.NoArgs,
		RunE:  WithNamespace(u.run),
	}
	return u
}

// Private

func (u *proxyUpdateCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	updated, err := ns.Proxy().Update(ctx)
	if err != nil {
		return err
	}

	if updated {
		fmt.Printf("Proxy updated to %s\n", ns.Proxy().Image)
	} else {
		fmt.Println("Proxy is already up to date")
	}

	return nil
}
