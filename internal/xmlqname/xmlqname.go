package xmlqname

import "strings"

// Resolve is the namespace name and local part lexical denotes with lookup's
// bindings in scope, and false where it denotes none (package doc).
func Resolve(lexical string, lookup func(prefix string) (string, bool)) (space, local string, ok bool) {
	collapsed := strings.Join(strings.FieldsFunc(lexical, isSpace), " ")
	prefix, local, prefixed := strings.Cut(collapsed, ":")
	if !prefixed {
		if collapsed == "" {
			return "", "", false
		}
		space, _ := lookup("")
		return space, collapsed, true
	}
	if prefix == "" || local == "" || strings.Contains(local, ":") {
		return "", "", false
	}
	space, bound := lookup(prefix)
	if !bound {
		return "", "", false
	}
	return space, local, true
}

// isSpace reports whether r is white space to whiteSpace = collapse: a space,
// tab, carriage return or line feed, and not a vertical tab, which
// unicode.IsSpace would admit.
func isSpace(r rune) bool { return strings.ContainsRune(" \t\r\n", r) }
