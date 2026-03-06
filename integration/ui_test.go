package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/ui"
)

func TestUIInstallAndManageApp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-ui-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	proxyPorts := getProxyPorts(t)
	require.NoError(t, ns.Proxy().Boot(ctx, proxyPorts))

	app := ui.NewApp(ns, "")
	d := newAppDriver(t, app)
	d.start()

	d.send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// -- Screen 1: App list → select "Custom Docker image" --
	d.send(keyMsg("up"))
	d.send(keyMsg("enter"))

	d.waitForView("Image", 5*time.Second)

	// -- Screen 2: Image form --
	d.typeText("ghcr.io/basecamp/once-campfire:main")
	d.send(keyMsg("tab"))
	d.send(keyMsg("enter"))

	d.waitForView("Hostname", 5*time.Second)

	// -- Screen 3: Hostname form --
	d.typeText("chat.localhost")
	d.send(keyMsg("tab"))
	d.send(keyMsg("enter"))

	// -- Install activity → dashboard --
	// Wait for "running" which only appears in the dashboard panel state info,
	// not in the install form or activity views.
	d.waitForView("running", 5*time.Minute)
	assert.Contains(t, d.viewContent(), "chat.localhost")

	// Verify the app is reachable via HTTP
	appURL := fmt.Sprintf("http://chat.localhost:%d", proxyPorts.HTTPPort)
	resp, err := http.Get(appURL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 400,
		"expected 2xx/3xx, got %d", resp.StatusCode)

	// -- Stop via actions menu --
	d.send(keyMsg("a"))
	d.send(keyMsg("s"))
	d.waitForView("stopped", 30*time.Second)

	// -- Start via actions menu --
	d.send(keyMsg("a"))
	d.send(keyMsg("s"))
	d.waitForView("running", 30*time.Second)

	// -- Remove via actions menu --
	d.send(keyMsg("a"))
	d.send(keyMsg("r"))
	d.waitForView("Remove application and data?", 10*time.Second)
	d.send(keyMsg("tab"))
	d.send(keyMsg("enter"))
	d.waitForView("There are no applications installed", 30*time.Second)
}

// appDriver drives a ui.App outside of tea.Program by manually executing
// commands and feeding their results back through a message channel.
type appDriver struct {
	t     *testing.T
	app   *ui.App
	msgCh chan tea.Msg
}

func newAppDriver(t *testing.T, app *ui.App) *appDriver {
	return &appDriver{
		t:     t,
		app:   app,
		msgCh: make(chan tea.Msg, 256),
	}
}

// start enqueues the commands returned by App.Init, including the Docker
// event watcher and scrape timers.
func (d *appDriver) start() {
	d.enqueueCmd(d.app.Init())
}

// send flushes any pending messages, then delivers msg to App.Update and
// enqueues the returned command. All App.Update calls happen on the caller's
// goroutine, so there is no concurrent access.
func (d *appDriver) send(msg tea.Msg) {
	d.flush()
	_, cmd := d.app.Update(msg)
	d.enqueueCmd(cmd)
}

func (d *appDriver) typeText(text string) {
	for _, r := range text {
		d.send(keyMsg(string(r)))
	}
}

func (d *appDriver) viewContent() string {
	return d.app.View().Content
}

// waitForView processes messages from the channel until the app's view
// contains target, or until timeout.
func (d *appDriver) waitForView(target string, timeout time.Duration) {
	d.t.Helper()
	deadline := time.After(timeout)
	for {
		if strings.Contains(d.viewContent(), target) {
			return
		}
		select {
		case msg := <-d.msgCh:
			d.processMsg(msg)
		case <-deadline:
			d.t.Fatalf("timed out waiting for view to contain %q\n\nView:\n%s",
				target, d.viewContent())
		}
	}
}

// flush drains any immediately available messages from the channel.
func (d *appDriver) flush() {
	for {
		select {
		case msg := <-d.msgCh:
			d.processMsg(msg)
		default:
			return
		}
	}
}

func (d *appDriver) processMsg(msg tea.Msg) {
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, cmd := range batch {
			d.enqueueCmd(cmd)
		}
		return
	}
	_, cmd := d.app.Update(msg)
	d.enqueueCmd(cmd)
}

func (d *appDriver) enqueueCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if msg != nil {
			d.msgCh <- msg
		}
	}()
}

func keyMsg(s string) tea.KeyPressMsg {
	k := tea.Key{Text: s}
	if r := []rune(s); len(r) == 1 {
		k.Code = r[0]
	}
	return tea.KeyPressMsg(k)
}
