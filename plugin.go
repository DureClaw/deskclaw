// Plugins — plug any app-specific CLI in as a bus verb, no Go changes.
//
// Some apps deserve a dedicated tool (KakaoTalk via `kax`, a CRM export script, an ERP
// CLI…). Drop a manifest into DESK_PLUGIN_DIR (default: OS config dir …/deskclaw/plugins)
// and `[<VERB>] <args>` runs `<command…> <args>` on this node:
//
//	{
//	  "verb": "KAKAO",                         → [KAKAO] …
//	  "description": "KakaoTalk read/triage/send",
//	  "command": ["python3", "/abs/path/kakao.py"],
//	  "os": ["darwin"],                         optional — skip on other OSes
//	  "subcommands": ["rooms", "read", "send"], optional whitelist for the first arg
//	  "front_flags": ["--json"],                optional — hoisted before the subcommand
//	  "confirm_flag": "--yes",                  optional — args that act on the real world…
//	  "allow_confirm": false,                   …are refused unless this node opts in here
//	  "timeout_sec": 120,                       default 120
//	  "record": false,                          true → steps are captured into RPA macros
//	  "overlay": true                           show the "computer use running" indicator
//	}
//
// The manifest lives on the node, so the node owner — not the remote master — decides what
// runs and whether confirm-flagged (e.g. sending) actions are allowed.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

type plugin struct {
	Verb         string   `json:"verb"`
	Description  string   `json:"description"`
	Command      []string `json:"command"`
	OS           []string `json:"os"`
	Subcommands  []string `json:"subcommands"`
	FrontFlags   []string `json:"front_flags"`
	ConfirmFlag  string   `json:"confirm_flag"`
	AllowConfirm bool     `json:"allow_confirm"`
	TimeoutSec   int      `json:"timeout_sec"`
	Record       bool     `json:"record"`
	Overlay      bool     `json:"overlay"`
}

// builtinVerbs can't be shadowed by a plugin.
var builtinVerbs = []string{"[RECORD]", "[ENDREC]", "[RUN]", "[MACROS]", "[SCREENSHOT]", "[AX]", "[PLUGINS]", "[APPS]",
	"[LAUNCH]", "[CLICK]", "[DOUBLECLICK]", "[RIGHTCLICK]", "[MOVE]", "[TYPE]", "[KEY]", "[SCROLL]"}

var (
	pluginMu sync.RWMutex
	plugins  = map[string]*plugin{} // "[VERB]" → plugin
)

func pluginDir() string {
	if d := os.Getenv("DESK_PLUGIN_DIR"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "deskclaw", "plugins")
}

// loadPlugins (re)reads every manifest; bad ones are logged and skipped, never fatal.
func loadPlugins() []string {
	found := map[string]*plugin{}
	var notes []string
	files, _ := filepath.Glob(filepath.Join(pluginDir(), "*.json"))
	for _, f := range files {
		var p plugin
		b, err := os.ReadFile(f)
		if err == nil {
			err = json.Unmarshal(b, &p)
		}
		verb := "[" + strings.ToUpper(strings.Trim(p.Verb, "[] ")) + "]"
		switch {
		case err != nil:
			notes = append(notes, fmt.Sprintf("skip %s: %v", filepath.Base(f), err))
		case verb == "[]" || len(p.Command) == 0:
			notes = append(notes, fmt.Sprintf("skip %s: needs verb and command", filepath.Base(f)))
		case slices.Contains(builtinVerbs, verb) || appAdapters[verb] != nil:
			notes = append(notes, fmt.Sprintf("skip %s: %s is built in", filepath.Base(f), verb))
		case len(p.OS) > 0 && !slices.Contains(p.OS, runtime.GOOS):
			notes = append(notes, fmt.Sprintf("skip %s: not for %s", filepath.Base(f), runtime.GOOS))
		default:
			if p.TimeoutSec <= 0 {
				p.TimeoutSec = 120
			}
			found[verb] = &p
		}
	}
	pluginMu.Lock()
	plugins = found
	pluginMu.Unlock()
	for _, n := range notes {
		log.Printf("[deskclaw] plugin %s", n)
	}
	return notes
}

func lookupPlugin(verb string) *plugin {
	pluginMu.RLock()
	defer pluginMu.RUnlock()
	return plugins[verb]
}

func pluginVerbs() []string {
	pluginMu.RLock()
	defer pluginMu.RUnlock()
	var vs []string
	for v := range plugins {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

func listPlugins() string {
	notes := loadPlugins()
	var lines []string
	for _, v := range pluginVerbs() {
		p := lookupPlugin(v)
		line := v + " " + p.Description
		if len(p.Subcommands) > 0 {
			line += " (" + strings.Join(p.Subcommands, "|") + ")"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "no plugins (dir: "+pluginDir()+")")
	}
	return strings.Join(append(lines, notes...), "\n")
}

func runPlugin(verb string, p *plugin, rest string) (string, int) {
	args, err := splitArgs(rest)
	if err != nil {
		return verb + " " + err.Error(), 1
	}
	var front, sub []string
	for _, a := range args {
		if slices.Contains(p.FrontFlags, a) {
			front = append(front, a)
		} else {
			sub = append(sub, a)
		}
	}
	if len(p.Subcommands) > 0 && (len(sub) == 0 || !slices.Contains(p.Subcommands, sub[0])) {
		return verb + " " + strings.Join(p.Subcommands, "|") + " …", 1
	}
	if p.ConfirmFlag != "" && slices.Contains(sub, p.ConfirmFlag) && !p.AllowConfirm {
		return fmt.Sprintf("%s %s refused — this node's manifest has allow_confirm=false", verb, p.ConfirmFlag), 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.TimeoutSec)*time.Second)
	defer cancel()
	argv := append(append(append([]string{}, p.Command[1:]...), front...), sub...)
	out, err := exec.CommandContext(ctx, p.Command[0], argv...).CombinedOutput()
	res := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Sprintf("%s timed out after %ds\n%s", verb, p.TimeoutSec, res), 1
		}
		return fmt.Sprintf("%s failed: %v\n%s", verb, err, res), 1
	}
	return res, 0
}

// splitArgs splits like a shell: whitespace-separated, "double" / 'single' quotes group,
// backslash escapes inside double quotes and bare words.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord, quote := false, rune(0)
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			inWord = true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote")
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out, nil
}
