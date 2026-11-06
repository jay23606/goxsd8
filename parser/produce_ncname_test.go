package parser_test

import (
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// TestProduceNonNCNameDeclarationNameRejected pins, END TO END from a schema
// DOCUMENT, that a declaration whose name attribute is not in the ·lexical
// space· of xs:NCName is rejected charged cvc-datatype-valid (Datatypes §4.1.4)
// at the declaration's name attribute — never registered as a {name} nobody can
// reference, which is what a colonized or digit-initial name became before #632.
//
// One row per CODE PATH into declarationName, not per bad name: the six
// top-level kinds routed through topLevelName, the <notation> that is not, the
// two local declaration forms, the prohibited <attribute> that maps to no
// component at all (§3.4.2.4's Note) yet is still a name in a schema document,
// and the identity constraint, whose name= form declares a component of the
// schema (§3.17.1) without being a top-level item of the document at all.
// Reverting any one call site leaves its row failing.
func TestProduceNonNCNameDeclarationNameRejected(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2). Every body starts on
	// line 2 of the wrapped document and puts the offending name attribute on the
	// line named by wantLine, so the charged position is pinned exactly.
	cases := []struct {
		name     string
		body     string
		wantLine int
	}{
		{
			name:     `top-level <element> name="a:b"`,
			body:     "\n" + `<xs:element name="a:b" type="xs:string"/>`,
			wantLine: 2,
		},
		{
			name:     `top-level <attribute> name="0"`,
			body:     "\n" + `<xs:attribute name="0" type="xs:string"/>`,
			wantLine: 2,
		},
		{
			name:     `top-level <complexType> name="1foo"`,
			body:     "\n" + `<xs:complexType name="1foo"><xs:sequence/></xs:complexType>`,
			wantLine: 2,
		},
		{
			name:     `top-level <group> name="-foo"`,
			body:     "\n" + `<xs:group name="-foo"><xs:sequence/></xs:group>`,
			wantLine: 2,
		},
		{
			name:     `top-level <attributeGroup> name=".x"`,
			body:     "\n" + `<xs:attributeGroup name=".x"/>`,
			wantLine: 2,
		},
		{
			// The LEXICAL half of the <simpleType> arm, which stayed
			// declarationName's when #523 routed that arm through topLevelName:
			// "nsk:Test" is not empty, so the name reaches this check first and
			// takes the rule verdict rather than the rule-less grammar fault.
			name: `top-level <simpleType> name="nsk:Test"`,
			body: "\n" + `<xs:simpleType name="nsk:Test">` + "\n" +
				`<xs:restriction base="xs:string"/></xs:simpleType>`,
			wantLine: 2,
		},
		{
			// Outside topLevelName: produceNotation builds the component directly.
			name:     `<notation> name="foo:"`,
			body:     "\n" + `<xs:notation name="foo:" system="viewer.exe"/>`,
			wantLine: 2,
		},
		{
			// produceLocalElement. This is the path xsd.NewElementDeclaration's
			// e-props-correct clause 1 guard does NOT reach: that guard tests
			// emptiness, and ":bar" is not empty.
			name: `local <element> name=":bar"`,
			body: "\n" + `<xs:complexType name="CT"><xs:sequence>` + "\n" +
				`<xs:element name=":bar" type="xs:string"/>` + "\n" +
				`</xs:sequence></xs:complexType>`,
			wantLine: 3,
		},
		{
			// produceLocalAttribute, the a-props-correct clause 1 counterpart.
			name: `local <attribute> name="a:b:b"`,
			body: "\n" + `<xs:complexType name="CT"><xs:sequence/>` + "\n" +
				`<xs:attribute name="a:b:b" type="xs:string"/>` + "\n" +
				`</xs:complexType>`,
			wantLine: 3,
		},
		{
			// prohibitedAttributeNames: §3.4.2.4 clause 3.2.2's input, where the
			// <attribute> corresponds to no component and so has no constructor
			// guard behind it at all.
			name: `prohibited local <attribute> name="1x"`,
			body: "\n" + `<xs:complexType name="Base"><xs:sequence/></xs:complexType>` + "\n" +
				`<xs:complexType name="CT"><xs:complexContent>` + "\n" +
				`<xs:restriction base="Base"><xs:sequence/>` + "\n" +
				`<xs:attribute name="1x" use="prohibited"/>` + "\n" +
				`</xs:restriction></xs:complexContent></xs:complexType>`,
			wantLine: 5,
		},
		{
			// produceIdentityConstraint, on the name= arm of
			// src-identity-constraint clause 1's fork. Charged at the <unique>'s
			// own position, not the host <element>'s, and not the empty-name
			// c-props-correct clause 1 xsd.NewIdentityConstraint keeps.
			name: `<unique> name="a:b"`,
			body: "\n" + `<xs:element name="root">` + "\n" +
				`<xs:unique name="a:b"><xs:selector xpath="."/><xs:field xpath="@a"/></xs:unique>` + "\n" +
				`</xs:element>`,
			wantLine: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", tc.body))
			assertRule(t, err, "cvc-datatype-valid")
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no position, want %s:%d:col (E3)", err, produceURI, tc.wantLine)
			}
			if loc.URI != produceURI || loc.Line != tc.wantLine || loc.Col == 0 {
				t.Fatalf("position = %s:%d:%d, want the offending declaration at %s:%d with a column",
					loc.URI, loc.Line, loc.Col, produceURI, tc.wantLine)
			}
		})
	}
}

// TestProduceEmptyIdentityConstraintNameChargedProps pins the other half of the
// split declarationName leaves at the identity constraint: name="" is NOT the
// lexical fault, even though the empty string is outside xs:NCName's ·lexical
// space· too. It stays xsd.NewIdentityConstraint's c-props-correct clause 1
// (§3.11.6.1), against §3.11.1's Required {name} — the verdict the W3C suite's
// idA031/idB031/idC031 already take.
func TestProduceEmptyIdentityConstraintNameChargedProps(t *testing.T) {
	_, err := produce(t, wrap("", `<xs:element name="root">`+
		`<xs:unique name=""><xs:selector xpath="."/><xs:field xpath="@a"/></xs:unique>`+
		`</xs:element>`))
	assertRule(t, err, "c-props-correct")
}

// TestProduceIdentityConstraintRefResolvesAgainstNormalizedIndex pins the
// pre-scan index (indexIdentityConstraint) and the definition's own {name}
// (produceIdentityConstraint) as minted by the SAME declarationName call: a
// <key ref="k"> resolves to a definition written name=" k ", whose ·actual
// value· xs:NCName's whiteSpace = collapse (§3.4.7.1) normalizes to "k".
// Keying the index on the raw attribute instead leaves the reference charged
// src-resolve clause 1.7 (§3.17.6.2) against a definition that is right there,
// and builds a twin component under the raw name if it ever did resolve.
func TestProduceIdentityConstraintRefResolvesAgainstNormalizedIndex(t *testing.T) {
	s, err := produce(t, wrap("", `<xs:element name="root">`+
		`<xs:key name=" k "><xs:selector xpath="a"/><xs:field xpath="@b"/></xs:key>`+
		`</xs:element>`+
		`<xs:element name="other"><xs:key ref="k"/></xs:element>`))
	if err != nil {
		t.Fatalf("Produce: %v, want <key ref=\"k\"> to resolve to the definition named \" k \"", err)
	}
	ics := s.IdentityConstraints()
	if len(ics) != 1 || ics[0].Name() != (xsd.QName{Local: "k"}) {
		t.Fatalf("{identity-constraint definitions} = %v, want exactly one definition named {,k}", ics)
	}
}

// TestProduceXmlnsAttributeNameChargedNCName pins that an <attribute> named
// "xmlns:" or "xmlns:a" is charged the NCName lexical rule and NOT no-xmlns
// (§3.2.6.3). no-xmlns governs the bare string "xmlns", which IS an NCName; its
// own Note derives the xmlns:* prohibition FROM the NCName constraint, so a
// colonized name fails the lexical rule before no-xmlns could apply, and
// charging both would double-charge one fault (STYLE E2).
func TestProduceXmlnsAttributeNameChargedNCName(t *testing.T) {
	for _, name := range []string{"xmlns:", "xmlns:a"} {
		t.Run(name, func(t *testing.T) {
			_, err := produce(t, wrap("", `<xs:attribute name="`+name+`" type="xs:string"/>`))
			assertRule(t, err, "cvc-datatype-valid")
		})
	}
}

// TestProduceRedefiningNonNCNameRejected pins the <redefine> child, the one
// declaration-name path that mints a top-level {name} without passing through
// run's dispatch: produceRedefinition took the name straight off the entry key
// before #632. The lexical fault is charged ahead of src-expredef's closing
// requirement, so the redefined document's matching <group a:b> — which is
// present here, recorded as the original — does not change the verdict.
func TestProduceRedefiningNonNCNameRejected(t *testing.T) {
	docs := map[string]string{
		"main.xsd": wrap("", `<xs:redefine schemaLocation="base.xsd">`+"\n"+
			`<xs:group name="a:b"><xs:sequence/></xs:group>`+
			`</xs:redefine>`),
		"base.xsd": wrap("", `<xs:group name="a:b"><xs:sequence/></xs:group>`),
	}
	_, err := parseMap(t, "main.xsd", docs)
	assertRule(t, err, "cvc-datatype-valid")
}

// TestProduceDeclarationNameWhiteSpaceCollapsed pins the ·actual value· of a
// name attribute as the whiteSpace-normalized one: xs:NCName carries whiteSpace
// = collapse (Datatypes §3.4.7.1), so <element name="sub2-elem "> declares
// "sub2-elem" and is VALID. Rejecting the raw lexical instead would false-reject
// a schema the W3C suite declares valid for exactly this reason (addB193).
func TestProduceDeclarationNameWhiteSpaceCollapsed(t *testing.T) {
	s, err := produce(t, wrap("", "\n"+
		`<xs:element name="&#x9;sub2-elem&#xA; " type="xs:string"/>`))
	if err != nil {
		t.Fatalf("Produce: %v, want the collapsed name to be a valid xs:NCName", err)
	}
	if _, ok := s.Element(xsd.QName{Local: "sub2-elem"}); !ok {
		t.Fatalf("element {,sub2-elem} not declared; the name attribute's surrounding whitespace was not normalized away")
	}
}

// TestProduceNCNameDeclarationNamesAccepted guards the check against
// over-rejection: every character class the NCName production admits beyond a
// bare ASCII letter — '_' and ':'-free punctuation in a non-initial position, a
// digit in a non-initial position, and a non-ASCII NameStartChar — still
// declares a component.
func TestProduceNCNameDeclarationNamesAccepted(t *testing.T) {
	for _, name := range []string{"_foo.bar-1", "é", "A0"} {
		t.Run(name, func(t *testing.T) {
			s, err := produce(t, wrap("", `<xs:element name="`+name+`" type="xs:string"/>`))
			if err != nil {
				t.Fatalf("Produce: %v, want %q accepted as an xs:NCName", err, name)
			}
			if _, ok := s.Element(xsd.QName{Local: name}); !ok {
				t.Fatalf("element {,%s} not declared", name)
			}
		})
	}
}
