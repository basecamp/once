package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

const systemdUnitDir = "/etc/systemd/system"

const unitTemplate = `[Unit]
Description=Once background tasks (%s)
After=network.target docker.service

[Service]
Type=simple
ExecStart=%s background run --namespace %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

type Systemd struct{}

func (s *Systemd) IsInstalled(name string) bool {
	_, err := os.Stat(s.unitFilePath(name))
	return err == nil
}

func (s *Systemd) Install(ctx context.Context, name, execPath, namespace string) error {
	path := s.unitFilePath(name)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating systemd directory: %w", err)
	}

	unitContent := fmt.Sprintf(unitTemplate, namespace, execPath, namespace)

	if err := os.WriteFile(path, []byte(unitContent), 0o644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	if err := s.daemonReload(ctx); err != nil {
		return err
	}

	return s.systemctl(ctx, "enable", "--now", name)
}

func (s *Systemd) Remove(ctx context.Context, name string) error {
	if err := s.systemctl(ctx, "disable", "--now", name); err != nil {
		return err
	}

	path := s.unitFilePath(name)

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing unit file: %w", err)
	}

	return s.daemonReload(ctx)
}

func (s *Systemd) ServiceName(name string) string {
	return name + ".service"
}

// Private

func (s *Systemd) daemonReload(ctx context.Context) error {
	return s.systemctl(ctx, "daemon-reload")
}

func (s *Systemd) systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s: %w", args[0], err)
	}
	return nil
}

func (s *Systemd) unitFilePath(name string) string {
	return filepath.Join(systemdUnitDir, name+".service")
}
