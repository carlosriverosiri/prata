// Package hotkey provides a global low-level keyboard hook on Windows
// for detecting the Ctrl+Win combination (press and release) and F9 taps.
//
// Implementation: Win32 WH_KEYBOARD_LL hook installed on the OS thread
// that calls Run. The hook callback runs on that same thread, so any
// caller-provided callbacks (onPress, onRelease, onF9) MUST return
// quickly — Windows' default LowLevelHooksTimeout is 300 ms and the
// system will silently uninstall the hook if a callback exceeds it.
// Spawn goroutines for any non-trivial work.
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
	vkF9       = 0x78

	// llkhfInjected (LLKHF_INJECTED) marks events synthesized by SendInput,
	// e.g. our own Ctrl+C / Ctrl+V / Unicode input from internal/inject. The
	// hook passes these through untouched so they reach the target app but
	// are never interpreted as user input by our state machine.
	llkhfInjected = 0x10
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
	onF9      func()

	threadID  atomic.Uint32
	started   chan struct{}
	startOnce sync.Once

	// State below is mutated only from the hook callback (single thread).
	ctrlDown bool
	winDown  bool
	bothDown bool
	f9Down   bool
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

// SetOnF9 registers a callback that fires once per F9 tap, on the F9
// key-up transition. While a callback is registered, both the F9 key-down
// and key-up are swallowed so the foreground app never sees the key; a
// Listener with no F9 handler leaves F9 untouched.
//
// SetOnF9 MUST be called before Run. The field is read on the hook thread
// without synchronization, so setting it before the goroutine that calls
// Run is started gives the same happens-before guarantee that NewListener
// relies on for onPress/onRelease. Like those, onF9 runs on the hook
// thread and must return within ~300 ms, so spawn a goroutine for any
// real work.
func (l *Listener) SetOnF9(cb func()) {
	l.onF9 = cb
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
		// Pass injected events straight through, before any Ctrl+Win or F9
		// logic: our own synthesized input (Ctrl+C/Ctrl+V/Unicode) must
		// reach the target app but must never be read as a hotkey here.
		if kbd.Flags&llkhfInjected != 0 {
			ret, _, _ := procCallNextHook.Call(0, nCode, wParam, lParam)
			return ret
		}
		if l.handleKey(uint32(wParam), kbd.VkCode) {
			// Returning nonzero without chaining to the next hook
			// swallows the event so it never reaches the foreground app.
			return 1
		}
	}
	ret, _, _ := procCallNextHook.Call(0, nCode, wParam, lParam)
	return ret
}

// handleKey updates key state and fires callbacks. It returns true when
// the event should be swallowed instead of passed to the foreground app,
// which happens only for F9 while an onF9 callback is registered.
func (l *Listener) handleKey(msgType, vk uint32) bool {
	if vk == vkF9 {
		if l.onF9 == nil {
			return false // no handler: leave F9 untouched for the app
		}
		switch msgType {
		case wmKeyDown, wmSysKeyDown:
			l.f9Down = true
		case wmKeyUp, wmSysKeyUp:
			// Fire on the key-up transition so F9 is no longer physically
			// held when the callback later synthesizes input. The f9Down
			// guard collapses auto-repeat into one fire per tap.
			if l.f9Down {
				l.f9Down = false
				l.onF9()
			}
		}
		return true // swallow both down and up
	}

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
	return false
}
