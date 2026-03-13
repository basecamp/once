package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type deployCommand struct {
	cmd          *cobra.Command
	host         string
	disableTLS   bool
	env          []string
	smtpServer   string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	smtpFrom     string
	cpus         int
	memory       int
	autoUpdate   bool
	backupPath   string
	autoBackup   bool
}

func newDeployCommand() *deployCommand {
	d := &deployCommand{}
	d.cmd = &cobra.Command{
		Use:   "deploy <image>",
		Short: "Deploy an application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(d.run),
	}

	d.cmd.Flags().StringVar(&d.host, "host", "", "hostname for the application (defaults to <name>.localhost)")
	d.cmd.Flags().BoolVar(&d.disableTLS, "disable-tls", false, "disable TLS for this application")
	d.cmd.Flags().StringArrayVar(&d.env, "env", nil, "environment variable in KEY=VALUE format (can be repeated)")
	d.cmd.Flags().StringVar(&d.smtpServer, "smtp-server", "", "SMTP server address")
	d.cmd.Flags().StringVar(&d.smtpPort, "smtp-port", "", "SMTP server port")
	d.cmd.Flags().StringVar(&d.smtpUsername, "smtp-username", "", "SMTP username")
	d.cmd.Flags().StringVar(&d.smtpPassword, "smtp-password", "", "SMTP password")
	d.cmd.Flags().StringVar(&d.smtpFrom, "smtp-from", "", "SMTP from address")
	d.cmd.Flags().IntVar(&d.cpus, "cpus", 0, "CPU limit for the container")
	d.cmd.Flags().IntVar(&d.memory, "memory", 0, "memory limit in MB for the container")
	d.cmd.Flags().BoolVar(&d.autoUpdate, "auto-update", true, "automatically update the application")
	d.cmd.Flags().StringVar(&d.backupPath, "backup-path", "", "path for backups")
	d.cmd.Flags().BoolVar(&d.autoBackup, "auto-backup", false, "enable automatic backups")

	return d
}

// Private

func (d *deployCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	imageRef := args[0]

	if err := ns.Setup(ctx); err != nil {
		return fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)
	}

	host := d.host
	if host == "" {
		host = docker.NameFromImageRef(imageRef) + ".localhost"
	}

	envVars, err := d.parseEnvVars()
	if err != nil {
		return err
	}

	settings := docker.ApplicationSettings{
		Image:      imageRef,
		Host:       host,
		DisableTLS: d.disableTLS,
		EnvVars:    envVars,
		SMTP: docker.SMTPSettings{
			Server:   d.smtpServer,
			Port:     d.smtpPort,
			Username: d.smtpUsername,
			Password: d.smtpPassword,
			From:     d.smtpFrom,
		},
		Resources: docker.ContainerResources{
			CPUs:     d.cpus,
			MemoryMB: d.memory,
		},
		AutoUpdate: d.autoUpdate,
		Backup: docker.BackupSettings{
			Path:       d.backupPath,
			AutoBackup: d.autoBackup,
		},
	}

	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			fmt.Printf("Downloading: %d%%\n", p.Percentage)
		case docker.DeployStageStarting:
			fmt.Println("Starting...")
		case docker.DeployStageFinished:
			fmt.Println("Finished")
		}
	}

	app, isNew, err := ns.DeployApplication(ctx, settings, progress)
	if err != nil {
		return fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)
	}

	fmt.Println("Verifying...")
	if err := app.VerifyHTTP(ctx); err != nil {
		if isNew {
			if cleanupErr := app.Destroy(context.Background(), true); cleanupErr != nil {
				slog.Error("Failed to clean up after verification failure", "app", app.Settings.Name, "error", cleanupErr)
			}
		}
		return err
	}

	fmt.Printf("Deployed %s\n", app.Settings.Name)
	return nil
}

func (d *deployCommand) parseEnvVars() (map[string]string, error) {
	if d.env == nil {
		return nil, nil
	}

	envVars := make(map[string]string, len(d.env))
	for _, e := range d.env {
		key, value, ok := strings.Cut(e, "=")
		if !ok {
			return nil, fmt.Errorf("invalid environment variable %q: must be in KEY=VALUE format", e)
		}
		if key == "" {
			return nil, fmt.Errorf("invalid environment variable %q: key must not be empty", e)
		}
		envVars[key] = value
	}

	return envVars, nil
}
