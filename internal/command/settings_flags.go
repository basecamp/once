package command

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type settingsFlags struct {
	host         string
	disableTLS   bool
	env          []string
	mounts       []string
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

func (f *settingsFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.host, "host", "", "hostname for the application")
	cmd.Flags().BoolVar(&f.disableTLS, "disable-tls", false, "disable TLS for this application")
	cmd.Flags().StringArrayVar(&f.env, "env", nil, "environment variable in KEY=VALUE format (can be repeated)")
	cmd.Flags().StringArrayVar(&f.mounts, "mount", nil, "bind mount in SOURCE:TARGET format (can be repeated)")
	cmd.Flags().StringVar(&f.smtpServer, "smtp-server", "", "SMTP server address")
	cmd.Flags().StringVar(&f.smtpPort, "smtp-port", "", "SMTP server port")
	cmd.Flags().StringVar(&f.smtpUsername, "smtp-username", "", "SMTP username")
	cmd.Flags().StringVar(&f.smtpPassword, "smtp-password", "", "SMTP password")
	cmd.Flags().StringVar(&f.smtpFrom, "smtp-from", "", "SMTP from address")
	cmd.Flags().IntVar(&f.cpus, "cpus", 0, "CPU limit for the container")
	cmd.Flags().IntVar(&f.memory, "memory", 0, "memory limit in MB for the container")
	cmd.Flags().BoolVar(&f.autoUpdate, "auto-update", true, "automatically update the application")
	cmd.Flags().StringVar(&f.backupPath, "backup-path", "", "path for backups")
	cmd.Flags().BoolVar(&f.autoBackup, "auto-backup", false, "enable automatic backups")
}

func (f *settingsFlags) buildSettings(image, host string) (docker.ApplicationSettings, error) {
	envVars, err := f.parseEnvVars()
	if err != nil {
		return docker.ApplicationSettings{}, err
	}

	mounts, err := f.parseMounts()
	if err != nil {
		return docker.ApplicationSettings{}, err
	}

	if f.backupPath != "" && !filepath.IsAbs(f.backupPath) {
		return docker.ApplicationSettings{}, docker.ErrBackupPathRelative
	}

	s := docker.ApplicationSettings{
		Image:      image,
		Host:       host,
		DisableTLS: f.disableTLS,
		EnvVars:    envVars,
		Mounts:     mounts,
		SMTP: docker.SMTPSettings{
			Server:   f.smtpServer,
			Port:     f.smtpPort,
			Username: f.smtpUsername,
			Password: f.smtpPassword,
			From:     f.smtpFrom,
		},
		Resources: docker.ContainerResources{
			CPUs:     f.cpus,
			MemoryMB: f.memory,
		},
		AutoUpdate: f.autoUpdate,
		Backup: docker.BackupSettings{
			Path:       f.backupPath,
			AutoBackup: f.autoBackup,
		},
	}

	if err := s.Validate(); err != nil {
		return docker.ApplicationSettings{}, err
	}

	return s, nil
}

func (f *settingsFlags) applyChanges(cmd *cobra.Command, existing docker.ApplicationSettings, image string) (docker.ApplicationSettings, error) {
	s := existing

	s.Image = image

	if cmd.Flags().Changed("host") {
		s.Host = f.host
	}
	if cmd.Flags().Changed("disable-tls") {
		s.DisableTLS = f.disableTLS
	}
	if cmd.Flags().Changed("env") {
		envVars, err := f.parseEnvVars()
		if err != nil {
			return s, err
		}
		s.EnvVars = envVars
	}
	if cmd.Flags().Changed("smtp-server") {
		s.SMTP.Server = f.smtpServer
	}
	if cmd.Flags().Changed("smtp-port") {
		s.SMTP.Port = f.smtpPort
	}
	if cmd.Flags().Changed("smtp-username") {
		s.SMTP.Username = f.smtpUsername
	}
	if cmd.Flags().Changed("smtp-password") {
		s.SMTP.Password = f.smtpPassword
	}
	if cmd.Flags().Changed("smtp-from") {
		s.SMTP.From = f.smtpFrom
	}
	if cmd.Flags().Changed("cpus") {
		s.Resources.CPUs = f.cpus
	}
	if cmd.Flags().Changed("memory") {
		s.Resources.MemoryMB = f.memory
	}
	if cmd.Flags().Changed("auto-update") {
		s.AutoUpdate = f.autoUpdate
	}
	if cmd.Flags().Changed("backup-path") {
		if f.backupPath != "" && !filepath.IsAbs(f.backupPath) {
			return s, docker.ErrBackupPathRelative
		}
		s.Backup.Path = f.backupPath
	}
	if cmd.Flags().Changed("auto-backup") {
		s.Backup.AutoBackup = f.autoBackup
	}
	if cmd.Flags().Changed("mount") {
		mounts, err := f.parseMounts()
		if err != nil {
			return s, err
		}
		s.Mounts = mounts
	}

	if err := s.Validate(); err != nil {
		return s, err
	}

	return s, nil
}

func (f *settingsFlags) parseMounts() ([]docker.MountSetting, error) {
	if f.mounts == nil {
		return nil, nil
	}

	var mounts []docker.MountSetting
	for _, m := range f.mounts {
		source, target, ok := strings.Cut(m, ":")
		if !ok {
			return nil, fmt.Errorf("invalid mount %q: must be in SOURCE:TARGET format", m)
		}
		mounts = append(mounts, docker.MountSetting{Source: source, Target: target})
	}

	return mounts, nil
}

func (f *settingsFlags) parseEnvVars() (map[string]string, error) {
	if f.env == nil {
		return nil, nil
	}

	envVars := make(map[string]string, len(f.env))
	for _, e := range f.env {
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
