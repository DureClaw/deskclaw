//go:build !darwin && !windows

// Stub GUI backend — non-Win/macOS (e.g. Linux). The node still joins the bus and
// answers, but GUI verbs report unsupported. (A Linux backend — X11/xdotool or
// Wayland — can be added later as gui_linux.go.)
package main

import (
	"fmt"
	"runtime"
)

func guiOS() string      { return runtime.GOOS }
func guiSupported() bool { return false }

func guiScreenshot(path string) (int, int, error) {
	return 0, 0, fmt.Errorf("GUI not supported on %s yet", runtime.GOOS)
}
func guiLaunch(app string) error                          { return unsupported() }
func guiClick(x, y int, button string, double bool) error { return unsupported() }
func guiMove(x, y int) error                              { return unsupported() }
func guiType(text string) error                           { return unsupported() }
func guiKey(combo string) error                           { return unsupported() }
func guiScroll(x, y, amount int, dir string) error        { return unsupported() }

func unsupported() error { return fmt.Errorf("GUI control not supported on %s yet", runtime.GOOS) }
