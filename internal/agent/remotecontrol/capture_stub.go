//go:build !linux && !windows && !darwin
// +build !linux,!windows,!darwin

package remotecontrol

import (
	"image"
	"image/color"
	"math"
	"time"
)

type softwareCapturer struct {
	width  int
	height int
	start  time.Time
}

type softwareInjector struct{}

func NewCapturer() Capturer {
	return &softwareCapturer{width: 1280, height: 720, start: time.Now()}
}

func (c *softwareCapturer) Init() error { return nil }

func (c *softwareCapturer) Capture() (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, c.width, c.height))
	elapsed := time.Since(c.start).Seconds()
	for y := 0; y < c.height; y++ {
		for x := 0; x < c.width; x++ {
			r := uint8(128 + 64*math.Sin(float64(x)*0.02+elapsed))
			g := uint8(128 + 64*math.Sin(float64(y)*0.02+elapsed*1.3))
			b := uint8(128 + 64*math.Sin((float64(x)+float64(y))*0.01+elapsed*0.7))
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	offset := int(elapsed*60) % c.height
	for y := 0; y < 40; y++ {
		for x := 0; x < c.width; x++ {
			row := (y + offset) % c.height
			img.Set(x, row, color.RGBA{255, 255, 255, 255})
		}
	}
	return img, nil
}

func (c *softwareCapturer) Close() error { return nil }

func NewInjector() InputInjector {
	return &softwareInjector{}
}

func (inj *softwareInjector) Init() error                                  { return nil }
func (inj *softwareInjector) SendMouseMove(x, y float64) error             { return nil }
func (inj *softwareInjector) SendMouseClick(button int, down bool) error   { return nil }
func (inj *softwareInjector) SendKey(key string, down bool, mod int) error { return nil }
func (inj *softwareInjector) Close() error                                 { return nil }
