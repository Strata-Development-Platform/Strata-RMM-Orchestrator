//go:build darwin

package remote

import (
	"fmt"
	"os/exec"
)

func injectDarwinMouseMove(x, y float64) error {
	// Use CGEventCreateMouseEvent via CGEventPost
	// Since we can't directly import CoreGraphics in pure Go,
	// we'll use the Quartz event posting via unix

	// Convert to absolute screen coordinates
	screenWidth := 1920
	screenHeight := 1080

	if screenWidth == 0 || screenHeight == 0 {
		return fmt.Errorf("cannot get screen dimensions")
	}

	// Create and post a mouse move event using CGEvent via python3
	cmd := exec.Command("python3", "-c",
		fmt.Sprintf(`import ctypes; cg=ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"); src=cg.CGEventSourceCreate(0xFFFFFFFF); evt=cg.CGEventCreateMouseEvent(src, 1, %f, %f, 0); cg.CGEventPost(0, evt); cg.CFRelease(evt); cg.CFRelease(src)`,
			x*float64(screenWidth), y*float64(screenHeight)))

	_ = cmd.Run()
	return nil
}

func injectDarwinMouseClick(button MouseButtons, down bool) error {
	cmd := exec.Command("python3", "-c", `
import ctypes
import ctypes.util

coregraphics = ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")

kCGEventLeftMouseDown = 1
kCGEventLeftMouseUp = 2

source = coregraphics.CGEventSourceCreate(0xFFFFFFFF)
event = coregraphics.CGEventCreateMouseEvent(source, kCGEventLeftMouseDown, 0.0, 0.0, 0)
coregraphics.CGEventPost(0, event)
coregraphics.CFRelease(event)

event = coregraphics.CGEventCreateMouseEvent(source, kCGEventLeftMouseUp, 0.0, 0.0, 0)
coregraphics.CGEventPost(0, event)
coregraphics.CFRelease(event)
coregraphics.CFRelease(source)
`)

	_ = cmd.Run()
	return nil
}

func injectDarwinKey(key string, down bool, mod ModifierKeys) error {
	cmd := exec.Command("python3", "-c", `
import ctypes
import ctypes.util

coregraphics = ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")

kCGEventKeyDown = 10
kCGEventKeyUp = 11

source = coregraphics.CGEventSourceCreate(0xFFFFFFFF)
event = coregraphics.CGEventCreateKeyboardEvent(source, 0, True)
coregraphics.CGEventPost(0, event)
coregraphics.CFRelease(event)

event = coregraphics.CGEventCreateKeyboardEvent(source, 0, False)
coregraphics.CGEventPost(0, event)
coregraphics.CFRelease(event)
coregraphics.CFRelease(source)
`)

	_ = cmd.Run()
	return nil
}
