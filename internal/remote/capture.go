package remote

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"runtime"
	"sync"
	"time"
)

type Frame struct {
	Data      []byte
	Width     int
	Height    int
	Timestamp int64
	Format    string
	Seq       int64
}

type CaptureConfig struct {
	Width    int
	Height   int
	FPS      int
	Quality  int
	Format   string
	Compress bool
	Region   *image.Rectangle
	Scale    float64
}

type Capturer interface {
	Init() error
	Capture() (*Frame, error)
	Close() error
}

type capturer struct {
	width    int
	height   int
	fps      int
	quality  int
	format   string
	compress bool
	region   *image.Rectangle
	scale    float64
	closed   bool
	mu       sync.Mutex
	seq      int64
	source   ScreenSource
}

func NewCapturer() Capturer {
	width, height := getPrimaryMonitorSize()
	return &capturer{
		width:    width,
		height:   height,
		fps:      30,
		quality:  80,
		format:   "jpeg",
		compress: true,
		scale:    1.0,
		source:   getScreenSource(),
	}
}

func NewCapturerWithConfig(config CaptureConfig) Capturer {
	c := &capturer{
		width:    config.Width,
		height:   config.Height,
		fps:      config.FPS,
		quality:  config.Quality,
		format:   config.Format,
		compress: config.Compress,
		region:   config.Region,
		scale:    config.Scale,
		source:   getScreenSource(),
	}

	if c.width == 0 || c.height == 0 {
		c.width, c.height = getPrimaryMonitorSize()
	}

	if c.width > 0 && c.height > 0 && c.scale > 0 && c.scale != 1.0 {
		c.width = int(float64(c.width) * c.scale)
		c.height = int(float64(c.height) * c.scale)
	}

	return c
}

func (c *capturer) Init() error {
	return nil
}

func (c *capturer) Capture() (*Frame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("capturer closed")
	}

	c.seq++

	img, err := c.source.Capture()
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}

	if c.region != nil {
		img = cropImage(img, *c.region)
	}

	if c.scale != 1.0 {
		img = scaleImage(img, c.scale)
	}

	frame := &Frame{
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
		Timestamp: time.Now().UnixMilli(),
		Seq:       c.seq,
	}

	if c.compress {
		buf := &bytes.Buffer{}
		opts := &jpeg.Options{Quality: c.quality}
		if err := jpeg.Encode(buf, img, opts); err != nil {
			return nil, fmt.Errorf("encode: %w", err)
		}
		frame.Format = "jpeg"
		frame.Data = buf.Bytes()
	} else {
		frame.Format = "raw"
		frame.Data = convertToYUV420(img)
	}

	return frame, nil
}

func (c *capturer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if closer, ok := c.source.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (c *capturer) SetFPS(fps int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fps = fps
}

func (c *capturer) SetQuality(quality int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.quality = quality
}

func getPrimaryMonitorSize() (int, int) {
	switch runtime.GOOS {
	case "windows":
		return getWindowsMonitorSize()
	case "darwin":
		return getDarwinMonitorSize()
	case "linux":
		return getLinuxMonitorSize()
	default:
		return 1920, 1080
	}
}

func getWindowsMonitorSize() (int, int) {
	bounds, err := getWindowsDisplayBounds()
	if err != nil {
		return 1920, 1080
	}
	return bounds.Dx(), bounds.Dy()
}

func getDarwinMonitorSize() (int, int) {
	bounds, err := getDarwinDisplayBounds()
	if err != nil {
		return 1920, 1080
	}
	return bounds.Dx(), bounds.Dy()
}

func getLinuxMonitorSize() (int, int) {
	bounds, err := getLinuxDisplayBounds()
	if err != nil {
		return 1920, 1080
	}
	return bounds.Dx(), bounds.Dy()
}

type ScreenSource interface {
	Capture() (image.Image, error)
	Name() string
}

type captureFunc func() (image.Image, error)

func (fn captureFunc) Capture() (image.Image, error) { return fn() }
func (fn captureFunc) Name() string                  { return "built-in" }

func getScreenSource() ScreenSource {
	switch runtime.GOOS {
	case "windows":
		return captureFunc(captureWindows)
	case "darwin":
		return captureFunc(captureDarwin)
	case "linux":
		return captureFunc(captureLinux)
	default:
		return captureFunc(captureFallback)
	}
}

func captureFallback() (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 1920, 1080)), nil
}

func captureScreen() (image.Image, error) {
	src := getScreenSource()
	return src.Capture()
}

func convertToYUV420(img image.Image) []byte {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	ySize := width * height
	uSize := (width / 2) * (height / 2)
	vSize := (width / 2) * (height / 2)

	buf := make([]byte, ySize+uSize+vSize)
	yIdx := 0
	uIdx := ySize
	vIdx := ySize + uSize

	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			c1 := img.At(x, y).(color.RGBA)
			y1 := uint8((float64(c1.R) + float64(c1.G) + float64(c1.B)) / 3)

			var c2, c3, c4 color.RGBA
			if x+1 < width {
				c2 = img.At(x+1, y).(color.RGBA)
			}
			if y+1 < height {
				c3 = img.At(x, y+1).(color.RGBA)
			}
			if x+1 < width && y+1 < height {
				c4 = img.At(x+1, y+1).(color.RGBA)
			}

			y2 := uint8((float64(c2.R) + float64(c2.G) + float64(c2.B)) / 3)
			y3 := uint8((float64(c3.R) + float64(c3.G) + float64(c3.B)) / 3)
			y4 := uint8((float64(c4.R) + float64(c4.G) + float64(c4.B)) / 3)

			u := uint8(float64(c1.B-c1.R)/2 + 128)
			v := uint8(float64(c1.R-c1.G)/4 + float64(c1.B-c1.G)/4 + 128)

			buf[yIdx] = y1
			yIdx++
			if x+1 < width {
				buf[yIdx] = y2
				yIdx++
			}
			if y+1 < height {
				buf[yIdx] = y3
				yIdx++
			}
			if x+1 < width && y+1 < height {
				buf[yIdx] = y4
				yIdx++
			}

			buf[uIdx] = u
			buf[vIdx] = v
			uIdx++
			vIdx++
		}
	}

	return buf
}

func scaleImage(src image.Image, scale float64) image.Image {
	if scale == 1.0 {
		return src
	}

	bounds := src.Bounds()
	newWidth := int(float64(bounds.Dx()) * scale)
	newHeight := int(float64(bounds.Dy()) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) / scale)
			if srcX >= bounds.Dx() {
				srcX = bounds.Dx() - 1
			}
			if srcY >= bounds.Dy() {
				srcY = bounds.Dy() - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func cropImage(img image.Image, rect image.Rectangle) image.Image {
	bounds := img.Bounds()
	rect = rect.Intersect(bounds)
	if rect.Empty() {
		return img
	}

	dst := image.NewRGBA(rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			dst.Set(x-rect.Min.X, y-rect.Min.Y, img.At(x, y))
		}
	}
	return dst
}

func encodeFrame(img image.Image, format string, quality int) ([]byte, error) {
	buf := &bytes.Buffer{}
	switch format {
	case "png":
		return nil, fmt.Errorf("png encoding not yet implemented for real capture")
	case "jpeg":
		opts := &jpeg.Options{Quality: quality}
		if err := jpeg.Encode(buf, img, opts); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		opts := &jpeg.Options{Quality: quality}
		if err := jpeg.Encode(buf, img, opts); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}
