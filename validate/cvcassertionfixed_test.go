package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive an assertion {test} on <root> over its xs:dateTime
// @d, whose fixed value 2008-01-01T00:00:00 sits on one of two components. On
// the attribute DECLARATION, reached through ref (globalref, and restriction,
// where Root restricts Base, whose assertion it inherits), a mismatch is a
// cvc-attribute clause 4 charge, which makes @d invalid in the partial ·PSVI·
// cvc-assertion clause 1.2 builds (sic-a-outcome), so the {test} reads it
// annotated xs:anySimpleType with an xs:untypedAtomic typed value
// (xpath-datamodel §3.3.1.1, §3.3.1.2). On the attribute USE (localfixed, and
// reffixedonuse, a ref to an unconstrained declaration), a mismatch is a
// cvc-au charge alone, which leaves @d valid and annotated xs:dateTime.
var fxSchemaTexts = map[string]string{
	"globalref": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:attribute name="d" type="xs:dateTime" fixed="2008-01-01T00:00:00"/>
  <xs:complexType name="Root">
    <xs:attribute ref="d"/>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`,
	"restriction": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:attribute name="d" type="xs:dateTime" fixed="2008-01-01T00:00:00"/>
  <xs:complexType name="Base">
    <xs:attribute ref="d"/>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:complexType name="Root">
    <xs:complexContent>
      <xs:restriction base="Base"><xs:attribute ref="d"/></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`,
	"localfixed": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Root">
    <xs:attribute name="d" type="xs:dateTime" fixed="2008-01-01T00:00:00"/>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`,
	"reffixedonuse": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:attribute name="d" type="xs:dateTime"/>
  <xs:complexType name="Root">
    <xs:attribute ref="d" fixed="2008-01-01T00:00:00"/>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`,
}

// fxSchema is the shape's schema text with test as its one assertion.
func fxSchema(t *testing.T, shape, test string) *xsd.Schema {
	t.Helper()
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;").Replace(test)
	return parsedSchema(t, map[string]string{"main.xsd": strings.Replace(fxSchemaTexts[shape], "TEST", escaped, 1)})
}

// fxRules is the rules res charged, in order.
func fxRules(res *Result) []xsderr.Rule {
	var rules []xsderr.Rule
	for _, v := range res.Violations() {
		rules = append(rules, v.Rule)
	}
	return rules
}

// fxWant fails unless res recorded nothing and charged exactly fixed — the
// fixed-value rule, or none — then cvc-assertion where charged, the latter's
// message opening with <root> and the assertion's position, which a swapped
// argument would move.
func fxWant(t *testing.T, res *Result, fixed xsderr.Rule, charged bool) {
	t.Helper()
	if got := res.Unevaluated(); len(got) != 0 {
		t.Fatalf("Unevaluated() = %v, want none: the assertion is evaluated", messages(got))
	}
	var want []xsderr.Rule
	if fixed != "" {
		want = append(want, fixed)
	}
	if charged {
		want = append(want, ruleCvcAssertion)
	}
	got := fxRules(res)
	if strings.Join(rulesText(got), " ") != strings.Join(rulesText(want), " ") {
		t.Fatalf("Violations() = %v, want the rules %v", res.Violations(), want)
	}
	vs := res.Violations()
	if fixed == ruleCvcAttribute && !strings.HasPrefix(vs[0].Msg, "the ·actual value· of the attribute d is neither equal nor identical to the {value} of the fixed {value constraint} \"2008-01-01T00:00:00\" on its attribute declaration, which cvc-attribute clause 4 requires") {
		t.Errorf("Msg = %q, want the cvc-attribute clause 4 charge over d", vs[0].Msg)
	}
	if charged && !strings.HasPrefix(vs[len(vs)-1].Msg, "the element root is not ·valid· with respect to assertion 1 of 1 in the {assertions} of the ·governing type definition· ") {
		t.Errorf("Msg = %q, want it to open with root and assertion 1 of 1", vs[len(vs)-1].Msg)
	}
}

// rulesText is rules as strings.
func rulesText(rules []xsderr.Rule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, string(r))
	}
	return out
}

// fxRoot is <root> at 1:1 carrying d of lexical at 1:10.
func fxRoot(lexical string) *testElement {
	return &testElement{name: local("root"), loc: loc(1, 1),
		attrs: []Attribute{&testAttribute{name: local("d"), value: lexical, loc: loc(1, 10)}}}
}

// An @d whose value differs from its DECLARATION's fixed value is invalid in
// the partial ·PSVI·, so each {test} reads it annotated xs:anySimpleType with
// an xs:untypedAtomic typed value: `not(data(@d) instance of xs:dateTime)` and
// `not(@d instance of attribute(*, xs:dateTime))` hold, `data(@d) instance of
// xs:untypedAtomic` holds, and the two positive dateTime forms are charged —
// beside the cvc-attribute clause 4 charge, which stands. An @d matching it is
// valid and read as its declared xs:dateTime, every verdict flipped and nothing
// charged under clause 4. An @d differing from its USE's fixed value is charged
// under cvc-au alone and stays valid, read as xs:dateTime, mismatched or not.
//
// With walk.annotations answering the static closure whatever the verdict, the
// rows reading a mismatched globalref and restriction @d fail and no other;
// with fixedAgreement's charge reporting attributeValid, the same rows fail;
// with matchedAttribute returning the cvc-au call's outcome as the verdict, the
// rows reading a mismatched localfixed and reffixedonuse @d fail and no other;
// and with walk.annotations overriding every name but the invalid ones, every
// row fails.
func TestAssertionReadsAFixedValueInvalidAttributeAsAnySimpleType(t *testing.T) {
	tests := []struct {
		test     string
		declared bool // whether it holds over @d annotated xs:dateTime
	}{
		{"not(data(@d) instance of xs:dateTime)", false},
		{"not(@d instance of attribute(*, xs:dateTime))", false},
		{"data(@d) instance of xs:dateTime", true},
		{"@d instance of attribute(*, xs:dateTime)", true},
		{"data(@d) instance of xs:untypedAtomic", false},
	}
	for _, shape := range []struct {
		name  string
		fixed xsderr.Rule // the rule a mismatch is charged under
	}{
		{"globalref", ruleCvcAttribute},
		{"restriction", ruleCvcAttribute},
		{"localfixed", ruleCvcAu},
		{"reffixedonuse", ruleCvcAu},
	} {
		for _, tc := range tests {
			t.Run(shape.name+"/mismatch/"+tc.test, func(t *testing.T) {
				res := aAssess(t, fxSchema(t, shape.name, tc.test), fxRoot("2009-01-01T00:00:00"))
				holds := tc.declared
				if shape.fixed == ruleCvcAttribute {
					holds = !tc.declared
				}
				fxWant(t, res, shape.fixed, !holds)
			})
			t.Run(shape.name+"/match/"+tc.test, func(t *testing.T) {
				res := aAssess(t, fxSchema(t, shape.name, tc.test), fxRoot("2008-01-01T00:00:00"))
				fxWant(t, res, "", !tc.declared)
			})
		}
	}
}

// The xs:untypedAtomic value an invalid @d is read as is its [schema normalized
// value], which xs:dateTime's whiteSpace collapse makes of the ·initial value·
// (xpath-datamodel §3.3.1.2), and never the raw characters: `@d =
// '2009-01-01T00:00:00'` holds over " 2009-01-01T00:00:00 ", cast to xs:string
// for the comparison (xpath20.md §3.5.2), and is charged over "2010-…". With
// assertionTyped yielding the ·initial value· unnormalized, the padded row
// fails.
func TestAssertionReadsAnInvalidAttributesNormalizedValue(t *testing.T) {
	const test = "@d = '2009-01-01T00:00:00'"
	fxWant(t, aAssess(t, fxSchema(t, "globalref", test), fxRoot(" 2009-01-01T00:00:00 ")), ruleCvcAttribute, false)
	fxWant(t, aAssess(t, fxSchema(t, "globalref", test), fxRoot("2010-01-01T00:00:00")), ruleCvcAttribute, true)
}

// A cvc-attribute clause 4 comparison this package declines leaves @n neither
// known valid nor known invalid, so whether the partial ·PSVI· annotates it
// with its declared type or xs:anySimpleType is undecided, and every assertion
// of <root> is DECLINED rather than read either way: here the declaration's
// fixed value 37 over xs:anySimpleType, under a backend that leaves xs:string,
// one member of its lexical mapping union, unmapped (fixedAgreement). With
// walk.compileAssertions ignoring an attributeNotKnown verdict, the assertion
// is evaluated and this test fails.
func TestAssertionOverAnUndecidedFixedValueIsDeclined(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "37", nil, nil)
	uses := []xsd.AttributeUse{typedUse(t, "n", icBuiltin("anySimpleType"), false, &fixed, nil)}
	schema := aSchema(t, aComplexType(t, uses, xsd.EmptyContent{}, aAssertions("not(@n)")))
	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("string")), schema, valuedRoot("n", "36"))
	wantSilence(t, got, "an undecided comparison charges nothing")
	wantDeclines(t, undecided,
		Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: "cvc-attribute clause 4"},
		Unevaluated{rule: ruleCvcAssertion, loc: loc(1, 1), msg: "the attribute n of the element root is not known to be ·valid· or invalid"})
}

// An @d of a union type whose value differs from its declaration's fixed one
// is invalid by cvc-attribute clause 4 and has an ·actual value·, but the
// union carries no single whiteSpace facet, so its [schema normalized value],
// the xs:untypedAtomic the {test} reads it as, is one this package does not
// read (walk.assertionTyped): the assertion is DECLINED for that reason, never
// for a missing ·actual value·, beside the clause 4 charge, which stands.
func TestAssertionOverAnInvalidUnionAttributeDeclines(t *testing.T) {
	schema := parsedSchema(t, map[string]string{"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="U"><xs:union memberTypes="xs:dateTime xs:date"/></xs:simpleType>
  <xs:attribute name="d" type="U" fixed="2008-01-01T00:00:00"/>
  <xs:complexType name="Root">
    <xs:attribute ref="d"/>
    <xs:assert test="not(data(@d) instance of xs:dateTime)"/>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`})
	res := aAssess(t, schema, fxRoot("2009-01-01T00:00:00"))
	smDeclined(t, res, "the attribute d of the element root is invalid by cvc-attribute clause 4, so the data model instance cvc-assertion clause 1 builds annotates it xs:anySimpleType with its [schema normalized value] as typed value, which this package does not read: its declaration's {type definition} has no single whiteSpace facet this package reads")
	if got := fxRules(res); len(got) != 1 || got[0] != ruleCvcAttribute || !strings.HasPrefix(res.Violations()[0].Msg, "the ·actual value· of the attribute d is neither equal nor identical to the {value} of the fixed {value constraint} \"2008-01-01T00:00:00\" on its attribute declaration, which cvc-attribute clause 4 requires") {
		t.Errorf("Violations() = %v, want the cvc-attribute clause 4 charge over d alone", res.Violations())
	}
}

// fxBelowSchemaText is <root>, of Root, over <a> children of A, which has
// simple content over xs:integer and refs p, an xs:decimal whose DECLARATION
// fixes it to 5, with TEST as Root's one assertion.
const fxBelowSchemaText = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:attribute name="p" type="xs:decimal" fixed="5"/>
  <xs:complexType name="A">
    <xs:simpleContent>
      <xs:extension base="xs:integer"><xs:attribute ref="p"/></xs:extension>
    </xs:simpleContent>
  </xs:complexType>
  <xs:complexType name="Root">
    <xs:sequence><xs:element name="a" type="A" maxOccurs="unbounded"/></xs:sequence>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`

// fxA is an <a> at line holding 1 and carrying p of lexical.
func fxA(line int, lexical string) Child {
	return ccNode(local("a"), line, []Attribute{ccAttr(local("p"), lexical, line)},
		TextChild(&testText{data: "1", loc: loc(line, 9)}))
}

// A {test} reading p below <root> — through a path, `sum(a/@p)` (the Tally,
// walk.tallyAttributes), or through a range variable bound to each a,
// `$v/@p` (walk.childAttributes) — is compiled against p's STATIC type, and
// one compiled tree reads every a's p. An a whose p differs from its
// declaration's fixed 5 is invalid in the partial ·PSVI·, annotated
// xs:anySimpleType, which that tree cannot read, so every assertion of <root>
// is DECLINED, beside the cvc-attribute clause 4 charge, rather than read p as
// an xs:decimal; a p of 5 is read and the assertion holds. With
// walk.attributesRead's verdict check removed, both mismatch rows fail and
// neither match row; with walk.tallyAttributes handed no verdicts, the sum
// mismatch row alone fails; and with walk.childAttributes handed none, the
// `$v/@p` mismatch row alone fails.
func TestAssertionBelowAFixedValueInvalidAttributeDeclines(t *testing.T) {
	const why = "cvc-attribute charged or declined its fixed-value agreement, so it is not known ·valid·"
	for _, test := range []string{
		"sum(a/@p) ge 0",
		"every $v in a satisfies 0 le $v/@p",
	} {
		escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;").Replace(test)
		schema := parsedSchema(t, map[string]string{"main.xsd": strings.Replace(fxBelowSchemaText, "TEST", escaped, 1)})
		t.Run(test+"/mismatch", func(t *testing.T) {
			res := aAssess(t, schema, ccRootNamed("root", fxA(2, "5"), fxA(3, "6")))
			smDeclined(t, res, "the attribute p of the element a at "+loc(3, 3).String()+", in the subtree of the element root, whose typed value a {test} reads, has no typed value for the data model instance cvc-assertion clause 1 builds: "+why)
			if got := fxRules(res); len(got) != 1 || got[0] != ruleCvcAttribute {
				t.Errorf("Violations() = %v, want the cvc-attribute clause 4 charge alone", res.Violations())
			}
		})
		t.Run(test+"/match", func(t *testing.T) {
			wantSatisfied(t, aAssess(t, schema, ccRootNamed("root", fxA(2, "5"), fxA(3, "5"))), test)
		})
	}
}

// ccRootNamed is the validation root named name at 1:1 over kids.
func ccRootNamed(name string, kids ...Child) *testElement {
	return &testElement{name: local(name), loc: loc(1, 1), kids: kids}
}
