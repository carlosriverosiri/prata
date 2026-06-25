// Package inject sends Unicode text to the foreground window by one of two
// paths: clipboard paste (Type — place the text on the clipboard and issue
// a Ctrl+V chord) or direct character synthesis (TypeUnicode — a single
// SendInput call using KEYEVENTF_UNICODE). It also reads the foreground
// window's current selection via CopySelection (a synthesized Ctrl+C that
// leaves the clipboard as it found it), and reports the foreground window's
// class via ForegroundWindowClass for class-based injection routing.
package inject

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	inputKeyboard    = 1
	keyEventfKeyUp   = 0x0002
	keyEventfUnicode = 0x0004
	vkControl        = 0x11
	vkShift          = 0x10
	vkReturn         = 0x0D
	vkV              = 0x56
	vkC              = 0x43

	cfUnicodeText    = 13
	gmemMoveable     = 0x0002
	gmemZeroInit     = 0x0040
	interEventDelay  = 2 * time.Millisecond
	pasteSettleDelay = 50 * time.Millisecond

	// copySettleTimeout is how long CopySelection waits for the clipboard
	// sequence number to change after Ctrl+C. Chromium/Webdoc often needs
	// more than a fixed short sleep; polling the seq avoids a race.
	copySettleTimeout = 300 * time.Millisecond
	copySettlePoll    = 10 * time.Millisecond

	// focusSettle is how long RestoreForeground waits after
	// SetForegroundWindow before confirming the window actually became
	// foreground. Tuned conservatively; revisit during device testing.
	focusSettle = 30 * time.Millisecond
)

// KEYBDINPUT from winuser.h. Go inserts padding between Time and
// ExtraInfo automatically (uintptr is 8-byte aligned on x64).
type keybdInput struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// input mirrors Win32's INPUT union. INPUT is 40 bytes on x64 because
// the largest union member (MOUSEINPUT) is 32 bytes after the 8-byte
// type-and-padding prefix. KEYBDINPUT is only 24 bytes, so we add an
// explicit 8-byte tail pad.
type input struct {
	inputType uint32
	ki        keybdInput
	_         [8]byte
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSendInput                  = user32.NewProc("SendInput")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	procIsWindow                   = user32.NewProc("IsWindow")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput          = user32.NewProc("AttachThreadInput")
	procGetClassNameW              = user32.NewProc("GetClassNameW")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory              = kernel32.NewProc("RtlMoveMemory")
	procGetCurrentThreadId         = kernel32.NewProc("GetCurrentThreadId")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
)

// Type sends text to the foreground window via clipboard paste. Direct
// KEYEVENTF_UNICODE input is unreliable in some targets (notably modern
// Notepad and Chromium/Electron apps), which can drop key-up events and
// produce repeated characters. Clipboard paste is the same path users
// exercise manually with Ctrl+V and is therefore more robust.
//
// Any prior CF_UNICODETEXT clipboard content is saved before the paste
// and best-effort restored afterwards. Non-text formats (images, files,
// rich text from Office apps) are not preserved.
//
// The dictated text is marked to stay out of clipboard history (Win+V), the
// cloud clipboard, and clipboard monitors (setDictatedClipboardText), so
// medical-record text neither lingers nor syncs after the paste. The restore of
// the prior clipboard is marked the same way (setClipboardText): Prata adds no
// entry of its own to Win+V, so a clipboard-history user sees only the items
// they copied themselves, never a Prata-made duplicate of their prior copy.
func Type(text string) error {
	text = normalizeClipboardText(text)
	if text == "" {
		return nil
	}

	previous, hadPrevious, _ := getClipboardText()

	if err := setDictatedClipboardText(text); err != nil {
		return err
	}
	if err := sendChord(vkV); err != nil {
		return err
	}

	time.Sleep(pasteSettleDelay)

	if hadPrevious {
		_ = setClipboardText(previous)
	} else {
		_ = clearClipboard()
	}
	return nil
}

// TypeUnicode is an experimental, clipboard-free alternative to Type. It
// synthesizes the entire string as Unicode character input
// (KEYEVENTF_UNICODE) and sends it in a SINGLE SendInput call. Batching the
// whole string into one call is the deliberate difference from the earlier
// per-rune attempt, which autorepeated characters in Electron/Chromium and
// modern Notepad; one atomic call mirrors the approach the Diktell Rust app
// uses via enigo. In production it is reached only through TypeAuto, for
// foreground windows whose class is on the SendInput allowlist; every other
// window keeps the clipboard-paste path (Type).
//
// Newlines are emitted as Shift+Enter (a chat-safe soft line break), never
// a bare Enter, which would send the message in chat apps. Line breaks are
// normalized to '\n' first so '\r\n' and '\r' collapse to one break each.
func TypeUnicode(text string) error {
	if text == "" {
		return nil
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var events []input
	for _, r := range text {
		if r == '\n' {
			events = append(events,
				makeVKInput(vkShift, false),
				makeVKInput(vkReturn, false),
				makeVKInput(vkReturn, true),
				makeVKInput(vkShift, true),
			)
			continue
		}
		// Encode to UTF-16 so characters outside the BMP are sent as their
		// two surrogate code units, each as its own key-down/key-up pair.
		for _, codeUnit := range utf16.Encode([]rune{r}) {
			events = append(events,
				makeUnicodeInput(codeUnit, false),
				makeUnicodeInput(codeUnit, true),
			)
		}
	}

	return sendInputs(events)
}

// sendInputSafeClasses is the allowlist of foreground window classes for
// which TypeAuto uses SendInput (TypeUnicode) instead of clipboard paste.
// Classes are added ONLY after verification with realistic, multi-line
// text. "Chrome_WidgetWin_1" deliberately covers the ENTIRE Chromium/
// Electron family (Chrome, Edge, Cursor, Claude Desktop, and also Slack,
// VS Code, Discord, ...) plus the verified web-based journal system, which
// reports the same class. This is an intentional engine-level bet: the
// earlier autorepeat failure was engine-level, and SendInput is verified
// across several distinct Chromium hosts. Modern Notepad (class "Notepad")
// is intentionally omitted — a short test can hide its content/length-
// dependent failure.
var sendInputSafeClasses = map[string]struct{}{
	"Chrome_WidgetWin_1": {},
	// "Notepad++" (the Scintilla-based editor) reports class "Notepad++". Its
	// clipboard-paste path fails to insert the dictated text — unlike classic
	// Notepad (class "Notepad"), which shares that path and works — so route it
	// through SendInput instead, which sidesteps the clipboard (and its history
	// exclusion markers) entirely. Scintilla accepts the single batched Unicode
	// SendInput call, the same way Chromium does. Pending verification with
	// realistic multi-line text and digit strings before this is treated as
	// settled.
	"Notepad++": {},
}

// IsSendInputSafeClass reports whether class is on the SendInput allowlist
// (see sendInputSafeClasses). Exported so test harnesses can mirror
// TypeAuto's routing decision for logging.
func IsSendInputSafeClass(class string) bool {
	_, ok := sendInputSafeClasses[class]
	return ok
}

// TypeAuto sends text to the foreground window, routing on that window's
// class: SendInput (TypeUnicode) for allowlisted, verified-safe classes,
// and clipboard paste (Type) otherwise. This is the patient-safety-critical
// dictation path.
//
// Safe default: ANY uncertainty — no foreground window, a failed class
// read, or an unknown class — routes to Type, the proven clipboard-paste
// path. SendInput is used only when the class is positively identified AND
// on the allowlist.
//
// No execution fallback: TypeAuto picks the path ONCE and calls it. If
// TypeUnicode returns an error it is deliberately NOT retried via Type —
// SendInput may already have sent some characters, and a following
// clipboard paste would double-inject, which in a patient journal is a
// safety hazard. Lost text on a rare SendInput failure means the user
// re-dictates, which is safe. Never add a fallback here.
func TypeAuto(text string) error {
	class, ok := ForegroundWindowClass()
	if ok && IsSendInputSafeClass(class) {
		return TypeUnicode(text)
	}
	return Type(text)
}

// CopySelection grabs the foreground window's current selection by
// synthesizing Ctrl+C and reading the clipboard. It is clipboard-neutral:
// any prior CF_UNICODETEXT content is restored afterwards (or the
// clipboard cleared if there was none).
//
// The clipboard is cleared before the copy so that an empty clipboard
// afterwards reliably means nothing was selected. Without clearing first,
// a no-op Ctrl+C (nothing selected) would leave the prior text in place
// and we would misreport it as the selection.
//
// After Ctrl+C the clipboard sequence number is polled (not a fixed sleep)
// until it changes from the post-clear baseline, then CF_UNICODETEXT is
// read once. Slow targets such as Chromium/Webdoc often need >50 ms to
// populate the clipboard; gating on the sequence number avoids losing the
// race on a single short F8 tap. In a "nothing selected" case ok is false.
// Non-text prior formats (images, files, rich text) are not preserved.
func CopySelection() (text string, ok bool, err error) {
	previous, hadPrevious, _ := getClipboardText()

	if err := clearClipboard(); err != nil {
		return "", false, err
	}

	// seqBefore must be captured AFTER clear: EmptyClipboard bumps the
	// sequence number itself; reading before clear would make the gate
	// fire on our own clear rather than the app's copy response.
	seqBefore := getClipboardSequenceNumber()

	if err := sendChord(vkC); err != nil {
		return "", false, err
	}

	if waitClipboardSequenceChange(seqBefore, copySettleTimeout, copySettlePoll) {
		text, ok, err = getClipboardText()
	}

	if hadPrevious {
		_ = setClipboardText(previous)
	} else {
		_ = clearClipboard()
	}
	if err != nil {
		return "", false, err
	}
	if !ok || text == "" {
		return "", false, nil
	}
	return text, true, nil
}

// getClipboardSequenceNumber returns the current clipboard sequence number.
// GetClipboardSequenceNumber does not require OpenClipboard.
func getClipboardSequenceNumber() uint32 {
	seq, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(seq)
}

// waitClipboardSequenceChange polls until the clipboard sequence number
// differs from before, or timeout elapses. Returns false on timeout.
func waitClipboardSequenceChange(before uint32, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if getClipboardSequenceNumber() != before {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

func clearClipboard() error {
	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	ret, _, sysErr := procEmptyClipboard.Call()
	if ret == 0 {
		return fmt.Errorf("EmptyClipboard: %v", sysErr)
	}
	return nil
}

func normalizeClipboardText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n")
}

// setClipboardText places text on the clipboard as CF_UNICODETEXT, replacing
// any prior content. It is unmarked — used to restore a saved clipboard, so
// the user's own content goes back exactly as it was (in history, syncable).
// setClipboardText restores or sets ordinary (non-dictated) clipboard text —
// e.g. putting the user's prior clipboard back after a paste or selection read.
// Like every Prata clipboard write it is excluded from history, the cloud
// clipboard, and monitors (see writeClipboardText), so restoring the user's own
// content never duplicates their earlier copy in Win+V.
func setClipboardText(text string) error {
	return writeClipboardText(text)
}

// setDictatedClipboardText places dictated text on the clipboard. Like every
// Prata clipboard write it is kept out of clipboard history, the cloud
// clipboard, and clipboard monitors, so medical-record text neither lingers in
// Win+V nor syncs to the cloud after the paste (patient confidentiality).
func setDictatedClipboardText(text string) error {
	return writeClipboardText(text)
}

// writeClipboardText is the shared clipboard writer behind setClipboardText and
// setDictatedClipboardText. It always marks the entry to stay out of clipboard
// history (Win+V), the cloud clipboard, and clipboard monitors: every clipboard
// write Prata makes is excluded, so Prata never adds an entry to the user's
// clipboard history — neither the dictated text (patient confidentiality) nor
// the restore of the user's prior clipboard (which would otherwise duplicate
// the user's own earlier copy in Win+V). The user only ever sees clipboard
// entries they copied themselves. The markers are best-effort: a failure to set
// one never fails the write — the worst case reverts to the prior behavior,
// where the entry is an ordinary clipboard entry.
func writeClipboardText(text string) error {
	data, err := syscall.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("encode clipboard text: %w", err)
	}

	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()

	ret, _, sysErr := procEmptyClipboard.Call()
	if ret == 0 {
		return fmt.Errorf("EmptyClipboard: %v", sysErr)
	}

	size := uintptr(len(data) * 2)
	handle, _, sysErr := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, size)
	if handle == 0 {
		return fmt.Errorf("GlobalAlloc: %v", sysErr)
	}

	ptr, _, sysErr := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return fmt.Errorf("GlobalLock: %v", sysErr)
	}

	procRtlMoveMemory.Call(
		ptr,
		uintptr(unsafe.Pointer(&data[0])),
		size,
	)
	runtime.KeepAlive(data)
	procGlobalUnlock.Call(handle)

	ret, _, sysErr = procSetClipboardData.Call(cfUnicodeText, handle)
	if ret == 0 {
		procGlobalFree.Call(handle)
		return fmt.Errorf("SetClipboardData: %v", sysErr)
	}

	setClipboardExclusionMarkers()
	return nil
}

// clipboardExclusionFormats are the registered clipboard formats that opt an
// entry out of clipboard history (Win+V), the cloud clipboard, and third-party
// clipboard monitors. For the Can* formats a DWORD 0 means "exclude"; the
// Exclude... format only needs to be present.
var clipboardExclusionFormats = []string{
	"CanIncludeInClipboardHistory",
	"CanUploadToCloudClipboard",
	"ExcludeClipboardContentFromMonitorProcessing",
}

// setClipboardExclusionMarkers marks the currently-open clipboard so its
// content is kept out of clipboard history, the cloud clipboard, and clipboard
// monitors. It must run inside an open clipboard session, after the data is
// set. Best-effort throughout: a registration or set failure is ignored so the
// paste still succeeds — losing a marker only reverts to the prior behavior,
// never injects or leaks anything new.
func setClipboardExclusionMarkers() {
	for _, name := range clipboardExclusionFormats {
		format := registerClipboardFormat(name)
		if format == 0 {
			continue
		}
		// A movable, zero-initialized DWORD: 0 is "exclude" for the Can*
		// formats, and the Exclude... format only needs to be present. The
		// system owns the handle once SetClipboardData succeeds; on failure
		// we free it.
		handle, _, _ := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, unsafe.Sizeof(uint32(0)))
		if handle == 0 {
			continue
		}
		if ret, _, _ := procSetClipboardData.Call(format, handle); ret == 0 {
			procGlobalFree.Call(handle)
		}
	}
}

// registerClipboardFormat registers (or looks up, if already registered) a
// named clipboard format and returns its ID, or 0 on failure.
func registerClipboardFormat(name string) uintptr {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	id, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
	return id
}

// getClipboardText returns the current CF_UNICODETEXT clipboard content,
// if any. The second return value distinguishes "no text on clipboard"
// from "empty text". Errors are returned to the caller but Type ignores
// them on purpose: failing dictation because we couldn't read the prior
// clipboard would be worse than silently overwriting it.
func getClipboardText() (string, bool, error) {
	if err := openClipboard(); err != nil {
		return "", false, err
	}
	defer procCloseClipboard.Call()

	avail, _, _ := procIsClipboardFormatAvailable.Call(cfUnicodeText)
	if avail == 0 {
		return "", false, nil
	}

	handle, _, sysErr := procGetClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", false, fmt.Errorf("GetClipboardData: %v", sysErr)
	}

	ptr, _, sysErr := procGlobalLock.Call(handle)
	if ptr == 0 {
		return "", false, fmt.Errorf("GlobalLock: %v", sysErr)
	}
	defer procGlobalUnlock.Call(handle)

	size, _, _ := procGlobalSize.Call(handle)
	if size == 0 {
		return "", true, nil
	}

	buf := make([]uint16, size/2)
	procRtlMoveMemory.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		ptr,
		size,
	)
	runtime.KeepAlive(buf)
	return syscall.UTF16ToString(buf), true, nil
}

func openClipboard() error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		ret, _, sysErr := procOpenClipboard.Call(0)
		if ret != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("OpenClipboard: %v", sysErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// sendChord synthesizes a Ctrl+<vk> chord (Ctrl down, vk down, vk up,
// Ctrl up) with a short delay between events so the target app observes a
// clean sequence. Used for both paste (Ctrl+V) and copy (Ctrl+C).
func sendChord(vk uint16) error {
	events := []input{
		makeVKInput(vkControl, false),
		makeVKInput(vk, false),
		makeVKInput(vk, true),
		makeVKInput(vkControl, true),
	}
	for i, event := range events {
		if err := sendInputs([]input{event}); err != nil {
			return err
		}
		if i < len(events)-1 {
			time.Sleep(interEventDelay)
		}
	}
	return nil
}

func sendInputs(inputs []input) error {
	sent, _, sysErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	// Keep inputs reachable until SendInput has finished reading the
	// array, so the GC cannot relocate or collect it mid-syscall.
	runtime.KeepAlive(inputs)

	if int(sent) != len(inputs) {
		return fmt.Errorf("SendInput dispatched %d of %d events: %v", sent, len(inputs), sysErr)
	}
	return nil
}

// makeVKInput builds an INPUT for a virtual-key press (no Unicode flag),
// used for keys that target apps interpret structurally — Enter, Tab,
// arrow keys — rather than as character input.
func makeVKInput(vk uint16, keyUp bool) input {
	var flags uint32
	if keyUp {
		flags = keyEventfKeyUp
	}
	return input{
		inputType: inputKeyboard,
		ki: keybdInput{
			Vk:    vk,
			Flags: flags,
		},
	}
}

// makeUnicodeInput builds an INPUT that injects a single UTF-16 code unit
// as a character via KEYEVENTF_UNICODE. Vk is left zero (the OS reads the
// character from Scan); surrogate pairs are sent as two consecutive code
// units. Used only by TypeUnicode.
func makeUnicodeInput(codeUnit uint16, keyUp bool) input {
	flags := uint32(keyEventfUnicode)
	if keyUp {
		flags |= keyEventfKeyUp
	}
	return input{
		inputType: inputKeyboard,
		ki: keybdInput{
			Scan:  codeUnit,
			Flags: flags,
		},
	}
}

// ForegroundWindow returns the current foreground window's handle and
// true, or (0, false) when there is no foreground window.
func ForegroundWindow() (uintptr, bool) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return 0, false
	}
	return hwnd, true
}

// IsWindow reports whether hwnd refers to an existing window. It wraps the
// Win32 IsWindow call. Use this to fast-fail before RestoreForeground when a
// stale job HWND may have been closed between capture and injection.
func IsWindow(hwnd uintptr) bool {
	ret, _, _ := procIsWindow.Call(hwnd)
	return ret != 0
}

// ForegroundWindowClass returns the window-class name of the current
// foreground window and true, or ("", false) when there is no foreground
// window or its class cannot be read. It is the basis for class-based
// injection routing: a caller can pick SendInput (TypeUnicode) for
// verified-safe classes and otherwise fall back to clipboard paste (Type).
func ForegroundWindowClass() (string, bool) {
	hwnd, ok := ForegroundWindow()
	if !ok {
		return "", false
	}
	var buf [256]uint16
	ret, _, _ := procGetClassNameW.Call(
		hwnd,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf[:ret]), true
}

// RestoreForeground brings hwnd back to the foreground and reports whether
// it actually became the foreground window. SetForegroundWindow will not
// steal focus across processes unless the calling thread shares an input
// queue with the target, so this attaches to the target window's thread
// (AttachThreadInput) for the duration of the call. It pins its OS thread
// because AttachThreadInput and SetForegroundWindow are thread-affine.
//
// The bool return is a safety gate, not a courtesy: an orchestrator (the
// F8 quick-fix flow) MUST abort paste-back when it is false, so a
// correction is never injected into the wrong window after a failed focus
// restore. A non-nil error means the restore could not even be attempted.
func RestoreForeground(hwnd uintptr) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if hwnd == 0 {
		return false, fmt.Errorf("RestoreForeground: nil window")
	}

	targetTID, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	if targetTID == 0 {
		return false, fmt.Errorf("GetWindowThreadProcessId: no thread for window")
	}
	currentTID, _, _ := procGetCurrentThreadId.Call()

	// Self-focus edge: if we already own the target's input thread there is
	// nothing to attach, and AttachThreadInput to self would fail.
	if targetTID != currentTID {
		ret, _, sysErr := procAttachThreadInput.Call(currentTID, targetTID, 1)
		if ret == 0 {
			return false, fmt.Errorf("AttachThreadInput attach: %v", sysErr)
		}
		defer procAttachThreadInput.Call(currentTID, targetTID, 0)
	}

	procSetForegroundWindow.Call(hwnd)
	time.Sleep(focusSettle)

	front, _ := ForegroundWindow()
	return front == hwnd, nil
}
