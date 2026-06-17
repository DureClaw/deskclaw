//go:build windows

// Windows GUI backend — built-in PowerShell only (no CGo, no extra installs):
// System.Drawing for screenshots, user32 P/Invoke for the pointer/wheel, and
// System.Windows.Forms.SendKeys for the keyboard.
package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func guiOS() string     { return "windows" }
func guiSupported() bool { return true }

func ps(script string) (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell: %v %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func guiLaunch(app string) error {
	if app == "" {
		return fmt.Errorf("launch needs an app name or path")
	}
	_, err := ps("Start-Process '" + strings.ReplaceAll(app, "'", "''") + "'")
	return err
}

func guiScreenshot(path string) (int, int, error) {
	p := strings.ReplaceAll(path, "'", "''")
	script := `Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$b=[System.Windows.Forms.SystemInformation]::VirtualScreen
$bmp=New-Object System.Drawing.Bitmap $b.Width,$b.Height
$g=[System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($b.Location,[System.Drawing.Point]::Empty,$b.Size)
$bmp.Save('` + p + `',[System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose();$bmp.Dispose()
"$($b.Width) $($b.Height)"`
	out, err := ps(script)
	if err != nil {
		return 0, 0, err
	}
	f := strings.Fields(out)
	if len(f) < 2 {
		return 0, 0, nil
	}
	w, _ := strconv.Atoi(f[0])
	h, _ := strconv.Atoi(f[1])
	return w, h, nil
}

const user32 = `Add-Type @"
using System;using System.Runtime.InteropServices;
public class DeskU32{
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint f,uint dx,uint dy,uint data,int extra);
}
"@
`

func guiClick(x, y int, button string, double bool) error {
	down, up := "0x0002", "0x0004" // left
	if button == "right" {
		down, up = "0x0008", "0x0010"
	}
	clickOnce := fmt.Sprintf("[DeskU32]::mouse_event(%s,0,0,0,0);[DeskU32]::mouse_event(%s,0,0,0,0)", down, up)
	body := fmt.Sprintf("[DeskU32]::SetCursorPos(%d,%d);Start-Sleep -Milliseconds 30;%s", x, y, clickOnce)
	if double {
		body += ";Start-Sleep -Milliseconds 60;" + clickOnce
	}
	_, err := ps(user32 + body)
	return err
}

func guiMove(x, y int) error {
	_, err := ps(user32 + fmt.Sprintf("[DeskU32]::SetCursorPos(%d,%d)", x, y))
	return err
}

func guiScroll(x, y, amount int, dir string) error {
	delta := 120 * amount
	if dir == "down" {
		delta = -delta
	}
	body := fmt.Sprintf("[DeskU32]::SetCursorPos(%d,%d);[DeskU32]::mouse_event(0x0800,0,0,%d,0)", x, y, delta)
	_, err := ps(user32 + body)
	return err
}

func guiType(text string) error {
	_, err := ps(`Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('` + sendKeysEscape(text) + `')`)
	return err
}

func guiKey(combo string) error {
	seq := toSendKeys(combo)
	if seq == "" {
		return fmt.Errorf("unknown key %q", combo)
	}
	_, err := ps(`Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('` + seq + `')`)
	return err
}

// sendKeysEscape — escape SendKeys metacharacters and the PowerShell single-quote.
func sendKeysEscape(s string) string {
	r := strings.NewReplacer(
		"{", "{{}", "}", "{}}", "+", "{+}", "^", "{^}", "%", "{%}",
		"~", "{~}", "(", "{(}", ")", "{)}", "[", "{[}", "]", "{]}",
	)
	return strings.ReplaceAll(r.Replace(s), "'", "''")
}

// toSendKeys — "ctrl+c" → "^c", "enter" → "{ENTER}", etc.
func toSendKeys(combo string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	key := parts[len(parts)-1]
	prefix := ""
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "ctrl", "control":
			prefix += "^"
		case "alt", "option":
			prefix += "%"
		case "shift":
			prefix += "+"
		case "win", "cmd", "meta":
			// no portable SendKeys Windows-key; ignore
		}
	}
	named := map[string]string{
		"enter": "{ENTER}", "return": "{ENTER}", "tab": "{TAB}", "esc": "{ESC}", "escape": "{ESC}",
		"space": " ", "backspace": "{BACKSPACE}", "delete": "{DELETE}", "del": "{DELETE}",
		"up": "{UP}", "down": "{DOWN}", "left": "{LEFT}", "right": "{RIGHT}",
		"pageup": "{PGUP}", "pagedown": "{PGDN}", "home": "{HOME}", "end": "{END}",
		"f1": "{F1}", "f2": "{F2}", "f3": "{F3}", "f4": "{F4}", "f5": "{F5}",
	}
	if v, ok := named[key]; ok {
		return prefix + v
	}
	if len(key) == 1 {
		return prefix + key
	}
	return ""
}
