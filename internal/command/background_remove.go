package command

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/service"
)

type BackgroundUninstallCommand struct {
	cmd *cobra.Command
}

func NewBackgroundUninstallCommand() *BackgroundUninstallCommand {
	b := &BackgroundUninstallCommand{}
	b.cmd = &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the background tasks system service",
		Args:  cobra.NoArgs,
		RunE:  b.run,
	}
	return b
}

func (b *BackgroundUninstallCommand) Command() *cobra.Command {
	return b.cmd
}

// Private

func (b *BackgroundUninstallCommand) run(cmd *cobra.Command, args []string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must be run as root")
	}

	ctx := context.Background()

	namespace, _ := cmd.Root().PersistentFlags().GetString("namespace")

	svc, err := service.New()
	if err != nil {
		return err
	}

	serviceName := namespace + backgroundServiceSuffix

	if !svc.IsInstalled(serviceName) {
		fmt.Printf("Service %s is not installed\n", svc.ServiceName(serviceName))
		return nil
	}

	if err := svc.Remove(ctx, serviceName); err != nil {
		return fmt.Errorf("removing service: %w", err)
	}

	fmt.Printf("Uninstalled %s\n", svc.ServiceName(serviceName))
	return nil
}
