package xpath

import (
	"slices"
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive fn:data over a relative path of two child steps
// (xpath-functions.md §2.4, xpath20.md §3.2, ctaElementPath): `data(a/b)`
// atomizes each b grandchild of E under its a child, in document order, to
// its typed value (§2.4.2), which the caller reports to the [Tally]
// (Tally.ElementValue) once b's subtree is reported, as saxonData's
// assert022.xsd:12 writes `data(event/d) instance of xs:date*`.

// evPath is the chain below E of the elements whose values the rows read: each
// b under E's a child.
var evPath = []xsd.QName{uq("a"), uq("b")}

// evElems is the [ElementTypes] of the rows: [a] has element-only content,
// [a, b] is last, [y] is xs:integer, [a, m] has mixed content, [a, o] has
// element-only content, [a, z] has empty content, [a, s] is xs:anySimpleType
// and [a, c] is a complex type with xs:date simple content; every other path
// types nothing.
func evElems(t *testing.T, last xsd.TypeDefinition) ElementTypes {
	t.Helper()
	only := asComplex(t, "Only", asElementContent(t, false))
	types := map[string]xsd.TypeDefinition{
		"a":   only,
		"a/b": last,
		"y":   asBuiltin(t, "integer"),
		"a/m": asComplex(t, "Mixed", asElementContent(t, true)),
		"a/o": only,
		"a/z": asComplex(t, "Empty", xsd.EmptyContent{}),
		"a/s": asBuiltin(t, "anySimpleType"),
		"a/c": asComplex(t, "DateContent", xsd.SimpleContent{SimpleType: asBuiltin(t, "date")}),
	}
	return func(path []xsd.QName) (xsd.TypeDefinition, bool) {
		key := ""
		for i, step := range path {
			if i > 0 {
				key += "/"
			}
			key += step.Local
		}
		td, ok := types[key]
		return td, ok
	}
}

// evUses types p at evPath as xs:decimal, and nothing else.
func evUses(t *testing.T) AttributeTypes {
	t.Helper()
	d := asBuiltin(t, "decimal")
	return func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		return d, slices.Equal(path, evPath) && name == uq("p")
	}
}

// evCompileAs compiles expr for an E with element-only content whose b
// grandchildren under a are of the builtin last, reporting whether it
// compiled.
func evCompileAs(t *testing.T, expr, last string) (AssertionTest, bool) {
	t.Helper()
	return CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), evUses(t), evElems(t, asBuiltin(t, last)))
}

// evCompile is evCompileAs over xs:date b grandchildren, failing the test where
// it declines.
func evCompile(t *testing.T, expr string) AssertionTest {
	t.Helper()
	test, ok := evCompileAs(t, expr, "date")
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test
}

// evDate is lexical as the typed value of an xs:date element.
func evDate(t *testing.T, lexical string) TypedValue {
	t.Helper()
	v, err := value.ValidateLexical(backend(), seededTypes, asBuiltin(t, "date"), lexical, nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping %q as xs:date: %v", lexical, err)
	}
	return Typed(v)
}

// evTally is test's Tally filled as [Tally] obliges: E, its one a child where
// withA is true, and under it one b per value of bs, in order, each value
// reported right after its b, which has no subtree.
func evTally(test AssertionTest, withA bool, bs ...TypedValue) *Tally {
	c := test.Tally()
	c.Element(nil, nil)
	if !withA {
		return c
	}
	c.Element(evPath[:1], nil)
	for _, v := range bs {
		c.Element(evPath, nil)
		c.ElementValue(v)
	}
	return c
}

// evEval evaluates test over counts, with E's children children.
func evEval(t *testing.T, test AssertionTest, counts *Tally, children ...asChild) bool {
	t.Helper()
	return test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, children...), counts, ValueBinding{}, time.Time{})
}

// `data(a/b) instance of xs:date*` holds over every number of xs:date b
// grandchildren — none because E has no a, none under an a, one and two
// (xpath20.md §2.5.4.1: `*` admits zero or more) — while `instance of
// xs:date` holds over exactly one item and is a plain false over none and over
// two, never an error. A ·nilled· b is a node whose typed value is the empty
// sequence, so it contributes no item. Every row declines at
// CompileAssertionTest, and so fails, with dataInstanceOf routing no path to
// dataElementPath.
func TestAssertionElementPathInstanceOf(t *testing.T) {
	d1, d2 := evDate(t, "2008-07-01"), evDate(t, "2010-11-17Z")
	for _, tc := range []struct {
		expr  string
		withA bool
		bs    []TypedValue
		holds bool
	}{
		{"data(a/b) instance of xs:date*", false, nil, true},
		{"data(a/b) instance of xs:date*", true, nil, true},
		{"data(a/b) instance of xs:date*", true, []TypedValue{d1}, true},
		{"data(a/b) instance of xs:date*", true, []TypedValue{d1, d2}, true},
		{"data(a/b) instance of xs:date", true, []TypedValue{d1, d2}, false},
		{"data(a/b) instance of xs:date", true, []TypedValue{d1}, true},
		{"data(a/b) instance of xs:date", true, nil, false},
		{"not(data(a/b) instance of xs:date)", true, []TypedValue{d1, d2}, true},
		{"data(a/b) instance of xs:date", true, []TypedValue{nil, d1}, true},
		{"data(a/b) instance of xs:date", true, []TypedValue{nil}, false},
		{"data(a/b) instance of xs:date?", true, []TypedValue{nil}, true},
	} {
		test, ok := evCompileAs(t, tc.expr, "date")
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			continue
		}
		if got := evEval(t, test, evTally(test, tc.withA, tc.bs...)); got != tc.holds {
			t.Errorf("%q over %d b (a present: %v): Evaluate = %v, want %v", tc.expr, len(tc.bs), tc.withA, got, tc.holds)
		}
	}
}

// A b's annotation is its own governing type's (cvc-assertion clause 1.2), which
// an xsi:type can make a type derived from the xs:decimal the path was compiled
// against, so an AtomicType xs:decimal derives from matches every item while
// one it does not derive from, xs:integer, decides nothing and declines rather
// than answering false (ctaTypes.instanceItem's derived). With
// ctaTypes.instanceItem reporting a ctaElementPath with derived false, the
// xs:integer* row compiles, and fails; with dataInstanceOf routing no path to
// dataElementPath, the two xs:decimal* rows decline, and fail.
func TestAssertionElementPathDerivedDeclines(t *testing.T) {
	if _, ok := evCompileAs(t, "data(a/b) instance of xs:integer*", "decimal"); ok {
		t.Error("data(a/b) instance of xs:integer* over xs:decimal b: compiled, want declined")
	}
	if _, ok := evCompileAs(t, "data(a/b) instance of xs:decimal*", "decimal"); !ok {
		t.Error("data(a/b) instance of xs:decimal* over xs:decimal b: declined, want compiled")
	}
	if _, ok := evCompileAs(t, "data(a/b) instance of xs:decimal*", "integer"); !ok {
		t.Error("data(a/b) instance of xs:decimal* over xs:integer b: declined, want compiled")
	}
}

// A path the compile cannot type declines: a step [ElementTypes] answers false
// for (a/q, q/b, and b/a), an AtomicType xs:date does not derive from, which
// decides nothing on TestAssertionElementPathDerivedDeclines' terms, a mixed
// last step, whose string-value no frame collects below a child, an
// element-only or empty one, whose fn:data raises err:FOTY0012 or yields
// nothing and is no node here, a ·special· one, a wildcard last step, a longer
// path's own decline, and the forms beyond fn:data under `instance of`. A last
// step of simple content over xs:date compiles. The xs:string* row compiles,
// and fails, with ctaTypes.instanceItem reporting a ctaElementPath with
// derived false, and the simple-content row declines, and fails, with
// dataInstanceOf routing no path to dataElementPath; every other row is a
// guard, `a/b = 1`, `count(a/b) = 1` and `string(a/b)` among them.
func TestAssertionElementPathDeclines(t *testing.T) {
	for _, expr := range []string{
		"data(a/q) instance of xs:date*",
		"data(a/b) instance of xs:string*",
		"data(q/b) instance of xs:date*",
		"data(b/a) instance of xs:date*",
		"data(a/m) instance of xs:untypedAtomic*",
		"data(a/o) instance of xs:date*",
		"data(a/z) instance of xs:date*",
		"data(a/s) instance of xs:untypedAtomic*",
		"data(a/*) instance of xs:date*",
		"data(a/b/c) instance of xs:date*",
		"data(a//b) instance of xs:date*",
		"data(a/b) = xs:date('2008-07-01')",
		"data(a/b)",
		"a/b = 1",
		"count(a/b) = 1",
		"string(a/b) = ''",
		"a/b instance of xs:date*",
	} {
		if _, ok := evCompileAs(t, expr, "date"); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	if _, ok := evCompileAs(t, "data(a/c) instance of xs:date*", "date"); !ok {
		t.Error("data(a/c) instance of xs:date* over simple content: declined, want compiled")
	}
}

// A Type Alternative's {test} declines the path, fn:data being outside its
// grammar (§3.12.6), and an assertions facet's raises err:XPDY0002 over it, its
// first step reading the absent context item, so the facet fails rather than
// declining (ctaFacetFacade.elementPath). The facet row declines, and fails,
// with dataInstanceOf routing no path to dataElementPath; the Type Alternative
// row is a guard.
func TestAssertionElementPathOtherFacades(t *testing.T) {
	if _, ok := CompileCTATest(asRecord("data(a/b) instance of xs:int*"), seededTypes); ok {
		t.Error("CompileCTATest(data(a/b) instance of xs:int*): compiled, want declined")
	}
	str := asBuiltin(t, "string")
	if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, asRecord("data(a/b) instance of xs:date*"), fcValue(t, str, "x")); got != value.AssertionFails {
		t.Errorf("facet data(a/b) instance of xs:date*: Evaluate = %d, want AssertionFails", got)
	}
}

// A Tally reads the values of the elements a path of child steps selects:
// ReadsElementValue answers for [a, b] alone — not E, not the a child, not
// [a, c], not [b, a] — no attribute name at any depth, and neither a nor b is
// a child read; the {test} whose only read below E is `data(a/b)` has a Tally.
// A nil Tally reads nothing and takes nothing. With ctaElementPath.reads
// matching any path of its length, the [a, c] and [b, a] rows fail.
func TestTallyReadsElementValues(t *testing.T) {
	test := evCompile(t, "data(a/b) instance of xs:date*")
	c := test.Tally()
	if c == nil {
		t.Fatal("Tally() of data(a/b) instance of xs:date* is nil, want one")
	}
	for _, tc := range []struct {
		path  []xsd.QName
		reads bool
	}{
		{evPath, true},
		{evPath[:1], false},
		{nil, false},
		{[]xsd.QName{uq("a"), uq("c")}, false},
		{[]xsd.QName{uq("b"), uq("a")}, false},
	} {
		if got := c.ReadsElementValue(tc.path); got != tc.reads {
			t.Errorf("ReadsElementValue(%v) = %v, want %v", tc.path, got, tc.reads)
		}
	}
	for depth := range 4 {
		if c.ReadsAttributesAt(depth) {
			t.Errorf("ReadsAttributesAt(%d) = true, want false", depth)
		}
	}
	if test.ReadsChild(uq("a")) || test.ReadsChild(uq("b")) {
		t.Error("ReadsChild(a) or ReadsChild(b) = true, want false")
	}
	var none *Tally
	if none.ReadsElementValue(evPath) {
		t.Error("nil Tally: ReadsElementValue = true, want false")
	}
	none.ElementValue(evDate(t, "2008-07-01"))
}

// Each breach of [Tally.ElementValue]'s ordering drops the collectors, so
// Evaluate answers false where the same reports made in order answer true: a
// value before any report, after E's report, after a child's, a second value
// for one b, a value while an attribute value of the latest report is owed, a
// report at b's depth or above while b owes its value, and Evaluate over a
// Tally still owing one. A value after the reports of b's own subtree is in
// order. With Tally.ElementValue returning, not breaching, where nothing is
// owed, the before-any-report, after-E's, after-a's and twice rows hold; with
// its owed-attribute check removed, the value-while-the-attribute-is-owed row
// holds; with Tally.Element's owing-depth check removed, the second-b and
// an-a rows hold; and with Tally.settled ignoring the values owed, the
// owing-at-Evaluate row holds.
func TestTallyElementValueBreaches(t *testing.T) {
	d := evDate(t, "2008-07-01")
	b := evPath
	sub := []xsd.QName{uq("a"), uq("b"), uq("x")}
	for _, tc := range []struct {
		name  string
		fill  func(c *Tally)
		holds bool
	}{
		{"in order", func(c *Tally) { c.Element(nil, nil); c.Element(b[:1], nil); c.Element(b, nil); c.ElementValue(d) }, true},
		{"after b's subtree", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(b[:1], nil)
			c.Element(b, nil)
			c.Element(sub, nil)
			c.ElementValue(d)
		}, true},
		{"before any report", func(c *Tally) { c.ElementValue(d); c.Element(nil, nil) }, false},
		{"after E's report", func(c *Tally) { c.Element(nil, nil); c.ElementValue(d) }, false},
		{"after a's report", func(c *Tally) { c.Element(nil, nil); c.Element(b[:1], nil); c.ElementValue(d) }, false},
		{"twice for one b", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(b[:1], nil)
			c.Element(b, nil)
			c.ElementValue(d)
			c.ElementValue(d)
		}, false},
		{"a second b while one owes", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(b[:1], nil)
			c.Element(b, nil)
			c.Element(b, nil)
			c.ElementValue(d)
			c.ElementValue(d)
		}, false},
		{"an a while b owes, then b's value", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(b[:1], nil)
			c.Element(b, nil)
			c.Element(b[:1], nil)
			c.ElementValue(d)
		}, false},
		{"owing at Evaluate", func(c *Tally) { c.Element(nil, nil); c.Element(b[:1], nil); c.Element(b, nil) }, false},
	} {
		test := evCompile(t, "data(a/b) instance of xs:date*")
		c := test.Tally()
		tc.fill(c)
		if got := evEval(t, test, c); got != tc.holds {
			t.Errorf("%s: Evaluate = %v, want %v", tc.name, got, tc.holds)
		}
	}
	both := evCompile(t, "sum(a/b/@p) = 0 and data(a/b) instance of xs:date*")
	for _, tc := range []struct {
		name  string
		fill  func(c *Tally)
		holds bool
	}{
		{"attribute then value", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(b[:1], nil)
			c.Element(b, []xsd.QName{uq("p")})
			c.Attribute(uq("p"), smValue(t, "decimal", "0"))
			c.ElementValue(d)
		}, true},
		{"value while the attribute is owed", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(b[:1], nil)
			c.Element(b, []xsd.QName{uq("p")})
			c.ElementValue(d)
			c.Attribute(uq("p"), smValue(t, "decimal", "0"))
		}, false},
	} {
		c := both.Tally()
		tc.fill(c)
		if got := evEval(t, both, c); got != tc.holds {
			t.Errorf("%s: Evaluate = %v, want %v", tc.name, got, tc.holds)
		}
	}
}

// A value of the [Untyped] arm where the path was compiled TYPED breaks
// [Tally.ElementValue]'s obligation, and the tree raises on it: false where the
// [Typed] value holds, even under `*`, which no item count fails. The Untyped
// row holds with ctaElementPathItem skipping a value of the wrong arm as it
// skips nil.
func TestAssertionElementPathRaisesOnWrongArm(t *testing.T) {
	for _, tc := range []struct {
		v     TypedValue
		holds bool
	}{
		{evDate(t, "2008-07-01"), true},
		{Untyped("2008-07-01"), false},
	} {
		test := evCompile(t, "data(a/b) instance of xs:date*")
		if got := evEval(t, test, evTally(test, true, tc.v)); got != tc.holds {
			t.Errorf("b reported as %#v: Evaluate = %v, want %v", tc.v, got, tc.holds)
		}
	}
}

// Inside a quantifier's body the focus stays at E (xpath20.md §3.9), so the
// path reads E's grandchildren there as it does outside: in a body over a typed
// `$value` (ctaRangeFacade) and in one over a typed child step
// (ctaChildRangeFacade), each inheriting ctaAssertionFacade.elementPath. Over
// one b the body holds and over two `instance of xs:date` does not. The two
// `some $x` rows decline at CompileAssertionTest, and so fail, with
// ctaRangeFacade overriding elementPath to decline, and the two `every $v`
// rows with ctaChildRangeFacade overriding it.
func TestAssertionElementPathInQuantifierBodies(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	types := asTypesWith(list)
	d := evDate(t, "2008-07-01")
	for _, tc := range []struct {
		bs    []TypedValue
		holds bool
	}{{[]TypedValue{d}, true}, {[]TypedValue{d, d}, false}} {
		expr := "some $x in data($value) satisfies data(a/b) instance of xs:date"
		test, ok := CompileAssertionTest(asRecord(expr), types, xsd.SimpleContent{SimpleType: list}, evUses(t), evElems(t, asBuiltin(t, "date")))
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		if ok {
			got := test.Evaluate(backend(), types, asElem, asValues(t), asNoChildren, evTally(test, true, tc.bs...), asBindIn(t, types, list, "1"), time.Time{})
			if got != tc.holds {
				t.Errorf("%q over %d b: Evaluate = %v, want %v", expr, len(tc.bs), got, tc.holds)
			}
		}

		expr = "every $v in y satisfies data(a/b) instance of xs:date"
		test, ok = evCompileAs(t, expr, "date")
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", expr)
			continue
		}
		c := test.Tally()
		c.Element(nil, nil)
		c.Element([]xsd.QName{uq("y")}, nil)
		c.Element(evPath[:1], nil)
		for _, v := range tc.bs {
			c.Element(evPath, nil)
			c.ElementValue(v)
		}
		if got := evEval(t, test, c, asChild{name: uq("y"), typ: "integer", lexical: "1"}); got != tc.holds {
			t.Errorf("%q over %d b: Evaluate = %v, want %v", expr, len(tc.bs), got, tc.holds)
		}
	}
}
