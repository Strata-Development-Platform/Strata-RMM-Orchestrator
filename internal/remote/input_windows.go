//go:build windows

package remote

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")

	procSendInput     = user32.NewProc("SendInput")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procMapVirtualKey = user32.NewProc("MapVirtualKeyW")
)

type inputUnion struct {
	dwType uint32
	mouseInput [5]uint32
	keyboardInput [5]uint32
	hardwareInput [3]uint32
}

func (u *inputUnion) asInterface() interface{} {
	return u
}

func injectWindowsMouseMove(x, y float64) error {
	// Get screen dimensions
	screenWidth := windows.GetSystemMetrics(0)
	screenHeight := windows.GetSystemMetrics(1)

	if screenWidth == 0 || screenHeight == 0 {
		return fmt.Errorf("cannot get screen dimensions")
	}

	// Convert to 16-bit signed integer for SendInput
	dx := int32(x * 65535.0 / float64(screenWidth))
	dy := int32(y * 65535.0 / float64(screenHeight))

	input := createMouseMoveInput(dx, dy)
	n := uint32(1)

	ret, _, err := procSendInput.Call(
		uintptr(n),
		uintptr(unsafe.Pointer(&input)),
		uintptr(int32(unsafe.Sizeof(input))),
	)
	if ret == 0 {
		return fmt.Errorf("SendInput failed: %w", err)
	}

	return nil
}

func createMouseMoveInput(dx, dy int32) INPUT {
	return INPUT{
		Type: INPUT_MOUSE,
		Mi: MOUSEINPUT{
			Dx:          dx,
			Dy:          dy,
			MouseData:   0,
			Flags:       MOUSEEVENTF_MOVE,
			Time:        0,
			ExtraInfo:   0,
		},
	}
}

const (
	INPUT_MOUSE   = 0
	INPUT_KEYBOARD = 1
	INPUT_HARDWARE = 2

	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040

	VK_LBUTTON = 0x01
	VK_RBUTTON = 0x02
	VK_MBUTTON = 0x04
)

type INPUT struct {
	Type   uint32
	Mi     MOUSEINPUT
	Ki     KEYBDINPUT
	Hi     HARDWAREINPUT
	Padding [16]byte
}

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	Flags       uint32
	Time        uint32
	ExtraInfo   uintptr
}

type KEYBDINPUT struct {
	Vk         uint16
	Scan       uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

type HARDWAREINPUT struct {
	UMsg  uint32
	WParamL uint16
	WParamH uint16
}

func injectWindowsMouseClick(button MouseButtons, down bool) error {
	var flags uint32
	var vk uint16

	switch button {
	case MouseLeft:
		vk = VK_LBUTTON
		if down {
			flags = MOUSEEVENTF_LEFTDOWN
		} else {
			flags = MOUSEEVENTF_LEFTUP
		}
	case MouseRight:
		vk = VK_RBUTTON
		if down {
			flags = MOUSEEVENTF_RIGHTDOWN
		} else {
			flags = MOUSEEVENTF_RIGHTUP
		}
	case MouseMiddle:
		vk = VK_MBUTTON
		if down {
			flags = MOUSEEVENTF_MIDDLEDOWN
		} else {
			flags = MOUSEEVENTF_MIDDLEUP
		}
	}

	input := INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:      vk,
			Scan:    0,
			Flags:   0,
			Time:    0,
			ExtraInfo: 0,
		},
	}

	// First press
	n := uint32(1)
	ret, _, _ := procSendInput.Call(
		uintptr(n),
		uintptr(unsafe.Pointer(&input)),
		uintptr(int32(unsafe.Sizeof(input))),
	)

	if down {
		// Then release
		input.Ki.Flags = KEYEVENTF_KEYUP
		ret, _, _ = procSendInput.Call(
			uintptr(n),
			uintptr(unsafe.Pointer(&input)),
			uintptr(int32(unsafe.Sizeof(input))),
		)
	}

	if ret == 0 {
		return fmt.Errorf("SendInput for mouse click failed")
	}

	return nil
}

func injectWindowsKey(key string, down bool, mod ModifierKeys) error {
	var vk uint16

	switch key {
	case "a", "A":
		vk = 0x41
	case "b", "B":
		vk = 0x42
	case "c", "C":
		vk = 0x43
	case "d", "D":
		vk = 0x44
	case "e", "E":
		vk = 0x45
	case "f", "F":
		vk = 0x46
	case "g", "G":
		vk = 0x47
	case "h", "H":
		vk = 0x48
	case "i", "I":
		vk = 0x49
	case "j", "J":
		vk = 0x4A
	case "k", "K":
		vk = 0x4B
	case "l", "L":
		vk = 0x4C
	case "m", "M":
		vk = 0x4D
	case "n", "N":
		vk = 0x4E
	case "o", "O":
		vk = 0x4F
	case "p", "P":
		vk = 0x50
	case "q", "Q":
		vk = 0x51
	case "r", "R":
		vk = 0x52
	case "s", "S":
		vk = 0x53
	case "t", "T":
		vk = 0x54
	case "u", "U":
		vk = 0x55
	case "v", "V":
		vk = 0x56
	case "w", "W":
		vk = 0x57
	case "x", "X":
		vk = 0x58
	case "y", "Y":
		vk = 0x59
	case "z", "Z":
		vk = 0x5A
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		vk = uint16(key[0])
	case "space":
		vk = 0x20
	case "enter":
		vk = 0x0D
	case "escape":
		vk = 0x1B
	case "tab":
		vk = 0x09
	case "backspace":
		vk = 0x08
	case "delete":
		vk = 0x2E
	case "insert":
		vk = 0x2D
	case "home":
		vk = 0x24
	case "end":
		vk = 0x23
	case "pageup":
		vk = 0x21
	case "pagedown":
		vk = 0x22
	case "up":
		vk = 0x26
	case "down":
		vk = 0x28
	case "left":
		vk = 0x25
	case "right":
		vk = 0x27
	case "f1":
		vk = 0x70
	case "f2":
		vk = 0x71
	case "f3":
		vk = 0x72
	case "f4":
		vk = 0x73
	case "f5":
		vk = 0x74
	case "f6":
		vk = 0x75
	case "f7":
		vk = 0x76
	case "f8":
		vk = 0x77
	case "f9":
		vk = 0x78
	case "f10":
		vk = 0x79
	case "f11":
		vk = 0x7A
	case "f12":
		vk = 0x7B
	default:
		return nil
	}

	input := INPUT{
		Type: INPUT_KEYBOARD,
		Ki: KEYBDINPUT{
			Vk:      vk,
			Scan:    0,
			Flags:   0,
			Time:    0,
			ExtraInfo: 0,
		},
	}

	if !down {
		input.Ki.Flags = KEYEVENTF_KEYUP
	}

	n := uint32(1)
	ret, _, _ := procSendInput.Call(
		uintptr(n),
		uintptr(unsafe.Pointer(&input)),
		uintptr(int32(unsafe.Sizeof(input))),
	)

	if ret == 0 {
		return fmt.Errorf("SendInput for key failed")
	}

	return nil
}
