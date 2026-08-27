package command

import (
	"github.com/spf13/cobra"
)

type proxyCommand struct {
	cmd *cobra.Command
}

func newProxyCommand() *proxyCommand {
	p := &proxyCommand{}
	p.cmd = &cobra.Command{
		Use:   "proxy",
		Short: "Manage the proxy container",
	}

	p.cmd.AddCommand(newProxyUpdateCommand().cmd)

	return p
}
