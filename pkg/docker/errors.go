package docker

import (
	"errors"
	"strings"
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
