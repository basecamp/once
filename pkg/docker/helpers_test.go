package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNameFromImageRef(t *testing.T) {
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire:main"))
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire"))
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire:latest"))
}

func TestIsLocalhost(t *testing.T) {
	assert.True(t, IsLocalhost("localhost"))
	assert.True(t, IsLocalhost("app.localhost"))
	assert.False(t, IsLocalhost("example.com"))
	assert.False(t, IsLocalhost("localhost.example.com"))
}

func TestCutLast(t *testing.T) {
	before, after, found := cutLast("once-app-myapp-abc123", "-")
	assert.True(t, found)
	assert.Equal(t, "once-app-myapp", before)
	assert.Equal(t, "abc123", after)

	before, after, found = cutLast("nosep", "-")
	assert.False(t, found)
	assert.Equal(t, "nosep", before)
	assert.Equal(t, "", after)
}
