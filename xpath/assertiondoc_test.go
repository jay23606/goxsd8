package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// An fn:doc call over a StringLiteral raises err:FODC0005: cvc-xpath clause 8
// makes the available documents empty, so no string has a mapping
// (xpath-functions.md §15.5.4), and the {test} is false whatever E carries —
// as a comparison operand, assert011's `doc('…')//event/@name` among them,
// under fn:exists and fn:empty, as the whole {test}, and under fn:not, the
// error propagating. The path after the call is never evaluated.
func TestAssertionDocRaises(t *testing.T) {
	s := []asTyped{{uq("s"), "string", "x"}}
	for _, expr := range []string{
		"@s = doc('x.xml')",
		"doc('x.xml') = @s",
		"@s = doc('assert011-event-list.xml')//event/@name",
		"@s = doc('x.xml')/r/@s",
		"exists(doc('x.xml'))",
		"empty(doc('x.xml'))",
		"not(exists(doc('x.xml')))",
		"doc('x.xml')",
		"doc('x.xml')//r",
		"not(doc('x.xml'))",
		"count(doc('x.xml')) = 0",
		"doc('not a uri') = @s",
	} {
		if afEval(t, expr, s) {
			t.Errorf("Evaluate(%q) = true, want false: fn:doc raises err:FODC0005", expr)
		}
	}
}

// `doc(())` is the empty sequence (xpath-functions.md §15.5.4: "If $uri is the
// empty sequence, the result is an empty sequence"), which no available
// document decides, and so is any path after it; a `$value` that is
// statically empty, under a {content type} that is not simple, is the same
// argument.
func TestAssertionDocOverTheEmptySequence(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"empty(doc(()))", true},
		{"exists(doc(()))", false},
		{"empty(doc(())//event/@name)", true},
		{"not(doc(()))", true},
		{"empty(doc($value))", true},
	} {
		if got := afEval(t, tc.expr, nil); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// Every other fn:doc shape declines on the assertion façade: an argument that
// is no StringLiteral and not statically empty, an arity but one, a path
// after the call with a wildcard, a spelled-out axis, a predicate, an
// unbound prefix or no step after its "/".
func TestAssertionDocDeclines(t *testing.T) {
	for _, expr := range []string{
		"doc(@s) = 'x'",
		"doc(1) = 'x'",
		"exists(doc())",
		"exists(doc('a', 'b'))",
		"exists(doc('x')[1])",
		"exists(doc('x')/*)",
		"exists(doc('x')/child::r)",
		"exists(doc('x')/p:r)",
		"exists(doc('x')/)",
	} {
		if _, ok := afCompile(t, expr, xsd.EmptyContent{}); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	if _, ok := afCompile(t, "doc($value) = 'x'", xsd.SimpleContent{SimpleType: asBuiltin(t, "string")}); ok {
		t.Error("CompileAssertionTest(doc($value) = 'x') over simple content: compiled, want declined")
	}
}

// fn:doc never reaches a node outside the assertion façade (guard). A Type
// Alternative's {test} calls no library function, so the call declines at
// the constructor function and charges nothing (key-cta-ta-select). An
// assertions facet declines a call over a StringLiteral
// (ctaFacetFacade.document) and decides `doc(())`, the empty sequence.
func TestDocOutsideTheAssertionFacade(t *testing.T) {
	for _, expr := range []string{"@a = doc('x.xml')", "doc('x.xml')", "not(doc('x.xml'))"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		test string
		want value.AssertionOutcome
	}{
		{"exists(doc('x.xml'))", value.AssertionDeclined},
		{"$value = doc('x.xml')//event/@name", value.AssertionDeclined},
		{"empty(doc(()))", value.AssertionHolds},
		{"exists(doc(()))", value.AssertionFails},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(tc.test, ""), fcValue(t, str, "x"))
		if got != tc.want {
			t.Errorf("FacetAssertions.Evaluate(%q) = %d, want %d", tc.test, got, tc.want)
		}
	}
}
