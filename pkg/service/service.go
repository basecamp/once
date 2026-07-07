package service

import (
	"context"
	"fmt"
	"runtime"
)

type Service interface {
	IsInstalled(name string) bool
	Install(ctx context.Context, name, execPath, namespace string) error
	Remove(ctx context.Context, name string) error
	ServiceName(name string) string
}

func New() (Service, error) {
	switch runtime.GOOS {
	case "linux":
		return &Systemd{}, nil
	case "darwin":
		return &Launchd{}, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
