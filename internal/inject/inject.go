// Package inject sends Unicode text to the foreground window via
// Win32's SendInput with KEYEVENTF_UNICODE. Each UTF-16 code unit
// produces a key-down + key-up event; runes outside the BMP are
// emitted as surrogate pairs (two units → four events).
package inject

import (
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	inputKeyboard    = 1
	keyEventfKeyUp   = 0x0002
	keyEventfUnicode = 0x0004
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
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

// Type sends `text` to the foreground window. Returns an error if
// SendInput fails to dispatch every queued event.
func Type(text string) error {
	units := utf16.Encode([]rune(text))
	if len(units) == 0 {
		return nil
	}

	inputs := make([]input, 0, len(units)*2)
	for _, u := range units {
		inputs = append(inputs,
			makeKeyInput(u, false), // down
			makeKeyInput(u, true),  // up
		)
	}

	sent, _, sysErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if int(sent) != len(inputs) {
		return fmt.Errorf("SendInput dispatched %d of %d events: %v", sent, len(inputs), sysErr)
	}
	return nil
}

func makeKeyInput(unit uint16, keyUp bool) input {
	flags := uint32(keyEventfUnicode)
	if keyUp {
		flags |= keyEventfKeyUp
	}
	return input{
		inputType: inputKeyboard,
		ki: keybdInput{
			Scan:  unit,
			Flags: flags,
		},
	}
}
