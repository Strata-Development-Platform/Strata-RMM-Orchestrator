//go:build !darwin

package remote

import "image"

func getDarwinDisplayBounds() (image.Rectangle, error) {
	return image.Rect(0, 0, 1920, 1080), nil
}

func captureDarwin() (image.Image, error) {
	return nil, nil
}

func injectDarwinMouseMove(x, y float64) error {
	return nil
}

func injectDarwinMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectDarwinKey(key string, down bool, mod ModifierKeys) error {
	return nil
}
