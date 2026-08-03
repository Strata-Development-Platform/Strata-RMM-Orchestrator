//go:build !linux

package remote

func injectLinuxMouseMove(x, y float64) error {
	return nil
}

func injectLinuxMouseClick(button MouseButtons, down bool) error {
	return nil
}

func injectLinuxKey(key string, down bool, mod ModifierKeys) error {
	return nil
}
