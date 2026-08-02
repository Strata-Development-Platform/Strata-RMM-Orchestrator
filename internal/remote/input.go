package remote

import (
	"fmt"
	"runtime"
	"sync"
)

type InputType string

const (
	InputMouseMove   InputType = "mousemove"
	InputMouseDown   InputType = "mousedown"
	InputMouseUp     InputType = "mouseup"
	InputKeyDown     InputType = "keydown"
	InputKeyUp       InputType = "keyup"
)

type MouseButtons int

const (
	MouseLeft   MouseButtons = 0
	MouseMiddle MouseButtons = 1
	MouseRight  MouseButtons = 2
)

type ModifierKeys int

const (
	ModifierShift ModifierKeys = 1 << iota
	ModifierCtrl
	ModifierAlt
	ModifierMeta
)

type InputEvent struct {
	Type     InputType   `json:"type"`
	X        float64     `json:"x,omitempty"`
	Y        float64     `json:"y,omitempty"`
	Button   MouseButtons `json:"button,omitempty"`
	Key      string      `json:"key,omitempty"`
	Mod      ModifierKeys `json:"mod,omitempty"`
}

type InputInjector interface {
	Init() error
	SendMouseMove(x, y float64) error
	SendMouseClick(button MouseButtons, down bool) error
	SendKey(key string, down bool, mod ModifierKeys) error
	Close() error
}

type injector struct {
	closed bool
	mu     sync.Mutex
}

func NewInjector() InputInjector {
	return &injector{}
}

func (inj *injector) Init() error {
	return nil
}

func (inj *injector) SendMouseMove(x, y float64) error {
	inj.mu.Lock()
	defer inj.mu.Unlock()

	if inj.closed {
		return fmt.Errorf("injector closed")
	}

	switch runtime.GOOS {
	case "windows":
		return injectWindowsMouseMove(x, y)
	case "darwin":
		return injectDarwinMouseMove(x, y)
	case "linux":
		return injectLinuxMouseMove(x, y)
	default:
		return nil
	}
}

func (inj *injector) SendMouseClick(button MouseButtons, down bool) error {
	inj.mu.Lock()
	defer inj.mu.Unlock()

	if inj.closed {
		return fmt.Errorf("injector closed")
	}

	switch runtime.GOOS {
	case "windows":
		return injectWindowsMouseClick(button, down)
	case "darwin":
		return injectDarwinMouseClick(button, down)
	case "linux":
		return injectLinuxMouseClick(button, down)
	default:
		return nil
	}
}

func (inj *injector) SendKey(key string, down bool, mod ModifierKeys) error {
	inj.mu.Lock()
	defer inj.mu.Unlock()

	if inj.closed {
		return fmt.Errorf("injector closed")
	}

	switch runtime.GOOS {
	case "windows":
		return injectWindowsKey(key, down, mod)
	case "darwin":
		return injectDarwinKey(key, down, mod)
	case "linux":
		return injectLinuxKey(key, down, mod)
	default:
		return nil
	}
}

func (inj *injector) Close() error {
	inj.mu.Lock()
	defer inj.mu.Unlock()
	inj.closed = true
	return nil
}

// Windows implementations
func injectWindowsMouseMove(x, y float64) error {
	return nil
}

func injectWindowsMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectWindowsKey(key string, down bool, mod ModifierKeys) error {
	return nil
}

// macOS implementations
func injectDarwinMouseMove(x, y float64) error {
	return nil
}

func injectDarwinMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectDarwinKey(key string, down bool, mod ModifierKeys) error {
	return nil
}

// Linux implementations
func injectLinuxMouseMove(x, y float64) error {
	return nil
}

func injectLinuxMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectLinuxKey(key string, down bool, mod ModifierKeys) error {
	return nil
}
