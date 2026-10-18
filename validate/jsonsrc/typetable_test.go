package jsonsrc

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/validate"
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
// declarations it is under: the mapping classifies them under the type the
// table selects (walker.settled), a "$type" on its child is settled under
// the selected type (walker.instanceGoverned), and the engine assesses it
// under the type the table selects. The rows marked "selected" discriminate
// the selection: under typeTableSchema with its <xs:alternative>s removed,
// the first, the particle's first and the "$type" on a child charge
// cvc-complex-type, and the other four charge nothing. The rest pin
// behaviour that schema shares.
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
// it is "2". t2 is of type T and selects U where it has an m. tz is of type T
// and selects T where it has no m, and its second {test}, selecting U, is
// outside the §3.12.6 compiler's subset. P extends U with the attribute p, and
// tp is of type T and selects P where it has a p. V extends T with the
// attribute p, and tv is of type T and selects T where it has an m, else P
// where it has a p, else V where it has a zzz. t0 is of type Q and selects Q2
// where it has an m in no namespace and Q1 where it has one in urn:s. Under T
// a key "m" is a child no wildcard names (walker.namedChild).
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
  <xs:element name="t2" type="s:T"><xs:alternative test="@m" type="s:U"/></xs:element>
  <xs:element name="tz" type="s:T">
    <xs:alternative test="not(@m)" type="s:T"/>
    <xs:alternative test="count(@m) > 0" type="s:U"/>
  </xs:element>
  <xs:complexType name="P">
    <xs:complexContent><xs:extension base="s:U"><xs:attribute name="p" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="tp" type="s:T"><xs:alternative test="@p" type="s:P"/></xs:element>
  <xs:complexType name="V">
    <xs:complexContent><xs:extension base="s:T"><xs:attribute name="p" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="tv" type="s:T">
    <xs:alternative test="@m" type="s:T"/>
    <xs:alternative test="@p" type="s:P"/>
    <xs:alternative test="@zzz" type="s:V"/>
  </xs:element>
  <xs:element name="t0" type="s:Q">
    <xs:alternative test="@m" type="s:Q2"/>
    <xs:alternative test="@s:m" type="s:Q1"/>
  </xs:element>
</xs:schema>`

// TestTypeTableBelowRoot pins an element below the root whose {type table}'s
// types classify a member differently: the mapping reads the type the table
// ·conditionally selects· for it (walker.selector, walker.settled), as at the
// root, its parent p handing it no [inherited attributes]: under T, m is a
// child, and a = "u" selects U, under which m is an attribute.
func TestTypeTableBelowRoot(t *testing.T) {
	v := validatorOver(t, runSchema)
	twin(t, v, `{"p":{"tu":{"a":"u","m":"x"}}}`, `<p xmlns="urn:s"><tu a="u" m="x"/></p>`, none)
}

// TestTypeTableRunDeclines pins the declines the selection leaves, each
// beside the verdict the engine gives its XML twin: a run no selection
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
		{"no run settled", `{"tn":{"a":"u","m":"x"}}`,
			`<tn xmlns="urn:s" a="u" m="x"/>`, complexType, false,
			`jsonsrc: instance.json:1:23: the members of {urn:s}tn settle no type`},
		{"withheld after a candidate dropped", `{"tw":{"a":"u","m":"x"}}`,
			`<tw xmlns="urn:s" a="u" m="x"/>`, none, true,
			`jsonsrc: instance.json:1:23: the members of {urn:s}tw are classified differently`},
		{"withheld where the candidates classify a member differently", `{"tx":{"a":"u","m":"x"}}`,
			`<tx xmlns="urn:s" a="u" m="x"/>`, none, true,
			`jsonsrc: instance.json:1:16: the members of {urn:s}tx are classified differently`},
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

// TestTypeTableReadAhead pins a root whose attribute run the member-by-member
// walk leaves more than one reading of open, which maps by reading on to the
// one reading its own selected type classifies as it reads (walker.readAhead):
// xml is that reading's twin; other, where it is not empty, the twin of a
// reading its own selected type rules out, which otherWant measures. Under
// runSchema, tv's p alone, as V names it, selects P, which takes m after it
// as an attribute, and p and m together select T, which takes p as none, so
// the children's reading stands alone under T. tu's a = "z" selects T, which
// ends the run at m, though U takes m as an attribute: a and m together select
// T again, which takes m as no attribute. tq's Q takes m as an attribute in no
// namespace and Q2 in urn:s; m alone selects Q, which names it in no namespace,
// and beside a = "1" or "2" selects Q1, which does too, or Q2, which names it
// in urn:s. tn's m alone selects T, which takes it as no attribute, U taking it
// as one. Under laxExtensionSchema, U declines y and every key no declaration
// carries, and no a = "u" ever selects it: the reading taking y, and b, c and
// d, as attributes selects T, which takes y as no attribute, so the children's
// reading stands alone. Each row fails with walker.readAhead returning
// unsettled at once.
func TestTypeTableReadAhead(t *testing.T) {
	run := validatorOver(t, runSchema)
	lax := validatorOver(t, laxExtensionSchema)
	for _, tc := range []struct {
		name      string
		v         *validate.Validator
		json, xml string
		want      []xsderr.Rule
		other     string
		otherWant []xsderr.Rule
	}{
		{"an attribute under one type where the selected type ends the run", run, `{"tu":{"a":"z","m":"x"}}`,
			`<tu xmlns="urn:s" a="z"><m>x</m></tu>`, complexType, `<tu xmlns="urn:s" a="z" m="x"/>`, complexType},
		{"attributes of two expanded names", run, `{"tq":{"m":"x"}}`,
			`<tq xmlns="urn:s" m="x"/>`, none, `<tq xmlns="urn:s"><m>x</m></tq>`, complexType},
		{"attributes of two expanded names, after an attribute selecting the type naming one", run, `{"tq":{"a":"1","m":"x"}}`,
			`<tq xmlns="urn:s" a="1" m="x"/>`, none, "", nil},
		{"attributes of two expanded names, after an attribute selecting the type naming the other", run, `{"tq":{"a":"2","m":"x"}}`,
			`<tq xmlns="urn:s" xmlns:s="urn:s" a="2" s:m="x"/>`, none, "", nil},
		{"an attribute under one type where the selected type ends the run before it", run, `{"tn":{"m":"x"}}`,
			`<tn xmlns="urn:s"><m>x</m></tn>`, complexType, `<tn xmlns="urn:s" m="x"/>`, complexType},
		{"a candidate's names select a type taking the member after them as an attribute", run, `{"tv":{"p":"1","m":"x"}}`,
			`<tv xmlns="urn:s"><p>1</p><m>x</m></tv>`, []xsderr.Rule{"cvc-complex-type", "cvc-complex-type"}, `<tv xmlns="urn:s" p="1"><m>x</m></tv>`, complexType},
		{"another member follows the member", lax, `{"r":{"y":"1","z":"2"}}`,
			`<r><y>1</y><z>2</z></r>`, content, `<r y="1"><z>2</z></r>`, complexType},
		{"three members a candidate declines", lax, `{"r":{"b":"1","c":"2","d":"3","z":"4"}}`,
			`<r><b>1</b><c>2</c><d>3</d><z>4</z></r>`, content, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.other != "" {
				xres, err := xmlsrc.Validate(tc.v, strings.NewReader(tc.other), xmlsrc.WithURI("instance.xml"))
				if err != nil || xres.Err() != nil {
					t.Fatalf("xmlsrc.Validate(%s): %v / %v", tc.other, err, xres.Err())
				}
				if got := rules(xres); !slices.Equal(got, tc.otherWant) {
					t.Fatalf("XML twin %s charges %v, want %v", tc.other, got, tc.otherWant)
				}
			}
			twin(t, tc.v, tc.json, tc.xml, tc.want)
		})
	}
}

// repeatSchema declares, in urn:p, X, empty; A, which extends X with the
// attribute y in no namespace and a lax {attribute wildcard}; B, which extends
// X with the attribute y in urn:p; and D, which extends X with a lax element
// wildcard. e and d are of type X; each selects B where it has an attribute
// y in urn:p, and otherwise A and D respectively.
const repeatSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:p" xmlns:p="urn:p" elementFormDefault="qualified">
  <xs:complexType name="X"/>
  <xs:complexType name="A">
    <xs:complexContent><xs:extension base="p:X"><xs:attribute name="y" type="xs:string"/><xs:anyAttribute processContents="lax"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="B">
    <xs:complexContent><xs:extension base="p:X"><xs:attribute name="y" form="qualified" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="D">
    <xs:complexContent><xs:extension base="p:X"><xs:sequence><xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="e" type="p:X"><xs:alternative test="@p:y" type="p:B"/><xs:alternative type="p:A"/></xs:element>
  <xs:element name="d" type="p:X"><xs:alternative test="@p:y" type="p:B"/><xs:alternative type="p:D"/></xs:element>
</xs:schema>`

// TestTypeTableReadAheadDeclines pins walker.readAhead's declines, each
// beside the verdicts the engine gives the twins of the readings in play,
// withheld saying whether it withholds a selection for one: under runSchema,
// t2's two readings each stand, the children's under T and the attribute's
// under U; tz's attribute reading reaches a {test} outside the §3.12.6
// compiler's subset; tp's p selects P, which takes the null m after it as an
// attribute, where the children's reading stands under T; and no reading of
// t0's m stands, m alone selecting Q, which takes it as an attribute, m in no
// namespace Q2, which names it in urn:s, and m in urn:s Q1, which names it in
// none. Under laxExtensionSchema, y and a together select U, which declines y
// (walker.candidates' fallback, undetermined); four members U declines make
// more name assignments than nameBudget; and where the one reading standing
// leaves an attribute member after a child, it declines as one read from the
// stream does (members.key). Under repeatSchema, the one reading of e's y and
// p:y that B classifies as it reads names both {urn:p}y, and d's p:y and q:y,
// both {urn:p}y, are such a reading under B beside the children's reading,
// which stands under D: neither repeating reading has an XML twin.
func TestTypeTableReadAheadDeclines(t *testing.T) {
	run := validatorOver(t, runSchema)
	lax := validatorOver(t, laxExtensionSchema)
	rep := validatorOver(t, repeatSchema)
	type measured struct {
		doc      string
		want     []xsderr.Rule
		withheld bool
	}
	const undetermined = ` is declined under a type its {type table} can select`
	for _, tc := range []struct {
		name  string
		v     *validate.Validator
		json  string
		twins []measured
		opens string
	}{
		{"two readings stand", run, `{"t2":{"m":"x"}}`,
			[]measured{{`<t2 xmlns="urn:s"><m>x</m></t2>`, complexType, false}, {`<t2 xmlns="urn:s" m="x"/>`, none, false}},
			`jsonsrc: instance.json:1:8: member "m" of {urn:s}t2 is an attribute under a type its {type table} can select and a child under the one`},
		{"the engine withholds a longer reading's selection", run, `{"tz":{"m":"x"}}`,
			[]measured{{`<tz xmlns="urn:s"><m>x</m></tz>`, complexType, false}, {`<tz xmlns="urn:s" m="x"/>`, none, true}},
			`jsonsrc: instance.json:1:8: member "m" of {urn:s}tz is an attribute under a type its {type table} can select and a child under the one`},
		{"a longer reading's type takes a null member as an attribute", run, `{"tp":{"p":"1","m":null}}`,
			[]measured{{`<tp xmlns="urn:s" ` + xsiDecl + `><p>1</p><m xsi:nil="true"/></tp>`, []xsderr.Rule{"cvc-complex-type", "cvc-complex-type"}, false}, {`<tp xmlns="urn:s" ` + xsiDecl + ` p="1"><m xsi:nil="true"/></tp>`, complexType, false}},
			`jsonsrc: instance.json:1:8: member "p" of {urn:s}tp is an attribute under a type its {type table} can select and a child under the one`},
		{"no reading stands", run, `{"t0":{"m":"x"}}`,
			[]measured{{`<t0 xmlns="urn:s"><m>x</m></t0>`, complexType, false}, {`<t0 xmlns="urn:s" m="x"/>`, none, false}, {`<t0 xmlns="urn:s" xmlns:s="urn:s" s:m="x"/>`, complexType, false}},
			`jsonsrc: instance.json:1:8: member "m" of {urn:s}t0 is an attribute of the expanded names`},
		{"a longer reading's type declines the member", lax, `{"r":{"y":"1","a":"u"}}`,
			[]measured{{`<r><y>1</y><a>u</a></r>`, content, false}, {`<r y="1" a="u"/>`, content, false}},
			`jsonsrc: instance.json:1:7: member "y" of r` + undetermined},
		{"four members a candidate declines", lax, `{"r":{"b":"1","c":"2","d":"3","e":"4","z":"5"}}`,
			[]measured{{`<r><b>1</b><c>2</c><d>3</d><e>4</e><z>5</z></r>`, content, false}},
			`jsonsrc: instance.json:1:7: member "b" of r` + undetermined},
		{"an attribute member after a child the reading leaves", lax, `{"r":{"y":"1","a":"t","z":"2"}}`,
			[]measured{{`<r a="t"><y>1</y><z>2</z></r>`, content, false}},
			`jsonsrc: instance.json:1:15: attribute member "a" follows a child member of r`},
		{"the one reading standing repeats an expanded name", rep, `{"e":{"$xmlns":{"p":"urn:p"},"y":"1","p:y":"2"}}`,
			[]measured{
				{`<e xmlns="urn:p" xmlns:p="urn:p"><y>1</y><p:y>2</p:y></e>`, complexType, false},
				{`<e xmlns="urn:p" xmlns:p="urn:p" y="1"><p:y>2</p:y></e>`, complexType, false},
				{`<e xmlns="urn:p" xmlns:p="urn:p" p:y="1"><p:y>2</p:y></e>`, complexType, false},
				{`<e xmlns="urn:p" xmlns:p="urn:p" y="1" p:y="2"/>`, complexType, false}},
			`jsonsrc: instance.json:1:30: member "y" of {urn:p}e is an attribute of the expanded names`},
		{"a reading repeats an expanded name beside one that stands", rep, `{"d":{"$xmlns":{"p":"urn:p","q":"urn:p"},"p:y":"1","q:y":"2"}}`,
			[]measured{
				{`<d xmlns="urn:p" xmlns:p="urn:p" xmlns:q="urn:p"><p:y>1</p:y><q:y>2</q:y></d>`, none, false},
				{`<d xmlns="urn:p" xmlns:p="urn:p" xmlns:q="urn:p" p:y="1"><q:y>2</q:y></d>`, complexType, false}},
			`jsonsrc: instance.json:1:42: member "p:y" of {urn:p}d is an attribute under a type its {type table} can select and a child under the one`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, x := range tc.twins {
				xres, err := xmlsrc.Validate(tc.v, strings.NewReader(x.doc), xmlsrc.WithURI("instance.xml"))
				if err != nil || xres.Err() != nil {
					t.Fatalf("xmlsrc.Validate(%s): %v / %v", x.doc, err, xres.Err())
				}
				if got := rules(xres); !slices.Equal(got, x.want) || (len(xres.Unevaluated()) > 0) != x.withheld {
					t.Fatalf("XML twin %s charges %v, %d unevaluated; want %v, withheld %t", x.doc, got, len(xres.Unevaluated()), x.want, x.withheld)
				}
			}
			res, err := Validate(tc.v, strings.NewReader(tc.json), WithURI(jsonURI))
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

// laxExtensionSchema declares, in no namespace, T, an optional child z and the
// attribute a, and U, which extends T with a lax element wildcard and a lax
// {attribute wildcard}, under both of which an unprefixed key's kind and
// namespace are open (walker.wildcardMember). r is of type T and selects U
// where its a is "u". Each of r3, r4, r5 and r6 is of type T and selects U
// where it has the attribute y in one class of names attributeNames tells
// apart: in no namespace, in urn:p, which its {test} binds to p, in a
// namespace its {test} binds none to, and in the xml namespace. r7 selects U
// where it has no a, r9 where its a is counted, a {test} outside the §3.12.6
// compiler's subset, and r10 where it has a y and no a.
const laxExtensionSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="T">
    <xs:sequence><xs:element name="z" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="U">
    <xs:complexContent><xs:extension base="T">
      <xs:sequence><xs:any processContents="lax"/></xs:sequence>
      <xs:anyAttribute processContents="lax"/>
    </xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="r" type="T"><xs:alternative test="@a = 'u'" type="U"/></xs:element>
  <xs:element name="r3" type="T"><xs:alternative test="@y" type="U"/></xs:element>
  <xs:element name="r4" type="T" xmlns:p="urn:p"><xs:alternative test="@p:y" type="U"/></xs:element>
  <xs:element name="r5" type="T"><xs:alternative test="@*:y and not(@y) and not(@xml:y)" type="U"/></xs:element>
  <xs:element name="r6" type="T"><xs:alternative test="@xml:y" type="U"/></xs:element>
  <xs:element name="r7" type="T"><xs:alternative test="not(@a)" type="U"/></xs:element>
  <xs:element name="r9" type="T"><xs:alternative test="count(@a) > 0" type="U"/></xs:element>
  <xs:element name="r10" type="T"><xs:alternative test="@y and not(@a)" type="U"/></xs:element>
</xs:schema>`

// unboundSchema declares, in no namespace and with the XSD namespace the
// default one, so that a {test}'s {namespace bindings} bind the xml namespace
// alone, r8 of xs:anyType, which selects a type like laxExtensionSchema's U
// where it has the attribute y in a namespace its {test} binds none to, and
// otherwise one like its T.
const unboundSchema = `<schema xmlns="http://www.w3.org/2001/XMLSchema">
  <element name="r8">
    <alternative test="@*:y and not(@y) and not(@xml:y)">
      <complexType><sequence><any processContents="lax"/></sequence><anyAttribute processContents="lax"/></complexType>
    </alternative>
    <alternative>
      <complexType><sequence><element name="z" minOccurs="0"/></sequence><attribute name="a" type="string"/></complexType>
    </alternative>
  </element>
</schema>`

// settleSchema declares, in no namespace, V, empty content with the attribute
// a; W, which restricts V, prohibiting a; and X, which extends V with the
// attribute b, a lax element wildcard and a lax {attribute wildcard}. q is of
// type V and selects W where it has an a and no b, X where it has a zzz, and
// V otherwise.
const settleSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="V"><xs:attribute name="a" type="xs:string"/></xs:complexType>
  <xs:complexType name="W">
    <xs:complexContent><xs:restriction base="V"><xs:attribute name="a" use="prohibited"/></xs:restriction></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="X">
    <xs:complexContent><xs:extension base="V">
      <xs:sequence><xs:any processContents="lax"/></xs:sequence>
      <xs:attribute name="b" type="xs:string"/>
      <xs:anyAttribute processContents="lax"/>
    </xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="q" type="V">
    <xs:alternative test="@a and not(@b)" type="W"/>
    <xs:alternative test="@zzz" type="X"/>
  </xs:element>
</xs:schema>`

// anyWildcardSchema declares, in no namespace, T, a lax ##any element wildcard
// and the attribute a; U, which extends T with the attribute y of xs:int; and
// V, which restricts T to a child y. r is of type T and selects U where it has
// a y; r2 is of type T and selects U where it has a y, else V where its a is
// "v".
const anyWildcardSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="T">
    <xs:sequence><xs:any namespace="##any" processContents="lax" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="U">
    <xs:complexContent><xs:extension base="T"><xs:attribute name="y" type="xs:int"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="V">
    <xs:complexContent><xs:restriction base="T">
      <xs:sequence><xs:element name="y" minOccurs="0"/></xs:sequence>
    </xs:restriction></xs:complexContent>
  </xs:complexType>
  <xs:element name="r" type="T"><xs:alternative test="@y" type="U"/></xs:element>
  <xs:element name="r2" type="T">
    <xs:alternative test="@y" type="U"/>
    <xs:alternative test="@a = 'v'" type="V"/>
  </xs:element>
</xs:schema>`

// TestTypeTableCandidateDrops pins a root whose candidate type declines a
// member of the attribute run where no reading of the run selects a type that
// declines it (walker.candidates, walker.droppable): under
// laxExtensionSchema, y is a child no wildcard names under T and declines
// under U, whose two wildcards leave its kind open; no reading selects U, a
// never being "u", so U drops and the root maps as T, the one type any
// reading selects, classifies it. Under r3, whose y selects U where it is an
// attribute, a null y is no attribute under any reading, so the one that
// takes it as an attribute is never read. Each row's y closes the object;
// TestTypeTableReadAhead pins a y another member follows.
func TestTypeTableCandidateDrops(t *testing.T) {
	v := validatorOver(t, laxExtensionSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"a later candidate declines a member after an attribute", `{"r":{"a":"t","y":"1"}}`,
			`<r a="t"><y>1</y></r>`, content},
		{"a later candidate declines the run's first member", `{"r":{"y":"1"}}`,
			`<r><y>1</y></r>`, content},
		{"a member no attribute reading can take", `{"r3":{"y":null}}`,
			`<r3 ` + xsiDecl + `><y xsi:nil="true"/></r3>`, content},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestTypeTableCandidateDeclines pins walker.candidates' GAP(jsonsrc) markers:
// a member of a root's attribute run that one live candidate type declines
// declines the whole root where a reading of the run may select that type,
// each twin's verdict measured beside the decline; xml is the child reading's
// twin and attr the attribute reading's, which attrWant measures. Under
// anyWildcardSchema, y declines under T, the declared {type definition},
// whose element wildcard of variety any fixes no namespace for it, alone
// through wildcardMember's own exit, and T is the type the reading that takes
// no attribute selects. Under laxExtensionSchema, y declines under U alone
// (walker.undetermined): r3 to r6 select U under the reading that takes y as
// an attribute, each where y is in one class of names attributeNames gives
// it; r7 selects U under the reading that takes no attribute, though that
// reading is inconsistent, U taking a as an attribute, and the one reading
// that is, T's taking a alone, would map; r10 selects U under the reading
// that takes y alone as an attribute, though another member follows and that
// reading is inconsistent, U taking a as an attribute, and the one reading
// that is, the children's, would map; r9's {test} is outside the §3.12.6
// compiler's subset, so the engine withholds every reading's selection, and
// charges neither twin. Under unboundSchema, r8 selects the type that
// declines y where y is in a namespace no {test} binds, and the declared
// xs:anyType, which no reading selects, declines first. Under settleSchema,
// every live candidate declines y, X alone having taken both a and b as
// attributes, and no reading selects X, so the run ends at y under the type a
// and b select, which classifies b as no attribute: walker.settled's #2701
// decline, no reading being consistent.
func TestTypeTableCandidateDeclines(t *testing.T) {
	lax := validatorOver(t, laxExtensionSchema)
	anyWildcard := validatorOver(t, anyWildcardSchema)
	unbound := validatorOver(t, unboundSchema)
	settle := validatorOver(t, settleSchema)
	twoComplexType := []xsderr.Rule{"cvc-complex-type", "cvc-complex-type"}
	const wildcardDecline = `names no child element declaration and no attribute use of the type of `
	const undetermined = ` is declined under a type its {type table} can select`
	for _, tc := range []struct {
		name     string
		v        *validate.Validator
		json     string
		xml      string
		want     []xsderr.Rule
		attr     string
		attrWant []xsderr.Rule
		opens    string
	}{
		{"the declared type declines a member alone", anyWildcard, `{"r":{"y":"x"}}`,
			`<r><y>x</y></r>`, none, `<r y="x"/>`, []xsderr.Rule{"cvc-attribute"},
			`jsonsrc: instance.json:1:7: member "y" ` + wildcardDecline + `r, and its element wildcard set includes one of variety any`},
		{"the declared type declines a member alone, beside two alternatives", anyWildcard, `{"r2":{"y":"x"}}`,
			`<r2><y>x</y></r2>`, none, `<r2 y="x"/>`, []xsderr.Rule{"cvc-attribute"},
			`jsonsrc: instance.json:1:8: member "y" ` + wildcardDecline + `r2, and its element wildcard set includes one of variety any`},
		{"the attribute reading selects the declining type", lax, `{"r3":{"y":"1"}}`,
			`<r3><y>1</y></r3>`, content, `<r3 y="1"/>`, content,
			`jsonsrc: instance.json:1:8: member "y" of r3` + undetermined},
		{"the attribute reading selects it in a namespace the test binds", lax, `{"r4":{"y":"1"}}`,
			`<r4><y>1</y></r4>`, content, `<r4 xmlns:p="urn:p" p:y="1"/>`, content,
			`jsonsrc: instance.json:1:8: member "y" of r4` + undetermined},
		{"the attribute reading selects it in a namespace the test binds none to", lax, `{"r5":{"y":"1"}}`,
			`<r5><y>1</y></r5>`, content, `<r5 xmlns:q="urn:q" q:y="1"/>`, content,
			`jsonsrc: instance.json:1:8: member "y" of r5` + undetermined},
		{"the attribute reading selects it in the xml namespace", lax, `{"r6":{"y":"1"}}`,
			`<r6><y>1</y></r6>`, content, `<r6 xml:y="1"/>`, content,
			`jsonsrc: instance.json:1:8: member "y" of r6` + undetermined},
		{"the attribute reading selects it in a namespace no test binds", unbound, `{"r8":{"y":"1"}}`,
			`<r8><y>1</y></r8>`, content, `<r8 xmlns:q="urn:q" q:y="1"/>`, content,
			`jsonsrc: instance.json:1:8: member "y" names no child element declaration and no attribute use of the type of r8, which has both`},
		{"an earlier reading selects the declining type", lax, `{"r7":{"a":"t","y":"1"}}`,
			`<r7 a="t"><y>1</y></r7>`, content, `<r7 a="t" y="1"/>`, complexType,
			`jsonsrc: instance.json:1:16: member "y" of r7` + undetermined},
		{"the attribute reading selects the declining type, another member following", lax, `{"r10":{"y":"1","a":"t"}}`,
			`<r10><y>1</y><a>t</a></r10>`, content, `<r10 y="1"><a>t</a></r10>`, none,
			`jsonsrc: instance.json:1:9: member "y" of r10` + undetermined},
		{"the engine withholds a reading's selection", lax, `{"r9":{"y":"1"}}`,
			`<r9><y>1</y></r9>`, none, `<r9 y="1"/>`, none,
			`jsonsrc: instance.json:1:8: member "y" of r9` + undetermined},
		{"every live candidate declines the member", settle, `{"q":{"a":"1","b":"2","y":"3"}}`,
			`<q a="1" b="2"><y>3</y></q>`, twoComplexType, `<q a="1" b="2" y="3"/>`, twoComplexType,
			`jsonsrc: instance.json:1:23: the members of q settle no type`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, x := range []struct {
				doc  string
				want []xsderr.Rule
			}{{tc.xml, tc.want}, {tc.attr, tc.attrWant}} {
				xres, err := xmlsrc.Validate(tc.v, strings.NewReader(x.doc), xmlsrc.WithURI("instance.xml"))
				if err != nil || xres.Err() != nil {
					t.Fatalf("xmlsrc.Validate(%s): %v / %v", x.doc, err, xres.Err())
				}
				if got := rules(xres); !slices.Equal(got, x.want) {
					t.Fatalf("XML twin %s charges %v, want %v", x.doc, got, x.want)
				}
			}
			res, err := Validate(tc.v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, nil error; want the decline", tc.json, rules(res))
			}
			if !strings.HasPrefix(err.Error(), tc.opens) {
				t.Errorf("Validate(%s) = %q, want it to open %q", tc.json, err, tc.opens)
			}
		})
	}
}
