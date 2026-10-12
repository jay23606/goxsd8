package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive fn:number (xpath-functions.md §14.4, ctaNumber)
// and fn:substring-after (§7.5.5, ctaSubstringAfter), the two functions
// ibmData/mixed/assertions/test15.xsd:17's {test} calls.

// fn:number casts its one item to xs:double (§17.1.3.2): over a StringLiteral,
// an xs:string attribute whose whiteSpace the cast collapses, an
// xs:untypedAtomic attribute, `.` with no argument and an fn:substring-after
// result, numerically, so "10" is above "9". A string no xs:double lexical
// spells, an absent attribute and `()` are NaN, under which `lt`, `ge` and `eq`
// are all false and fn:not over one true: NaN is a value, never an error and
// never empty (§6.3.2). With the "number" arm of ctaParser.libraryCall
// removed, the first row declines and the test fails there.
func TestAssertionNumber(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"number('1') = 1", nil, true},
		{"number('1') eq 1e0", nil, true},
		{"number('1') = 2", nil, false},
		{"number(@s) eq 7", []asTyped{{uq("s"), "string", " 7 "}}, true},
		{"number(@s) lt number(@k)", []asTyped{{uq("s"), "string", "9"}, {uq("k"), "token", "10"}}, true},
		{"number(@w) eq 5", []asTyped{{uq("w"), "anySimpleType", "5"}}, true},
		{"number('x') lt 3", nil, false},
		{"number('x') ge 3", nil, false},
		{"number('x') eq number('x')", nil, false},
		{"not(number('x') lt 3)", nil, true},
		{"number(()) lt 3", nil, false},
		{"number(()) ge 3", nil, false},
		{"not(number(()) ge 3)", nil, true},
		{"number(@s) lt 3", nil, false},
		{"not(number(@s) ge 3)", nil, true},
		{"number(@w) lt 3", []asTyped{{uq("w"), "anySimpleType", "abc"}}, false},
		{"not(number(@w) lt 3)", []asTyped{{uq("w"), "anySimpleType", "abc"}}, true},
		{"number(substring-after(@s, 'val')) lt number(substring-after(@k, 'val'))", []asTyped{{uq("s"), "string", "val2"}, {uq("k"), "token", "val10"}}, true},
		{"number(substring-after(@s, 'val')) lt number(substring-after(@k, 'val'))", []asTyped{{uq("s"), "string", "val3"}, {uq("k"), "token", "val2"}}, false},
	} {
		if got := afEval(t, tc.expr, tc.attrs); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"12", true},
		{"twelve", false},
	} {
		test, ok := afCompile(t, "number() eq 12", xsd.EmptyContent{})
		if !ok {
			t.Fatalf("CompileAssertionTest(number() eq 12): declined, want compiled")
		}
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, BindValue(tc.text, nil), time.Time{}); got != tc.want {
			t.Errorf("Evaluate(number() eq 12) over `.` %q = %v, want %v", tc.text, got, tc.want)
		}
	}
}

// fn:number over two or more items is err:XPTY0004 (xpath20.md §3.1.5), which
// fails the assertion under fn:not too, where NaN would make it hold: over a
// `$value` list of two xs:string items both rows fail, and over one item they
// split. With ctaNumberItem reading two items as NaN, the fn:not row over "1 2"
// holds.
func TestAssertionNumberOfTwoItems(t *testing.T) {
	list := asList(t, "StringList", ctaBuiltin("string"))
	types := asTypesWith(list)
	for _, tc := range []vcCase{
		{"number($value) lt 3", list, "1 2", false},
		{"not(number($value) lt 3)", list, "1 2", false},
		{"number($value) lt 3", list, "1", true},
		{"not(number($value) lt 3)", list, "4", true},
		{"some $x in $value satisfies number($x) lt 3", list, "x 2", true},
		{"every $x in $value satisfies number($x) lt 3", list, "x 2", false},
	} {
		if got := vcAssertion(t, types, tc); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// fn:substring-after (§7.5.5) is the part of its first argument after the
// first occurrence of its second, under the codepoint collation: "12" of
// "val12", "" where the needle is missing or the first argument is empty or
// absent, the first argument whole under a zero-length needle, and the text
// after the FIRST occurrence where there are two. With the "substring-after"
// arm of ctaParser.libraryCall removed, the first row declines and the test
// fails there.
func TestAssertionSubstringAfter(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"substring-after('val12', 'val') = '12'", nil, true},
		{"substring-after('val12', 'val') = 'val12'", nil, false},
		{"substring-after('abc', 'x') = ''", nil, true},
		{"substring-after('', 'x') = ''", nil, true},
		{"substring-after((), 'x') = ''", nil, true},
		{"substring-after('abc', '') = 'abc'", nil, true},
		{"substring-after('abc', ()) = 'abc'", nil, true},
		{"substring-after('a-b-c', '-') = 'b-c'", nil, true},
		{"substring-after('tattoo', 'tattoo') = ''", nil, true},
		{"substring-after(@s, 'val') = '1'", []asTyped{{uq("s"), "string", "val1"}}, true},
		{"substring-after(@s, 'val') = ''", nil, true},
		{"substring-after(@w, 'v') = '9'", []asTyped{{uq("w"), "anySimpleType", "v9"}}, true},
		{"substring-after(@i, '1') = '2'", []asTyped{{uq("i"), "integer", "12"}}, false},
		{"not(substring-after(@i, '1') = '2')", []asTyped{{uq("i"), "integer", "12"}}, false},
	} {
		if got := afEval(t, tc.expr, tc.attrs); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}

// Outside the admitted forms each call declines: fn:substring-after with a
// collation argument or the wrong arity, fn:number with two arguments, and
// fn:number over a typed operand whose cast to xs:double castsFrom declines —
// an xs:integer attribute, an xs:boolean, a date — under numberCall's
// GAP(xpath). A Type Alternative's {test} calls neither function
// (ctaTypeAlternativeFacade.callsLibrary).
func TestAssertionNumberAndSubstringAfterDecline(t *testing.T) {
	for _, expr := range []string{
		"substring-after('a', 'b', 'http://www.w3.org/2005/xpath-functions/collation/codepoint') = ''",
		"substring-after('a') = ''",
		"number('1', '2') = 1",
		"number(@i) = 1",
		"number(true()) = 1",
		"number(current-date()) = 1",
	} {
		if _, ok := afCompile(t, expr, xsd.EmptyContent{}); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"number(@a) = 1", "substring-after(@a, 'v') = '1'"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
}

// The facet façade calls both functions over `$value` (cvc-assertions-valid),
// so `number($value) lt 3` decides an xs:string facet both ways and NaN fails
// it, and `number()`, whose implicit argument is the absent context item,
// raises err:XPDY0002 and fails it under fn:not too.
func TestFacetNumberAndSubstringAfter(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		test, lexical string
		want          value.AssertionOutcome
	}{
		{"number($value) lt 3", "2", value.AssertionHolds},
		{"number($value) lt 3", "4", value.AssertionFails},
		{"number($value) lt 3", "x", value.AssertionFails},
		{"substring-after($value, 'val') = '1'", "val1", value.AssertionHolds},
		{"substring-after($value, 'val') = '1'", "val2", value.AssertionFails},
		{"number(substring-after($value, 'val')) lt 3", "val2", value.AssertionHolds},
		{"number() lt 3", "2", value.AssertionFails},
		{"not(number() lt 3)", "2", value.AssertionFails},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(tc.test, ""), fcValue(t, str, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %d, want %d", tc.test, tc.lexical, got, tc.want)
		}
	}
}
