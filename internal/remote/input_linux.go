//go:build linux

package remote

import (
	"fmt"
	"os/exec"
)

func injectLinuxMouseMove(x, y float64) error {
	cmd := exec.Command("xdotool", "mousemove",
		fmt.Sprintf("%.0f", x),
		fmt.Sprintf("%.0f", y),
	)
	_ = cmd.Run()
	return nil
}

func injectLinuxMouseClick(button MouseButtons, down bool) error {
	var buttonCode int

	switch button {
	case MouseLeft:
		buttonCode = 1
	case MouseRight:
		buttonCode = 3
	case MouseMiddle:
		buttonCode = 2
	}

	cmd := exec.Command("xdotool", "click", fmt.Sprintf("%d", buttonCode))
	_ = cmd.Run()
	return nil
}

func injectLinuxKey(key string, down bool, mod ModifierKeys) error {
	var xdotKey string

	switch key {
	case "a":
		xdotKey = "a"
	case "b":
		xdotKey = "b"
	case "c":
		xdotKey = "c"
	case "d":
		xdotKey = "d"
	case "e":
		xdotKey = "e"
	case "f":
		xdotKey = "f"
	case "g":
		xdotKey = "g"
	case "h":
		xdotKey = "h"
	case "i":
		xdotKey = "i"
	case "j":
		xdotKey = "j"
	case "k":
		xdotKey = "k"
	case "l":
		xdotKey = "l"
	case "m":
		xdotKey = "m"
	case "n":
		xdotKey = "n"
	case "o":
		xdotKey = "o"
	case "p":
		xdotKey = "p"
	case "q":
		xdotKey = "q"
	case "r":
		xdotKey = "r"
	case "s":
		xdotKey = "s"
	case "t":
		xdotKey = "t"
	case "u":
		xdotKey = "u"
	case "v":
		xdotKey = "v"
	case "w":
		xdotKey = "w"
	case "x":
		xdotKey = "x"
	case "y":
		xdotKey = "y"
	case "z":
		xdotKey = "z"
	case "0":
		xdotKey = "0"
	case "1":
		xdotKey = "1"
	case "2":
		xdotKey = "2"
	case "3":
		xdotKey = "3"
	case "4":
		xdotKey = "4"
	case "5":
		xdotKey = "5"
	case "6":
		xdotKey = "6"
	case "7":
		xdotKey = "7"
	case "8":
		xdotKey = "8"
	case "9":
		xdotKey = "9"
	case "space":
		xdotKey = "space"
	case "enter":
		xdotKey = "Return"
	case "escape":
		xdotKey = "Escape"
	case "tab":
		xdotKey = "Tab"
	case "backspace":
		xdotKey = "BackSpace"
	case "delete":
		xdotKey = "Delete"
	case "insert":
		xdotKey = "Insert"
	case "home":
		xdotKey = "Home"
	case "end":
		xdotKey = "End"
	case "pageup":
		xdotKey = "Prior"
	case "pagedown":
		xdotKey = "Next"
	case "up":
		xdotKey = "Up"
	case "down":
		xdotKey = "Down"
	case "left":
		xdotKey = "Left"
	case "right":
		xdotKey = "Right"
	case "f1":
		xdotKey = "F1"
	case "f2":
		xdotKey = "F2"
	case "f3":
		xdotKey = "F3"
	case "f4":
		xdotKey = "F4"
	case "f5":
		xdotKey = "F5"
	case "f6":
		xdotKey = "F6"
	case "f7":
		xdotKey = "F7"
	case "f8":
		xdotKey = "F8"
	case "f9":
		xdotKey = "F9"
	case "f10":
		xdotKey = "F10"
	case "f11":
		xdotKey = "F11"
	case "f12":
		xdotKey = "F12"
	default:
		return nil
	}

	cmd := exec.Command("xdotool", "key", xdotKey)
	_ = cmd.Run()
	return nil
}

func init() {
	// Linux input injection requires xdotool to be installed
}
