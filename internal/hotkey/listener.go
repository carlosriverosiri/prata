// Package hotkey provides global push-to-talk (F1) and dictionary
// quick-fix (F8) hotkeys for Windows using RegisterHotKey.
//
// Unlike the previous WH_KEYBOARD_LL implementation, there is no
// OS-enforced callback timeout, no hook invalidation across sleep/resume
// cycles, and no keylogger-pattern AV/EDR signature.
//
// F1 (PTT) is registered unconditionally. F8 is registered only when a
// handler has been provided via SetOnF8, preserving the semantics of the
// previous implementation: without a handler, F8 is not registered and
// passes through untouched globally. A failed F8 registration is
// non-fatal (soft-degrade with a warning to stderr); a failed F1
// registration is fatal.
//
// Thread model: Run pins itself to one OS thread. RegisterHotKey is
// thread-bound and WM_HOTKEY is posted to the registering thread's
// message queue, so registration, the GetMessageW loop, and
// UnregisterHotKey all run on that same locked thread. Physical-release
// detection runs in lightweight goroutines that poll GetAsyncKeyState at
// 20 ms; they are signalled and fully awaited before Run returns, so no
// callback ever fires after Run exits.
//
// Callbacks (onPress, onRelease, onF8) must return quickly to keep the
// message loop responsive. The 300 ms LowLevelHooksTimeout death penalty
// of the previous hook implementation no longer applies.
package hotkey

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Win32 constants from winuser.h.
const (
	wmHotKey    = 0x0312
	wmQuit      = 0x0012
	modNoRepeat = 0x4000 // MOD_NOREPEAT

	// vkPTT is the virtual-key code for the push-to-talk key (F1).
	// An env-var override is deferred until a real Fn-layer problem
	// surfaces on a mini-PC keyboard; see ADR 2026-06-09 in
	// PRATA-DESIGN-LOG.md.
	vkPTT = 0x70 // VK_F1

	vkF8 = 0x77 // VK_F8
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

// Listener detects the push-to-talk (F1) and dictionary quick-fix (F8)
// hotkeys globally.
//
// NewListener creates one configured with press/release callbacks. Run
// registers the hotkeys and blocks; Stop signals it to exit.
type Listener struct {
	onPress   func()
	onRelease func()
	onF8      func()

	threadID  atomic.Uint32
	started   chan struct{}
	startOnce sync.Once

	// f1Held and f8Held guard against duplicate WM_HOTKEY deliveries
	// while a press is already being tracked (possible with certain
	// keyboard firmware or when the message loop is briefly busy).
	f1Held atomic.Int32
	f8Held atomic.Int32
}

// NewListener returns a Listener that fires onPress when F1 is first
// pressed and onRelease when it is released. Either callback may be nil.
// Callbacks must return quickly to keep the message loop responsive.
func NewListener(onPress, onRelease func()) *Listener {
	return &Listener{
		onPress:   onPress,
		onRelease: onRelease,
		started:   make(chan struct{}),
	}
}

// SetOnF8 registers a callback that fires once per F8 tap, on the
// physical key-up transition. While a callback is registered, F8 is
// claimed as a system hotkey so the foreground app never sees the key;
// without a handler, F8 is not registered and passes through untouched
// globally.
//
// SetOnF8 MUST be called before Run. The field is read on the goroutine
// that calls Run before the message loop starts, so setting it before
// that goroutine is started gives the required happens-before guarantee.
// Like the PTT callbacks, onF8 must return quickly.
func (l *Listener) SetOnF8(cb func()) {
	l.onF8 = cb
}

// Run registers the hotkeys on the current OS thread, pumps the Windows
// message loop, and blocks until Stop is called from another goroutine.
// It pins itself with runtime.LockOSThread internally.
//
// F1 registration failure returns a non-nil error. F8 registration
// failure is non-fatal: a warning is written to stderr and Run continues
// with only F1 registered.
func (l *Listener) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Guarantee started is closed on every return path (including
	// registration failure) so Stop is never deadlocked on <-l.started.
	// On the success path startOnce.Do fires explicitly below after
	// threadID is stored; this defer is a no-op for that path.
	defer l.startOnce.Do(func() { close(l.started) })

	tid, _, _ := procGetCurrentThreadID.Call()

	// Register F1 (PTT) — fatal: without PTT the app has no purpose.
	ret, _, sysErr := procRegisterHotKey.Call(0, 1, modNoRepeat, vkPTT)
	if ret == 0 {
		return fmt.Errorf("RegisterHotKey F1: %w", sysErr)
	}
	defer procUnregisterHotKey.Call(0, 1)

	// Register F8 (quick-fix) — only when a handler is set; soft-degrade
	// on failure (same policy as the dictionary and tray icon in main.go).
	// Without a handler, F8 is not registered and passes through untouched.
	if l.onF8 != nil {
		ret, _, sysErr = procRegisterHotKey.Call(0, 2, modNoRepeat, vkF8)
		if ret == 0 {
			fmt.Fprintf(os.Stderr, "warn: F8 quick-fix disabled (%v)\n", sysErr)
		} else {
			defer procUnregisterHotKey.Call(0, 2)
		}
	}

	// quit signals poll goroutines to exit without firing callbacks.
	// wg blocks Run until all goroutines have exited, guaranteeing that
	// no callback fires after Run returns — critical because main.go
	// closes the events channel immediately after <-listenerDone.
	quit := make(chan struct{})
	var wg sync.WaitGroup
	defer wg.Wait()
	defer close(quit)

	l.threadID.Store(uint32(tid))
	l.startOnce.Do(func() { close(l.started) })

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case -1:
			return fmt.Errorf("GetMessageW: system error")
		case 0: // WM_QUIT
			return nil
		}
		if m.Message == wmHotKey {
			l.handleHotKey(m.WParam, quit, &wg)
		}
	}
}

// Stop signals Run to return. It blocks briefly until Run has registered
// the hotkeys (so calling Stop immediately after spawning Run is safe),
// then posts WM_QUIT to the loop thread.
//
// Stop is safe to call if Run returned with a registration error (threadID
// is 0 — the message loop never started) or if Run has already returned.
func (l *Listener) Stop() {
	<-l.started
	if l.threadID.Load() == 0 {
		return // Run never reached the message loop
	}
	procPostThreadMessage.Call(
		uintptr(l.threadID.Load()),
		uintptr(wmQuit),
		0, 0,
	)
}

// handleHotKey dispatches a WM_HOTKEY event received by the message loop.
//
// F1 (PTT): the atomic guard fires first to prevent duplicate handling,
// then onPress, then a poll goroutine that detects the physical release
// via GetAsyncKeyState and calls onRelease.
//
// F8 (quick-fix): the atomic guard fires first, then a poll goroutine
// waits for the physical key release before calling onF8 — preserving
// the key-up-firing semantics so F8 is not physically held when f8Worker
// later synthesizes Ctrl+C/Ctrl+V.
//
// Both goroutines respect quit: on WM_QUIT they exit without calling any
// callback. Run waits for all goroutines via wg before returning, so no
// callback ever fires after Run exits.
//
// Benign edge: if F1 is physically held during shutdown, evPress is sent
// but evRelease is never sent. The audio.Session is abandoned, but the
// process exits cleanly.
func (l *Listener) handleHotKey(id uintptr, quit <-chan struct{}, wg *sync.WaitGroup) {
	switch id {
	case 1: // F1 (PTT)
		if !l.f1Held.CompareAndSwap(0, 1) {
			return // press already being tracked
		}
		if l.onPress != nil {
			l.onPress()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer l.f1Held.Store(0)
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-quit:
					return // shutdown: do not call onRelease
				case <-tick.C:
				}
				ret, _, _ := procGetAsyncKeyState.Call(vkPTT)
				if ret&0x8000 == 0 {
					if l.onRelease != nil {
						l.onRelease()
					}
					return
				}
			}
		}()

	case 2: // F8 quick-fix
		if !l.f8Held.CompareAndSwap(0, 1) {
			return // tap already in flight
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer l.f8Held.Store(0)
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-quit:
					return // shutdown: do not call onF8
				case <-tick.C:
				}
				ret, _, _ := procGetAsyncKeyState.Call(vkF8)
				if ret&0x8000 == 0 {
					// Fire on physical key-up so F8 is not held
					// when f8Worker later synthesizes Ctrl+C/Ctrl+V.
					if l.onF8 != nil {
						l.onF8()
					}
					return
				}
			}
		}()
	}
}
