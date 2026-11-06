package parser_test

import (
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// TestProduceAttributeValueChargeLocated pins that a charge against one
// attribute's value is located at that attribute's name, not at its element's
// start tag (#3429): one row per charge, each faulted attribute written after
// another attribute and on a later line than the element's "<", so neither the
// element's position nor its first attribute's answers it. The last row is the
// guard: src-simple-type clause 1 charges the second <maxLength> as a whole, and
// stays at its "<" though its value attribute sits on the next line.
func TestProduceAttributeValueChargeLocated(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2).
	cases := []struct {
		name string
		body string
		rule xsderr.Rule
		line int
		col  int
	}{
		{
			name: "rejectInvalidID non-NCName",
			body: "\n" + `<xs:element name="e"` + "\n" + `  type="xs:string" id="1"/>`,
			rule: "cvc-datatype-valid", line: 3, col: 20,
		},
		{
			name: "rejectInvalidID duplicate",
			body: "\n" + `<xs:element name="e" id="a" type="xs:string"/>` + "\n" +
				`<xs:element name="f"` + "\n" + `   type="xs:string" id="a"/>`,
			rule: "cvc-id", line: 4, col: 21,
		},
		{
			name: "rejectInvalidDerivationSet block",
			body: "\n" + `<xs:element name="e"` + "\n" + ` type="xs:string" block="foo"/>`,
			rule: "cvc-datatype-valid", line: 3, col: 19,
		},
		{
			name: "declarationName non-NCName",
			body: "\n" + `<xs:element type="xs:string"` + "\n" + `    name="a:b"/>`,
			rule: "cvc-datatype-valid", line: 3, col: 5,
		},
		{
			name: "rejectXmlnsName",
			body: "\n" + `<xs:attribute type="xs:string"` + "\n" + `  name="xmlns"/>`,
			rule: "no-xmlns", line: 3, col: 3,
		},
		{
			name: "facetCountValue nonNegativeInteger",
			body: "\n" + `<xs:simpleType name="T"><xs:restriction base="xs:string">` + "\n" +
				`<xs:maxLength id="m"` + "\n" + `  value="-1"/></xs:restriction></xs:simpleType>`,
			rule: "cvc-datatype-valid", line: 4, col: 3,
		},
		{
			name: "facetCountValue positiveInteger",
			body: "\n" + `<xs:simpleType name="T"><xs:restriction base="xs:decimal">` + "\n" +
				`<xs:totalDigits id="t"` + "\n" + ` value="0"/></xs:restriction></xs:simpleType>`,
			rule: "cvc-datatype-valid", line: 4, col: 2,
		},
		{
			name: "facetFixed",
			body: "\n" + `<xs:simpleType name="T"><xs:restriction base="xs:string">` + "\n" +
				`<xs:maxLength value="1"` + "\n" + `    fixed="yes"/></xs:restriction></xs:simpleType>`,
			rule: "cvc-datatype-valid", line: 4, col: 5,
		},
		{
			name: "guard: src-simple-type clause 1 at the element",
			body: "\n" + `<xs:simpleType name="T"><xs:restriction base="xs:string">` + "\n" +
				`<xs:maxLength value="1"/>  <xs:maxLength` + "\n" + `  value="2"/></xs:restriction></xs:simpleType>`,
			rule: "src-simple-type", line: 3, col: 28,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", tc.body))
			assertRule(t, err, tc.rule)
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no position (STYLE E3)", err)
			}
			if loc.URI != produceURI || loc.Line != tc.line || loc.Col != tc.col {
				t.Fatalf("position = %s:%d:%d, want %s:%d:%d (%v)",
					loc.URI, loc.Line, loc.Col, produceURI, tc.line, tc.col, err)
			}
		})
	}
}
