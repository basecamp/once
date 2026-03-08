package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUniqueName(t *testing.T) {
	ns := &Namespace{name: "test"}

	name, err := ns.UniqueName("myapp")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(name, "myapp."))
	assert.Len(t, name, len("myapp.")+6)

	name2, err := ns.UniqueName("myapp")
	require.NoError(t, err)
	assert.NotEqual(t, name, name2)
}

func TestApplicationLookup(t *testing.T) {
	ns := &Namespace{name: "test"}
	ns.applications = append(ns.applications, NewApplication(ns, ApplicationSettings{Name: "alpha"}))
	ns.applications = append(ns.applications, NewApplication(ns, ApplicationSettings{Name: "beta"}))

	assert.NotNil(t, ns.Application("alpha"))
	assert.Equal(t, "alpha", ns.Application("alpha").Settings.Name)
	assert.NotNil(t, ns.Application("beta"))
	assert.Nil(t, ns.Application("missing"))
}

func TestHostInUse(t *testing.T) {
	ns := &Namespace{name: "test"}
	ns.applications = append(ns.applications,
		NewApplication(ns, ApplicationSettings{Name: "app1", Host: "app1.localhost"}),
		NewApplication(ns, ApplicationSettings{Name: "app2", Host: "app2.localhost"}),
	)

	assert.True(t, ns.HostInUse("app1.localhost"))
	assert.True(t, ns.HostInUse("app2.localhost"))
	assert.False(t, ns.HostInUse("other.localhost"))
}

func TestHostInUseByAnother(t *testing.T) {
	ns := &Namespace{name: "test"}
	ns.applications = append(ns.applications,
		NewApplication(ns, ApplicationSettings{Name: "app1", Host: "shared.localhost"}),
		NewApplication(ns, ApplicationSettings{Name: "app2", Host: "unique.localhost"}),
	)

	assert.False(t, ns.HostInUseByAnother("shared.localhost", "app1"))
	assert.True(t, ns.HostInUseByAnother("shared.localhost", "app2"))
	assert.False(t, ns.HostInUseByAnother("other.localhost", "app1"))
}

func TestContainerAppName(t *testing.T) {
	ns := &Namespace{name: "once"}

	t.Run("standard app", func(t *testing.T) {
		assert.Equal(t, "campfire", ns.containerAppName("once-app-campfire-a1b2c3"))
	})

	t.Run("dotted unique name", func(t *testing.T) {
		assert.Equal(t, "campfire.a1b2c3", ns.containerAppName("once-app-campfire.a1b2c3-d4e5f6"))
	})

	t.Run("dashed app name", func(t *testing.T) {
		assert.Equal(t, "my-app", ns.containerAppName("once-app-my-app-abcdef"))
	})

	t.Run("wrong namespace", func(t *testing.T) {
		assert.Equal(t, "", ns.containerAppName("other-app-campfire-a1b2c3"))
	})

	t.Run("not a container name", func(t *testing.T) {
		assert.Equal(t, "", ns.containerAppName("something-else"))
	})

	t.Run("no ID suffix", func(t *testing.T) {
		assert.Equal(t, "", ns.containerAppName("once-app-campfire"))
	})
}
