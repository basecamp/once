package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameFromImageRef(t *testing.T) {
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire:main"))
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire"))
	assert.Equal(t, "nginx", NameFromImageRef("nginx:latest"))
	assert.Equal(t, "nginx", NameFromImageRef("nginx"))
}

func TestBuildEnvWithSMTP(t *testing.T) {
	settings := ApplicationSettings{
		SMTP: SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
			From:     "noreply@example.com",
		},
	}

	env := settings.BuildEnv("test-secret-key")

	assert.Contains(t, env, "SMTP_ADDRESS=smtp.example.com")
	assert.Contains(t, env, "SMTP_PORT=587")
	assert.Contains(t, env, "SMTP_USERNAME=user@example.com")
	assert.Contains(t, env, "SMTP_PASSWORD=secret")
	assert.Contains(t, env, "MAILER_FROM_ADDRESS=noreply@example.com")
}

func TestBuildEnvWithoutSMTP(t *testing.T) {
	settings := ApplicationSettings{}

	env := settings.BuildEnv("test-secret-key")

	for _, e := range env {
		assert.NotContains(t, e, "SMTP_")
	}
}

func TestContainerResourcesEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 512}}

	differentCPUs := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 2, MemoryMB: 512}}
	assert.False(t, base.Equal(differentCPUs))

	differentMemory := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 1024}}
	assert.False(t, base.Equal(differentMemory))

	zeroResources := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(zeroResources))
}

func TestContainerResourcesMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:      "app",
		Image:     "img:latest",
		Resources: ContainerResources{CPUs: 2, MemoryMB: 512},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, 2, restored.Resources.CPUs)
	assert.Equal(t, 512, restored.Resources.MemoryMB)
	assert.True(t, original.Equal(restored))
}

func TestAutoUpdateEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", AutoUpdate: false}
	different := ApplicationSettings{Name: "app", AutoUpdate: true}
	assert.False(t, base.Equal(different))
}

func TestBackupSettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBack: true}}

	differentPath := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/other", AutoBack: true}}
	assert.False(t, base.Equal(differentPath))

	differentAutoBack := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBack: false}}
	assert.False(t, base.Equal(differentAutoBack))

	noBackup := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(noBackup))
}

func TestAutoUpdateAndBackupMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:       "app",
		Image:      "img:latest",
		AutoUpdate: true,
		Backup:     BackupSettings{Path: "/backups", AutoBack: true},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.True(t, restored.AutoUpdate)
	assert.Equal(t, "/backups", restored.Backup.Path)
	assert.True(t, restored.Backup.AutoBack)
	assert.True(t, original.Equal(restored))
}

func TestBackupToFile_EmptyDir(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "chat"}}
	err := app.BackupToFile(context.Background(), "", "backup.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup location is required")
}

func TestBackupToFile_RelativePath(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "chat"}}
	err := app.BackupToFile(context.Background(), "relative/path", "backup.tar.gz")
	require.ErrorIs(t, err, ErrBackupPathRelative)
}

func TestBackupToFile_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "backup", "dir")

	_, _, err := prepareBackupDir(dir)
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestBackup_EmptyPath(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "chat"}}
	err := app.Backup(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup location is required")
}

func TestVerifyHTTP_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
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

func TestBackupName(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{Name: "myapp"},
	}

	name := app.BackupName()
	assert.Contains(t, name, "myapp-")
	assert.True(t, len(name) > len("myapp-.tar.gz"))
	assert.Contains(t, name, ".tar.gz")

	// Verify the embedded timestamp is parseable
	_, ok := parseBackupTime("myapp", name)
	assert.True(t, ok)
}

func TestParseBackupTime(t *testing.T) {
	t.Run("valid backup name", func(t *testing.T) {
		ts, ok := parseBackupTime("myapp", "myapp-20250115-093000.tar.gz")
		require.True(t, ok)
		assert.Equal(t, time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC), ts)
	})

	t.Run("wrong app name", func(t *testing.T) {
		_, ok := parseBackupTime("other", "myapp-20250115-093000.tar.gz")
		assert.False(t, ok)
	})

	t.Run("unrelated file", func(t *testing.T) {
		_, ok := parseBackupTime("myapp", "unrelated.txt")
		assert.False(t, ok)
	})

	t.Run("bad date", func(t *testing.T) {
		_, ok := parseBackupTime("myapp", "myapp-baddate.tar.gz")
		assert.False(t, ok)
	})
}

func TestTrimBackups(t *testing.T) {
	dir := t.TempDir()

	app := &Application{
		Settings: ApplicationSettings{
			Name:   "myapp",
			Backup: BackupSettings{Path: dir},
		},
	}

	oldTime := time.Now().Add(-BackupRetention - time.Hour)
	recentTime := time.Now().Add(-time.Minute)

	createFile := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644))
	}

	oldFile := fmt.Sprintf("myapp-%s.tar.gz", oldTime.Format("20060102-150405"))
	recentFile := fmt.Sprintf("myapp-%s.tar.gz", recentTime.Format("20060102-150405"))
	unrelatedFile := "notes.txt"

	createFile(oldFile)
	createFile(recentFile)
	createFile(unrelatedFile)

	err := app.TrimBackups()
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, oldFile))
	assert.FileExists(t, filepath.Join(dir, recentFile))
	assert.FileExists(t, filepath.Join(dir, unrelatedFile))
}
