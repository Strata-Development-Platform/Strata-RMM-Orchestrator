//go:build !windows && !darwin && !linux

package remote

import (
	"fmt"
	"image"
)

func getWindowsDisplayBounds() (image.Rectangle, error) {
	return image.Rect(0, 0, 1920, 1080), fmt.Errorf("windows capture not supported")
}

func captureWindows() (image.Image, error) {
	return nil, fmt.Errorf("windows capture not supported")
}

func getDarwinDisplayBounds() (image.Rectangle, error) {
	return image.Rect(0, 0, 1920, 1080), fmt.Errorf("darwin capture not supported")
}

func captureDarwin() (image.Image, error) {
	return nil, fmt.Errorf("darwin capture not supported")
}

func getLinuxDisplayBounds() (image.Rectangle, error) {
	return image.Rect(0, 0, 1920, 1080), fmt.Errorf("linux capture not supported")
}

func captureLinux() (image.Image, error) {
	return nil, fmt.Errorf("linux capture not supported")
}
