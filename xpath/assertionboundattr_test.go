package xpath

import (
	"slices"
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive an attribute step over a quantifier's range
// variable and over the sibling after it, with a function call as the path's
// last step (ctaBoundAttribute, ctaBoundAttributeCall), the form of
// ibmData/mixed/assertions/test15.xsd:17: each attribute is read off the
// [ChildElement] of the child that has it ([ChildWithAttributes]), and the
// call is evaluated once per attribute node, that node its context item
// (xpath20.md §3.2).

// baTest15 is test15.xsd:17's {test}.
const baTest15 = "every $x in x[position() lt last()] satisfies number($x/@a/substring-after(.,'val')) lt number($x/following-sibling::x[1]/@a/substring-after(.,'val'))"

// baElems is E's child element types: x and y are complex types with simple
// content over xs:integer, and z is xs:string.
func baElems(t *testing.T) ElementTypes {
	t.Helper()
	simple := asComplex(t, "XType", xsd.SimpleContent{SimpleType: asBuiltin(t, "integer")})
	return asElems(map[xsd.QName]xsd.TypeDefinition{uq("x"): simple, uq("y"): simple, uq("z"): asBuiltin(t, "string")})
}

// baUses is the attribute types of E's children: x's a and c are xs:string,
// its i xs:integer and its w xs:anySimpleType, and y's b is xs:string. E
// itself has none.
func baUses(t *testing.T) AttributeTypes {
	t.Helper()
	types := map[[2]string]string{
		{"x", "a"}: "string", {"x", "c"}: "string", {"x", "i"}: "integer", {"x", "w"}: "anySimpleType",
		{"y", "b"}: "string",
	}
	return func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		if len(path) != 1 {
			return nil, false
		}
		typ, ok := types[[2]string{path[0].Local, name.Local}]
		if !ok {
			return nil, false
		}
		return asBuiltin(t, typ), true
	}
}

// baCompile compiles expr for an E with element-only content whose children
// baElems types and whose children's attributes baUses types.
func baCompile(t *testing.T, expr string) (AssertionTest, bool) {
	t.Helper()
	return CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), baUses(t), baElems(t))
}

// baChild is one child of E: its name, its xs:integer lexical, "" for a child
// yielded for its attributes alone, whose value is nil, and its attributes,
// each xs:string.
type baChild struct {
	name    string
	lexical string
	attrs   []string
}

// baChildren is the [ChildElements] over children in the order written, each
// built by [ChildWithAttributes], its attributes given as alternating names
// and lexicals.
func baChildren(t *testing.T, children ...baChild) ChildElements {
	t.Helper()
	out := make([]ChildElement, 0, len(children))
	for _, c := range children {
		var v TypedValue
		if c.lexical != "" {
			v = Typed(fcValue(t, asBuiltin(t, "integer"), c.lexical))
		}
		var attrs []asTyped
		for i := 0; i+1 < len(c.attrs); i += 2 {
			attrs = append(attrs, asTyped{uq(c.attrs[i]), "string", c.attrs[i+1]})
		}
		out = append(out, ChildWithAttributes(uq(c.name), v, asValues(t, attrs...)))
	}
	return func(yield func(ChildElement) bool) {
		for _, c := range out {
			if !yield(c) {
				return
			}
		}
	}
}

// baDecides compiles expr, failing the test where it declines, and evaluates
// it over children.
func baDecides(t *testing.T, expr string, children ...baChild) bool {
	t.Helper()
	test, ok := baCompile(t, expr)
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test.Evaluate(backend(), seededTypes, asElem, asValues(t), baChildren(t, children...), nil, ValueBinding{}, time.Time{})
}

// baXs is one x child per value of a, in order, each with no value of its own;
// "-" is an x carrying no a.
func baXs(as ...string) []baChild {
	out := make([]baChild, 0, len(as))
	for _, a := range as {
		if a == "-" {
			out = append(out, baChild{name: "x"})
			continue
		}
		out = append(out, baChild{name: "x", attrs: []string{"a", a}})
	}
	return out
}

// test15.xsd:17's {test} holds where the numbers after "val" ascend, compared
// numerically after fn:number, so val2 val10 holds where a string comparison
// would not, and fails where one does not exceed the one before. Each
// attribute is read off its own child: over x(val1) x(no a) x(val3) the middle
// x's `$x/@a` is empty, its fn:number NaN, and the {test} fails both where the
// middle x is bound and where it is the next x. `following-sibling::x[1]` is the
// next x however many children of other names lie between, so a y carrying a
// val0 between two x is passed over. An absent attribute is NaN and never an
// error: `not(number($x/@a) lt 3)` holds over an x with no a.
//
// Every row declines with ctaParser.variablePath returning the owner before
// any attribute step. With ctaChildBinding.next returning the next child of any
// name, "y between" fails.
func TestAssertionBoundAttributes(t *testing.T) {
	for _, tc := range []struct {
		why      string
		expr     string
		children []baChild
		holds    bool
	}{
		{"val1 val2 val3", baTest15, baXs("val1", "val2", "val3"), true},
		{"val1 val3 val2", baTest15, baXs("val1", "val3", "val2"), false},
		{"val2 val10, numeric", baTest15, baXs("val2", "val10"), true},
		{"val10 val2", baTest15, baXs("val10", "val2"), false},
		{"per element, not flattened", baTest15, baXs("val1", "-", "val3"), false},
		{"a lone x", baTest15, baXs("val1"), true},
		{"no a anywhere", baTest15, baXs("-", "-"), false},
		{"y between", baTest15, []baChild{{name: "x", attrs: []string{"a", "val1"}}, {name: "y", attrs: []string{"b", "val0"}}, {name: "x", attrs: []string{"a", "val2"}}}, true},
		{"absent a, NaN not an error", "some $x in x satisfies not(number($x/@a) lt 3)", baXs("-"), true},
		{"absent a, NaN under lt", "some $x in x satisfies number($x/@a) lt 3", baXs("-"), false},
		{"a bare attribute step", "some $x in x satisfies number($x/@a) eq 7", baXs("5", "7"), true},
		{"a bare attribute step, none", "some $x in x satisfies number($x/@a) eq 7", baXs("5", "6"), false},
		{"its existence", "every $x in x[position() lt last()] satisfies $x/following-sibling::x[1]/@a", baXs("-", "val1"), true},
		{"its existence, absent", "every $x in x[position() lt last()] satisfies $x/following-sibling::x[1]/@a", baXs("val1", "-"), false},
		{"a sibling of another name", "every $x in x satisfies $x/following-sibling::y[1]/@b/string-length(.) = 1", []baChild{{name: "x"}, {name: "y", attrs: []string{"b", "v"}}}, true},
		{"the call after the step, empty", "some $x in x satisfies 0 = $x/@a/string-length(.)", baXs("-"), false},
		{"the call after the step", "some $x in x satisfies 0 = $x/@a/string-length()", baXs("", "x"), true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := baDecides(t, tc.expr, tc.children...); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.children, got, tc.holds)
			}
		})
	}
}

// The bound child's value and its attributes are separate reads: a body
// reading both reads each off the one child. An attribute is read per element
// and never off a sequence flattened across the children: over x 3 (no a), x
// 5 a=3 no x equals its own a, so `some` fails, where the flattened (3) read
// by binding position would pair the first x with the second's a and hold.
// With ctaBoundAttributeValues reading that flattened sequence, the "per
// element" row fails.
func TestAssertionBoundAttributesBesideTheValue(t *testing.T) {
	for _, tc := range []struct {
		why      string
		expr     string
		children []baChild
		holds    bool
	}{
		{"each equal", "every $x in x satisfies $x = number($x/@a)", []baChild{{"x", "3", []string{"a", "3"}}, {"x", "5", []string{"a", "5"}}}, true},
		{"one unequal", "every $x in x satisfies $x = number($x/@a)", []baChild{{"x", "3", []string{"a", "3"}}, {"x", "5", []string{"a", "6"}}}, false},
		{"per element", "some $x in x satisfies $x = number($x/@a)", []baChild{{"x", "3", nil}, {"x", "5", []string{"a", "3"}}}, false},
	} {
		if got := baDecides(t, tc.expr, tc.children...); got != tc.holds {
			t.Errorf("Evaluate(%q) over %v = %v, want %v (%s)", tc.expr, tc.children, got, tc.holds, tc.why)
		}
	}
}

// A carried attribute breaking the obligation [ChildElements] states for a
// child built by [ChildWithAttributes] — an [Untyped] value, or a name yielded
// twice — raises wherever the tree reads it, so the {test} fails under fn:not
// too.
func TestAssertionBoundAttributesBreach(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		why   string
		attrs TypedAttributes
	}{
		{"an Untyped a", func(yield func(xsd.QName, TypedValue) bool) { yield(uq("a"), Untyped("val1")) }},
		{"a twice", func(yield func(xsd.QName, TypedValue) bool) {
			_ = yield(uq("a"), Typed(fcValue(t, str, "val1"))) && yield(uq("a"), Typed(fcValue(t, str, "val2")))
		}},
	} {
		children := func(yield func(ChildElement) bool) { yield(ChildWithAttributes(uq("x"), nil, tc.attrs)) }
		for _, expr := range []string{
			"some $x in x satisfies number($x/@a/substring-after(., 'val')) lt 3",
			"not(some $x in x satisfies number($x/@a/substring-after(., 'val')) lt 3)",
		} {
			test, ok := baCompile(t, expr)
			if !ok {
				t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
			}
			if test.Evaluate(backend(), seededTypes, asElem, asValues(t), children, nil, ValueBinding{}, time.Time{}) {
				t.Errorf("Evaluate(%q) over %s = true, want false: the read raises", expr, tc.why)
			}
		}
	}
}

// ReadsChildAttributes names the attributes of a child the body reads through
// a range variable bound to it, in written order, each once, and ReadsChild
// is false for a binding step whose body reads its children's attributes and
// not their values; a body reading neither keeps the binding step's ReadsChild,
// its children then yielded for their places. No row counts a path, so Tally
// is nil. With ctaQuantifiedChildren.readsChild back to `n.over.name == name
// || n.body.readsChild(name)`, ReadsChild(x) fails in the test15 row and the
// `@c` row; with ctaBoundChild.readsChild answering false, it fails in the
// `$x = …` row; with AssertionTest.ReadsChildAttributes keeping a name twice,
// ReadsChildAttributes(x) fails in the test15 row and the `@c` row.
func TestAssertionBoundAttributesReads(t *testing.T) {
	for _, tc := range []struct {
		expr     string
		reads    []string
		notReads []string
		ofX, ofY []string
	}{
		{baTest15, nil, []string{"x", "y"}, []string{"a"}, nil},
		{"every $x in x satisfies $x = number($x/@a)", []string{"x"}, []string{"y"}, []string{"a"}, nil},
		{"every $x in x satisfies number($x/@c) lt number($x/@a) and number($x/@c) gt 0", nil, []string{"x"}, []string{"c", "a"}, nil},
		{"every $x in x satisfies number($x/following-sibling::y[1]/@b) lt 3", []string{"x"}, []string{"y"}, nil, []string{"b"}},
		{"every $x in x satisfies $x/following-sibling::z[1] = 'v'", []string{"x", "z"}, nil, nil, nil},
	} {
		test, ok := baCompile(t, tc.expr)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
		}
		for _, name := range tc.reads {
			if !test.ReadsChild(uq(name)) {
				t.Errorf("(%s).ReadsChild(%s) = false, want true", tc.expr, name)
			}
		}
		for _, name := range tc.notReads {
			if test.ReadsChild(uq(name)) {
				t.Errorf("(%s).ReadsChild(%s) = true, want false", tc.expr, name)
			}
		}
		for _, c := range []struct {
			name string
			want []string
		}{{"x", tc.ofX}, {"y", tc.ofY}} {
			var got []string
			for _, a := range test.ReadsChildAttributes(uq(c.name)) {
				got = append(got, a.Local)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("(%s).ReadsChildAttributes(%s) = %v, want %v", tc.expr, c.name, got, c.want)
			}
		}
		if test.Tally() != nil {
			t.Errorf("(%s).Tally() = non-nil, want nil", tc.expr)
		}
	}
}

// Outside the admitted form the attribute step declines: over a variable that
// is no bound node — `$value` — an attribute of ·special· type, of a type
// attrs does not answer or of a list type, a wildcard, a call after the step
// that carries no type (fn:distinct-values) or reads another node — the range
// variable, a child or attribute step, the focus — and a step after the call.
// Inside the body of a quantifier over `$value` the step over its range
// variable, an atomic value, declines too, the call after it or not. A body
// opening `$x/@` stays the [Tally]'s (ctaParser.atTalliedBody), so `$x/@a =
// 'x'` declines there as before.
func TestAssertionBoundAttributesDecline(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"every $x in x satisfies number($x/@w) lt 3", "an attribute of ·special· type"},
		{"every $x in x satisfies number($x/@q) lt 3", "an attribute attrs answers no type for"},
		{"every $x in x satisfies number($x/@*) lt 3", "a wildcard"},
		{"every $x in x satisfies number($x/following-sibling::y[1]/@a) lt 3", "an attribute of the sibling's name attrs does not answer"},
		{"every $x in x satisfies count($x/@a/distinct-values(.)) = 1", "a call carrying no type"},
		{"every $x in x satisfies 'v' = $x/@a/concat(., $x)", "the range variable inside the call"},
		{"every $x in x satisfies 1 = $x/@a/string-length(z)", "a child step inside the call"},
		{"every $x in x satisfies 1 = $x/@a/string-length(@c)", "an attribute step inside the call"},
		{"every $x in x satisfies 1 = $x/@a/position()", "the focus inside the call"},
		{"every $x in x satisfies 'v' = $x/@a/string(.)/x", "a step after the call"},
		{"every $x in x satisfies $x/@a = 'x'", "a body opening as the Tally's"},
		{"$value/@a = 'x'", "`$value` is no node"},
	} {
		if _, ok := baCompile(t, tc.expr); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
	list := asList(t, "StringList", ctaBuiltin("string"))
	for _, expr := range []string{
		"some $v in $value satisfies number($v/@a) lt 3",
		"some $v in $value satisfies number($v/@a/substring-after(., 'val')) lt 3",
	} {
		if _, ok := CompileAssertionTest(asRecord(expr), asTypesWith(list), xsd.SimpleContent{SimpleType: list}, baUses(t), baElems(t)); ok {
			t.Errorf("CompileAssertionTest(%q) over a `$value` list: compiled, want declined", expr)
		}
	}
}

// The facet façade has no range variable bound to a node, and a Type
// Alternative no variable at all: each declines the step, the call after it or
// not.
func TestAssertionBoundAttributesOutsideTheAssertion(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, test := range []string{
		"number($value/@a) lt 3",
		"number($value/@a/substring-after(., 'val')) lt 3",
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, ""), fcValue(t, str, "val1"))
		if got != value.AssertionDeclined {
			t.Errorf("FacetAssertions.Evaluate(%q) = %d, want declined", test, got)
		}
	}
	for _, expr := range []string{baTest15, "$x/@a = 'val1'", "$x/@a/substring-after(., 'val') = '1'"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
}
