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

	eventX := int(x * screenWidth)
	eventY := int(y * screenHeight)

	// Create and post a mouse move event
	// CGEventCreateMouseEvent and CGEventPost are in CoreGraphics
	// For now, we'll use a subprocess approach

	cmd := exec.Command("python3", "-c", fmt.Sprintf(`
import ctypes
import ctypes.util

# Load Quartz framework
coregraphics = ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")

# Create mouse event
kCGEventMouseMoved = 1
kCGEventSourceStateHIDSystemState = 0xFFFFFFFF

# Get source
source = coregraphics.CGEventSourceCreate(kCGEventSourceStateHIDSystemState)

# Create and post the event
event = coregraphics.CGEventCreateMouseEvent(source, kCGEventMouseMoved,
    ctypes.c_double(%d), ctypes.c_double(%d), 0)
coregraphics.CGEventPost(0, event)
coregraphics.CFRelease(event)
coregraphics.CFRelease(source)
`% eventX, eventY))

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
