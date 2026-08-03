//go:build !linux

package remote

import "image"

func getLinuxDisplayBounds() (image.Rectangle, error) {
	return image.Rect(0, 0, 1920, 1080), nil
}

func captureLinux() (image.Image, error) {
	return nil, nil
}

func injectLinuxMouseMove(x, y float64) error {
	return nil
}

func injectLinuxMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectLinuxKey(key string, down bool, mod ModifierKeys) error {
	return nil
}
