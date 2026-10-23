// Node policy — what a remote master may do on this desktop.
//
// Anyone on the bus with the token and work key can drive this machine, and the master is an LLM
// that reads untrusted text (chat messages, web pages). A prompt injection is one [LAUNCH]
// Terminal + [TYPE] away from running commands here, so the node owner can narrow it:
//
//	DESK_ALLOW_VERBS=SCREENSHOT,AX,KAKAO   only these verbs run (empty = all, the default)
//	DESK_ALLOW_SHELL=1                     allow driving terminals / script hosts (default: refused)
//	DESK_ALLOW_FROM=orchestrator,…         only tasks from these senders run (empty = any)
//	DESK_ACCEPT_BROADCAST=1                let un-addressed / "broadcast" tasks act (default: listing only)
//
// A task must name this node in "to" to act on the desktop, so one mis-addressed or broadcast
// task doesn't run on every desktop in the work key. "from" is what the sender says it is: the
// bus may not verify it, so DESK_ALLOW_FROM narrows honest mistakes more than a hostile sender.
//
// The shell guard refuses [LAUNCH] of a terminal, script or executable file (or ssh:// etc.),
// [AX] press/set into a terminal, and — on macOS — [TYPE]/[KEY] while a terminal is frontmost.
// It is defense in depth, not a sandbox: an app with its own built-in terminal (an IDE) is not
// recognised. DESK_ALLOW_VERBS is the hard boundary.
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// broadcastSafe: verbs an un-addressed task may still run — they only list, touch nothing.
// No verb means a natural-language task for the brain, which answers but doesn't act here.
var broadcastSafe = []string{"", "[PLUGINS]", "[APPS]", "[MACROS]"}

// senderGuard returns a refusal when this task's sender or addressing isn't accepted here.
func senderGuard(from, to, instr string) string {
	if allow := splitCSV(os.Getenv("DESK_ALLOW_FROM")); len(allow) > 0 && !slices.Contains(allow, from) {
		return fmt.Sprintf("task from %q refused — not in DESK_ALLOW_FROM on this node", from)
	}
	verb, _ := splitVerb(strings.TrimSpace(instr))
	if to != agentName && os.Getenv("DESK_ACCEPT_BROADCAST") != "1" && !slices.Contains(broadcastSafe, verb) {
		return fmt.Sprintf("%s refused — task isn't addressed to this node (to=%q); send it with to=%q", verb, to, agentName)
	}
	return ""
}

// busSocketURL: STATE_SERVER is host:port (ws://) or a full ws:// / wss:// URL; https:// → wss://.
func busSocketURL() string {
	s := strings.TrimRight(stateServer, "/")
	switch {
	case strings.HasPrefix(s, "ws://"), strings.HasPrefix(s, "wss://"):
		return s
	case strings.HasPrefix(s, "https://"):
		return "wss://" + strings.TrimPrefix(s, "https://")
	case strings.HasPrefix(s, "http://"):
		return "ws://" + strings.TrimPrefix(s, "http://")
	}
	return "ws://" + s
}

// plaintextTokenWarning: the token rides in the socket URL, so over plain ws:// to anything but
// this machine or the tailnet (WireGuard-encrypted) it can be sniffed — and it controls the desktop.
func plaintextTokenWarning() string {
	u := busSocketURL()
	if secret == "" || !strings.HasPrefix(u, "ws://") {
		return ""
	}
	host := strings.TrimPrefix(u, "ws://")
	if i := strings.IndexAny(host, "/"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" || strings.HasSuffix(host, ".ts.net") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		_, tailnet, _ := net.ParseCIDR("100.64.0.0/10")
		if ip.IsLoopback() || tailnet.Contains(ip) {
			return ""
		}
	}
	return "bus token is sent unencrypted (ws://" + host + ") — use wss:// (STATE_SERVER=wss://…) or a Tailscale address"
}

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
