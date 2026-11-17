package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyPullImageReportsErrorsInTheStream(t *testing.T) {
	pullStream := func(lines ...string) *Proxy {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/images/create") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			for _, line := range lines {
				fmt.Fprintln(w, line)
			}
		}))
		t.Cleanup(server.Close)

		c, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(server.URL, "http://")), client.WithAPIVersion("1.47"))
		require.NoError(t, err)
		return NewProxy(&Namespace{name: "once", client: c})
	}

	t.Run("fails when the daemon reports an error after starting the pull", func(t *testing.T) {
		proxy := pullStream(
			`{"status":"Pulling from basecamp/kamal-proxy"}`,
			`{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}`,
		)

		err := proxy.pullImage(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest unknown")
	})

	t.Run("succeeds when the stream has no error", func(t *testing.T) {
		proxy := pullStream(
			`{"status":"Pulling from basecamp/kamal-proxy"}`,
			`{"status":"Status: Image is up to date"}`,
		)

		assert.NoError(t, proxy.pullImage(context.Background()))
	})
}

func TestProxyContainerName(t *testing.T) {
	ns := &Namespace{name: "once"}
	proxy := NewProxy(ns)
	assert.Equal(t, "once-proxy", proxy.containerName())

	ns2 := &Namespace{name: "staging"}
	proxy2 := NewProxy(ns2)
	assert.Equal(t, "staging-proxy", proxy2.containerName())
}

func TestDeployArgs(t *testing.T) {
	proxy := &Proxy{}

	t.Run("basic deploy includes timeout", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000"})

		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
		}, args)
	})

	t.Run("with host", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000", Host: "chat.example.com"})

		assert.Contains(t, args, "--host")
		assert.Contains(t, args, "chat.example.com")
	})

	t.Run("with TLS", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000", TLS: true})

		assert.Contains(t, args, "--tls")
	})

	t.Run("with host and TLS", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{
			AppName: "chat",
			Target:  "localhost:3000",
			Host:    "chat.example.com",
			TLS:     true,
		})

		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
			"--host", "chat.example.com",
			"--tls",
		}, args)
	})
}

func TestProxyImageIsPinnedByDigest(t *testing.T) {
	assert.Regexp(t, `^basecamp/kamal-proxy:v\d+\.\d+\.\d+@sha256:[0-9a-f]{64}$`, ProxyImage)
}

func TestProxyIsCurrent(t *testing.T) {
	proxy := NewProxy(&Namespace{name: "once"})

	inspect := func(image string) container.InspectResponse {
		return container.InspectResponse{Config: &container.Config{Image: image}}
	}

	assert.True(t, proxy.isCurrent(inspect(ProxyImage)))
	assert.False(t, proxy.isCurrent(inspect("basecamp/kamal-proxy:once-01")))
	assert.False(t, proxy.isCurrent(inspect("sha256:8179b0e60661cb97083d63152cf513bf08a4cb025f734a5a5fda2461b8ff0472")))
}

func TestSettingsFromConfig(t *testing.T) {
	t.Run("uses the settings stored on the container", func(t *testing.T) {
		stored := ProxySettings{HTTPPort: 8080, HTTPSPort: 8443, MetricsPort: 9090}
		settings, err := settingsFromConfig(&container.Config{Labels: map[string]string{labelKey: stored.Marshal()}})

		require.NoError(t, err)
		assert.Equal(t, stored, settings)
	})

	t.Run("fills in defaults for missing ports", func(t *testing.T) {
		settings, err := settingsFromConfig(&container.Config{Labels: map[string]string{labelKey: `{"httpPort":8080}`}})

		require.NoError(t, err)
		assert.Equal(t, ProxySettings{HTTPPort: 8080, HTTPSPort: DefaultHTTPSPort, MetricsPort: DefaultMetricsPort}, settings)
	})

	t.Run("uses defaults without a label", func(t *testing.T) {
		settings, err := settingsFromConfig(&container.Config{})

		require.NoError(t, err)
		assert.Equal(t, ProxySettings{HTTPPort: DefaultHTTPPort, HTTPSPort: DefaultHTTPSPort, MetricsPort: DefaultMetricsPort}, settings)
	})

	t.Run("rejects a malformed label", func(t *testing.T) {
		_, err := settingsFromConfig(&container.Config{Labels: map[string]string{labelKey: "{"}})

		assert.Error(t, err)
	})
}
