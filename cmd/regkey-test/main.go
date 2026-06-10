// Command regkey-test is a canary test for the proposed RegisterHotKey
// architecture (replacing WH_KEYBOARD_LL with RegisterHotKey + polling loop).
//
// # Test protocol
//
//  1. Open Notepad (or a browser tab) and give it keyboard focus.
//  2. Run this program in a separate terminal (normal user, not elevated).
//  3. Press F1 while Notepad is focused: Notepad's built-in help MUST NOT
//     appear. This terminal MUST print "F1 PRESS" then "F1 RELEASE (held N ms)".
//  4. Press F9 while a browser is focused: the browser MUST NOT react
//     (e.g. no address-bar focus in Firefox). MUST print "F9 TAP".
//  5. Press Ctrl+C here. Press F1 in Notepad again — help MUST reappear,
//     confirming that UnregisterHotKey released the key correctly.
//
// If the target app still sees the F-key while WM_HOTKEY is delivered here,
// Windows' documented consumption guarantee is defeated in this environment
// (likely cause: elevated process, accessibility tool, or PowerToys claiming
// the same key).
package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Win32 constants from winuser.h.
const (
	wmHotKey    = 0x0312
	wmQuit      = 0x0012
	modNoRepeat = 0x4000 // MOD_NOREPEAT — suppress auto-repeat WM_HOTKEY delivery
	vkF1        = 0x70
	vkF9        = 0x78
)

// msg mirrors the Win32 MSG structure (winuser.h).
type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type point struct {
	X, Y int32
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessage         = user32.NewProc("GetMessageW")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

// f1Held is 1 while an F1 press is being tracked. Guards against duplicate
// WM_HOTKEY events that can arrive with certain keyboard firmware.
var f1Held atomic.Int32

func main() {
	// threadReady receives the OS thread ID on success, or 0 on registration
	// failure (Windows thread IDs are never 0, so 0 is a safe sentinel).
	threadReady := make(chan uint32, 1)
	loopDone := make(chan error, 1)
	go runLoop(threadReady, loopDone)

	tid := <-threadReady
	if tid == 0 {
		<-loopDone // errors already printed by runLoop
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "regkey-test: F1 och F9 registrerade. Testa i Notepad/webbläsare. Ctrl+C avslutar.")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	select {
	case <-sigs:
		postQuit(tid)
		if err := <-loopDone; err != nil {
			fmt.Fprintf(os.Stderr, "regkey-test: loop error: %v\n", err)
			os.Exit(1)
		}
	case err := <-loopDone:
		if err != nil {
			fmt.Fprintf(os.Stderr, "regkey-test: %v\n", err)
			os.Exit(1)
		}
	}
}

// runLoop pins itself to one OS thread. RegisterHotKey is thread-bound and
// WM_HOTKEY is posted to the registering thread's queue, so both registration
// and the GetMessageW loop must live on the same locked thread. It unregisters
// both hotkeys before returning, regardless of exit reason.
func runLoop(threadReady chan<- uint32, done chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadID.Call()

	ret, _, sysErr := procRegisterHotKey.Call(0, 1, modNoRepeat, vkF1)
	if ret == 0 {
		fmt.Fprintf(os.Stderr, "regkey-test: RegisterHotKey F1: %v\n", sysErr)
		threadReady <- 0
		done <- fmt.Errorf("RegisterHotKey F1: %w", sysErr)
		return
	}

	ret, _, sysErr = procRegisterHotKey.Call(0, 2, modNoRepeat, vkF9)
	if ret == 0 {
		procUnregisterHotKey.Call(0, 1)
		fmt.Fprintf(os.Stderr, "regkey-test: RegisterHotKey F9: %v\n", sysErr)
		threadReady <- 0
		done <- fmt.Errorf("RegisterHotKey F9: %w", sysErr)
		return
	}

	threadReady <- uint32(tid)

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case -1:
			procUnregisterHotKey.Call(0, 1)
			procUnregisterHotKey.Call(0, 2)
			done <- fmt.Errorf("GetMessageW: system error")
			return
		case 0: // WM_QUIT
			procUnregisterHotKey.Call(0, 1)
			procUnregisterHotKey.Call(0, 2)
			done <- nil
			return
		}
		if m.Message == wmHotKey {
			handleHotKey(m.WParam)
		}
	}
}

// handleHotKey dispatches a WM_HOTKEY event. For F1 it starts a goroutine
// that polls GetAsyncKeyState every 20 ms until the physical key is released —
// this is the polling loop the production code will use instead of waiting for
// a second WM_HOTKEY (which RegisterHotKey with MOD_NOREPEAT never delivers).
func handleHotKey(id uintptr) {
	switch id {
	case 1: // F1
		if !f1Held.CompareAndSwap(0, 1) {
			return // already tracking a press
		}
		fmt.Println("F1 PRESS")
		pressedAt := time.Now()
		go func() {
			for {
				time.Sleep(20 * time.Millisecond)
				ret, _, _ := procGetAsyncKeyState.Call(vkF1)
				// High bit (0x8000) is set while the key is physically down.
				// Works whether GetAsyncKeyState sign-extends or zero-extends
				// the SHORT return value into the uintptr register.
				if ret&0x8000 == 0 {
					ms := time.Since(pressedAt).Milliseconds()
					fmt.Printf("F1 RELEASE (held %d ms)\n", ms)
					f1Held.Store(0)
					return
				}
			}
		}()
	case 2: // F9
		fmt.Println("F9 TAP")
	}
}

func postQuit(tid uint32) {
	procPostThreadMessage.Call(uintptr(tid), uintptr(wmQuit), 0, 0)
}
