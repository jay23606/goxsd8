package jsonsrc

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsderr"
)

// typeSchema declares base, an element-only type holding a; derived, an
// extension of base adding the child b and the attribute d; other, unrelated
// to base, holding c; item, of type base; anything, of xs:anyType; and names,
// whose sequence holds a repeating xs:QName q, then inner, which holds q
// again, then an item. Local declarations are qualified.
const typeSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns="urn:t" elementFormDefault="qualified">
  <xs:complexType name="base">
    <xs:sequence>
      <xs:element name="a" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="derived">
    <xs:complexContent>
      <xs:extension base="base">
        <xs:sequence>
          <xs:element name="b" type="xs:string"/>
        </xs:sequence>
        <xs:attribute name="d" type="xs:integer"/>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="other">
    <xs:sequence>
      <xs:element name="c" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="item" type="base"/>
  <xs:element name="anything"/>
  <xs:element name="names">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="q" type="xs:QName" minOccurs="0" maxOccurs="unbounded"/>
        <xs:element name="inner" minOccurs="0">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="q" type="xs:QName"/>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
        <xs:element ref="item" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// TestTypeMember pins the "$type" arm: a "$type" member is the element's
// xsi:type, resolved against the "$xmlns" bindings in scope, and the members
// after it are classified under the type that governs the root: the type it
// names where that resolves and ·overrides· the declared type, and the
// declared type where it does not, cvc-elt clause 4 charged
// (walker.instanceGoverned). Each row agrees with its XML twin.
func TestTypeMember(t *testing.T) {
	v := validatorOver(t, typeSchema)
	const x = `xmlns="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"derived", `{"item":{"$xmlns":{"t":"urn:t"},"$type":"t:derived","a":"x","b":"y"}}`,
			`<item ` + x + ` xmlns:t="urn:t" xsi:type="t:derived"><a>x</a><b>y</b></item>`, none},
		{"derived's own attribute", `{"item":{"$xmlns":{"t":"urn:t"},"$type":"t:derived","d":"1","a":"x","b":"y"}}`,
			`<item ` + x + ` xmlns:t="urn:t" xsi:type="t:derived" d="1"><a>x</a><b>y</b></item>`, none},
		{"unprefixed, in the declaration's namespace", `{"item":{"$type":"derived","a":"x","b":"y"}}`,
			`<item ` + x + ` xsi:type="derived"><a>x</a><b>y</b></item>`, none},
		{"collapsed", `{"item":{"$xmlns":{"t":"urn:t"},"$type":" t:derived\n","a":"x","b":"y"}}`,
			`<item ` + x + ` xmlns:t="urn:t" xsi:type=" t:derived&#10;"><a>x</a><b>y</b></item>`, none},
		{"an empty-prefix binding is shadowed", `{"item":{"$xmlns":{"":"urn:other"},"$type":"derived","a":"x","b":"y"}}`,
			`<item ` + x + ` xsi:type="derived"><a>x</a><b>y</b></item>`, none},
		{"inherited binding", `{"names":{"$xmlns":{"t":"urn:t"},"item":{"$type":"t:derived","a":"x","b":"y"}}}`,
			`<names ` + x + ` xmlns:t="urn:t"><item xsi:type="t:derived"><a>x</a><b>y</b></item></names>`, none},
		{"innermost binding wins", `{"names":{"$xmlns":{"t":"urn:x"},"item":{"$xmlns":{"t":"urn:t"},"$type":"t:derived","a":"x","b":"y"}}}`,
			`<names ` + x + ` xmlns:t="urn:x"><item xmlns:t="urn:t" xsi:type="t:derived"><a>x</a><b>y</b></item></names>`, none},
		{"not overriding", `{"item":{"$xmlns":{"t":"urn:t"},"$type":"t:other","c":"z"}}`,
			`<item ` + x + ` xmlns:t="urn:t" xsi:type="t:other"><c>z</c></item>`, []xsderr.Rule{"cvc-elt", "cvc-complex-content"}},
		{"naming no type", `{"item":{"$xmlns":{"t":"urn:t"},"$type":"t:missing","a":"x"}}`,
			`<item ` + x + ` xmlns:t="urn:t" xsi:type="t:missing"><a>x</a></item>`, attr},
		{"unbound prefix", `{"item":{"$xmlns":{"u":"urn:t"},"$type":"t:derived","a":"x"}}`,
			`<item ` + x + ` xmlns:u="urn:t" xsi:type="t:derived"><a>x</a></item>`, attr},
		{"xs:error is not nilled", `{"anything":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:error"}}`,
			`<anything ` + x + ` xmlns:xs="http://www.w3.org/2001/XMLSchema" xsi:type="xs:error"/>`, typ},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestXmlnsMember pins the "$xmlns" arm for QName-valued content: a prefix a
// "$xmlns" binds resolves in its own element's content and its descendants',
// array items included, and nowhere above it. Each row agrees with its XML
// twin.
func TestXmlnsMember(t *testing.T) {
	v := validatorOver(t, typeSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"bound", `{"names":{"$xmlns":{"t":"urn:t"},"q":"t:foo"}}`,
			`<names xmlns="urn:t" xmlns:t="urn:t"><q>t:foo</q></names>`, none},
		{"unbound", `{"names":{"$xmlns":{"u":"urn:t"},"q":"t:foo"}}`,
			`<names xmlns="urn:t" xmlns:u="urn:t"><q>t:foo</q></names>`, typ},
		{"array items", `{"names":{"$xmlns":{"t":"urn:t"},"q":["t:a","t:b"]}}`,
			`<names xmlns="urn:t" xmlns:t="urn:t"><q>t:a</q><q>t:b</q></names>`, none},
		{"two levels down", `{"names":{"$xmlns":{"t":"urn:t"},"inner":{"q":"t:foo"}}}`,
			`<names xmlns="urn:t" xmlns:t="urn:t"><inner><q>t:foo</q></inner></names>`, none},
		{"bound in a sibling only", `{"names":{"inner":{"$xmlns":{"t":"urn:t"},"q":"t:foo"},"item":{"$type":"t:base","a":"x"}}}`,
			`<names xmlns="urn:t" ` + xsiDecl + `><inner xmlns:t="urn:t"><q>t:foo</q></inner><item xsi:type="t:base"><a>x</a></item></names>`,
			attr},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestTypeAttribute pins what the engine reads off a "$type" member: the
// element's first attribute, xsi:type, its value the member's string
// unchanged, located at the "$type" key, before the attribute members.
func TestTypeAttribute(t *testing.T) {
	v := validatorOver(t, typeSchema)
	w := newWalker(jsonURI, strings.NewReader(`{"item":{"$type":" derived ","d":"1","a":"x","b":"y"}}`), v.Schema(), v.ConditionallySelected, v.Governing)
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	var got []string
	for _, a := range root.Attributes() {
		got = append(got, a.Name().String()+"="+a.Value()+"@"+a.Loc().String())
	}
	want := []string{"{http://www.w3.org/2001/XMLSchema-instance}type= derived @instance.json:1:10", "d=1@instance.json:1:30"}
	if !slices.Equal(got, want) {
		t.Fatalf("Attributes() = %q, want %q", got, want)
	}
	if err := w.end(root); err != nil {
		t.Fatalf("end: %v", err)
	}
}

// TestReservedMemberDeclines pins every "$type" and "$xmlns" the arm does not
// map: each is (nil, err), err opening with "jsonsrc: <uri>:<line>:<col>: " at
// the construct it names.
func TestReservedMemberDeclines(t *testing.T) {
	v := validatorOver(t, typeSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"$type after a member", `{"item":{"a":"x","$type":"derived"}}`,
			"jsonsrc: instance.json:1:18: ", `reserved member "$type" is out of place; "$xmlns" and then "$type" lead an element's object`},
		{"$type before $xmlns", `{"item":{"$type":"t:derived","$xmlns":{"t":"urn:t"}}}`,
			"jsonsrc: instance.json:1:30: ", `reserved member "$xmlns" is out of place`},
		{"$type repeated", `{"item":{"$type":"base","$type":"base"}}`,
			"jsonsrc: instance.json:1:25: ", `reserved member "$type" is out of place`},
		{"$xmlns repeated", `{"item":{"$xmlns":{},"$xmlns":{}}}`,
			"jsonsrc: instance.json:1:22: ", `reserved member "$xmlns" is out of place`},
		{"$xmlns after a member", `{"names":{"q":"x","$xmlns":{}}}`,
			"jsonsrc: instance.json:1:19: ", `reserved member "$xmlns" is out of place`},
		{"$type not a string", `{"item":{"$type":1}}`,
			"jsonsrc: instance.json:1:18: ", `the "$type" member of {urn:t}item is not a string; its value is the lexical form of the element's xsi:type, so give it as a string, "local" or "p:local"`},
		{"$xmlns not an object", `{"item":{"$xmlns":"urn:t"}}`,
			"jsonsrc: instance.json:1:19: ", `the "$xmlns" member of "item" is not an object`},
		{"$xmlns an array", `{"item":{"$xmlns":[]}}`,
			"jsonsrc: instance.json:1:19: ", `the "$xmlns" member of "item" is not an object`},
		{"$xmlns binding not a string", `{"item":{"$xmlns":{"t":null}}}`,
			"jsonsrc: instance.json:1:24: ", `the "$xmlns" member of "item" binds "t" to a value that is not a string`},
		{"$xmlns binding a prefix twice", `{"item":{"$xmlns":{"t":"urn:t","t":"urn:t"}}}`,
			"jsonsrc: instance.json:1:32: ", `the "$xmlns" member of "item" binds "t" twice; bind each prefix once`},
		{"$xmlns binding the empty prefix twice", `{"item":{"$xmlns":{"":"urn:t","":"urn:t"}}}`,
			"jsonsrc: instance.json:1:31: ", `binds "" twice; bind each prefix once`},
		{"$xmlns binding a prefix no NCName starts", `{"item":{"$xmlns":{"1x":"urn:t"},"$type":"1x:derived","a":"x","b":"y"}}`,
			"jsonsrc: instance.json:1:20: ", `the "$xmlns" member of "item" binds "1x", which is not an NCName; no xmlns:prefix attribute declares it, so bind an NCName prefix instead`},
		{"$xmlns binding a prefix with a space", `{"item":{"$xmlns":{"a b":"urn:t"},"$type":"a b:derived","a":"x","b":"y"}}`,
			"jsonsrc: instance.json:1:20: ", `binds "a b", which is not an NCName; no xmlns:prefix attribute declares it, so bind an NCName prefix instead`},
		{"$xmlns binding a prefix with a colon", `{"item":{"$xmlns":{"a:b":"urn:t"}}}`,
			"jsonsrc: instance.json:1:20: ", `binds "a:b", which is not an NCName; no xmlns:prefix attribute declares it, so bind an NCName prefix instead`},
		{"$xmlns binding xml", `{"item":{"$xmlns":{"xml":"http://www.w3.org/XML/1998/namespace"}}}`,
			"jsonsrc: instance.json:1:20: ", `the "$xmlns" member of "item" binds the reserved prefix "xml", which every element binds already, so drop the binding`},
		{"$xmlns binding xml to another namespace", `{"item":{"$xmlns":{"xml":"urn:foo"}}}`,
			"jsonsrc: instance.json:1:20: ", `the "$xmlns" member of "item" binds the reserved prefix "xml" to "urn:foo", but every element binds it to http://www.w3.org/XML/1998/namespace and none to any other namespace name, so bind another prefix to "urn:foo" and use it in place of "xml" on the keys meant to be in that namespace`},
		{"$xmlns binding xml to the empty string", `{"item":{"$xmlns":{"xml":""}}}`,
			"jsonsrc: instance.json:1:20: ", `the "$xmlns" member of "item" binds the reserved prefix "xml", which every element binds already, so drop the binding`},
		{"$xmlns binding xmlns", `{"item":{"$xmlns":{"xmlns":"urn:t"}}}`,
			"jsonsrc: instance.json:1:20: ", `binds the reserved prefix "xmlns", which no element binds, so bind another prefix`},
		{"$xmlns undeclaring a prefix", `{"item":{"$xmlns":{"t":""}}}`,
			"jsonsrc: instance.json:1:24: ", `binds "t" to the empty string; a prefix cannot be undeclared, so bind it to a namespace name or drop the binding`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, nil error; want the decline", tc.json, rules(res))
			}
			if res != nil {
				t.Errorf("Validate(%s) returned a result beside its error", tc.json)
			}
			if !strings.HasPrefix(err.Error(), tc.prefix) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Validate(%s) = %q, want it to open %q and say %q", tc.json, err, tc.prefix, tc.says)
			}
		})
	}
}

// openContentSchema gives x, of type dt, a ·locally declared type· under four
// parents: p, whose lax {open content} may take an x its sequence does not;
// r, a restriction whose own content model contains no x but whose base B's
// does (key-ldt-elem case 3), opened by a lax {open content} of the target
// namespace; s, whose {open content} is skip; and q, which has none. dt holds
// an optional c of type ct, and both dt and ct have a strict {attribute
// wildcard}; other, unrelated to dt, holds a c of type oc and the attribute
// foo, which oc carries too.
const openContentSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="ct"><xs:anyAttribute processContents="strict"/></xs:complexType>
  <xs:complexType name="oc"><xs:attribute name="foo" type="xs:string"/></xs:complexType>
  <xs:complexType name="dt">
    <xs:sequence><xs:element name="c" type="t:ct" minOccurs="0"/></xs:sequence>
    <xs:anyAttribute processContents="strict"/>
  </xs:complexType>
  <xs:complexType name="other">
    <xs:sequence><xs:element name="c" type="t:oc" minOccurs="0"/></xs:sequence>
    <xs:attribute name="foo" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="B">
    <xs:sequence>
      <xs:element name="x" type="t:dt" minOccurs="0"/>
      <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="p">
    <xs:complexType>
      <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
      <xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="r">
    <xs:complexType>
      <xs:complexContent>
        <xs:restriction base="t:B">
          <xs:openContent mode="interleave"><xs:any namespace="##targetNamespace" processContents="lax"/></xs:openContent>
          <xs:sequence/>
        </xs:restriction>
      </xs:complexContent>
    </xs:complexType>
  </xs:element>
  <xs:element name="s">
    <xs:complexType>
      <xs:openContent mode="interleave"><xs:any processContents="skip"/></xs:openContent>
      <xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="q">
    <xs:complexType>
      <xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// TestOpenContentType pins walker.governedTypes' {open content} arm: a
// "$type" on an element a non-skip {open content} may take, whose name has a
// ·locally declared type· dt within the type, classifies the element's
// members under dt where the type it names does not ·override· dt, and under
// that type where it does, as the engine governs it. The second x of a "x"
// array is the {open content}'s, the sequence taking one x at most. Each row
// agrees with its XML twin.
func TestOpenContentType(t *testing.T) {
	v := validatorOver(t, openContentSchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"naming the locally declared type", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:dt"}]}}`,
			`<p ` + x + `><x/><x xsi:type="t:dt"/></p>`, none},
		{"declared on the base chain alone", `{"r":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other"}}}`,
			`<r ` + x + `><x xsi:type="t:other"/></r>`, none},
		{"not overriding, a child of the locally declared type", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","c":{}}]}}`,
			`<p ` + x + `><x/><x xsi:type="t:other"><c/></x></p>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestOpenContentTypeKeyDeclines pins #2615's two reproductions, where other
// does not ·override· dt: the members of x are classified under dt, which
// governs it, and the key foo, which no use of dt or of ct carries, declines
// under their strict {attribute wildcard} of variety any, as wildcardMember's
// #2634 marker rules: an unprefixed key does not say its namespace. The XML
// twins, <x xsi:type="t:other" foo="1"/> and <c foo="1"/> under the second x,
// are valid; no key spells foo in no namespace. The rows under q, which has
// no {open content}, and s, whose {open content} is skip, decline alike: x is
// ·attributed· to the element particle there, the engine charges cvc-elt
// clause 4 and dt, its ·selected type definition·, governs it
// (walker.instanceGoverned).
func TestOpenContentTypeKeyDeclines(t *testing.T) {
	v := validatorOver(t, openContentSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		owner      string
	}{
		{"its own attribute", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","foo":"1"}]}}`,
			"jsonsrc: instance.json:1:57: ", "{urn:t}x"},
		{"its descendant", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","c":{"foo":"1"}}]}}`,
			"jsonsrc: instance.json:1:62: ", "{urn:t}c"},
		{"prefixed keys", `{"t:p":{"$xmlns":{"t":"urn:t"},"t:x":[{},{"$type":"t:other","foo":"1"}]}}`,
			"jsonsrc: instance.json:1:61: ", "{urn:t}x"},
		{"no open content", `{"q":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other","foo":"1"}}}`,
			"jsonsrc: instance.json:1:53: ", "{urn:t}x"},
		{"skip open content", `{"s":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other","foo":"1"}}}`,
			"jsonsrc: instance.json:1:53: ", "{urn:t}x"},
		{"no open content, prefixed keys", `{"t:q":{"$xmlns":{"t":"urn:t"},"t:x":{"$type":"t:other","foo":"1"}}}`,
			"jsonsrc: instance.json:1:57: ", "{urn:t}x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, %d unevaluated, nil error; want the decline", tc.json, rules(res), len(res.Unevaluated()))
			}
			want := tc.prefix + `member "foo" names no child element declaration and no attribute use of the type of ` + tc.owner + `, and its {attribute wildcard} set includes one of variety any`
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("Validate(%s) = %q, want it to open %q", tc.json, err, want)
			}
		})
	}
}

// governedSchema gives x the ·locally declared type· L, an optional child k
// and an attribute a, under p's lax {open content}, under pb's, where the
// declaration blocks extension, and in q, which has no {open content}, so
// that a second x is rejected. Xe extends L with the attribute b; Xo
// restricts L, dropping k; X, unrelated to L, has the attribute k. Each row
// of TestGovernedType that a curated json case mirrors names it.
const governedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="L">
    <xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="Xe">
    <xs:complexContent><xs:extension base="t:L"><xs:attribute name="b" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="Xo">
    <xs:complexContent><xs:restriction base="t:L"><xs:sequence/></xs:restriction></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="X"><xs:attribute name="k" type="xs:string"/></xs:complexType>
  <xs:element name="p">
    <xs:complexType>
      <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
      <xs:sequence><xs:element name="x" type="t:L" minOccurs="0"/></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="pb">
    <xs:complexType>
      <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
      <xs:sequence><xs:element name="x" type="t:L" minOccurs="0" block="extension"/></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="q">
    <xs:complexType><xs:sequence><xs:element name="x" type="t:L" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
</xs:schema>`

// TestGovernedType pins walker.governedTypes against governedSchema: the
// members of the second x are classified under the type its "$type" names
// where that type ·overrides· L ·without limitation·, and under L where it
// does not, uncharged (key-governing-type-elem clauses 6 and 7). The
// non-overriding row's k is a child of L and an attribute of X, so it fails
// where the members are classified under the type "$type" names. The blocked
// rows ask ·override· with no blocking keyword on the {open content}'s x and
// under the declaration's {disallowed substitutions} on the particle's, where
// Xe does not ·override· L: the engine charges cvc-elt clause 4 and L, the
// particle's ·selected type definition·, governs x, of which b is a child. The
// rejected rows' x, which q's content model ·attributes· to nothing, has no
// ·governing element declaration· (key-governing-ed clause 4.3), so L governs
// it there too, and only q's content is charged (cvc-complex-content): k is a
// child and a an attribute of L, where X would make k an attribute and a a
// child. Each row agrees with its XML twin.
func TestGovernedType(t *testing.T) {
	v := validatorOver(t, governedSchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"overriding (governed-overriding.v)", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:Xe","b":"1","k":"z"}]}}`,
			`<p ` + x + `><x/><x xsi:type="t:Xe" b="1"><k>z</k></x></p>`, none},
		{"not overriding (governed-local.v)", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"z"}]}}`,
			`<p ` + x + `><x/><x xsi:type="t:X"><k>z</k></x></p>`, none},
		{"overriding, members that do not fit (governed-overriding.i)", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:Xo","k":"z"}]}}`,
			`<p ` + x + `><x/><x xsi:type="t:Xo"><k>z</k></x></p>`, complexType},
		{"not overriding, prefixed keys", `{"t:p":{"$xmlns":{"t":"urn:t"},"t:x":[{},{"$type":"t:X","t:k":"z"}]}}`,
			`<t:p ` + xsiDecl + ` xmlns:t="urn:t"><t:x/><t:x xsi:type="t:X"><t:k>z</t:k></t:x></t:p>`, none},
		{"blocked, open content", `{"pb":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:Xe","b":"1"}]}}`,
			`<pb ` + x + `><x/><x xsi:type="t:Xe" b="1"/></pb>`, none},
		{"blocked, particle", `{"pb":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:Xe","b":"1"}}}`,
			`<pb ` + x + `><x xsi:type="t:Xe"><b>1</b></x></pb>`, []xsderr.Rule{"cvc-elt", "cvc-complex-content"}},
		{"rejected, not overriding", `{"q":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"z"}]}}`,
			`<q ` + x + `><x/><x xsi:type="t:X"><k>z</k></x></q>`, content},
		{"rejected, not overriding, an attribute of L", `{"q":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","a":"z"}]}}`,
			`<q ` + x + `><x/><x xsi:type="t:X" a="z"/></q>`, content},
		{"after a rejected sibling, not overriding", `{"q":{"x":[{},{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"z"}]}}`,
			`<q ` + x + `><x/><x/><x xsi:type="t:X"><k>z</k></x></q>`, content},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// chargedSchema declares L, with an optional child k; M, which extends L with
// the attribute m; Xe, which extends L with the attribute b; and X, unrelated
// to L, with the attribute k. The top-level y is of type L, ytm of type
// L with a {type table} whose one Type Alternative, a default, selects M, and
// yb of type L, blocking extension. q2 holds one e and declares no y, ytm or
// yb; q holds an optional x of type L, and qt an optional x of type L whose
// {type table} selects M likewise; s is of xs:string and sc of a complex type
// with simple content; lw holds one child under a lax ##targetNamespace
// wildcard.
const chargedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="L">
    <xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="M">
    <xs:complexContent><xs:extension base="t:L"><xs:attribute name="m" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="Xe">
    <xs:complexContent><xs:extension base="t:L"><xs:attribute name="b" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="X"><xs:attribute name="k" type="xs:string"/></xs:complexType>
  <xs:element name="y" type="t:L"/>
  <xs:element name="ytm" type="t:L"><xs:alternative type="t:M"/></xs:element>
  <xs:element name="yb" type="t:L" block="extension"/>
  <xs:element name="q2"><xs:complexType><xs:sequence><xs:element name="e" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="q"><xs:complexType><xs:sequence><xs:element name="x" type="t:L" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="qt">
    <xs:complexType><xs:sequence><xs:element name="x" type="t:L" minOccurs="0"><xs:alternative type="t:M"/></xs:element></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="s" type="xs:string"/>
  <xs:element name="sc"><xs:complexType><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType></xs:element>
  <xs:element name="lw">
    <xs:complexType><xs:sequence><xs:any namespace="##targetNamespace" processContents="lax"/></xs:sequence></xs:complexType>
  </xs:element>
</xs:schema>`

// TestChargedType pins walker.instanceGoverned where the engine charges
// cvc-elt clause 4, the "$type" not ·overriding· the ·selected type
// definition· of the element's ·governing element declaration·, which then
// governs it (the Note closing cvc-elt): the members are classified under
// that type, so k is a child of L, where X would make it an attribute, and m
// an attribute of M, where X and L would make it a child. The declaration is the
// top-level one the name ·resolves· to under a parent whose content model
// ·attributes· the element to nothing (key-governing-ed clause 4) — q2's,
// which rejects it, s's simple type and sc's simple content, each charged at
// it — or to its lax wildcard (clause 3); the element particle's (clause 2);
// and the root's. Its {type table} selects M for ytm and qt's x, and yb's
// {disallowed substitutions} keep Xe, which extends L, from ·overriding· it.
// Each row agrees with its XML twin.
func TestChargedType(t *testing.T) {
	v := validatorOver(t, chargedSchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	const b = `"$xmlns":{"t":"urn:t"},`
	rejected := []xsderr.Rule{"cvc-complex-content", "cvc-elt"}
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"a rejected child's top-level declaration", `{"q2":{"e":"1","y":{` + b + `"$type":"t:X","k":"z"}}}`,
			`<q2 ` + x + `><e>1</e><y xsi:type="t:X"><k>z</k></y></q2>`, rejected},
		{"a rejected child's tabled top-level declaration, a child of L", `{"q2":{"e":"1","ytm":{` + b + `"$type":"t:X","k":"z"}}}`,
			`<q2 ` + x + `><e>1</e><ytm xsi:type="t:X"><k>z</k></ytm></q2>`, rejected},
		{"a rejected child's tabled top-level declaration, an attribute of M", `{"q2":{"e":"1","ytm":{` + b + `"$type":"t:X","m":"z"}}}`,
			`<q2 ` + x + `><e>1</e><ytm xsi:type="t:X" m="z"/></q2>`, rejected},
		{"a rejected child's blocking top-level declaration", `{"q2":{"e":"1","yb":{` + b + `"$type":"t:Xe","b":"1"}}}`,
			`<q2 ` + x + `><e>1</e><yb xsi:type="t:Xe"><b>1</b></yb></q2>`, []xsderr.Rule{"cvc-complex-content", "cvc-elt", "cvc-complex-content"}},
		{"an element particle's", `{"q":{"x":{` + b + `"$type":"t:X","k":"z"}}}`,
			`<q ` + x + `><x xsi:type="t:X"><k>z</k></x></q>`, elt},
		{"a tabled element particle's", `{"qt":{"x":{` + b + `"$type":"t:X","m":"z"}}}`,
			`<qt ` + x + `><x xsi:type="t:X" m="z"/></qt>`, elt},
		{"under a simple type", `{"s":{"y":{` + b + `"$type":"t:X","k":"z"}}}`,
			`<s ` + x + `><y xsi:type="t:X"><k>z</k></y></s>`, []xsderr.Rule{"cvc-type", "cvc-elt"}},
		{"under simple content", `{"sc":{"y":{` + b + `"$type":"t:X","k":"z"}}}`,
			`<sc ` + x + `><y xsi:type="t:X"><k>z</k></y></sc>`, []xsderr.Rule{"cvc-complex-type", "cvc-elt"}},
		{"a lax wildcard's top-level declaration", `{"lw":{"y":{` + b + `"$type":"t:X","k":"z"}}}`,
			`<lw ` + x + `><y xsi:type="t:X"><k>z</k></y></lw>`, elt},
		{"the root's", `{"y":{` + b + `"$type":"t:X","k":"z"}}`,
			`<y ` + x + ` xsi:type="t:X"><k>z</k></y>`, elt},
		{"the root's tabled declaration", `{"ytm":{` + b + `"$type":"t:X","m":"z"}}`,
			`<ytm ` + x + ` xsi:type="t:X" m="z"/>`, elt},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestUnresolvedWildcardType pins walker.governedTypes where a ·wildcard
// particle· ·attributes· a child whose name ·resolves· to no declaration: the
// child has no ·governing element declaration·, so no cvc-elt clause 4 is
// charged, its "$type" governs it (key-governing-type-elem clause 8), and the
// engine charges the parent cvc-complex-type clause 5 against the child's
// ·locally declared type·. The second z of zw is such a child, of ·locally
// declared type· zt, and k is an attribute of X, where zt would make it a
// child. It agrees with its XML twin.
func TestUnresolvedWildcardType(t *testing.T) {
	v := validatorOver(t, ancestrySchema)
	twin(t, v, `{"zw":{"z":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}}`,
		`<zw xmlns="urn:t" xmlns:t="urn:t" `+xsiDecl+`><z/><z xsi:type="t:X" k="zz"/></zw>`, complexType)
}

// viewOnly is a schema view without the governor capability.
type viewOnly struct{ validate.SchemaView }

// TestTypeWithoutGovernor pins the governor capability's absence: under a
// schema view without it the mapping attributes no child of p
// (object.unattributed), as where xsd.Schema.ContentMatcher declines p's
// content model, and a "$type" below the root is settled on p alone
// (walker.instanceUnder). The governance itself is
// validate.Validator.Governing's, which reads no capability of the view. Xe
// ·overrides· x's ·locally declared type· L, so the second x's b maps as Xe's
// attribute and nothing is charged. q has no ·locally declared type· within
// p and ·resolves· to the top-level q, whose type Xe does not ·override·, so
// every ·attribution· p may give it charges it, and its members are
// classified under Xe rather than that type: b maps as an attribute, which
// the engine, governing q by the type Xe does not ·override·, charges under
// cvc-complex-type clause 2.1, where under q's type it would map as a child.
func TestTypeWithoutGovernor(t *testing.T) {
	v := validatorOver(t, governedSchema)
	for _, c := range []struct {
		doc    string
		charge string
	}{
		{`{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:Xe","b":"1"}]}}`, ""},
		{`{"p":{"$xmlns":{"t":"urn:t"},"t:q":{"$type":"t:Xe","b":"1"}}}`, "the attribute b matches no attribute use"},
	} {
		w := newWalker(jsonURI, strings.NewReader(c.doc), viewOnly{v.Schema()}, v.ConditionallySelected, v.Governing)
		root, err := w.root()
		if err != nil {
			t.Fatalf("root: %v", err)
		}
		res := v.Assess(root)
		if w.err != nil {
			t.Fatalf("walking %s latched %v, want it to map", c.doc, w.err)
		}
		got := res.Violations()
		if c.charge == "" && len(got) != 0 {
			t.Errorf("walking %s charged %v, want nothing: b is an attribute Xe declares", c.doc, got)
		}
		if c.charge != "" && !slices.ContainsFunc(got, func(e *xsderr.Error) bool {
			return e.Rule == ruleCvcComplexType && strings.HasPrefix(e.Msg, c.charge)
		}) {
			t.Errorf("walking %s charged %v, want a cvc-complex-type charge opening %q: b is an attribute", c.doc, got, c.charge)
		}
	}
}

// TestAbsentAffiliationLax pins the governance validate.Validator.Governing
// gives an element whose declaration has an ·absent· {substitution group
// affiliations} member: §5.3 sends it to ·lax assessment·, cvc-elt clause 1
// charged, so its children are ·attributed to· xs:anyType's lax wildcard and
// not to its declaration's content model. c then ·resolves· to no
// declaration and its "$type" governs it (key-governing-type-elem clause 8),
// so k maps as an attribute of X, as its XML twin carries it, and not under
// c's local declaration, of type Tc, which X does not ·override·.
func TestAbsentAffiliationLax(t *testing.T) {
	v := validatorOver(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="X"><xs:attribute name="k" type="xs:string"/></xs:complexType>
  <xs:complexType name="Tc"/>
  <xs:element name="e" substitutionGroup="t:rotten">
    <xs:complexType><xs:sequence><xs:element name="c" type="t:Tc"/></xs:sequence></xs:complexType>
  </xs:element>
</xs:schema>`)
	twin(t, v, `{"e":{"c":{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"1"}}}`,
		`<e xmlns="urn:t" xmlns:t="urn:t" `+xsiDecl+`><c xsi:type="t:X" k="1"/></e>`, []xsderr.Rule{"cvc-elt"})
}

// TestTypeOnUndeclaredElement pins a "$type" on an element a wildcard names
// with no declaration: it maps to the element's xsi:type, as its XML twin
// carries one, and the members after it decline as every member of such an
// element does, whatever type it names.
func TestTypeOnUndeclaredElement(t *testing.T) {
	v := validatorOver(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="holder"><xs:sequence><xs:element name="x" type="xs:string" form="unqualified"/></xs:sequence></xs:complexType>
  <xs:element name="bag">
    <xs:complexType><xs:sequence><xs:any namespace="##local" processContents="lax"/></xs:sequence></xs:complexType>
  </xs:element>
</xs:schema>`)
	twin(t, v, `{"bag":{"q":{"$xmlns":{"t":"urn:t"},"$type":"t:holder"}}}`,
		`<bag xmlns="urn:t" xmlns:t="urn:t" `+xsiDecl+`><q xmlns="" xsi:type="t:holder"/></bag>`, []xsderr.Rule{"cvc-complex-content"})
	const doc = `{"bag":{"q":{"$xmlns":{"t":"urn:t"},"$type":"t:holder","x":"1"}}}`
	_, err := Validate(v, strings.NewReader(doc), WithURI(jsonURI))
	const want = `jsonsrc: instance.json:1:56: member "x" of q, an element no contained and no top-level element declaration names`
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("Validate(%s) = %v, want it to open %q", doc, err, want)
	}
}

// governanceSchema builds parents whose classifying type may not govern them.
// w's sequence takes one optional y of type Td and then a lax wildcard, and
// the top-level y is of type Tg, so a second y is ·attributed to· the
// wildcard and governed by Tg (key-governing-ed clause 3), which extends Td
// with a lax {open content}. Tg2 instead restricts Td2, giving v the type vr,
// which restricts vt's wildcard to the declaration x, and w2 takes a y2 as w
// takes a y. r restricts B, which declares u, and opens to the target
// namespace, so a u under r is governed by its ·locally declared type· ub,
// which has a lax {open content}, while the top-level u is of type ug, which
// has none. sg holds the head h, which m, of type mt, a type with a lax {open
// content}, substitutes for; z holds a zt under a lax wildcard, with no
// top-level z. dt holds an optional k, and other, unrelated to dt, has the
// attribute k.
const governanceSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="dt"><xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="other"><xs:attribute name="k" type="xs:string"/></xs:complexType>
  <xs:complexType name="Td"><xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Tg">
    <xs:complexContent>
      <xs:extension base="t:Td">
        <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="y" type="t:Tg"/>
  <xs:element name="w">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="y" type="t:Td" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="vt">
    <xs:openContent mode="interleave"><xs:any namespace="##targetNamespace" processContents="lax"/></xs:openContent>
    <xs:sequence><xs:any namespace="##targetNamespace" processContents="lax" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="vr">
    <xs:complexContent>
      <xs:restriction base="t:vt">
        <xs:openContent mode="interleave"><xs:any namespace="##targetNamespace" processContents="lax"/></xs:openContent>
        <xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="x" type="t:dt"/>
  <xs:complexType name="Td2"><xs:sequence><xs:element name="v" type="t:vt" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Tg2">
    <xs:complexContent>
      <xs:restriction base="t:Td2">
        <xs:sequence><xs:element name="v" type="t:vr" minOccurs="0"/></xs:sequence>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="y2" type="t:Tg2"/>
  <xs:element name="w2">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="y2" type="t:Td2" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="ub">
    <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
    <xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="ug"><xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:element name="u" type="t:ug"/>
  <xs:complexType name="B">
    <xs:sequence>
      <xs:element name="u" type="t:ub" minOccurs="0"/>
      <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="r">
    <xs:complexType>
      <xs:complexContent>
        <xs:restriction base="t:B">
          <xs:openContent mode="interleave"><xs:any namespace="##targetNamespace" processContents="lax"/></xs:openContent>
          <xs:sequence/>
        </xs:restriction>
      </xs:complexContent>
    </xs:complexType>
  </xs:element>
  <xs:element name="h" type="t:dt" abstract="true"/>
  <xs:complexType name="mt">
    <xs:complexContent>
      <xs:extension base="t:dt">
        <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
        <xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="m" type="t:mt" substitutionGroup="t:h"/>
  <xs:element name="sg"><xs:complexType><xs:sequence><xs:element ref="t:h"/></xs:sequence></xs:complexType></xs:element>
  <xs:complexType name="zt"><xs:sequence><xs:element name="x" type="t:dt" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:element name="zw">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="z" type="t:zt" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// TestUngovernedParentType pins walker.governing and walker.instanceGoverned
// under a parent whose classifying type is not the one that governs it: the
// "$type" on x is settled under the type the parent's ·attribution· gives it,
// on which dt governs the second x, other not ·overriding· it. The first row
// is #2615's round-3 reproduction: the second y is matched to w's declaration
// of type Td but ·attributed to· the lax wildcard and governed by the
// top-level y's Tg (key-governing-ed clause 3), whose lax {open content}
// leaves the second x to dt, uncharged. The third carries it one level down:
// under Tg2, v is governed by vr, which declares x, where Td2's vt does not.
// The fourth row's u is taken by r's {open content} and governed by its
// ·locally declared type· ub, not by the top-level u's ug. The fifth reaches
// mt through the substitution group of h (cvc-accept clause 2.3.2). Each row
// agrees with its XML twin.
func TestUngovernedParentType(t *testing.T) {
	v := validatorOver(t, governanceSchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct{ name, json, xml string }{
		{"a wildcard's top-level declaration governs the parent", `{"w":{"y":[{},{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}]}}`,
			`<w ` + x + `><y/><y><x/><x xsi:type="t:other"><k>zz</k></x></y></w>`},
		{"prefixed keys", `{"t:w":{"$xmlns":{"t":"urn:t"},"t:y":[{},{"t:x":[{},{"$type":"t:other","t:k":"zz"}]}]}}`,
			`<t:w xmlns:t="urn:t" ` + xsiDecl + `><t:y/><t:y><t:x/><t:x xsi:type="t:other"><t:k>zz</t:k></t:x></t:y></t:w>`},
		{"a grandparent's", `{"w2":{"y2":[{},{"v":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}}]}}`,
			`<w2 ` + x + `><y2/><y2><v><x/><x xsi:type="t:other"><k>zz</k></x></v></y2></w2>`},
		{"a locally declared type governs the parent", `{"r":{"u":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}}}`,
			`<r ` + x + `><u><x/><x xsi:type="t:other"><k>zz</k></x></u></r>`},
		{"a substitution group member's open content", `{"sg":{"m":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}}}`,
			`<sg ` + x + `><m><x/><x xsi:type="t:other"><k>zz</k></x></m></sg>`},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, none) })
	}
}

// ancestrySchema builds parents whose members are classified under one type
// and governed by another, for walker.governing and walker.instanceGoverned. w
// takes one optional y of type Ta and then a lax wildcard, and the top-level y
// is of type Tb, which restricts Ta, narrowing x's type from L to Lr; both
// have a lax {open content}. w6 takes one optional u of type A, which declares
// e of type Ta, and then a lax wildcard, and the top-level u is of type B,
// which has a lax {open content} and declares nothing. zw takes one optional z
// of type zt and then a lax wildcard, with no top-level z, so a second z is
// ·laxly assessed·; zt declares g, and the top-level g carries a {type table}.
// pp is of the empty type D, which Q extends with a lax {open content} and x.
// ws takes one optional v of type Ta and then a skip wildcard, and the
// top-level v is of the empty type Tz; wo takes one optional v of type Ta
// and has a skip suffix {open content}; nb takes one optional v of type Ta, a
// required b and then a lax wildcard. wt takes a yt as w takes a y, and the
// top-level yt is of type Tt, which restricts Ta and whose x carries a {type
// table}. big's {content type} is one xsd.Schema.ContentMatcher declines (its
// GAP(xsd) ceiling), holding a of type Ta. X, unrelated to L, has the
// attribute k; there is no top-level x. Tn restricts Ta to its x alone, which
// blocks extension, with no {open content}, and the top-level gd is of type
// Ta and selects Tn where it has an attribute a. Le extends L with the
// attribute a; ht holds an optional x of type L that selects Lr where its a is
// present, and the top-level hr is of type L and selects Lr likewise. Gw takes
// one optional q of type L and then a lax wildcard, Gq restricts it to the q
// alone, and the top-level gw is of type Gw and selects Gq where it has an
// attribute a; the top-level q is of type Qo, which has a lax {open content}
// and declares k of xs:token. hold holds an optional gd and then an optional
// gw.
const ancestrySchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="L"><xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Lr">
    <xs:complexContent><xs:restriction base="t:L"><xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence></xs:restriction></xs:complexContent>
  </xs:complexType>
  <xs:complexType name="X"><xs:attribute name="k" type="xs:string"/></xs:complexType>
  <xs:complexType name="Ta">
    <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
    <xs:sequence><xs:element name="x" type="t:L" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Tb">
    <xs:complexContent>
      <xs:restriction base="t:Ta">
        <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
        <xs:sequence><xs:element name="x" type="t:Lr" minOccurs="0"/></xs:sequence>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="y" type="t:Tb"/>
  <xs:element name="w">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="y" type="t:Ta" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="g" type="t:Ta">
    <xs:alternative test="@a" type="t:Ta"/>
  </xs:element>
  <xs:complexType name="zt"><xs:sequence><xs:element name="g" type="t:Ta" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:element name="zw">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="z" type="t:zt" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="A"><xs:sequence><xs:element name="e" type="t:Ta" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="B">
    <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
    <xs:sequence/>
  </xs:complexType>
  <xs:element name="u" type="t:B"/>
  <xs:element name="w6">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="u" type="t:A" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="D"/>
  <xs:complexType name="Q">
    <xs:complexContent>
      <xs:extension base="t:D">
        <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
        <xs:sequence><xs:element name="x" type="t:L" minOccurs="0"/></xs:sequence>
      </xs:extension>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="pp" type="t:D"/>
  <xs:complexType name="Tz"/>
  <xs:element name="v" type="t:Tz"/>
  <xs:element name="ws">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="v" type="t:Ta" minOccurs="0"/>
        <xs:any processContents="skip" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="wo">
    <xs:complexType>
      <xs:openContent mode="suffix"><xs:any processContents="skip"/></xs:openContent>
      <xs:sequence><xs:element name="v" type="t:Ta" minOccurs="0"/></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="nb">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="v" type="t:Ta" minOccurs="0"/>
        <xs:element name="b" type="xs:string"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="Tt">
    <xs:complexContent>
      <xs:restriction base="t:Ta">
        <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
        <xs:sequence><xs:element name="x" type="t:Lr" minOccurs="0"><xs:alternative test="@a" type="t:Lr"/></xs:element></xs:sequence>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="yt" type="t:Tt"/>
  <xs:element name="wt">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="yt" type="t:Ta" minOccurs="0"/>
        <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="big">
    <xs:complexType>
      <xs:sequence>
        <xs:sequence maxOccurs="1000"><xs:element name="a" type="t:Ta" maxOccurs="unbounded"/></xs:sequence>
        <xs:element name="b" type="xs:string"/>
        <xs:sequence maxOccurs="1000"><xs:element name="c" maxOccurs="unbounded"/></xs:sequence>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="Tn">
    <xs:complexContent>
      <xs:restriction base="t:Ta"><xs:sequence><xs:element name="x" type="t:L" minOccurs="0" block="extension"/></xs:sequence></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="gd" type="t:Ta">
    <xs:alternative test="@a" type="t:Tn"/>
  </xs:element>
  <xs:complexType name="Le">
    <xs:complexContent><xs:extension base="t:L"><xs:attribute name="a" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="ht">
    <xs:complexType>
      <xs:sequence><xs:element name="x" type="t:L" minOccurs="0"><xs:alternative test="@a" type="t:Lr"/></xs:element></xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="hr" type="t:L">
    <xs:alternative test="@a" type="t:Lr"/>
  </xs:element>
  <xs:complexType name="Qo">
    <xs:openContent mode="interleave"><xs:any processContents="lax"/></xs:openContent>
    <xs:sequence><xs:element name="k" type="xs:token" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:element name="q" type="t:Qo"/>
  <xs:complexType name="Gw">
    <xs:sequence>
      <xs:element name="q" type="t:L" minOccurs="0"/>
      <xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Gq">
    <xs:complexContent>
      <xs:restriction base="t:Gw"><xs:sequence><xs:element name="q" type="t:L" minOccurs="0"/></xs:sequence></xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:element name="gw" type="t:Gw">
    <xs:alternative test="@a" type="t:Gq"/>
  </xs:element>
  <xs:element name="hold">
    <xs:complexType><xs:sequence><xs:element ref="t:gd" minOccurs="0"/><xs:element ref="t:gw" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
</xs:schema>`

// TestAncestrySelected pins a "$type" below a root whose {type table}'s types
// settle it differently: the mapping reads the type the root's table
// ·conditionally selects· (walker.settled), which alone governs the root, and
// settles the "$type" under it, as the engine assesses each XML twin. gd may
// be governed by Ta or Tn. Its second x, which Ta ·attributes· to the {open
// content} and Tn to nothing, is governed by x's L under either, t:X not
// ·overriding· it, so k is a child (key-governing-type-elem clause 7), and its
// row settles the "$type" alike whether the mapping reads the selection or
// not. Its one x is ·attributed· to the element particle of type L under
// either: t:Le ·overrides· L under Ta's x and governs it, so a is an
// attribute, and not under Tn's, which blocks extension, so the engine
// charges cvc-elt clause 4 and L governs it, of which a is a child
// (walker.governedTypes): the particle row fails where the mapping reads no
// selection. gd has no a, so its table selects Ta. gw may be governed by Gq,
// which rejects its second q, so that q's ·locally declared type· L governs
// it, or by Gw, which ·attributes· it to the lax wildcard, so that the
// top-level q's Qo does (walker.governing): xs:string ·overrides· the
// xs:string of L's k and governs it, and not the xs:token of Qo's, which
// governs it, cvc-elt clause 4 charged, so that row fails where the mapping
// reads no selection too. gw has no a, so its table selects Gw.
func TestAncestrySelected(t *testing.T) {
	v := validatorOver(t, ancestrySchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"a type table's types settling a child's $type differently", `{"gd":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}}`,
			`<gd ` + x + `><x/><x xsi:type="t:X"><k>zz</k></x></gd>`, none},
		{"a type table's types governing a child differently", `{"gw":{"q":[{},{"k":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}]}}`,
			`<gw ` + x + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><q/><q><k xsi:type="xs:string"/></q></gw>`, []xsderr.Rule{"cvc-complex-type", "cvc-elt"}},
		{"a type table's types settling a particle's $type differently", `{"gd":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:Le","a":"1"}}}`,
			`<gd ` + x + `><x xsi:type="t:Le" a="1"/></gd>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestAncestrySelectedBelowRoot pins TestAncestrySelected's rows one level
// down, gd and gw under hold: the mapping reads the type each one's {type
// table} ·conditionally selects· below the root (walker.selector), which
// alone governs it, and settles the "$type" under that type, as the engine
// assesses each XML twin.
func TestAncestrySelectedBelowRoot(t *testing.T) {
	v := validatorOver(t, ancestrySchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"a type table's types settling a child's $type differently", `{"hold":{"gd":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}}}`,
			`<hold ` + x + `><gd><x/><x xsi:type="t:X"><k>zz</k></x></gd></hold>`, none},
		{"a type table's types governing a child differently", `{"hold":{"gw":{"q":[{},{"k":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}]}}}`,
			`<hold ` + x + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><gw><q/><q><k xsi:type="xs:string"/></q></gw></hold>`, []xsderr.Rule{"cvc-complex-type", "cvc-elt"}},
		{"a type table's types settling a particle's $type differently", `{"hold":{"gd":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:Le","a":"1"}}}}`,
			`<hold ` + x + `><gd><x xsi:type="t:Le" a="1"/></gd></hold>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestUnattributedParentTypeDeclines pins instanceGoverned's GAP(jsonsrc)
// marker for a content model xsd.Schema.ContentMatcher declines: big's
// children are not ·attributed·, so no type is determined to govern a, and
// the "$type" on its x declines. The engine walks a undecided, recording
// cvc-complex-content as Unevaluated against it.
func TestUnattributedParentTypeDeclines(t *testing.T) {
	v := validatorOver(t, ancestrySchema)
	const xml = `<big xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl + `><a><x/><x xsi:type="t:X"><k>zz</k></x></a><b/></big>`
	xres, err := xmlsrc.Validate(v, strings.NewReader(xml), xmlsrc.WithURI("instance.xml"))
	if err != nil {
		t.Fatalf("xmlsrc.Validate(%s): %v", xml, err)
	}
	if len(xres.Unevaluated()) == 0 {
		t.Fatalf("XML twin %s charges %v and leaves nothing unevaluated; want big's content model undecided", xml, rules(xres))
	}
	const doc = `{"big":{"a":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]},"b":""}}`
	_, err = Validate(v, strings.NewReader(doc), WithURI(jsonURI))
	const want = `jsonsrc: instance.json:1:46: the "$type" member of {urn:t}x: the mapping determines no type that governs {urn:t}a`
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("Validate(%s) = %v, want it to open %q", doc, err, want)
	}
}

// TestAncestryTypeMaps pins walker.governing's arms: a "$type" is settled
// under each type that may govern its parent, read off the parent's
// ·attribution·, whatever type the parent's members are classified under.
// Under pp's "$type" Q, x takes L, where under pp's declared D it would take
// X. The second y of w is ·attributed to· the lax wildcard and governed by
// the top-level y's Tb, under whose {open content} the second x takes Lr,
// which X does not ·override·, so k is a child of Lr (key-governing-ed clause
// 3, key-governing-type-elem clause 7); its members are classified under Ta,
// the contained declaration's. The second u of w6 is governed by the top-level
// u's B, whose {open content} takes e with no ·locally declared type·, so e is
// ·laxly assessed· and X governs the second x (key-governing-type-elem clause
// 8), of which k is an attribute; B is charged cvc-complex-type clause 5
// against u's A. A skip wildcard leaves the second v of ws ·skipped·, as a
// skip {open content} does the second v of wo, and the declared Ta stands for
// each. The second v of nb, rejected because b must follow the first, and the v
// after the rejected t:c, which validate attributes to nothing once its
// matcher rejected, are governed by their ·locally declared type· Ta, where
// the top-level v's Tz would make k an attribute. A {type table} every type
// of which is one named type gives that type: the second z of zw is ·laxly
// assessed·, so g is ·attributed to· xs:anyType's wildcard and governed by
// the top-level g's Ta, whichever alternative is selected, under whose {open
// content} the second x takes L, of which k is a child (walker.governedTypes); the
// second yt of wt is governed by the top-level yt's Tt, whose x selects Lr
// either way, of which k is a child, where its members are classified under
// Ta's x of type L, the mapping reading the selection of Tt's x
// (walker.selector). gd has no a, so its table selects Ta (walker.settled),
// which, as Tn would, ·attributes· its one x to the element particle of type L,
// so L governs x and k's "$type" is settled under it (walker.governing). Each
// row agrees with its XML twin.
func TestAncestryTypeMaps(t *testing.T) {
	v := validatorOver(t, ancestrySchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"under a $type", `{"pp":{"$xmlns":{"t":"urn:t"},"$type":"t:Q","x":[{},{"$type":"t:X","k":"zz"}]}}`,
			`<pp ` + x + ` xsi:type="t:Q"><x/><x xsi:type="t:X"><k>zz</k></x></pp>`, none},
		{"a wildcard's top-level declaration governs the parent", `{"w":{"y":[{},{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}]}}`,
			`<w ` + x + `><y/><y><x/><x xsi:type="t:X"><k>zz</k></x></y></w>`, none},
		{"an open content's resolution", `{"w6":{"u":[{},{"e":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}}]}}`,
			`<w6 ` + x + `><u/><u><e><x/><x xsi:type="t:X" k="zz"/></e></u></w6>`, complexType},
		{"under an element particle before a skip wildcard", `{"ws":{"v":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}}}`,
			`<ws ` + x + `><v><x/><x xsi:type="t:X"><k>zz</k></x></v></ws>`, none},
		{"skipped", `{"ws":{"v":[{},{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}]}}`,
			`<ws ` + x + `><v/><v><x/><x xsi:type="t:X"><k>zz</k></x></v></ws>`, none},
		{"skipped by an open content", `{"wo":{"v":[{},{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}]}}`,
			`<wo ` + x + `><v/><v><x/><x xsi:type="t:X"><k>zz</k></x></v></wo>`, none},
		{"rejected", `{"nb":{"v":[{},{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}],"b":""}}`,
			`<nb ` + x + `><v/><v><x/><x xsi:type="t:X"><k>zz</k></x></v><b/></nb>`, content},
		{"after a rejected sibling", `{"nb":{"$xmlns":{"t":"urn:t"},"v":{},"t:c":"1","b":"","v":{"x":[{},{"$type":"t:X","k":"zz"}]}}}`,
			`<nb ` + x + `><v/><t:c>1</t:c><b/><v><x/><x xsi:type="t:X"><k>zz</k></x></v></nb>`, content},
		{"a wildcard's type table", `{"zw":{"z":[{},{"g":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:X","k":"zz"}]}}]}}`,
			`<zw ` + x + `><z/><z><g><x/><x xsi:type="t:X"><k>zz</k></x></g></z></zw>`, none},
		{"an element particle's type table", `{"wt":{"yt":[{},{"x":{"k":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}}]}}`,
			`<wt ` + x + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><yt/><yt><x><k xsi:type="xs:string"/></x></yt></wt>`, none},
		{"below a child of a type table's element", `{"gd":{"x":{"k":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string"}}}}`,
			`<gd ` + x + ` xmlns:xs="http://www.w3.org/2001/XMLSchema"><x><k xsi:type="xs:string"/></x></gd>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestTabledElementTypeMaps pins declaredElement.object's "$type" arm on an
// element whose declaration carries a {type table} whose selection the
// mapping reads (walker.overrode): where a is absent, the table selects L,
// which Le ·overrides· and governs, so the members are classified under Le.
// The first is an element particle's, the second the root's.
// TestTabledElementTypeDeclines pins the rows where a is present.
func TestTabledElementTypeMaps(t *testing.T) {
	v := validatorOver(t, ancestrySchema)
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"an element particle's, L selected", `{"ht":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:Le"}}}`,
			`<ht ` + x + `><x xsi:type="t:Le"/></ht>`, none},
		{"the root's, L selected", `{"hr":{"$xmlns":{"t":"urn:t"},"$type":"t:Le"}}`,
			`<hr ` + x + ` xsi:type="t:Le"/>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// selfSelectedSchema declares L, with an optional child k and the attribute
// a; Lr, which restricts L to the same; Lc, which extends L with the attribute
// c; and hc, of type L, which selects Lr where it has an a.
const selfSelectedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:complexType name="L">
    <xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:attribute name="a" type="xs:string"/>
  </xs:complexType>
  <xs:complexType name="Lr">
    <xs:complexContent>
      <xs:restriction base="t:L">
        <xs:sequence><xs:element name="k" type="xs:string" minOccurs="0"/></xs:sequence>
        <xs:attribute name="a" type="xs:string"/>
      </xs:restriction>
    </xs:complexContent>
  </xs:complexType>
  <xs:complexType name="Lc">
    <xs:complexContent><xs:extension base="t:L"><xs:attribute name="c" type="xs:string"/></xs:extension></xs:complexContent>
  </xs:complexType>
  <xs:element name="hc" type="t:L"><xs:alternative test="@a" type="t:Lr"/></xs:element>
</xs:schema>`

// TestTabledElementTypeDeclines pins walker.overrode's GAP(jsonsrc) markers: a
// "$type" whose members, classified under the type it names, select a type
// it does not ·override·, which governs the element and classifies them
// otherwise, declines; its XML twin is charged cvc-elt clause 4. In
// ancestrySchema Le, which extends L with the attribute a, ·overrides· L but
// not Lr, which x's and hr's {type table}s select where a is present: a,
// an attribute of Le, selects Lr, of which it is a child, and as a child it
// selects L, which Le ·overrides·, and of which it is an attribute (#2831). In
// selfSelectedSchema Lc ·overrides· L but not Lr, which hc's table selects
// where a is present: under Lr, a is an attribute and selects Lr again, but c
// is a child, where Lc makes it an attribute (#2832).
func TestTabledElementTypeDeclines(t *testing.T) {
	const x = `xmlns="urn:t" xmlns:t="urn:t" ` + xsiDecl
	for _, tc := range []struct {
		name, schema, json, xml string
		xmlWant                 []xsderr.Rule
		opens                   string
		owner, selected         string
	}{
		{"an element particle's, Lr selected", ancestrySchema, `{"ht":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:Le","a":"1"}}}`,
			`<ht ` + x + `><x xsi:type="t:Le" a="1"/></ht>`, []xsderr.Rule{"cvc-elt", "cvc-complex-type"},
			"jsonsrc: instance.json:1:36: ", "{urn:t}x", "{urn:t}Lr"},
		{"the root's, Lr selected", ancestrySchema, `{"hr":{"$xmlns":{"t":"urn:t"},"$type":"t:Le","a":"1"}}`,
			`<hr ` + x + ` xsi:type="t:Le" a="1"/>`, []xsderr.Rule{"cvc-elt", "cvc-complex-type"},
			"jsonsrc: instance.json:1:31: ", "{urn:t}hr", "{urn:t}Lr"},
		{"a selection the other attribute does not read", selfSelectedSchema, `{"hc":{"$xmlns":{"t":"urn:t"},"$type":"t:Lc","a":"1","c":"1"}}`,
			`<hc ` + x + ` xsi:type="t:Lc" a="1"><c>1</c></hc>`, []xsderr.Rule{"cvc-elt", "cvc-complex-content"},
			"jsonsrc: instance.json:1:31: ", "{urn:t}hc", "{urn:t}Lr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := validatorOver(t, tc.schema)
			xres, err := xmlsrc.Validate(v, strings.NewReader(tc.xml), xmlsrc.WithURI("instance.xml"))
			if err != nil || xres.Err() != nil {
				t.Fatalf("xmlsrc.Validate(%s): %v / %v", tc.xml, err, xres.Err())
			}
			if got := rules(xres); !slices.Equal(got, tc.xmlWant) {
				t.Fatalf("XML twin %s charges %v, want %v", tc.xml, got, tc.xmlWant)
			}
			_, err = Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			want := tc.opens + `the "$type" member of ` + tc.owner + `: the type its {type table} selects for the attributes its members are classified with, ` + tc.selected + `, is one the type it names does not ·override·`
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("Validate(%s) = %v, want it to open %q", tc.json, err, want)
			}
		})
	}
}

// TestGovernedParentTypeMaps pins walker.instanceGoverned where the type that
// governs the parent charges the child: the one z is ·attributed to· zw's
// element particle and governed by zt, which has no {open content}, and other
// does not ·override· x's dt there, so the engine charges cvc-elt clause 4 and
// dt, the ·selected type definition· of zt's x, governs x: the members are
// classified under dt, of which k is a child, where other would make it an
// attribute, as for the XML twin.
func TestGovernedParentTypeMaps(t *testing.T) {
	v := validatorOver(t, governanceSchema)
	twin(t, v, `{"zw":{"z":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}}}}`,
		`<zw xmlns="urn:t" xmlns:t="urn:t" `+xsiDecl+`><z><x xsi:type="t:other"><k>zz</k></x></z></zw>`,
		elt)
}
