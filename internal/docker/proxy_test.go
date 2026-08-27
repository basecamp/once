package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProxyContainerName(t *testing.T) {
	ns := &Namespace{name: "once"}
	proxy := NewProxy(ns)
	assert.Equal(t, "once-proxy", proxy.containerName())

	ns2 := &Namespace{name: "staging"}
	proxy2 := NewProxy(ns2)
	assert.Equal(t, "staging-proxy", proxy2.containerName())
}

func TestProxySettingsWithDefaults(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		settings := ProxySettings{}.withDefaults()

		assert.Equal(t, DefaultHTTPPort, settings.HTTPPort)
		assert.Equal(t, DefaultHTTPSPort, settings.HTTPSPort)
		assert.Equal(t, DefaultMetricsPort, settings.MetricsPort)
	})

	t.Run("explicit values are preserved", func(t *testing.T) {
		settings := ProxySettings{HTTPPort: 8080, HTTPSPort: 8443, MetricsPort: 9090}.withDefaults()

		assert.Equal(t, ProxySettings{HTTPPort: 8080, HTTPSPort: 8443, MetricsPort: 9090}, settings)
	})
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
