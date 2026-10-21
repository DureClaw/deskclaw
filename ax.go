// [AX] verb — element-targeted GUI control for any app (see axhelper/axhelper.swift).
//
//	[AX] apps                                  running GUI apps
//	[AX] tree  <app> [--depth N --max N]       element tree: path role "label"
//	[AX] find  <app> "<query>"                 matches with path + frame
//	[AX] text  <app> ["<query>"]               readable text under a match / the windows
//	[AX] press <app> "<query>"                 press a button/menu/row (falls back to click)
//	[AX] set   <app> "<query>" "<value>"       fill a field, verified by read-back
//
// Query: words the label must contain, or role=AXButton title=… desc~… value~… #2 path=0.1.3
//
// press/set are recorded into RPA macros. Unlike [CLICK] x y they name the element, so a
// replayed macro still works after the window moves or resizes.
package main

import (
	"encoding/json"
	"slices"
	"strings"
)

var axCmds = []string{"apps", "tree", "find", "text", "press", "set"}

func doAX(rest string) (string, int) {
	args, err := splitArgs(rest)
	if err != nil {
		return "[AX] " + err.Error(), 1
	}
	if len(args) == 0 || !slices.Contains(axCmds, args[0]) {
		return "[AX] apps | tree <app> | find <app> <query> | text <app> [query] | press <app> <query> | set <app> <query> <value>", 1
	}
	out, err := axRun(args)
	if err != nil {
		return "[AX] " + err.Error(), 1
	}
	return out, 0
}

// axIsAction: only state-changing AX steps belong in a macro; reads are replay noise.
func axIsAction(rest string) bool {
	args, err := splitArgs(rest)
	return err == nil && len(args) > 0 && (args[0] == "press" || args[0] == "set")
}

// axNode is one line of `axhelper dump` — what app adapters parse.
type axNode struct {
	D    int    `json:"d"`
	R    string `json:"r"`
	T    string `json:"t"`
	V    string `json:"v"`
	Desc string `json:"desc"`
}

func axDump(args ...string) ([]axNode, error) {
	out, err := axRun(append([]string{"dump"}, args...))
	if err != nil {
		return nil, err
	}
	var nodes []axNode
	for _, line := range strings.Split(out, "\n") {
		var n axNode
		if line != "" && json.Unmarshal([]byte(line), &n) == nil {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// axRows groups dumped nodes by AXRow (nodes outside any row are dropped).
func axRows(nodes []axNode) [][]axNode {
	var rows [][]axNode
	inRow, rowDepth := false, 0
	for _, n := range nodes {
		switch {
		case n.R == "AXRow":
			rows = append(rows, nil)
			inRow, rowDepth = true, n.D
		case inRow && n.D > rowDepth:
			rows[len(rows)-1] = append(rows[len(rows)-1], n)
		default:
			inRow = false
		}
	}
	return rows
}
