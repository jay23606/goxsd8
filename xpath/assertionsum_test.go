package xpath

import (
	"slices"
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive fn:sum with one argument over a path to an
// attribute below E (xpath-functions.md §15.4.5, ctaSum): `sum(a/b/@p)` reads
// the typed value of each p of each b grandchild of E under its a child, in
// document order (xpath20.md §3.2, §2.4.2), which the caller reports to the
// [Tally] (Tally.Attribute) and Evaluate sums.

// smPath is the chain below E of the elements whose attributes the rows sum:
// each b under E's a child.
var smPath = []xsd.QName{uq("a"), uq("b")}

// smUses is the [AttributeTypes] of the rows: at smPath, p is the builtin typ
// and q an xs:string; E itself, and every other path, types nothing.
func smUses(t *testing.T, typ string) AttributeTypes {
	t.Helper()
	p, q := asBuiltin(t, typ), asBuiltin(t, "string")
	return func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		if !slices.Equal(path, smPath) {
			return nil, false
		}
		switch name {
		case uq("p"):
			return p, true
		case uq("q"):
			return q, true
		}
		return nil, false
	}
}

// smElems is E's child element types: c and t xs:decimal.
func smElems(t *testing.T) ElementTypes {
	t.Helper()
	d := asBuiltin(t, "decimal")
	return asElems(map[xsd.QName]xsd.TypeDefinition{uq("c"): d, uq("t"): d})
}

// smCompile compiles expr for an E with element-only content whose p below
// smPath is typ, or fails the test.
func smCompile(t *testing.T, expr, typ string) AssertionTest {
	t.Helper()
	test, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), smUses(t, typ), smElems(t))
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test
}

// smValue is lexical as the typed value of an attribute of the builtin typ, on
// [Tally.Attribute]'s terms: [Untyped] where typ is ·special·, and [Typed] of
// the value the backend maps otherwise.
func smValue(t *testing.T, typ, lexical string) TypedValue {
	t.Helper()
	st := asBuiltin(t, typ)
	if st.IsSpecial() {
		return Untyped(lexical)
	}
	v, err := value.ValidateLexical(backend(), seededTypes, st, lexical, nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping %q as xs:%s: %v", lexical, typ, err)
	}
	return Typed(v)
}

// smB is one b grandchild of E: with @p of lexical where has is true, and
// none otherwise.
type smB struct {
	lexical string
	has     bool
}

// smP is a b carrying @p = lexical.
func smP(lexical string) smB { return smB{lexical: lexical, has: true} }

// smTally is test's Tally filled as [Tally] obliges: E, its one a child, and
// each of bs under it in the order written, each @p's value reported, as typ,
// right after its b.
func smTally(t *testing.T, test AssertionTest, typ string, bs ...smB) *Tally {
	t.Helper()
	c := test.Tally()
	c.Element(nil, nil)
	c.Element(smPath[:1], nil)
	for _, b := range bs {
		if !b.has {
			c.Element(smPath, nil)
			continue
		}
		c.Element(smPath, []xsd.QName{uq("p")})
		c.Attribute(uq("p"), smValue(t, typ, b.lexical))
	}
	return c
}

// smEval evaluates test over counts, with E's children children.
func smEval(t *testing.T, test AssertionTest, counts *Tally, children ...asChild) bool {
	t.Helper()
	return test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, children...), counts, ValueBinding{}, time.Time{})
}

// smDec is E's child named local, an xs:decimal of lexical.
func smDec(local, lexical string) asChild {
	return asChild{name: uq(local), typ: "decimal", lexical: lexical}
}

// fn:sum over xs:decimal attributes is exact (F&O §6.2): 0.1 + 0.2 = 0.3 in
// xs:decimal, where an xs:double sum is 0.30000000000000004. Each row is
// decided both ways, against a typed child c and after `* 0.1` and `+ t`, the
// shapes assert_019's po.xsd writes; its own figures, 88.10 and 20 against a
// tax of 10.81 and a bill of 118.91, follow, and a bare call's ·effective
// boolean value· closes the table. With the items added in float64 and the
// sum rendered as a decimal lexical, the four 0.1 + 0.2 rows that hold fail,
// and no other: po.xml's own figures cannot tell the two sums apart. The bare
// `sum(a/b/@p)` over 5 fails with ctaSum's arm of ctaEffectiveBoolean.eval
// removed.
func TestAssertionSumDecimalExact(t *testing.T) {
	for _, tc := range []struct {
		expr     string
		bs       []smB
		children []asChild
		holds    bool
	}{
		{"c = sum(a/b/@p)", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.3")}, true},
		{"c = sum(a/b/@p)", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.4")}, false},
		{"c eq sum(a/b/@p)", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.3")}, true},
		{"c = sum(a/b/@p) * 0.1", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.03")}, true},
		{"c = sum(a/b/@p) * 0.1", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.3")}, false},
		{"c = sum(a/b/@p) + t", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.9"), smDec("t", "0.6")}, true},
		{"c = sum(a/b/@p) + t", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "1"), smDec("t", "0.6")}, false},
		{"t = sum(a/b/@p) * 0.1", []smB{smP("88.10"), smP("20")}, []asChild{smDec("t", "10.81")}, true},
		{"c = sum(a/b/@p) + t", []smB{smP("88.10"), smP("20")}, []asChild{smDec("c", "118.91"), smDec("t", "10.81")}, true},
		{"c = sum(a/b/@p) + t", []smB{smP("88.10"), smP("20")}, []asChild{smDec("c", "118.92"), smDec("t", "10.81")}, false},
		{"sum(a/b/@p) = 5", []smB{smP("5")}, nil, true},
		{"sum(a/b/@p) = 5", []smB{smP("2"), {}, smP("3")}, nil, true},
		{"sum(a/b/@p)", []smB{smP("5")}, nil, true},
		{"sum(a/b/@p)", []smB{smP("0")}, nil, false},
		{"c = distinct-values(sum(a/b/@p))", []smB{smP("0.1"), smP("0.2")}, []asChild{smDec("c", "0.3")}, true},
	} {
		test := smCompile(t, tc.expr, "decimal")
		if got := smEval(t, test, smTally(t, test, "decimal", tc.bs...), tc.children...); got != tc.holds {
			t.Errorf("%q over %v: Evaluate = %v, want %v", tc.expr, tc.bs, got, tc.holds)
		}
	}
}

// Over an empty argument fn:sum returns the xs:integer 0 (F&O §15.4.5): with
// no b at all, and with b children none of which carries a p. Its DYNAMIC type
// is read: `instance of xs:integer` holds over each empty shape and is false
// over one xs:decimal item and over two, whose sums are xs:decimal values. The
// two empty `instance of xs:integer` rows fail, and no other row here, both
// with ctaSum.total typing the empty sum by its static type and with
// ctaSumInstanceItem reading the static type in place of the dynamic one.
func TestAssertionSumOfNothingIsTheIntegerZero(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		bs    []smB
		holds bool
	}{
		{"sum(a/b/@p) instance of xs:integer", nil, true},
		{"sum(a/b/@p) instance of xs:integer", []smB{{}, {}}, true},
		{"sum(a/b/@p) = 0", nil, true},
		{"sum(a/b/@p) = 0", []smB{{}}, true},
		{"sum(a/b/@p) instance of xs:integer", []smB{smP("5")}, false},
		{"sum(a/b/@p) instance of xs:integer", []smB{smP("2"), smP("3")}, false},
		{"sum(a/b/@p) instance of xs:decimal", []smB{smP("2"), smP("3")}, true},
		{"sum(a/b/@p) instance of xs:decimal", nil, true},
		{"sum(a/b/@p) instance of xs:untypedAtomic", nil, false},
	} {
		test := smCompile(t, tc.expr, "decimal")
		if got := smEval(t, test, smTally(t, test, "decimal", tc.bs...)); got != tc.holds {
			t.Errorf("%q over %v: Evaluate = %v, want %v", tc.expr, tc.bs, got, tc.holds)
		}
	}
}

// An xs:string item does not support addition, so fn:sum over one raises
// err:FORG0006 (F&O §15.4.5): the assertion is FALSE, and false under fn:not
// too, never declined, even where the lexical is numeric and a cast to
// xs:double would sum it. Over no item it is the xs:integer 0 all the same.
// `sum(a/b/@p) = 5` and `= 11` hold with ctaTypes.sum reading a non-numeric
// item as xs:double, and `= 5` with ctaSum.total handing one item back before
// it asks whether the type supports addition.
func TestAssertionSumOfStringsRaises(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		bs    []smB
		holds bool
	}{
		{"sum(a/b/@p) = 5", []smB{smP("5")}, false},
		{"not(sum(a/b/@p) = 5)", []smB{smP("5")}, false},
		{"sum(a/b/@p) = 11", []smB{smP("5"), smP("6")}, false},
		{"not(sum(a/b/@p) = 11)", []smB{smP("5"), smP("6")}, false},
		{"sum(a/b/@p) = 0", nil, true},
	} {
		test := smCompile(t, tc.expr, "string")
		if got := smEval(t, test, smTally(t, test, "string", tc.bs...)); got != tc.holds {
			t.Errorf("%q over %v: Evaluate = %v, want %v", tc.expr, tc.bs, got, tc.holds)
		}
	}
}

// An xs:anySimpleType attribute's typed value is xs:untypedAtomic, which
// fn:sum casts to xs:double (F&O §15.4.5): "1e0" sums, which no xs:decimal
// cast admits, and 0.1 + 0.2 is the xs:double 0.30000000000000004 and not
// 0.3e0. A lexical no cast to xs:double admits raises err:FORG0001. With the
// items read in xs:decimal, the 1e0, 0.1 + 0.2 and `instance of xs:double`
// rows fail.
func TestAssertionSumOfUntypedIsDouble(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		bs    []smB
		holds bool
	}{
		{"sum(a/b/@p) = 1e0", []smB{smP("1e0")}, true},
		{"sum(a/b/@p) = 1", []smB{smP("1e0")}, true},
		{"sum(a/b/@p) = 0.3e0", []smB{smP("0.1"), smP("0.2")}, false},
		{"sum(a/b/@p) = 0.30000000000000004e0", []smB{smP("0.1"), smP("0.2")}, true},
		{"sum(a/b/@p) instance of xs:double", []smB{smP("0.1"), smP("0.2")}, true},
		{"sum(a/b/@p) instance of xs:double", []smB{smP("1")}, true},
		{"sum(a/b/@p) instance of xs:integer", nil, true},
		{"sum(a/b/@p) = 2", []smB{smP("INF"), smP("1")}, false},
		{"sum(a/b/@p) = 1e0", []smB{smP("abc")}, false},
		{"not(sum(a/b/@p) = 1e0)", []smB{smP("abc")}, false},
	} {
		test := smCompile(t, tc.expr, "anySimpleType")
		if got := smEval(t, test, smTally(t, test, "anySimpleType", tc.bs...)); got != tc.holds {
			t.Errorf("%q over %v: Evaluate = %v, want %v", tc.expr, tc.bs, got, tc.holds)
		}
	}
}

// A value of the other arm than the compiled type fixes breaks
// [Tally.Attribute]'s obligation, and the tree raises on it: an [Untyped]
// value where p is xs:decimal, and a [Typed] one where it is ·special·, are
// false where the right arm of the same lexical holds. The Untyped-for-
// xs:decimal row holds with ctaAttributePathItem validating an [Untyped]
// lexical under the compiled type, and the Typed-for-·special· row with it
// reading a [Typed] value as the lexical "5".
func TestAssertionSumRaisesOnWrongArm(t *testing.T) {
	for _, tc := range []struct {
		typ   string
		v     TypedValue
		holds bool
	}{
		{"decimal", smValue(t, "decimal", "5"), true},
		{"decimal", Untyped("5"), false},
		{"decimal", nil, false},
		{"anySimpleType", Untyped("5"), true},
		{"anySimpleType", smValue(t, "double", "5"), false},
	} {
		test := smCompile(t, "sum(a/b/@p) = 5", tc.typ)
		c := test.Tally()
		c.Element(nil, nil)
		c.Element(smPath[:1], nil)
		c.Element(smPath, []xsd.QName{uq("p")})
		c.Attribute(uq("p"), tc.v)
		if got := smEval(t, test, c); got != tc.holds {
			t.Errorf("xs:%s p reported as %#v: Evaluate = %v, want %v", tc.typ, tc.v, got, tc.holds)
		}
	}
}

// A Tally reads the values of the attribute nodes a path to an attribute
// selects: ReadsAttribute answers for (a/b, p) alone — not E's own p, not the
// a child's, not q — ReadsAttributesAt for depth 2 alone, and the {test}
// whose only path below E is `sum(a/b/@p)` has a Tally. A nil Tally reads
// nothing and takes nothing. With ctaAttributePath.reads matching the name at
// any non-empty path, the (a, p) and (b/a, p) rows fail.
func TestTallyReadsAttributeValues(t *testing.T) {
	c := smCompile(t, "sum(a/b/@p) = 0", "decimal").Tally()
	if c == nil {
		t.Fatal("Tally() of sum(a/b/@p) = 0 is nil, want one")
	}
	for _, tc := range []struct {
		path  []xsd.QName
		name  string
		reads bool
	}{
		{smPath, "p", true},
		{smPath[:1], "p", false},
		{nil, "p", false},
		{smPath, "q", false},
		{[]xsd.QName{uq("b"), uq("a")}, "p", false},
	} {
		if got := c.ReadsAttribute(tc.path, uq(tc.name)); got != tc.reads {
			t.Errorf("ReadsAttribute(%v, %s) = %v, want %v", tc.path, tc.name, got, tc.reads)
		}
	}
	for depth, want := range []bool{false, false, true, false} {
		if got := c.ReadsAttributesAt(depth); got != want {
			t.Errorf("ReadsAttributesAt(%d) = %v, want %v", depth, got, want)
		}
	}
	var none *Tally
	if none.ReadsAttribute(smPath, uq("p")) {
		t.Error("nil Tally: ReadsAttribute = true, want false")
	}
	none.Attribute(uq("p"), Untyped("1"))
	if test := smCompile(t, "sum(a/b/@p) = 0", "decimal"); test.ReadsChild(uq("a")) || test.ReadsChild(uq("b")) {
		t.Error("sum(a/b/@p) = 0: ReadsChild of a or b is true, want false")
	}
}

// Each breach of [Tally.Attribute]'s ordering makes Evaluate false where the
// same reports made in order hold: `sum(a/b/@p) = 5` over one b carrying p =
// 5, reported right, is the first row. Each breach row adds one misplaced call
// to that sequence, or drops one, so ignoring it would leave the sum 5. With
// Tally.Attribute keeping its counters on a value nothing owes, the first
// five breach rows hold; with Tally.Element ignoring an owed value, the
// next-report row does; and with Evaluate not asking Tally.settled, the
// Evaluate row does.
func TestTallyAttributeBreaches(t *testing.T) {
	test := smCompile(t, "sum(a/b/@p) = 5", "decimal")
	five := smValue(t, "decimal", "5")
	right := func(c *Tally) {
		c.Element(nil, nil)
		c.Element(smPath[:1], nil)
		c.Element(smPath, []xsd.QName{uq("p")})
		c.Attribute(uq("p"), five)
	}
	for _, tc := range []struct {
		breach string
		report func(c *Tally)
		holds  bool
	}{
		{"none", right, true},
		{"a value before any report", func(c *Tally) {
			c.Attribute(uq("p"), five)
			right(c)
		}, false},
		{"a value after E's own report", func(c *Tally) {
			c.Element(nil, []xsd.QName{uq("p")})
			c.Attribute(uq("p"), five)
			c.Element(smPath[:1], nil)
			c.Element(smPath, []xsd.QName{uq("p")})
			c.Attribute(uq("p"), five)
		}, false},
		{"a value for a name the report does not carry", func(c *Tally) {
			right(c)
			c.Element(smPath, nil)
			c.Attribute(uq("p"), five)
		}, false},
		{"a value no path reads", func(c *Tally) {
			right(c)
			c.Element(smPath, []xsd.QName{uq("q")})
			c.Attribute(uq("q"), five)
		}, false},
		{"a second value for one name", func(c *Tally) {
			right(c)
			c.Attribute(uq("p"), five)
		}, false},
		{"a value owed at the next report", func(c *Tally) {
			c.Element(nil, nil)
			c.Element(smPath[:1], nil)
			c.Element(smPath, []xsd.QName{uq("p")})
			c.Element(smPath, []xsd.QName{uq("p")})
			c.Attribute(uq("p"), five)
		}, false},
		{"a value owed at Evaluate", func(c *Tally) {
			right(c)
			c.Element(smPath, []xsd.QName{uq("p")})
		}, false},
	} {
		c := test.Tally()
		tc.report(c)
		if got := smEval(t, test, c); got != tc.holds {
			t.Errorf("breach %q: Evaluate = %v, want %v", tc.breach, got, tc.holds)
		}
	}
}

// An assertions facet has no context item, so the path fn:sum reads raises
// err:XPDY0002 and the facet FAILS rather than declining, which both rows do
// with ctaFacetFacade.attributePath declining.
func TestFacetSumOverAPathFails(t *testing.T) {
	intType := asBuiltin(t, "int")
	for _, expr := range []string{"sum(a/b/@p) = 0", "not(sum(a/b/@p) = 0)"} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, intType, ctaExprRecord(expr, ""), fcValue(t, intType, "1")); got != value.AssertionFails {
			t.Errorf("FacetAssertions(%q) = %d, want AssertionFails (%d)", expr, got, value.AssertionFails)
		}
	}
}

// The body of a quantifier over `$value` parses under a façade embedding the
// assertion's, which compiles a path to an attribute as the assertion's does.
func TestAssertionSumInsideAQuantifierBody(t *testing.T) {
	intType := asBuiltin(t, "int")
	content := xsd.SimpleContent{SimpleType: asList(t, "Ints", intType.Name())}
	test, ok := CompileAssertionTest(asRecord("some $x in data($value) satisfies $x = sum(a/b/@p)"), seededTypes, content, smUses(t, "decimal"), smElems(t))
	if !ok {
		t.Fatal("CompileAssertionTest: declined, want compiled")
	}
	bound := asBind(t, asList(t, "Ints", intType.Name()), "1 5")
	if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, smTally(t, test, "decimal", smP("2"), smP("3")), bound, time.Time{}); !got {
		t.Error("Evaluate = false, want true")
	}
}

// fn:sum is admitted with one argument over a path through child steps to an
// attribute step with a QName NameTest, and no other way. Every row below
// declines at CompileAssertionTest, under its GAP(xpath) (#1042): the
// two-argument form, E's own attribute, a child or element path read for its
// value, a wildcard attribute step, fn:avg, fn:min and fn:max, an attribute
// whose type is a duration or a list, an argument the path's static types do
// not type, and an fn:sum over xs:untypedAtomic or xs:double items — whose
// empty value, the xs:integer 0, is not of its static type — as an arithmetic
// operand, or compared with an xs:decimal operand but a literal whose value
// is zero exactly where its xs:double rounding is. The two arithmetic rows
// compile with ctaTypes.arithmeticOperand's fn:sum arm removed; the two c rows
// and the tiny-literal row with ctaTypes.sumCompares answering true, the tiny one
// alone with ctaZeroAlike answering true; and the two duration rows with
// ctaTypes.sum's duration test removed. An fn:distinct-values call over such
// a sum takes its static type and declines with it: its four rows compile
// with ctaFloatingSum not looking through fn:distinct-values.
func TestCompileAssertionTestDeclinesSums(t *testing.T) {
	for _, tc := range []struct{ expr, typ string }{
		{"sum(a/b/@p, 0) = 0", "decimal"},
		{"sum(@p) = 0", "decimal"},
		{"sum(@p, 0) = 0", "decimal"},
		{"sum(c) = 0", "decimal"},
		{"sum(a/c) = 0", "decimal"},
		{"sum(a/b/@*) = 0", "decimal"},
		{"sum(a//b/@p) = 0", "decimal"},
		{"sum(a/b[@q]/@p) = 0", "decimal"},
		{"sum(a/b/@r) = 0", "decimal"},
		{"avg(a/b/@p) = 0", "decimal"},
		{"min(a/b/@p) = 0", "decimal"},
		{"max(a/b/@p) = 0", "decimal"},
		{"sum(a/b/@p) = xs:dayTimeDuration('PT1S')", "dayTimeDuration"},
		{"sum(a/b/@p) = 0", "duration"},
		{"sum(a/b/@p) = 0", "NMTOKENS"},
		{"sum(a/b/@p) + 1 = 1", "anySimpleType"},
		{"sum(a/b/@p) div 0 = 1", "double"},
		{"c = sum(a/b/@p)", "anySimpleType"},
		{"sum(a/b/@p) eq c", "double"},
		{"sum(a/b/@p) = 0." + zeros(400) + "1", "anySimpleType"},
		{"distinct-values(sum(a/b/@p)) = 0." + zeros(400) + "1", "anySimpleType"},
		{"distinct-values(sum(a/b/@p)) = c", "anySimpleType"},
		{"not(distinct-values(sum(a/b/@p)) div 0 = 1)", "double"},
		{"distinct-values(sum(a/b/@p)) + 0.1 + 0.2 = 0.3", "anySimpleType"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, asElementContent(t, false), smUses(t, tc.typ), smElems(t)); ok {
			t.Errorf("CompileAssertionTest(%q) over xs:%s p: compiled, want declined", tc.expr, tc.typ)
		}
	}
}

// zeros is n zero digits.
func zeros(n int) string { return string(slices.Repeat([]byte{'0'}, n)) }

// A Type Alternative's {test} calls no F&O function but fn:not and
// constructors (§3.12.6 clause 3), so fn:sum, over E's attribute or a path,
// declines there.
func TestCompileCTATestDeclinesSum(t *testing.T) {
	for _, expr := range []string{"sum(@p) = 0", "sum(a/b/@p) = 0"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
}
