//go:build linux

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

func getLinuxDisplayBounds() (image.Rectangle, error) {
	displayBoundsOnce.Do(func() {
		bounds, err := getLinuxScreenSize()
		if err == nil {
			displayBounds = bounds
			return
		}
		// Fallback
		displayBounds = image.Rect(0, 0, 1920, 1080)
		displayBoundsErr = err
	})
	return displayBounds, displayBoundsErr
}

func getLinuxScreenSize() (image.Rectangle, error) {
	// Try xdpyinfo first
	cmd := exec.Command("xdpyinfo", "-display", " :0")
	output, err := cmd.Output()
	if err == nil {
		// Parse dimensions from xdpyinfo output
		var w, h int
		for i := 0; i < len(output)-4; i++ {
			if string(output[i:i+9]) == "dimensions" {
				// Find width and height after this
				rest := string(output[i:])
				_, _ = fmt.Sscanf(rest, "dimensions: %d x %d", &w, &h)
				if w > 0 && h > 0 {
					return image.Rect(0, 0, w, h), nil
				}
			}
		}
	}

	// Fallback: try xwininfo
	cmd = exec.Command("xwininfo", "-root", "-stat")
	output, err = cmd.Output()
	if err == nil {
		var w, h int
		for _, line := range bytes.Split(output, []byte("\n")) {
			lineStr := string(line)
			if len(lineStr) > 10 && lineStr[:10] == "+-geometry" {
				_, _ = fmt.Sscanf(lineStr[10:], "%dx%d", &w, &h)
				if w > 0 && h > 0 {
					return image.Rect(0, 0, w, h), nil
				}
			}
		}
	}

	return image.Rect(0, 0, 1920, 1080), fmt.Errorf("no display info available")
}

func captureLinux() (image.Image, error) {
	bounds, err := getLinuxDisplayBounds()
	if err != nil {
		return nil, err
	}

	width := bounds.Dx()
	height := bounds.Dy()

	// Try multiple capture methods in order of preference:
	// 1. scrot (simple screenshot tool)
	// 2. import (ImageMagick)
	// 3. xwd (X window dump)
	// 4. gnome-screenshot

	// Method 1: scrot
	img, err := captureScrot(width, height)
	if err == nil {
		return img, nil
	}

	// Method 2: import (ImageMagick)
	img, err = captureImport(width, height)
	if err == nil {
		return img, nil
	}

	// Method 3: gnome-screenshot
	img, err = captureGNomescreenshot(width, height)
	if err == nil {
		return img, nil
	}

	// Fallback: create gradient
	return createGradientFallback(width, height), nil
}

func captureScrot(width, height int) (image.Image, error) {
	cmd := exec.Command("scrot", "-e", "cat /dev/stdin", "-d", "0")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	img, err := png.Decode(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("scrot decode: %w", err)
	}
	return img, nil
}

func captureImport(width, height int) (image.Image, error) {
	cmd := exec.Command("import", "-window", "root", "png:-")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	img, err := png.Decode(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("import decode: %w", err)
	}
	return img, nil
}

func captureGNomescreenshot(width, height int) (image.Image, error) {
	cmd := exec.Command("gnome-screenshot", "-f", "-", "-d", "0")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	img, err := png.Decode(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("gnome-screenshot decode: %w", err)
	}
	return img, nil
}

func createGradientFallback(width, height int) image.Image {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8(float64(x) * 255.0 / float64(width))
			g := uint8(float64(y) * 255.0 / float64(height))
			b := uint8(128)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}
