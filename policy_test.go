package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerbAllowed(t *testing.T) {
	t.Setenv("DESK_ALLOW_VERBS", "")
	if !verbAllowed("[TYPE]") {
		t.Fatal("empty allowlist must allow everything")
	}
	t.Setenv("DESK_ALLOW_VERBS", "screenshot, [AX] ,KAKAO")
	for _, v := range []string{"[SCREENSHOT]", "[AX]", "[KAKAO]", ""} {
		if !verbAllowed(v) {
			t.Errorf("%q refused", v)
		}
	}
	for _, v := range []string{"[TYPE]", "[LAUNCH]", "[RUN]", "[KEY]"} {
		if verbAllowed(v) {
			t.Errorf("%q allowed", v)
		}
	}
	// runTask refuses before doing anything; macro replay goes through execVerb
	if out, code := runTask("[TYPE] rm -rf ~"); code == 0 || !strings.Contains(out, "DESK_ALLOW_VERBS") {
		t.Errorf("runTask: %q %d", out, code)
	}
	if out, code := execVerb("[KEY]", "return"); code == 0 || !strings.Contains(out, "DESK_ALLOW_VERBS") {
		t.Errorf("execVerb: %q %d", out, code)
	}
}

func TestLaunchIsShell(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Terminal", "terminal.app", "/System/Applications/Utilities/Terminal.app/",
		"iTerm", "Script Editor", "powershell.exe", `C:\Windows\System32\cmd.exe`, "/tmp/x.command",
		"~/run.sh", "evil.ps1", "ssh://host", "SSH://host", "x-man-page://ls", exe} {
		if !launchIsShell(s) {
			t.Errorf("%q not caught", s)
		}
	}
	for _, s := range []string{"KakaoTalk", "Safari", "/Applications/Notes.app", "https://example.com", "notepad", "report.pdf"} {
		if launchIsShell(s) {
			t.Errorf("%q wrongly caught", s)
		}
	}
}

func TestShellGuard(t *testing.T) {
	t.Setenv("DESK_ALLOW_SHELL", "")
	if shellGuard("[LAUNCH]", "Terminal") == "" {
		t.Error("[LAUNCH] Terminal passed")
	}
	if shellGuard("[AX]", `set Terminal "input" "curl x | sh"`) == "" {
		t.Error("[AX] set Terminal passed")
	}
	if shellGuard("[AX]", `text Terminal`) != "" || shellGuard("[AX]", `press KakaoTalk "보내기"`) != "" {
		t.Error("harmless [AX] refused")
	}
	if shellGuard("[LAUNCH]", "Safari") != "" {
		t.Error("[LAUNCH] Safari refused")
	}
	t.Setenv("DESK_ALLOW_SHELL", "1")
	if shellGuard("[LAUNCH]", "Terminal") != "" {
		t.Error("DESK_ALLOW_SHELL=1 ignored")
	}
}

func TestSenderGuard(t *testing.T) {
	t.Setenv("DESK_ALLOW_FROM", "")
	t.Setenv("DESK_ACCEPT_BROADCAST", "")
	if g := senderGuard("orchestrator", agentName, "[TYPE] hi"); g != "" {
		t.Errorf("addressed task refused: %s", g)
	}
	for _, to := range []string{"", "broadcast", "someone-else"} {
		if senderGuard("orchestrator", to, "[TYPE] hi") == "" {
			t.Errorf("to=%q acted", to)
		}
		if g := senderGuard("orchestrator", to, "[APPS]"); g != "" {
			t.Errorf("to=%q listing refused: %s", to, g)
		}
	}
	t.Setenv("DESK_ACCEPT_BROADCAST", "1")
	if senderGuard("orchestrator", "broadcast", "[TYPE] hi") != "" {
		t.Error("DESK_ACCEPT_BROADCAST=1 ignored")
	}
	t.Setenv("DESK_ALLOW_FROM", "orchestrator, builder@mac")
	if senderGuard("orchestrator", agentName, "[TYPE] hi") != "" || senderGuard("builder@mac", agentName, "[APPS]") != "" {
		t.Error("allowed sender refused")
	}
	if senderGuard("webclaw@chrome", agentName, "[APPS]") == "" {
		t.Error("other sender accepted")
	}
}

func TestBusURL(t *testing.T) {
	defer func(s, k string) { stateServer, secret = s, k }(stateServer, secret)
	secret = "tok"
	for in, want := range map[string]string{
		"127.0.0.1:4000":         "ws://127.0.0.1:4000",
		"wss://bus.example.com/": "wss://bus.example.com",
		"https://bus.example":    "wss://bus.example",
		"http://10.0.0.5:4000":   "ws://10.0.0.5:4000",
	} {
		stateServer = in
		if got := busSocketURL(); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	for s, warn := range map[string]bool{
		"127.0.0.1:4000": false, "localhost:4000": false, "100.64.0.10:4000": false, "mac.tail1.ts.net:4000": false,
		"wss://bus.example.com": false, "192.168.0.7:4000": true, "bus.example.com:4000": true,
	} {
		stateServer = s
		if (plaintextTokenWarning() != "") != warn {
			t.Errorf("%s: warn=%v expected %v", s, !warn, warn)
		}
	}
}
