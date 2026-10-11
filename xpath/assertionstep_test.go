package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive one element step, `N`, `./N` or `.//N`, as the
// whole operand of fn:exists, fn:empty or an ·effective boolean value·
// (ctaSelectedElements): whether it selects a node is read off the [Tally],
// whatever type N has (xpath20.md §2.4.3 rule 2, xpath-functions.md §15.1.4,
// §15.1.5).

// esElems is test18.xsd's child types (ibmData/mixed/assertions): a of an
// element-only type, b, c and d of xs:string — plus mixed, of a mixed type, and
// empty, of a type with empty content.
func esElems(t *testing.T) ElementTypes {
	t.Helper()
	str := asBuiltin(t, "string")
	return asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("a"): asComplex(t, "AOnly", asElementContent(t, false)), uq("b"): str, uq("c"): str, uq("d"): str,
		uq("mixed"): asComplex(t, "Mixed", asElementContent(t, true)), uq("empty"): asComplex(t, "Empty", xsd.EmptyContent{}),
	})
}

// esCompile compiles expr for an E with element-only content over esElems, or
// fails the test.
func esCompile(t *testing.T, expr string) AssertionTest {
	t.Helper()
	test, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), asUses(t, map[string]string{"x": "int"}), esElems(t))
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test
}

// An element step whose existence alone is asked is decided by whether the
// [Tally] counted a node under it, whatever N's type: `a and b and d` and
// `a and b and c and d` are test18.xsd's (assert_018_2), a of an element-only
// type; `exists(a)` and `empty(a)` read an element-only a, and mixed and empty
// content are selected as well. `.//d` selects a grandchild and never E itself,
// reported as the empty chain, so `not(.//d)` is false over a d grandchild and
// true with none or with only E named d. `./a` is `a`. A ·nilled· element is a
// node, which the caller reports like any other. Every row fails with
// ctaParser.selectedElements' two call sites removed: each declines at
// CompileAssertionTest (ctaAssertionFacade.child declining the element-only and
// empty types, and `./` and `.//` taking no production outside fn:count) but
// `exists(mixed)`, which compiles to a value step over the mixed child
// (ctaUntypedChild) and reads [ChildElements], which hold no child here.
func TestAssertionElementStepExistence(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		nodes []acNode
		holds bool
	}{
		{"a and b", []acNode{acPath("a"), acPath("b")}, true},
		{"a and b", []acNode{acPath("b")}, false},
		{"a and b", []acNode{acPath("a")}, false},
		{"a and b and d", []acNode{acPath("a"), acPath("a", "x"), acPath("b"), acPath("d")}, true},
		{"a and b and d", []acNode{acPath("a"), acPath("b"), acPath("c")}, false},
		{"a and b and c and d", []acNode{acPath("a"), acPath("b"), acPath("c"), acPath("d")}, true},
		{"a and b and c and d", []acNode{acPath("a"), acPath("b"), acPath("d")}, false},
		{"exists(a)", []acNode{acPath("a")}, true},
		{"exists(a)", nil, false},
		{"exists(a)", []acNode{acPath("b", "a")}, false},
		{"empty(a)", []acNode{acPath("a")}, false},
		{"empty(a)", nil, true},
		{"not(a)", nil, true},
		{"(a)", []acNode{acPath("a")}, true},
		{"mixed or empty", []acNode{acPath("empty")}, true},
		{"exists(mixed)", []acNode{acPath("mixed")}, true},
		{"not(.//d)", []acNode{acPath("b"), acPath("b", "d")}, false},
		{"not(.//d)", []acNode{acPath("b"), acPath("b", "c")}, true},
		{"not(.//d)", []acNode{acPath()}, true},
		{"not(.//d)", []acNode{acPath("d")}, false},
		{"exists(.//d)", []acNode{acPath("a", "a", "d")}, true},
		{"empty(.//d)", []acNode{acPath("a", "a", "d")}, false},
		{"./a", []acNode{acPath("a")}, true},
		{"exists(./a)", []acNode{acPath("b", "a")}, false},
		{"empty(./a) or @x = 1", []acNode{acPath("a")}, false},
		{"count(a) ge 1 and a", []acNode{acPath("a")}, true},
		{"count(a) ge 1 or a", nil, false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			test := esCompile(t, tc.expr)
			if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(test, tc.nodes...), ValueBinding{}, time.Time{}); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.nodes, got, tc.holds)
			}
		})
	}
}

// An element step whose existence is asked reads no child's value: ReadsChild
// is false for its name even where that name is xs:string and its value could
// be read — the position decides, not the type — and its Tally counts the
// step under the key an fn:count over the same step counts under, so
// `count(a) ge 1 and a` keeps one counter and `./a` is `a`. With
// ctaSelectedElements.readsChild answering true the exists(d) row fails, and
// with ctaSelectedElements.counted appending nothing the Tally rows do.
func TestAssertionElementStepReadsNoChild(t *testing.T) {
	for _, tc := range []struct {
		expr     string
		counters int
	}{
		{"exists(d)", 1},
		{"d and not(empty(./d))", 1},
		{"count(a) ge 1 and a", 1},
		{"a and .//a", 2},
	} {
		test := esCompile(t, tc.expr)
		for _, name := range []string{"a", "d"} {
			if test.ReadsChild(uq(name)) {
				t.Errorf("(%s).ReadsChild(%s) = true, want false", tc.expr, name)
			}
		}
		if c := test.Tally(); c == nil || len(c.counters) != tc.counters {
			t.Errorf("(%s).Tally() = %v, want %d counters", tc.expr, c, tc.counters)
		}
	}
	if c := esCompile(t, "not(.//d)").Tally(); c.ReadsAttributesAt(0) || c.ReadsAttributesAt(2) {
		t.Error("Tally of not(.//d) counts attributes, want none: an element step selects no attribute")
	}
}

// A step whose VALUE is read still goes through [ChildElements]: `d = 'x'`
// over an xs:string d reads it, with no Tally, and decides both ways from the
// value; `a = 1` over an element-only a, whose atomization is a type error,
// still declines, and so do `.//d = 'x'` and `./d eq 'x'`, steps behind `.`
// outside the three positions, a wildcard step, and `.//@x` outside fn:count.
// An attribute step is the attribute's node in every position, so `@x` and
// `exists(@x)` read the attribute and count nothing.
func TestAssertionValueStepStillReadsChildElements(t *testing.T) {
	test := esCompile(t, "d = 'x'")
	if !test.ReadsChild(uq("d")) || test.Tally() != nil {
		t.Errorf("(d = 'x'): ReadsChild(d) = %v, Tally() = %v, want true and nil", test.ReadsChild(uq("d")), test.Tally())
	}
	for _, tc := range []struct {
		lexical string
		want    bool
	}{{"x", true}, {"y", false}} {
		children := asChildren(t, asChild{uq("d"), "string", tc.lexical, false})
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), children, nil, ValueBinding{}, time.Time{}); got != tc.want {
			t.Errorf("Evaluate(d = 'x') over d %q = %v, want %v", tc.lexical, got, tc.want)
		}
	}
	for _, expr := range []string{"a = 1", "a eq 1", ".//d = 'x'", "exists(.//d = 'x')", "./d eq 'x'", "string(a) = ''", "exists(.//*)", "not(.//@x)"} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), asUses(t, map[string]string{"x": "int"}), esElems(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"@x", "exists(@x)"} {
		if attr := esCompile(t, expr); attr.Tally() != nil {
			t.Errorf("(%s).Tally() = %v, want nil: an attribute step is no element step", expr, attr.Tally())
		}
	}
}

// ctaSelectedElementsOf refuses an attribute axis, which `@N` and `.//@N`
// already have ctaAttr and ctaTypedAttr for, and admits both element axes.
func TestSelectedElementsRefusesAttributeAxes(t *testing.T) {
	for _, tc := range []struct {
		axis ctaCountAxis
		want bool
	}{
		{ctaCountChildren, true},
		{ctaCountDescendants, true},
		{ctaCountOwnAttributes, false},
		{ctaCountSubtreeAttributes, false},
	} {
		if _, ok := ctaSelectedElementsOf(ctaCountPath{axis: tc.axis, name: uq("a")}); ok != tc.want {
			t.Errorf("ctaSelectedElementsOf(axis %d) = %v, want %v", tc.axis, ok, tc.want)
		}
	}
}

// An element step is the assertion façade's alone: a Type Alternative's {test}
// declines `N` and `.//N` in every position (ctaTypeAlternativeFacade.elements),
// and an assertions facet's reads its absent context item and Fails, never
// Holds and never reads a Tally (ctaFacetFacade.elements, err:XPDY0002), fn:not
// over it included, the error propagating. The facet rows over `.//a` and `./a`
// decline instead with ctaParser.selectedElements' two call sites removed, the
// "/" or "//" after `.` taking no production; the Type Alternative rows compile
// with its elements admitting the step.
func TestElementStepOutsideTheAssertionFacade(t *testing.T) {
	for _, expr := range []string{"a", "not(a)", "exists(a)", ".//a", "not(.//a)", "./a and @x"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	str := asBuiltin(t, "string")
	for _, test := range []string{"a", "not(a)", "exists(a)", "empty(a)", ".//a", "not(.//a)", "exists(.//a)", "./a"} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, ""), fcValue(t, str, "x")); got != value.AssertionFails {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want Fails (%d): never Holds", test, got, value.AssertionFails)
		}
	}
}
