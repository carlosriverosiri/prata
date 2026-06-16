// Package ui provides minimal Win32 GUI feedback for the parts of Prata
// that run without a console. The release binary is built with
// -H windowsgui, so stdout/stderr are not visible; one-shot maintenance
// actions (--set-key now, --install/--uninstall later) report their outcome
// through a message box instead.
//
// This package is intentionally outside the dictation hot path: the daemon
// never calls it.
package ui

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

// Icon selects the message box icon and severity.
type Icon uint32

const (
	mbOK uint32 = 0x00000000 // MB_OK — single OK button

	// IconInfo marks a success/information message.
	IconInfo Icon = 0x00000040 // MB_ICONINFORMATION
	// IconError marks a failure message.
	IconError Icon = 0x00000010 // MB_ICONERROR
)

// MessageBox shows a modal Win32 message box with a single OK button. title
// and body are user-facing Swedish strings. The call is best-effort: if the
// strings cannot be converted to UTF-16 it silently does nothing, since there
// is no console to fall back to and no meaningful recovery.
func MessageBox(title, body string, icon Icon) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	bodyPtr, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return
	}
	procMessageBoxW.Call(
		0, // hWnd — no owner window
		uintptr(unsafe.Pointer(bodyPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|uint32(icon)),
	)
}
