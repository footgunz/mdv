package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// x/sys/windows has no wrappers for these, so bind them lazily from the
// system directory (NewLazySystemDLL never searches the app/current dir).
var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
)

// openDialogOnEmpty: with no document on the command line, a GUI launch pops
// the native open-file dialog instead of leaving the user at the placeholder.
const openDialogOnEmpty = true

// guiLaunch reports whether we were started from Explorer (double-click,
// "Open with", a file association, a shortcut) rather than from a terminal.
// mdv.exe is a console-subsystem binary, so Windows creates a fresh console
// for it in that case; we are then the only process attached to it. From a
// shell the console is shared with cmd/pwsh, so the count is at least 2.
func guiLaunch() bool {
	var pids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}

// detachConsole drops the console Windows created for an Explorer launch, so
// only the viewer window remains. The console is destroyed once its last
// process detaches. Only called on the GUI path; stderr is gone afterwards.
func detachConsole() {
	_, _, _ = procFreeConsole.Call()
}

// showError surfaces a fatal message when there is no console to read it in
// (GUI launch only). The console Windows made for us would just sit empty
// behind the box, so drop it first.
func showError(msg string) {
	detachConsole()
	text, err := windows.UTF16PtrFromString(msg)
	if err != nil {
		return
	}
	caption, _ := windows.UTF16PtrFromString("mdv")
	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR)
}
