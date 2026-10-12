package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive xpath20.md §3.9's QuantifiedExpr over the items of
// a typed `$value` (ctaQuantifiedValue), the form of
// ibmData/mixed/assertions/list_union/listunion6.xsd:8, `every $x in
// data($value) satisfies ($x mod 2 = 0)`, its range variable bound to each item
// at evaluation (ctaRangeItem).

// TestAssertionQuantifiedOverValue decides `every` and `some` over a list of
// xs:integer and over an atomic xs:integer: `every` true where each item
// satisfies the body, `some` where one does, and over the empty list `every`
// true and `some` false (§3.9). `data($value)` and `$value` bare are one
// binding (xpath-functions.md §2.4). Every row declines at CompileAssertionTest,
// and so fails, with quantifiedExpr's valueQuantified dispatch removed.
func TestAssertionQuantifiedOverValue(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	integer := asBuiltin(t, "integer")
	const even = "every $x in data($value) satisfies ($x mod 2 = 0)"
	const above5 = "some $x in data($value) satisfies $x gt 5"
	for _, tc := range []vcCase{
		{even, list, "2 4 6", true},
		{even, list, "2 3", false},
		{even, list, "", true},
		{even, list, "2 4 6 8 10", true},
		{even, list, "2 4 6 7 10", false},
		{above5, list, "1 6", true},
		{above5, list, "1 5", false},
		{above5, list, "", false},
		{"every $x in $value satisfies ($x mod 2 = 0)", list, "2 4 6", true},
		{"every $x in $value satisfies ($x mod 2 = 0)", list, "2 3", false},
		{"some $x in $value satisfies $x gt 5", list, "1 6", true},
		{"every $x in data($value) satisfies $x gt 0", integer, "5", true},
		{"every $x in data($value) satisfies $x gt 0", integer, "-1", false},
		{"some $x in data($value) satisfies $x gt 0", integer, "5", true},
		{"some $x in data($value) satisfies $x gt 0", integer, "-1", false},
		// The body is one ExprSingle (§3.9's grammar), the `and` inside it.
		{"every $x in data($value) satisfies $x gt 0 and $x lt 9", list, "2 4", true},
		{"every $x in data($value) satisfies $x gt 0 and $x lt 9", list, "2 10", false},
		// The quantifier as a parenthesized operand, beside `count($value)`, as
		// listunion6's two assertions would stand in one {test}.
		{"count($value) le 5 and (" + even + ")", list, "2 4 6 8 10", true},
		{"count($value) le 5 and (" + even + ")", list, "2 4 6 8 10 12", false},
		{"not(" + even + ")", list, "2 3", true},
		// `$value` stays in scope inside the body.
		{"every $x in data($value) satisfies $x le count($value)", list, "1 2 3", true},
		{"every $x in data($value) satisfies $x le count($value)", list, "1 2 4", false},
		// A prefixed range variable is named by its ·expanded name·.
		{"every $a:x in data($value) satisfies $a:x gt 0", list, "1 2", true},
		{"every $a:x in data($value) satisfies $a:x gt 0", list, "1 0", false},
	} {
		if got := vcAssertion(t, asTypesWith(list), tc); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// The zero ValueBinding is the nil `$value` cvc-assertion clause 2.3.2 binds
// for an invalid or ·nilled· E: the empty sequence, so `every` is true and
// `some` false, both evaluated and neither declined.
func TestAssertionQuantifiedOverNilValue(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	types := asTypesWith(list)
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"every $x in data($value) satisfies $x gt 0", true},
		{"some $x in data($value) satisfies $x gt 0", false},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), types, xsd.SimpleContent{SimpleType: list}, asUses(t, nil), asNoElems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
		}
		if got := test.Evaluate(backend(), types, asElem, asValues(t), asNoChildren, nil, ValueBinding{}, time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over a nil $value = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// A decisive binding decides the whole quantifier whether or not another
// raised, and otherwise an error survives (§3.9 over §3.6's tables): `4 mod
// $x` raises err:FOAR0001 for the 0 item. `some` over `0 2` is true, and over
// `0 3` raises, so fn:not over it is false where false would make it true;
// `every` over `0 3` is false, so fn:not over it is true, and over `0 2`
// raises, so fn:not over it is false. With the first raised binding deciding
// the quantifier, the rows `some` over "0 2" and fn:not over `every` over "0 3"
// fail; with a raised binding read as false, the rows fn:not over `some` over
// "0 3" and `every` over "0 2" fail.
func TestAssertionQuantifiedOverValueErrors(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	const some = "some $x in data($value) satisfies 4 mod $x = 0"
	const every = "every $x in data($value) satisfies 4 mod $x = 0"
	for _, tc := range []vcCase{
		{some, list, "0 2", true},
		{some, list, "2 0", true},
		{some, list, "0 3", false},
		{"not(" + some + ")", list, "0 3", false},
		{every, list, "0 3", false},
		{every, list, "3 0", false},
		{"not(" + every + ")", list, "0 3", true},
		{every, list, "0 2", false},
		{"not(" + every + ")", list, "0 2", false},
	} {
		if got := vcAssertion(t, asTypesWith(list), tc); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// The range variable carries `$value`'s item type into the body's operators
// (ctaCarriedType) and is read as an atomic value, never a node: `$x = '2'`
// over an xs:integer is xpath20.md §B.2's err:XPTY0004 — read untyped, it
// would cast to xs:string and hold — and `$x`'s ·effective boolean value· is
// fn:boolean's over the integer (§2.4.3 rule 5), false for 0. With the
// ctaRangeItem arm of ctaCarriedType removed the `= '2'` row holds and the
// "0 1" and the `every` rows are false, among other failures; with
// ctaItemOf's removed the `2 4 6` rows of TestAssertionQuantifiedOverValue
// fail, among others.
func TestAssertionQuantifiedRangeItem(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	for _, tc := range []vcCase{
		{"some $x in data($value) satisfies $x = '2'", list, "2", false},
		{"some $x in data($value) satisfies $x", list, "0 1", true},
		{"some $x in data($value) satisfies $x", list, "0 0", false},
		{"every $x in data($value) satisfies $x", list, "1 2", true},
		{"some $x in data($value) satisfies not($x)", list, "1 0", true},
		{"some $x in data($value) satisfies $x = 2", list, "1 2", true},
	} {
		if got := vcAssertion(t, asTypesWith(list), tc); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// The quantifier keys nothing of its own and reads no child or `.`: its Tally,
// ReadsChild and ReadsContextItem are its body's. A body over `count(c)` keys
// c's counter and decides off it; one comparing a child's value reads that
// child; one reading `.` reads E's string value. The Tally row fails with
// ctaQuantifiedValue.counted appending nothing, and the ReadsChild(c) row with
// ctaQuantifiedValue.readsChild answering false.
func TestAssertionQuantifiedOverValueReads(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	types := asTypesWith(list)
	content := xsd.SimpleContent{SimpleType: list}
	elems := func([]xsd.QName) (xsd.TypeDefinition, bool) { return asBuiltin(t, "integer"), true }
	compile := func(expr string) AssertionTest {
		t.Helper()
		test, ok := CompileAssertionTest(asRecord(expr), types, content, asUses(t, nil), elems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		return test
	}
	bare := compile("every $x in data($value) satisfies ($x mod 2 = 0)")
	if bare.Tally() != nil || bare.ReadsChild(uq("c")) || bare.ReadsContextItem() {
		t.Errorf("the bare quantifier keys a Tally, reads a child or reads `.`, want none")
	}
	counting := compile("every $x in data($value) satisfies count(c) ge $x")
	c := counting.Tally()
	if c == nil || len(c.counters) != 1 {
		t.Fatalf("(count(c) in the body).Tally() = %v, want 1 counter", c)
	}
	if counting.ReadsChild(uq("c")) {
		t.Error("(count(c) in the body).ReadsChild(c) = true, want false")
	}
	for _, tc := range []struct {
		lexical string
		want    bool
	}{{"1 2", true}, {"1 3", false}} {
		counts := acTally(counting, acPath("c"), acPath("c"))
		if got := counting.Evaluate(backend(), types, asElem, asValues(t), asNoChildren, counts, asBindIn(t, types, list, tc.lexical), time.Time{}); got != tc.want {
			t.Errorf("Evaluate(count(c) ge $x) over two c and %q = %v, want %v", tc.lexical, got, tc.want)
		}
	}
	if !compile("some $x in data($value) satisfies c = $x").ReadsChild(uq("c")) {
		t.Error("(c = $x in the body).ReadsChild(c) = false, want true")
	}
	if !compile("some $x in data($value) satisfies . = $x").ReadsContextItem() {
		t.Error("(. = $x in the body).ReadsContextItem() = false, want true")
	}
}

// asBindIn maps lexical against st under types and binds it to $value.
func asBindIn(t *testing.T, types xsd.TypeResolver, st *xsd.SimpleType, lexical string) ValueBinding {
	t.Helper()
	v, err := value.ValidateLexical(backend(), types, st, lexical, nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping %q against %s: %v", lexical, st.Name().Local, err)
	}
	return BindValue(lexical, Typed(v))
}

// Every quantifier over `$value` outside the admitted shapes declines: the
// range variable outside its body (err:XPST0008, withheld), a nested
// quantifier in either direction, a binding over a `$value` that is
// statically empty or ·special· (no static type for the range variable), a
// path or `instance of` over the range variable, and fn:data in any other
// position. The two "outside its body" rows compile with p.facade left as
// the range scope after the body; the "not naming $x" row with
// ctaRangeFacade.rangeScope admitting a scope over the assertion façade, which
// the rows naming `$x` still decline under, the inner scope dropping `$x`; the
// "over a child step in the body" row with ctaRangeFacade.quantified
// admitting; the first statically empty row panics with valueBinding admitting
// any `$value`; the `instance of` row compiles with ctaRangeItem added to
// ctaTypes.instanceItem.
func TestAssertionQuantifiedOverValueDeclines(t *testing.T) {
	list := asList(t, "IntegerList", ctaBuiltin("integer"))
	types := asTypesWith(list)
	simpleList := xsd.SimpleContent{SimpleType: list}
	anySimple := xsd.SimpleContent{SimpleType: asBuiltin(t, "anySimpleType")}
	for _, tc := range []struct {
		expr    string
		content xsd.ContentType
		why     string
	}{
		{"(every $x in data($value) satisfies $x gt 0) and $x gt 0", simpleList, "the variable outside its body"},
		{"(some $x in data($value) satisfies $x gt 0) and $x", simpleList, "the variable outside its body, bare"},
		{"every $x in data($value) satisfies some $y in data($value) satisfies $y gt $x", simpleList, "a nested quantifier over $value"},
		{"every $x in data($value) satisfies (some $y in data($value) satisfies $y gt $x)", simpleList, "a parenthesized nested quantifier over $value"},
		{"every $x in data($value) satisfies (some $y in data($value) satisfies $y gt 0)", simpleList, "a nested quantifier over $value not naming $x"},
		{"every $x in data($value) satisfies (every $c in c satisfies $c/@a)", simpleList, "a quantifier over a child step in the body"},
		{"every $x in data($value) satisfies $x gt 0", asElementContent(t, false), "a statically empty $value"},
		{"every $x in data($value) satisfies $x gt 0", xsd.EmptyContent{}, "a statically empty $value, empty content"},
		{"every $c in $value satisfies $c/@a", asElementContent(t, false), "a statically empty $value, bare"},
		{"every $x in data($value) satisfies $x = 'a'", anySimple, "a ·special· $value"},
		{"every $x in data($value) satisfies $x/@a", simpleList, "a path over an atomic range variable"},
		{"every $c in $value satisfies $c/@a", simpleList, "a path over an atomic range variable, $value bare"},
		{"every $x in data($value) satisfies $x instance of xs:integer", simpleList, "instance of over the range variable"},
		{"every $x in data($value, 1) satisfies $x gt 0", simpleList, "fn:data of two arguments"},
		{"every $x in data(.) satisfies $x gt 0", simpleList, "fn:data over another argument"},
		{"every $x in data($value) satisfies data($x) = 1", simpleList, "fn:data in the body"},
		{"every $x in $value, $y in $value satisfies $x gt $y", simpleList, "two in-clauses"},
		{"every $x in ($value) satisfies $x gt 0", simpleList, "a parenthesized binding"},
		{"every $x in $y satisfies $x gt 0", simpleList, "another variable as binding"},
		{"every $x in data($value) satisfies $y gt 0", simpleList, "another variable in the body"},
		{"every $x in data($x) satisfies $x gt 0", simpleList, "the variable in its own binding"},
		{"count($value[every $x in data($value) satisfies $x gt 0]) = 1", simpleList, "a quantifier in a predicate"},
		{"count(c[every $x in data($value) satisfies $x gt 0]) = 1", simpleList, "a quantifier in a value predicate"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), types, tc.content, asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}
