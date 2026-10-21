// Built-in app adapters — app-specific skills compiled into deskclaw, on top of [AX].
//
// [AX] is the universal layer (any app, any element). An adapter knows ONE app well: which
// window/table holds what, how to parse rows into rooms/messages/orders…, and how to verify
// an action really happened. Adding an app = one app_<name>.go that calls registerApp in
// init(); it gets a bus verb, help, the on-screen "running" overlay, and the confirm guard.
//
//	[KAKAO] rooms -n 30          (see app_kakao.go)
//	[APPS]                       list built-in adapters
//
// Confirm guard: an arg equal to the adapter's confirmFlag (e.g. --yes on a send) acts on
// the real world, so it is refused unless the NODE opts in: DESK_ALLOW_CONFIRM=KAKAO,…
// A remote master can't flip that.
package main

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

type appAdapter struct {
	verb        string // "[KAKAO]"
	description string
	subcommands map[string]string // name → one-line help; first arg must be one of these
	confirmFlag string            // optional, e.g. "--yes"
	run         func(sub string, args []string) (string, error)
}

var appAdapters = map[string]*appAdapter{}

func registerApp(a *appAdapter) { appAdapters[a.verb] = a }

func appVerbs() []string {
	var vs []string
	for v := range appAdapters {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

func (a *appAdapter) usage() string {
	var subs []string
	for s := range a.subcommands {
		subs = append(subs, s)
	}
	sort.Strings(subs)
	lines := []string{a.verb + " " + a.description}
	for _, s := range subs {
		lines = append(lines, "  "+a.verb+" "+s+" "+a.subcommands[s])
	}
	return strings.Join(lines, "\n")
}

func listApps() string {
	if len(appAdapters) == 0 {
		return "no built-in app adapters"
	}
	var out []string
	for _, v := range appVerbs() {
		out = append(out, appAdapters[v].usage())
	}
	return strings.Join(out, "\n")
}

func confirmAllowed(verb string) bool {
	name := strings.Trim(verb, "[]")
	for _, v := range splitCSV(os.Getenv("DESK_ALLOW_CONFIRM")) {
		if strings.EqualFold(strings.Trim(v, "[]"), name) {
			return true
		}
	}
	return false
}

func runApp(a *appAdapter, rest string) (string, int) {
	args, err := splitArgs(rest)
	if err != nil {
		return a.verb + " " + err.Error(), 1
	}
	if len(args) == 0 || a.subcommands[args[0]] == "" {
		return a.usage(), 1
	}
	sub, args := args[0], args[1:]
	if a.confirmFlag != "" && slices.Contains(args, a.confirmFlag) && !confirmAllowed(a.verb) {
		return fmt.Sprintf("%s %s %s refused — this node hasn't opted in (DESK_ALLOW_CONFIRM=%s); drop %s for a dry-run",
			a.verb, sub, a.confirmFlag, strings.Trim(a.verb, "[]"), a.confirmFlag), 1
	}
	if i := slices.Index(args, "--no-overlay"); i >= 0 { // overlay itself is handled in runTask
		args = slices.Delete(args, i, i+1)
	}
	out, err := a.run(sub, args)
	if err != nil {
		return a.verb + " " + sub + ": " + err.Error(), 1
	}
	return out, 0
}

// appFlags is a tiny flag reader for adapter args: positional values plus -n N style options.
type appFlags struct {
	pos   []string
	opts  map[string]string
	bools map[string]bool
}

func parseAppFlags(args []string, withValue ...string) appFlags {
	f := appFlags{opts: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case slices.Contains(withValue, a) && i+1 < len(args):
			f.opts[a] = args[i+1]
			i++
		case strings.HasPrefix(a, "-") && len(a) > 1:
			f.bools[a] = true
		default:
			f.pos = append(f.pos, a)
		}
	}
	return f
}

func (f appFlags) int(name string, def int) int {
	var n int
	if _, err := fmt.Sscan(f.opts[name], &n); err == nil && n > 0 {
		return n
	}
	return def
}
