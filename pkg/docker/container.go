package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/term"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const ContainerLogMaxSize = "10m"

func ContainerLogConfig() container.LogConfig {
	return container.LogConfig{
		Type: "json-file",
		Config: map[string]string{
			"max-size": ContainerLogMaxSize,
			"max-file": "1",
		},
	}
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Helpers

func execInContainer(ctx context.Context, c *client.Client, containerName string, cmd []string) (ExecResult, error) {
	execID, resp, err := startExec(ctx, c, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return ExecResult{}, err
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return ExecResult{}, fmt.Errorf("reading exec output: %w", err)
	}

	exitCode, err := inspectExecExitCode(ctx, c, execID)
	if err != nil {
		return ExecResult{}, err
	}

	return ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

func execAttachedInContainer(ctx context.Context, c *client.Client, containerName string, cmd []string) error {
	stdinFD := os.Stdin.Fd()
	stdoutFD := os.Stdout.Fd()
	tty := term.IsTerminal(stdinFD) && term.IsTerminal(stdoutFD)
	consoleSize := execConsoleSize(stdoutFD, tty)

	restoreTerminal := func() {}
	if tty {
		state, err := term.MakeRaw(stdinFD)
		if err != nil {
			return fmt.Errorf("setting terminal raw mode: %w", err)
		}

		var once sync.Once
		restoreTerminal = func() {
			once.Do(func() {
				_ = term.Restore(stdinFD, state)
			})
		}
		defer restoreTerminal()
	}

	execID, resp, err := startExec(ctx, c, containerName, container.ExecOptions{
		Cmd:          cmd,
		Tty:          tty,
		ConsoleSize:  consoleSize,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	resizeCtx, cancelResize := context.WithCancel(ctx)
	defer cancelResize()
	if tty {
		monitorExecTTYSize(resizeCtx, c, execID, stdoutFD)
	}

	go func() {
		_, _ = io.Copy(resp.Conn, os.Stdin)
		_ = resp.CloseWrite()
	}()

	if err := copyExecOutput(resp.Reader, tty); err != nil {
		restoreTerminal()
		return err
	}
	restoreTerminal()

	exitCode, err := inspectExecExitCode(ctx, c, execID)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return execExitError{exitCode: exitCode}
	}

	return nil
}

func startExec(ctx context.Context, c *client.Client, containerName string, opts container.ExecOptions) (string, types.HijackedResponse, error) {
	execResp, err := c.ContainerExecCreate(ctx, containerName, opts)
	if err != nil {
		return "", types.HijackedResponse{}, fmt.Errorf("creating exec: %w", err)
	}
	if execResp.ID == "" {
		return "", types.HijackedResponse{}, fmt.Errorf("creating exec: empty exec ID")
	}

	resp, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{
		Tty:         opts.Tty,
		ConsoleSize: opts.ConsoleSize,
	})
	if err != nil {
		return "", types.HijackedResponse{}, fmt.Errorf("attaching exec: %w", err)
	}

	return execResp.ID, resp, nil
}

func inspectExecExitCode(ctx context.Context, c *client.Client, execID string) (int, error) {
	inspect, err := c.ContainerExecInspect(ctx, execID)
	if err != nil {
		return 0, fmt.Errorf("inspecting exec: %w", err)
	}
	return inspect.ExitCode, nil
}

func copyExecOutput(r io.Reader, tty bool) error {
	var err error
	if tty {
		_, err = io.Copy(os.Stdout, r)
	} else {
		_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, r)
	}
	if err != nil {
		return fmt.Errorf("reading exec output: %w", err)
	}
	return nil
}

func execConsoleSize(fd uintptr, tty bool) *[2]uint {
	if !tty {
		return nil
	}

	width, height, ok := terminalSize(fd)
	if !ok {
		return nil
	}

	return &[2]uint{height, width}
}

func monitorExecTTYSize(ctx context.Context, c *client.Client, execID string, fd uintptr) {
	resize := func() {
		if options, ok := execResizeOptions(fd); ok {
			_ = c.ContainerExecResize(ctx, execID, options)
		}
	}

	resize()

	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(sigwinch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigwinch:
				resize()
			}
		}
	}()
}

func execResizeOptions(fd uintptr) (container.ResizeOptions, bool) {
	width, height, ok := terminalSize(fd)
	if !ok {
		return container.ResizeOptions{}, false
	}

	return container.ResizeOptions{
		Height: height,
		Width:  width,
	}, true
}

func terminalSize(fd uintptr) (uint, uint, bool) {
	width, height, err := term.GetSize(fd)
	if err != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}

	return uint(width), uint(height), true
}

type execExitError struct {
	exitCode int
}

func (e execExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.exitCode)
}

func (e execExitError) ExitCode() int {
	return e.exitCode
}
