//go:build windows

package glow

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/paint"
)

const frameInterval = 33 * time.Millisecond

func init() {
	Backend = "windows"
	Run = runWindows
	Install = func(st *config.InstallState) (string, error) {
		if st == nil {
			return "", errors.New("glow: nil install state")
		}
		return "", nil
	}
	Uninstall = func(st *config.InstallState) error {
		if st == nil {
			return errors.New("glow: nil install state")
		}
		return nil
	}
}

// runWindows draws the frame in a click-through layered window on the
// primary monitor for d. All Win32 calls happen on one locked OS thread.
func runWindows(ctx context.Context, d time.Duration) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		done <- renderLoop(ctx, d)
	}()
	return <-done
}

func renderLoop(ctx context.Context, d time.Duration) error {
	// Without this the process is DPI-virtualized at >100% scaling:
	// GetSystemMetrics returns the scaled-down resolution and DWM upscales
	// the band, making it blurry and the wrong thickness.
	_, _, _ = pSetProcessDPIAware.Call()
	cx, _, _ := pGetSystemMetrics.Call(smCxScreen)
	cy, _, _ := pGetSystemMetrics.Call(smCyScreen)
	w, h := int(int32(cx)), int(int32(cy))
	if w <= 0 || h <= 0 {
		return errors.New("glow: no primary display")
	}
	inst, _, e := pGetModuleHandleW.Call(0)
	if inst == 0 {
		return fmt.Errorf("glow: GetModuleHandle: %v", e)
	}
	className := utf16ptr("PushItGlowWindow")
	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		WndProc:   wndProcPtr,
		Instance:  windows.Handle(inst),
		ClassName: className,
	}
	if atom, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		if e != windows.ERROR_CLASS_ALREADY_EXISTS {
			return fmt.Errorf("glow: RegisterClassEx: %v", e)
		}
	}
	defer pUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), inst)

	hwnd, _, e := pCreateWindowExW.Call(
		wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16ptr("push-it glow"))),
		wsPopup, 0, 0, uintptr(w), uintptr(h), 0, 0, inst, 0)
	if hwnd == 0 {
		return fmt.Errorf("glow: CreateWindowEx: %v", e)
	}
	defer pDestroyWindow.Call(hwnd)

	screenDC, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screenDC)
	memDC, _, _ := pCreateCompatibleDC.Call(screenDC)
	defer pDeleteDC.Call(memDC)

	bmi := bitmapInfoHeader{
		Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), // top-down
		Planes: 1, BitCount: 32, Compression: biRGB,
	}
	var bits unsafe.Pointer
	bmp, _, e := pCreateDIBSection.Call(screenDC, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return fmt.Errorf("glow: CreateDIBSection: %v", e)
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(memDC, bmp)
	defer pSelectObject.Call(memDC, old)

	// Zero once; RenderGlow then rewrites only the edge glow each frame and
	// leaves the (permanently transparent) interior alone.
	buf := unsafe.Slice((*byte)(bits), w*h*4)
	clear(buf)
	pShowWindow.Call(hwnd, swShowNoActivate)

	start := time.Now()
	deadline := start.Add(d)
	tick := time.NewTicker(frameInterval)
	defer tick.Stop()
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	sz := size{int32(w), int32(h)}
	src := point{0, 0}
	var m msg
	for {
		// Pump pending messages so the window stays responsive to the system.
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		paint.RenderGlow(buf, w, h, time.Since(start))
		if r, _, e := pUpdateLayeredWindow.Call(hwnd, screenDC, 0, uintptr(unsafe.Pointer(&sz)), memDC,
			uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha); r == 0 {
			return fmt.Errorf("glow: UpdateLayeredWindow: %v", e)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-tick.C:
			if now.After(deadline) {
				return nil
			}
		}
	}
}
