// Package tray puts a Prata icon in the Windows notification area (system
// tray) with a right-click menu whose only item is "Avsluta" (Quit).
//
// Implementation mirrors internal/hotkey: a hidden Windows message pump runs
// on a single OS thread pinned with runtime.LockOSThread. Run registers a
// window class, creates a hidden top-level window (never shown — not an
// HWND_MESSAGE window, so SetForegroundWindow + TrackPopupMenu behave),
// builds an HICON from raw .ico bytes, adds the tray icon, and pumps the
// message loop until Stop is called from another goroutine. A failed initial
// add is non-fatal: the icon is (re-)added when the shell broadcasts
// TaskbarCreated (the shell becoming ready, or Explorer restarting).
//
// The onQuit callback fires on the message-loop thread when the user clicks
// Avsluta. Like the hotkey callbacks it MUST return quickly — do any real
// shutdown work in a goroutine. onQuit must not call Stop: the tray already
// posts WM_QUIT to unwind its own loop.
package tray

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Win32 constants from winuser.h and shellapi.h.
const (
	wmQuit        = 0x0012
	wmNull        = 0x0000
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	wmUser        = 0x0400

	// callbackMsg is the private message Shell_NotifyIcon sends to our
	// window for tray mouse events. Any value in the WM_USER range works.
	callbackMsg = wmUser + 1

	// balloonMsg is the private message Notify posts (from any goroutine) to
	// the message-loop thread so the balloon is shown by the thread that owns
	// the icon. Distinct from callbackMsg.
	balloonMsg = wmUser + 2

	// AppendMenuW / TrackPopupMenu / CheckMenuRadioItem flags.
	mfString       = 0x0000
	mfSeparator    = 0x0800
	mfByCommand    = 0x0000
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	tpmNoNotify    = 0x0080

	// Shell_NotifyIconW messages and NOTIFYICONDATAW flags.
	nimAdd     = 0x0000
	nimModify  = 0x0001
	nimDelete  = 0x0002
	nifMessage = 0x0001
	nifIcon    = 0x0002
	nifTip     = 0x0004
	nifInfo    = 0x0010

	// NIIF_INFO: show the info-level (i) glyph on the balloon.
	niifInfo = 0x0001

	// CreateIconFromResourceEx requires this version word for .ico image
	// bits; a wrong value makes the call fail.
	iconVer = 0x00030000

	// GetSystemMetrics index for the recommended small-icon width.
	smCXSmIcon = 49

	// Menu command ids. Must be non-zero: TrackPopupMenu with TPM_RETURNCMD
	// returns 0 when dismissed without a choice.
	idQuit        = 1
	idCheckUpdate = 2

	// idBackendBase is the first command id for the backend radio items;
	// item i has id idBackendBase+i. Kept clear of idQuit/idCheckUpdate.
	idBackendBase = 100

	// dpiPerMonitorAwareV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 from
	// winuser.h: the pseudo-handle value -4 passed to
	// SetProcessDpiAwarenessContext.
	dpiPerMonitorAwareV2 = ^uintptr(0) - 3
)

// msg mirrors the Win32 MSG struct for GetMessageW / DispatchMessageW.
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type point struct {
	x, y int32
}

// wndClassExW mirrors the Win32 WNDCLASSEXW struct for RegisterClassExW.
type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

// notifyIconData mirrors the modern Win32 NOTIFYICONDATAW struct. The full
// layout is used so cbSize matches the current shell version on Windows
// 10/11.
type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procUnregisterClassW       = user32.NewProc("UnregisterClassW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW     = user32.NewProc("PostThreadMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procCheckMenuRadioItem     = user32.NewProc("CheckMenuRadioItem")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procCreateIconFromResource = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon            = user32.NewProc("DestroyIcon")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")

	// Windows 10 1703+ only; guard calls with Find (see SetProcessDPIAware).
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

// Tray owns a single notification-area icon and its right-click menu.
//
// New configures it; Run installs the icon and blocks on the message loop;
// Stop signals it to exit from another goroutine.
type Tray struct {
	iconICO       []byte
	tooltip       string
	onQuit        func()
	onCheckUpdate func() // optional; when set, adds a "check for update" menu item

	// Backend selector: when backendNames is non-empty, the menu shows a
	// radio item per name above the other items, with activeBackend bulleted.
	// Set before Run (same happens-before rule as onCheckUpdate). After Run,
	// activeBackend is only touched on the message-loop thread (showMenu).
	backendNames    []string
	activeBackend   int
	onSelectBackend func(int) // fired with the chosen index on a deliberate switch

	threadID  atomic.Uint32
	hwnd      atomic.Uintptr // hidden window handle, set in Run; target for Notify's posted message
	started   chan struct{}
	startOnce sync.Once

	// balloon* hold the pending balloon text. Notify (any goroutine) writes
	// them under balloonMu and posts balloonMsg; the message-loop thread
	// reads them under the same lock and shows the balloon.
	balloonMu    sync.Mutex
	balloonTitle string
	balloonText  string

	// The fields below are created in Run and then only touched on the
	// message-loop thread (Run itself and the WndProc it dispatches), so
	// they need no synchronisation.
	menu              uintptr        // popup menu, created once and reused
	nid               notifyIconData // tray icon data, reused for add/delete
	added             bool           // whether NIM_ADD has succeeded
	taskbarCreatedMsg uint32         // id of the "TaskbarCreated" broadcast
}

// New returns a Tray that shows iconICO (raw bytes of an .ico file) with the
// given tooltip. onQuit fires when the user clicks Avsluta; it may be nil.
// onQuit runs on the message-loop thread and must return quickly.
func New(iconICO []byte, tooltip string, onQuit func()) *Tray {
	return &Tray{
		iconICO: iconICO,
		tooltip: tooltip,
		onQuit:  onQuit,
		started: make(chan struct{}),
	}
}

// SetOnCheckUpdate registers a callback for a "Sök efter uppdatering…" menu
// item. The item is added to the right-click menu only when a callback is
// set, so harnesses that don't need it (cmd/tray-test) keep just Avsluta.
//
// SetOnCheckUpdate MUST be called before Run: the menu is built in Run, and
// the field is read on the message-loop thread when the menu opens. Setting
// it before the goroutine that calls Run is started gives the required
// happens-before guarantee. Like onQuit, the callback runs on the
// message-loop thread and MUST return quickly — do the network check on its
// own goroutine and report the result via Notify.
func (t *Tray) SetOnCheckUpdate(cb func()) {
	t.onCheckUpdate = cb
}

// SetBackends configures the backend radio items shown at the top of the
// right-click menu, with active pre-selected (bulleted). The active name is
// also appended to the tooltip ("Prata — Rngv GPU-server"). Like SetOnCheckUpdate it
// MUST be called before Run: the menu and tooltip are built in Run, and the
// fields are read on the message-loop thread.
func (t *Tray) SetBackends(names []string, active int) {
	t.backendNames = names
	if active >= 0 && active < len(names) {
		t.activeBackend = active
	}
}

// SetOnSelectBackend registers a callback fired (on the message-loop thread)
// when the user picks a different backend item. Like onQuit it MUST return
// quickly — do any disk/network work on its own goroutine. The index maps
// into the names passed to SetBackends.
func (t *Tray) SetOnSelectBackend(cb func(int)) {
	t.onSelectBackend = cb
}

// SetProcessDPIAware opts the current process into per-monitor DPI awareness
// (v2) so Windows reports DPI-scaled metrics and the tray icon is built at
// the real on-screen size instead of an unscaled 16px that the shell would
// blur upward. Call it once, before any window or HICON is created.
//
// The result is intentionally ignored (it fails harmlessly if awareness was
// already set), and the Find guard skips the call entirely on Windows older
// than 1703 — where the API is absent — instead of panicking. cmd/prata can
// reuse this exact call; it is safe there because inject uses virtual-key
// input, not screen coordinates.
func SetProcessDPIAware() {
	if procSetProcessDpiAwarenessContext.Find() != nil {
		return
	}
	procSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2)
}

// Run registers the window class, creates a hidden window, adds the tray
// icon, and pumps the Windows message loop until Stop is called. It pins
// itself to its OS thread with runtime.LockOSThread.
func (t *Tray) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Safety net: guarantee Stop never deadlocks on <-t.started even if we
	// fail before the explicit success signal below. sync.Once dedupes with
	// that signal; the defer alone would be too late — it fires on Run's
	// return (after the message loop), so Stop would block for the whole run.
	defer t.startOnce.Do(func() { close(t.started) })

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, err := syscall.UTF16PtrFromString("PrataTrayWindow")
	if err != nil {
		return fmt.Errorf("class name: %w", err)
	}

	wc := wndClassExW{
		lpfnWndProc:   syscall.NewCallback(t.wndProc),
		hInstance:     hInstance,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	atom, _, sysErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW failed: %v", sysErr)
	}
	defer func() { procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance) }()

	windowName, _ := syscall.UTF16PtrFromString("Prata")
	hwnd, _, sysErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0, // dwStyle: not WS_VISIBLE — the window is never shown
		0, 0, 0, 0,
		0, // hWndParent: none (a hidden top-level window, not HWND_MESSAGE)
		0, // hMenu
		hInstance,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %v", sysErr)
	}
	defer procDestroyWindow.Call(hwnd)
	// Published so Notify can PostMessageW to this window from another
	// goroutine; cleared on teardown so a late Notify is a harmless no-op.
	t.hwnd.Store(hwnd)
	defer t.hwnd.Store(0)

	hIcon, err := t.buildIcon()
	if err != nil {
		return err
	}
	defer procDestroyIcon.Call(hIcon)

	t.menu, err = t.buildMenu()
	if err != nil {
		return err
	}
	defer procDestroyMenu.Call(t.menu)

	// Register for the shell's TaskbarCreated broadcast so the icon can be
	// (re-)added once the shell is ready and again after an Explorer restart.
	// The hidden window is top-level, so it receives the broadcast.
	tcName, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	tcID, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(tcName)))
	t.taskbarCreatedMsg = uint32(tcID)

	t.nid = notifyIconData{
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: callbackMsg,
		hIcon:            hIcon,
	}
	t.nid.cbSize = uint32(unsafe.Sizeof(t.nid))
	putUTF16(t.nid.szTip[:], t.tooltipText())

	// Best-effort initial add: at login the shell may not be ready yet, so a
	// failure is non-fatal — we keep pumping and (re-)add on TaskbarCreated.
	t.addIcon()
	// Closure (not a bare deferred Call) so t.nid stays reachable for the GC
	// until cleanup; remove the icon only if it was ever added.
	defer func() {
		if t.added {
			procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
		}
	}()

	tid, _, _ := procGetCurrentThreadID.Call()
	t.threadID.Store(uint32(tid))
	t.startOnce.Do(func() { close(t.started) })

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case -1:
			return fmt.Errorf("GetMessageW failed")
		case 0:
			return nil // WM_QUIT received
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Stop signals Run to return. It blocks until Run has reached its start
// signal (so calling Stop right after launching Run is safe), then posts
// WM_QUIT to the message-loop thread.
func (t *Tray) Stop() {
	<-t.started
	procPostThreadMessageW.Call(uintptr(t.threadID.Load()), wmQuit, 0, 0)
}

// addIcon issues NIM_ADD for the tray icon and records success. It runs for
// the initial add and again on the TaskbarCreated broadcast; both happen on
// the message-loop thread, so t.added needs no synchronisation.
func (t *Tray) addIcon() {
	if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid))); r != 0 {
		t.added = true
	}
}

// wndProc handles tray callback and shell messages on the message-loop thread
// and defers everything else to DefWindowProcW.
func (t *Tray) wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch {
	case uint32(message) == callbackMsg:
		switch uint32(lParam) & 0xffff {
		case wmRButtonUp, wmContextMenu:
			t.showMenu(hwnd)
		}
		return 0
	case uint32(message) == balloonMsg:
		// Notify posted this from another goroutine; show the stashed text.
		t.balloonMu.Lock()
		title, text := t.balloonTitle, t.balloonText
		t.balloonMu.Unlock()
		t.showBalloon(title, text)
		return 0
	case t.taskbarCreatedMsg != 0 && uint32(message) == t.taskbarCreatedMsg:
		// The shell (re)started: (re-)add the icon so it appears/reappears.
		t.addIcon()
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return ret
}

// showMenu pops the reused right-click menu at the cursor and acts on the
// choice. The SetForegroundWindow call before and the WM_NULL post after are
// the documented workaround for the menu not dismissing on an outside click.
func (t *Tray) showMenu(hwnd uintptr) {
	procSetForegroundWindow.Call(hwnd)

	// Reflect the active backend as the bulleted radio item each time the
	// menu opens (the menu itself is built once and reused).
	if n := len(t.backendNames); n > 0 {
		procCheckMenuRadioItem.Call(
			t.menu,
			uintptr(idBackendBase),
			uintptr(idBackendBase+n-1),
			uintptr(idBackendBase+t.activeBackend),
			uintptr(mfByCommand),
		)
	}

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	cmd, _, _ := procTrackPopupMenu.Call(
		t.menu,
		tpmRightButton|tpmReturnCmd|tpmNoNotify,
		uintptr(pt.x),
		uintptr(pt.y),
		0,
		hwnd,
		0,
	)

	procPostMessageW.Call(hwnd, wmNull, 0, 0)

	switch uint32(cmd) {
	case idQuit:
		if t.onQuit != nil {
			t.onQuit()
		}
		procPostQuitMessage.Call(0)
	case idCheckUpdate:
		if t.onCheckUpdate != nil {
			t.onCheckUpdate()
		}
	default:
		// A backend radio item: switch only on an actual change, update the
		// tooltip so the active backend stays visible, and notify the app.
		if n := len(t.backendNames); n > 0 {
			id := int(uint32(cmd))
			if id >= idBackendBase && id < idBackendBase+n {
				idx := id - idBackendBase
				if idx != t.activeBackend {
					t.activeBackend = idx
					t.updateTooltip()
					if t.onSelectBackend != nil {
						t.onSelectBackend(idx)
					}
				}
			}
		}
	}
}

// Notify shows a tray balloon (title + body). Safe to call from any
// goroutine: the text is stashed under a lock and a private message posted
// to the message-loop thread, which owns the icon and actually shows the
// balloon. It blocks only until Run has reached its start signal, then
// returns immediately. A no-op if the icon was never added or the window is
// gone (Notify after teardown posts to a zero handle, which fails harmlessly).
func (t *Tray) Notify(title, text string) {
	<-t.started
	t.balloonMu.Lock()
	t.balloonTitle = title
	t.balloonText = text
	t.balloonMu.Unlock()
	procPostMessageW.Call(t.hwnd.Load(), uintptr(balloonMsg), 0, 0)
}

// showBalloon updates the existing icon with balloon info (NIF_INFO) so the
// shell shows a notification toast. It runs on the message-loop thread, where
// t.nid is otherwise touched, so the field writes need no lock. uFlags is
// restored afterwards so later add/delete calls are unaffected. A no-op until
// the icon has been added — a balloon for a missing icon would be dropped.
func (t *Tray) showBalloon(title, text string) {
	if !t.added {
		return
	}
	prevFlags := t.nid.uFlags
	t.nid.uFlags = nifInfo
	putUTF16(t.nid.szInfoTitle[:], title)
	putUTF16(t.nid.szInfo[:], text)
	t.nid.dwInfoFlags = niifInfo
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
	t.nid.uFlags = prevFlags
}

// tooltipText is the base tooltip plus the active backend name when a
// backend selector is configured, e.g. "Prata — Rngv GPU-server".
func (t *Tray) tooltipText() string {
	if n := len(t.backendNames); n > 0 && t.activeBackend >= 0 && t.activeBackend < n {
		return t.tooltip + " — " + t.backendNames[t.activeBackend]
	}
	return t.tooltip
}

// updateTooltip refreshes the tray tooltip to show the active backend. Runs
// on the message-loop thread (from showMenu), where t.nid is owned, so the
// field writes need no lock. uFlags is restored afterwards so later
// add/delete calls are unaffected. A no-op until the icon has been added.
func (t *Tray) updateTooltip() {
	if !t.added {
		return
	}
	prevFlags := t.nid.uFlags
	t.nid.uFlags = nifTip
	putUTF16(t.nid.szTip[:], t.tooltipText())
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
	t.nid.uFlags = prevFlags
}

// buildMenu creates the popup menu once; Run reuses it for every right-click
// and destroys it on the cleanup path. When onCheckUpdate is set it prepends
// a "Sök efter uppdatering…" item and a separator above Avsluta; otherwise the
// menu is just Avsluta.
func (t *Tray) buildMenu() (uintptr, error) {
	menu, _, sysErr := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0, fmt.Errorf("CreatePopupMenu failed: %v", sysErr)
	}

	// Backend radio items (Rngv GPU-server / Rum1 GPU-server / Berget Ai) at the
	// top, followed by a separator. The active one is bulleted per-open in showMenu.
	for i, name := range t.backendNames {
		p, err := syscall.UTF16PtrFromString(name)
		if err != nil {
			procDestroyMenu.Call(menu)
			return 0, fmt.Errorf("menu item: %w", err)
		}
		if ret, _, sysErr := procAppendMenuW.Call(menu, mfString, uintptr(idBackendBase+i), uintptr(unsafe.Pointer(p))); ret == 0 {
			procDestroyMenu.Call(menu)
			return 0, fmt.Errorf("AppendMenuW (backend) failed: %v", sysErr)
		}
	}
	if len(t.backendNames) > 0 {
		if ret, _, sysErr := procAppendMenuW.Call(menu, mfSeparator, 0, 0); ret == 0 {
			procDestroyMenu.Call(menu)
			return 0, fmt.Errorf("AppendMenuW (backend separator) failed: %v", sysErr)
		}
	}

	if t.onCheckUpdate != nil {
		upd, err := syscall.UTF16PtrFromString("Sök efter uppdatering…")
		if err != nil {
			procDestroyMenu.Call(menu)
			return 0, fmt.Errorf("menu item: %w", err)
		}
		if ret, _, sysErr := procAppendMenuW.Call(menu, mfString, idCheckUpdate, uintptr(unsafe.Pointer(upd))); ret == 0 {
			procDestroyMenu.Call(menu)
			return 0, fmt.Errorf("AppendMenuW (update) failed: %v", sysErr)
		}
		if ret, _, sysErr := procAppendMenuW.Call(menu, mfSeparator, 0, 0); ret == 0 {
			procDestroyMenu.Call(menu)
			return 0, fmt.Errorf("AppendMenuW (separator) failed: %v", sysErr)
		}
	}

	item, err := syscall.UTF16PtrFromString("Avsluta")
	if err != nil {
		procDestroyMenu.Call(menu)
		return 0, fmt.Errorf("menu item: %w", err)
	}
	ret, _, sysErr := procAppendMenuW.Call(menu, mfString, idQuit, uintptr(unsafe.Pointer(item)))
	if ret == 0 {
		procDestroyMenu.Call(menu)
		return 0, fmt.Errorf("AppendMenuW failed: %v", sysErr)
	}
	return menu, nil
}

// buildIcon parses the .ico bytes, picks a frame at least as large as the
// DPI-scaled small-icon size, and builds an HICON of exactly that size via
// CreateIconFromResourceEx — so any scaling is downward (crisp), not upward.
func (t *Tray) buildIcon() (uintptr, error) {
	cx, _, _ := procGetSystemMetrics.Call(smCXSmIcon)
	desired := int(cx)
	if desired <= 0 {
		desired = 16
	}

	img, err := pickIconFrame(t.iconICO, desired)
	if err != nil {
		return 0, err
	}

	h, _, sysErr := procCreateIconFromResource.Call(
		uintptr(unsafe.Pointer(&img[0])),
		uintptr(len(img)),
		1, // fIcon = TRUE
		iconVer,
		uintptr(desired),
		uintptr(desired),
		0,
	)
	if h == 0 {
		return 0, fmt.Errorf("CreateIconFromResourceEx failed: %v", sysErr)
	}
	return h, nil
}

// pickIconFrame returns the raw image bits of the best .ico frame for a
// target width of want: the smallest frame at least want wide (so an exact
// match wins, otherwise the next size up), or — when no frame reaches want —
// the largest frame available. Choosing a frame >= want keeps
// CreateIconFromResourceEx scaling downward (crisp) rather than upward
// (blurry). The function consumes these bits directly, so the ICONDIR header
// itself is not included.
func pickIconFrame(ico []byte, want int) ([]byte, error) {
	const dirSize = 6
	const entrySize = 16

	if len(ico) < dirSize {
		return nil, fmt.Errorf("ico too small: %d bytes", len(ico))
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	if count == 0 {
		return nil, fmt.Errorf("ico has no images")
	}
	if len(ico) < dirSize+count*entrySize {
		return nil, fmt.Errorf("ico directory truncated")
	}

	// up* tracks the smallest frame >= want; max* tracks the largest frame
	// overall, used only when nothing reaches want.
	upOffset, upSize, upWidth := 0, 0, 0
	maxOffset, maxSize, maxWidth := 0, 0, 0
	for i := 0; i < count; i++ {
		e := dirSize + i*entrySize
		w := int(ico[e])
		if w == 0 {
			w = 256
		}
		size := int(binary.LittleEndian.Uint32(ico[e+8 : e+12]))
		offset := int(binary.LittleEndian.Uint32(ico[e+12 : e+16]))
		if size == 0 || offset < 0 || offset+size > len(ico) {
			continue
		}
		if w > maxWidth {
			maxOffset, maxSize, maxWidth = offset, size, w
		}
		if w >= want && (upWidth == 0 || w < upWidth) {
			upOffset, upSize, upWidth = offset, size, w
		}
	}

	switch {
	case upSize != 0:
		return ico[upOffset : upOffset+upSize], nil
	case maxSize != 0:
		return ico[maxOffset : maxOffset+maxSize], nil
	default:
		return nil, fmt.Errorf("ico has no usable frame")
	}
}

// putUTF16 writes s as a null-terminated UTF-16 string into a fixed-size
// buffer (tooltip, balloon title, or balloon body), truncating to fit and
// always leaving room for the terminator.
func putUTF16(dst []uint16, s string) {
	enc := utf16.Encode([]rune(s))
	n := copy(dst[:len(dst)-1], enc)
	dst[n] = 0
}
