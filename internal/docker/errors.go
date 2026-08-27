package docker

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/client"
)

type DescribedError interface {
	error
	Description() string
}

var (
	ErrProxyNotInstalled = &describedError{
		msg:         "proxy is not installed",
		description: "No proxy container was found. Deploy an application first to set up the proxy.",
	}
	ErrProxyPortInUse = &describedError{
		msg:         "proxy port conflict",
		description: "Something else is using the web ports on this machine. You'll need to stop that service, and then try deploying again.",
	}
	ErrAppNotStarted = &describedError{
		msg:         "application did not start",
		description: "The application did not start within the time limit. Check the application logs for errors.",
	}
	ErrDockerPermissionDenied = &describedError{
		msg:         "permission denied connecting to Docker",
		description: "Permission denied when connecting to the Docker socket. Run with `sudo`, or add yourself to the `docker` group.",
	}
	ErrDockerNotRunning = &describedError{
		msg:         "cannot connect to Docker",
		description: "Could not connect to Docker. Make sure Docker is installed and the Docker daemon is running.",
	}
)

func ErrorMessage(err error) string {
	if de, ok := errors.AsType[DescribedError](err); ok {
		return de.Description()
	}
	return err.Error()
}

// Private

type describedError struct {
	msg         string
	description string
}

func (e *describedError) Error() string       { return e.msg }
func (e *describedError) Description() string { return e.description }

// Helpers

func isPortConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "bind: address already in use") ||
		strings.Contains(msg, "port is already allocated")
}

func connectionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("%w: %w", ErrDockerPermissionDenied, err)
	case client.IsErrConnectionFailed(err):
		return fmt.Errorf("%w: %w", ErrDockerNotRunning, err)
	default:
		return err
	}
}
