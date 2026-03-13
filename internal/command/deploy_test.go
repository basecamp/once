package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvVars(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		d := &deployCommand{}
		result, err := d.parseEnvVars()
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("valid pairs", func(t *testing.T) {
		d := &deployCommand{env: []string{"FOO=bar", "BAZ=qux"}}
		result, err := d.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"FOO": "bar", "BAZ": "qux"}, result)
	})

	t.Run("value containing equals", func(t *testing.T) {
		d := &deployCommand{env: []string{"DSN=postgres://host?opt=val"}}
		result, err := d.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "postgres://host?opt=val", result["DSN"])
	})

	t.Run("missing equals", func(t *testing.T) {
		d := &deployCommand{env: []string{"INVALID"}}
		_, err := d.parseEnvVars()
		assert.ErrorContains(t, err, "must be in KEY=VALUE format")
	})

	t.Run("empty key", func(t *testing.T) {
		d := &deployCommand{env: []string{"=value"}}
		_, err := d.parseEnvVars()
		assert.ErrorContains(t, err, "key must not be empty")
	})

	t.Run("empty value is valid", func(t *testing.T) {
		d := &deployCommand{env: []string{"KEY="}}
		result, err := d.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "", result["KEY"])
	})

	t.Run("duplicate keys last wins", func(t *testing.T) {
		d := &deployCommand{env: []string{"KEY=first", "KEY=second"}}
		result, err := d.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "second", result["KEY"])
	})
}
