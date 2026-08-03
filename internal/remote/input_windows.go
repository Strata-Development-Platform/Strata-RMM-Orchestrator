//go:build windows

package remote

import (
	"unsafe"
)

const (
	INPUT_MOUSE            = 0
	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	KEYEVENTF_KEYUP        = 0x0002
)

type mouseInput struct {
	dx        int32
	dy        int32
	mouseData uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type keybdInput struct {
	vk        uint16
	_         uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type input struct {
	inputType uint32
	padding   [20]byte
	mouse     mouseInput
	keybd     keybdInput
}

func injectWindowsMouseMove(x, y float64) error {
	var mi input
	mi.inputType = INPUT_MOUSE
	mi.mouse.dx = int32(x * 65535.0)
	mi.mouse.dy = int32(y * 65535.0)
	mi.mouse.flags = MOUSEEVENTF_MOVE

	_, _, _ = procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(&mi)),
		uintptr(unsafe.Sizeof(mi)),
	)
	return nil
}

func injectWindowsMouseClick(button MouseButtons, down bool) error {
	var flags uint32
	switch button {
	case MouseLeft:
		if down {
			flags = MOUSEEVENTF_LEFTDOWN
		} else {
			flags = MOUSEEVENTF_LEFTUP
		}
	case MouseRight:
		if down {
			flags = MOUSEEVENTF_RIGHTDOWN
		} else {
			flags = MOUSEEVENTF_RIGHTUP
		}
	case MouseMiddle:
		if down {
			flags = MOUSEEVENTF_MIDDLEDOWN
		} else {
			flags = MOUSEEVENTF_MIDDLEUP
		}
	default:
		return nil
	}

	var mi input
	mi.inputType = INPUT_MOUSE
	mi.mouse.flags = flags

	_, _, _ = procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(&mi)),
		uintptr(unsafe.Sizeof(mi)),
	)
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
	case "0":
		vk = 0x30
	case "1":
		vk = 0x31
	case "2":
		vk = 0x32
	case "3":
		vk = 0x33
	case "4":
		vk = 0x34
	case "5":
		vk = 0x35
	case "6":
		vk = 0x36
	case "7":
		vk = 0x37
	case "8":
		vk = 0x38
	case "9":
		vk = 0x39
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

	var ki input
	ki.inputType = INPUT_MOUSE // placeholder for keyboard
	ki.keybd.vk = vk
	if !down {
		ki.keybd.flags = KEYEVENTF_KEYUP
	}

	_, _, _ = procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(&ki)),
		uintptr(unsafe.Sizeof(ki)),
	)
	return nil
}
