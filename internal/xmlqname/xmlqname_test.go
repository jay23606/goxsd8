package xmlqname

import "testing"

// TestResolve pins each outcome the package doc names: the collapse, the
// unprefixed arm with and without a binding of the empty prefix, the three
// colon structures no QName has, and an unbound prefix.
func TestResolve(t *testing.T) {
	bound := func(p string) (string, bool) {
		switch p {
		case "":
			return "urn:d", true
		case "t":
			return "urn:t", true
		}
		return "", false
	}
	unboundDefault := func(p string) (string, bool) {
		if p == "" {
			return "", false
		}
		return bound(p)
	}
	for _, tc := range []struct {
		why, lexical string
		lookup       func(string) (string, bool)
		space, local string
		ok           bool
	}{
		{"prefixed", "t:x", bound, "urn:t", "x", true},
		{"collapsed", " \t\r\nt:x\n ", bound, "urn:t", "x", true},
		{"vertical tab is no white space", "\vt:x", bound, "", "", false},
		{"unprefixed takes the empty prefix's binding", "x", bound, "urn:d", "x", true},
		{"unprefixed with no default namespace is absent, not unresolved", "x", unboundDefault, "", "x", true},
		{"empty", " ", bound, "", "", false},
		{"empty prefix", ":x", bound, "", "", false},
		{"empty local part", "t:", bound, "", "", false},
		{"second colon", "t:x:y", bound, "", "", false},
		{"unbound prefix", "u:x", bound, "", "", false},
	} {
		space, local, ok := Resolve(tc.lexical, tc.lookup)
		if space != tc.space || local != tc.local || ok != tc.ok {
			t.Errorf("%s: Resolve(%q) = %q, %q, %v; want %q, %q, %v", tc.why, tc.lexical, space, local, ok, tc.space, tc.local, tc.ok)
		}
	}
}
