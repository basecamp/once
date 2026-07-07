package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/pkg/service"
)

type backgroundInstallCommand struct {
	cmd *cobra.Command
}

const backgroundServiceSuffix = "-background"

func newBackgroundInstallCommand() *backgroundInstallCommand {
	b := &backgroundInstallCommand{}
	b.cmd = &cobra.Command{
		Use:   "install",
		Short: "Install background tasks as a system service",
		Args:  cobra.NoArgs,
		RunE:  b.run,
	}
	return b
}

// Private

func (b *backgroundInstallCommand) run(cmd *cobra.Command, args []string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must be run as root")
	}

	ctx := context.Background()
	namespace := namespaceFlag(cmd)

	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	svc, err := service.New()
	if err != nil {
		return err
	}

	serviceName := namespace + backgroundServiceSuffix

	if svc.IsInstalled(serviceName) {
		fmt.Printf("Service %s is already installed\n", svc.ServiceName(serviceName))
		return nil
	}

	if err := svc.Install(ctx, serviceName, execPath, namespace); err != nil {
		return fmt.Errorf("installing service: %w", err)
	}

	fmt.Printf("Installed and started %s\n", svc.ServiceName(serviceName))
	return nil
}

// Helpers

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
