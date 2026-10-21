// Window stack — put the desktop back the way it was. Doing a job often opens windows only
// as a means (a settings window, a Finder/save panel, a chat room…). Before a job deskclaw
// pushes a snapshot of the on-screen windows; when the job ends it pops it and closes every
// window that appeared in between, frontmost first. Windows that were already there — even
// if moved or resized — are identified by window number and left alone.
//
// Automatic around app adapters ([KAKAO] …) and macro replays ([RUN]). A master driving step
// by step brackets a job itself: [MARK] … [RESTORE] (nestable). DESK_WINSTACK=0 disables.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type winInfo struct {
	ID    int    `json:"id"`
	PID   int    `json:"pid"`
	Owner string `json:"owner"`
	B     []int  `json:"b"`
}

const winRestoreMax = 6

var (
	winMu    sync.Mutex
	winStack []map[int]bool
)

func winStackOn() bool { return os.Getenv("DESK_WINSTACK") != "0" && axSupported() }

func winSnapshot() ([]winInfo, error) {
	out, err := axRun([]string{"snapshot"})
	if err != nil {
		return nil, err
	}
	var ws []winInfo
	for _, line := range strings.Split(out, "\n") {
		var w winInfo
		if line != "" && json.Unmarshal([]byte(line), &w) == nil {
			ws = append(ws, w)
		}
	}
	return ws, nil
}

// winMark pushes the current windows. Returns the stack depth after the push.
func winMark() int {
	ws, err := winSnapshot()
	if err != nil {
		ws = nil // still push, so marks and restores stay paired
	}
	seen := map[int]bool{}
	for _, w := range ws {
		seen[w.ID] = true
	}
	winMu.Lock()
	defer winMu.Unlock()
	winStack = append(winStack, seen)
	return len(winStack)
}

// winRestore pops the last mark and closes windows opened since, frontmost first.
func winRestore() string {
	winMu.Lock()
	if len(winStack) == 0 {
		winMu.Unlock()
		return "window stack empty — nothing to restore"
	}
	before := winStack[len(winStack)-1]
	winStack = winStack[:len(winStack)-1]
	winMu.Unlock()
	if before == nil || len(before) == 0 {
		return "no snapshot at mark — nothing closed"
	}
	ws, err := winSnapshot()
	if err != nil {
		return "restore: " + err.Error()
	}
	self := map[int]bool{os.Getpid(): true, os.Getppid(): true} // DeskClaw.app's own dialogs
	var fresh []winInfo
	for _, w := range ws { // snapshot is front-to-back
		if !before[w.ID] && !self[w.PID] && len(w.B) == 4 && w.B[2] > 1 && w.B[3] > 1 {
			fresh = append(fresh, w)
		}
	}
	// a job opens a few helper windows; dozens "new" means the snapshot can't be trusted
	if len(fresh) > winRestoreMax {
		return fmt.Sprintf("restore skipped: %d new windows looks wrong (max %d) — nothing closed", len(fresh), winRestoreMax)
	}
	var closed, failed []string
	for _, w := range fresh {
		args := []string{"closewin", fmt.Sprint(w.PID)}
		for _, v := range w.B {
			args = append(args, fmt.Sprint(v))
		}
		if _, err := axRun(args); err != nil {
			failed = append(failed, w.Owner)
		} else {
			closed = append(closed, w.Owner)
		}
	}
	if len(failed) > 0 { // helper surfaces often vanish with their main window — count only survivors
		failed = nil
		if now, err := winSnapshot(); err == nil {
			still := map[int]bool{}
			for _, w := range now {
				still[w.ID] = true
			}
			for _, w := range fresh {
				if still[w.ID] {
					failed = append(failed, w.Owner)
				}
			}
		}
	}
	if len(closed) == 0 && len(failed) == 0 {
		return "no new windows"
	}
	msg := fmt.Sprintf("closed %d window(s) opened during the job: %s", len(closed), strings.Join(closed, ", "))
	if len(failed) > 0 {
		msg += " · couldn't close: " + strings.Join(failed, ", ")
	}
	return msg
}

// winScoped: jobs that clean up after themselves automatically.
func winScoped(verb string) bool { return verb == "[RUN]" || appAdapters[verb] != nil }
