package main

import "testing"

func TestValidMacroName(t *testing.T) {
	for _, ok := range []string{"login", "카톡-답장_1", "v1.2"} {
		if validMacroName(ok) != nil {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../config", "../../../../.claude/settings", "a/b", `a\b`, ".hidden", "/etc/x"} {
		if validMacroName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
