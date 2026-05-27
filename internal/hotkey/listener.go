// Package hotkey provides a global low-level keyboard hook on Windows
// for detecting the Ctrl+Win combination press and release.
//
// Implementation: Win32 WH_KEYBOARD_LL hook installed on the OS thread
// that calls Run. The hook callback runs on that same thread, so any
// caller-provided callbacks (onPress, onRelease) MUST return quickly —
// Windows' default LowLevelHooksTimeout is 300 ms and the system will
// silently uninstall the hook if a callback exceeds it. Spawn goroutines
// for any non-trivial work.
package hotkey

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Win32 constants from winuser.h.
const (
	whKeyboardLL = 13
	hcAction     = 0

	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	wmQuit       = 0x0012

	vkLControl = 0xA2
	vkRControl = 0xA3
	vkLWin     = 0x5B
	vkRWin     = 0x5C
)

// KBDLLHOOKSTRUCT from winuser.h. lParam points to one of these inside
// the WH_KEYBOARD_LL callback.
type kbdLLHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// MSG from winuser.h, used by GetMessageW / DispatchMessageW.
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
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetWindowsHook     = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHook       = user32.NewProc("CallNextHookEx")
	procGetMessage         = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

// Listener detects the Ctrl+Win combination globally.
//
// NewListener creates one configured with press/release callbacks. Run
// installs the hook and blocks; Stop signals it to exit.
type Listener struct {
	onPress   func()
	onRelease func()

	threadID  atomic.Uint32
	started   chan struct{}
	startOnce sync.Once

	// State below is mutated only from the hook callback (single thread).
	ctrlDown bool
	winDown  bool
	bothDown bool
}

// NewListener returns a Listener that fires onPress when both Ctrl and
// Win are first held down together, and onRelease when either is then
// released. Either callback may be nil. Callbacks run on the hook
// thread and must return quickly.
func NewListener(onPress, onRelease func()) *Listener {
	return &Listener{
		onPress:   onPress,
		onRelease: onRelease,
		started:   make(chan struct{}),
	}
}

// Run installs the keyboard hook on the current OS thread, pumps the
// Windows message loop, and blocks until Stop is called from another
// goroutine. Run pins itself with runtime.LockOSThread internally.
func (l *Listener) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	callback := syscall.NewCallback(l.hookProc)
	hook, _, sysErr := procSetWindowsHook.Call(uintptr(whKeyboardLL), callback, 0, 0)
	if hook == 0 {
		return fmt.Errorf("SetWindowsHookExW failed: %v", sysErr)
	}
	defer procUnhookWindowsHook.Call(hook)

	tid, _, _ := procGetCurrentThreadID.Call()
	l.threadID.Store(uint32(tid))
	l.startOnce.Do(func() { close(l.started) })

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return fmt.Errorf("GetMessageW failed")
		case 0:
			return nil // WM_QUIT received
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Stop signals Run to return. It blocks briefly until Run has installed
// the hook (so calling Stop immediately after spawning Run is safe),
// then posts WM_QUIT to that thread.
func (l *Listener) Stop() {
	<-l.started
	procPostThreadMessage.Call(
		uintptr(l.threadID.Load()),
		uintptr(wmQuit),
		0, 0,
	)
}

func (l *Listener) hookProc(nCode uintptr, wParam uintptr, lParam uintptr) uintptr {
	if int32(nCode) == hcAction {
		// lParam is a real OS pointer delivered as uintptr. Read it as
		// unsafe.Pointer via a typed pointer to satisfy go vet's
		// unsafeptr check, which forbids direct uintptr→unsafe.Pointer.
		ptr := *(*unsafe.Pointer)(unsafe.Pointer(&lParam))
		kbd := (*kbdLLHookStruct)(ptr)
		l.handleKey(uint32(wParam), kbd.VkCode)
	}
	ret, _, _ := procCallNextHook.Call(0, nCode, wParam, lParam)
	return ret
}

func (l *Listener) handleKey(msgType, vk uint32) {
	switch msgType {
	case wmKeyDown, wmSysKeyDown:
		switch vk {
		case vkLControl, vkRControl:
			l.ctrlDown = true
		case vkLWin, vkRWin:
			l.winDown = true
		}
	case wmKeyUp, wmSysKeyUp:
		switch vk {
		case vkLControl, vkRControl:
			l.ctrlDown = false
		case vkLWin, vkRWin:
			l.winDown = false
		}
	}

	combo := l.ctrlDown && l.winDown
	switch {
	case combo && !l.bothDown:
		l.bothDown = true
		if l.onPress != nil {
			l.onPress()
		}
	case !combo && l.bothDown:
		l.bothDown = false
		if l.onRelease != nil {
			l.onRelease()
		}
	}
}
