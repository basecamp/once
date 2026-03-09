package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyHTTP_Success(t *testing.T) {
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, HealthCheckPath, requestPath)
}

func TestVerifyHTTP_RedirectToSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/home", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
}

func TestVerifyHTTP_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationFailed)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestVerifyHTTP_Unreachable(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{Host: "127.0.0.1:1", DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationFailed)
}

func TestVerifyHTTP_NoHost(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
}

func TestURL(t *testing.T) {
	newAppWithProxy := func(host string, disableTLS bool, proxySettings *ProxySettings) *Application {
		ns := &Namespace{}
		ns.proxy = &Proxy{Settings: proxySettings}
		return &Application{
			namespace: ns,
			Settings:  ApplicationSettings{Host: host, DisableTLS: disableTLS},
		}
	}

	t.Run("empty host", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{}}
		assert.Equal(t, "", app.URL())
	})

	t.Run("nil namespace", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{Host: "app.example.com"}}
		assert.Equal(t, "https://app.example.com", app.URL())
	})

	t.Run("nil proxy settings", func(t *testing.T) {
		ns := &Namespace{}
		ns.proxy = &Proxy{}
		app := &Application{
			namespace: ns,
			Settings:  ApplicationSettings{Host: "app.localhost", DisableTLS: true},
		}
		assert.Equal(t, "http://app.localhost", app.URL())
	})

	t.Run("default HTTP port", func(t *testing.T) {
		app := newAppWithProxy("app.localhost", true, &ProxySettings{HTTPPort: 80})
		assert.Equal(t, "http://app.localhost", app.URL())
	})

	t.Run("custom HTTP port", func(t *testing.T) {
		app := newAppWithProxy("app.localhost", true, &ProxySettings{HTTPPort: 8080})
		assert.Equal(t, "http://app.localhost:8080", app.URL())
	})

	t.Run("default HTTPS port", func(t *testing.T) {
		app := newAppWithProxy("app.example.com", false, &ProxySettings{HTTPSPort: 443})
		assert.Equal(t, "https://app.example.com", app.URL())
	})

	t.Run("custom HTTPS port", func(t *testing.T) {
		app := newAppWithProxy("app.example.com", false, &ProxySettings{HTTPSPort: 8443})
		assert.Equal(t, "https://app.example.com:8443", app.URL())
	})

	t.Run("localhost disables TLS", func(t *testing.T) {
		app := newAppWithProxy("chat.localhost", false, &ProxySettings{HTTPPort: 9090})
		assert.Equal(t, "http://chat.localhost:9090", app.URL())
	})
}
