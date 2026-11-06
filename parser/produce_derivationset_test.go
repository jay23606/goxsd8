package parser_test

import (
	"slices"
	"testing"

	"github.com/kud360/goxsd8/xsd"
)

// TestProduceDerivationSetValueRejected pins that a block=/final= family value
// outside the s4s type its owner's production declares it with (xs:blockSet,
// xs:derivationSet, xs:simpleDerivationSet, xs:fullDerivationSet; Structures
// §A, Datatypes §A) is charged cvc-datatype-valid at the offending attribute. Each
// type is "#all" alone or a list of its members, so a stray token, a token of a
// sibling type, "#all" beside another token, a case variant, and a token split
// by a character §4.3.6 is not whitespace for are all outside it. Every row puts
// the offending attribute on the line named by wantLine, and wantMsg names the
// element, the attribute, the value and the type, in that order.
func TestProduceDerivationSetValueRejected(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2).
	cases := []struct {
		name     string
		defaults string
		body     string
		wantMsg  string
		wantLine int
	}{
		{
			name:     `<element> block="foo"`,
			body:     `<xs:element name="e" type="xs:string" block="foo"/>`,
			wantMsg:  `<element> block "foo" is not in the ·lexical space· of xs:blockSet`,
			wantLine: 2,
		},
		{
			name:     `<element> block="#all extension"`,
			body:     `<xs:element name="e" type="xs:string" block="#all extension"/>`,
			wantMsg:  `<element> block "#all extension" is not in the ·lexical space· of xs:blockSet`,
			wantLine: 2,
		},
		{
			name:     `<element> block="#  all"`,
			body:     `<xs:element name="e" type="xs:string" block="#  all"/>`,
			wantMsg:  `<element> block "#  all" is not in the ·lexical space· of xs:blockSet`,
			wantLine: 2,
		},
		{
			name:     `<element> block with a U+00A0 separator`,
			body:     `<xs:element name="e" type="xs:string" block="extension&#xA0;restriction"/>`,
			wantMsg:  "<element> block \"extension\\u00a0restriction\" is not in the ·lexical space· of xs:blockSet",
			wantLine: 2,
		},
		{
			name: `local <element> block="list"`,
			body: `<xs:complexType name="ct"><xs:sequence>` + "\n" +
				`<xs:element name="l" type="xs:string" block="list"/>` + "\n" +
				`</xs:sequence></xs:complexType>`,
			wantMsg:  `<element> block "list" is not in the ·lexical space· of xs:blockSet`,
			wantLine: 3,
		},
		{
			name:     `<element> final="substitution"`,
			body:     `<xs:element name="e" type="xs:string" final="substitution"/>`,
			wantMsg:  `<element> final "substitution" is not in the ·lexical space· of xs:derivationSet`,
			wantLine: 2,
		},
		{
			name:     `<element> final="#All"`,
			body:     `<xs:element name="e" type="xs:string" final="#All"/>`,
			wantMsg:  `<element> final "#All" is not in the ·lexical space· of xs:derivationSet`,
			wantLine: 2,
		},
		{
			name:     `<element> final="Extension"`,
			body:     `<xs:element name="e" type="xs:string" final="Extension"/>`,
			wantMsg:  `<element> final "Extension" is not in the ·lexical space· of xs:derivationSet`,
			wantLine: 2,
		},
		{
			name:     `<complexType> block="substitution"`,
			body:     `<xs:complexType name="ct" block="substitution"><xs:sequence/></xs:complexType>`,
			wantMsg:  `<complexType> block "substitution" is not in the ·lexical space· of xs:derivationSet`,
			wantLine: 2,
		},
		{
			name:     `<complexType> final="restriction #all"`,
			body:     `<xs:complexType name="ct" final="restriction #all"><xs:sequence/></xs:complexType>`,
			wantMsg:  `<complexType> final "restriction #all" is not in the ·lexical space· of xs:derivationSet`,
			wantLine: 2,
		},
		{
			name:     `<complexType> final="list"`,
			body:     `<xs:complexType name="ct" final="list"><xs:sequence/></xs:complexType>`,
			wantMsg:  `<complexType> final "list" is not in the ·lexical space· of xs:derivationSet`,
			wantLine: 2,
		},
		{
			name:     `<simpleType> final="substitution"`,
			body:     `<xs:simpleType name="st" final="substitution"><xs:restriction base="xs:string"/></xs:simpleType>`,
			wantMsg:  `<simpleType> final "substitution" is not in the ·lexical space· of xs:simpleDerivationSet`,
			wantLine: 2,
		},
		{
			name:     `<schema> blockDefault="list"`,
			defaults: ` blockDefault="list"`,
			wantMsg:  `<schema> blockDefault "list" is not in the ·lexical space· of xs:blockSet`,
			wantLine: 1,
		},
		{
			name:     `<schema> finalDefault="substitution"`,
			defaults: ` finalDefault="substitution"`,
			wantMsg:  `<schema> finalDefault "substitution" is not in the ·lexical space· of xs:fullDerivationSet`,
			wantLine: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, finalSchema(tc.defaults, "\n"+tc.body))
			assertRule(t, err, "cvc-datatype-valid")
			assertIDFault(t, err, tc.wantMsg, tc.wantLine)
		})
	}
}

// TestProduceDerivationSetValueAccepted pins the values inside each type that a
// token-by-token reading could get wrong: the empty list, a repeated member,
// each Default attribute's own wider vocabulary, extension on a <simpleType>
// (xs:simpleDerivationSet admits it), and "#all" padded with the whitespace
// xs:token's collapse removes. The padded rows also pin the mapping: a padded
// "#all" is the ·actual value· "#all" and expands to the whole ·relevant set·,
// on the local attribute and on the Default fallback alike.
func TestProduceDerivationSetValueAccepted(t *testing.T) {
	blockAll := []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction, xsd.DerivationSubstitution}
	finalAll := []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction}
	for _, tc := range []struct {
		name      string
		defaults  string
		attrs     string
		wantBlock []xsd.DerivationMethod
		wantFinal []xsd.DerivationMethod
	}{
		{name: "empty values", attrs: ` block="" final=""`},
		{name: "repeated members", attrs: ` block="extension extension" final="restriction restriction"`,
			wantBlock: []xsd.DerivationMethod{xsd.DerivationExtension},
			wantFinal: []xsd.DerivationMethod{xsd.DerivationRestriction}},
		{name: "padded #all", attrs: " block=\" #all \" final=\"&#x9;#all&#xA;\"",
			wantBlock: blockAll, wantFinal: finalAll},
		{name: "padded #all Defaults", defaults: ` blockDefault="  #all  " finalDefault=" #all"`,
			wantBlock: blockAll, wantFinal: finalAll},
		{name: "Defaults' own vocabularies", defaults: ` blockDefault="substitution" finalDefault="list union extension"`,
			wantBlock: []xsd.DerivationMethod{xsd.DerivationSubstitution},
			wantFinal: []xsd.DerivationMethod{xsd.DerivationExtension}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := produce(t, finalSchema(tc.defaults,
				`<xs:element name="e" type="xs:string"`+tc.attrs+`/>`+
					`<xs:simpleType name="st" final="extension"><xs:restriction base="xs:string"/></xs:simpleType>`))
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			ed, ok := s.Element(xsd.QName{Local: "e"})
			if !ok {
				t.Fatalf("element e not found")
			}
			if got := ed.DisallowedSubstitutions(); !slices.Equal(got, tc.wantBlock) {
				t.Errorf("{disallowed substitutions} = %v, want %v", got, tc.wantBlock)
			}
			if got := ed.SubstitutionGroupExclusions(); !slices.Equal(got, tc.wantFinal) {
				t.Errorf("{substitution group exclusions} = %v, want %v", got, tc.wantFinal)
			}
		})
	}
}
