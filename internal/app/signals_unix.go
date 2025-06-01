//go:build !windows
// +build !windows

package app

import (
	"os"
	"os/signal"
	"syscall"
)

// setupSignalHandling sets up signal handling for Unix-like systems.
func (app *App) setupSignalHandling(sigChan chan os.Signal) {
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
}
