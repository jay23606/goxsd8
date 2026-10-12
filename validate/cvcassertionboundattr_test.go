package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive an assertion {test} that reads the attributes of
// <root>'s <x> children through a quantifier's range variable bound to each,
// the form of ibmData/mixed/assertions/test15.xsd:17: the walk keeps each x
// with the typed values of the attributes a {test} reads (walk.keepChild,
// walk.childAttributes), and its value only where a {test} reads that too.
// XType has simple content over xs:integer, an AttrType a, a d defaulting to
// val5, and a lax ##local attribute wildcard.
const baSchemaText = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="AttrType">
    <xs:restriction base="xs:string"><xs:pattern value="val[0-9]+"/></xs:restriction>
  </xs:simpleType>
  <xs:complexType name="XType">
    <xs:simpleContent>
      <xs:extension base="xs:integer">
        <xs:attribute name="a" type="AttrType"/>
        <xs:attribute name="d" type="AttrType" default="val5"/>
        <xs:anyAttribute namespace="##local" processContents="lax"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>
  <xs:complexType name="Root">
    <xs:sequence>
      <xs:element name="x" type="XType" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
    <xs:assert test="TEST"/>
  </xs:complexType>
  <xs:element name="root" type="Root"/>
</xs:schema>`

// baTest15 is test15.xsd:17's {test}.
const baTest15 = "every $x in x[position() lt last()] satisfies number($x/@a/substring-after(.,'val')) lt number($x/following-sibling::x[1]/@a/substring-after(.,'val'))"

// baSchema is baSchemaText with test as Root's one assertion.
func baSchema(t *testing.T, test string) *xsd.Schema {
	t.Helper()
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;").Replace(test)
	return parsedSchema(t, map[string]string{"main.xsd": strings.Replace(baSchemaText, "TEST", escaped, 1)})
}

// baX is an <x> at line holding text and carrying attrs, alternating names and
// values.
func baX(line int, text string, attrs ...string) Child {
	var as []Attribute
	for i := 0; i+1 < len(attrs); i += 2 {
		as = append(as, ccAttr(local(attrs[i]), attrs[i+1], line))
	}
	return ccNode(local("x"), line, as, TextChild(&testText{data: text, loc: loc(line, 9)}))
}

// baRoot is the validation root <root> at 1:1 over kids.
func baRoot(kids ...Child) *testElement {
	return &testElement{name: local("root"), loc: loc(1, 1), kids: kids}
}

// baEvaluated fails unless res recorded no Unevaluated and charged
// cvc-assertion exactly where charged says — whatever else res charged.
func baEvaluated(t *testing.T, res *Result, charged bool) {
	t.Helper()
	if got := res.Unevaluated(); len(got) != 0 {
		t.Fatalf("Unevaluated() = %v, want none: the assertion is evaluated", messages(got))
	}
	found := false
	for _, v := range res.Violations() {
		found = found || v.Rule == ruleCvcAssertion
	}
	if found != charged {
		t.Errorf("Violations() = %v, cvc-assertion charged %v, want %v", res.Violations(), found, charged)
	}
}

// The walk keeps each x with the a its {test} reads, so test15's {test} is
// satisfied over val1 val2 val3 and val2 val10 and charged over val1 val3 val2;
// a ·defaulted· d is kept as a carried one is, so `'5' =
// $x/@d/substring-after(., 'val')` holds over an x carrying no d and is charged
// over one carrying val6. An attribute no {test} reads is not kept, so the lax
// zz, which matches no {attribute use}, declines nothing. And an x whose own
// value the walk cannot give — "abc", no xs:integer, which its own
// cvc-datatype-valid charge makes invalid — declines nothing either, where the
// {test} reads its attributes alone: its value is never asked
// (walk.keepChild). With walk.childAttributes returning no attribute, the
// rows "val1 val2 val3", "val2 val10", "a ·defaulted· d" and "an unread zz"
// fail; with its ·defaulted· loop skipped, "a ·defaulted· d" alone fails; with
// its filter keeping every carried attribute, "an unread zz" alone fails; and
// with walk.keepChild asking every kept child its value, the "abc" check alone
// fails here.
func TestAssertionReadsBoundChildAttributes(t *testing.T) {
	const defaulted = "every $x in x satisfies '5' = $x/@d/substring-after(., 'val')"
	for _, tc := range []struct {
		why, test string
		root      *testElement
		charged   bool
	}{
		{"val1 val2 val3", baTest15, baRoot(baX(2, "2", "a", "val1"), baX(3, "4", "a", "val2"), baX(4, "6", "a", "val3")), false},
		{"val1 val3 val2", baTest15, baRoot(baX(2, "2", "a", "val1"), baX(3, "4", "a", "val3"), baX(4, "6", "a", "val2")), true},
		{"val2 val10", baTest15, baRoot(baX(2, "2", "a", "val2"), baX(3, "4", "a", "val10")), false},
		{"a ·defaulted· d", defaulted, baRoot(baX(2, "2")), false},
		{"a carried d", defaulted, baRoot(baX(2, "2", "d", "val6")), true},
		{"an unread zz", baTest15, baRoot(baX(2, "2", "a", "val1", "zz", "q"), baX(3, "4", "a", "val2")), false},
	} {
		t.Run(tc.why, func(t *testing.T) {
			baEvaluated(t, aAssess(t, baSchema(t, tc.test), tc.root), tc.charged)
		})
	}
	res := aAssess(t, baSchema(t, baTest15), baRoot(baX(2, "abc", "a", "val1"), baX(3, "4", "a", "val2")))
	baEvaluated(t, res, false)
	if len(res.Violations()) == 0 {
		t.Errorf("Violations() = none, want the x holding abc charged")
	}
}

// Where a {test} reads the bound child's value as well as its attributes, the
// value is read on walk.childValue's terms, so an x holding "abc" declines the
// assertion; and an a with no ·actual value· under AttrType declines it on
// walk.attributeBelow's, whichever x carries it. With walk.childAttributes
// returning no attribute, the second row is evaluated and charged.
func TestAssertionBoundChildAttributeDeclines(t *testing.T) {
	for _, tc := range []struct {
		why, test, want string
		root            *testElement
	}{
		{"a value read beside the attributes", "every $x in x satisfies $x gt number($x/@a/substring-after(., 'val'))",
			"a violation or an unevaluated check was recorded for it",
			baRoot(baX(2, "abc", "a", "val1"))},
		{"an a with no ·actual value·", baTest15,
			`"bad" has no ·actual value· under its type AttrType`,
			baRoot(baX(2, "2", "a", "val1"), baX(3, "4", "a", "bad"))},
	} {
		t.Run(tc.why, func(t *testing.T) {
			smDeclined(t, aAssess(t, baSchema(t, tc.test), tc.root), tc.want)
		})
	}
}
