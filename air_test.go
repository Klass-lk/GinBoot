package ginboot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAirConfig(t *testing.T) {
	t.Run("DefaultAirConfigTOML returns non-empty string", func(t *testing.T) {
		toml := DefaultAirConfigTOML()
		assert.Contains(t, toml, "[build]")
		assert.Contains(t, toml, "include_ext = [\"go\", \"tpl\", \"tmpl\", \"html\", \"yml\", \"yaml\", \"env\"]")
	})

	t.Run("EnsureAirConfig creates file when missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := EnsureAirConfig(tmpDir)
		require.NoError(t, err)

		airPath := filepath.Join(tmpDir, ".air.toml")
		assert.FileExists(t, airPath)

		content, err := os.ReadFile(airPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "include_ext = [\"go\", \"tpl\", \"tmpl\", \"html\", \"yml\", \"yaml\", \"env\"]")

		// Calling again when file already exists should not fail or overwrite
		err = EnsureAirConfig(tmpDir)
		require.NoError(t, err)
	})

	t.Run("EnsureAirConfig uses current dir when dir is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		origWd, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(origWd) }()

		require.NoError(t, os.Chdir(tmpDir))

		err = EnsureAirConfig("")
		require.NoError(t, err)
		assert.FileExists(t, ".air.toml")
	})
}
