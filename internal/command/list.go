package command

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

var hostStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlue)

type listCommand struct {
	cmd *cobra.Command
}

func newListCommand() *listCommand {
	l := &listCommand{}
	l.cmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed applications",
		RunE:    WithNamespace(l.run),
	}
	return l
}

// Private

func (l *listCommand) run(_ context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	for _, app := range ns.Applications() {
		status := "stopped"
		if app.Running {
			status = "running"
		}

		host := hostStyle.Hyperlink(app.URL()).Render(app.Settings.Host)

		fmt.Printf("%s (%s)\n", host, status)
	}

	return nil
}
