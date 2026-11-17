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
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
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
	execID, resp, err := startExec(ctx, c, containerName, client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return ExecResult{}, err
	}
	defer resp.Close()

	// The Docker client doesn't close a hijacked connection when the context
	// ends, so a command that never finishes would block here regardless.
	stop := context.AfterFunc(ctx, resp.Close)
	defer stop()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		if ctx.Err() != nil {
			return ExecResult{}, fmt.Errorf("reading exec output: %w", context.Cause(ctx))
		}
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

	execID, resp, err := startExec(ctx, c, containerName, client.ExecCreateOptions{
		Cmd:          cmd,
		TTY:          tty,
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

func startExec(ctx context.Context, c *client.Client, containerName string, opts client.ExecCreateOptions) (string, client.HijackedResponse, error) {
	execResp, err := c.ExecCreate(ctx, containerName, opts)
	if err != nil {
		return "", client.HijackedResponse{}, fmt.Errorf("creating exec: %w", err)
	}
	if execResp.ID == "" {
		return "", client.HijackedResponse{}, fmt.Errorf("creating exec: empty exec ID")
	}

	resp, err := c.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{
		TTY:         opts.TTY,
		ConsoleSize: opts.ConsoleSize,
	})
	if err != nil {
		return "", client.HijackedResponse{}, fmt.Errorf("attaching exec: %w", err)
	}

	return execResp.ID, resp.HijackedResponse, nil
}

func inspectExecExitCode(ctx context.Context, c *client.Client, execID string) (int, error) {
	inspect, err := c.ExecInspect(ctx, execID, client.ExecInspectOptions{})
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

func execConsoleSize(fd uintptr, tty bool) client.ConsoleSize {
	if !tty {
		return client.ConsoleSize{}
	}

	size, _ := terminalSize(fd)
	return size
}

func monitorExecTTYSize(ctx context.Context, c *client.Client, execID string, fd uintptr) {
	resize := func() {
		if options, ok := execResizeOptions(fd); ok {
			_, _ = c.ExecResize(ctx, execID, options)
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

func execResizeOptions(fd uintptr) (client.ExecResizeOptions, bool) {
	size, ok := terminalSize(fd)
	return client.ExecResizeOptions(size), ok
}

func terminalSize(fd uintptr) (client.ConsoleSize, bool) {
	width, height, err := term.GetSize(fd)
	if err != nil || width <= 0 || height <= 0 {
		return client.ConsoleSize{}, false
	}

	return client.ConsoleSize{Height: uint(height), Width: uint(width)}, true
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
