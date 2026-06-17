// RPA record → replay — "LLM as compiler" for the desktop.
//
// First time, the master (Claude vision) drives deskclaw step-by-step: locate & launch a
// program, click a menu, type, etc. deskclaw RECORDS those grounded steps into a named macro.
// After that, [RUN] <name> REPLAYS the macro deterministically — fast, no LLM, no vision.
// Same closed-loop idea as DureClaw skill crystallization, applied to GUI automation.
//
// MVP records launch + pointer/keyboard verbs with coordinates. Coordinate replay is the
// honest first step; element-targeting (accessibility tree) is the planned robustness upgrade.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type macroStep struct {
	Verb    string `json:"verb"`
	Args    string `json:"args"`
	DelayMs int    `json:"delay_ms,omitempty"`
}

type macro struct {
	Name    string      `json:"name"`
	Steps   []macroStep `json:"steps"`
	Created string      `json:"created,omitempty"`
}

var (
	recMu     sync.Mutex
	recording *macro // non-nil while recording
	replaying bool   // true during replay → don't re-record
)

func macroDir() string {
	if d := os.Getenv("DESK_MACRO_DIR"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "deskclaw", "macros")
}

func startRecording(name string) {
	recMu.Lock()
	defer recMu.Unlock()
	recording = &macro{Name: name}
}

// recordStep appends a successfully-executed action to the active recording (if any).
func recordStep(verb, args string) {
	recMu.Lock()
	defer recMu.Unlock()
	if recording == nil || replaying {
		return
	}
	recording.Steps = append(recording.Steps, macroStep{Verb: verb, Args: args})
}

func stopRecording(createdAt string) (string, int, error) {
	recMu.Lock()
	m := recording
	recording = nil
	recMu.Unlock()
	if m == nil {
		return "", 0, fmt.Errorf("not recording")
	}
	if len(m.Steps) == 0 {
		return m.Name, 0, fmt.Errorf("macro %q has no steps — nothing saved", m.Name)
	}
	m.Created = createdAt
	dir := macroDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, m.Name+".json"), b, 0o644); err != nil {
		return "", 0, err
	}
	return m.Name, len(m.Steps), nil
}

func loadMacro(name string) (*macro, error) {
	b, err := os.ReadFile(filepath.Join(macroDir(), name+".json"))
	if err != nil {
		return nil, fmt.Errorf("macro %q not found", name)
	}
	var m macro
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("macro %q corrupt: %v", name, err)
	}
	return &m, nil
}

// runMacro replays a saved macro deterministically (no LLM). Aborts on the first failed step.
func runMacro(name string) (string, int) {
	m, err := loadMacro(name)
	if err != nil {
		return err.Error(), 1
	}
	stepDelay := 400 * time.Millisecond
	recMu.Lock()
	replaying = true
	recMu.Unlock()
	defer func() { recMu.Lock(); replaying = false; recMu.Unlock() }()

	var log []string
	for i, s := range m.Steps {
		out, code := execVerb(s.Verb, s.Args)
		log = append(log, fmt.Sprintf("%d. %s %s → %s", i+1, s.Verb, s.Args, oneLine(out)))
		if code != 0 {
			return fmt.Sprintf("macro %q FAILED at step %d/%d:\n%s", name, i+1, len(m.Steps), strings.Join(log, "\n")), 1
		}
		d := stepDelay
		if s.DelayMs > 0 {
			d = time.Duration(s.DelayMs) * time.Millisecond
		}
		time.Sleep(d)
	}
	return fmt.Sprintf("macro %q replayed OK (%d steps, no LLM):\n%s", name, len(m.Steps), strings.Join(log, "\n")), 0
}

func listMacros() string {
	entries, err := os.ReadDir(macroDir())
	if err != nil {
		return "no macros yet (dir: " + macroDir() + ")"
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "no macros yet (dir: " + macroDir() + ")"
	}
	return "macros: " + strings.Join(names, ", ")
}

func isRecording() bool {
	recMu.Lock()
	defer recMu.Unlock()
	return recording != nil
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	return clip(s, 80)
}
