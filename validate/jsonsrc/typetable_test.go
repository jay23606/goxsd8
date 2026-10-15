package jsonsrc

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsderr"
)

// typeTableSchema declares, in urn:c, the top-level m of xs:string; T, empty
// content with the attribute a; S, which extends T with a required reference
// to m; and U, which extends T with the attribute m. tt is of type T and
// selects S where its a is "m", or where its xsi:type is spelled "p:T"; tu is
// of type T and selects U where its a is "u"; r holds an optional reference to
// tt. Under T a key "m" is the child m (no wildcard: walker.namedChild), as it
// is under S (a contained reference), and the attribute m under U. V holds an
// optional n of xs:string, an optional k of an anonymous type and an optional
// j of xs:string carrying a {type table}; W restricts V, its n of xs:token
// and no k or j; X extends V, adding nothing, so it contains V's own n, k and
// j. tv is of type V and selects W where its a is "w"; tx is of
// type V and selects X where its a is "x".
const typeTableSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    targetNamespace="urn:c" xmlns:c="urn:c" elementFormDefault="qualified">
  <xs:element name="m" type="xs:string"/>
  <xs:complexType name="T"><xs:attribute name="a" type="xs:string"/></xs:complexType>
  <xs:complexType name="S">
    <xs:complexContent><xs:extension base="c:T"><xs:sequence><xs:element ref="c:m"/></xs:sequence></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="U">
    <xs:complexContent><xs:extension base="c:T"><xs:attribute name="m" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="tt" type="c:T">
    <xs:alternative test="@a = 'm'" type="c:S"/>
    <xs:alternative test="@xsi:type = 'p:T'" type="c:S"/>
  </xs:element>
  <xs:element name="tu" type="c:T">
    <xs:alternative test="@a = 'u'" type="c:U"/>
  </xs:element>
  <xs:element name="r">
    <xs:complexType><xs:sequence><xs:element ref="c:tt" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:complexType name="V">
    <xs:sequence>
      <xs:element name="n" type="xs:string" minOccurs="0"/>
      <xs:element name="k" minOccurs="0"><xs:complexType><xs:attribute name="b"/></xs:complexType></xs:element>
      <xs:element name="j" type="xs:string" minOccurs="0"><xs:alternative test="@b" type="xs:token"/></xs:element>
    </xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="W">
    <xs:complexContent>
      <xs:restriction base="c:V"><xs:sequence><xs:element name="n" type="xs:token" minOccurs="0"/></xs:sequence></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="X"><xs:complexContent><xs:extension base="c:V"/></xs:complexContent></xs:complexType>
  <xs:element name="tv" type="c:V">
    <xs:alternative test="@a = 'w'" type="c:W"/>
  </xs:element>
  <xs:element name="tx" type="c:V">
    <xs:alternative test="@a = 'x'" type="c:X"/>
  </xs:element>
</xs:schema>`

// TestTypeTable pins an element whose declaration carries a {type table}: its
// members are classified under its declared {type definition}, and alike
// under every type the table can select (walker.classify), and the engine
// assesses it under the type the table selects. The rows marked "selected"
// discriminate the selection: under typeTableSchema with its <xs:alternative>s
// removed, the first and the particle's first charge cvc-complex-type, and the
// other three charge nothing. The rest pin behaviour that schema shares. Every
// row declined before walker.typeTable's decline was lifted.
func TestTypeTable(t *testing.T) {
	v := validatorOver(t, typeTableSchema)
	const c = `xmlns="urn:c" `
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"selected: an alternative's {test} reads an attribute the row presents", `{"tt":{"a":"m","m":"x"}}`,
			`<tt ` + c + `a="m"><m>x</m></tt>`, none},
		{"selected: the type an alternative selects rejects", `{"tt":{"a":"m"}}`,
			`<tt ` + c + `a="m"/>`, content},
		{"no alternative true selects the {default type definition}", `{"tt":{"a":"z"}}`,
			`<tt ` + c + `a="z"/>`, none},
		{"no alternative true, a child only a selectable type admits", `{"tt":{"a":"z","m":"x"}}`,
			`<tt ` + c + `a="z"><m>x</m></tt>`, complexType},
		{"selected: the {test} reads the xsi:type a \"$type\" presents", `{"tt":{"$xmlns":{"p":"urn:c"},"$type":"p:T"}}`,
			`<tt ` + c + `xmlns:p="urn:c" ` + xsiDecl + ` xsi:type="p:T"/>`, []xsderr.Rule{"cvc-elt", "cvc-complex-content"}},
		{"a scalar value", `{"tt":"1"}`,
			`<tt ` + c + `>1</tt>`, complexType},
		{"selected: an element particle's declaration", `{"r":{"tt":{"a":"m","m":"x"}}}`,
			`<r ` + c + `><tt a="m"><m>x</m></tt></r>`, none},
		{"selected: an element particle's declaration, selecting a type that rejects", `{"r":{"tt":{"a":"m"}}}`,
			`<r ` + c + `><tt a="m"/></r>`, content},
		{"a key every selectable type classifies alike", `{"tu":{"a":"u"}}`,
			`<tu ` + c + `a="u"/>`, none},
		{"a child whose declaration every selectable type shares, of a named type", `{"tx":{"a":"x","n":"y"}}`,
			`<tx ` + c + `a="x"><n>y</n></tx>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestTypeTableDeclines pins the {type table} declines: a member the declared
// type and a selectable type classify differently (walker.classify's
// GAP(jsonsrc) marker, #2693) — m is the child m under T and the attribute m
// under U; n is under V's declaration of xs:string and W's of xs:token; and k
// is under V's one declaration in V and X both, which alike reads no identity
// of, its type being anonymous, as j is, its declaration carrying a {type
// table} — and a "$type" on a child of a tabled element, whose ·governing
// type definition· the mapping does not determine (walker.declarationType's
// GAP(jsonsrc) marker, #2693), though the engine assesses each XML twin.
func TestTypeTableDeclines(t *testing.T) {
	v := validatorOver(t, typeTableSchema)
	for _, tc := range []struct {
		name, json, xml string
		opens           string
	}{
		{"a key classified as a child under one type and an attribute under another", `{"tu":{"a":"u","m":"x"}}`,
			`<tu xmlns="urn:c" a="u" m="x"/>`,
			`jsonsrc: instance.json:1:16: member "m" of {urn:c}tu is not classified alike under its declaration's {type definition} and every type its {type table} can select`},
		{"a child under declarations of two types", `{"tv":{"n":"x"}}`,
			`<tv xmlns="urn:c"><n>x</n></tv>`,
			`jsonsrc: instance.json:1:8: member "n" of {urn:c}tv is not classified alike`},
		{"a child under one declaration of an anonymous type", `{"tx":{"k":{}}}`,
			`<tx xmlns="urn:c"><k/></tx>`,
			`jsonsrc: instance.json:1:8: member "k" of {urn:c}tx is not classified alike`},
		{"a child under one declaration carrying a {type table}", `{"tx":{"j":"y"}}`,
			`<tx xmlns="urn:c"><j>y</j></tx>`,
			`jsonsrc: instance.json:1:8: member "j" of {urn:c}tx is not classified alike`},
		{"a \"$type\" on a child of the root", `{"tt":{"a":"m","m":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}}`,
			`<tt xmlns="urn:c" a="m" ` + xsiDecl + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><m xsi:type="xs:string"/></tt>`,
			`jsonsrc: instance.json:1:72: the "$type" member of {urn:c}m: the mapping determines no type that governs {urn:c}tt`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xres, err := xmlsrc.Validate(v, strings.NewReader(tc.xml), xmlsrc.WithURI("instance.xml"))
			if err != nil {
				t.Fatalf("xmlsrc.Validate(%s): %v", tc.xml, err)
			}
			if got := rules(xres); len(got) > 0 || len(xres.Unevaluated()) > 0 {
				t.Fatalf("XML twin %s charges %v, %d unevaluated; want none", tc.xml, got, len(xres.Unevaluated()))
			}
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, nil error; want the decline", tc.json, rules(res))
			}
			if !strings.HasPrefix(err.Error(), tc.opens) {
				t.Errorf("Validate(%s) = %q, want it to open %q", tc.json, err, tc.opens)
			}
		})
	}
}
