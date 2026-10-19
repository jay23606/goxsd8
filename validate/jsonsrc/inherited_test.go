package jsonsrc

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsderr"
)

// inheritSchema declares, in urn:i, the top-level attribute ia, inheritable; T, an optional child m of
// xs:int, the attribute a and a lax ##local {attribute wildcard}; and U, which restricts T to no child
// and the attribute m of xs:string. Under T a key "m" is the child, whose "x" charges cvc-type, and
// under U the attribute; an infoset built under the type the engine does not select charges otherwise —
// a child m under U charges cvc-complex-type, and an attribute m under T, which its wildcard admits,
// nothing — so each row's verdict tells the two selections apart. tu is of type T and selects U where
// its a is "u", ti where its i:ia is "u", and tb where its i:ib is "u"; no top-level ib exists. p holds
// an optional p and then an optional tu, and carries a, inheritable; pd carries a, inheritable,
// defaulting to "u"; pn carries a, not inheritable, and pnd carries it defaulting to "u"; each holds an
// optional tu. pw holds an optional ti and an optional tb under a lax {attribute wildcard} of urn:i, and
// pk holds an optional ti under a skip one. pt is of type P0, which holds an optional tu, and PT extends
// P0 with a, inheritable. po holds an optional x of type T that selects U where its a is "u", and has a
// lax interleave {open content}; ps is po with a skip one. pr carries a, required, inheritable and fixed
// to "u", and holds an optional tu. tn is of type T and selects U where its xsi:nil is "true", and px
// holds an optional tn and carries xsi:nil, inheritable, defaulting to "true". wy holds an optional y of
// type Yd and then a lax wildcard, and the top-level y is of type Yg: each holds an optional x of type
// T, Yg's selecting U where its a is "u". mid, and mid2 of type M, hold an optional tu. H holds an
// optional mid and an optional mid2 and carries a; H2 extends it with ib, inheritable, defaulting to
// "v", and H3 with ic, not inheritable; H4 extends H2 with nothing. rw is of type H and selects H2 where
// it counts an a, a {test} outside the §3.12.6 compiler's subset, and re selects H3 likewise. B holds an
// optional tu and carries k; X holds an optional tu and carries a, inheritable, defaulting to "u", and
// derives from no B; X2 extends B with that a, and N extends B with nothing. e is of type B, r holds an
// e, and et is of type B and selects N where its k is "n"; pq holds one q of type B. Z0 holds an
// optional tu, and Z1 extends it with a, inheritable, defaulting to "z"; z is of type Z0 and selects Z1
// where its a is "u", and pa holds an optional z and carries a, inheritable. O1 holds an optional c of
// type T under a lax interleave {open content} and carries a, and O2 restricts it to no {open content},
// its c blocking restriction; ro is of type O1 and selects O2 where it counts an a. big's {content type}
// is one xsd.Schema.ContentMatcher declines (its GAP(xsd) ceiling), holding mids.
const inheritSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" targetNamespace="urn:i" xmlns:i="urn:i" elementFormDefault="qualified">
  <xs:attribute name="ia" type="xs:string" inheritable="true"/>
  <xs:complexType name="T">
    <xs:sequence><xs:element name="m" type="xs:int" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
    <xs:anyAttribute namespace="##local" processContents="lax"/>
  </xs:complexType>
  <xs:complexType name="U">
    <xs:complexContent>
      <xs:restriction base="i:T"><xs:sequence/><xs:attribute name="m" type="xs:string"/></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="tu" type="i:T"><xs:alternative test="@a = 'u'" type="i:U"/></xs:element>
  <xs:element name="ti" type="i:T"><xs:alternative test="@i:ia = 'u'" type="i:U"/></xs:element>
  <xs:element name="tb" type="i:T"><xs:alternative test="@i:ib = 'u'" type="i:U"/></xs:element>
  <xs:element name="p">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:p" minOccurs="0"/><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
      <xs:attribute name="a" type="xs:string" inheritable="true"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pd">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
      <xs:attribute name="a" type="xs:string" default="u" inheritable="true"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pn">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
      <xs:attribute name="a" type="xs:string"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pnd">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
      <xs:attribute name="a" type="xs:string" default="u"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pw">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:ti" minOccurs="0"/><xs:element ref="i:tb" minOccurs="0"/></xs:sequence>
      <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pk">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:ti" minOccurs="0"/></xs:sequence>
      <xs:anyAttribute namespace="##targetNamespace" processContents="skip"/>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="P0"><xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="PT">
    <xs:complexContent><xs:extension base="i:P0"><xs:attribute name="a" type="xs:string" inheritable="true"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="pt" type="i:P0"/>
  <xs:element name="po">
    <xs:complexType>
      <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
      <xs:sequence><xs:element name="x" type="i:T" minOccurs="0"><xs:alternative test="@a = 'u'" type="i:U"/></xs:element></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="ps">
    <xs:complexType>
      <xs:openContent mode="interleave"><xs:any processContents="skip"/></xs:openContent>
      <xs:sequence><xs:element name="x" type="i:T" minOccurs="0"><xs:alternative test="@a = 'u'" type="i:U"/></xs:element></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="pr">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
      <xs:attribute name="a" type="xs:string" use="required" fixed="u" inheritable="true"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="tn" type="i:T"><xs:alternative test="@xsi:nil = 'true'" type="i:U"/></xs:element>
  <xs:element name="px">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:tn" minOccurs="0"/></xs:sequence>
      <xs:attribute ref="xsi:nil" default="true" inheritable="true"/>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="Yd"><xs:sequence><xs:element name="x" type="i:T" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Yg">
    <xs:sequence><xs:element name="x" type="i:T" minOccurs="0"><xs:alternative test="@a = 'u'" type="i:U"/></xs:element></xs:sequence>
  </xs:complexType>
  <xs:element name="y" type="i:Yg"/>
  <xs:element name="wy">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="y" type="i:Yd" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="mid"><xs:complexType><xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>
  <xs:complexType name="M"><xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:element name="mid2" type="i:M"/>
  <xs:complexType name="H">
    <xs:sequence><xs:element ref="i:mid" minOccurs="0"/><xs:element ref="i:mid2" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="H2">
    <xs:complexContent><xs:extension base="i:H"><xs:attribute name="ib" type="xs:string" default="v" inheritable="true"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="H3">
    <xs:complexContent><xs:extension base="i:H"><xs:attribute name="ic" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="H4"><xs:complexContent><xs:extension base="i:H2"/></xs:complexContent></xs:complexType>
  <xs:element name="rw" type="i:H"><xs:alternative test="count(@a) > 0" type="i:H2"/></xs:element>
  <xs:element name="re" type="i:H"><xs:alternative test="count(@a) > 0" type="i:H3"/></xs:element>
  <xs:complexType name="B">
    <xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
    <xs:attribute name="k" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="X">
    <xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string" default="u" inheritable="true"/>
  </xs:complexType>
  <xs:complexType name="X2">
    <xs:complexContent><xs:extension base="i:B"><xs:attribute name="a" type="xs:string" default="u" inheritable="true"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="N"><xs:complexContent><xs:extension base="i:B"/></xs:complexContent></xs:complexType>
  <xs:element name="e" type="i:B"/>
  <xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="i:e"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="et" type="i:B"><xs:alternative test="@k = 'n'" type="i:N"/></xs:element>
  <xs:element name="pq"><xs:complexType><xs:sequence><xs:element name="q" type="i:B"/></xs:sequence></xs:complexType></xs:element>
  <xs:complexType name="Z0"><xs:sequence><xs:element ref="i:tu" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Z1">
    <xs:complexContent><xs:extension base="i:Z0"><xs:attribute name="a" type="xs:string" default="z" inheritable="true"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="z" type="i:Z0"><xs:alternative test="@a = 'u'" type="i:Z1"/></xs:element>
  <xs:complexType name="O1">
    <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
    <xs:sequence><xs:element name="c" type="i:T" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="O2">
    <xs:complexContent>
      <xs:restriction base="i:O1">
        <xs:sequence><xs:element name="c" type="i:T" minOccurs="0" block="restriction"/></xs:sequence>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="ro" type="i:O1"><xs:alternative test="count(@a) > 0" type="i:O2"/></xs:element>
  <xs:element name="pa">
    <xs:complexType>
      <xs:sequence><xs:element ref="i:z" minOccurs="0"/></xs:sequence>
      <xs:attribute name="a" type="xs:string" inheritable="true"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="big">
    <xs:complexType>
      <xs:sequence>
        <xs:sequence maxOccurs="1000"><xs:element ref="i:mid" maxOccurs="unbounded"/></xs:sequence>
        <xs:element name="b" type="xs:string"/>
        <xs:sequence maxOccurs="1000"><xs:element name="c" maxOccurs="unbounded"/></xs:sequence>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// TestInheritedSelected pins the [inherited attributes] the selection of a
// {type table} below the root reads (walker.handDown, walker.selector): an
// ancestor's attribute that is ·potentially inherited· (key-p-inherited)
// decides the type tu or ti selects, and so whether m is its attribute or its
// child, as the engine selects it for each XML twin (key-cta-ta-select
// clause 1.1.3). The rows read, in turn: a specified attribute whose use is
// inheritable (clause 3.1); one ·defaulted· from such a use; one whose use is
// not inheritable, specified and ·defaulted·; the nearer of two same-named
// inheritable ones (e-inherited_attributes clause 2), in either order; the
// element's own attribute over an inherited one of its name
// (key-cta-ta-select clause 1.1.3); an attribute an {attribute wildcard}
// admits, whose top-level declaration is inheritable (clause 3.2); the same
// attribute ·skipped· by a skip wildcard; one a lax wildcard admits with no
// declaration; an attribute whose use the type a "$type" names, which
// ·overrides· the declared one, makes inheritable; an attribute specified
// where its use defaults, which it is not ·defaulted· beside; a required use
// absent, which no default supplies (key-dflt-att clause 2); an xsi: use's
// default, which key-dflt-att clause 4 supplies none of; and the second y of
// wy, ·attributed to· the wildcard and governed by the top-level y's Yg, whose
// x's {type table} the engine reads, where its members are classified under
// Yd's x, which carries none (walker.selector); Yg not deriving from Yd, the
// y's ·locally declared type·, the engine charges y cvc-complex-type clause 5.
func TestInheritedSelected(t *testing.T) {
	v := validatorOver(t, inheritSchema)
	const i = `xmlns="urn:i" xmlns:i="urn:i" `
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"an inheritable attribute", `{"p":{"a":"u","tu":{"m":"x"}}}`,
			`<p ` + i + `a="u"><tu m="x"/></p>`, none},
		{"a defaulted inheritable attribute", `{"pd":{"tu":{"m":"x"}}}`,
			`<pd ` + i + `><tu m="x"/></pd>`, none},
		{"an attribute that is not inheritable", `{"pn":{"a":"u","tu":{"m":"x"}}}`,
			`<pn ` + i + `a="u"><tu><m>x</m></tu></pn>`, typ},
		{"a defaulted attribute that is not inheritable", `{"pnd":{"tu":{"m":"x"}}}`,
			`<pnd ` + i + `><tu><m>x</m></tu></pnd>`, typ},
		{"the nearer of two inheritable attributes", `{"p":{"a":"z","p":{"a":"u","tu":{"m":"x"}}}}`,
			`<p ` + i + `a="z"><p a="u"><tu m="x"/></p></p>`, none},
		{"the nearer of two inheritable attributes, the farther selecting", `{"p":{"a":"u","p":{"a":"z","tu":{"m":"x"}}}}`,
			`<p ` + i + `a="u"><p a="z"><tu><m>x</m></tu></p></p>`, typ},
		{"the element's own attribute over an inherited one", `{"p":{"a":"u","tu":{"a":"z","m":"x"}}}`,
			`<p ` + i + `a="u"><tu a="z"><m>x</m></tu></p>`, typ},
		{"a wildcard's attribute whose declaration is inheritable", `{"pw":{"$xmlns":{"i":"urn:i"},"i:ia":"u","ti":{"m":"x"}}}`,
			`<pw ` + i + `i:ia="u"><ti m="x"/></pw>`, none},
		{"a skipped attribute", `{"pk":{"$xmlns":{"i":"urn:i"},"i:ia":"u","ti":{"m":"x"}}}`,
			`<pk ` + i + `i:ia="u"><ti><m>x</m></ti></pk>`, typ},
		{"a wildcard's attribute with no declaration", `{"pw":{"$xmlns":{"i":"urn:i"},"i:ib":"u","tb":{"m":"x"}}}`,
			`<pw ` + i + `i:ib="u"><tb><m>x</m></tb></pw>`, typ},
		{"an inheritable attribute of the type a \"$type\" names", `{"pt":{"$xmlns":{"i":"urn:i"},"$type":"i:PT","a":"u","tu":{"m":"x"}}}`,
			`<pt ` + i + xsiDecl + ` xsi:type="i:PT" a="u"><tu m="x"/></pt>`, none},
		{"a specified attribute whose use defaults", `{"pd":{"a":"z","tu":{"m":"x"}}}`,
			`<pd ` + i + `a="z"><tu><m>x</m></tu></pd>`, typ},
		{"a required attribute absent", `{"pr":{"tu":{"m":"x"}}}`,
			`<pr ` + i + `><tu><m>x</m></tu></pr>`, []xsderr.Rule{"cvc-complex-type", "cvc-type"}},
		{"an xsi: attribute use's default", `{"px":{"tn":{"m":"x"}}}`,
			`<px ` + i + `><tn><m>x</m></tn></px>`, typ},
		{"a governing element declaration other than the classifying one", `{"wy":{"y":[{},{"x":{"a":"u","m":"x"}}]}}`,
			`<wy ` + i + `><y/><y><x a="u" m="x"/></y></wy>`, complexType},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestOverriddenTypeHandsDown pins the [inherited attributes] an element
// carrying a "$type" hands down (walker.instanceGoverning): under the type
// that governs it as the engine determines it, which is the type the "$type"
// names only where that type ·overrides· the ·selected type definition· of
// its ·governing element declaration· (key-governing-type-elem clauses 3 and
// 4), which its members are classified under too (walker.instanceGoverned). X
// does not ·override· B, so e is charged cvc-elt clause 4 and governed by B, which
// hands tu no a, and tu, governed by T, takes m as a child, at the root and below it.
// X2 ·overrides· B, the type et selects without a k, and hands down its ·defaulted· a,
// but not N, the one it selects where its k is "n", which then governs et
// (walker.overrode). The second q of pq, which pq's content model rejects, has
// no ·governing element declaration·, and X does not ·override· its ·locally declared
// type· B, which governs it (key-governing-type-elem clause 7). The z of pa reads
// pa's a, "u", among its [inherited attributes] and selects Z1, which Z0 does not
// ·override· (cvc-elt clause 4), so Z1 governs z and hands tu its ·defaulted· a,
// "z", the nearer, and tu, governed by T, takes m as a child (walker.overrode).
func TestOverriddenTypeHandsDown(t *testing.T) {
	v := validatorOver(t, inheritSchema)
	const i = `xmlns="urn:i" xmlns:i="urn:i" `
	charged := []xsderr.Rule{"cvc-elt", "cvc-type"}
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"charged at the root", `{"e":{"$xmlns":{"i":"urn:i"},"$type":"i:X","tu":{"m":"x"}}}`,
			`<e ` + i + xsiDecl + ` xsi:type="i:X"><tu><m>x</m></tu></e>`, charged},
		{"charged below the root", `{"r":{"e":{"$xmlns":{"i":"urn:i"},"$type":"i:X","tu":{"m":"x"}}}}`,
			`<r ` + i + xsiDecl + `><e xsi:type="i:X"><tu><m>x</m></tu></e></r>`, charged},
		{"overriding the selected type", `{"et":{"$xmlns":{"i":"urn:i"},"$type":"i:X2","tu":{"m":"x"}}}`,
			`<et ` + i + xsiDecl + ` xsi:type="i:X2"><tu m="x"/></et>`, none},
		{"not overriding the selected type", `{"et":{"$xmlns":{"i":"urn:i"},"$type":"i:X2","k":"n","tu":{"m":"x"}}}`,
			`<et ` + i + xsiDecl + ` xsi:type="i:X2" k="n"><tu><m>x</m></tu></et>`, charged},
		{"a locally declared type it does not override", `{"pq":{"q":[{},{"$xmlns":{"i":"urn:i"},"$type":"i:X","tu":{"m":"x"}}]}}`,
			`<pq ` + i + xsiDecl + `><q/><q xsi:type="i:X"><tu><m>x</m></tu></q></pq>`, []xsderr.Rule{"cvc-complex-content", "cvc-type"}},
		{"a selection read with inherited attributes", `{"pa":{"a":"u","z":{"$xmlns":{"i":"urn:i"},"$type":"i:Z0","tu":{"m":"x"}}}}`,
			`<pa ` + i + xsiDecl + ` a="u"><z xsi:type="i:Z0"><tu><m>x</m></tu></z></pa>`, charged},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestNoGoverningDeclaration pins walker.selector's arm for an element its
// parent's ·governing type definition· leaves with no ·governing element
// declaration·: the second x of po is ·attributed to· the lax {open content}
// and governed by its ·locally declared type· T (key-governing-ed clause 4.3,
// key-governing-type-elem clause 7), and the second x of ps is ·skipped· by
// the skip one, so the engine reads no {type table} for either, and the
// mapping classifies the members under the declaration's T alone: a an
// attribute and m a child, where U would take m as an attribute.
func TestNoGoverningDeclaration(t *testing.T) {
	v := validatorOver(t, inheritSchema)
	const i = `xmlns="urn:i" `
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"a locally declared type", `{"po":{"x":[{},{"a":"u","m":"x"}]}}`,
			`<po ` + i + `><x/><x a="u"><m>x</m></x></po>`, typ},
		{"skipped", `{"ps":{"x":[{},{"a":"u","m":"x"}]}}`,
			`<ps ` + i + `><x/><x a="u"><m>x</m></x></ps>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestInheritedUndetermined pins walker.classify's GAP(jsonsrc) markers: where
// the mapping determines no [inherited attributes] for tu, it reads no
// selection of its {type table} (walker.selector), and m, a child under T and
// an attribute under U, declines, where the engine walks each XML twin's
// subtree undecided. rw's selection is withheld, and the types it may select,
// H and H2, hand down different attributes, H2 a ·defaulted· ib (walker.handDown),
// so neither mid nor mid2, governed by M alone, hands down any determined; big's
// content model is one xsd.Schema.ContentMatcher declines, so mid is attributed to
// nothing. It pins walker.instanceGoverned's marker of the same
// ruling too: ro's selection is withheld, and c is ·attributed· to an element
// particle of type T under either, which its "$type" U ·overrides· under O1's, so
// U governs it, and not under O2's, which blocks restriction, so T governs it,
// cvc-elt clause 4 charged: the "$type" settles differently under the two
// (walker.governedTypes).
func TestInheritedUndetermined(t *testing.T) {
	v := validatorOver(t, inheritSchema)
	for _, tc := range []struct {
		name, json, xml string
		opens           string
	}{
		{"below a withheld selection", `{"rw":{"a":"1","mid":{"tu":{"a":"u","m":"x"}}}}`,
			`<rw xmlns="urn:i" a="1"><mid><tu a="u" m="x"/></mid></rw>`,
			`jsonsrc: instance.json:1:37: member "m" of {urn:i}tu is not classified alike`},
		{"below a withheld selection, under one named type", `{"rw":{"a":"1","mid2":{"tu":{"a":"u","m":"x"}}}}`,
			`<rw xmlns="urn:i" a="1"><mid2><tu a="u" m="x"/></mid2></rw>`,
			`jsonsrc: instance.json:1:38: member "m" of {urn:i}tu is not classified alike`},
		{"a \"$type\" below a withheld selection", `{"ro":{"a":"1","c":{"$xmlns":{"i":"urn:i"},"$type":"i:U"}}}`,
			`<ro xmlns="urn:i" xmlns:i="urn:i" ` + xsiDecl + ` a="1"><c xsi:type="i:U"/></ro>`,
			`jsonsrc: instance.json:1:44: the "$type" member of {urn:i}c: the types the {type table} of {urn:i}ro`},
		{"below an unattributed parent", `{"big":{"mid":{"tu":{"a":"u","m":"x"}},"b":""}}`,
			`<big xmlns="urn:i"><mid><tu a="u" m="x"/></mid><b/></big>`,
			`jsonsrc: instance.json:1:30: member "m" of {urn:i}tu is not classified alike`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xres, err := xmlsrc.Validate(v, strings.NewReader(tc.xml), xmlsrc.WithURI("instance.xml"))
			if err != nil || xres.Err() != nil {
				t.Fatalf("xmlsrc.Validate(%s): %v / %v", tc.xml, err, xres.Err())
			}
			if got := rules(xres); !slices.Equal(got, none) || len(xres.Unevaluated()) == 0 {
				t.Fatalf("XML twin %s charges %v, %d unevaluated; want none charged and its subtree undecided", tc.xml, got, len(xres.Unevaluated()))
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

// TestInheritedBelowWithheld pins walker.handDown's alike rule below an element
// whose selection the engine withholds: the mapping determines the [inherited
// attributes] a child is handed wherever every type that may govern its parent
// hands down the same ones, and a {type table} below reads its selection where
// one type may govern that table's element's parent (walker.selector), while
// the engine, walking each XML twin's subtree undecided, charges nothing. re's
// selection is withheld, H and H3 hand down alike, and M governs mid2 under
// both, so tu reads its own a and selects U; mid's anonymous type governs it
// under both, which gathered keeps once (walker.governing), as it does where
// mid's "$type" names M, which ·overrides· no type of mid's and leaves that one
// governing it (walker.instanceGoverning); rw's "$type" names H4, which
// ·overrides· both H and H2, so H4 alone may govern rw (walker.governOverriding).
func TestInheritedBelowWithheld(t *testing.T) {
	v := validatorOver(t, inheritSchema)
	const i = `xmlns="urn:i" xmlns:i="urn:i" `
	for _, tc := range []struct {
		name, json, xml string
	}{
		{"types handing down alike", `{"re":{"a":"1","mid2":{"tu":{"a":"u","m":"x"}}}}`,
			`<re ` + i + `a="1"><mid2><tu a="u" m="x"/></mid2></re>`},
		{"one anonymous type under types handing down alike", `{"re":{"a":"1","mid":{"tu":{"a":"u","m":"x"}}}}`,
			`<re ` + i + `a="1"><mid><tu a="u" m="x"/></mid></re>`},
		{"one anonymous type under a \"$type\" it is not overridden by", `{"re":{"a":"1","mid":{"$xmlns":{"i":"urn:i"},"$type":"i:M","tu":{"a":"u","m":"x"}}}}`,
			`<re ` + i + xsiDecl + ` a="1"><mid xsi:type="i:M"><tu a="u" m="x"/></mid></re>`},
		{"a \"$type\" overriding every type the table can select", `{"rw":{"$xmlns":{"i":"urn:i"},"$type":"i:H4","a":"1","mid2":{"tu":{"a":"u","m":"x"}}}}`,
			`<rw ` + i + xsiDecl + ` xsi:type="i:H4" a="1"><mid2><tu a="u" m="x"/></mid2></rw>`},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, none) })
	}
}
