//go:build darwin
// +build darwin

package remotecontrol

import (
	"image"
)

type darwinCapturer struct {
	width  int
	height int
}

func NewCapturer() Capturer {
	return &darwinCapturer{width: 1920, height: 1080}
}

func (c *darwinCapturer) Init() error {
	return nil
}

func (c *darwinCapturer) Capture() (*image.RGBA, error) {
	return image.NewRGBA(image.Rect(0, 0, c.width, c.height)), nil
}

func (c *darwinCapturer) Close() error {
	return nil
}

type darwinInjector struct{}

func NewInjector() InputInjector {
	return &darwinInjector{}
}

func (inj *darwinInjector) Init() error { return nil }
func (inj *darwinInjector) SendMouseMove(x, y float64) error { return nil }
func (inj *darwinInjector) SendMouseClick(button int, down bool) error { return nil }
func (inj *darwinInjector) SendKey(key string, down bool, mod int) error { return nil }
func (inj *darwinInjector) Close() error { return nil }
