//go:build linux
// +build linux

package remotecontrol

import (
	"bytes"
	"image"
	"image/png"
	"os/exec"
	"strconv"
	"strings"
)

type linuxX11Capturer struct {
	display string
	width   int
	height  int
	haveImport bool
}

func NewCapturer() Capturer {
	return &linuxX11Capturer{width: 1280, height: 720}
}

func (c *linuxX11Capturer) Init() error {
	c.display = ":0"
	c.haveImport = exec.Command("import", "-version").Run() == nil

	if c.haveImport {
		out, err := exec.Command("xdpyinfo", "-display", c.display).Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "dimensions:") {
					parts := strings.Fields(line)
					if len(parts) > 1 {
						dims := strings.Split(parts[1], "x")
						if len(dims) == 2 {
							w, _ := strconv.Atoi(dims[0])
							h, _ := strconv.Atoi(dims[1])
							if w > 0 && h > 0 {
								c.width = w
								c.height = h
							}
						}
					}
					break
				}
			}
		}
	}
	return nil
}

func (c *linuxX11Capturer) Capture() (*image.RGBA, error) {
	if c.haveImport {
		out, err := exec.Command("import", "-display", c.display, "-window", "root", "png:-").Output()
		if err == nil && len(out) > 100 {
			decoded, err := png.Decode(bytes.NewReader(out))
			if err == nil {
				bounds := decoded.Bounds()
				dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
				for y := 0; y < bounds.Dy(); y++ {
					for x := 0; x < bounds.Dx(); x++ {
						dst.Set(x, y, decoded.At(x, y))
					}
				}
				return dst, nil
			}
		}
	}

	return image.NewRGBA(image.Rect(0, 0, c.width, c.height)), nil
}

func (c *linuxX11Capturer) Close() error {
	return nil
}

type linuxX11Injector struct {
	display string
}

func NewInjector() InputInjector {
	return &linuxX11Injector{}
}

func (inj *linuxX11Injector) Init() error {
	inj.display = ":0"
	return nil
}

func (inj *linuxX11Injector) SendMouseMove(x, y float64) error {
	return exec.Command("xdotool", "mousemove", "--display", inj.display,
		strconv.Itoa(int(x)), strconv.Itoa(int(y))).Run()
}

func (inj *linuxX11Injector) SendMouseClick(button int, down bool) error {
	btn := strconv.Itoa(button + 1)
	if down {
		return exec.Command("xdotool", "mousedown", "--display", inj.display, btn).Run()
	}
	return exec.Command("xdotool", "mouseup", "--display", inj.display, btn).Run()
}

func (inj *linuxX11Injector) SendKey(key string, down bool, mod int) error {
	action := "keydown"
	if !down {
		action = "keyup"
	}
	return exec.Command("xdotool", action, "--display", inj.display, key).Run()
}

func (inj *linuxX11Injector) Close() error {
	return nil
}
