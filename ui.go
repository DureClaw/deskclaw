// UI bridge — lets a desktop shell (the macOS menu-bar app, DeskClaw.app) show status,
// notify, and ask the person at the machine to approve real-world actions.
//
// Enabled with DESK_UI=1 (the shell sets it when it launches deskclaw as a child).
// Wire: one JSON object per line.
//
//	deskclaw → stdout:  {"event":"status","state":"connected","detail":"…"}
//	                    {"event":"task","id":"…","from":"…","text":"…"}
//	                    {"event":"result","id":"…","status":"done|blocked","text":"…"}
//	                    {"event":"approve","id":"…","text":"…"}         (waits for a reply)
//	shell → stdin:      {"approve":"<id>","ok":true|false}
//
// Logs stay on stderr, so stdout carries only these events.
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

const approveTimeout = 90 * time.Second

var (
	uiOn      = os.Getenv("DESK_UI") == "1"
	uiOutMu   sync.Mutex
	uiWaitMu  sync.Mutex
	uiWaiting = map[string]chan bool{}
	uiStdin   sync.Once
)

func uiEmit(event string, fields map[string]any) {
	if !uiOn {
		return
	}
	m := map[string]any{"event": event}
	for k, v := range fields {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	uiOutMu.Lock()
	os.Stdout.Write(append(b, '\n'))
	uiOutMu.Unlock()
}

func uiStatus(state, detail string) {
	uiEmit("status", map[string]any{"state": state, "detail": detail})
}

// uiApprove asks the person at the machine. Without a UI, or on timeout, the answer is no.
func uiApprove(text string) bool {
	if !uiOn {
		return false
	}
	uiStdin.Do(func() { go uiReadStdin() })
	id := nextRef()
	ch := make(chan bool, 1)
	uiWaitMu.Lock()
	uiWaiting[id] = ch
	uiWaitMu.Unlock()
	defer func() { uiWaitMu.Lock(); delete(uiWaiting, id); uiWaitMu.Unlock() }()

	overlayPause()
	defer overlayResume()
	uiEmit("approve", map[string]any{"id": id, "text": text})
	select {
	case ok := <-ch:
		return ok
	case <-time.After(approveTimeout):
		return false
	}
}

func uiReadStdin() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var r struct {
			Approve string `json:"approve"`
			OK      bool   `json:"ok"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Approve == "" {
			continue
		}
		uiWaitMu.Lock()
		if ch := uiWaiting[r.Approve]; ch != nil {
			ch <- r.OK
		}
		uiWaitMu.Unlock()
	}
}
