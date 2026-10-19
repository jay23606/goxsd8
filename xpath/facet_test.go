package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// fcValue maps lexical against st, or fails the test.
func fcValue(t *testing.T, st *xsd.SimpleType, lexical string) value.Value {
	t.Helper()
	v, err := value.ValidateLexical(backend(), seededTypes, st, lexical, nil, FacetAssertions(time.Time{}))
	if err != nil {
		t.Fatalf("mapping %q against %s: %v", lexical, st.Name(), err)
	}
	return v
}

// FacetAssertions decides an assertions facet's {test} over `$value` alone
// (cvc-assertions-valid clauses 1.1 and 1.4): `$value eq 100` holds for 100
// and fails for 5, `$value = 'x'` holds for "x" and fails for "y". A read of
// the context item, which clause 1.2 makes absent, raises err:XPDY0002 and
// FAILS — `.`, fn:not over it (the error propagates rather than inverting), an
// attribute step, a child-axis step and a rooted path alike. Each such row
// declines with the ctaFacetFacade method that builds its node answering
// false; `not(.)` and `not(@a)` hold with ctaNoContextItem.nodes answering
// (0, true), and `not(. = 'x')` with the ctaNoContextItem arm of ctaItemOf
// removed. fn:position and fn:last raise it too, there being no
// context position or size (clause 1.3): `position() le 50` and `last() le
// 50` fail, and so do they under fn:not, which a call evaluating to false
// would make hold, and so do `position()` and `last()` read for their
// ·effective boolean value·, bare or under fn:not, and under `instance of
// xs:integer`. `not(position())` and `not(last())` hold with the ctaNoFocus
// arm of ctaCarriedType removed, and the two `instance of` rows
// decline with ctaNoFocus removed from ctaTypes.instanceItem. fn:string over
// either fails too, the call raising before there is a value to cast
// (castsFrom): `string(position()) = '1'` and `string(last()) = '1'` decline
// with ctaNoFocus dropped from ctaTypes.castSource's unjudged arm. A
// constructor function over either fails as fn:string does, the cast's
// operand raising: `xs:integer(position())` and `xs:string(position()) = '1'`,
// each of which declines with constructorFunction parsing simpleValue alone,
// and the second with ctaNoFocus dropped from castSource's unjudged arm.
func TestFacetAssertionsDecideTheValue(t *testing.T) {
	intType, str := asBuiltin(t, "int"), asBuiltin(t, "string")
	for _, tc := range []struct {
		test    string
		st      *xsd.SimpleType
		lexical string
		want    value.AssertionOutcome
	}{
		{"$value eq 100", intType, "100", value.AssertionHolds},
		{"$value eq 100", intType, "5", value.AssertionFails},
		{"$value = 'x'", str, "x", value.AssertionHolds},
		{"$value = 'x'", str, "y", value.AssertionFails},
		{".", str, "x", value.AssertionFails},
		{"not(.)", str, "x", value.AssertionFails},
		{". = 'x'", str, "x", value.AssertionFails},
		{"not(. = 'x')", str, "x", value.AssertionFails},
		{"@a", str, "x", value.AssertionFails},
		{"not(@a)", str, "x", value.AssertionFails},
		{"a = 'x'", str, "x", value.AssertionFails},
		{"/root = 'present'", str, "x", value.AssertionFails},
		{"//root", str, "x", value.AssertionFails},
		{"$value = 'x' or .", str, "x", value.AssertionHolds},
		{"position() le 50", str, "x", value.AssertionFails},
		{"not(position() le 50)", str, "x", value.AssertionFails},
		{"last() le 50", str, "x", value.AssertionFails},
		{"not(last() le 50)", str, "x", value.AssertionFails},
		{"position()", str, "x", value.AssertionFails},
		{"not(position())", str, "x", value.AssertionFails},
		{"last()", str, "x", value.AssertionFails},
		{"not(last())", str, "x", value.AssertionFails},
		{"position() instance of xs:integer", str, "x", value.AssertionFails},
		{"last() instance of xs:integer", str, "x", value.AssertionFails},
		{"string(position()) = '1'", str, "x", value.AssertionFails},
		{"string(last()) = '1'", str, "x", value.AssertionFails},
		{"xs:integer(position())", str, "x", value.AssertionFails},
		{"xs:string(position()) = '1'", str, "x", value.AssertionFails},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, tc.st, ctaExprRecord(tc.test, "", "xs", xsd.XMLSchemaNS), fcValue(t, tc.st, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %d, want %d", tc.test, tc.lexical, got, tc.want)
		}
	}
}

// A castable or instance-of expression is an xs:boolean and an integer
// sequence a sequence of xs:integer (ctaCarriedType), never an
// xs:untypedAtomic: xpath20.md §3.5.2 casts only an untypedAtomic operand to
// the other's type, so each comparison with an xs:string is a B.2 mismatch,
// err:XPTY0004, and fails the facet, and fn:string over the xs:boolean is a
// cast across primitives castsFrom declines.
func TestFacetAssertionsStaticTypeOfAResult(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		test string
		want value.AssertionOutcome
	}{
		{"$value castable as xs:double = 'false'", value.AssertionFails},
		{"$value instance of xs:string = 'true'", value.AssertionFails},
		{"(1 to 3) = '2'", value.AssertionFails},
		{"string($value castable as xs:double) = 'false'", value.AssertionDeclined},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(tc.test, "", "xs", xsd.XMLSchemaNS), fcValue(t, str, "x"))
		if got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %d, want %d", tc.test, "x", got, tc.want)
		}
	}
}

// FacetAssertions declines a {test} outside the grammar CompileAssertionTest
// admits — arithmetic over a non-numeric `$value`, a call to a function
// libraryCall does not name, a variable other than `$value`, the
// abbreviated parent step.
func TestFacetAssertionsDecline(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, test := range []string{"$value mod 2 = 0", "upper-case($value) = 'X'", "$other = 'x'", ".. = 'x'"} {
		v := fcValue(t, str, "x")
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, ""), v); got != value.AssertionDeclined {
			t.Errorf("Evaluate(%q over %s) = %d, want declined", test, str.Name(), got)
		}
	}
}

// fcUnion builds a NAMED union simple type over by-name members in declared
// order, with no facets of its own.
func fcUnion(t *testing.T, local string, members ...xsd.QName) *xsd.SimpleType {
	t.Helper()
	slots := make([]xsd.SimpleTypeOrRef, 0, len(members))
	for _, m := range members {
		slots = append(slots, xsd.SimpleTypeRef{Name: m})
	}
	st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: local},
		xsd.UnionDerivation{Members: slots}, xsd.SimpleTypeRef{Name: ctaBuiltin("anySimpleType")}, nil, nil)
	if err != nil {
		t.Fatalf("building the %s union: %v", local, err)
	}
	return st
}

// fcAssertedUnion restricts the union base, whose {member type definitions}
// are members, with an assertions facet of its own carrying test.
func fcAssertedUnion(t *testing.T, base *xsd.SimpleType, test string, members ...xsd.QName) *xsd.SimpleType {
	t.Helper()
	slots := make([]xsd.SimpleTypeOrRef, 0, len(members))
	for _, m := range members {
		slots = append(slots, xsd.SimpleTypeRef{Name: m})
	}
	facets := []xsd.Facet{xsd.NewAssertionsFacet([]xsd.Assertion{xsd.NewAssertion(asRecord(test))})}
	st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: "Asserted" + base.Name().Local},
		xsd.UnionDerivation{Members: slots}, xsd.SimpleTypeRef{Name: base.Name()}, facets, nil)
	if err != nil {
		t.Fatalf("building the restriction of %s: %v", base.Name(), err)
	}
	return st
}

// fcOutcome is what value.ValidateLexical's err says the assertions stage
// decided: accepted is AssertionHolds, a cvc-assertions-valid verdict is
// AssertionFails and an assertions decline is AssertionDeclined.
func fcOutcome(t *testing.T, err error) value.AssertionOutcome {
	t.Helper()
	if err == nil {
		return value.AssertionHolds
	}
	if value.IsAssertionDeclined(err) {
		return value.AssertionDeclined
	}
	if rule, _ := xsderr.RuleOf(err); rule == "cvc-assertions-valid" && value.IsDatatypeVerdict(err) {
		return value.AssertionFails
	}
	t.Fatalf("ValidateLexical: %v, want an assertions-stage outcome", err)
	return value.AssertionDeclined
}

// A union's own assertions facet binds `$value` under the ·active basic
// member· the dispatch chose (dt-xdmrep clause 4, cvc-assertions-valid clause
// 1.4): over union(xs:date, xs:integer), `$value = 5` holds for 5 and fails
// for 6, `$value = xs:date('2008-01-01')` holds for that date and fails for
// another, and each fails across members, where comparing an xs:date with an
// xs:integer is a type error (err:XPTY0004), which cvc-assertions-valid
// charges as false. A union of that union with xs:string descends to the same
// basic member (dt-active-basic-member) and binds it, and a string literal
// binds xs:string. Every row but the guard's declines with validateUnion
// handing checkAssertions the union in place of the member, which
// ctaTypes.valueVariable declines. The guard: a {test} the façade declines
// over a non-union (fn:upper-case) still declines over a union.
func TestFacetAssertionsOverAUnionBindTheActiveBasicMember(t *testing.T) {
	date, integer, str := ctaBuiltin("date"), ctaBuiltin("integer"), ctaBuiltin("string")
	dateOrInt := fcUnion(t, "DateOrInt", date, integer)
	nested := fcUnion(t, "DateOrIntOrString", dateOrInt.Name(), str)
	for _, tc := range []struct {
		base    *xsd.SimpleType
		members []xsd.QName
		test    string
		lexical string
		want    value.AssertionOutcome
	}{
		{dateOrInt, []xsd.QName{date, integer}, "$value = 5", "5", value.AssertionHolds},
		{dateOrInt, []xsd.QName{date, integer}, "$value = 5", "6", value.AssertionFails},
		{dateOrInt, []xsd.QName{date, integer}, "$value = 5", "2008-01-01", value.AssertionFails},
		{dateOrInt, []xsd.QName{date, integer}, "$value = xs:date('2008-01-01')", "2008-01-01", value.AssertionHolds},
		{dateOrInt, []xsd.QName{date, integer}, "$value = xs:date('2008-01-01')", "2008-01-02", value.AssertionFails},
		{dateOrInt, []xsd.QName{date, integer}, "$value = xs:date('2008-01-01')", "5", value.AssertionFails},
		{nested, []xsd.QName{dateOrInt.Name(), str}, "$value = 5", "5", value.AssertionHolds},
		{nested, []xsd.QName{dateOrInt.Name(), str}, "$value = 5", "6", value.AssertionFails},
		{nested, []xsd.QName{dateOrInt.Name(), str}, "$value = xs:date('2008-01-01')", "2008-01-01", value.AssertionHolds},
		{nested, []xsd.QName{dateOrInt.Name(), str}, "$value = 'abc'", "abc", value.AssertionHolds},
		{nested, []xsd.QName{dateOrInt.Name(), str}, "$value = 'abc'", "5", value.AssertionFails},
		{dateOrInt, []xsd.QName{date, integer}, "upper-case($value) = 'X'", "5", value.AssertionDeclined},
	} {
		st := fcAssertedUnion(t, tc.base, tc.test, tc.members...)
		_, err := value.ValidateLexical(backend(), asTypesWith(dateOrInt, nested), st, tc.lexical, nil, FacetAssertions(time.Time{}))
		if got := fcOutcome(t, err); got != tc.want {
			t.Errorf("%s over %q against a restriction of %s = %d, want %d (err %v)", tc.test, tc.lexical, tc.base.Name().Local, got, tc.want, err)
		}
	}
}

// `.` raises on the facet façade alone: a Type Alternative's {test} declines
// it, and an assertion's, whose context item is E, declines it as a node — `.`
// and `not(.)` — and reads it as E's string value where it is atomized,
// `. = 'x'` (TestAssertionContextItemIsTheStringValue).
func TestContextItemDeclinesOutsideTheFacet(t *testing.T) {
	for _, expr := range []string{".", ". = 'x'", "not(.)"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{".", "not(.)"} {
		if _, ok := CompileAssertionTest(ctaExprRecord(expr, ""), seededTypes, xsd.SimpleContent{SimpleType: asBuiltin(t, "string")}, asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
}

// fn:position and fn:last over the facet's absent focus compile to an
// xs:integer that raises err:XPDY0002 (ctaNoFocus): `position() le 50` is a
// value comparison of that operand with 50, not the err:XPTY0004 ctaTypeError
// an untyped operand compared with an xs:integer would compile to.
func TestFacetFocusIsAnIntegerThatRaises(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, expr := range []string{"position() le 50", "last() le 50"} {
		root, defect := compileCTATest(ctaExprRecord(expr, ""), seededTypes, ctaFacetFacade{st: str})
		if defect.kind != ctaNoDefect {
			t.Fatalf("compileCTATest(%q): declined, want compiled", expr)
		}
		compare, isCompare := root.(ctaValueCompare)
		if !isCompare {
			t.Fatalf("compileCTATest(%q) = %T, want ctaValueCompare", expr, root)
		}
		if _, isFocus := compare.left.(ctaNoFocus); !isFocus {
			t.Errorf("compileCTATest(%q): left operand %T, want ctaNoFocus", expr, compare.left)
		}
	}
}

// fn:position and fn:last raise on the facet façade alone (guard): an
// assertion's focus is defined — E, position and size 1 (cvc-xpath) — and a
// Type Alternative's {test} calls no library function, so each declines
// there, under a constructor function too, and neither ever compiles to E's
// value.
func TestFocusDeclinesOutsideTheFacet(t *testing.T) {
	for _, expr := range []string{"position() = 1", "last() = 1"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"position() le 50", "last() le 50", "not(position() le 50)", "position() = '1'", "xs:integer(position())", "xs:string(position()) = '1'"} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.SimpleContent{SimpleType: asBuiltin(t, "string")}, asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
}

// An assertions facet casts a function result by a constructor function
// (xpath-functions.md §5, §7.4.1): over an xs:date `$value`, assert-simple007's
// `xs:date(concat(string($value), '!!!'))` is no xs:date lexical, so the cast
// raises err:FORG0001 and the facet fails (cvc-assertions-valid), while the
// same cast over `concat(string($value), "")` decides by the date, and a
// constructor over a concat of literals is that date. Every row declines
// without libraryCall's concat arm, and with constructorFunction parsing
// simpleValue alone.
func TestFacetAssertionsCastAFunctionResult(t *testing.T) {
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		test, lexical string
		want          value.AssertionOutcome
	}{
		{"xs:date(concat(string($value), '!!!')) gt xs:date('1900-01-01')", "2001-01-01", value.AssertionFails},
		{"xs:date(concat(string($value), '!!!')) gt xs:date('1900-01-01')", "1999-11-16+01:00", value.AssertionFails},
		{"xs:date(concat(string($value), '')) gt xs:date('1900-01-01')", "2001-01-01", value.AssertionHolds},
		{"xs:date(concat(string($value), '')) gt xs:date('1900-01-01')", "1066-03-03", value.AssertionFails},
		{"xs:date(concat('2008-01-0', '1')) eq xs:date('2008-01-01')", "2001-01-01", value.AssertionHolds},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, date, asRecord(tc.test), fcValue(t, date, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %d, want %d", tc.test, tc.lexical, got, tc.want)
		}
	}
}
