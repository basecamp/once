package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestWithApplicationFound(t *testing.T) {
	ns := &docker.Namespace{}
	ns.AddApplication(docker.ApplicationSettings{Name: "myapp"})

	var called bool
	err := withApplication(ns, "myapp", "testing", func(app *docker.Application) error {
		called = true
		assert.Equal(t, "myapp", app.Settings.Name)
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestWithApplicationNotFound(t *testing.T) {
	ns := &docker.Namespace{}

	err := withApplication(ns, "missing", "testing", func(app *docker.Application) error {
		t.Fatal("should not be called")
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `application "missing" not found`)
}

func TestWithApplicationError(t *testing.T) {
	ns := &docker.Namespace{}
	ns.AddApplication(docker.ApplicationSettings{Name: "myapp"})

	err := withApplication(ns, "myapp", "starting", func(app *docker.Application) error {
		return assert.AnError
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "starting application")
}
