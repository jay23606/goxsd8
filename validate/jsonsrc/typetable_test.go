package jsonsrc

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsderr"
)

// typeTableSchema declares, in urn:c, the top-level m of xs:string and k of
// an anonymous type with the attribute b; T, empty content with the attribute
// a; S, which extends T with a required reference to m; and U, which extends T
// with the attribute m. tt is of type T and selects S where its a is "m", or
// where its xsi:type is spelled "p:T"; tu is of type T and selects U where its
// a is "u"; r holds an optional reference to tt. Under T a key "m" is the
// child m (no wildcard: walker.namedChild), as it is under S (a contained
// reference), and the attribute m under U. V holds an optional, nillable n of
// xs:string, an optional k of an anonymous type and an optional j of xs:string
// carrying a {type table}; W restricts V, its n of xs:token and not nillable,
// and no k or j; X extends V, adding nothing, so it contains V's own n, k and
// j. tv is of type V and selects W where its a is "w"; tx is of type V and
// selects X where its a is "x".
const typeTableSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    targetNamespace="urn:c" xmlns:c="urn:c" elementFormDefault="qualified">
  <xs:element name="m" type="xs:string"/>
  <xs:element name="k"><xs:complexType><xs:attribute name="b"/></xs:complexType></xs:element>
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
      <xs:element name="n" type="xs:string" minOccurs="0" nillable="true"/>
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

// TestTypeTable pins an element whose declaration carries a {type table},
// each of whose members every type the table can select classifies alike
// (walker.alike) — a scalar or null child by its name alone, whatever
// declarations it is under: at the root the mapping classifies them under the
// type the table selects (walker.settled), below it under its declared {type
// definition}, a "$type" on its child is settled under the selected type at
// the root and alike under every one below it (walker.instanceGoverned), and
// the engine assesses it under the type the table selects. The rows marked
// "selected" discriminate the selection: under typeTableSchema with its
// <xs:alternative>s removed, the first, the particle's first and the "$type"
// on a child charge cvc-complex-type, and the other four charge nothing. The
// rest pin behaviour that schema shares.
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
		{"a scalar child under declarations of two types", `{"tv":{"n":"x"}}`,
			`<tv ` + c + `><n>x</n></tv>`, none},
		{"a scalar array item under declarations of two types", `{"tv":{"n":["x"]}}`,
			`<tv ` + c + `><n>x</n></tv>`, none},
		{"selected: a null child under a declaration the selected type's is not nillable as", `{"tv":{"a":"w","n":null}}`,
			`<tv ` + c + xsiDecl + ` a="w"><n xsi:nil="true"/></tv>`, []xsderr.Rule{"cvc-elt"}},
		{"a null child under the declared type's nillable declaration", `{"tv":{"n":null}}`,
			`<tv ` + c + xsiDecl + `><n xsi:nil="true"/></tv>`, none},
		{"a child under one declaration of an anonymous type", `{"tx":{"k":{}}}`,
			`<tx ` + c + `><k/></tx>`, none},
		{"a child under one declaration of an anonymous type, the extension selected", `{"tx":{"a":"x","k":{"b":"1"}}}`,
			`<tx ` + c + `a="x"><k b="1"/></tx>`, none},
		{"a scalar child under one declaration carrying a {type table}", `{"tx":{"j":"y"}}`,
			`<tx ` + c + `><j>y</j></tx>`, none},
		{"selected: a \"$type\" on a child of the root", `{"tt":{"a":"m","m":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}}`,
			`<tt ` + c + `a="m" ` + xsiDecl + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><m xsi:type="xs:string"/></tt>`, none},
		{"a \"$type\" on a child of the root, the {default type definition} selected", `{"tt":{"a":"z","m":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}}`,
			`<tt ` + c + `a="z" ` + xsiDecl + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><m xsi:type="xs:string"/></tt>`, complexType},
		{"a \"$type\" on a child of the root overriding under no selectable type", `{"tt":{"m":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:int"}}}`,
			`<tt ` + c + xsiDecl + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><m xsi:type="xs:int"/></tt>`, []xsderr.Rule{"cvc-complex-type", "cvc-elt"}},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestTypeTableSelected pins a root whose {type table}'s types classify a
// member differently: the mapping reads the type the table ·conditionally
// selects· for the root's leading attribute members (walker.attributes,
// walker.settled) and classifies the members under that type, as the engine
// assesses each XML twin under it. Under T, m is the child m, and a = "u"
// selects U, under which m is an attribute; under U, a = "u" and m = "x"
// select U again, so m is U's attribute. k, an object, is under V's local
// declaration of one anonymous type and, W containing no k, under the
// top-level k of another, whose {context}s differ (oneType); no a selects V,
// which classifies k as its own, as the member that ends the attribute run
// (walker.agrees) and as a later one (walker.classify, under V's object
// alone).
func TestTypeTableSelected(t *testing.T) {
	v := validatorOver(t, typeTableSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"a key classified as a child under one type and an attribute under another", `{"tu":{"a":"u","m":"x"}}`,
			`<tu xmlns="urn:c" a="u" m="x"/>`, none},
		{"a child under declarations of two anonymous types", `{"tv":{"k":{}}}`,
			`<tv xmlns="urn:c"><k/></tv>`, none},
		{"a later child under declarations of two anonymous types", `{"tv":{"n":"x","k":{}}}`,
			`<tv xmlns="urn:c"><n>x</n><k/></tv>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// runSchema declares, in urn:s, T, empty content with the attribute a; U,
// which extends T with the attribute m; Q, with the attribute a and a lax
// ##local {attribute wildcard}, and Q1 and Q2, which extend Q with an
// attribute m, unqualified and qualified. tu is of type T and selects U where
// its a is "u"; p holds an optional tu. tn is of type T and selects T where it
// has an m, else U where its a is "u". tw selects U where its a is "u" and it
// has no m, and its second {test} is outside the §3.12.6 compiler's subset, as
// tx's first is. tq is of type Q and selects Q1 where its a is "1" and Q2 where
// it is "2". Under T a key "m" is a child no wildcard names (walker.namedChild).
const runSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:s" xmlns:s="urn:s" elementFormDefault="qualified">
  <xs:complexType name="T"><xs:attribute name="a" type="xs:string"/></xs:complexType>
  <xs:complexType name="U">
    <xs:complexContent><xs:extension base="s:T"><xs:attribute name="m" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="tu" type="s:T"><xs:alternative test="@a = 'u'" type="s:U"/></xs:element>
  <xs:element name="p"><xs:complexType><xs:sequence><xs:element ref="s:tu" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="tn" type="s:T">
    <xs:alternative test="@m" type="s:T"/>
    <xs:alternative test="@a = 'u'" type="s:U"/>
  </xs:element>
  <xs:element name="tw" type="s:T">
    <xs:alternative test="@a = 'u' and not(@m)" type="s:U"/>
    <xs:alternative test="count(@a) > 0" type="s:T"/>
  </xs:element>
  <xs:element name="tx" type="s:T"><xs:alternative test="count(@a) > 0" type="s:U"/></xs:element>
  <xs:complexType name="Q"><xs:attribute name="a" type="xs:string"/><xs:anyAttribute namespace="##local" processContents="lax"/></xs:complexType>
  <xs:complexType name="Q1">
    <xs:complexContent><xs:extension base="s:Q"><xs:attribute name="m" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="Q2">
    <xs:complexContent><xs:extension base="s:Q"><xs:attribute name="m" form="qualified" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="tq" type="s:Q">
    <xs:alternative test="@a = '1'" type="s:Q1"/>
    <xs:alternative test="@a = '2'" type="s:Q2"/>
  </xs:element>
</xs:schema>`

// TestTypeTableRunDeclines pins the declines the selection leaves, each
// beside the verdict the engine gives its XML twin: below the root, where the
// mapping computes no [inherited attributes] (selection's GAP(jsonsrc)
// marker); a member one type takes as an attribute while the type the
// attributes before it select ends the run on it, and one two types take as
// attributes of different names (walker.extend's); a run no selection
// settles: under T, a = "u" selects U, which takes m as an attribute, and
// under U, m selects T, which does not (walker.settled's); and a {test} the
// engine withholds the selection at, after a candidate dropped or where the
// candidates classify a member differently (walker.settled's).
func TestTypeTableRunDeclines(t *testing.T) {
	v := validatorOver(t, runSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
		withheld        bool
		opens           string
	}{
		{"below the root", `{"p":{"tu":{"a":"u","m":"x"}}}`,
			`<p xmlns="urn:s"><tu a="u" m="x"/></p>`, none, false,
			`jsonsrc: instance.json:1:21: member "m" of {urn:s}tu is not classified alike`},
		{"an attribute under one type where the selected type ends the run", `{"tu":{"a":"z","m":"x"}}`,
			`<tu xmlns="urn:s" a="z"><m>x</m></tu>`, complexType, false,
			`jsonsrc: instance.json:1:16: member "m" of {urn:s}tu is an attribute under a type its {type table} can select and a child under the one`},
		{"attributes of two expanded names", `{"tq":{"m":"x"}}`,
			`<tq xmlns="urn:s" m="x"/>`, none, false,
			`jsonsrc: instance.json:1:8: member "m" of {urn:s}tq is an attribute of the expanded names`},
		{"no run settled", `{"tn":{"a":"u","m":"x"}}`,
			`<tn xmlns="urn:s" a="u" m="x"/>`, complexType, false,
			`jsonsrc: instance.json:1:23: the members of {urn:s}tn settle no type`},
		{"withheld after a candidate dropped", `{"tw":{"a":"u","m":"x"}}`,
			`<tw xmlns="urn:s" a="u" m="x"/>`, none, true,
			`jsonsrc: instance.json:1:23: the members of {urn:s}tw are classified differently`},
		{"withheld where the candidates classify a member differently", `{"tx":{"a":"u","m":"x"}}`,
			`<tx xmlns="urn:s" a="u" m="x"/>`, none, true,
			`jsonsrc: instance.json:1:16: member "m" of {urn:s}tx is not classified alike`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xres, err := xmlsrc.Validate(v, strings.NewReader(tc.xml), xmlsrc.WithURI("instance.xml"))
			if err != nil {
				t.Fatalf("xmlsrc.Validate(%s): %v", tc.xml, err)
			}
			if got := rules(xres); !slices.Equal(got, tc.want) || (len(xres.Unevaluated()) > 0) != tc.withheld {
				t.Fatalf("XML twin %s charges %v, %d unevaluated; want %v, withheld %t", tc.xml, got, len(xres.Unevaluated()), tc.want, tc.withheld)
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

// TestTypeTableWithheldMaps pins a root whose {type table}'s selection the
// engine withholds where every type the table can select classifies its
// members alike: the mapping maps them as without a selection, and the
// engine records the withhold for the JSON instance as for its XML twin.
func TestTypeTableWithheldMaps(t *testing.T) {
	v := validatorOver(t, runSchema)
	twin(t, v, `{"tx":{"a":"u"}}`, `<tx xmlns="urn:s" a="u"/>`, none)
}
