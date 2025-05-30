//go:build windows
// +build windows

package app

import (
	"os"
	"os/signal"
)

// setupSignalHandling sets up signal handling for Windows
func (app *App) setupSignalHandling(sigChan chan os.Signal) {
	// Windows only supports os.Interrupt (SIGINT)
	signal.Notify(sigChan, os.Interrupt)
}