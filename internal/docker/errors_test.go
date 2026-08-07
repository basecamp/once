package docker

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPortConflict(t *testing.T) {
	assert.True(t, isPortConflict(errors.New("Ports are not available: listen tcp :80: bind: address already in use")))
	assert.True(t, isPortConflict(errors.New("driver failed programming external connectivity: port is already allocated")))
	assert.False(t, isPortConflict(errors.New("something else went wrong")))
	assert.False(t, isPortConflict(nil))
}

func TestErrorMessage(t *testing.T) {
	t.Run("returns description for described error", func(t *testing.T) {
		assert.Equal(t, ErrProxyPortInUse.Description(), ErrorMessage(ErrProxyPortInUse))
	})

	t.Run("returns description for wrapped described error", func(t *testing.T) {
		wrapped := fmt.Errorf("setup failed: %w", ErrProxyPortInUse)
		assert.Equal(t, ErrProxyPortInUse.Description(), ErrorMessage(wrapped))
	})

	t.Run("returns Error for plain error", func(t *testing.T) {
		err := errors.New("something broke")
		assert.Equal(t, "something broke", ErrorMessage(err))
	})
}

func TestIsRegistryAuthError(t *testing.T) {
	authErrors := []string{
		"Error response from daemon: pull access denied for acme/private, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
		"Error response from daemon: unauthorized: incorrect username or password",
		"unauthorized: authentication required",
		`Error response from daemon: Get "https://registry.example.com/v2/": no basic auth credentials`,
		"pull access denied, repository does not exist or may require authorization: server message: insufficient_scope: authorization failed",
		`failed to resolve reference "ghcr.io/acme/app:latest": failed to authorize: failed to fetch anonymous token: unexpected status: 401 Unauthorized`,
	}
	for _, msg := range authErrors {
		assert.True(t, isRegistryAuthError(errors.New(msg)), msg)
	}

	otherErrors := []string{
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
		"Error response from daemon: manifest for acme/app:latest not found: manifest unknown: manifest unknown",
		`Get "https://registry.example.com/v2/": dial tcp: lookup registry.example.com: no such host`,
		"context deadline exceeded",
		"open /var/lib/docker/tmp/foo: permission denied",
	}
	for _, msg := range otherErrors {
		assert.False(t, isRegistryAuthError(errors.New(msg)), msg)
	}

	assert.False(t, isRegistryAuthError(nil))
	assert.True(t, isRegistryAuthError(fmt.Errorf("pull: %w", unauthorizedStub{})))
}

func TestRegistryAuthError(t *testing.T) {
	cause := errors.New("no basic auth credentials")
	err := fmt.Errorf("%w: %w", ErrDeployFailed, &RegistryAuthError{Registry: "ghcr.io", Cause: cause})

	assert.ErrorIs(t, err, ErrPullFailed)

	var authErr *RegistryAuthError
	require.ErrorAs(t, err, &authErr)
	assert.Equal(t, "ghcr.io", authErr.Registry)
	assert.Equal(t, cause, authErr.Unwrap())
	assert.Equal(t, "Log in to ghcr.io first. This image can't be downloaded without registry credentials.", ErrorMessage(err))
}

// Helpers

type unauthorizedStub struct{}

func (unauthorizedStub) Error() string { return "the registry said no" }
func (unauthorizedStub) Unauthorized() {}
