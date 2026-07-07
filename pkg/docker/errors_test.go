package docker

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPortConflict(t *testing.T) {
	assert.True(t, isPortConflict(errors.New("Ports are not available: listen tcp :80: bind: address already in use")))
	assert.True(t, isPortConflict(errors.New("driver failed programming external connectivity: port is already allocated")))
	assert.False(t, isPortConflict(errors.New("something else went wrong")))
	assert.False(t, isPortConflict(nil))
}

func TestErrorMessage(t *testing.T) {
	t.Run("returns description for described error", func(t *testing.T) {
		assert.Equal(t, ErrProxyPortInUse.Description(), ErrorMessage(ErrProxyPortInUse))
	})

	t.Run("returns description for wrapped described error", func(t *testing.T) {
		wrapped := fmt.Errorf("setup failed: %w", ErrProxyPortInUse)
		assert.Equal(t, ErrProxyPortInUse.Description(), ErrorMessage(wrapped))
	})

	t.Run("returns Error for plain error", func(t *testing.T) {
		err := errors.New("something broke")
		assert.Equal(t, "something broke", ErrorMessage(err))
	})
}
