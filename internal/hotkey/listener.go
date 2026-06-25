// Package hotkey provides global push-to-talk (F1) and dictionary
// quick-fix (F8) hotkeys for Windows using RegisterHotKey.
//
// Unlike the previous WH_KEYBOARD_LL implementation, there is no
// OS-enforced callback timeout, no hook invalidation across sleep/resume
// cycles, and no keylogger-pattern AV/EDR signature.
//
// F1 (PTT) is registered unconditionally; F8 is registered only when a
// handler has been provided via SetOnF8, preserving the previous semantics:
// without a handler, F8 is not registered and passes through untouched
// globally. A failed F8 registration is non-fatal (soft-degrade with a
// warning to stderr).
//
// A failed F1 registration is NOT fatal. F1 is a single system-wide hotkey,
// so another program that already owns it (a macro tool, a game overlay, a
// leftover Prata, a boot-time race) blocks Prata's registration. Instead of
// exiting, the listener stays alive, reports the contention via the
// SetOnF1State "unavailable" callback (so the daemon can cue and show a
// persistent "F1 UPPTAGEN" tray state), and re-probes RegisterHotKey(F1) on a
// timer. The moment F1 frees up — typically when the offending program closes
// — it registers and fires the "recovered" callback, all without a restart.
// This is the "see and forget" behaviour: a clinician closes the conflicting
// program and dictation resumes by itself.
//
// Thread model: Run pins itself to one OS thread. RegisterHotKey is
// thread-bound and WM_HOTKEY is posted to the registering thread's message
// queue, so registration (including every retry), the GetMessageW loop, and
// UnregisterHotKey all run on that same locked thread. The retry timer lives
// in a helper goroutine that only PostThreadMessageW's a private wake message
// to the loop thread; the actual RegisterHotKey retry runs on the loop thread.
// Physical-release detection runs in lightweight goroutines that poll
// GetAsyncKeyState at 20 ms; all goroutines are signalled and fully awaited
// before Run returns, so no callback ever fires after Run exits.
//
// Callbacks (onPress, onRelease, onF8, onF1Unavailable, onF1Recovered) must
// return quickly to keep the message loop responsive.
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
	wmUser      = 0x0400
	modNoRepeat = 0x4000 // MOD_NOREPEAT

	// wmRetryF1 is a private thread message the retry helper posts to the
	// message-loop thread to ask it to re-attempt RegisterHotKey(F1) while F1
	// is held by another program. Any value at/above WM_USER works.
	wmRetryF1 = wmUser + 1

	// vkPTT is the virtual-key code for the push-to-talk key (F1).
	// An env-var override is deferred until a real Fn-layer problem
	// surfaces on a mini-PC keyboard; see ADR 2026-06-09 in
	// PRATA-DESIGN-LOG.md.
	vkPTT = 0x70 // VK_F1

	vkF8 = 0x77 // VK_F8
)

// f1RetryInterval is how often the listener re-probes RegisterHotKey(F1) while
// F1 is unavailable. Short enough that dictation resumes within a few seconds
// of the conflicting program closing, long enough that the probe is free.
const f1RetryInterval = 3 * time.Second

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
	onPress         func()
	onRelease       func()
	onF8            func()
	onF1Unavailable func()
	onF1Recovered   func()

	threadID  atomic.Uint32
	started   chan struct{}
	startOnce sync.Once

	// f1Held and f8Held guard against duplicate WM_HOTKEY deliveries
	// while a press is already being tracked (possible with certain
	// keyboard firmware or when the message loop is briefly busy).
	f1Held atomic.Int32
	f8Held atomic.Int32

	// The fields below are touched only on the message-loop thread (Run and
	// the handlers it calls inline), so they need no synchronisation.
	f1Registered bool
	f8Registered bool
	stopRetry    chan struct{} // created/closed on the loop thread; stops the retry helper on recovery
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

// SetOnF1State registers callbacks for F1 availability, both fired on the
// message-loop thread. unavailable fires once when F1 cannot be registered
// (another program owns it) — the daemon should cue and show a persistent
// degraded state. recovered fires once when a later retry succeeds — the
// daemon should clear that state. Either may be nil. Like the other
// callbacks they must return quickly. MUST be called before Run, for the
// same happens-before reason as SetOnF8.
func (l *Listener) SetOnF1State(unavailable, recovered func()) {
	l.onF1Unavailable = unavailable
	l.onF1Recovered = recovered
}

// Run registers the hotkeys on the current OS thread, pumps the Windows
// message loop, and blocks until Stop is called from another goroutine.
// It pins itself with runtime.LockOSThread internally.
//
// F1 registration failure is NOT fatal: Run enters a degraded state
// (onF1Unavailable) and re-probes on a timer until F1 is free (onF1Recovered),
// staying alive throughout. F8 registration failure is a silent soft-degrade.
// Run returns a non-nil error only for a genuine message-loop system failure.
func (l *Listener) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Guarantee started is closed on every return path so Stop is never
	// deadlocked on <-l.started.
	defer l.startOnce.Do(func() { close(l.started) })

	tid, _, _ := procGetCurrentThreadID.Call()
	l.threadID.Store(uint32(tid))
	// Publish readiness before registering F1: with self-heal, F1 may be
	// unavailable for a while, but Stop must still be able to post WM_QUIT
	// during that degraded phase.
	l.startOnce.Do(func() { close(l.started) })

	// Unregister whatever ends up registered, after all goroutines exit.
	// Registration can now happen on a retry (not just up front), so clean up
	// by flag rather than a defer set at each registration site. Defer order
	// (LIFO): close(quit) -> wg.Wait() -> cleanupHotkeys() -> close(started).
	defer l.cleanupHotkeys()

	// quit signals all goroutines (release polls and the retry helper) to
	// exit without firing callbacks; wg blocks Run until they have, so no
	// callback fires after Run returns — critical because main.go closes the
	// events channel immediately after <-listenerDone.
	quit := make(chan struct{})
	var wg sync.WaitGroup
	defer wg.Wait()
	defer close(quit)

	// F8 is independent of F1 (a different key); register it once up front,
	// soft-degrading on failure, regardless of F1's state.
	l.registerF8()

	// F1: try once; if another program owns it, go degraded and keep retrying
	// on a timer instead of exiting.
	if !l.tryRegisterF1() {
		l.enterDegraded(tid, quit, &wg)
	}

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case -1:
			return fmt.Errorf("GetMessageW: system error")
		case 0: // WM_QUIT
			return nil
		}
		switch m.Message {
		case wmHotKey:
			l.handleHotKey(m.WParam, quit, &wg)
		case wmRetryF1:
			l.handleRetryF1()
		}
	}
}

// registerF8 registers F8 when a handler is set, soft-degrading on failure.
// Runs on the loop thread.
func (l *Listener) registerF8() {
	if l.onF8 == nil {
		return
	}
	ret, _, sysErr := procRegisterHotKey.Call(0, 2, modNoRepeat, vkF8)
	if ret == 0 {
		fmt.Fprintf(os.Stderr, "warn: F8 quick-fix disabled (%v)\n", sysErr)
		return
	}
	l.f8Registered = true
}

// tryRegisterF1 attempts to claim F1 and records success. Runs on the loop
// thread; returns false when another program already owns F1.
func (l *Listener) tryRegisterF1() bool {
	ret, _, _ := procRegisterHotKey.Call(0, 1, modNoRepeat, vkPTT)
	if ret == 0 {
		return false
	}
	l.f1Registered = true
	return true
}

// enterDegraded reports F1 contention and starts the retry helper. Runs on the
// loop thread. The helper only posts wmRetryF1 to this thread on a timer; the
// actual re-registration happens in handleRetryF1 on the loop thread, because
// RegisterHotKey is bound to the thread that pumps the message loop.
func (l *Listener) enterDegraded(tid uintptr, quit <-chan struct{}, wg *sync.WaitGroup) {
	fmt.Fprintln(os.Stderr, "F1 unavailable (held by another program); will retry")
	l.stopRetry = make(chan struct{})
	if l.onF1Unavailable != nil {
		l.onF1Unavailable()
	}
	stop := l.stopRetry // captured so the helper never reads the field
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(f1RetryInterval)
		defer tick.Stop()
		for {
			select {
			case <-quit:
				return
			case <-stop:
				return
			case <-tick.C:
				procPostThreadMessage.Call(tid, uintptr(wmRetryF1), 0, 0)
			}
		}
	}()
}

// handleRetryF1 re-attempts F1 registration on a wmRetryF1 wake. Runs on the
// loop thread. On success it stops the retry helper and fires onF1Recovered;
// a stale wake after recovery is a no-op.
func (l *Listener) handleRetryF1() {
	if l.f1Registered {
		return // already recovered; stale wake
	}
	if !l.tryRegisterF1() {
		return // still held; wait for the next wake
	}
	fmt.Fprintln(os.Stderr, "F1 recovered; dictation active")
	if l.stopRetry != nil {
		close(l.stopRetry)
		l.stopRetry = nil
	}
	if l.onF1Recovered != nil {
		l.onF1Recovered()
	}
}

// cleanupHotkeys unregisters whatever is currently registered. Runs on the
// loop thread at Run return, after all goroutines have exited.
func (l *Listener) cleanupHotkeys() {
	if l.f1Registered {
		procUnregisterHotKey.Call(0, 1)
	}
	if l.f8Registered {
		procUnregisterHotKey.Call(0, 2)
	}
}

// Stop signals Run to return. It blocks briefly until Run has published its
// thread id (so calling Stop immediately after spawning Run is safe), then
// posts WM_QUIT to the loop thread. Safe whether Run is still in the F1-retry
// degraded phase, running normally, or has already returned.
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
