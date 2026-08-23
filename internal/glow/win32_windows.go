//go:build windows

package glow

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	pRegisterClassExW    = user32.NewProc("RegisterClassExW")
	pCreateWindowExW     = user32.NewProc("CreateWindowExW")
	pDefWindowProcW      = user32.NewProc("DefWindowProcW")
	pDestroyWindow       = user32.NewProc("DestroyWindow")
	pShowWindow          = user32.NewProc("ShowWindow")
	pPeekMessageW        = user32.NewProc("PeekMessageW")
	pTranslateMessage    = user32.NewProc("TranslateMessage")
	pDispatchMessageW    = user32.NewProc("DispatchMessageW")
	pUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	pGetDC               = user32.NewProc("GetDC")
	pReleaseDC           = user32.NewProc("ReleaseDC")
	pUnregisterClassW    = user32.NewProc("UnregisterClassW")
	pSetProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
	// golang.org/x/sys/windows v0.47.0 exports neither GetSystemMetrics nor
	// GetModuleHandle (only GetModuleHandleEx), so both go through the same
	// lazy-proc mechanism as the rest of this file.
	pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	wsPopup          = 0x80000000
	wsExLayered      = 0x00080000
	wsExTransparent  = 0x00000020
	wsExTopmost      = 0x00000008
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	swShowNoActivate = 4
	pmRemove         = 1
	ulwAlpha         = 2
	acSrcOver        = 0
	acSrcAlpha       = 1
	biRGB            = 0
	dibRGBColors     = 0
	smCxScreen       = 0
	smCyScreen       = 1
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct{ X, Y int32 }
type size struct{ CX, CY int32 }

type msg struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type blendFunction struct {
	BlendOp, BlendFlags, SourceConstantAlpha, AlphaFormat byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Compile-time C ABI checks. Windows amd64 and arm64 are both little-endian
// LP64 with natural alignment, so these sizes must match the SDK exactly on
// both. A mismatch fails the build (uintptr constant overflow) rather than
// corrupting memory at runtime.
const (
	_ = unsafe.Sizeof(wndClassExW{}) - 80
	_ = 80 - unsafe.Sizeof(wndClassExW{})
	_ = unsafe.Sizeof(msg{}) - 48
	_ = 48 - unsafe.Sizeof(msg{})
	_ = unsafe.Sizeof(point{}) - 8
	_ = 8 - unsafe.Sizeof(point{})
	_ = unsafe.Sizeof(size{}) - 8
	_ = 8 - unsafe.Sizeof(size{})
	_ = unsafe.Sizeof(blendFunction{}) - 4
	_ = 4 - unsafe.Sizeof(blendFunction{})
	_ = unsafe.Sizeof(bitmapInfoHeader{}) - 40
	_ = 40 - unsafe.Sizeof(bitmapInfoHeader{})
)

func wndProc(hwnd windows.Handle, m uint32, wp, lp uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(m), wp, lp)
	return r
}

var wndProcPtr = syscall.NewCallback(wndProc)

func utf16ptr(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }
