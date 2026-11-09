package parser_test

import (
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// TestProduceProcessContentsFaultCharged pins which rule an out-of-enumeration
// processContents is charged, drawing the line at whether a Wildcard exists to be
// constrained (#950).
//
// The schema for schema documents declares processContents as an xs:NMTOKEN
// restricted to the enumeration skip/lax/strict, in the wildcard attribute group
// both <any> and <anyAttribute> use (xmlschema11-1.md:5346-5353). A lexical
// outside that value space is §5.1's first bullet — the document is not valid
// against the Schema for Schema Documents — charged cvc-datatype-valid (Datatypes
// §4.1.4), the convention parser/produce.go's ruleDatatypeValid doc comment states
// for a schema document attribute that fails its own declared type with no
// Structures Schema Representation Constraint covering the case. The producer
// never reaches xsd.NewWildcard on these rows: produceWildcard takes the lexical
// verdict from processContentsOf first, so no Wildcard is ever built.
//
// w-props-correct is a Schema Component Constraint over a Wildcard's PROPERTIES
// (§3.10.6.1): clause 1 reads the {process contents} property, which needs the
// component. It stays charged by xsd.NewWildcard, on the programmatic path that
// bypasses this producer — xsd/wildcard_test.go pins that side.
//
// Every row REJECTS, before and after #950: this table pins the rule ID, which a
// test asserting only that an error occurred cannot see.
func TestProduceProcessContentsFaultCharged(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2). Every body starts
	// on line 2 of the wrapped document and puts the offending element, and so
	// its processContents the charge is located at, on line 3;
	// TestProduceAttributeValueChargeLocated pins the column.
	cases := []struct {
		name string
		body string
	}{
		{
			name: `<any> processContents="bogus"`,
			body: "\n" + `<xs:complexType name="CT"><xs:sequence>` + "\n" +
				`<xs:any processContents="bogus"/>` + "\n" +
				`</xs:sequence></xs:complexType>`,
		},
		{
			name: `<anyAttribute> processContents="bogus"`,
			body: "\n" + `<xs:complexType name="CT"><xs:sequence/>` + "\n" +
				`<xs:anyAttribute processContents="bogus"/>` + "\n" +
				`</xs:complexType>`,
		},
		{
			// The enumeration compares NMTOKEN values, which are case-sensitive:
			// "Strict" is a well-formed NMTOKEN outside the three enumerated ones.
			name: `<any> processContents="Strict"`,
			body: "\n" + `<xs:complexType name="CT"><xs:sequence>` + "\n" +
				`<xs:any processContents="Strict"/>` + "\n" +
				`</xs:sequence></xs:complexType>`,
		},
		{
			// A lexical that is not even an NMTOKEN fails the base type rather than
			// the enumeration facet, and carries the same rule ID: one attribute
			// failing one declared type.
			name: `<anyAttribute> processContents="a b"`,
			body: "\n" + `<xs:complexType name="CT"><xs:sequence/>` + "\n" +
				`<xs:anyAttribute processContents="a b"/>` + "\n" +
				`</xs:complexType>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", tc.body))
			assertRule(t, err, "cvc-datatype-valid")
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no position, want %s:3:col (STYLE E3)", err, produceURI)
			}
			if loc.URI != produceURI || loc.Line != 3 || loc.Col == 0 {
				t.Fatalf("position = %s:%d:%d, want the offending processContents at %s:3 with a column",
					loc.URI, loc.Line, loc.Col, produceURI)
			}
		})
	}
}

// TestProduceProcessContentsEnumerationAccepted is the unchanged control for
// TestProduceProcessContentsFaultCharged: each enumerated lexical still maps to
// its ProcessContents token, on an element wildcard and an attribute wildcard
// alike.
func TestProduceProcessContentsEnumerationAccepted(t *testing.T) {
	cases := []struct {
		lexical string
		want    xsd.ProcessContents
	}{
		{lexical: "skip", want: xsd.ProcessSkip},
		{lexical: "lax", want: xsd.ProcessLax},
		{lexical: "strict", want: xsd.ProcessStrict},
	}
	for _, tc := range cases {
		t.Run(tc.lexical, func(t *testing.T) {
			ct := complexType(t, `<xs:complexType name="CT"><xs:sequence>`+
				`<xs:any processContents="`+tc.lexical+`"/></xs:sequence>`+
				`<xs:anyAttribute processContents="`+tc.lexical+`"/></xs:complexType>`, "CT")
			elem, ok := topGroup(t, ct).Particles()[0].Term().(xsd.ResolvedTerm).Term.(xsd.Wildcard)
			if !ok {
				t.Fatalf("<any> did not map to a Wildcard term")
			}
			if elem.ProcessContents() != tc.want {
				t.Errorf("<any> {process contents} = %s, want %s", elem.ProcessContents(), tc.want)
			}
			attr, ok := ct.AttributeWildcard()
			if !ok {
				t.Fatalf("attribute wildcard absent, want present")
			}
			if attr.ProcessContents() != tc.want {
				t.Errorf("<anyAttribute> {process contents} = %s, want %s", attr.ProcessContents(), tc.want)
			}
		})
	}
}
