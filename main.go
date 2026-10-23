// deskclaw — the DureClaw desktop GUI node.
//
// Lends a Windows/macOS desktop to a DureClaw fleet: eyes (screenshot) + hands
// (click / type / key / scroll). The master brings the brain (Claude computer-use
// vision decides *what* to do); deskclaw is the hands+eyes that carry it out — the
// desktop counterpart of webclaw (which lends the browser).
//
// Pure-Go, no CGo: GUI is driven by built-in OS tooling (macOS screencapture +
// osascript; Windows PowerShell P/Invoke) rather than native-linked libraries, so the
// same one-file install story as edgeclaw holds. Off Win/macOS it's a no-op stub.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// ── config (env) ────────────────────────────────────────────────────────────
var (
	stateServer  = env("STATE_SERVER", "127.0.0.1:4000")
	secret       = env("OAH_SECRET", "")
	workKey      = env("WORK_KEY", "WK-demo")
	agentName    = env("AGENT_NAME", "deskclaw@"+hostname())
	agentRole    = env("AGENT_ROLE", "executor")
	machine      = env("AGENT_MACHINE", hostname())
	brainURL     = env("BRAIN_URL", "")
	brainToken   = env("BRAIN_TOKEN", "")
	caps         = splitCSV(env("CAPABILITIES", "desktop,gui,screenshot,click,type,key,launch,rpa"))
	shotDir      = env("DESK_SHOT_DIR", filepath.Join(os.TempDir(), "deskclaw-shots"))
	maxOutput, _ = strconv.Atoi(env("DESK_MAX_OUTPUT", "4000"))
)

var (
	refCounter    int64
	joinRefGlobal atomic.Value // string
)

func nextRef() string { return fmt.Sprintf("%d", atomic.AddInt64(&refCounter, 1)) }

func joinRef() any {
	if v, ok := joinRefGlobal.Load().(string); ok {
		return v
	}
	return nil
}

func main() {
	log.SetFlags(log.Ltime)
	_ = os.MkdirAll(shotDir, 0o755)
	if maxOutput <= 0 {
		maxOutput = 4000
	}
	log.Printf("[deskclaw] %s (role=%s, os=%s) → bus %s work:%s", agentName, agentRole, guiOS(), stateServer, workKey)
	if w := plaintextTokenWarning(); w != "" {
		log.Printf("[deskclaw] ⚠️  %s", w)
	}
	if axSupported() {
		caps = append(caps, "ax")
	}
	for _, v := range appVerbs() {
		caps = append(caps, strings.ToLower(strings.Trim(v, "[]")))
	}
	loadPlugins()
	for _, v := range pluginVerbs() {
		caps = append(caps, strings.ToLower(strings.Trim(v, "[]")))
	}
	log.Printf("[deskclaw] plugins: %v (dir %s)", pluginVerbs(), pluginDir())
	if !guiSupported() {
		log.Printf("[deskclaw] ⚠️  GUI control not implemented on this OS — node joins but GUI verbs return an error")
	}
	for {
		uiStatus("connecting", stateServer)
		if err := run(); err != nil {
			log.Printf("[deskclaw] %v — reconnecting in 3s", err)
			uiStatus("disconnected", err.Error())
			time.Sleep(3 * time.Second)
		}
	}
}

func run() error {
	url := busSocketURL() + "/socket/websocket?vsn=2.0.0"
	if secret != "" {
		url += "&token=" + secret // Phoenix reads connect params from the query
	}
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Close()
	log.Printf("[deskclaw] connected")

	jr := nextRef()
	joinRefGlobal.Store(jr)
	send(c, []any{jr, jr, "work:" + workKey, "phx_join", map[string]any{
		"agent_name": agentName, "role": agentRole, "machine": machine,
		"capabilities": caps, "preferred_model": "desktop-gui", "version": "deskclaw/0.1",
	}})
	log.Printf("[deskclaw] joined work:%s as %s", workKey, agentName)
	uiStatus("connected", fmt.Sprintf("%s · work:%s · %s", stateServer, workKey, agentName))

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				send(c, []any{nil, nextRef(), "phoenix", "heartbeat", map[string]any{}})
			}
		}
	}()

	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		if os.Getenv("DEBUG") != "" {
			log.Printf("[deskclaw][rx] %.200s", string(raw))
		}
		var frame []json.RawMessage
		if json.Unmarshal(raw, &frame) != nil || len(frame) != 5 {
			continue
		}
		var event string
		json.Unmarshal(frame[3], &event)
		if event != "task.assign" {
			continue
		}
		var p taskPayload
		if json.Unmarshal(frame[4], &p) != nil {
			continue
		}
		if p.To != "" && p.To != agentName && p.To != "broadcast" {
			continue
		}
		go handle(c, p)
	}
}

type taskPayload struct {
	TaskID       string `json:"task_id"`
	To           string `json:"to"`
	From         string `json:"from"`
	Instructions string `json:"instructions"`
}

func handle(c *websocket.Conn, p taskPayload) {
	instr := strings.TrimSpace(p.Instructions)
	if instr == "" {
		return
	}
	from := p.From
	if from == "" {
		from = "http@controller"
	}
	log.Printf("[deskclaw] task %s: %.70s", p.TaskID, instr)
	uiEmit("task", map[string]any{"id": p.TaskID, "from": from, "text": clip(instr, 300)})
	out, code := "", 0
	if refusal := senderGuard(from, p.To, instr); refusal != "" {
		out, code = refusal, 1
	} else {
		out, code = runTask(instr)
	}
	status := "done"
	if code != 0 {
		status = "blocked"
	}
	uiEmit("result", map[string]any{"id": p.TaskID, "status": status, "text": clip(out, 300)})
	send(c, []any{joinRef(), nextRef(), "work:" + workKey, "task.result", map[string]any{
		"task_id": p.TaskID, "to": from, "from": agentName,
		"status": status, "output": clip(out, maxOutput), "exit_code": code, "backend": "deskclaw",
	}})
	log.Printf("[deskclaw] result %s: %s", p.TaskID, status)
}

// ── GUI verbs — eyes + hands. The master (computer-use vision) decides the action;
// deskclaw carries it out. Natural-language tasks delegate to the master brain.
//
// RPA loop: [RECORD] <name> → (master drives [LAUNCH]/[CLICK]/[TYPE]/… step-by-step) →
// [ENDREC] saves a macro → [RUN] <name> replays it deterministically with no LLM.
func runTask(instr string) (string, int) {
	t := strings.TrimSpace(instr)
	verb, rest := splitVerb(t)
	if !verbAllowed(verb) {
		return verbRefused(verb), 1
	}
	if verb == "[SCREENSHOT]" {
		overlayDrop() // keep the indicator out of the image the master's vision reads
	} else if drivesGUI(verb, rest) && !strings.Contains(rest, "--no-overlay") {
		defer overlayHold()()
	}
	if winScoped(verb) && winStackOn() && !strings.Contains(rest, "--keep-windows") {
		winMark()
		defer func() { log.Printf("[deskclaw] %s", winRestore()) }()
	}
	switch verb {
	// ── RPA control (not recorded) ──
	case "[RECORD]":
		name := strings.TrimSpace(rest)
		if name == "" {
			return "record needs a name: [RECORD] <macro>", 1
		}
		if err := validMacroName(name); err != nil {
			return err.Error(), 1
		}
		startRecording(name)
		return "● recording macro " + name + " — following actions are captured; [ENDREC] to save", 0
	case "[ENDREC]":
		name, n, err := stopRecording(time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err.Error(), 1
		}
		return fmt.Sprintf("■ saved macro %q (%d steps) → replay: [RUN] %s", name, n, name), 0
	case "[RUN]":
		if strings.TrimSpace(rest) == "" {
			return "run needs a name: [RUN] <macro>", 1
		}
		return runMacro(strings.TrimSpace(rest))
	case "[MACROS]":
		return listMacros(), 0
	case "[SCREENSHOT]":
		return doScreenshot(rest)
	case "[PLUGINS]":
		return listPlugins(), 0
	case "[APPS]":
		return listApps(), 0
	case "[MARK]":
		return fmt.Sprintf("window stack depth %d — [RESTORE] closes windows opened from here", winMark()), 0
	case "[RESTORE]":
		return winRestore(), 0

	// ── action verbs (captured while recording) ──
	case "[LAUNCH]", "[CLICK]", "[DOUBLECLICK]", "[RIGHTCLICK]", "[MOVE]", "[TYPE]", "[KEY]", "[SCROLL]":
		return execAndRecord(verb, rest, true)
	case "[AX]":
		return execAndRecord(verb, rest, axIsAction(rest))

	default:
		if a := appAdapters[verb]; a != nil {
			return runApp(a, rest)
		}
		if p := lookupPlugin(verb); p != nil {
			return execAndRecord(verb, rest, p.Record)
		}
		if brainURL != "" { // keyless: master brings the brain
			out, err := brainExec(instr)
			if err != nil {
				return err.Error(), 1
			}
			return out, 0
		}
		return helpText() + " echo: " + instr, 0
	}
}

// execAndRecord runs an action and, when it's a recordable step, captures it into the
// active macro.
func execAndRecord(verb, rest string, recordable bool) (string, int) {
	out, code := execVerb(verb, rest)
	if code == 0 && recordable {
		recordStep(verb, rest)
		if isRecording() {
			out += " [rec]"
		}
	}
	return out, code
}

// execVerb runs ONE GUI action — shared by live handling and deterministic macro replay.
// The policy is checked here too, so a macro recorded under a looser policy can't replay past it.
func execVerb(verb, rest string) (string, int) {
	if !verbAllowed(verb) {
		return verbRefused(verb), 1
	}
	if refusal := shellGuard(verb, rest); refusal != "" {
		return refusal, 1
	}
	switch verb {
	case "[LAUNCH]":
		app := strings.TrimSpace(rest)
		if err := guiLaunch(app); err != nil {
			return err.Error(), 1
		}
		return "launched " + app, 0
	case "[CLICK]", "[DOUBLECLICK]", "[RIGHTCLICK]":
		return doClick(verb, rest)
	case "[MOVE]":
		return doMove(rest)
	case "[TYPE]":
		visCaption("⌨ 입력: " + clip(rest, 40))
		if err := guiType(rest); err != nil {
			return err.Error(), 1
		}
		return "typed " + strconv.Itoa(len(rest)) + " chars", 0
	case "[KEY]":
		visCaption("⌨ " + strings.TrimSpace(rest))
		if err := guiKey(strings.TrimSpace(rest)); err != nil {
			return err.Error(), 1
		}
		return "key " + strings.TrimSpace(rest), 0
	case "[SCROLL]":
		return doScroll(rest)
	case "[AX]":
		return doAX(rest)
	}
	if p := lookupPlugin(verb); p != nil {
		return runPlugin(verb, p, rest)
	}
	return "unknown verb " + verb, 1
}

func helpText() string {
	return "[deskclaw] eyes/hands: [SCREENSHOT] [LAUNCH app] [CLICK x y] [DOUBLECLICK x y] " +
		"[RIGHTCLICK x y] [MOVE x y] [TYPE text] [KEY combo] [SCROLL x y amount up|down] · " +
		"RPA: [RECORD name] … [ENDREC] then [RUN name] · [MACROS] · " +
		"Any app by element (macOS): [AX] apps|tree|find|text|press|set · " +
		"built-in apps: [APPS] (e.g. [KAKAO]) · plugins: [PLUGINS]."
}

func doScreenshot(arg string) (string, int) {
	name := "shot-" + nextRef() + ".png"
	path := filepath.Join(shotDir, name)
	w, h, err := guiScreenshot(path)
	if err != nil {
		return "screenshot failed: " + err.Error(), 1
	}
	res := fmt.Sprintf("screenshot %s (%dx%d)", path, w, h)
	// Optionally inline a base64 data URL so a master without file access can see it
	// (small screens only; large ones blow the 1800-char result clip — kept opt-in).
	if strings.Contains(strings.ToLower(arg), "b64") || os.Getenv("DESK_SHOT_B64") != "" {
		if b, e := os.ReadFile(path); e == nil {
			res += "\ndata:image/png;base64," + base64.StdEncoding.EncodeToString(b)
		}
	}
	return res, 0
}

func doClick(verb, rest string) (string, int) {
	x, y, err := twoInts(rest)
	if err != nil {
		return "click needs: x y", 1
	}
	button, double := "left", false
	switch verb {
	case "[DOUBLECLICK]":
		double = true
	case "[RIGHTCLICK]":
		button = "right"
	}
	visMove(x, y)
	if err := guiClick(x, y, button, double); err != nil {
		return err.Error(), 1
	}
	visClick(x, y)
	return fmt.Sprintf("%s at %d,%d", strings.Trim(strings.ToLower(verb), "[]"), x, y), 0
}

func doMove(rest string) (string, int) {
	x, y, err := twoInts(rest)
	if err != nil {
		return "move needs: x y", 1
	}
	if err := guiMove(x, y); err != nil {
		return err.Error(), 1
	}
	return fmt.Sprintf("move to %d,%d", x, y), 0
}

func doScroll(rest string) (string, int) {
	f := strings.Fields(rest)
	if len(f) < 3 {
		return "scroll needs: x y amount [up|down]", 1
	}
	x, _ := strconv.Atoi(f[0])
	y, _ := strconv.Atoi(f[1])
	amt, _ := strconv.Atoi(f[2])
	dir := "down"
	if len(f) >= 4 {
		dir = strings.ToLower(f[3])
	}
	if err := guiScroll(x, y, amt, dir); err != nil {
		return err.Error(), 1
	}
	return fmt.Sprintf("scroll %s %d at %d,%d", dir, amt, x, y), 0
}

func brainExec(prompt string) (string, error) {
	body, _ := json.Marshal(map[string]string{"prompt": prompt})
	req, _ := http.NewRequest("POST", strings.TrimRight(brainURL, "/")+"/brain/exec", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if brainToken != "" {
		req.Header.Set("Authorization", "Bearer "+brainToken)
	}
	cl := &http.Client{Timeout: 180 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var m map[string]any
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return "", fmt.Errorf("bad brain response")
	}
	if s, ok := m["output"].(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", m), nil
}

// ── helpers ─────────────────────────────────────────────────────────────────
func splitVerb(s string) (verb, rest string) {
	if !strings.HasPrefix(s, "[") {
		return "", s
	}
	i := strings.Index(s, "]")
	if i < 0 {
		return "", s
	}
	return strings.ToUpper(s[:i+1]), strings.TrimSpace(s[i+1:])
}

func twoInts(s string) (int, int, error) {
	f := strings.Fields(strings.ReplaceAll(s, ",", " "))
	if len(f) < 2 {
		return 0, 0, fmt.Errorf("need two ints")
	}
	x, e1 := strconv.Atoi(f[0])
	y, e2 := strconv.Atoi(f[1])
	if e1 != nil || e2 != nil {
		return 0, 0, fmt.Errorf("non-int coords")
	}
	return x, y, nil
}

func send(c *websocket.Conn, v []any) {
	b, _ := json.Marshal(v)
	c.WriteMessage(websocket.TextMessage, b)
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hostname() string { h, _ := os.Hostname(); return h }

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) { // don't split a multi-byte (e.g. Korean) rune
		n--
	}
	return s[:n]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
