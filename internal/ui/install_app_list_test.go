package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallAppList_SelectKnownApp(t *testing.T) {
	list := NewInstallAppList()

	// First item (Campfire) is selected by default; press enter
	list, cmd := list.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	// Menu emits MenuSelectMsg; feed it back to get the app list's own message
	_, cmd = list.Update(cmd().(MenuSelectMsg))
	require.NotNil(t, cmd)

	msg := cmd()
	selected, ok := msg.(InstallAppSelectedMsg)
	require.True(t, ok, "expected InstallAppSelectedMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", selected.ImageRef)
}

func TestInstallAppList_SelectOther(t *testing.T) {
	list := NewInstallAppList()

	// Wrap up from first item to reach last item ("Custom Docker image")
	list, _ = list.Update(keyPressMsg("up"))
	list, cmd := list.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	_, cmd = list.Update(cmd().(MenuSelectMsg))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(InstallCustomSelectedMsg)
	assert.True(t, ok, "expected InstallCustomSelectedMsg, got %T", msg)
}

func TestInstallAppList_NavigateAndSelect(t *testing.T) {
	list := NewInstallAppList()

	// Navigate down to Fizzy
	list, _ = list.Update(keyPressMsg("down"))
	list, cmd := list.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	_, cmd = list.Update(cmd().(MenuSelectMsg))
	require.NotNil(t, cmd)

	msg := cmd()
	selected, ok := msg.(InstallAppSelectedMsg)
	require.True(t, ok, "expected InstallAppSelectedMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/fizzy:main", selected.ImageRef)
}

func TestInstallAppList_View(t *testing.T) {
	list := NewInstallAppList()
	view := list.View()

	assert.Contains(t, view, "Choose an application")
	assert.Contains(t, view, "Campfire")
	assert.Contains(t, view, "Fizzy")
	assert.Contains(t, view, "Writebook")
	assert.Contains(t, view, "Custom Docker image")
}

func TestExpandAlias(t *testing.T) {
	ref, ok := expandAlias("campfire")
	assert.True(t, ok)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", ref)

	ref, ok = expandAlias("fizzy")
	assert.True(t, ok)
	assert.Equal(t, "ghcr.io/basecamp/fizzy:main", ref)

	ref, ok = expandAlias("writebook")
	assert.True(t, ok)
	assert.Equal(t, "ghcr.io/basecamp/writebook", ref)

	ref, ok = expandAlias("ghcr.io/basecamp/once-campfire:latest")
	assert.False(t, ok)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire:latest", ref)
}
