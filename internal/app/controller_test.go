package app_test

import (
	"path/filepath"
	"testing"

	zopapp "github.com/peterwwillis/zop/internal/app"
	"github.com/peterwwillis/zop/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerDefaults(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	_, err := config.EnsureConfigFile(configPath)
	require.NoError(t, err)

	controller, err := zopapp.NewController(configPath, "mobile", "")
	require.NoError(t, err)

	assert.NotEmpty(t, controller.ActiveAgent())
	assert.Equal(t, configPath, controller.ConfigPath())
	assert.NotEmpty(t, controller.AgentNames())
	_ = controller.MissingAPIKeyWarning()
}

func TestControllerSetAgent(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	_, err := config.EnsureConfigFile(configPath)
	require.NoError(t, err)

	controller, err := zopapp.NewController(configPath, "mobile", "")
	require.NoError(t, err)

	err = controller.SetAgent("does-not-exist")
	assert.Error(t, err)
}
