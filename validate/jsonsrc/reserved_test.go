package jsonsrc

import (
	"slices"
	"strings"
	"testing"

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
// after it are classified under the type it names wherever that resolves.
// Each row agrees with its XML twin.
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
	w := newWalker(jsonURI, strings.NewReader(`{"item":{"$type":" derived ","d":"1","a":"x","b":"y"}}`), v.Schema())
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
			"jsonsrc: instance.json:1:18: ", `the "$type" member of {urn:t}item is not a string`},
		{"$xmlns not an object", `{"item":{"$xmlns":"urn:t"}}`,
			"jsonsrc: instance.json:1:19: ", `the "$xmlns" member of "item" is not an object`},
		{"$xmlns an array", `{"item":{"$xmlns":[]}}`,
			"jsonsrc: instance.json:1:19: ", `the "$xmlns" member of "item" is not an object`},
		{"$xmlns binding not a string", `{"item":{"$xmlns":{"t":null}}}`,
			"jsonsrc: instance.json:1:24: ", `the "$xmlns" member of "item" binds "t" to a value that is not a string`},
		{"$xmlns binding a prefix twice", `{"item":{"$xmlns":{"t":"urn:t","t":"urn:t"}}}`,
			"jsonsrc: instance.json:1:32: ", `the "$xmlns" member of "item" binds "t" twice`},
		{"$xmlns binding the empty prefix twice", `{"item":{"$xmlns":{"":"urn:t","":"urn:t"}}}`,
			"jsonsrc: instance.json:1:31: ", `binds "" twice`},
		{"$xmlns binding a prefix no NCName starts", `{"item":{"$xmlns":{"1x":"urn:t"},"$type":"1x:derived","a":"x","b":"y"}}`,
			"jsonsrc: instance.json:1:20: ", `the "$xmlns" member of "item" binds "1x", which is not an NCName`},
		{"$xmlns binding a prefix with a space", `{"item":{"$xmlns":{"a b":"urn:t"},"$type":"a b:derived","a":"x","b":"y"}}`,
			"jsonsrc: instance.json:1:20: ", `binds "a b", which is not an NCName`},
		{"$xmlns binding a prefix with a colon", `{"item":{"$xmlns":{"a:b":"urn:t"}}}`,
			"jsonsrc: instance.json:1:20: ", `binds "a:b", which is not an NCName`},
		{"$xmlns binding xml", `{"item":{"$xmlns":{"xml":"http://www.w3.org/XML/1998/namespace"}}}`,
			"jsonsrc: instance.json:1:20: ", `the "$xmlns" member of "item" binds the reserved prefix "xml"`},
		{"$xmlns binding xmlns", `{"item":{"$xmlns":{"xmlns":"urn:t"}}}`,
			"jsonsrc: instance.json:1:20: ", `binds the reserved prefix "xmlns"`},
		{"$xmlns undeclaring a prefix", `{"item":{"$xmlns":{"t":""}}}`,
			"jsonsrc: instance.json:1:24: ", `binds "t" to the empty string; a prefix cannot be undeclared`},
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

// TestOpenContentTypeDeclines pins walker.localType: a "$type" on an element a
// non-skip {open content} may take, whose name has a ·locally declared type·
// there, declines whatever it names. The first two rows are #2615's two
// reproductions, where other does not ·override· dt; the third names dt
// itself; the fourth finds dt on r's base chain alone.
func TestOpenContentTypeDeclines(t *testing.T) {
	v := validatorOver(t, openContentSchema)
	const says = `the {open content} of the type of`
	for _, tc := range []struct {
		name, json string
		prefix     string
	}{
		{"its own attribute", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","foo":"1"}]}}`,
			"jsonsrc: instance.json:1:39: "},
		{"its descendant", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","c":{"foo":"1"}}]}}`,
			"jsonsrc: instance.json:1:39: "},
		{"whatever it names", `{"p":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:dt"}]}}`,
			"jsonsrc: instance.json:1:39: "},
		{"declared on the base chain alone", `{"r":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other"}}}`,
			"jsonsrc: instance.json:1:35: "},
		{"prefixed keys", `{"t:p":{"$xmlns":{"t":"urn:t"},"t:x":[{},{"$type":"t:other","foo":"1"}]}}`,
			"jsonsrc: instance.json:1:43: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, %d unevaluated, nil error; want the decline", tc.json, rules(res), len(res.Unevaluated()))
			}
			if res != nil {
				t.Errorf("Validate(%s) returned a result beside its error", tc.json)
			}
			if !strings.HasPrefix(err.Error(), tc.prefix+`the "$type" member of {urn:t}x: `) || !strings.Contains(err.Error(), says) {
				t.Errorf("Validate(%s) = %q, want it to open %q and say %q", tc.json, err, tc.prefix, says)
			}
		})
	}
}

// TestOpenContentTypeMaps pins localType's limit: the same "$type" maps under
// a parent with no {open content} and under one whose {open content} is
// skip, and the engine charges cvc-elt clause 4 there, as for the XML twin.
func TestOpenContentTypeMaps(t *testing.T) {
	v := validatorOver(t, openContentSchema)
	for _, tc := range []struct{ name, json, xml string }{
		{"no open content", `{"q":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other","foo":"1"}}}`,
			`<q xmlns="urn:t" ` + xsiDecl + ` xmlns:t="urn:t"><x xsi:type="t:other" foo="1"/></q>`},
		{"skip open content", `{"s":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other","foo":"1"}}}`,
			`<s xmlns="urn:t" ` + xsiDecl + ` xmlns:t="urn:t"><x xsi:type="t:other" foo="1"/></s>`},
		{"no open content, prefixed keys", `{"t:q":{"$xmlns":{"t":"urn:t"},"t:x":{"$type":"t:other","foo":"1"}}}`,
			`<t:q ` + xsiDecl + ` xmlns:t="urn:t"><t:x xsi:type="t:other" foo="1"/></t:q>`},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, elt) })
	}
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

// TestUngovernedParentTypeDeclines pins walker.governs: a "$type" declines
// under a parent whose classifying type may not govern it. The first row is
// the round-3 reproduction: the second y is matched to w's declaration of
// type Td but governed by the top-level y's Tg, whose lax {open content}
// leaves the second x to its ·locally declared type· dt, uncharged. The
// second row carries that doubt one level down, to v, where Tg2's vr, not
// Td2's vt, declares x. The third row's u is named by r's {open content} and
// governed by its ·locally declared type· ub, not by the top-level u's ug.
// The fourth row reaches mt through the substitution group of h, and
// declines on mt's own {open content}.
func TestUngovernedParentTypeDeclines(t *testing.T) {
	v := validatorOver(t, governanceSchema)
	const (
		ungoverned = `may not govern it`
		open       = `the {open content} of the type of`
	)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"a wildcard's top-level declaration governs the parent", `{"w":{"y":[{},{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}]}}`,
			"jsonsrc: instance.json:1:48: ", ungoverned},
		{"prefixed keys", `{"t:w":{"$xmlns":{"t":"urn:t"},"t:y":[{},{"t:x":[{},{"$type":"t:other","k":"zz"}]}]}}`,
			"jsonsrc: instance.json:1:54: ", ungoverned},
		{"a grandparent's", `{"w2":{"y2":[{},{"v":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}}]}}`,
			"jsonsrc: instance.json:1:55: ", ungoverned},
		{"a locally declared type governs the parent", `{"r":{"u":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}}}`,
			"jsonsrc: instance.json:1:44: ", ungoverned},
		{"a substitution group member's open content", `{"sg":{"m":{"x":[{},{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}]}}}`,
			"jsonsrc: instance.json:1:45: ", open},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, %d unevaluated, nil error; want the decline", tc.json, rules(res), len(res.Unevaluated()))
			}
			if !strings.HasPrefix(err.Error(), tc.prefix+`the "$type" member of {urn:t}x: `) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Validate(%s) = %q, want it to open %q and say %q", tc.json, err, tc.prefix, tc.says)
			}
		})
	}
}

// TestGovernedParentTypeMaps pins walker.governs' limit: under a lax wildcard
// no top-level declaration of z can govern z in its declaration's place, so
// a "$type" on z's child maps, and the engine charges cvc-elt clause 4 as
// for the XML twin.
func TestGovernedParentTypeMaps(t *testing.T) {
	v := validatorOver(t, governanceSchema)
	twin(t, v, `{"zw":{"z":{"x":{"$xmlns":{"t":"urn:t"},"$type":"t:other","k":"zz"}}}}`,
		`<zw xmlns="urn:t" xmlns:t="urn:t" `+xsiDecl+`><z><x xsi:type="t:other" k="zz"/></z></zw>`,
		[]xsderr.Rule{"cvc-elt", "cvc-complex-type"})
}
