// Package speak says a short sentence out loud via the Windows built-in speech
// engine (SAPI 5, the `SpVoice` COM object) — no audio files and no external
// dependency, just ole32 + COM through syscall like the rest of internal/.
//
// It exists for one job: when a dictation fails for an actionable, common
// reason (a muted/disconnected microphone), the generic error cue is ambiguous
// — the same double-pulse also means "backend down", "window gone", etc. A
// spoken sentence ("Inget ljud. Är mikrofonen påslagen?") is unmissable (the
// clinician hears it like the start/stop cues, even while looking at the
// journal, not the tray) and self-explanatory. It is best-effort: if SAPI is
// unavailable the call is a silent no-op and never disrupts dictation.
package speak

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
)

// guid mirrors Win32 GUID.
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// CLSID_SpVoice {96749377-3391-11D2-9EE3-00C04F797396} and
// IID_ISpVoice {6C44DF74-72B9-4992-A1EC-EF996E0422D4}.
var (
	clsidSpVoice = guid{0x96749377, 0x3391, 0x11d2, [8]byte{0x9e, 0xe3, 0x00, 0xc0, 0x4f, 0x79, 0x73, 0x96}}
	iidISpVoice  = guid{0x6c44df74, 0x72b9, 0x4992, [8]byte{0xa1, 0xec, 0xef, 0x99, 0x6e, 0x04, 0x22, 0xd4}}
)

const (
	coinitMultithreaded = 0x0 // COINIT_MULTITHREADED — no message pump needed
	clsctxInprocServer  = 0x1
	spfIsNotXML         = 0x10 // treat the text literally, never as SAPI XML
)

// spVoiceVtbl lays out the ISpVoice COM vtable up to the methods we call.
// ISpVoice : ISpEventSource : ISpNotifySource : IUnknown, so Release is at
// index 2 (IUnknown) and Speak at index 20. The padding arrays skip the methods
// in between so the named fields land at the right offsets without any pointer
// arithmetic (which `go vet` rightly flags).
type spVoiceVtbl struct {
	queryInterface uintptr
	addRef         uintptr
	Release        uintptr
	_              [7]uintptr // ISpNotifySource (indices 3..9)
	_              [3]uintptr // ISpEventSource (10..12)
	_              [7]uintptr // ISpVoice SetOutput..GetVoice (13..19)
	Speak          uintptr    // index 20
}

// spVoice is a COM object: its first word is the vtable pointer.
type spVoice struct {
	vtbl *spVoiceVtbl
}

// Say speaks text synchronously and returns when the speech is done. It runs
// the whole COM lifecycle (initialize → create voice → speak → release →
// uninitialize) on a single locked OS thread, so it is self-contained and safe
// to launch as its own goroutine: `go speak.Say(...)`. Synchronous Speak (no
// SPF_ASYNC) means the voice object is released only after playback finishes,
// avoiding a cut-off. Best-effort: any failure (SAPI missing, COM error) leaves
// the function a silent no-op.
func Say(text string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	// S_OK (0) and S_FALSE (1) both mean this thread is now initialized and we
	// own the matching CoUninitialize. RPC_E_CHANGED_MODE means it was already
	// initialized in another mode by someone else — do not uninitialize then.
	if hr == 0 || hr == 1 {
		defer procCoUninitialize.Call()
	}

	var ptr unsafe.Pointer
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidSpVoice)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidISpVoice)),
		uintptr(unsafe.Pointer(&ptr)),
	)
	if r != 0 || ptr == nil {
		return // SAPI unavailable — no-op
	}
	voice := (*spVoice)(ptr)
	defer syscall.SyscallN(voice.vtbl.Release, uintptr(ptr))

	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	syscall.SyscallN(voice.vtbl.Speak, uintptr(ptr), uintptr(unsafe.Pointer(p)), spfIsNotXML, 0)
	runtime.KeepAlive(p)
}
