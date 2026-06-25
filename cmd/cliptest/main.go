// Command cliptest is a throwaway diagnostic for the 2026-06-25 investigation
// into which clipboard exclusion marker breaks paste in Notepad++ (Scintilla).
// It writes CF_UNICODETEXT plus a CHOSEN SUBSET of the three marker formats, so
// the dictated-clipboard write can be reproduced exactly while isolating one
// marker at a time. The user then pastes manually (Ctrl+V) into the target —
// manual paste removes Prata's synthetic-input/focus path as a variable.
//
// Usage:
//
//	cliptest <markers> [text]
//	  <markers>: "none", "all", or a comma list of: history,cloud,monitor
//	  text:      defaults to a recognizable marker string
//
// The clipboard content (text + markers) survives this process exiting, so the
// user can paste any time after it prints "clipboard set".
package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard            = user32.NewProc("OpenClipboard")
	procCloseClipboard           = user32.NewProc("CloseClipboard")
	procEmptyClipboard           = user32.NewProc("EmptyClipboard")
	procSetClipboardData         = user32.NewProc("SetClipboardData")
	procRegisterClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc              = kernel32.NewProc("GlobalAlloc")
	procGlobalLock               = kernel32.NewProc("GlobalLock")
	procGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
	procGlobalFree               = kernel32.NewProc("GlobalFree")
	procRtlMoveMemory            = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
	gmemZeroInit  = 0x0040
)

// markerOrder mirrors clipboardExclusionFormats in internal/inject (same order).
var markerOrder = []struct{ key, format string }{
	{"history", "CanIncludeInClipboardHistory"},
	{"cloud", "CanUploadToCloudClipboard"},
	{"monitor", "ExcludeClipboardContentFromMonitorProcessing"},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cliptest <none|all|history,cloud,monitor> [text]")
		os.Exit(2)
	}
	want := map[string]bool{}
	switch os.Args[1] {
	case "none":
	case "all":
		for _, m := range markerOrder {
			want[m.key] = true
		}
	default:
		for _, m := range strings.Split(os.Args[1], ",") {
			want[strings.TrimSpace(m)] = true
		}
	}
	text := "CLIPTEST-" + strings.ToUpper(os.Args[1])
	if len(os.Args) >= 3 {
		text = os.Args[2]
	}

	data, err := syscall.UTF16FromString(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	if r, _, e := procOpenClipboard.Call(0); r == 0 {
		fmt.Fprintln(os.Stderr, "OpenClipboard:", e)
		os.Exit(1)
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	size := uintptr(len(data) * 2)
	h, _, _ := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, size)
	p, _, _ := procGlobalLock.Call(h)
	procRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&data[0])), size)
	procGlobalUnlock.Call(h)
	if r, _, e := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		fmt.Fprintln(os.Stderr, "SetClipboardData text:", e)
		os.Exit(1)
	}

	var applied []string
	for _, m := range markerOrder {
		if !want[m.key] {
			continue
		}
		fp, _ := syscall.UTF16PtrFromString(m.format)
		f, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(fp)))
		if f == 0 {
			continue
		}
		mh, _, _ := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, unsafe.Sizeof(uint32(0)))
		if mh == 0 {
			continue
		}
		if r, _, _ := procSetClipboardData.Call(f, mh); r == 0 {
			procGlobalFree.Call(mh)
		} else {
			applied = append(applied, m.key)
		}
	}
	fmt.Printf("clipboard set: text=%q markers=%v — now Ctrl+V into Notepad++\n", text, applied)
}
