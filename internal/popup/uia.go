// UI Automation support for anchoring the popup to the actual text
// selection. The legacy system caret (GetGUIThreadInfo) is reported
// inconsistently by Chromium/Electron apps — the web-based journal and the
// editor this tool targets — so when it is missing we ask UI Automation for
// the bounding rectangle of the focused element's current text selection,
// which Chromium reports reliably. All of it is best-effort: any failure
// falls back to the caret and then the mouse cursor (see anchorPoint).
//
// Pure Win32/COM via syscall, no cgo, matching the rest of the package. The
// COM work runs on a dedicated, apartment-isolated goroutine so it never
// interferes with the popup's message-pump thread, and it is wrapped in a
// timeout so an unresponsive target window can never hang the popup. COM
// object handles are held as unsafe.Pointer (not uintptr) so vtable reads use
// unsafe.Add rather than pointer arithmetic, keeping the code vet-clean.
package popup

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	ole32    = syscall.NewLazyDLL("ole32.dll")
	oleaut32 = syscall.NewLazyDLL("oleaut32.dll")

	procCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	procCoUninitialize        = ole32.NewProc("CoUninitialize")
	procCoCreateInstance      = ole32.NewProc("CoCreateInstance")
	procSafeArrayAccessData   = oleaut32.NewProc("SafeArrayAccessData")
	procSafeArrayUnaccessData = oleaut32.NewProc("SafeArrayUnaccessData")
	procSafeArrayGetUBound    = oleaut32.NewProc("SafeArrayGetUBound")
	procSafeArrayDestroy      = oleaut32.NewProc("SafeArrayDestroy")
)

// guid mirrors the Win32 GUID struct for CoCreateInstance / GetCurrentPatternAs.
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidCUIAutomation          = guid{0xFF48DBA4, 0x60EF, 0x4201, [8]byte{0xAA, 0x87, 0x54, 0x10, 0x3E, 0xEF, 0x59, 0x4E}}
	iidIUIAutomation            = guid{0x30CBE57D, 0xD9D0, 0x452A, [8]byte{0xAB, 0x13, 0x7A, 0xC5, 0xAC, 0x48, 0x25, 0xEE}}
	iidIUIAutomationTextPattern = guid{0x32EBA289, 0x3583, 0x42C9, [8]byte{0x9C, 0x59, 0x3B, 0x6D, 0x9A, 0x1E, 0x9B, 0x6A}}
)

const (
	coinitMultithreaded = 0x0
	clsctxInprocServer  = 0x1
	uiaTextPatternID    = 10014

	uiaQueryTimeout = 500 * time.Millisecond
)

// COM vtable indices (after IUnknown's QueryInterface=0, AddRef=1, Release=2).
const (
	idxRelease = 2

	idxAutomationGetFocusedElement = 8  // IUIAutomation::GetFocusedElement
	idxElementGetCurrentPatternAs  = 14 // IUIAutomationElement::GetCurrentPatternAs
	idxTextPatternGetSelection     = 5  // IUIAutomationTextPattern::GetSelection
	idxRangeArrayGetElement        = 4  // IUIAutomationTextRangeArray::GetElement
	idxTextRangeGetBoundingRects   = 10 // IUIAutomationTextRange::GetBoundingRectangles
)

const ptrSize = int(unsafe.Sizeof(uintptr(0)))

// comCall invokes the COM method at vtable[idx] on `this`, passing `this` as
// the implicit first argument, and returns the HRESULT.
func comCall(this unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(this)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*ptrSize))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(this))
	all = append(all, args...)
	ret, _, _ := syscall.SyscallN(fn, all...)
	return ret
}

func release(this unsafe.Pointer) {
	if this != nil {
		comCall(this, idxRelease)
	}
}

// selectionRect returns the screen rectangle of the focused element's text
// selection via UI Automation, or ok=false when unavailable. The COM query
// runs on its own goroutine (isolated COM apartment, no UI work) and is
// bounded by uiaQueryTimeout so an unresponsive window cannot hang the popup.
func selectionRect() (rect, bool) {
	type result struct {
		r  rect
		ok bool
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r, ok := querySelectionRect()
		ch <- result{r, ok}
	}()

	select {
	case res := <-ch:
		return res.r, res.ok
	case <-time.After(uiaQueryTimeout):
		return rect{}, false
	}
}

// querySelectionRect walks IUIAutomation → focused element → TextPattern →
// selection → bounding rectangles and returns the first rectangle (the first
// line of the selection) in screen coordinates. Every COM object is released
// and COM is uninitialised on return. Any failed step returns ok=false.
func querySelectionRect() (rect, bool) {
	if hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded); int32(hr) < 0 {
		return rect{}, false // S_OK (0) and S_FALSE (1) are both fine
	}
	defer procCoUninitialize.Call()

	var automation unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidCUIAutomation)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIUIAutomation)),
		uintptr(unsafe.Pointer(&automation)),
	)
	if int32(hr) < 0 || automation == nil {
		return rect{}, false
	}
	defer release(automation)

	var element unsafe.Pointer
	if comCall(automation, idxAutomationGetFocusedElement, uintptr(unsafe.Pointer(&element))) != 0 || element == nil {
		return rect{}, false
	}
	defer release(element)

	var textPattern unsafe.Pointer
	if comCall(element, idxElementGetCurrentPatternAs,
		uiaTextPatternID,
		uintptr(unsafe.Pointer(&iidIUIAutomationTextPattern)),
		uintptr(unsafe.Pointer(&textPattern)),
	) != 0 || textPattern == nil {
		return rect{}, false // app exposes no text pattern (e.g. a non-text control)
	}
	defer release(textPattern)

	var rangeArray unsafe.Pointer
	if comCall(textPattern, idxTextPatternGetSelection, uintptr(unsafe.Pointer(&rangeArray))) != 0 || rangeArray == nil {
		return rect{}, false
	}
	defer release(rangeArray)

	var textRange unsafe.Pointer
	if comCall(rangeArray, idxRangeArrayGetElement, 0, uintptr(unsafe.Pointer(&textRange))) != 0 || textRange == nil {
		return rect{}, false
	}
	defer release(textRange)

	// SAFEARRAY of float64: groups of 4 (left, top, width, height) per line,
	// in screen coordinates. We take the first line.
	var psa unsafe.Pointer
	if comCall(textRange, idxTextRangeGetBoundingRects, uintptr(unsafe.Pointer(&psa))) != 0 || psa == nil {
		return rect{}, false
	}
	defer procSafeArrayDestroy.Call(uintptr(psa))

	var ub int32
	if r, _, _ := procSafeArrayGetUBound.Call(uintptr(psa), 1, uintptr(unsafe.Pointer(&ub))); int32(r) < 0 || ub < 3 {
		return rect{}, false // fewer than 4 doubles → no usable rectangle
	}

	var data unsafe.Pointer
	if r, _, _ := procSafeArrayAccessData.Call(uintptr(psa), uintptr(unsafe.Pointer(&data))); int32(r) < 0 || data == nil {
		return rect{}, false
	}
	left := *(*float64)(data)
	top := *(*float64)(unsafe.Add(data, 8))
	width := *(*float64)(unsafe.Add(data, 16))
	height := *(*float64)(unsafe.Add(data, 24))
	procSafeArrayUnaccessData.Call(uintptr(psa))

	if width <= 0 || height <= 0 {
		return rect{}, false
	}
	return rect{
		left:   int32(left),
		top:    int32(top),
		right:  int32(left + width),
		bottom: int32(top + height),
	}, true
}
