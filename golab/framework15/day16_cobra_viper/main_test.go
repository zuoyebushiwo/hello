package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func execute(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestDefaults(t *testing.T) {
	got := execute(t, "serve")
	require.Contains(t, got, "listen=127.0.0.1:8080")
}

func TestConfigFileAndEnvironmentAndFlagPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: 0.0.0.0\n  port: 7000\nlog:\n  level: debug\n"), 0o600))

	t.Setenv("APP_SERVER_PORT", "7100")
	got := execute(t, "serve", "--config", path, "--server.port", "7200")
	require.Contains(t, got, "listen=0.0.0.0:7200")
	require.Contains(t, got, "level=debug")
}

func TestConfigFileCanSetAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: 10.0.0.1\n  port: 9000\ndb:\n  dsn: postgres://db/app\n"), 0o600))

	got := execute(t, "serve", "--config", path)
	require.Contains(t, got, "listen=10.0.0.1:9000")
	require.Contains(t, got, "dsn=postgres://db/app")
}

func TestMissingConfigReturnsError(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"serve", "--config", filepath.Join(t.TempDir(), "missing.yaml")})
	require.Error(t, cmd.Execute())
}
