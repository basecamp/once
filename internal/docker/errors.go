package docker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/errdefs"
)

type DescribedError interface {
	error
	Description() string
}

var (
	ErrProxyPortInUse = &describedError{
		msg:         "proxy port conflict",
		description: "Something else is using the web ports on this machine. You'll need to stop that service, and then try deploying again.",
	}
	ErrAppNotStarted = &describedError{
		msg:         "application did not start",
		description: "The application did not start within the time limit. Check the application logs for errors.",
	}
)

type RegistryAuthError struct {
	Registry string
	Cause    error
}

func (e *RegistryAuthError) Error() string {
	return fmt.Sprintf("registry authentication required for %s: %v", e.Registry, e.Cause)
}

func (e *RegistryAuthError) Description() string {
	return fmt.Sprintf("Log in to %s first. This image can't be downloaded without registry credentials.", e.Registry)
}

func (e *RegistryAuthError) Unwrap() error { return e.Cause }

func (e *RegistryAuthError) Is(target error) bool { return target == ErrPullFailed }

func ErrorMessage(err error) string {
	var de DescribedError
	if errors.As(err, &de) {
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

func isRegistryAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errdefs.IsUnauthorized(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, indicator := range []string{
		"unauthorized",
		"authentication required",
		"pull access denied",
		"no basic auth credentials",
		"insufficient_scope",
		"failed to authorize",
	} {
		if strings.Contains(msg, indicator) {
			return true
		}
	}
	return false
}
