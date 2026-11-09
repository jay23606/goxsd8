package parser_test

import (
	"fmt"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// These tests pin the two mapping stages §4.1.4 fixes for the four
// keyword-enumeration attributes of the schema for schema documents that decide
// {required} and {target namespace} — use=, form=, elementFormDefault and
// attributeFormDefault — in the order it fixes them. All four are declared as
// xs:NMTOKEN restrictions (use= inline, the other three as xs:formChoice,
// xmlschema11-1.md:4685 and :4478), NMTOKEN fixes whiteSpace to collapse
// (§3.4.4.1), and the facet is applied BEFORE lexical-space membership is
// tested, so use=" required " is the value required and form=" qualified " is
// the value qualified.
//
// Both directions bite. A padded literal must be READ — reverting the trim mints
// {required} = false and a name in NO namespace on a schema the spec accepts —
// and a literal outside the enumeration must be CHARGED rather than read as the
// declared default, since §4.1.4 states no fallback clause (#1328).

// TestProduceUseAndFormPaddedActualValue reads each of the four attributes with
// §4.3.6 whitespace on both sides of the literal and asserts the property it
// maps. Every row produces the WRONG component under a raw compare, so each
// check is an inequality a revert breaks.
func TestProduceUseAndFormPaddedActualValue(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2).
	cases := []struct {
		name  string
		doc   string
		check func(*testing.T, *xsd.Schema)
	}{
		{
			// §3.2.2.2's {required}: "true if the <attribute> element has use =
			// required, otherwise false" — a comparison of the ·actual value·
			// (key-vv), which a raw compare reads as false.
			name: `use=" required " on a local <attribute>`,
			doc: wrap("urn:x", `<xs:complexType name="T"><xs:sequence/>`+
				`<xs:attribute name="a" type="xs:string" use=" required "/></xs:complexType>`),
			check: func(t *testing.T, s *xsd.Schema) {
				uses := topComplexTypeIn(t, s, xq("T")).AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("{attribute uses} has %d entries, want 1", len(uses))
				}
				if !uses[0].Required() {
					t.Error("{required} = false, want true — use=\" required \" is the ·actual value· required")
				}
			},
		},
		{
			// The prohibited half of the same read: §3.2.2's bullet list makes a
			// prohibited <attribute> map to no component at all, so the padded form
			// must build NOTHING where a raw compare builds an optional use.
			name: `use=" prohibited " on a local <attribute> builds no use`,
			doc: wrap("urn:x", `<xs:complexType name="T"><xs:sequence/>`+
				`<xs:attribute name="a" type="xs:string" use=" prohibited "/></xs:complexType>`),
			check: func(t *testing.T, s *xsd.Schema) {
				if uses := topComplexTypeIn(t, s, xq("T")).AttributeUses(); len(uses) != 0 {
					t.Errorf("{attribute uses} has %d entries, want none — §3.2.2 maps a prohibited <attribute> to no component", len(uses))
				}
			},
		},
		{
			// src-attribute clause 2 reads the same token: "If default and use are
			// both present, use must have the ·actual value· optional". Raw, the
			// padded literal is not "optional" and the schema is FALSELY REJECTED.
			name: `use=" optional " with default= is accepted (src-attribute clause 2)`,
			doc: wrap("urn:x", `<xs:complexType name="T"><xs:sequence/>`+
				`<xs:attribute name="a" type="xs:string" default="d" use=" optional "/></xs:complexType>`),
			check: func(t *testing.T, s *xsd.Schema) {
				if !hasAttrUse(topComplexTypeIn(t, s, xq("T")).AttributeUses(), "a") {
					t.Error("{attribute uses} is missing a, want the optional use built")
				}
			},
		},
		{
			// §3.2.2.2 {target namespace} case 2.1: form = qualified takes the
			// ancestor <schema>'s targetNamespace. Raw, the name is minted in NO
			// namespace, which then false-rejects instances downstream.
			name: `form=" qualified " on a local <attribute>`,
			doc: wrap("urn:x", `<xs:complexType name="T"><xs:sequence/>`+
				`<xs:attribute name="a" type="xs:string" form=" qualified "/></xs:complexType>`),
			check: func(t *testing.T, s *xsd.Schema) {
				uses := topComplexTypeIn(t, s, xq("T")).AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("{attribute uses} has %d entries, want 1", len(uses))
				}
				if got := uses[0].DeclarationName(); got != xq("a") {
					t.Errorf("{name} = %s, want %s — form=\" qualified \" is case 2.1", got, xq("a"))
				}
			},
		},
		{
			// §3.3.2.3 dcl.elt.local states the identical case 2.1 for elements.
			name: `form=" qualified " on a local <element>`,
			doc: wrap("urn:x", `<xs:complexType name="T"><xs:sequence>`+
				`<xs:element name="a" type="xs:string" form=" qualified "/></xs:sequence></xs:complexType>`),
			check: func(t *testing.T, s *xsd.Schema) {
				group := groupTermOf(t, elementContentOf(t, s, xq("T")).Particle)
				if got := elementTermOf(t, group.Particles()[0]).Name(); got != xq("a") {
					t.Errorf("{name} = %s, want %s", got, xq("a"))
				}
			},
		},
		{
			// Case 2.2, the *FormDefault arm: the eager checkFormDefaults pass
			// admits a padded-but-valid value, so the COMPARE localTargetNS makes
			// against the same literal has to read it too — otherwise the eager
			// check accepts the document and the declaration is still minted
			// unqualified.
			name: `elementFormDefault=" qualified " on <schema>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x" elementFormDefault=" qualified ">
<xs:complexType name="T"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
</xs:schema>`,
			check: func(t *testing.T, s *xsd.Schema) {
				group := groupTermOf(t, elementContentOf(t, s, xq("T")).Particle)
				if got := elementTermOf(t, group.Particles()[0]).Name(); got != xq("a") {
					t.Errorf("{name} = %s, want %s — case 2.2 reads the *FormDefault ·actual value·", got, xq("a"))
				}
			},
		},
		{
			name: `attributeFormDefault=" qualified " on <schema>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x" attributeFormDefault=" qualified ">
<xs:complexType name="T"><xs:sequence/><xs:attribute name="a" type="xs:string"/></xs:complexType>
</xs:schema>`,
			check: func(t *testing.T, s *xsd.Schema) {
				uses := topComplexTypeIn(t, s, xq("T")).AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("{attribute uses} has %d entries, want 1", len(uses))
				}
				if got := uses[0].DeclarationName(); got != xq("a") {
					t.Errorf("{name} = %s, want %s", got, xq("a"))
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := produce(t, tc.doc)
			if err != nil {
				t.Fatalf("Produce rejected a document whose only oddity is §4.3.6 padding: %v", err)
			}
			tc.check(t, s)
		})
	}
}

// TestProduceProhibitedUsePaddedExcludesInheritedAttributeUse is the second half
// of use=" prohibited ", the one no count of a type's own uses can see: §3.4.2.4
// clause 3.2.2 (att-prohibited) excludes from the inherited uses "the expanded
// name of the {attribute declaration} of what would have been an attribute use
// corresponding to an <attribute> [child], if the <attribute> had not had use =
// prohibited".
//
// prohibitedAttributeNames computes that exclusion set, and it read use= raw and
// independently of produceAttributeUse until #1328. With only the mapping half
// fixed, the padded form builds no new component AND fails to exclude, so the
// base's {}w use is silently reinstated by the finalize-time fold — the
// restriction ends up with exactly the use it prohibits.
//
// The pair is the assertion: the same document with no <attribute> child at all
// inherits {}w, so the zero-use half can only come from the exclusion.
func TestProduceProhibitedUsePaddedExcludesInheritedAttributeUse(t *testing.T) {
	const doc = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="base"><xs:sequence/>
<xs:attribute name="w" type="xs:string"/>
</xs:complexType>
<xs:complexType name="T"><xs:complexContent><xs:restriction base="tns:base"><xs:sequence/>
%s
</xs:restriction></xs:complexContent></xs:complexType>
</xs:schema>`
	s, err := produce(t, fmt.Sprintf(doc, `<xs:attribute name="w" use=" prohibited "/>`))
	if err != nil {
		t.Fatalf("Produce rejected the padded prohibited form: %v", err)
	}
	if uses := topComplexTypeIn(t, s, xq("T")).AttributeUses(); len(uses) != 0 {
		t.Fatalf("{attribute uses} has %d entries, want the base's {}w excluded by clause 3.2.2", len(uses))
	}
	s, err = produce(t, fmt.Sprintf(doc, ""))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if uses := topComplexTypeIn(t, s, xq("T")).AttributeUses(); len(uses) != 1 {
		t.Fatalf("{attribute uses} has %d entries, want the base's {}w inherited when nothing prohibits it", len(uses))
	}
}

// TestProduceUseAndFormOutOfEnumeration charges the other direction: a literal
// outside the enumeration its declared type restricts xs:NMTOKEN to fails that
// type, and cvc-datatype-valid (§4.1.4) states no clause letting it fall back to
// the attribute's declared default. Read raw, every row below is ACCEPTED and
// mints the default component — use="foo" an optional use, form="Qualified" a
// name in no namespace.
//
// wantLine pins the charge to the line of the attribute that WROTE the value
// (STYLE E3): use= and form= on the declaration, the two *FormDefault
// attributes on <schema>, which is where checkFormDefaults reads them;
// TestProduceAttributeValueChargeLocated pins the column.
func TestProduceUseAndFormOutOfEnumeration(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2).
	cases := []struct {
		name     string
		doc      string
		wantMsg  string
		wantLine int
	}{
		{
			// msData/attribute/attF004's shape: use="default" is not one of the
			// three, and no clause reads it as optional.
			name: `use="default" on a local <attribute>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string" use="default"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `<attribute> use value "default"`,
			wantLine: 3,
		},
		{
			// The empty string is a member of no NMTOKEN enumeration either, and is
			// the row a "treat unrecognized as absent" reading would wave through.
			name: `use="" on a local <attribute>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string" use=""/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `<attribute> use value ""`,
			wantLine: 3,
		},
		{
			// The enumeration is case sensitive, like every NMTOKEN enumeration.
			name: `use="Required" on a local <attribute>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string" use="Required"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `<attribute> use value "Required"`,
			wantLine: 3,
		},
		{
			// The character-class boundary, per #324's precedent and #455's table.
			// U+00A0 is not §4.3.6 whitespace and collapse PRESERVES it, so the
			// padded literal is NOT the keyword and must be charged — where the
			// wider class strings.TrimSpace cuts would read it as required.
			name: `use="&#xA0;required" — U+00A0 is not §4.3.6 whitespace`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string" use="&#xA0;required"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `<attribute> use value "\u00a0required"`,
			wantLine: 3,
		},
		{
			// msData/attribute/attKc018a's shape, one letter off qualified.
			name: `form="Qualified" on a local <attribute>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string" form="Qualified"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `form value "Qualified"`,
			wantLine: 3,
		},
		{
			name: `form="" on a local <element>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence>
<xs:element name="a" type="xs:string" form=""/>
</xs:sequence></xs:complexType>
</xs:schema>`,
			wantMsg:  `form value ""`,
			wantLine: 3,
		},
		{
			name: `form="&#xA0;qualified" on a local <element> — U+00A0 is not §4.3.6 whitespace`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence>
<xs:element name="a" type="xs:string" form="&#xA0;qualified"/>
</xs:sequence></xs:complexType>
</xs:schema>`,
			wantMsg:  `form value "\u00a0qualified"`,
			wantLine: 3,
		},
		{
			// The prohibited <attribute> reaches localTargetNS through
			// prohibitedAttributeNames ALONE — produceAttributeUse returns before
			// building the sibling declaration that reads form= for every other
			// use= — so this row is the only one that pins that call site.
			name: `form="Qualified" on a prohibited local <attribute>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" use="prohibited" form="Qualified"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `form value "Qualified"`,
			wantLine: 3,
		},
		{
			// msData/element/elemH004's shape.
			name: `elementFormDefault="Qualified" on <schema>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x" elementFormDefault="Qualified">
<xs:complexType name="T"><xs:sequence>
<xs:element name="a" type="xs:string"/>
</xs:sequence></xs:complexType>
</xs:schema>`,
			wantMsg:  `elementFormDefault value "Qualified"`,
			wantLine: 1,
		},
		{
			// msData/element/elemH003's shape.
			name: `elementFormDefault="" on <schema>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x" elementFormDefault="">
<xs:complexType name="T"><xs:sequence>
<xs:element name="a" type="xs:string"/>
</xs:sequence></xs:complexType>
</xs:schema>`,
			wantMsg:  `elementFormDefault value ""`,
			wantLine: 1,
		},
		{
			// msData/element/elemH006's shape: two enumeration members side by side
			// are one NMTOKEN that is neither. collapse removes no INTERIOR
			// whitespace that separates them into a member.
			name: `attributeFormDefault="qualified unqualified" on <schema>`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x" attributeFormDefault="qualified unqualified">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `attributeFormDefault value "qualified unqualified"`,
			wantLine: 1,
		},
		{
			name: `attributeFormDefault="&#xA0;qualified" — U+00A0 is not §4.3.6 whitespace`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" xmlns:tns="urn:x" attributeFormDefault="&#xA0;qualified">
<xs:complexType name="T"><xs:sequence/>
<xs:attribute name="a" type="xs:string"/>
</xs:complexType>
</xs:schema>`,
			wantMsg:  `attributeFormDefault value "\u00a0qualified"`,
			wantLine: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, tc.doc)
			mustRule(t, err, "cvc-datatype-valid", tc.wantMsg, "the schema for schema documents restricts")
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no position, want the offending attribute's (STYLE E3)", err)
			}
			if loc.URI != produceURI || loc.Line != tc.wantLine || loc.Col == 0 {
				t.Fatalf("position = %s:%d:%d, want the attribute that wrote the value at %s:%d with a column",
					loc.URI, loc.Line, loc.Col, produceURI, tc.wantLine)
			}
		})
	}
}

// TestProduceFormDefaultChargedWithNoLocalDeclaration is why the *FormDefault
// charge is eager and once per document rather than at localTargetNS' read: that
// function consults a *FormDefault ONLY for a local declaration carrying neither
// targetNamespace nor form=, so the document below — which declares none at all —
// would never be charged from there.
//
// The document is malformed because it is malformed, not because some
// declaration happened to read the attribute (the content-independence argument
// checkDefaultOpenContent's doc states for the same pass).
func TestProduceFormDefaultChargedWithNoLocalDeclaration(t *testing.T) {
	_, err := produce(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x" elementFormDefault="Qualified">
<xs:complexType name="T"><xs:sequence/></xs:complexType>
</xs:schema>`)
	mustRule(t, err, "cvc-datatype-valid", `elementFormDefault value "Qualified"`)
	loc, ok := xsderr.LocOf(err)
	if !ok || loc.Line != 1 || loc.Col == 0 {
		t.Fatalf("position = %+v (ok=%v), want the elementFormDefault the <schema> element wrote", loc, ok)
	}
}

// TestParseFormDefaultChargedInEveryAssemblyDocument places the malformed
// attribute on an INCLUDED document that contributes a type nothing in the root
// reads, so no local declaration of the root's own consults it. The check runs
// per document beside checkDefaultOpenContent, for the reason that pass runs
// there: a document produced on demand through symbols.typeSource must not mint
// a local name off a *FormDefault nothing has judged.
func TestParseFormDefaultChargedInEveryAssemblyDocument(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
		"lib.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a" attributeFormDefault="foo">
<xs:complexType name="T"><xs:sequence/></xs:complexType>
</xs:schema>`,
	})
	mustRule(t, err, "cvc-datatype-valid", `attributeFormDefault value "foo"`)
}
