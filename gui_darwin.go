//go:build darwin

// macOS GUI backend — built-in tooling only (no CGo): screencapture for eyes,
// osascript/System Events for keyboard. Pointer (click/move) uses `cliclick` if
// installed (brew install cliclick); everything else is built in.
//
// Permissions: typing/keys need Accessibility; screenshots need Screen Recording.
// macOS prompts on first use — grant them to the terminal/binary running deskclaw.
package main

import (
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func guiOS() string     { return "darwin" }
func guiSupported() bool { return true }

func guiLaunch(app string) error {
	if app == "" {
		return fmt.Errorf("launch needs an app name or path")
	}
	// path → open it; otherwise treat as an app name (open -a).
	if strings.HasPrefix(app, "/") || strings.HasPrefix(app, ".") || strings.HasPrefix(app, "~") {
		return runErr("open", app)
	}
	return runErr("open", "-a", app)
}

func guiScreenshot(path string) (int, int, error) {
	if out, err := exec.Command("screencapture", "-x", "-t", "png", path).CombinedOutput(); err != nil {
		return 0, 0, fmt.Errorf("screencapture: %v %s", err, out)
	}
	return pngSize(path)
}

func guiClick(x, y int, button string, double bool) error {
	cl, err := exec.LookPath("cliclick")
	if err != nil {
		return fmt.Errorf("pointer control needs cliclick (brew install cliclick); typing/keys/screenshot work without it")
	}
	verb := "c"
	if double {
		verb = "dc"
	} else if button == "right" {
		verb = "rc"
	}
	return runErr(cl, fmt.Sprintf("%s:%d,%d", verb, x, y))
}

func guiMove(x, y int) error {
	cl, err := exec.LookPath("cliclick")
	if err != nil {
		return fmt.Errorf("pointer control needs cliclick (brew install cliclick)")
	}
	return runErr(cl, fmt.Sprintf("m:%d,%d", x, y))
}

func guiType(text string) error {
	// keystroke "<text>" — escape backslash and double-quote for AppleScript.
	esc := strings.ReplaceAll(text, "\\", "\\\\")
	esc = strings.ReplaceAll(esc, "\"", "\\\"")
	return runErr("osascript", "-e", fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, esc))
}

func guiKey(combo string) error {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	key := parts[len(parts)-1]
	var mods []string
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "cmd", "command", "meta", "win":
			mods = append(mods, "command down")
		case "ctrl", "control":
			mods = append(mods, "control down")
		case "alt", "option", "opt":
			mods = append(mods, "option down")
		case "shift":
			mods = append(mods, "shift down")
		}
	}
	using := ""
	if len(mods) > 0 {
		using = " using {" + strings.Join(mods, ", ") + "}"
	}
	// named keys → key code; single chars → keystroke
	if code, ok := keyCodes[key]; ok {
		return runErr("osascript", "-e", fmt.Sprintf(`tell application "System Events" to key code %d%s`, code, using))
	}
	if len(key) == 1 {
		return runErr("osascript", "-e", fmt.Sprintf(`tell application "System Events" to keystroke "%s"%s`, key, using))
	}
	return fmt.Errorf("unknown key %q", key)
}

func guiScroll(x, y, amount int, dir string) error {
	// MVP: no built-in arbitrary scroll on macOS without extra tooling.
	return fmt.Errorf("scroll not implemented on macOS MVP — use [KEY] pagedown/pageup or arrows")
}

var keyCodes = map[string]int{
	"return": 36, "enter": 36, "tab": 48, "space": 49, "delete": 51, "backspace": 51,
	"escape": 53, "esc": 53, "left": 123, "right": 124, "down": 125, "up": 126,
	"pageup": 116, "pagedown": 121, "home": 115, "end": 119,
}

func pngSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func runErr(name string, args ...string) error {
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

var _ = strconv.Atoi // keep strconv imported for parity across backends
