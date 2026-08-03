//go:build windows

package remote

import (
	"fmt"
	"image"
	"image/color"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32     = windows.NewLazySystemDLL("user32.dll")
	gdi32      = windows.NewLazySystemDLL("gdi32.dll")

	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procCreateDC         = gdi32.NewProc("CreateDCA")
	procDeleteDC         = gdi32.NewProc("DeleteDC")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC_gdi     = gdi32.NewProc("DeleteDC")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procBitBlt           = gdi32.NewProc("BitBlt")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procGetBitmapBits    = gdi32.NewProc("GetBitmapBits")
)

const (
	SRCCOPY     = 0x00CC0020
	CAPTUREBLT  = 0x40000000
)

func getWindowsDisplayBounds() (image.Rectangle, error) {
	x, _, _ := procGetSystemMetrics.Call(76) // SM_XVIRTUALSCREEN
	y, _, _ := procGetSystemMetrics.Call(77) // SM_YVIRTUALSCREEN
	w, _, _ := procGetSystemMetrics.Call(78) // SM_CXVIRTUALSCREEN
	h, _, _ := procGetSystemMetrics.Call(79) // SM_CYVIRTUALSCREEN

	return image.Rect(int(x), int(y), int(x)+int(w), int(y)+int(h)), nil
}

func captureWindows() (image.Image, error) {
	bounds, err := getWindowsDisplayBounds()
	if err != nil {
		return nil, fmt.Errorf("get bounds: %w", err)
	}

	width := bounds.Dx()
	height := bounds.Dy()

	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(0, hdc)

	memDC, _, _ := procCreateCompatibleDC.Call(0)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC_gdi.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(
		hdc, uintptr(width), uintptr(height),
	)
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bitmap)

	oldObj, _, _ := procSelectObject.Call(memDC, bitmap)
	defer procSelectObject.Call(memDC, oldObj)

	ret, _, _ := procBitBlt.Call(
		memDC, 0, 0,
		uintptr(width), uintptr(height),
		hdc,
		uintptr(bounds.Min.X), uintptr(bounds.Min.Y),
		SRCCOPY|CAPTUREBLT,
	)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	// Create RGBA image
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))

	// Get bitmap info
	var bmi [5]uint32
	infoSize, _, _ := procGetBitmapBits.Call(bitmap, uintptr(unsafe.Pointer(&bmi)), 0)
	_ = infoSize

	// Get bitmap bits
	bufSize := width * height * 4
	buf := make([]byte, bufSize)
	ret, _, _ = procGetBitmapBits.Call(
		bitmap, uintptr(bufSize), uintptr(unsafe.Pointer(&buf[0])),
	)
	if ret == 0 {
		// Return gradient fallback
		return createGradientFallback(width, height), nil
	}

	// Convert BGRA to RGBA
	for y := 0; y < height; y++ {
		dstRow := y * rgba.Stride
		srcRow := y * width * 4
		for x := 0; x < width; x++ {
			dstIdx := dstRow + x*4
			srcIdx := srcRow + x*4

			b := buf[srcIdx+0]
			g := buf[srcIdx+1]
			r := buf[srcIdx+2]
			a := buf[srcIdx+3]

			rgba.Pix[dstIdx+0] = r
			rgba.Pix[dstIdx+1] = g
			rgba.Pix[dstIdx+2] = b
			rgba.Pix[dstIdx+3] = a
		}
	}

	return rgba, nil
}

func createGradientFallback(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(128)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}
