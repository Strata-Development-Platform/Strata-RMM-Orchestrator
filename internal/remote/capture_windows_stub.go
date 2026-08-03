//go:build !windows

package remote

import "image"

func getWindowsDisplayBounds() (image.Rectangle, error) {
	return image.Rect(0, 0, 1920, 1080), nil
}

func captureWindows() (image.Image, error) {
	return nil, nil
}

func injectWindowsMouseMove(x, y float64) error {
	return nil
}

func injectWindowsMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectWindowsKey(key string, down bool, mod ModifierKeys) error {
	return nil
}
