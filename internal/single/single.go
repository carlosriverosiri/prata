// Package single provides a Windows named-mutex single-instance guard.
// A tray app like Prata must never run in two copies: two instances
// share the F1 hotkey and would both capture audio and inject,
// producing duplicate output (or garbled output in async apps).
package single

import (
	"syscall"
	"unsafe"
)

const errorAlreadyExists = 183 // ERROR_ALREADY_EXISTS

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
)

// held keeps the mutex handle for the process lifetime. We never close
// it; the OS releases the named mutex automatically on process exit.
var held uintptr

// Acquire takes a session-scoped named single-instance lock. It returns
// true if this process is the first/only holder, false if another
// instance already holds the lock. A plain (unprefixed) name is scoped
// to the current logon session, which is exactly what we want: it
// blocks a second instance in the same user session (autostart + a
// manual run, or two dev terminals).
func Acquire(name string) bool {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return true // bad name shouldn't block startup
	}

	handle, _, callErr := procCreateMutexW.Call(
		0,                                // default security attributes
		0,                                // not initially owned
		uintptr(unsafe.Pointer(namePtr)), // mutex name
	)
	if handle == 0 {
		return true // CreateMutexW failed; don't block startup
	}
	held = handle

	// CreateMutexW sets last error to ERROR_ALREADY_EXISTS when the
	// named mutex already existed (another instance created it first).
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		return false
	}
	return true
}
