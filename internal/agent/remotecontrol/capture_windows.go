//go:build windows
// +build windows

package remotecontrol

import (
	"image"
)

type windowsCapturer struct {
	width  int
	height int
}

func NewCapturer() Capturer {
	return &windowsCapturer{width: 1920, height: 1080}
}

func (c *windowsCapturer) Init() error {
	return nil
}

func (c *windowsCapturer) Capture() (*image.RGBA, error) {
	return image.NewRGBA(image.Rect(0, 0, c.width, c.height)), nil
}

func (c *windowsCapturer) Close() error {
	return nil
}

type windowsInjector struct{}

func NewInjector() InputInjector {
	return &windowsInjector{}
}

func (inj *windowsInjector) Init() error                                  { return nil }
func (inj *windowsInjector) SendMouseMove(x, y float64) error             { return nil }
func (inj *windowsInjector) SendMouseClick(button int, down bool) error   { return nil }
func (inj *windowsInjector) SendKey(key string, down bool, mod int) error { return nil }
func (inj *windowsInjector) Close() error                                 { return nil }
