package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive fn:data over one child step whose axis is spelled
// out, `child::d` (xpath20.md §3.2.4: the abbreviated step's axis is child),
// or filtered by an IntegerLiteral predicate, `d[k]` (§3.2.2), as
// saxonData/Assert/assert018.xsd's `data(child::d[1]) instance of xs:date`
// writes it (ctaParser.dataChildStep, ctaChildAt). d and e are xs:date, m is
// of mixed content.

// acaElems is the [ElementTypes] the rows below compile against.
func acaElems(t *testing.T) ElementTypes {
	t.Helper()
	date := asBuiltin(t, "date")
	return asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("d"): date, uq("e"): date,
		uq("m"): asComplex(t, "Mixed", asElementContent(t, true)),
	})
}

// acaDate is the xs:date child named local holding lexical.
func acaDate(local, lexical string) asChild { return asChild{uq(local), "date", lexical, false} }

// `child::d` reads what `d` reads, and `d[k]` the k-th d child in document
// order, counted among the d children alone (an e before them moves no
// position), or nothing past the last, at `[0]`, or at a position no int
// holds: `instance of xs:date` asks exactly one item and `xs:date?` zero or
// one (§2.5.4.1). A ·nilled· d is a node with no typed value, so it holds a
// position and contributes no item. Every row declines with
// dataInstanceOf's dataChildStepLength branch removed. With childAt compiling
// the step unfiltered, nine rows fail: `d[1]` and `child::d[1]` over two,
// `child::d[2]` over one and over two, `d[2]` under `?` over two, `d[2]` after
// an e, the `[0]` and out-of-range rows under the bare type, and `d[1]` under
// the bare type over a ·nilled· first d. With ctaChildAtItem counting
// positions over every child, whatever its name, the two rows after an e fail.
// With childAt declining an IntegerLiteral no int holds, that row declines.
func TestAssertionDataChildAt(t *testing.T) {
	one := []asChild{acaDate("d", "2008-01-01")}
	two := []asChild{acaDate("d", "2008-01-01"), acaDate("d", "2008-01-02")}
	after := []asChild{acaDate("e", "2008-01-03"), acaDate("d", "2008-01-01")}
	nilledFirst := []asChild{{name: uq("d"), nilled: true}, acaDate("d", "2008-01-02")}
	for _, tc := range []struct {
		expr     string
		children []asChild
		want     bool
	}{
		{"data(child::d) instance of xs:date", one, true},
		{"data(child::d) instance of xs:date", two, false},
		{"data(child::d) instance of xs:date+", two, true},
		{"data(d[1]) instance of xs:date", one, true},
		{"data(d[1]) instance of xs:date", two, true},
		{"data(child::d[1]) instance of xs:date", one, true},
		{"data(child::d[1]) instance of xs:date", two, true},
		{"data(child::d[2]) instance of xs:date", one, false},
		{"data(child::d[2]) instance of xs:date", two, true},
		{"data(d[2]) instance of xs:date?", one, true},
		{"data(d[2]) instance of xs:date?", two, true},
		{"data(d[3]) instance of xs:date", two, false},
		{"data(d[1]) instance of xs:date", after, true},
		{"data(d[2]) instance of xs:date", after, false},
		{"data(d[0]) instance of xs:date", one, false},
		{"data(d[0]) instance of xs:date?", one, true},
		{"data(d[99999999999999999999]) instance of xs:date", one, false},
		{"data(d[1]) instance of xs:date", nilledFirst, false},
		{"data(d[1]) instance of xs:date?", nilledFirst, true},
		{"data(d[2]) instance of xs:date", nilledFirst, true},
		{"data(d[1]) instance of xs:date", nil, false},
		{"not(data(child::d[1]) instance of xs:date)", nil, true},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, asElementContent(t, false), asUses(t, nil), acaElems(t))
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			continue
		}
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, tc.children...), nil, ValueBinding{}, time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.children, got, tc.want)
		}
	}
}

// A quantifier binds variables alone and leaves the focus E (xpath20.md §3.9:
// "the test expression is evaluated for each tuple of variable bindings"), so
// `d[k]` in its body is E's k-th d child whatever the binding: over a typed
// child step (ctaChildRangeFacade) and over `$value` (ctaRangeFacade), each of
// which reaches ctaFacade.child through the assertion façade it embeds, as
// `data(d)` does. Each row declines with dataInstanceOf's dataChildStepLength
// branch removed, and each fails with ctaChildAtItem reading every d child.
func TestAssertionDataChildAtInQuantifier(t *testing.T) {
	one := []asChild{acaDate("d", "2008-01-01")}
	two := []asChild{acaDate("d", "2008-01-01"), acaDate("d", "2008-01-02")}
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		expr     string
		children []asChild
		want     bool
	}{
		{"some $v in d satisfies data(child::d[2]) instance of xs:date", one, false},
		{"some $v in d satisfies data(child::d[2]) instance of xs:date", two, true},
		{"every $x in data($value) satisfies data(d[2]) instance of xs:date", one, false},
		{"every $x in data($value) satisfies data(d[2]) instance of xs:date", two, true},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, xsd.SimpleContent{SimpleType: date}, asUses(t, nil), acaElems(t))
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			continue
		}
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, tc.children...), nil, asBind(t, date, "2008-01-01"), time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.children, got, tc.want)
		}
	}
}

// AssertionTest.ReadsChild reports the step `d[1]` filters, so the caller
// yields every d child and the first is found among them; it reports no other
// name. The d row fails with ctaChildAt.readsChild answering false.
func TestAssertionDataChildAtReadsChild(t *testing.T) {
	test, ok := CompileAssertionTest(asRecord("data(child::d[1]) instance of xs:date"), seededTypes, asElementContent(t, false), asUses(t, nil), acaElems(t))
	if !ok {
		t.Fatal("CompileAssertionTest: declined, want compiled")
	}
	if !test.ReadsChild(uq("d")) {
		t.Error("ReadsChild(d) = false, want true")
	}
	if test.ReadsChild(uq("e")) {
		t.Error("ReadsChild(e) = true, want false")
	}
}

// An assertions facet has no context item, so the step `d` raises
// err:XPDY0002 before its predicate is read, under `child::` too, and the
// facet Fails, under fn:not and `?` included (ctaFacetFacade.child). Every row
// is Declined with dataInstanceOf's dataChildStepLength branch removed, and the
// two `[1]` rows with ctaParser.childAt declining a ctaNoContextItem step.
func TestFacetAssertionsDataChildAt(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, test := range []string{
		"data(child::d) instance of xs:date?",
		"data(d[1]) instance of xs:date?",
		"not(data(child::d[1]) instance of xs:date)",
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, "", "xs", xsd.XMLSchemaNS), fcValue(t, str, "x"))
		if got != value.AssertionFails {
			t.Errorf("Evaluate(%q) = %d, want Fails (%d)", test, got, value.AssertionFails)
		}
	}
}

// Guard: what this slice does not admit still declines. A Type Alternative's
// {test} declines both constructs: §3.12.6's [16] ta-SimpleValue is AttrName
// or Literal, so neither a child step nor fn:data is in the required subset.
// In an assertion, `child::` and `[k]` are admitted only on one step as
// fn:data's argument under `instance of`: a predicate of any other shape — a
// non-numeric one (`d[@a]`), a DecimalLiteral, fn:last, fn:position — and a
// step that is not that argument whole, or is read anywhere else — a
// comparison, an ·effective boolean value·, fn:exists, fn:count, fn:sum, a
// step after or before it, a parenthesized step filtered from outside — each
// declines under #1042's GAP(xpath) markers (ctaParser.predicate, childPath,
// sumCall, dataInstanceOf). A predicate over a mixed child declines under
// childAt's own, where `data(m)` compiles; that row compiles with childAt
// returning a ctaUntypedChild step unfiltered.
func TestDataChildAtStillDeclines(t *testing.T) {
	for _, expr := range []string{"data(child::d[1]) instance of xs:date", "data(child::d) instance of xs:date", "child::d = 1", "d[1] = 1"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", "xs", xsd.XMLSchemaNS), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	if _, ok := CompileAssertionTest(asRecord("data(m) instance of xs:untypedAtomic"), seededTypes, asElementContent(t, false), asUses(t, nil), acaElems(t)); !ok {
		t.Error("CompileAssertionTest(data(m) ...): declined, want compiled")
	}
	for _, tc := range []struct{ expr, why string }{
		{"data(d[@a]) instance of xs:date", "a non-numeric predicate"},
		{"d[@a] = xs:date('2008-01-01')", "a non-numeric predicate read for its value"},
		{"data(d[1.0]) instance of xs:date", "a DecimalLiteral predicate"},
		{"data(d[1e0]) instance of xs:date", "a DoubleLiteral predicate"},
		{"data(d[last()]) instance of xs:date", "fn:last in the predicate"},
		{"data(d[position() = 1]) instance of xs:date", "fn:position in the predicate"},
		{"data(d[1][1]) instance of xs:date", "a second predicate"},
		{"data((d)[1]) instance of xs:date", "a filter expression"},
		{"data(m[1]) instance of xs:untypedAtomic", "a predicate over a mixed child"},
		{"data(child::d/e) instance of xs:date", "a step after the spelled-out axis"},
		{"data(d[1]/e) instance of xs:date", "a step after the predicate"},
		{"data(d/child::e) instance of xs:date", "the spelled-out axis on a later step"},
		{"data(self::d) instance of xs:date", "the self axis"},
		{"data(descendant::d) instance of xs:date", "the descendant axis"},
		{"data(child::*) instance of xs:date", "a wildcard"},
		{"data(d[1]) = xs:date('2008-01-01')", "fn:data outside instance of"},
		{"d[1] = xs:date('2008-01-01')", "a numeric predicate in a comparison"},
		{"child::d = xs:date('2008-01-01')", "the spelled-out axis in a comparison"},
		{"d[1]", "a numeric predicate as an effective boolean value"},
		{"child::d", "the spelled-out axis as an effective boolean value"},
		{"exists(d[1])", "a numeric predicate under fn:exists"},
		{"exists(child::d)", "the spelled-out axis under fn:exists"},
		{"count(d[1]) = 1", "a numeric predicate in an fn:count argument"},
		{"count(child::d) = 1", "the spelled-out axis in an fn:count argument"},
		{"sum(child::d/@p) = 0", "the spelled-out axis in an fn:sum argument"},
		{"sum(d[1]/@p) = 0", "a numeric predicate in an fn:sum argument"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, asElementContent(t, false), asUses(t, map[string]string{"a": "string", "p": "int"}), acaElems(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}
