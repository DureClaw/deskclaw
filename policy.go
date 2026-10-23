// Node policy — what a remote master may do on this desktop.
//
// Anyone on the bus with the token and work key can drive this machine, and the master is an LLM
// that reads untrusted text (chat messages, web pages). A prompt injection is one [LAUNCH]
// Terminal + [TYPE] away from running commands here, so the node owner can narrow it:
//
//	DESK_ALLOW_VERBS=SCREENSHOT,AX,KAKAO   only these verbs run (empty = all, the default)
//	DESK_ALLOW_SHELL=1                     allow driving terminals / script hosts (default: refused)
//
// The shell guard refuses [LAUNCH] of a terminal, script or executable file (or ssh:// etc.),
// [AX] press/set into a terminal, and — on macOS — [TYPE]/[KEY] while a terminal is frontmost.
// It is defense in depth, not a sandbox: an app with its own built-in terminal (an IDE) is not
// recognised. DESK_ALLOW_VERBS is the hard boundary.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// verbAllowed: DESK_ALLOW_VERBS empty → everything; natural-language tasks (no verb) only go to
// the brain and never act locally, so they always pass.
func verbAllowed(verb string) bool {
	allow := splitCSV(os.Getenv("DESK_ALLOW_VERBS"))
	if verb == "" || len(allow) == 0 {
		return true
	}
	name := strings.Trim(verb, "[]")
	for _, v := range allow {
		if strings.EqualFold(strings.Trim(v, "[] "), name) {
			return true
		}
	}
	return false
}

func verbRefused(verb string) string {
	return fmt.Sprintf("%s refused — not in DESK_ALLOW_VERBS on this node (%s)", verb, os.Getenv("DESK_ALLOW_VERBS"))
}

func shellAllowed() bool { return os.Getenv("DESK_ALLOW_SHELL") == "1" }

// shellApps: terminals and script hosts, by app/executable name (lowercase, no .app/.exe).
var shellApps = []string{
	"terminal", "iterm", "iterm2", "warp", "alacritty", "kitty", "wezterm", "wezterm-gui", "hyper", "ghostty",
	"tabby", "script editor", "automator", "shortcuts",
	"cmd", "powershell", "powershell_ise", "pwsh", "wt", "windowsterminal", "conhost", "wsl", "bash",
	"wscript", "cscript", "mshta", "rundll32", "regsvr32",
	"sh", "zsh", "fish", "osascript", "python", "python3", "node", "perl", "ruby",
}

// shellBundles: frontmost-app check by bundle id (macOS).
var shellBundles = []string{
	"com.apple.terminal", "com.googlecode.iterm2", "dev.warp.warp-stable", "io.alacritty", "net.kovidgoyal.kitty",
	"com.github.wez.wezterm", "co.zeit.hyper", "com.mitchellh.ghostty", "com.apple.scripteditor2", "com.apple.automator",
}

// shellExts: documents that run code when opened.
var shellExts = []string{
	".command", ".sh", ".zsh", ".bash", ".tool", ".terminal", ".scpt", ".applescript", ".workflow", ".py",
	".bat", ".cmd", ".ps1", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".hta", ".lnk", ".scr", ".msi",
}

// shellSchemes: URLs the OS hands to a terminal.
var shellSchemes = []string{"ssh:", "telnet:", "x-man-page:"}

func isShellApp(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), `\`, "/") // Windows paths too
	n = strings.TrimSuffix(strings.TrimSuffix(filepath.Base(strings.TrimRight(n, "/")), ".app"), ".exe")
	return slices.Contains(shellApps, n) || slices.Contains(shellBundles, n)
}

// launchIsShell: would [LAUNCH] <target> start a shell or run a script?
func launchIsShell(target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	for _, s := range shellSchemes {
		if strings.HasPrefix(t, s) {
			return true
		}
	}
	if isShellApp(t) || slices.Contains(shellExts, filepath.Ext(t)) {
		return true
	}
	// an executable file (no extension) opens in Terminal on macOS
	p := strings.TrimSpace(target)
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
		return true
	}
	return false
}

// frontmostIsShell (macOS): is a terminal the active app right now? Without the helper we can't
// tell, so typing still works (logged) rather than breaking every node without Xcode CLT.
func frontmostIsShell() (bool, string) {
	if runtime.GOOS != "darwin" || !axSupported() {
		return false, ""
	}
	out, err := axRun([]string{"apps"})
	if err != nil {
		log.Printf("[deskclaw] shell guard: can't read frontmost app: %v", err)
		return false, ""
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasSuffix(line, " · active") {
			continue
		}
		f := strings.Split(line, " · ") // name · bundle · pid N · active
		if len(f) >= 2 && (isShellApp(f[0]) || isShellApp(f[1])) {
			return true, f[0]
		}
		return false, ""
	}
	return false, ""
}

// shellGuard returns a refusal when an action would drive a shell, "" when it may run.
func shellGuard(verb, rest string) string {
	if shellAllowed() {
		return ""
	}
	const hint = " — refused by the shell guard (node owner can set DESK_ALLOW_SHELL=1)"
	switch verb {
	case "[LAUNCH]":
		if launchIsShell(rest) {
			return "[LAUNCH] " + strings.TrimSpace(rest) + hint
		}
	case "[AX]":
		if args, err := splitArgs(rest); err == nil && len(args) >= 2 && (args[0] == "press" || args[0] == "set") && isShellApp(args[1]) {
			return "[AX] " + args[0] + " into " + args[1] + hint
		}
	case "[TYPE]", "[KEY]":
		if yes, app := frontmostIsShell(); yes {
			return verb + " while " + app + " is frontmost" + hint
		}
	}
	return ""
}
