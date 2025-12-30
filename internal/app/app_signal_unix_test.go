//go:build !windows
// +build !windows

package app

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VatsalSy/CloudPull/internal/config"
)

func TestAppSignalHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal handling test in short mode")
	}

	v := setupTestConfig(t)

	// Create config loader that uses our local viper instance
	configLoader := func() (*config.Config, error) {
		return config.LoadFromViper(v)
	}

	app, err := New(WithConfigLoader(configLoader))
	require.NoError(t, err)

	err = app.Initialize()
	require.NoError(t, err)

	// Create a context that the app will use
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use a ready channel to ensure signal handler is registered
	ready := make(chan struct{})

	// Start the app's signal handling in a goroutine
	go func() {
		// This will close ready when signal.Notify is called
		app.handleSignalsWithReady(cancel, ready)
	}()

	// Wait for signal handler to be registered
	<-ready

	// Send SIGINT to the current process
	err = syscall.Kill(os.Getpid(), syscall.SIGINT)
	require.NoError(t, err)

	// Wait for context to be canceled by the signal handler
	select {
	case <-ctx.Done():
		// Signal was handled correctly
	case <-time.After(2 * time.Second):
		t.Fatal("Signal handler did not cancel context within timeout")
	}

	// App should handle graceful shutdown
	err = app.Stop()
	assert.NoError(t, err)
}
