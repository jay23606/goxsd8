package xsderr

import (
	"errors"
	"fmt"
	"testing"
)

// The "cvc-elt.1" Rule in TestErrorRendering, TestErrorRenderingUnknownLoc and
// TestWrapNil is incidental, not a claim that it is valid — IsValidRule rejects
// it. Those tests exercise Error() and Wrap() rendering and nil handling, where
// any string would serve identically.

func TestErrorRendering(t *testing.T) {
	err := New("cvc-elt.1", Loc{URI: "doc.xsd", Line: 12, Col: 4}, "element %q not resolvable", "foo")
	got := err.Error()
	want := `doc.xsd:12:4: [cvc-elt.1] element "foo" not resolvable`
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestErrorRenderingUnknownLoc(t *testing.T) {
	err := New("cvc-elt.1", Loc{}, "boom")
	want := "?: [cvc-elt.1] boom"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// TestLocString pins each arm of Loc.String's rendering (#1896).
func TestLocString(t *testing.T) {
	for _, tc := range []struct {
		loc  Loc
		want string
	}{
		{Loc{}, "?"},
		{Loc{URI: "u"}, "u"},
		{Loc{URI: "u", Line: 3}, "u:3"},
		{Loc{URI: "u", Col: 5}, "u"},
		{Loc{URI: "u", Line: -1}, "u"},
		{Loc{Line: 3, Col: 4}, "?:3:4"},
		{Loc{Col: 5}, "?"},
		{Loc{URI: "u", Line: 3, Col: 4}, "u:3:4"},
	} {
		if got := tc.loc.String(); got != tc.want {
			t.Errorf("%#v.String() = %q, want %q", tc.loc, got, tc.want)
		}
	}
}

func TestWrapReachThrough(t *testing.T) {
	sentinel := errors.New("underlying cause")
	wrapped := Wrap("src-resolve", Loc{URI: "a.xsd", Line: 1, Col: 1}, sentinel)

	if !errors.Is(wrapped, sentinel) {
		t.Fatalf("errors.Is(wrapped, sentinel) = false, want true")
	}
	// Wrap preserves a plain cause's message verbatim rather than duplicating it.
	if wrapped.Msg != sentinel.Error() {
		t.Fatalf("Msg = %q, want %q", wrapped.Msg, sentinel.Error())
	}
	want := "a.xsd:1:1: [src-resolve] underlying cause"
	if got := wrapped.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// TestWrapCauseRendering pins Wrap's Error() line over an *Error cause. The
// "zero-Loc cause" rows fail if Msg is err.Error() again, which prints the
// cause's unknown position as "?: "; the "placed cause" row fails if every
// *Error cause renders from its fields, which drops the cause's own Loc.
func TestWrapCauseRendering(t *testing.T) {
	outer := Loc{URI: "a.xsd", Line: 1, Col: 2}
	for _, tc := range []struct {
		name  string
		loc   Loc
		cause error
		want  string
	}{
		{"zero-Loc cause, placed outer", outer, New("cvc-datatype-valid", Loc{}, "inner"), "a.xsd:1:2: [maxInclusive-valid-restriction] [cvc-datatype-valid] inner"},
		{"zero-Loc cause, zero outer", Loc{}, New("cvc-datatype-valid", Loc{}, "inner"), "?: [maxInclusive-valid-restriction] [cvc-datatype-valid] inner"},
		{"placed cause", outer, New("cvc-datatype-valid", Loc{URI: "b.xsd", Line: 3, Col: 4}, "inner"), "a.xsd:1:2: [maxInclusive-valid-restriction] b.xsd:3:4: [cvc-datatype-valid] inner"},
	} {
		if got := Wrap("maxInclusive-valid-restriction", tc.loc, tc.cause).Error(); got != tc.want {
			t.Errorf("%s: Error() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWrapNil(t *testing.T) {
	if got := Wrap("cvc-elt.1", Loc{}, nil); got != nil {
		t.Fatalf("Wrap(nil) = %v, want nil", got)
	}
}

// TestCauseText pins one row per arm of CauseText's rendering. The "wrapper"
// row fails if the assertion becomes errors.As, which reaches the zero-Loc
// *Error inside the fmt wrapper and drops the wrapper's own text.
func TestCauseText(t *testing.T) {
	unplaced := New("cvc-datatype-valid", Loc{}, "inner")
	var typedNil *Error
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"bare zero-Loc", unplaced, "[cvc-datatype-valid] inner"},
		{"placed", New("cvc-datatype-valid", Loc{URI: "a.xsd", Line: 2, Col: 3}, "inner"), "a.xsd:2:3: [cvc-datatype-valid] inner"},
		{"partial Loc", New("cvc-datatype-valid", Loc{Line: 3}, "inner"), "?:3: [cvc-datatype-valid] inner"},
		{"plain", errors.New("plain cause"), "plain cause"},
		{"wrapper", fmt.Errorf("context: %w", unplaced), "context: ?: [cvc-datatype-valid] inner"},
		{"nil", nil, "<nil>"},
		{"typed-nil", typedNil, "<nil>"},
	} {
		if got := CauseText(tc.err); got != tc.want {
			t.Errorf("%s: CauseText = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRuleOfAndLocOf(t *testing.T) {
	loc := Loc{URI: "b.xsd", Line: 7, Col: 2}
	err := New("cos-st-restricts", loc, "bad restriction")

	// Narrowing through an intermediate fmt-wrapped error still reaches the *Error.
	chained := fmt.Errorf("context: %w", err)

	rule, ok := RuleOf(chained)
	if !ok || rule != "cos-st-restricts" {
		t.Fatalf("RuleOf = (%q, %v), want (cos-st-restricts, true)", rule, ok)
	}

	gotLoc, ok := LocOf(chained)
	if !ok || gotLoc != loc {
		t.Fatalf("LocOf = (%v, %v), want (%v, true)", gotLoc, ok, loc)
	}
}

func TestRuleOfLocOfNotAnError(t *testing.T) {
	plain := errors.New("plain error, not an *Error")

	if rule, ok := RuleOf(plain); ok {
		t.Fatalf("RuleOf(plain) = (%q, true), want (_, false)", rule)
	}
	if loc, ok := LocOf(plain); ok {
		t.Fatalf("LocOf(plain) = (%v, true), want (_, false)", loc)
	}
}

func TestIsValidRule(t *testing.T) {
	// cos-st-restricts is a real anchor confirmed present in the generated
	// catalog (and a doc.go example Rule).
	if !IsValidRule("cos-st-restricts") {
		t.Fatalf("IsValidRule(%q) = false, want true", "cos-st-restricts")
	}
	if IsValidRule("totally-made-up-rule") {
		t.Fatalf("IsValidRule(%q) = true, want false", "totally-made-up-rule")
	}
}

// TestIsValidRuleDerivationOkRestriction guards the doc.go contract example:
// derivation-ok-restriction is an irregular rule ID that the extraction must
// capture, or the package's own documentation would be false.
func TestIsValidRuleDerivationOkRestriction(t *testing.T) {
	if !IsValidRule("derivation-ok-restriction") {
		t.Fatalf("IsValidRule(%q) = false, want true", "derivation-ok-restriction")
	}
}

// TestIsValidRuleComponentInvariant guards the second non-spec sentinel
// exemption: a component-constructor representation-invariant rejection has no
// catalog rule ID, so RuleComponentInvariant must be accepted by IsValidRule
// even though it is absent from the generated catalog.
func TestIsValidRuleComponentInvariant(t *testing.T) {
	if !IsValidRule(RuleComponentInvariant) {
		t.Fatalf("IsValidRule(%q) = false, want true", RuleComponentInvariant)
	}
	// It is deliberately NOT in the generated spec catalog.
	if _, ok := ruleCatalog[RuleComponentInvariant]; ok {
		t.Fatalf("RuleComponentInvariant %q leaked into the generated ruleCatalog", RuleComponentInvariant)
	}
}

// TestIsValidRuleXMLWellFormed guards the non-spec sentinel exemption: XML
// well-formedness faults have no catalog rule ID, so RuleXMLWellFormed must be
// accepted by IsValidRule even though it is absent from the generated catalog.
func TestIsValidRuleXMLWellFormed(t *testing.T) {
	if !IsValidRule(RuleXMLWellFormed) {
		t.Fatalf("IsValidRule(%q) = false, want true", RuleXMLWellFormed)
	}
	// It is deliberately NOT in the generated spec catalog.
	if _, ok := ruleCatalog[RuleXMLWellFormed]; ok {
		t.Fatalf("RuleXMLWellFormed %q leaked into the generated ruleCatalog", RuleXMLWellFormed)
	}
}
