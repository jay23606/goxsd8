package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
)

// An attribute step behind `./` steps from the context node `.` (xpath20.md
// §3.2, §3.2.4), which in an assertion is E: `./@s` is E's own attribute `@s`,
// read TYPED, in either spelling of the axis, as a comparison or arithmetic
// operand and under fn:exists — and assert011's `./@event =
// doc('…')//event/@name` compiles, the call raising err:FODC0005.
func TestAssertionOwnAttributeStep(t *testing.T) {
	sx := []asTyped{{uq("s"), "string", "x"}}
	sy := []asTyped{{uq("s"), "string", "y"}}
	i2 := []asTyped{{uq("i"), "integer", "2"}}
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"./@s = 'x'", sx, true},
		{"./@s = 'x'", sy, false},
		{"./attribute::s = 'x'", sx, true},
		{"'x' = ./@s", sx, true},
		{"exists(./@s)", sx, true},
		{"exists(./@s)", nil, false},
		{"./@i + 1 = 3", i2, true},
		{"./@s = doc('assert011-event-list.xml')//event/@name", sx, false},
	} {
		if got := afEval(t, tc.expr, tc.attrs); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}

// `./@a` stays outside a Type Alternative's grammar, which has no
// ContextItemExpr, and outside a value predicate, whose `.` is the candidate
// child and not E. The Type Alternative rows compile with
// ctaParser.ownAttribute's contextNode gate removed; the predicate rows are a
// guard, declining without it too at ctaPredicateFacade.attribute. An
// assertions facet has no context item, so `./@a` raises err:XPDY0002 and
// fails the facet, as `@a` does.
func TestOwnAttributeStepOutsideTheAssertionFacade(t *testing.T) {
	for _, expr := range []string{"./@a = 'x'", "./attribute::a = 'x'"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"count(e1[./@a = 'x']) = 1", "count(e1[. = ./@s]) = 1"} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), afUses(t), asChildTypes(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	str := asBuiltin(t, "string")
	for _, test := range []string{"./@a = 'x'", "not(./@a = 'x')"} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, ""), fcValue(t, str, "x"))
		if got != value.AssertionFails {
			t.Errorf("FacetAssertions.Evaluate(%q) = %d, want %d", test, got, value.AssertionFails)
		}
	}
}
