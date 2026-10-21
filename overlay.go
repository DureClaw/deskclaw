// On-screen "computer use running" indicator — whenever deskclaw drives the GUI, the person at
// the machine sees an orange blinking border + top banner ("don't touch mouse/keyboard").
//
// A master drives step by step ([CLICK] → [TYPE] → [KEY] …), so the overlay is shared and
// lingers a few seconds after the last step instead of flickering per task. It's dropped
// right before [SCREENSHOT] so it never ends up in the image the master's vision reads.
// macOS only for now (axhelper overlay); DESK_OVERLAY=0 turns it off.
package main

import (
	"os"
	"sync"
	"time"
)

const overlayLinger = 3 * time.Second

var (
	ovMu     sync.Mutex
	ovActive int
	ovStop   func()
	ovTimer  *time.Timer
)

func overlayEnabled() bool { return os.Getenv("DESK_OVERLAY") != "0" }

// overlayHold shows the overlay (if not already up) and returns its release.
func overlayHold() (release func()) {
	if !overlayEnabled() {
		return func() {}
	}
	ovMu.Lock()
	ovActive++
	if ovTimer != nil {
		ovTimer.Stop()
		ovTimer = nil
	}
	if ovStop == nil {
		ovStop = axOverlay("Claude Computer Use 실행 중 · deskclaw (" + agentName + ")")
	}
	ovMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			ovMu.Lock()
			defer ovMu.Unlock()
			ovActive--
			if ovActive == 0 {
				ovTimer = time.AfterFunc(overlayLinger, overlayDrop)
			}
		})
	}
}

// overlayDrop hides the overlay now unless a task is still running.
func overlayDrop() {
	ovMu.Lock()
	defer ovMu.Unlock()
	if ovActive > 0 || ovStop == nil {
		return
	}
	if ovTimer != nil {
		ovTimer.Stop()
		ovTimer = nil
	}
	ovStop()
	ovStop = nil
}

// drivesGUI: tasks that move the pointer, type, focus apps or replay macros.
func drivesGUI(verb, rest string) bool {
	switch verb {
	case "[LAUNCH]", "[CLICK]", "[DOUBLECLICK]", "[RIGHTCLICK]", "[MOVE]", "[TYPE]", "[KEY]", "[SCROLL]",
		"[AX]", "[RUN]":
		return !(verb == "[AX]" && isAXAppsOnly(rest))
	}
	if appAdapters[verb] != nil {
		return true
	}
	if p := lookupPlugin(verb); p != nil {
		return p.Overlay
	}
	return false
}

func isAXAppsOnly(rest string) bool {
	args, _ := splitArgs(rest)
	return len(args) > 0 && args[0] == "apps" // listing apps touches nothing
}
