//go:build !windows && !darwin && !linux

package remote

import "fmt"

func injectWindowsMouseMove(x, y float64) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectWindowsMouseClick(button MouseButtons, down bool) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectWindowsKey(key string, down bool, mod ModifierKeys) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectDarwinMouseMove(x, y float64) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectDarwinMouseClick(button MouseButtons, down bool) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectDarwinKey(key string, down bool, mod ModifierKeys) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectLinuxMouseMove(x, y float64) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectLinuxMouseClick(button MouseButtons, down bool) error {
	return fmt.Errorf("input injection not supported on this platform")
}

func injectLinuxKey(key string, down bool, mod ModifierKeys) error {
	return fmt.Errorf("input injection not supported on this platform")
}
