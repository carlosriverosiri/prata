// Package inject sends Unicode text to the foreground window by placing
// the text on the Windows clipboard and issuing a Ctrl+V key chord.
package inject

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	inputKeyboard  = 1
	keyEventfKeyUp = 0x0002
	vkControl      = 0x11
	vkV            = 0x56

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
	if err := sendPasteChord(); err != nil {
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

func sendPasteChord() error {
	events := []input{
		makeVKInput(vkControl, false),
		makeVKInput(vkV, false),
		makeVKInput(vkV, true),
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
