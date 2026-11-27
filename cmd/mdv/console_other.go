//go:build !windows

package main

import (
	"os"
	"strings"
)

const openDialogOnEmpty = false

// guiLaunch reports whether we are running as mdv.app/Contents/MacOS/mdv, where
// Finder delivers the document as an Apple Event instead of argv and starting
// with no file is legitimate.
func guiLaunch() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

func detachConsole() {}

func showError(string) {}
