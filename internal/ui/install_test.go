package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/basecamp/once/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_KnownAppFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, installStateAppList, m.state)

	// Select Campfire (first item, already selected)
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateHostname, m.state)

	// Enter hostname and submit
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_CustomImageFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Select "Other image..."
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	assert.Equal(t, installStateImageForm, m.state)

	// Submit image
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	assert.Equal(t, installStateHostname, m.state)

	// Submit hostname
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_CLIModeSkipsToHostname(t *testing.T) {
	m := NewInstall(newTestNamespace(), "campfire")
	assert.Equal(t, installStateHostname, m.state)
}

func TestInstall_CLIModeExpandsAlias(t *testing.T) {
	m := NewInstall(newTestNamespace(), "campfire")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	view := ansi.Strip(m.View())
	assert.Contains(t, view, "once-campfire.example.com")
	assert.Contains(t, view, "Installing campfire")
}

func TestInstall_InteractiveModeHasNoTitle(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	view := ansi.Strip(m.View())
	assert.NotContains(t, view, "Installing")
}

func TestInstall_SubmitTriggersActivity(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_SuccessNavigatesToApp(t *testing.T) {
	m := newTestInstall()
	app := &docker.Application{}

	_, cmd := updateInstall(m, InstallActivityDoneMsg{App: app})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToAppMsg)
	require.True(t, ok, "expected NavigateToAppMsg, got %T", msg)
	assert.Equal(t, app, navMsg.App)
}

func TestInstall_FailureReturnsToHostname(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Go through known app flow to hostname
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	// Simulate failure
	installErr := errors.New("connection refused")
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: installErr})

	assert.Equal(t, installStateHostname, m.state)
	assert.Equal(t, installErr, m.err)
	assert.Contains(t, m.View(), "connection refused")
}

func TestInstall_ErrorClearsOnKeypress(t *testing.T) {
	m := newTestInstall()
	m.state = installStateHostname
	m.hostnameForm = NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "")
	m.err = errors.New("some error")

	m, _ = updateInstall(m, keyPressMsg("a"))
	assert.Nil(t, m.err)
}

func TestInstall_BackNavigation_AppListEscNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
}

func TestInstall_BackNavigation_ImageFormEscGoesToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	assert.Equal(t, installStateImageForm, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_HostnameEscGoesToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_HostnameEscGoesToImageForm(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	assert.Equal(t, installStateHostname, m.state)
	assert.True(t, m.customImage)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateImageForm, m.state)
}

func TestInstall_BackNavigation_HostnameBackMsgKnownApp(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})

	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_HostnameBackMsgCustomImage(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})

	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	assert.Equal(t, installStateImageForm, m.state)
}

func TestInstall_BackNavigation_ImageFormBackMsg(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})

	m, _ = updateInstall(m, InstallImageBackMsg{})
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_EscQuitsInCLIMode(t *testing.T) {
	m := NewInstall(newTestNamespace(), "ghcr.io/basecamp/once-campfire:latest")

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(QuitMsg)
	assert.True(t, ok, "expected QuitMsg, got %T", msg)
}

func TestInstall_HostnameBackQuitsInCLIMode(t *testing.T) {
	m := NewInstall(newTestNamespace(), "ghcr.io/basecamp/once-campfire:latest")

	_, cmd := updateInstall(m, InstallHostnameBackMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(QuitMsg)
	assert.True(t, ok, "expected QuitMsg, got %T", msg)
}

func TestInstall_ShowsLogoAndHidesTitleWhenNoApps(t *testing.T) {
	m := NewInstall(newTestNamespace(), "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	view := m.View()
	assert.Contains(t, view, "██████╗")
	assert.NotContains(t, view, "ONCE · install")
}

func TestInstall_ShowsTitleAndHidesLogoWhenAppsExist(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "myapp"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	view := m.View()
	assert.NotContains(t, view, "██████╗")
	assert.Contains(t, view, "ONCE · install")
}

func TestInstall_ShowLogo(t *testing.T) {
	noApps := newTestNamespace()
	withApps := newTestNamespace(docker.ApplicationSettings{Name: "myapp"})

	assert.True(t, NewInstall(noApps, "").showLogo())
	assert.False(t, NewInstall(withApps, "").showLogo())

	t.Setenv("ONCE_REDUCED_MOTION", "true")
	assert.False(t, NewInstall(noApps, "").showLogo())
	assert.False(t, NewInstall(withApps, "").showLogo())
}

func TestInstall_PullFailureReturnsToImageForm(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "bad:image"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "bad:image", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	pullErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateImageForm, m.state)
	assert.Equal(t, pullErr, m.err)
}

func TestInstall_PullFailureReturnsToAppList(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	pullErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateAppList, m.state)
	assert.Equal(t, pullErr, m.err)
}

func TestInstall_NonPullDeployFailureReturnsToHostname(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})

	deployErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, errors.New("container crashed"))
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: deployErr})
	assert.Equal(t, installStateHostname, m.state)
}

func TestInstall_HostnameInUseBlocksInstall(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "myapp", Host: "taken.example.com"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "taken.example.com"})
	assert.Equal(t, installStateHostname, m.state)
	assert.ErrorIs(t, m.err, docker.ErrHostnameInUse)
}

func TestInstall_UniqueHostnameAllowsInstall(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "myapp", Host: "taken.example.com"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "unique.example.com"})
	assert.Equal(t, installStateActivity, m.state)
	assert.Nil(t, m.err)
}

func TestInstall_FailureDoesNotRestartLogo(t *testing.T) {
	ns := newTestNamespace()
	noApps := NewInstall(ns, "")
	noApps, _ = updateInstall(noApps, tea.WindowSizeMsg{Width: 80, Height: 40})
	noApps, _ = updateInstall(noApps, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest", Hostname: "app.example.com"})
	_, cmd := updateInstall(noApps, InstallActivityFailedMsg{Err: errors.New("fail")})
	assert.Nil(t, cmd)
}

func TestInstall_HelpKeyShownOnHostnameScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// App list: no F1
	view := ansi.Strip(m.View())
	assert.NotContains(t, view, "F1")

	// Image form: no F1
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	view = ansi.Strip(m.View())
	assert.NotContains(t, view, "F1")

	// Hostname: F1 shown
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	view = ansi.Strip(m.View())
	assert.Contains(t, view, "F1")
	assert.Contains(t, view, "help")
}

func TestInstall_HelpKeyShownOnHostnameScreenNonFirstRun(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "campfire"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Navigate to hostname screen
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "F1")
	assert.Contains(t, view, "help")
}

func TestOverlayBlockContainsRow(t *testing.T) {
	block := newOverlayBlock("hello\nworld", 5, 10)
	assert.Equal(t, 5, block.width) // "hello" and "world" are both 5 chars

	// Row before block
	_, ok := block.containsRow(4)
	assert.False(t, ok)

	// First row
	line, ok := block.containsRow(5)
	assert.True(t, ok)
	assert.Equal(t, "hello", line)

	// Second row
	line, ok = block.containsRow(6)
	assert.True(t, ok)
	assert.Equal(t, "world", line)

	// Row after block
	_, ok = block.containsRow(7)
	assert.False(t, ok)
}

func TestOverlayBlockPadsShortLines(t *testing.T) {
	block := newOverlayBlock("hi\nworld", 0, 0)
	assert.Equal(t, 5, block.width) // max("hi", "world") = 5

	line, ok := block.containsRow(0)
	assert.True(t, ok)
	assert.Equal(t, "hi   ", line) // "hi" padded to width 5
}

func TestBlockWidth(t *testing.T) {
	lines := []string{"short", "a longer line", "mid"}
	assert.Equal(t, 13, blockWidth(lines))
}

func TestBlockWidthEmpty(t *testing.T) {
	assert.Equal(t, 0, blockWidth(nil))
}

// Helpers

func newTestInstall() Install {
	return NewInstall(newTestNamespace(), "")
}

func newTestNamespace(apps ...docker.ApplicationSettings) *docker.Namespace {
	ns, err := docker.NewNamespace("test", docker.WithApplications(apps...))
	if err != nil {
		panic(err)
	}
	return ns
}

func updateInstall(m Install, msg tea.Msg) (Install, tea.Cmd) {
	comp, cmd := m.Update(msg)
	return comp.(Install), cmd
}
