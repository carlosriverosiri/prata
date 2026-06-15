// Package popup shows a small modal text-input window for quickly editing
// a piece of text (e.g. a dictated term to correct). Prompt pre-fills an
// EDIT control, lets the user overtype it, and returns the edited text on
// Enter or reports cancellation on Esc / click-away / close.
//
// Implementation: pure Win32 P/Invoke, no cgo, matching the style of
// internal/tray and internal/hotkey. Prompt registers a window class,
// creates a borderless top-most popup with a child EDIT control, and runs
// a self-contained modal message loop on the calling goroutine (pinned
// with runtime.LockOSThread). An unexported popup struct carries the
// result state so its method can serve as the WndProc and signal the loop
// to exit when the window is closed or deactivated.
//
// Prompt blocks until dismissed. In integration it must therefore run on
// its own goroutine, never on the RegisterHotKey message-loop thread, so
// global hotkey handling stays responsive.
package popup

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// Win32 constants from winuser.h, wingdi.h and shellscalingapi.h.
const (
	wsPopup       = 0x80000000
	wsBorder      = 0x00800000
	wsVisible     = 0x10000000
	wsChild       = 0x40000000
	esAutoHScroll = 0x0080

	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080

	wmDestroy  = 0x0002
	wmActivate = 0x0006
	wmClose    = 0x0010
	wmSetFont  = 0x0030
	wmKeyDown  = 0x0100
	wmNull     = 0x0000
	emSetSel   = 0x00B1

	vkReturn = 0x0D
	vkEscape = 0x1B

	waInactive = 0

	swShow      = 5
	colorWindow = 5 // hbrBackground = COLOR_WINDOW+1; the window is shown

	monitorDefaultToNearest = 0x00000002
	mdtEffectiveDPI         = 0

	// CreateFontW parameters.
	fwNormal          = 400
	defaultCharset    = 1
	outDefaultPrecis  = 0
	clipDefaultPrecis = 0
	cleartypeQuality  = 5
	defaultPitch      = 0

	baseDPI = 96

	// Base (96-DPI) layout sizes, scaled up by the monitor DPI.
	baseWidth     = 360
	baseHeight    = 40
	baseMargin    = 5
	baseOffset    = 16 // popup offset from the cursor
	fontPointSize = 11
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

type rect struct {
	left, top, right, bottom int32
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

// monitorInfo mirrors the Win32 MONITORINFO struct for GetMonitorInfoW.
type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// guiThreadInfo mirrors the Win32 GUITHREADINFO struct for GetGUIThreadInfo,
// used to read the foreground window's caret rectangle so the popup can be
// anchored to the text being edited rather than the mouse.
type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       rect
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")

	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procUnregisterClassW     = user32.NewProc("UnregisterClassW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procSetFocus             = user32.NewProc("SetFocus")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procSetWindowTextW       = user32.NewProc("SetWindowTextW")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procMonitorFromPoint     = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")

	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	procClientToScreen           = user32.NewProc("ClientToScreen")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procCreateFontW  = gdi32.NewProc("CreateFontW")
	procDeleteObject = gdi32.NewProc("DeleteObject")

	// Windows 8.1+; guard with .Find and fall back to 96 DPI.
	procGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")
)

// popup holds the per-call window state. Its fields are touched only on
// the modal-loop thread (the loop itself and the WndProc it dispatches),
// so they need no synchronisation.
type popup struct {
	hwnd uintptr

	ok        bool // user confirmed with Enter
	done      bool // modal loop should exit
	activated bool // first WA_ACTIVE seen; gates WA_INACTIVE-as-cancel
}

// Prompt shows a modal single-line text popup pre-filled with initial,
// positioned over the edited line (foreground caret, or the mouse cursor as
// a fallback) and always on top. It returns the edited
// text and ok=true when the user presses Enter, or ok=false when the user
// cancels (Esc, clicks away / deactivates, or closes the window). err is
// non-nil only on a fundamental window-creation failure; when ok is false
// the returned text should be ignored.
func Prompt(initial string) (result string, ok bool, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	p := &popup{}
	return p.run(initial)
}

func (p *popup) run(initial string) (string, bool, error) {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, err := syscall.UTF16PtrFromString("PrataPopupWindow")
	if err != nil {
		return "", false, fmt.Errorf("class name: %w", err)
	}

	wc := wndClassExW{
		lpfnWndProc:   syscall.NewCallback(p.wndProc),
		hInstance:     hInstance,
		hbrBackground: colorWindow + 1,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	atom, _, sysErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return "", false, fmt.Errorf("RegisterClassExW failed: %v", sysErr)
	}
	defer procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)

	// Resolve the anchor (UIA selection rectangle, legacy caret, then mouse
	// cursor), then the DPI and work area of that anchor's monitor, before
	// sizing/positioning the window so it is crisp and on-screen.
	anchor := anchorPoint()
	dpi, work := monitorMetrics(anchor)

	scale := func(v int32) int32 { return v * int32(dpi) / baseDPI }
	width := scale(baseWidth)
	height := scale(baseHeight)
	offset := scale(baseOffset)

	// Open the box UPWARD from the anchor: its bottom edge lands `offset`
	// above the anchor so the popup sits over the selection instead of the
	// text below it, where the user keeps reading and writing. The work-area
	// clamp below still pushes it back on-screen if there is no room above.
	x, y := anchor.x, anchor.y-offset-height
	if work.right > work.left {
		if x+width > work.right {
			x = work.right - width
		}
		if x < work.left {
			x = work.left
		}
		if y+height > work.bottom {
			y = work.bottom - height
		}
		if y < work.top {
			y = work.top
		}
	}

	windowName, _ := syscall.UTF16PtrFromString("Prata")
	hwnd, _, sysErr := procCreateWindowExW.Call(
		wsExTopmost|wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		wsPopup|wsBorder|wsVisible,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return "", false, fmt.Errorf("CreateWindowExW failed: %v", sysErr)
	}
	p.hwnd = hwnd
	defer procDestroyWindow.Call(hwnd)

	edit, err := p.createEdit(hwnd, hInstance, dpi)
	if err != nil {
		return "", false, err
	}

	font := createFont(dpi)
	if font != 0 {
		procSendMessageW.Call(edit, wmSetFont, font, 1)
		defer procDeleteObject.Call(font)
	}

	if initial != "" {
		if text, terr := syscall.UTF16PtrFromString(initial); terr == nil {
			procSetWindowTextW.Call(edit, uintptr(unsafe.Pointer(text)))
		}
	}
	// EM_SETSEL(0, -1): select all so the user can overtype immediately.
	procSendMessageW.Call(edit, emSetSel, 0, ^uintptr(0))

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(edit)

	p.loop()

	// Read the edit text before the deferred DestroyWindow runs.
	text := getWindowText(edit)
	return text, p.ok, nil
}

// createEdit makes the child EDIT control filling the client area inside a
// DPI-scaled margin.
func (p *popup) createEdit(hwnd, hInstance uintptr, dpi uint32) (uintptr, error) {
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	margin := int32(baseMargin) * int32(dpi) / baseDPI

	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	edit, _, sysErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(editClass)),
		0,
		wsChild|wsVisible|wsBorder|esAutoHScroll,
		uintptr(margin), uintptr(margin),
		uintptr(rc.right-2*margin), uintptr(rc.bottom-2*margin),
		hwnd, 1, hInstance, 0,
	)
	if edit == 0 {
		return 0, fmt.Errorf("create edit control: %v", sysErr)
	}
	return edit, nil
}

// loop runs the modal message pump. Enter/Esc are queued WM_KEYDOWN
// messages, so they are caught here before dispatch (also avoiding the
// single-line EDIT's Enter beep). Close/deactivate are sent messages
// handled in wndProc, which sets done and posts WM_NULL to wake this loop.
func (p *popup) loop() {
	var m msg
	for !p.done {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case -1, 0: // error, or WM_QUIT
			p.done = true
			p.ok = false
			continue
		}

		if m.message == wmKeyDown {
			switch m.wParam {
			case vkReturn:
				p.ok = true
				p.done = true
				continue
			case vkEscape:
				p.ok = false
				p.done = true
				continue
			}
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// wndProc handles the window messages that never surface in the modal loop
// because they are sent, not posted: close, destroy, and deactivation.
func (p *popup) wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmActivate:
		if wParam&0xffff == waInactive {
			// Ignore the activation churn during initial show/focus;
			// only a deactivation after the window has truly activated
			// counts as the user clicking away.
			if p.activated {
				p.cancel()
			}
		} else {
			p.activated = true
		}
		return 0
	case wmClose, wmDestroy:
		p.cancel()
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return ret
}

// cancel marks the popup as dismissed without confirmation and wakes the
// modal loop with a no-op message so it re-checks done.
func (p *popup) cancel() {
	p.ok = false
	p.done = true
	procPostMessageW.Call(p.hwnd, wmNull, 0, 0)
}

// anchorPoint returns the screen point to position the popup near, trying
// three sources in order of reliability so the box lands on the edited line
// regardless of where the mouse sits:
//  1. The text selection's bounding rectangle via UI Automation — reliable
//     in Chromium/Electron (the journal, the editor) and native controls.
//  2. The legacy system caret (GetGUIThreadInfo) — works for many Win32
//     controls but is reported inconsistently by Chromium.
//  3. The mouse cursor — always available, usually near the writing zone.
//
// Must be called before the popup window is created, while the edited window
// is still in the foreground.
func anchorPoint() point {
	if r, ok := selectionRect(); ok {
		return point{r.left, r.top}
	}

	if fg, _, _ := procGetForegroundWindow.Call(); fg != 0 {
		var gti guiThreadInfo
		gti.cbSize = uint32(unsafe.Sizeof(gti))
		if tid, _, _ := procGetWindowThreadProcessId.Call(fg, 0); tid != 0 {
			r, _, _ := procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti)))
			// A real caret has positive height; a zero rect means the app
			// reports none. rcCaret is in hwndCaret's client coordinates.
			if r != 0 && gti.hwndCaret != 0 && gti.rcCaret.bottom > gti.rcCaret.top {
				pt := point{gti.rcCaret.left, gti.rcCaret.top}
				procClientToScreen.Call(gti.hwndCaret, uintptr(unsafe.Pointer(&pt)))
				return pt
			}
		}
	}

	var cursor point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	return cursor
}

// monitorMetrics returns the effective DPI and work-area rect of the
// monitor under pt. It falls back to 96 DPI (and a zero work rect, which
// disables clamping) when the per-monitor DPI API is unavailable.
func monitorMetrics(pt point) (dpi uint32, work rect) {
	dpi = baseDPI
	mon, _, _ := procMonitorFromPoint.Call(packPoint(pt), monitorDefaultToNearest)

	var mi monitorInfo
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
		work = mi.rcWork
	}

	if procGetDpiForMonitor.Find() == nil {
		var dx, dy uint32
		ret, _, _ := procGetDpiForMonitor.Call(
			mon,
			mdtEffectiveDPI,
			uintptr(unsafe.Pointer(&dx)),
			uintptr(unsafe.Pointer(&dy)),
		)
		if ret == 0 && dx != 0 { // S_OK
			dpi = dx
		}
	}
	return dpi, work
}

// packPoint packs a POINT into a single uintptr as MonitorFromPoint
// expects it by value: x in the low 32 bits, y in the high 32 bits.
func packPoint(pt point) uintptr {
	return uintptr(uint32(pt.x)) | uintptr(uint32(pt.y))<<32
}

// createFont builds a DPI-scaled Segoe UI font. The caller owns the handle
// and must DeleteObject it. Returns 0 on failure (caller then keeps the
// system default font).
func createFont(dpi uint32) uintptr {
	face, err := syscall.UTF16PtrFromString("Segoe UI")
	if err != nil {
		return 0
	}
	height := -(fontPointSize * int32(dpi) / 72)
	h, _, _ := procCreateFontW.Call(
		uintptr(height),
		0, 0, 0,
		fwNormal,
		0, 0, 0,
		defaultCharset,
		outDefaultPrecis,
		clipDefaultPrecis,
		cleartypeQuality,
		defaultPitch,
		uintptr(unsafe.Pointer(face)),
	)
	return h
}

// getWindowText reads a window's text (here the EDIT control's contents).
func getWindowText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, int(n)+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}
