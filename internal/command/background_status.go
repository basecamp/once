package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/service"
)

type backgroundStatusCommand struct {
	cmd *cobra.Command
}

func newBackgroundStatusCommand() *backgroundStatusCommand {
	b := &backgroundStatusCommand{}
	b.cmd = &cobra.Command{
		Use:   "status",
		Short: "Show whether the background tasks service is installed and running",
		Args:  cobra.NoArgs,
		RunE:  b.run,
	}
	return b
}

// Private

func (b *backgroundStatusCommand) run(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	namespace := namespaceFlag(cmd)

	svc, err := service.New()
	if err != nil {
		return err
	}

	serviceName := namespace + backgroundServiceSuffix

	switch {
	case !svc.IsInstalled(serviceName):
		fmt.Printf("Service %s is not installed\n", svc.ServiceName(serviceName))
	case svc.IsRunning(ctx, serviceName):
		fmt.Printf("Service %s is installed and running\n", svc.ServiceName(serviceName))
		return nil
	default:
		fmt.Printf("Service %s is installed but not running\n", svc.ServiceName(serviceName))
	}

	cmd.SilenceErrors = true
	return notRunningError{}
}

type notRunningError struct{}

func (notRunningError) Error() string {
	return "service is not running"
}

func (notRunningError) ExitCode() int {
	return 3
}
