package validate

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive the two assertion rules, which charge a {test} the
// engine evaluates to false and record one it declines: cvc-assertion
// (§3.13.4.1) for a complex type's {assertions}, and cvc-assertions-valid
// (§4.3.13.3) for a simple type's assertions facet at every variety level
// (PRINCIPLES 12). Every schema seeds the builtin types, because an
// assertion-bearing restriction is a restriction OF one.

// aAssertion is one Assertion whose {test} is expr. A facet's expression is
// evaluated wherever xpath.FacetAssertions admits it, and a complex type's
// wherever xpath.CompileAssertionTest does; every other one is declined, and
// names its assertion in the record.
func aAssertion(expr string) xsd.Assertion {
	return xsd.NewAssertion(xsd.NewXPathExpression(expr, nil, nil, nil))
}

// aAssertions turns test expressions into an assertions facet's {value}.
func aAssertions(exprs ...string) []xsd.Assertion {
	out := make([]xsd.Assertion, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, aAssertion(e))
	}
	return out
}

// aRestriction builds a NAMED restriction of base carrying one assertions
// facet. It serves every variety: an atomic base, a list base and a union base
// all take their own assertions this way, which is the only way §4.3.13.2
// xr-assertions puts one on a list or a union at all — a freshly constructed
// <list> or <union> mints no assertions facet of its own.
func aRestriction(t *testing.T, name string, base xsd.QName, exprs ...string) *xsd.SimpleType {
	t.Helper()
	st, err := xsd.NewSimpleType(xsderr.Loc{}, local(name), xsd.RestrictionDerivation{},
		xsd.SimpleTypeRef{Name: base},
		[]xsd.Facet{xsd.NewAssertionsFacet(aAssertions(exprs...))}, nil)
	if err != nil {
		t.Fatalf("building the %s restriction: %v", name, err)
	}
	return st
}

// aList builds a NAMED list simple type over a by-name {item type definition},
// with the fixed collapse whiteSpace §3.16.2.1 manufactures for every <list>.
func aList(t *testing.T, name string, item xsd.QName) *xsd.SimpleType {
	t.Helper()
	st, err := xsd.NewSimpleType(xsderr.Loc{}, local(name),
		xsd.ListDerivation{Item: xsd.SimpleTypeRef{Name: item}},
		xsd.SimpleTypeRef{Name: icBuiltin("anySimpleType")},
		[]xsd.Facet{xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)}, nil)
	if err != nil {
		t.Fatalf("building the %s list: %v", name, err)
	}
	return st
}

// aUnion builds a NAMED union simple type over by-name members in declared
// order — the order the dispatch tries them in (dt-active-member).
func aUnion(t *testing.T, name string, members ...xsd.QName) *xsd.SimpleType {
	t.Helper()
	slots := make([]xsd.SimpleTypeOrRef, 0, len(members))
	for _, m := range members {
		slots = append(slots, xsd.SimpleTypeRef{Name: m})
	}
	st, err := xsd.NewSimpleType(xsderr.Loc{}, local(name), xsd.UnionDerivation{Members: slots},
		xsd.SimpleTypeRef{Name: icBuiltin("anySimpleType")}, nil, nil)
	if err != nil {
		t.Fatalf("building the %s union: %v", name, err)
	}
	return st
}

// aVarietyTypes is the fixture set every variety level is read off: an atomic
// restriction carrying an assertion, a second one over a different primitive,
// a list of the first, that list restricted with an assertion of its own, a
// union of the two atomics, and that union restricted the same way.
func aVarietyTypes(t *testing.T) []*xsd.SimpleType {
	t.Helper()
	return []*xsd.SimpleType{
		aRestriction(t, "AssertedInt", integerType(), "$value > 0"),
		aRestriction(t, "AssertedStr", icBuiltin("string"), "upper-case($value) != ''"),
		aList(t, "PlainList", local("AssertedInt")),
		aRestriction(t, "AssertedList", local("PlainList"), "sum($value) > 1"),
		aUnion(t, "PlainUnion", local("AssertedInt"), local("AssertedStr")),
		aRestriction(t, "AssertedUnion", local("PlainUnion"), "upper-case($value) != ''"),
	}
}

// aComplexType builds the governing type <root> is declared over.
func aComplexType(t *testing.T, uses []xsd.AttributeUse, content xsd.ContentType, assertions []xsd.Assertion) xsd.ComplexType {
	t.Helper()
	ct, err := xsd.NewComplexType(xsderr.Loc{}, local("RootType"), xsd.QName{}, nil,
		xsd.DerivationRestriction, false, attrContent(uses), nil, nil, content, nil, assertions)
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	return ct
}

// aTypes adds the builtin simple types and extra to b. The builtins are always
// seeded: an assertion-bearing type is a restriction OF one, and a base that
// resolves to nothing declines before any facet is read.
func aTypes(t *testing.T, b *xsd.SchemaBuilder, extra ...*xsd.SimpleType) {
	t.Helper()
	seeded, err := builtin.Seed(testBackend())
	if err != nil {
		t.Fatalf("seeding the builtin types: %v", err)
	}
	for _, st := range seeded {
		b.AddType(st)
	}
	for _, st := range extra {
		b.AddType(st)
	}
}

// aSchema finalizes a schema declaring <root> of type ct, with the builtin
// simple types seeded and extra added alongside them.
func aSchema(t *testing.T, ct xsd.ComplexType, extra ...*xsd.SimpleType) *xsd.Schema {
	t.Helper()
	return cSchemaFrom(t, ct, func(b *xsd.SchemaBuilder) { aTypes(t, b, extra...) })
}

// aAssess assesses root against schema and returns the whole Result: these
// tests read both accessors, since the claim is always about the two together.
func aAssess(t *testing.T, schema *xsd.Schema, root Element) *Result {
	t.Helper()
	v, err := New(schema, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := v.Assess(root)
	if res.Err() != nil {
		t.Fatalf("Err() = %v, want nil", res.Err())
	}
	return res
}

// wantRecords fails unless res charged nothing and recorded exactly one
// Unevaluated per want entry, in order, each under rule at at and naming its
// want string in the message.
func wantRecords(t *testing.T, res *Result, rule xsderr.Rule, at xsderr.Loc, want ...string) {
	t.Helper()
	if got := res.Violations(); len(got) != 0 {
		t.Fatalf("Violations() = %v, want none: an unevaluated check is never a charge", got)
	}
	got := res.Unevaluated()
	if len(got) != len(want) {
		t.Fatalf("Unevaluated() recorded %d sites %v, want %d: %v", len(got), messages(got), len(want), want)
	}
	for i, u := range got {
		if u.Rule() != rule {
			t.Errorf("Unevaluated()[%d].Rule() = %q, want %q", i, u.Rule(), rule)
		}
		if u.Loc() != at {
			t.Errorf("Unevaluated()[%d].Loc() = %s, want %s", i, u.Loc(), at)
		}
		if !strings.Contains(u.Msg(), want[i]) {
			t.Errorf("Unevaluated()[%d].Msg() = %q, want it to name %s", i, u.Msg(), want[i])
		}
	}
}

func messages(us []Unevaluated) []string {
	out := make([]string, 0, len(us))
	for _, u := range us {
		out = append(out, u.Msg())
	}
	return out
}

// cvc-complex-type clause 6 sends E to cvc-assertion (§3.13.4.1) once per
// assertion in T.{assertions}. One xpath DECLINES — `@a = @b` names attributes
// T has no use for, and `count(*) = 0` is outside the grammar — is recorded at
// the ELEMENT's location, in {assertions} order, and none is charged: an
// element whose only defect could be an undecided assertion is not rejected.
func TestDeclinedComplexTypeAssertionsAreRecordedNeverCharged(t *testing.T) {
	schema := aSchema(t, aComplexType(t, nil, xsd.EmptyContent{},
		aAssertions("@a = @b", "count(*) = 0")))

	res := aAssess(t, schema, &testElement{name: local("root"), loc: loc(1, 1)})

	// The {test} identifies WHICH assertion each record stands for, so the
	// {assertions} order is observable and not merely counted.
	wantRecords(t, res, "cvc-assertion", loc(1, 1), `"@a = @b"`, `"count(*) = 0"`)
	if !strings.Contains(res.Unevaluated()[0].Msg(), "clause 6") {
		t.Errorf("Msg = %q, want cvc-complex-type clause 6 named", res.Unevaluated()[0].Msg())
	}
	if !strings.Contains(res.Unevaluated()[0].Msg(), "assertion 1 of 2") {
		t.Errorf("Msg = %q, want the assertion's position in {assertions} named", res.Unevaluated()[0].Msg())
	}
}

// aRoot is <root> carrying the attributes name/lexical pairs give, in that
// order, at columns 10, 20, ….
func aRoot(pairs ...string) *testElement {
	e := &testElement{name: local("root"), loc: loc(1, 1)}
	for i := 0; i+1 < len(pairs); i += 2 {
		e.attrs = append(e.attrs, &testAttribute{name: local(pairs[i]), value: pairs[i+1], loc: loc(1, 10*(i/2+1))})
	}
	return e
}

// aTyped builds RootType with one optional use per name/type pair, over a
// builtin type each, and the assertions given.
func aTyped(t *testing.T, pairs []string, exprs ...string) *xsd.Schema {
	t.Helper()
	var uses []xsd.AttributeUse
	for i := 0; i+1 < len(pairs); i += 2 {
		uses = append(uses, typedUse(t, pairs[i], icBuiltin(pairs[i+1]), false, nil, nil))
	}
	return aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, aAssertions(exprs...)))
}

// wantAssertionCharge fails unless res charged exactly one violation, under
// cvc-assertion at <root>, whose message OPENS with the element and the
// assertion's position — the subject a swapped argument would move — and
// recorded nothing.
func wantAssertionCharge(t *testing.T, res *Result, opening string) {
	t.Helper()
	if got := res.Unevaluated(); len(got) != 0 {
		t.Fatalf("Unevaluated() = %v, want none: an evaluated assertion is not a decline", messages(got))
	}
	got := res.Violations()
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want one cvc-assertion charge", got)
	}
	if got[0].Rule != "cvc-assertion" || got[0].Loc != loc(1, 1) {
		t.Errorf("charge = %s at %s, want cvc-assertion at %s", got[0].Rule, got[0].Loc, loc(1, 1))
	}
	if !strings.HasPrefix(got[0].Msg, opening) {
		t.Errorf("Msg = %q, want it to open %q", got[0].Msg, opening)
	}
}

// An assertion inside the grammar xpath compiles is EVALUATED over the
// element's TYPED attributes (cvc-assertion clause 1): `@x > 300` with x an
// xs:integer holds for 500, is charged for 200, and is charged for an absent
// x — the empty sequence forms no pair, so the general comparison is false.
func TestAdmittedAssertionIsEvaluated(t *testing.T) {
	schema := aTyped(t, []string{"x", "integer"}, "@x > 300")

	if res := aAssess(t, schema, aRoot("x", "500")); len(res.Violations()) != 0 || len(res.Unevaluated()) != 0 {
		t.Errorf("x=500: Violations() = %v, Unevaluated() = %v, want both empty: the assertion holds",
			res.Violations(), messages(res.Unevaluated()))
	}
	wantAssertionCharge(t, aAssess(t, schema, aRoot("x", "200")),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "@x > 300",`)
	wantAssertionCharge(t, aAssess(t, schema, aRoot()), "the element root is not ·valid· with respect to assertion 1 of 1")
}

// Attributes are compared TYPED: as xs:int values 10 <= 9 is false and
// charged, where the xs:string comparison an untyped reading makes holds.
func TestAssertionComparesTypedValues(t *testing.T) {
	schema := aTyped(t, []string{"min", "int", "max", "int"}, "@min <= @max")

	wantAssertionCharge(t, aAssess(t, schema, aRoot("min", "10", "max", "9")), "the element root is not ·valid·")
	if res := aAssess(t, schema, aRoot("min", "9", "max", "10")); len(res.Violations()) != 0 || len(res.Unevaluated()) != 0 {
		t.Errorf("9 <= 10: Violations() = %v, Unevaluated() = %v, want both empty", res.Violations(), messages(res.Unevaluated()))
	}
}

// An attribute whose type is ·special· is read as xs:untypedAtomic, its
// [schema normalized value] (xpath-datamodel §3.3.1.2): `@x > 300` casts it to
// xs:double (xpath20.md §3.5.2 clause 2.1), so x="304" is satisfied and
// x="204" charged — the vc001 shape — and x="abc" does not cast, which raises
// err:FORG0001 and is charged under cvc-assertion as well. Two such attributes
// compare as xs:string (clause 1), so "10" > "9" is charged.
//
// With the ·special· arm of xpath's assertion façade removed every row is
// declined instead, and fails; with walk.assertionValues yielding no value for
// a ·special· attribute, the two satisfied rows are charged, and fail.
func TestAssertionReadsSpecialAttributeUntyped(t *testing.T) {
	single := aTyped(t, []string{"x", "anySimpleType"}, "@x > 300")
	pair := aTyped(t, []string{"x", "anySimpleType", "y", "anySimpleType"}, "@x > @y")
	for _, tc := range []struct {
		name    string
		schema  *xsd.Schema
		root    *testElement
		charged bool
	}{
		{"304 > 300", single, aRoot("x", "304"), false},
		{"204 > 300", single, aRoot("x", "204"), true},
		{"abc > 300 raises FORG0001", single, aRoot("x", "abc"), true},
		{"10 > 9 as strings", pair, aRoot("x", "10", "y", "9"), true},
		{"9 > 10 as strings", pair, aRoot("x", "9", "y", "10"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := aAssess(t, tc.schema, tc.root)
			if !tc.charged {
				wantSatisfied(t, res, tc.name)
				return
			}
			wantAssertionCharge(t, res, "the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is ")
		})
	}
}

// A VALUE comparison is evaluated as §3.13.2's own example writes it:
// `@min le @max` over two xs:int attributes is charged for min="6" max="5" —
// the d4_3_15ii01 shape — and holds for min="5" max="6".
func TestValueComparisonAssertionIsEvaluated(t *testing.T) {
	schema := aTyped(t, []string{"min", "int", "max", "int"}, "@min le @max")

	wantAssertionCharge(t, aAssess(t, schema, aRoot("min", "6", "max", "5")),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "@min le @max",`)
	if res := aAssess(t, schema, aRoot("min", "5", "max", "6")); len(res.Violations()) != 0 || len(res.Unevaluated()) != 0 {
		t.Errorf("5 le 6: Violations() = %v, Unevaluated() = %v, want both empty", res.Violations(), messages(res.Unevaluated()))
	}
}

// aSimple builds RootType over a simple {content type} of the builtin typ, with
// the attribute uses and assertions given, and declares <root> over it,
// {nillable} as given.
func aSimple(t *testing.T, typ string, nillable bool, uses []xsd.AttributeUse, exprs ...string) *xsd.Schema {
	t.Helper()
	td, declared := builtinType(t, typ)
	if !declared {
		t.Fatalf("no builtin xs:%s", typ)
	}
	ct := aComplexType(t, uses, xsd.SimpleContent{SimpleType: td}, aAssertions(exprs...))
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "root"},
		xsd.TypeDefinitionRef{Name: ct.Name()}, nil, xsd.NewGlobalScope(), nil, nillable, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	aTypes(t, b)
	b.AddType(ct)
	b.AddElement(e)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the schema: %v", err)
	}
	return schema
}

// builtinType is the seeded builtin simple type named local.
func builtinType(t *testing.T, local string) (*xsd.SimpleType, bool) {
	t.Helper()
	seeded, err := builtin.Seed(testBackend())
	if err != nil {
		t.Fatalf("seeding the builtin types: %v", err)
	}
	for _, st := range seeded {
		if st.Name() == icBuiltin(local) {
			return st, true
		}
	}
	return nil, false
}

// wantSatisfied fails unless res charged and recorded nothing.
func wantSatisfied(t *testing.T, res *Result, why string) {
	t.Helper()
	if len(res.Violations()) != 0 || len(res.Unevaluated()) != 0 {
		t.Errorf("%s: Violations() = %v, Unevaluated() = %v, want both empty", why, res.Violations(), messages(res.Unevaluated()))
	}
}

// `$value` over a SIMPLE {content type} is the element's ·actual value·
// (cvc-assertion clause 2.3.1), typed by the {simple type definition}: `$value
// eq 5` holds for "+05", is charged for "6", and the value the ·initial value·
// maps to is what is compared, white space collapsed.
func TestValueAssertionReadsSimpleContent(t *testing.T) {
	schema := aSimple(t, "int", false, nil, "$value eq 5")

	wantSatisfied(t, aAssess(t, schema, cRoot("# +05 ")), "$value eq 5 over +05")
	wantAssertionCharge(t, aAssess(t, schema, cRoot("#6")),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "$value eq 5",`)
}

// `$value` over a ·special· {simple type definition} is the element's [schema
// normalized value] as xs:untypedAtomic (cvc-assertion clause 2.3.1, Datatypes
// dt-xdmrep clause 1): `$value eq 'x'` holds for "x", white space preserved,
// and is charged for " x". The satisfied row fails with walk.assertionValue
// binding the empty sequence over a ·special· type, which charges it.
func TestValueAssertionReadsSpecialContentUntyped(t *testing.T) {
	schema := aSimple(t, "anySimpleType", false, nil, "$value eq 'x'")

	wantSatisfied(t, aAssess(t, schema, cRoot("#x")), "$value eq 'x' over anySimpleType content x")
	wantAssertionCharge(t, aAssess(t, schema, cRoot("# x")),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "$value eq 'x'",`)
}

// `$value` over a {content type} that is not simple is the EMPTY SEQUENCE
// (cvc-assertion clause 2.3.2), so `$value eq 1` is the empty sequence, false
// under fn:boolean and charged, and its negation is satisfied.
func TestValueAssertionOverElementOnlyContentIsEmpty(t *testing.T) {
	content := cSequence(t, false, cParticle(t, "a", 0, 1))
	charged := aSchema(t, aComplexType(t, nil, content, aAssertions("$value eq 1")))
	wantAssertionCharge(t, aAssess(t, charged, cRoot()), "the element root is not ·valid· with respect to assertion 1 of 1")

	negated := aSchema(t, aComplexType(t, nil, content, aAssertions("not($value eq 1)")))
	wantSatisfied(t, aAssess(t, negated, cRoot()), "not($value eq 1) over element-only content")
}

// An element ALREADY KNOWN TO BE INVALID when its assertions are evaluated —
// here for a missing required attribute, cvc-complex-type clause 3 — binds
// `$value` to the empty sequence (cvc-assertion clauses 2.3.1.1 and 2.3.2),
// not to its ·actual value·: `not($value eq 5)` over "5" is satisfied, where
// the value would make it false.
func TestValueAssertionOverAnInvalidElementIsEmpty(t *testing.T) {
	uses := []xsd.AttributeUse{typedUse(t, "r", icBuiltin("string"), true, nil, nil)}
	schema := aSimple(t, "int", false, uses, "not($value eq 5)")

	res := aAssess(t, schema, cRoot("#5"))
	if got := res.Violations(); len(got) != 1 || got[0].Rule != "cvc-complex-type" {
		t.Fatalf("Violations() = %v, want the cvc-complex-type clause 3 charge alone: the assertion holds over the empty sequence", got)
	}
	if got := res.Unevaluated(); len(got) != 0 {
		t.Errorf("Unevaluated() = %v, want none", messages(got))
	}
	wantSatisfied(t, aAssess(t, aSimple(t, "int", false, nil, "$value eq 5"), cRoot("#5")), "$value eq 5 over a valid 5")
}

// `.` over simple content is the element's string value, its ·initial value·
// unnormalized and as xs:untypedAtomic, and never `$value` ([xpath.BindValue]):
// over xs:integer content "0030" `string-length(.) = 4` holds where `$value` is
// 30, and over "30" it is charged. An element already known to be invalid — a
// missing required attribute, cvc-complex-type clause 3 — still has its string
// value, while `$value` is the empty sequence (cvc-assertion clause 2.3.2), so
// `. = 5 and empty($value)` holds over "5". An empty element its declaration
// defaults reads the default's {lexical form}, the text node xpath-datamodel
// Appendix J.2 "may" build for it, and so `. = ""` is charged there: that is
// the processor's choice walk.assertionValue's GAP(xpath) marks, §6.2.4 giving
// an empty element "" instead. The invalid row is charged with
// walk.assertionValue binding "" for an invalid element, and the defaulted
// rows' verdicts swap with it binding the ·initial value· in place of
// [contentCheck.assessed]'s.
func TestContextItemAssertionReadsTheStringValue(t *testing.T) {
	schema := aSimple(t, "integer", false, nil, "string-length(.) = 4 and $value = 30")
	wantSatisfied(t, aAssess(t, schema, cRoot("#0030")), "string-length(.) = 4 over 0030")
	wantAssertionCharge(t, aAssess(t, schema, cRoot("#30")),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "string-length(.) = 4 and $value = 30",`)

	uses := []xsd.AttributeUse{typedUse(t, "r", icBuiltin("string"), true, nil, nil)}
	res := aAssess(t, aSimple(t, "int", false, uses, ". = 5 and empty($value)"), cRoot("#5"))
	if got := res.Violations(); len(got) != 1 || got[0].Rule != "cvc-complex-type" {
		t.Fatalf("Violations() = %v, want the cvc-complex-type clause 3 charge alone: `.` reads 5 while $value is empty", got)
	}
	if got := res.Unevaluated(); len(got) != 0 {
		t.Errorf("Unevaluated() = %v, want none", messages(got))
	}

	wantSatisfied(t, aAssess(t, aDefaultedContent(t, ". = '007' and $value = 7"), cRoot()), ". = '007' over an element defaulted to 007")
	wantAssertionCharge(t, aAssess(t, aDefaultedContent(t, ". = ''"), cRoot()),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is ". = ''",`)
}

// aDefaultedContent is a schema whose root element declaration defaults its
// xs:integer simple content to "007", under the one assertion test.
func aDefaultedContent(t *testing.T, test string) *xsd.Schema {
	t.Helper()
	td, _ := builtinType(t, "integer")
	ct := aComplexType(t, nil, xsd.SimpleContent{SimpleType: td}, aAssertions(test))
	dflt := xsd.NewValueConstraint(xsd.ValueDefault, "007", nil, nil)
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "root"},
		xsd.TypeDefinitionRef{Name: ct.Name()}, nil, xsd.NewGlobalScope(), &dflt, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	aTypes(t, b)
	b.AddType(ct)
	b.AddElement(e)
	defaulted, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the schema: %v", err)
	}
	return defaulted
}

// An element under simple content with an element [[child]] — charged under
// cvc-complex-type clause 1.2 — has a string value the walk never gathers, so
// its assertions are recorded Unevaluated and never evaluated over the text
// gathered before the charge: `. = 'ab'` over a<x/>b, whose string value "ab"
// satisfies it and whose gathered "a" does not, and `. = 'x'` over <x/>, false
// over either. Both rows are charged under cvc-assertion with
// walk.assertionValue binding the gathered text in place of declining.
func TestContextItemAssertionOverElementChildrenDeclines(t *testing.T) {
	for _, tc := range []struct {
		test string
		root *testElement
	}{
		{". = 'ab'", cRoot("#a", "x", "#b")},
		{". = 'x'", cRoot("x")},
	} {
		t.Run(tc.test, func(t *testing.T) {
			res := aAssess(t, aSimple(t, "string", false, nil, tc.test), tc.root)
			if got := res.Violations(); len(got) != 1 || got[0].Rule != "cvc-complex-type" {
				t.Fatalf("Violations() = %v, want the cvc-complex-type clause 1.2 charge alone", got)
			}
			got := res.Unevaluated()
			if len(got) != 1 || got[0].Rule() != "cvc-assertion" || got[0].Loc() != loc(1, 1) {
				t.Fatalf("Unevaluated() = %v, want one cvc-assertion record at %s", messages(got), loc(1, 1))
			}
			if want := "the element root has simple content and element [[children]]"; !strings.Contains(got[0].Msg(), want) {
				t.Errorf("Msg = %q, want it to name %q", got[0].Msg(), want)
			}
		})
	}
}

// A ·nilled· element binds `$value` to the empty sequence (cvc-assertion clause
// 2.3.1.2), whatever its type's {content type}. Over xs:string the case is
// discriminating: the empty ·initial value· a ·nilled· element carries is a
// valid xs:string, so reading it would make `$value` equal the empty string.
func TestValueAssertionOverANilledElementIsEmpty(t *testing.T) {
	root := cRoot()
	root.attrs = []Attribute{&testAttribute{
		name: xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, value: "true", loc: loc(1, 10)}}

	wantSatisfied(t, aAssess(t, aSimple(t, "string", true, nil, "not($value eq '')"), root), "not($value eq '') over a ·nilled· element")
	wantAssertionCharge(t, aAssess(t, aSimple(t, "string", true, nil, "$value eq ''"), root), "the element root is not ·valid· with respect to assertion 1 of 1")
}

// A ·nilled· element is dm:nilled false in the partial ·PSVI· (xpath-datamodel
// §6.2.4 nilled), so its string value is its descendants' text, never "" while
// it has [[children]]: under cvc-elt clause 3.2.3.1's charge its assertions are
// recorded Unevaluated and never evaluated over a string value this walk did
// not gather — `. = 'abc'` over the text abc, which that string value
// satisfies, and `. = 'x'` over <x/>, which it does not. Both rows are charged
// under cvc-assertion with walk.assertionValue binding "" for every ·nilled·
// element in place of declining. The guard: with no [[children]] `.` is the
// zero-length string, and `. = ""` holds.
func TestContextItemAssertionOverANilledElementWithChildrenDeclines(t *testing.T) {
	nilled := func(kids ...string) *testElement {
		root := cRoot(kids...)
		root.attrs = []Attribute{&testAttribute{
			name: xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, value: "true", loc: loc(1, 10)}}
		return root
	}
	for _, tc := range []struct {
		test string
		root *testElement
	}{
		{". = 'abc'", nilled("#abc")},
		{". = 'x'", nilled("x")},
	} {
		t.Run(tc.test, func(t *testing.T) {
			res := aAssess(t, aSimple(t, "string", true, nil, tc.test), tc.root)
			if got := res.Violations(); len(got) != 1 || got[0].Rule != "cvc-elt" {
				t.Fatalf("Violations() = %v, want the cvc-elt clause 3.2.3.1 charge alone", got)
			}
			got := res.Unevaluated()
			if len(got) != 1 || got[0].Rule() != "cvc-assertion" || got[0].Loc() != loc(1, 1) {
				t.Fatalf("Unevaluated() = %v, want one cvc-assertion record at %s", messages(got), loc(1, 1))
			}
			if want := "the element root has xsi:nil = true and [[children]]"; !strings.Contains(got[0].Msg(), want) {
				t.Errorf("Msg = %q, want it to name %q", got[0].Msg(), want)
			}
		})
	}
	wantSatisfied(t, aAssess(t, aSimple(t, "string", true, nil, ". = ''"), nilled()), ". = '' over a ·nilled· element with no [[children]]")
}

// Where String Valid over the ·initial value· is WITHHELD, `$value` is
// undecided, and every assertion of the element is declined — never charged,
// never satisfied — beside clause 1.2's own record.
func TestValueAssertionOverAnUndecidedValueIsDeclined(t *testing.T) {
	td, _ := builtinType(t, "decimal")
	schema := aSchema(t, aComplexType(t, nil, xsd.SimpleContent{SimpleType: td}, aAssertions("$value gt 1")))

	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("decimal")), schema, cRoot("#1.5"))
	if len(got) != 0 {
		t.Fatalf("Violations() = %v, want none: an undecided $value charges nothing", got)
	}
	if len(undecided) != 2 || undecided[1].Rule() != "cvc-assertion" ||
		!strings.Contains(undecided[1].Msg(), "the element root has simple content whose [schema actual value], which cvc-assertion clause 2.3.1 binds to $value, is undecided") {
		t.Fatalf("Unevaluated() = %v, want clause 1.2's record then the assertion declined for its undecided $value", messages(undecided))
	}
}

// An admitted {test} that RAISES is charged, never declined: `@b = 'true'`
// over an xs:boolean @b compares xs:boolean with xs:string, err:XPTY0004, and
// cvc-assertion is satisfied only by a {test} that is true "without raising
// any dynamic error or type error".
func TestAssertionRaisingATypeErrorIsCharged(t *testing.T) {
	schema := aTyped(t, []string{"b", "boolean"}, "@b = 'true'")

	wantAssertionCharge(t, aAssess(t, schema, aRoot("b", "true")), "the element root is not ·valid·")
}

// assert011's {test} calls fn:doc, which raises err:FODC0005 because cvc-xpath
// clause 8 leaves no available document (F&O §15.5.4), so the assertion is
// charged, never recorded Unevaluated, whatever @event holds. It is recorded
// Unevaluated instead with xpath's fn:doc arm removed, and with its `./@`
// attribute step declined.
func TestAssertionCallingDocIsCharged(t *testing.T) {
	const test = "./@event = doc('assert011-event-list.xml')//event/@name"
	schema := aSimple(t, "date", false, []xsd.AttributeUse{typedUse(t, "event", icBuiltin("string"), false, nil, nil)}, test)
	root := cRoot("#2008-07-01")
	root.attrs = append(root.attrs, &testAttribute{name: local("event"), value: "birth", loc: loc(1, 10)})

	wantAssertionCharge(t, aAssess(t, schema, root),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "`+test+`",`)
}

// A {test} outside what xpath compiles is DECLINED through walk.decline —
// recorded under cvc-assertion at the element, never charged, never satisfied
// — whatever the attribute values would have made of it.
func TestOutOfFamilyAssertionIsDeclined(t *testing.T) {
	for _, expr := range []string{"$other > 0", "@x eq 5 eq 5", "count(@*) = 1", "@* = 5", "@y = 5"} {
		schema := aTyped(t, []string{"x", "integer"}, expr)
		res := aAssess(t, schema, aRoot("x", "500"))
		wantRecords(t, res, "cvc-assertion", loc(1, 1), "whose {test} is "+strconv.Quote(expr)+", was not evaluated: this engine's XPath evaluator declined it")
	}
}

// An element one of whose use-matched attributes has no ·actual value· has
// every assertion DECLINED: reading `@x` as the empty sequence would make
// `not(@x)` true and `@x > 300` false on a value the instance does carry. The
// cvc-attribute clause 3 charge stands beside the record.
func TestAssertionOverAnAttributeWithoutActualValueIsDeclined(t *testing.T) {
	schema := aTyped(t, []string{"x", "integer"}, "@x > 300")

	res := aAssess(t, schema, aRoot("x", "many"))

	if got := res.Violations(); len(got) != 1 || got[0].Rule != "cvc-attribute" {
		t.Fatalf("Violations() = %v, want the cvc-attribute clause 3 charge alone", got)
	}
	got := res.Unevaluated()
	if len(got) != 1 || got[0].Rule() != "cvc-assertion" ||
		!strings.Contains(got[0].Msg(), "the attribute x of the element root has no ·actual value·") {
		t.Fatalf("Unevaluated() = %v, want the assertion declined for x's missing ·actual value·", messages(got))
	}
}

// aDefaulted builds RootType with one optional use of x, of the builtin typ,
// whose {value constraint} defaults it to lexical, and the assertions given.
func aDefaulted(t *testing.T, typ, lexical string, exprs ...string) *xsd.Schema {
	t.Helper()
	dflt := xsd.NewValueConstraint(xsd.ValueDefault, lexical, nil, nil)
	uses := []xsd.AttributeUse{typedUse(t, "x", icBuiltin(typ), false, nil, &dflt)}
	return aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, aAssertions(exprs...)))
}

// A ·defaulted attribute· the element does not carry is PRESENT in the partial
// ·PSVI· cvc-assertion clause 1.2 builds from (key-dflt-att), read as its
// ·effective value constraint· supplies it: `@x > 300` over a default of "500"
// is satisfied and over "200" charged, a ·special· type's default is read as
// xs:untypedAtomic, and a carried x overrides the default. The two satisfied
// rows over an uncarried x fail with walk.assertionValues yielding no
// defaulted attribute, which reads @x as the empty sequence.
func TestAssertionReadsADefaultedAttribute(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schema  *xsd.Schema
		root    *testElement
		charged bool
	}{
		{"default 500 > 300", aDefaulted(t, "integer", "500", "@x > 300"), aRoot(), false},
		{"default 200 > 300", aDefaulted(t, "integer", "200", "@x > 300"), aRoot(), true},
		{"untyped default 304 > 300", aDefaulted(t, "anySimpleType", "304", "@x > 300"), aRoot(), false},
		{"carried 200 over default 500", aDefaulted(t, "integer", "500", "@x > 300"), aRoot("x", "200"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := aAssess(t, tc.schema, tc.root)
			if !tc.charged {
				wantSatisfied(t, res, tc.name)
				return
			}
			wantAssertionCharge(t, res, "the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is ")
		})
	}
}

// A ·defaulted attribute· whose {lexical form} the value space cannot decide
// has no ·actual value·, so every assertion of the element is DECLINED,
// carrying the use's name — never read as the empty sequence, which would
// charge `not(@x)` false here — beside cvc-complex-type clause 4's own record.
func TestAssertionOverAnUndecidedDefaultedAttributeIsDeclined(t *testing.T) {
	schema := aDefaulted(t, "decimal", "500", "not(@x)")

	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("decimal")), schema, aRoot())
	if len(got) != 0 {
		t.Fatalf("Violations() = %v, want none: an undecided default charges nothing", got)
	}
	last := len(undecided) - 1
	if last < 0 || undecided[last].Rule() != "cvc-assertion" ||
		!strings.HasPrefix(undecided[last].Msg(), "assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is \"not(@x)\", was not evaluated: the ·defaulted attribute· x of the element root has no ·actual value·") {
		t.Fatalf("Unevaluated() = %v, want the assertion declined last for the defaulted x's missing ·actual value·", messages(undecided))
	}
}

// A ·defaulted attribute·'s ·actual value· is mapped under the namespace
// bindings its {value constraint} captured (value.ConstraintContext), never
// the element's: an xs:QName x defaulting to "p:a", p bound to urn:a on the
// constraint alone, reaches `not(@y)` as a value whether the element binds no p
// or binds it elsewhere, and the assertion is evaluated and satisfied, with
// cvc-complex-type clause 4 decided beside it. With the default mapped under
// elementContext the first row declines the assertion for x's missing ·actual
// value·; the second guards against declining every QName-governed default
// outright, which would decline it as well.
func TestAssertionReadsADefaultUnderItsOwnBindings(t *testing.T) {
	dflt := xsd.NewValueConstraint(xsd.ValueDefault, "p:a", []xsd.NamespaceBinding{xsd.NewNamespaceBinding("p", "urn:a")}, nil)
	uses := []xsd.AttributeUse{
		typedUse(t, "x", icBuiltin("QName"), false, &dflt, nil),
		typedUse(t, "y", icBuiltin("integer"), false, nil, nil),
	}
	schema := aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, aAssertions("not(@y)")))
	elsewhere := aRoot()
	elsewhere.bindings = map[string]string{"p": "urn:other"}
	for _, tc := range []struct {
		name string
		root *testElement
	}{
		{"p unbound in the instance", aRoot()},
		{"p bound elsewhere in the instance", elsewhere},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantSatisfied(t, aAssess(t, schema, tc.root), "not(@y) evaluated over x's ·actual value·")
		})
	}
}

// A complex type with no {assertions} records nothing: the visit is per
// assertion, not per element.
func TestElementWithoutAssertionsRecordsNothing(t *testing.T) {
	schema := aSchema(t, aComplexType(t, nil, xsd.EmptyContent{}, nil))

	res := aAssess(t, schema, &testElement{name: local("root"), loc: loc(1, 1)})

	if got := res.Unevaluated(); got != nil {
		t.Errorf("Unevaluated() = %v, want nil for a type carrying no assertions", messages(got))
	}
}

// aFacetTypes is aVarietyTypes plus the facets the evaluation tests read: two
// whose {test} the facet evaluator admits (`$value eq 100` over xs:integer,
// `$value = 'x'` over xs:string), one reading the absent context item, two it
// declines (a range expression whose upper operand is not an IntegerLiteral,
// and an fn:upper-case call, a function outside the string and sequence core
// xpath.CompileAssertionTest calls), and a union whose first
// member restricts xs:ENTITY with `$value = 'x'` ahead of xs:string.
func aFacetTypes(t *testing.T) []*xsd.SimpleType {
	t.Helper()
	return append(aVarietyTypes(t),
		aRestriction(t, "EqHundred", integerType(), "$value eq 100"),
		aRestriction(t, "IsX", icBuiltin("string"), "$value = 'x'"),
		aRestriction(t, "Dot", icBuiltin("string"), ". = 'x'"),
		aRestriction(t, "InRange", integerType(), "$value = 1 to $value"),
		aRestriction(t, "UpperX", icBuiltin("string"), "upper-case($value) = 'AX'"),
		aRestriction(t, "EntityX", icBuiltin("ENTITY"), "$value = 'x'"),
		aRestriction(t, "PastDate", icBuiltin("date"), "$value lt current-date()"),
		aUnion(t, "EntityXOrString", local("EntityX"), icBuiltin("string")),
	)
}

// aAttributeAssessed assesses <root n="lexical"/>, n typed by typ.
func aAttributeAssessed(t *testing.T, typ, lexical string) *Result {
	t.Helper()
	uses := []xsd.AttributeUse{typedUse(t, "n", local(typ), false, nil, nil)}
	schema := aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, nil), aFacetTypes(t)...)
	return aAssess(t, schema, valuedRoot("n", lexical))
}

// wantFacetCharge fails unless res charged exactly one violation under rule,
// whose wrapped cause is the cvc-assertions-valid verdict cvc-datatype-valid
// clause 3 folds into Datatype Valid, and recorded nothing as unevaluated.
func wantFacetCharge(t *testing.T, res *Result, rule xsderr.Rule, why string) {
	t.Helper()
	got := res.Violations()
	if len(got) != 1 || got[0].Rule != rule {
		t.Errorf("%s: Violations() = %v, want one %s charge", why, got, rule)
		return
	}
	if cause, _ := xsderr.RuleOf(errors.Unwrap(got[0])); cause != "cvc-assertions-valid" {
		t.Errorf("%s: the charge's cause carries %q, want cvc-assertions-valid", why, cause)
	}
	if u := res.Unevaluated(); len(u) != 0 {
		t.Errorf("%s: Unevaluated() = %v, want none", why, messages(u))
	}
}

// cvc-attribute clause 3's String Valid reaches the assertions facet of the
// declaration's {type definition} through cvc-datatype-valid clause 3, and
// EVALUATES each {test} the facet evaluator admits (cvc-assertions-valid,
// §4.3.13.3): `$value eq 100` holds for 100 and `$value = 'x'` for "x", and
// each is charged under cvc-attribute, its cause the cvc-assertions-valid
// verdict, for a value it is false of. With xpath.FacetAssertions declining
// every {test}, each satisfied row records an Unevaluated and each charged row
// charges nothing, so all four fail.
func TestAttributeAssertionsFacetIsEvaluated(t *testing.T) {
	for _, c := range []struct{ typ, lexical string }{{"EqHundred", "100"}, {"IsX", "x"}} {
		wantSatisfied(t, aAttributeAssessed(t, c.typ, c.lexical), c.typ+" over "+c.lexical)
	}
	for _, c := range []struct{ typ, lexical string }{{"EqHundred", "5"}, {"IsX", "y"}} {
		wantFacetCharge(t, aAttributeAssessed(t, c.typ, c.lexical), "cvc-attribute", c.typ+" over "+c.lexical)
	}
}

// cvc-assertions-valid clause 1.2 gives a facet's {test} no context item, so
// `. = 'x'` raises err:XPDY0002 whatever the value and the facet is not
// satisfied — charged, never declined (its Note: the expression "will raise a
// dynamic error, which will cause the assertion to be treated as false"). With
// the facet façade's ContextItemExpr arm declining, the row charges nothing and
// fails.
func TestFacetAssertionReadingTheContextItemIsCharged(t *testing.T) {
	wantFacetCharge(t, aAttributeAssessed(t, "Dot", "x"), "cvc-attribute", "Dot over x")
}

// fn:current-date is EVALUATED at the date of the instant Validator.Assess
// reads off the clock (cvc-xpath clauses 5 and 6): over an xs:date attribute a
// complex type's `@n lt current-date()` holds for 2000-01-01 and is charged
// under cvc-assertion for 9999-12-31, and the assertions facet `$value lt
// current-date()` holds and is charged under cvc-attribute alike. With the
// "current-date" arm of xpath's ctaParser.libraryCall removed, every row
// records an Unevaluated instead and fails.
func TestCurrentDateAssertionsAreEvaluated(t *testing.T) {
	schema := aTyped(t, []string{"n", "date"}, "@n lt current-date()")
	wantSatisfied(t, aAssess(t, schema, valuedRoot("n", "2000-01-01")), "@n lt current-date() over 2000-01-01")
	wantAssertionCharge(t, aAssess(t, schema, valuedRoot("n", "9999-12-31")),
		`the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· RootType, whose {test} is "@n lt current-date()",`)

	wantSatisfied(t, aAttributeAssessed(t, "PastDate", "2000-01-01"), "PastDate over 2000-01-01")
	wantFacetCharge(t, aAttributeAssessed(t, "PastDate", "9999-12-31"), "cvc-attribute", "PastDate over 9999-12-31")
}

// cvc-complex-type clause 1.2 reaches the same facet over an element's
// ·initial value·, and charges a value its {test} is false of at the
// CONTAINING element's location — the value is assembled from every character
// run and belongs to none of them.
func TestSimpleContentAssertionsFacetIsEvaluated(t *testing.T) {
	types := aFacetTypes(t)
	schema := aSchema(t, aComplexType(t, nil, xsd.SimpleContent{SimpleType: types[0]}, nil), types...)

	wantSatisfied(t, aAssess(t, schema, cRoot("#42")), "AssertedInt over 42")
	res := aAssess(t, schema, cRoot("#0"))
	wantFacetCharge(t, res, "cvc-complex-type", "AssertedInt over 0")
	if got := res.Violations(); len(got) == 1 && got[0].Loc != loc(1, 1) {
		t.Errorf("charge Loc = %s, want the element's %s", got[0].Loc, loc(1, 1))
	}
}

// A union's ·validating type· is the FIRST member the value is Datatype Valid
// against (dt-active-member), assertions facets included: "dup" fails EntityX's
// `$value = 'x'`, so xs:string validates it, no ·ENTITY value· exists for
// String Valid clause 3 to find undeclared, and the attribute is ·valid·. "x"
// satisfies the facet, so EntityX validates it and clause 3 charges the
// undeclared entity name — the row that shows the first is not vacuous. With
// the facet unevaluated as it was before #2246, EntityX validates "dup" too and
// clause 3 charges it; with xpath.FacetAssertions declining, the dispatch
// stops at EntityX and String Valid declines. Either way the first row fails.
func TestUnionMemberFailingItsFacetYieldsTheValidatingType(t *testing.T) {
	wantSatisfied(t, aAttributeAssessed(t, "EntityXOrString", "dup"), "EntityXOrString over dup")

	res := aAttributeAssessed(t, "EntityXOrString", "x")
	got := res.Violations()
	if len(got) != 1 || got[0].Rule != "cvc-attribute" {
		t.Fatalf("Violations() = %v, want the cvc-attribute charge for the undeclared ·ENTITY value·", got)
	}
	if cause, _ := xsderr.RuleOf(errors.Unwrap(got[0])); cause != ruleCvcSimpleType {
		t.Errorf("the charge's cause carries %q, want %s (String Valid clause 3)", cause, ruleCvcSimpleType)
	}
}

// An assertions facet whose {test} the facet evaluator declines is recorded
// through [walk.decline] under cvc-assertions-valid at the item, never charged,
// at every variety level the cvc-datatype-valid recursion reaches it at
// (PRINCIPLES 12) and at none it does not: a list item whose facet holds
// records nothing while the list's own declined facet is recorded; a union
// records only the member the dispatch REACHED — 42 is decided by AssertedInt,
// whose facet holds, so AssertedStr's is never asked, where "abc" reaches it —
// and then its own facet, compiled with `$value` typed by that ·active basic
// member· (dt-xdmrep clause 4), whose fn:upper-case call declines there as it
// would over AssertedInt itself. The two declined {test}s InRange and UpperX
// are the guard: with [walk.declineAssertions] reporting false they record
// under cvc-attribute instead, and with the evaluator answering AssertionFails
// for a decline they are charged.
func TestDeclinedFacetAssertionsAreRecorded(t *testing.T) {
	for _, c := range []struct {
		name    string
		typ     string
		lexical string
		want    []string
	}{
		{"a range expression", "InRange", "4", []string{"InRange"}},
		{"a function call", "UpperX", "ax", []string{"UpperX"}},
		{"atomic", "AssertedStr", "abc", []string{"AssertedStr"}},
		{"a list item evaluated", "PlainList", "1 2", nil},
		{"the list's own", "AssertedList", "1 2", []string{"AssertedList"}},
		{"a union decided by its first member", "PlainUnion", "42", nil},
		{"the union member reached", "PlainUnion", "abc", []string{"AssertedStr"}},
		{"the union's own", "AssertedUnion", "42", []string{"AssertedUnion"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			wantRecords(t, aAttributeAssessed(t, c.typ, c.lexical), "cvc-assertions-valid", loc(1, 10), c.want...)
		})
	}
}

// An ·initial value· String Valid REJECTS before its assertions stage records
// nothing for the facet: the stage runs after every other facet has accepted
// the value (cvc-datatype-valid clause 3), so a lexical outside the lexical
// space is charged and its assertions are never asked.
func TestRejectedInitialValueReachesNoAssertion(t *testing.T) {
	types := aFacetTypes(t)
	schema := aSchema(t, aComplexType(t, nil, xsd.SimpleContent{SimpleType: types[0]}, nil), types...)

	res := aAssess(t, schema, cRoot("#not an integer"))

	if len(res.Violations()) != 1 {
		t.Fatalf("Violations() = %v, want the clause 1.2 charge", res.Violations())
	}
	if got := res.Unevaluated(); len(got) != 0 {
		t.Fatalf("Unevaluated() = %v, want none", messages(got))
	}
}

// The two rule IDs are never conflated: one element carrying both an
// {assertions} on its complex type and a declined assertions facet on its
// simple {content type} records one site per rule, at the same Loc,
// discriminated by Rule alone. The facet's comes first: cvc-complex-type clause
// 1.2 reaches it over the ·initial value· once the [[children]] are exhausted,
// and clause 6 is evaluated after them, over the partial ·PSVI· clause 1.2 is
// part of (cvc-assertion clause 1.1).
func TestBothRulesRecordAtOneElement(t *testing.T) {
	types := aFacetTypes(t)
	schema := aSchema(t, aComplexType(t, nil, xsd.SimpleContent{SimpleType: types[1]},
		aAssertions("@a = @b")), types...)

	res := aAssess(t, schema, cRoot("#abc"))

	got := res.Unevaluated()
	if len(got) != 2 {
		t.Fatalf("Unevaluated() = %v, want one site per rule", messages(got))
	}
	if got[0].Rule() != "cvc-assertions-valid" || got[1].Rule() != "cvc-assertion" {
		t.Errorf("rules = %q, %q, want cvc-assertions-valid then cvc-assertion",
			got[0].Rule(), got[1].Rule())
	}
}

// An attribute matching no {attribute use} is ·attributed to· the {attribute
// wildcard} instead, and under a strict or a lax wildcard it is assessed
// against the top-level declaration its name ·resolves· to
// ([walk.wildcardAttribute]), whose cvc-attribute clause 3 records a declined
// facet exactly ONCE — wantRecords counts them, so a second recording path
// beside [walk.declaredAttribute] fails it (#1891).
//
// Under ***skip*** nothing is recorded: the item is ·skipped·, no facet of any
// type is reached over its lexical (#1043), and there is no unevaluated
// assertion to report.
func TestWildcardAttributeAssertionsAreRecorded(t *testing.T) {
	d, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, local("n"),
		xsd.TypeDefinitionRef{Name: local("AssertedStr")}, xsd.NewAttributeGlobalScope(), nil, false)
	if err != nil {
		t.Fatalf("building the top-level n declaration: %v", err)
	}
	assess := func(pc xsd.ProcessContents) *Result {
		t.Helper()
		ct, err := xsd.NewComplexType(xsderr.Loc{}, local("RootType"), xsd.QName{}, nil,
			xsd.DerivationRestriction, false, nil, nil, anyWildcard(t, pc), xsd.EmptyContent{}, nil, nil)
		if err != nil {
			t.Fatalf("building RootType: %v", err)
		}
		schema := cSchemaFrom(t, ct, func(b *xsd.SchemaBuilder) {
			aTypes(t, b, aFacetTypes(t)...)
			b.AddAttribute(d)
		})
		return aAssess(t, schema, valuedRoot("n", "abc"))
	}

	wantRecords(t, assess(xsd.ProcessStrict), "cvc-assertions-valid", loc(1, 10), "AssertedStr")
	wantRecords(t, assess(xsd.ProcessLax), "cvc-assertions-valid", loc(1, 10), "AssertedStr")
	if got := assess(xsd.ProcessSkip).Unevaluated(); len(got) != 0 {
		t.Errorf("Unevaluated() = %v, want none: a ·skipped· attribute reaches no assertion site", messages(got))
	}
}

// cvc-complex-type clause 4's String Valid over a ·defaulted attribute·'s
// {lexical form} EVALUATES the assertions facets of the declaration's {type
// definition} (walk.stringValid, xpath.FacetAssertions): `$value > 0` holds for
// a default of "42" and is charged under clause 4, its cause the
// cvc-assertions-valid verdict, for "0"; a {test} the facet evaluator declines
// (`$value = 1 to $value`) is recorded under cvc-assertions-valid at the
// ELEMENT's location, the attribute being absent. With the default decided
// through xsd.ValueSpace's ValidDefault, which evaluates no assertion, all
// three rows record a cvc-complex-type clause 4 decline instead.
func TestDefaultedAttributeAssertionsFacetIsEvaluated(t *testing.T) {
	assess := func(typ, lexical string) *Result {
		dflt := xsd.NewValueConstraint(xsd.ValueDefault, lexical, nil, nil)
		uses := []xsd.AttributeUse{typedUse(t, "n", local(typ), false, nil, &dflt)}
		schema := aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, nil), aFacetTypes(t)...)
		return aAssess(t, schema, &testElement{name: local("root"), loc: loc(1, 1)})
	}

	wantSatisfied(t, assess("AssertedInt", "42"), "AssertedInt over a default of 42")
	wantFacetCharge(t, assess("AssertedInt", "0"), "cvc-complex-type", "AssertedInt over a default of 0")
	wantRecords(t, assess("InRange", "42"), "cvc-assertions-valid", loc(1, 1), "InRange")
}

func TestResultUnevaluatedIsCopied(t *testing.T) {
	// Unevaluated is a window, not a handle, on Violations' terms.
	res := &Result{unevaluated: []Unevaluated{
		newUnevaluated("cvc-assertion", loc(1, 1), "sample")}}
	got := res.Unevaluated()
	got[0] = Unevaluated{}
	if res.Unevaluated()[0] == (Unevaluated{}) {
		t.Error("Unevaluated() shares its backing array with the Result")
	}
	if (&Result{}).Unevaluated() != nil {
		t.Error("Unevaluated() = non-nil for a Result carrying none")
	}
}

// A skipped check must never be mistakable for a verdict: an Unevaluated that
// satisfied error could be joined into a violation list, matched by errors.Is,
// or appended to a Result's violations, turning a check nothing was decided by
// into a false reject.
func TestUnevaluatedIsNotAnError(t *testing.T) {
	u := newUnevaluated("cvc-assertion", loc(1, 1), "sample")
	if _, isError := any(u).(error); isError {
		t.Fatal("Unevaluated satisfies error; it must carry no Error method")
	}
	// The pointer type is asserted separately because Result.Unevaluated()
	// hands back an ADDRESSABLE slice: a func (u *Unevaluated) Error() string
	// would leave the value assertion above clean while &got[0] mixed into a
	// violation list, which is the same false reject by another route.
	if _, isError := any(&u).(error); isError {
		t.Fatal("*Unevaluated satisfies error; it must carry no Error method on either receiver")
	}
}

// aFixedTypes is the pair the fixed-value comparisons read: NonNegative
// restricts xs:integer with `$value ge 0`, which the facet evaluator admits,
// and InRange with `$value = 1 to $value`, which it declines: a range over a
// non-literal operand.
func aFixedTypes(t *testing.T) []*xsd.SimpleType {
	t.Helper()
	return []*xsd.SimpleType{
		aRestriction(t, "NonNegative", integerType(), "$value ge 0"),
		aRestriction(t, "InRange", integerType(), "$value = 1 to $value"),
	}
}

// aFixedAttributeAssessed assesses <root n="lexical"/>, n declared of type typ
// with the fixed {value constraint} fixed.
func aFixedAttributeAssessed(t *testing.T, typ, fixed, lexical string) *Result {
	t.Helper()
	vc := xsd.NewValueConstraint(xsd.ValueFixed, fixed, nil, nil)
	uses := []xsd.AttributeUse{typedUse(t, "n", local(typ), false, &vc, nil)}
	schema := aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, nil), aFixedTypes(t)...)
	return aAssess(t, schema, valuedRoot("n", lexical))
}

// aFixedElementAssessed assesses <root>lexical</root>, root declared of type
// typ with the fixed {value constraint} fixed.
func aFixedElementAssessed(t *testing.T, typ, fixed, lexical string) *Result {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	aTypes(t, b, aFixedTypes(t)...)
	vc := xsd.NewValueConstraint(xsd.ValueFixed, fixed, nil, nil)
	d, err := xsd.NewElementDeclaration(xsderr.Loc{}, local("root"),
		xsd.TypeDefinitionRef{Name: local(typ)}, nil, xsd.NewGlobalScope(),
		&vc, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	b.AddElement(d)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the fixed-element schema: %v", err)
	}
	return aAssess(t, schema, cRoot("#"+lexical))
}

// wantFixedCharge fails unless res charged exactly one violation under rule,
// its message opening with prefix and naming clause, and recorded nothing as
// unevaluated.
func wantFixedCharge(t *testing.T, res *Result, rule xsderr.Rule, prefix, clause, why string) {
	t.Helper()
	got := res.Violations()
	if len(got) != 1 || got[0].Rule != rule {
		t.Fatalf("%s: Violations() = %v, want one %s charge", why, got, rule)
	}
	if !strings.HasPrefix(got[0].Msg, prefix) || !strings.Contains(got[0].Msg, clause) {
		t.Errorf("%s: Msg = %q, want it to open %q and name %s", why, got[0].Msg, prefix, clause)
	}
	if u := res.Unevaluated(); len(u) != 0 {
		t.Errorf("%s: Unevaluated() = %v, want none", why, messages(u))
	}
}

// cvc-attribute clause 4 compares the attribute's ·actual value· with the fixed
// {value} through the type's whole pipeline, assertions facet included
// (cvc-datatype-valid clause 3, value.ConstraintMatches): "+7" and a fixed "7"
// are one xs:integer, so nothing is charged or recorded, and "5" — valid under
// `$value ge 0`, so cvc-attribute clause 3 is satisfied — is a different value,
// charged under clause 4 alone. With value.ConstraintMatches handed an
// evaluator that declines every {test}, both comparisons are undecided, each
// row records a clause 4 Unevaluated and charges nothing, and both fail.
func TestFixedAttributeComparesThroughItsAssertionsFacet(t *testing.T) {
	wantSatisfied(t, aFixedAttributeAssessed(t, "NonNegative", "7", "+7"), "+7 against a fixed 7")
	wantFixedCharge(t, aFixedAttributeAssessed(t, "NonNegative", "7", "5"), "cvc-attribute",
		`the ·actual value· of the attribute n is neither equal nor identical to the {value} of the fixed {value constraint} "7" on its attribute declaration`,
		"cvc-attribute clause 4", "5 against a fixed 7")
}

// cvc-elt clause 5.2.2.2.2 makes the same comparison for an element whose
// ·governing type definition· is simple: "+7" agrees with a fixed "7", and "5"
// is charged under clause 5.2.2.2.2 alone, cvc-type clause 3.1.3 being
// satisfied by `$value ge 0`. With value.ConstraintMatches handed an evaluator
// that declines every {test}, the "5" row records a 5.2.2.2.2 Unevaluated and
// charges nothing, and fails.
func TestFixedElementComparesThroughItsAssertionsFacet(t *testing.T) {
	wantSatisfied(t, aFixedElementAssessed(t, "NonNegative", "7", "+7"), "+7 against a fixed 7")
	wantFixedCharge(t, aFixedElementAssessed(t, "NonNegative", "7", "5"), "cvc-elt",
		`the ·actual value· of the element root is neither equal nor identical to the {value} of the fixed {value constraint} "7"`,
		"cvc-elt clause 5.2.2.2.2", "5 against a fixed 7")
}

// The guard: a {test} the facet evaluator declines leaves the fixed-value
// comparison undecided, and nothing is charged. "4" against a fixed "7" is a
// NOT-same pair, so a decided comparison would charge cvc-elt clause
// 5.2.2.2.2; instead clause 3.1.3 records the declined InRange facet under
// cvc-assertions-valid and the comparison records its own clause 5.2.2.2.2
// decline. With FacetAssertions answering AssertionHolds where it declines,
// the row is charged and fails.
func TestFixedElementWithADeclinedAssertionIsUndecided(t *testing.T) {
	res := aFixedElementAssessed(t, "InRange", "7", "4")
	if got := res.Violations(); len(got) != 0 {
		t.Fatalf("Violations() = %v, want none: a declined {test} decides nothing", got)
	}
	got := res.Unevaluated()
	if len(got) != 2 || got[0].Rule() != "cvc-assertions-valid" || got[1].Rule() != "cvc-elt" ||
		!strings.Contains(got[1].Msg(), "cvc-elt clause 5.2.2.2.2 is undecided") {
		t.Errorf("Unevaluated() = %v, want the InRange facet's cvc-assertions-valid record then the cvc-elt clause 5.2.2.2.2 decline", messages(got))
	}
}
