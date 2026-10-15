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

// dropSchema declares, in urn:s, T, an optional z of xs:string and the
// attribute a; U, which extends T with a lax element wildcard and a lax
// {attribute wildcard}, so that under U a key no declaration or use carries
// declines (walker.wildcardMember); r, of type T, selects U where its a is
// "u". P holds a lax ##targetNamespace element wildcard, the attribute a and
// a lax {attribute wildcard}, so that under P such a key declines too; P1
// restricts P to the attributes a, b and c and no {attribute wildcard}, and
// P2 to a and b, so that under P2 a key "c" is a child the element wildcard
// names. q, of type P, selects P1 where its a is "1", P2 where it is "2" or
// it has a c; qw, of type P, selects P1 where it has an a, a {test} outside
// the §3.12.6 compiler's subset.
const dropSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:s" xmlns:s="urn:s" elementFormDefault="qualified">
  <xs:complexType name="T">
    <xs:sequence><xs:element name="z" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="U">
    <xs:complexContent><xs:extension base="s:T">
      <xs:sequence><xs:any processContents="lax" minOccurs="0"/></xs:sequence>
      <xs:anyAttribute processContents="lax"/>
    </xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="r" type="s:T"><xs:alternative test="@a = 'u'" type="s:U"/></xs:element>
  <xs:complexType name="P">
    <xs:sequence><xs:any namespace="##targetNamespace" processContents="lax" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
    <xs:anyAttribute processContents="lax"/>
  </xs:complexType>
  <xs:complexType name="P1">
    <xs:complexContent><xs:restriction base="s:P">
      <xs:sequence><xs:any namespace="##targetNamespace" processContents="lax" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>
      <xs:attribute name="a" type="xs:string"/>
      <xs:attribute name="b" type="xs:string"/>
      <xs:attribute name="c" type="xs:string"/>
    </xs:restriction></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="P2">
    <xs:complexContent><xs:restriction base="s:P">
      <xs:sequence><xs:any namespace="##targetNamespace" processContents="lax" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>
      <xs:attribute name="a" type="xs:string"/>
      <xs:attribute name="b" type="xs:string"/>
    </xs:restriction></xs:complexContent>
  </xs:complexType>
  <xs:element name="q" type="s:P">
    <xs:alternative test="@a = '1'" type="s:P1"/>
    <xs:alternative test="@a = '2'" type="s:P2"/>
    <xs:alternative test="@c" type="s:P2"/>
  </xs:element>
  <xs:element name="qw" type="s:P"><xs:alternative test="count(@a) > 0" type="s:P1"/></xs:element>
</xs:schema>`

// TestTypeTableDropped pins a root member one candidate type declines alone
// (walker.candidates): that candidate drops from the run, and the member maps
// under the type the attributes select, as the engine assesses each XML twin
// under it. Under r, U declines y and the attributes select T, the
// declaration's {type definition}, under which y is a child no wildcard names;
// under q, P, the declaration's {type definition}, declines every key but a,
// and the attributes select P1 or P2. In the last row the attributes before c
// select P, which declines b, while P1 takes c as an attribute, so the run
// goes on (walker.settled) and its attributes select P1.
func TestTypeTableDropped(t *testing.T) {
	v := validatorOver(t, dropSchema)
	const s = `xmlns="urn:s" `
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"another candidate declines, the declared type selected", `{"r":{"a":"t","y":"1"}}`,
			`<r ` + s + `a="t"><y>1</y></r>`, content},
		{"another candidate declines, no attribute", `{"r":{"y":"1"}}`,
			`<r ` + s + `><y>1</y></r>`, content},
		{"the declared type declines a child", `{"q":{"a":"1","y":"1"}}`,
			`<q ` + s + `a="1"><y>1</y></q>`, none},
		{"the declared type declines an attribute", `{"q":{"b":"x","a":"2"}}`,
			`<q ` + s + `b="x" a="2"/>`, none},
		{"the declared type declines the child ending the run and one after it", `{"q":{"a":"1","y":"1","w":"1"}}`,
			`<q ` + s + `a="1"><y>1</y><w>1</w></q>`, none},
		{"the type the attributes before a member select declines one, the run going on", `{"q":{"b":"x","c":"y","a":"1"}}`,
			`<q ` + s + `b="x" c="y" a="1"/>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestTypeTableDroppedDeclines pins the declines a dropped candidate leaves
// at the root, each beside the verdict the engine gives its XML twin: the
// type the attributes select is the one that declined a member, and the
// decline is that type's own (walker.settled, walker.declined); and a member
// every live candidate declines, under the type the attributes select
// (walker.declined), where that type classifies the run otherwise
// (walker.settled's #2701 marker), and where the engine withholds it
// (walker.settled's #2702 marker). x:k is a key in the xsi namespace, which
// every candidate declines.
func TestTypeTableDroppedDeclines(t *testing.T) {
	v := validatorOver(t, dropSchema)
	const s = `xmlns="urn:s" `
	const x = `{"$xmlns":{"x":"http://www.w3.org/2001/XMLSchema-instance"},`
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
		withheld        bool
		opens           string
	}{
		{"the selected type declines a child", `{"r":{"a":"u","y":"1"}}`,
			`<r ` + s + `a="u"><y>1</y></r>`, none, false,
			`jsonsrc: instance.json:1:15: member "y" names no child element declaration and no attribute use of the type of {urn:s}r, which has both`},
		{"the selected type declines an attribute", `{"q":{"b":"x"}}`,
			`<q ` + s + `b="x"/>`, none, false,
			`jsonsrc: instance.json:1:7: member "b" names no child element declaration and no attribute use of the type of {urn:s}q, which has both`},
		{"every candidate declines, the selected type's own", `{"q":` + x + `"a":"1","x:k":"1"}}`,
			`<q ` + s + xsiDecl + ` a="1"><xsi:k>1</xsi:k></q>`, content, false,
			`jsonsrc: instance.json:1:74: member "x:k" names no child element declaration and no attribute use of the type of {urn:s}q, and its prefix puts it in the namespace`},
		{"every candidate declines, the selected type classifying the run otherwise", `{"q":` + x + `"c":"1","x:k":"1"}}`,
			`<q ` + s + xsiDecl + ` c="1"><xsi:k>1</xsi:k></q>`, []xsderr.Rule{"cvc-complex-type", "cvc-complex-content"}, false,
			`jsonsrc: instance.json:1:74: the members of {urn:s}q settle no type`},
		{"every candidate declines, the selection withheld", `{"qw":` + x + `"a":"1","x:k":"1"}}`,
			`<qw ` + s + xsiDecl + ` a="1"><xsi:k>1</xsi:k></qw>`, nil, true,
			`jsonsrc: instance.json:1:75: the members of {urn:s}qw are classified differently under the types its {type table} can select, or declined under one of them`},
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
