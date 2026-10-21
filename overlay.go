// On-screen "computer use running" indicator — whenever deskclaw drives the GUI, the person at
// the machine sees an orange blinking border + top banner ("don't touch mouse/keyboard").
//
// A master drives step by step ([CLICK] → [TYPE] → [KEY] …), so the overlay is shared and
// lingers a few seconds after the last step instead of flickering per task. It's dropped
// right before [SCREENSHOT] so it never ends up in the image the master's vision reads.
// macOS only for now (axhelper overlay); DESK_OVERLAY=0 turns it off.
package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const overlayLinger = 3 * time.Second

var (
	ovMu     sync.Mutex
	ovActive int
	ovStop   func()
	ovSend   func([]byte) // visual layer; nil while the overlay is down
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
		ovStop, ovSend = axOverlay(overlayMessage())
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
	ovStop, ovSend = nil, nil
}

// overlayPause hides the overlay while deskclaw waits for the person (approval dialog) —
// "don't touch" would contradict "please click Allow". overlayResume brings it back if a
// task still holds it.
func overlayPause() {
	ovMu.Lock()
	defer ovMu.Unlock()
	if ovStop != nil {
		ovStop()
		ovStop, ovSend = nil, nil
	}
}

func overlayResume() {
	ovMu.Lock()
	defer ovMu.Unlock()
	if ovActive > 0 && ovStop == nil && overlayEnabled() {
		ovStop, ovSend = axOverlay(overlayMessage())
	}
}

func overlayMessage() string { return "Claude Computer Use 실행 중 · deskclaw (" + agentName + ")" }

// ── visual layer: a virtual cursor that shows what deskclaw looks at and clicks ─────────────
// AX presses don't move the real pointer, so without this the person sees buttons react
// "by themselves". Coordinates are global, top-left origin (same as [CLICK] x y and locate).
// DESK_VISUAL=0 keeps the border but skips the cursor animation (and its ~0.7s per step).

const visMoveMs = 450

func visualOn() bool {
	ovMu.Lock()
	defer ovMu.Unlock()
	return ovSend != nil && os.Getenv("DESK_VISUAL") != "0"
}

func visSend(m map[string]any) {
	b, _ := json.Marshal(m)
	ovMu.Lock()
	defer ovMu.Unlock()
	if ovSend != nil {
		ovSend(b)
	}
}

// visMove glides the virtual cursor to x,y and waits until it arrives.
func visMove(x, y int) {
	if !visualOn() {
		return
	}
	visSend(map[string]any{"c": "move", "x": x, "y": y, "ms": visMoveMs})
	time.Sleep(visMoveMs*time.Millisecond + 60*time.Millisecond)
}

func visClick(x, y int) {
	if visualOn() {
		visSend(map[string]any{"c": "click", "x": x, "y": y})
	}
}

func visCaption(text string) {
	if visualOn() {
		visSend(map[string]any{"c": "caption", "text": text})
	}
}

// visTarget shows the candidates an [AX] step is choosing from, highlights the chosen element
// and moves the cursor onto it. Returns the element's path (so the real action hits exactly
// what was shown) and its center; ok=false when visuals are off or the lookup failed.
func visTarget(app, query string, opts []string) (path string, cx, cy int, ok bool) {
	if !visualOn() {
		return "", 0, 0, false
	}
	out, err := axRun(append([]string{"locate", app, query}, opts...))
	if err != nil {
		return "", 0, 0, false
	}
	var loc struct {
		Path  string  `json:"path"`
		Label string  `json:"label"`
		Frame []int   `json:"frame"`
		Cands [][]int `json:"cands"`
	}
	if json.Unmarshal([]byte(out), &loc) != nil || len(loc.Frame) < 4 {
		return "", 0, 0, false
	}
	visSend(map[string]any{"c": "look", "cands": loc.Cands, "target": loc.Frame, "label": clip(loc.Label, 40)})
	time.Sleep(350 * time.Millisecond) // let the person see the options before the pick
	cx, cy = loc.Frame[0]+loc.Frame[2]/2, loc.Frame[1]+loc.Frame[3]/2
	visMove(cx, cy)
	return loc.Path, cx, cy, true
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
