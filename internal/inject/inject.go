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
func Type(text string) error {
	text = normalizeClipboardText(text)
	if text == "" {
		return nil
	}

	previous, hadPrevious, _ := getClipboardText()

	if err := setClipboardText(text); err != nil {
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
// and we would misreport it as the selection. In that "nothing selected"
// case ok is false. Non-text prior formats (images, files, rich text)
// are not preserved.
func CopySelection() (text string, ok bool, err error) {
	previous, hadPrevious, _ := getClipboardText()

	if err := clearClipboard(); err != nil {
		return "", false, err
	}

	if err := sendChord(vkC); err != nil {
		return "", false, err
	}
	time.Sleep(pasteSettleDelay)

	text, ok, err = getClipboardText()

	if hadPrevious {
		_ = setClipboardText(previous)
	} else {
		_ = clearClipboard()
	}
	if err != nil {
		return "", false, err
	}
	return text, ok, nil
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

func setClipboardText(text string) error {
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

	return nil
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

// ForegroundWindowClass returns the window-class name of the current
// foreground window and true, or ("", false) when there is no foreground
// window or its class cannot be read. It is the basis for class-based
// injection routing: a caller can pick SendInput (TypeUnicode) for
// verified-safe classes and otherwise fall back to clipboard paste (Type).
func ForegroundWindowClass() (string, bool) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
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
