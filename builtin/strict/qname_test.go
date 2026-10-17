package strict_test

import (
	"testing"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsderr"
)

// fakeContext is a package-private value.Context backed by a prefix→namespace
// map, mirroring how the other _test.go files pass nil for the context-free
// types. A prefix absent from the map is unbound (ok=false). The empty prefix
// is an ordinary key, so the map states the default namespace: mapping "" to
// "" is the [value.Context] contract's no-default case ("", true), and leaving
// "" out answers ("", false), which the contract forbids and only the
// off-contract defence in TestQNameUnboundPrefixRejected relies on.
type fakeContext map[string]string

func (c fakeContext) LookupNamespace(prefix string) (string, bool) {
	ns, ok := c[prefix]
	return ns, ok
}

// qnameLocals exercises both types through the identical grammar/resolution, so
// every table test runs against xs:QName and xs:NOTATION.
var qnameLocals = []string{"QName", "NOTATION"}

func TestQNamePrefixResolution(t *testing.T) {
	ctx := fakeContext{
		"":    "urn:default",
		"p":   "urn:p",
		"foo": "urn:foo",
	}
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		// Bound prefix resolves; the value round-trips its {namespace, local}
		// tuple via Eq against a second parse of the same lexical.
		v, err := m.Parse("p:x", ctx)
		if err != nil {
			t.Fatalf("%s Parse(%q): unexpected error %v", local, "p:x", err)
		}
		v2, err := m.Parse("p:x", ctx)
		if err != nil {
			t.Fatalf("%s Parse(%q) again: unexpected error %v", local, "p:x", err)
		}
		eq, ok := v.(value.Eq)
		if !ok {
			t.Fatalf("%s value does not implement value.Eq", local)
		}
		if !eq.Eq(v2) {
			t.Errorf("%s: two parses of %q are not Eq", local, "p:x")
		}
		// A distinct prefix binding to a distinct namespace is a distinct value
		// even with the same local part (tuple equality, §3.3.18.2).
		other, _ := m.Parse("foo:x", ctx)
		if eq.Eq(other) {
			t.Errorf("%s: p:x and foo:x resolve to different namespaces but compare Eq", local)
		}
		// Unprefixed name binds to the default namespace (§3.3.18): it must NOT
		// equal the same local part resolved through a non-default prefix.
		bare, err := m.Parse("x", ctx)
		if err != nil {
			t.Fatalf("%s Parse(%q): unexpected error %v", local, "x", err)
		}
		if bare.(value.Eq).Eq(other) {
			t.Errorf("%s: unqualified x (default ns) compares Eq to foo:x", local)
		}
	}
}

func TestQNameUnboundPrefixRejected(t *testing.T) {
	ctx := fakeContext{"p": "urn:p"} // no default namespace, no "q" binding
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		// An explicit but unbound prefix is a lexical-space rejection
		// (cvc-datatype-valid), never a value fabricated with an empty namespace.
		if _, err := m.Parse("q:x", ctx); !isDatatypeInvalid(t, err) {
			t.Errorf("%s Parse(%q): want cvc-datatype-valid rejection, got %v", local, "q:x", err)
		}
		// Off-contract defence: this ctx answers ("", false) for the empty
		// prefix, which value.Context forbids (no default namespace is ("",
		// true)). strict fails closed on such a Context and rejects the
		// unprefixed name rather than inventing a namespace for it.
		if _, err := m.Parse("x", ctx); !isDatatypeInvalid(t, err) {
			t.Errorf("%s Parse(%q) with no default ns: want cvc-datatype-valid rejection, got %v", local, "x", err)
		}
	}
}

func TestQNameNoDefaultNamespaceBoundToEmpty(t *testing.T) {
	// Guard: a context answering ("", true) for the empty prefix is the
	// value.Context contract's no-default-namespace case, and an unprefixed
	// name resolves to a no-namespace QName rather than being rejected.
	ctx := fakeContext{"": ""}
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		if _, err := m.Parse("x", ctx); err != nil {
			t.Errorf("%s Parse(%q) with empty default ns: unexpected error %v", local, "x", err)
		}
	}
}

func TestQNameMalformedGrammarRejected(t *testing.T) {
	ctx := fakeContext{"": "urn:default", "p": "urn:p"}
	// Every case is malformed QName grammar (bad NCName parts), independent of
	// resolution: empty local/prefix, multiple colons, invalid NCName characters.
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		for _, lex := range []string{"", ":x", "p:", "a:b:c", "1abc", "p:1x", "-x", "a b", "p:a b"} {
			if _, err := m.Parse(lex, ctx); !isDatatypeInvalid(t, err) {
				t.Errorf("%s Parse(%q): want cvc-datatype-valid rejection, got %v", local, lex, err)
			}
		}
	}
}

func TestQNameNilContextRejectedNotPanic(t *testing.T) {
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		// A nil context must reject cleanly (no namespace bindings to resolve),
		// never nil-panic — mirrors the nil ctx that context-free types accept.
		for _, lex := range []string{"p:x", "x"} {
			if _, err := m.Parse(lex, nil); !isDatatypeInvalid(t, err) {
				t.Errorf("%s Parse(%q, nil): want cvc-datatype-valid rejection, got %v", local, lex, err)
			}
		}
	}
}

func TestQNameNoCanonicalAndNotOrdered(t *testing.T) {
	ctx := fakeContext{"": "urn:default"}
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		// Canonical is nil: the spec defines no canonical form (§3.3.18/§3.3.19).
		if m.Canonical != nil {
			t.Errorf("%s: Mapping.Canonical is non-nil; want nil (no canonical form)", local)
		}
		v, err := m.Parse("x", ctx)
		if err != nil {
			t.Fatalf("%s Parse(%q): unexpected error %v", local, "x", err)
		}
		// ordered=false (§3.3.18/§3.3.19): neither type may be value.Ordered.
		if _, ok := v.(value.Ordered); ok {
			t.Errorf("%s value implements value.Ordered; it must not", local)
		}
	}
}

func TestQNameLen(t *testing.T) {
	ctx := fakeContext{"": "urn:default", "p": "urn:p"}
	// Len is the rune count of the LOCAL part (§4.3.1.3 clause 1.3 makes any
	// value length-facet-valid, so this is nominal): the prefix and namespace do
	// not contribute.
	cases := map[string]int{
		"x":      1,
		"p:name": 4, // "name"
		"café":   4, // é is one codepoint, two UTF-8 bytes
	}
	for _, local := range qnameLocals {
		m := mappingFor(t, local)
		for lex, want := range cases {
			v, err := m.Parse(lex, ctx)
			if err != nil {
				t.Fatalf("%s Parse(%q): unexpected error %v", local, lex, err)
			}
			l, ok := v.(value.Lengthed)
			if !ok {
				t.Fatalf("%s value %q does not implement value.Lengthed", local, lex)
			}
			if got := l.Len(); got != want {
				t.Errorf("%s Len(%q) = %d, want %d", local, lex, got, want)
			}
		}
	}
}

func TestQNameEqNeverCrossesNOTATION(t *testing.T) {
	ctx := fakeContext{"p": "urn:p"}
	q := mappingFor(t, "QName")
	n := mappingFor(t, "NOTATION")
	qv, err := q.Parse("p:x", ctx)
	if err != nil {
		t.Fatalf("QName Parse: %v", err)
	}
	nv, err := n.Parse("p:x", ctx)
	if err != nil {
		t.Fatalf("NOTATION Parse: %v", err)
	}
	// Identical {namespace, local} tuple, but distinct value types: Eq must be
	// false in both directions (mirrors hexBinaryVal vs base64BinaryVal).
	if qv.(value.Eq).Eq(nv) {
		t.Error("QName p:x compares Eq to NOTATION p:x; distinct types must never cross-match")
	}
	if nv.(value.Eq).Eq(qv) {
		t.Error("NOTATION p:x compares Eq to QName p:x; distinct types must never cross-match")
	}
	// A foreign Go value is unequal too.
	if qv.(value.Eq).Eq(42) {
		t.Error("QName Eq(int) = true, want false")
	}
}

// isDatatypeInvalid reports whether err is a non-nil *xsderr.Error carrying the
// cvc-datatype-valid rule — the uniform lexical-rejection rule for this backend.
func isDatatypeInvalid(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	rule, ok := xsderr.RuleOf(err)
	return ok && rule == "cvc-datatype-valid"
}
