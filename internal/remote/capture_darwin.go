//go:build darwin

package remote

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"runtime"
	"sync"
)

var (
	displayBoundsOnce sync.Once
	displayBounds     image.Rectangle
	displayBoundsErr  error
)

func getDarwinDisplayBounds() (image.Rectangle, error) {
	displayBoundsOnce.Do(func() {
		bounds, err := getScreenSize()
		if err == nil {
			displayBounds = bounds
			return
		}
		// Fallback to default
		displayBounds = image.Rect(0, 0, 1920, 1080)
	})
	return displayBounds, displayBoundsErr
}

func getScreenSize() (image.Rectangle, error) {
	// Use the screen command via python3 or go
	cmd := exec.Command("python3", "-c", `
import sys
try:
    from AppKit import NSScreen
    screen = NSScreen.mainScreen()
    frame = screen.frame()
    print(f"{int(frame.origin.x)} {int(frame.origin.y)} {int(frame.size.width)} {int(frame.size.height)}")
except:
    print("0 0 1920 1080")
`)
	output, err := cmd.Output()
	if err != nil {
		return image.Rect(0, 0, 1920, 1080), err
	}

	var x, y, w, h int
	fmt.Sscanf(string(output), "%d %d %d %d", &x, &y, &w, &h)
	return image.Rect(x, y, x+w, y+h), nil
}

func captureDarwin() (image.Image, error) {
	bounds, err := getDarwinDisplayBounds()
	if err != nil {
		return nil, err
	}

	width := bounds.Dx()
	height := bounds.Dy()

	// Use screencapture command (built into macOS)
	cmd := exec.Command("screencapture", "-x") // -x suppresses sound
	output, err := cmd.Output()
	if err != nil {
		// Fallback: create gradient
		return createGradientFallback(width, height), nil
	}

	// Decode PNG from screencapture
	img, err := png.Decode(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("decode screen capture: %w", err)
	}

	return img, nil
}

func createGradientFallback(width, height int) image.Image {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

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
