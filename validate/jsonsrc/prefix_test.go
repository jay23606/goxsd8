package jsonsrc

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// prefixSchema declares, in urn:p, the top-level doc, whose type holds k, a
// nillable xs:integer, then any number of references to urn:o's k, and the
// attributes a, a qualified xs:integer, and a reference to urn:o's a; and the
// empty type empty; it imports the xml namespace, whose four attribute
// declarations the parser supplies. Every key of doc, unprefixed, is carried
// by two expanded names, and so is doc at the root. any holds a lax ##any
// element wildcard, local a lax ##local one, att an empty sequence and a lax
// ##any {attribute wildcard}, both wildcards of both kinds, and plain no
// wildcard; w is a top-level xs:integer.
const prefixSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:p" xmlns:p="urn:p" xmlns:o="urn:o" elementFormDefault="qualified">
  <xs:import namespace="urn:o" schemaLocation="o.xsd"/>
  <xs:import namespace="http://www.w3.org/XML/1998/namespace"/>
  <xs:element name="doc">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="k" type="xs:integer" nillable="true" minOccurs="0" maxOccurs="unbounded"/>
        <xs:element ref="o:k" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
      <xs:attribute name="a" form="qualified" type="xs:integer"/>
      <xs:attribute ref="o:a"/>
    </xs:complexType>
  </xs:element>
  <xs:complexType name="empty"/>
  <xs:element name="any">
    <xs:complexType><xs:sequence><xs:any processContents="lax" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="local">
    <xs:complexType><xs:sequence><xs:any namespace="##local" processContents="lax" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="att">
    <xs:complexType><xs:sequence/><xs:anyAttribute processContents="lax"/></xs:complexType>
  </xs:element>
  <xs:element name="both">
    <xs:complexType>
      <xs:sequence><xs:any processContents="lax" minOccurs="0"/></xs:sequence>
      <xs:anyAttribute processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="plain">
    <xs:complexType><xs:sequence><xs:element name="s" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="w" type="xs:integer"/>
</xs:schema>`

// otherSchema is urn:o's: the top-level doc, of empty content; k, holding an
// optional n; and the attribute a, an xs:string.
const otherSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:o" elementFormDefault="qualified">
  <xs:element name="doc"><xs:complexType/></xs:element>
  <xs:element name="k">
    <xs:complexType><xs:sequence><xs:element name="n" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:attribute name="a" type="xs:string"/>
</xs:schema>`

// prefixValidator is a validator over prefixSchema and the otherSchema it
// imports.
func prefixValidator(t *testing.T) *validate.Validator {
	t.Helper()
	backend := strict.New()
	schema, err := parser.Parse("p.xsd", parser.WithResolver(loader.Map(map[string]string{
		"p.xsd": prefixSchema,
		"o.xsd": otherSchema,
	})), parser.WithBackend(backend))
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	return v
}

// pDoc opens a JSON instance whose root is urn:p's doc, binding p and o.
const pDoc = `{"p:doc":{"$xmlns":{"p":"urn:p","o":"urn:o"},`

// pXML opens its XML twin.
const pXML = `<p:doc xmlns:p="urn:p" xmlns:o="urn:o"`

// TestPrefixedKey pins the prefixed-key arm: a key "p:local" names {uri}local,
// uri the binding of p in scope at the element it names, its own "$xmlns"
// included — an object value's, an array item's own — or, for an attribute
// and a scalar or null value, at the element it is a member of; and it is
// matched by expanded name at every classification step. The root, child and
// attribute rows each choose between two expanded names an unprefixed key
// declines between (TestPrefixedKeyDeclines). The undeclared-root rows are
// rootMember's: an element no top-level declaration names, in a prefixed key's
// namespace or, unprefixed, in none. The xml-namespace rows are the attribute
// an {attribute wildcard} alone admits, assessed against the xml:lang
// declaration the parser supplies. Each row agrees with its XML twin.
func TestPrefixedKey(t *testing.T) {
	v := prefixValidator(t)
	const xsi = ` ` + xsiDecl
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"root, through its own binding", `{"p:doc":{"$xmlns":{"p":"urn:p"}}}`,
			`<p:doc xmlns:p="urn:p"/>`, none},
		{"root, the other namespace's", `{"o:doc":{"$xmlns":{"o":"urn:o"}}}`,
			`<o:doc xmlns:o="urn:o"/>`, none},
		{"child, through its parent's binding", pDoc + `"o:k":{"n":"x"}}}`,
			pXML + `><o:k><o:n>x</o:n></o:k></p:doc>`, none},
		{"child, through its own binding", `{"p:doc":{"$xmlns":{"p":"urn:p"},"o:k":{"$xmlns":{"o":"urn:o"},"n":"x"}}}`,
			`<p:doc xmlns:p="urn:p"><o:k xmlns:o="urn:o"><o:n>x</o:n></o:k></p:doc>`, none},
		{"child, its own binding over its parent's", `{"p:doc":{"$xmlns":{"p":"urn:p","o":"urn:p"},"o:k":{"$xmlns":{"o":"urn:o"},"n":"x"}}}`,
			`<p:doc xmlns:p="urn:p"><o:k xmlns:o="urn:o"><o:n>x</o:n></o:k></p:doc>`, none},
		{"child, the other declaration", pDoc + `"p:k":"x"}}`,
			pXML + `><p:k>x</p:k></p:doc>`, typ},
		{"child, null", pDoc + `"p:k":null}}`,
			pXML + xsi + `><p:k xsi:nil="true"/></p:doc>`, none},
		{"array items, scalar", pDoc + `"p:k":[1,2]}}`,
			pXML + `><p:k>1</p:k><p:k>2</p:k></p:doc>`, none},
		{"array items, each through its own binding", `{"p:doc":{"$xmlns":{"p":"urn:p"},"o:k":[{"$xmlns":{"o":"urn:o"}},{"$xmlns":{"o":"urn:o"},"n":"y"}]}}`,
			`<p:doc xmlns:p="urn:p"><o:k xmlns:o="urn:o"/><o:k xmlns:o="urn:o"><o:n>y</o:n></o:k></p:doc>`, none},
		{"array items, one binding another namespace", `{"p:doc":{"$xmlns":{"p":"urn:p"},"o:k":[{"$xmlns":{"o":"urn:o"}},{"$xmlns":{"o":"urn:p"}}]}}`,
			`<p:doc xmlns:p="urn:p"><o:k xmlns:o="urn:o"/><o:k xmlns:o="urn:p"/></p:doc>`, []xsderr.Rule{"cvc-complex-content", "cvc-type"}},
		{"attributes", pDoc + `"o:a":"x","p:a":5}}`,
			pXML + ` o:a="x" p:a="5"/>`, none},
		{"attribute, the other use", pDoc + `"p:a":"x"}}`,
			pXML + ` p:a="x"/>`, attr},
		{"undeclared root, prefixed", `{"p:none":{"$xmlns":{"p":"urn:p"},"$type":"p:empty"}}`,
			`<p:none xmlns:p="urn:p"` + xsi + ` xsi:type="p:empty"/>`, none},
		{"undeclared root, its $type unprefixed", `{"p:none":{"$xmlns":{"p":"urn:p"},"$type":"empty"}}`,
			`<p:none xmlns:p="urn:p" xmlns="urn:p"` + xsi + ` xsi:type="empty"/>`, none},
		{"undeclared root, unprefixed", `{"none":{"$xmlns":{"p":"urn:p"},"$type":"p:empty"}}`,
			`<none xmlns:p="urn:p"` + xsi + ` xsi:type="p:empty"/>`, none},
		{"undeclared root, no $type", `{"none":{}}`,
			`<none/>`, assessElt},
		{"undeclared root, scalar", `{"none":"x"}`,
			`<none>x</none>`, assessElt},
		{"##any element wildcard", `{"p:any":{"$xmlns":{"p":"urn:p","z":"urn:z"},"p:w":5,"z:q":"x"}}`,
			`<p:any xmlns:p="urn:p" xmlns:z="urn:z"><p:w>5</p:w><z:q>x</z:q></p:any>`, none},
		{"##any element wildcard, a declaration's type", `{"p:any":{"$xmlns":{"p":"urn:p"},"p:w":"x"}}`,
			`<p:any xmlns:p="urn:p"><p:w>x</p:w></p:any>`, typ},
		{"##local element wildcard", `{"p:local":{"$xmlns":{"p":"urn:p","z":"urn:z"},"z:q":"x"}}`,
			`<p:local xmlns:p="urn:p" xmlns:z="urn:z"><z:q>x</z:q></p:local>`, content},
		{"##any attribute wildcard", `{"p:att":{"$xmlns":{"p":"urn:p","z":"urn:z"},"z:q":"x"}}`,
			`<p:att xmlns:p="urn:p" xmlns:z="urn:z" z:q="x"/>`, none},
		{"##any attribute wildcard, the xml namespace", `{"p:att":{"$xmlns":{"p":"urn:p"},"xml:lang":"en"}}`,
			`<p:att xmlns:p="urn:p" xml:lang="en"/>`, none},
		{"##any attribute wildcard, the xml namespace's declaration", `{"p:att":{"$xmlns":{"p":"urn:p"},"xml:lang":"not a language"}}`,
			`<p:att xmlns:p="urn:p" xml:lang="not a language"/>`, attr},
		{"no wildcard", `{"p:plain":{"$xmlns":{"p":"urn:p","z":"urn:z"},"z:s":"x"}}`,
			`<p:plain xmlns:p="urn:p" xmlns:z="urn:z"><z:s>x</z:s></p:plain>`, content},
		{"no wildcard, an unprefixed key keeps its parent's namespace", `{"p:plain":{"$xmlns":{"p":"urn:p"},"s":"x"}}`,
			`<p:plain xmlns:p="urn:p"><p:s>x</p:s></p:plain>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestPrefixedKeyName pins the expanded name a prefixed key gives a child no
// declaration carries under a type with no wildcard, which TestPrefixedKey's
// verdicts cannot tell from (O,key): every XML twin of such a key is invalid
// alike (walker.wildcardMember).
func TestPrefixedKeyName(t *testing.T) {
	v := prefixValidator(t)
	w := newWalker(jsonURI, strings.NewReader(`{"p:plain":{"$xmlns":{"p":"urn:p","z":"urn:z"},"z:s":"x","s":"y"}}`), v.Schema(), v.ConditionallySelected)
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	var got []xsd.QName
	kids := root.Children()
	for {
		c, ok := kids.Next()
		if !ok {
			break
		}
		if e, ok := c.Element(); ok {
			got = append(got, e.Name())
		}
	}
	if err := kids.Err(); err != nil {
		t.Fatalf("children: %v", err)
	}
	want := []xsd.QName{{Space: "urn:z", Local: "s"}, {Space: "urn:p", Local: "s"}}
	if !slices.Equal(got, want) {
		t.Fatalf("children's names = %v, want %v", got, want)
	}
}

// TestPrefixedKeyDeclines pins every key the prefixed-key arm does not name:
// each is (nil, err), err opening with "jsonsrc: <uri>:<line>:<col>: " at the
// key, or at the array for an attribute's array value. A colon structure no
// QName has and an unbound prefix have no XML twin (walker.keyName); an
// unprefixed key two expanded names carry is RULED permanent by #2623
// (rootMember, childNamed, useNamed); a prefix keeps the kind-unfixed declines
// RULED permanent by #2634, and the xsi-namespace ones and the xml-namespace
// child element RULED permanent by #2635 (wildcardMember).
func TestPrefixedKeyDeclines(t *testing.T) {
	v := prefixValidator(t)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"unbound prefix at the root", `{"p:doc":{}}`,
			"jsonsrc: instance.json:1:2: ", `member "p:doc": no "$xmlns" in scope binds its prefix "p"; a prefixed key names the expanded name its prefix's binding gives it, so bind "p" in a "$xmlns" member, {"p":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`},
		{"unbound prefix, a scalar root", `{"p:doc":"x"}`,
			"jsonsrc: instance.json:1:2: ", `member "p:doc": no "$xmlns" in scope binds its prefix "p", and a root whose value is a scalar or null has no object for a "$xmlns" member to lead, so "xml" is the one prefix bound there; key it {"xml:doc":...} for the element of that local name in the namespace http://www.w3.org/XML/1998/namespace, or by its local name alone, {"doc":...}, which names the top-level element declaration of that local name where exactly one carries it`},
		{"unbound prefix, a null root", `{"p:w":null}`,
			"jsonsrc: instance.json:1:2: ", `member "p:w": no "$xmlns" in scope binds its prefix "p", and a root whose value is a scalar or null has no object for a "$xmlns" member to lead, so "xml" is the one prefix bound there; key it {"xml:w":...} for the element of that local name in the namespace http://www.w3.org/XML/1998/namespace, or by its local name alone, {"w":...}, which names the top-level element declaration of that local name where exactly one carries it`},
		{"unbound prefix in a child", pDoc + `"z:k":1}}`,
			"jsonsrc: instance.json:1:46: ", `member "z:k": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`},
		{"bound in a sibling only", pDoc + `"o:k":{"$xmlns":{"z":"urn:o"}},"z:k":{}}}`,
			"jsonsrc: instance.json:1:77: ", `member "z:k": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`},
		{"the xmlns prefix", pDoc + `"xmlns:z":"urn:z"}}`,
			"jsonsrc: instance.json:1:46: ", `member "xmlns:z": no "$xmlns" in scope binds its prefix "xmlns"; a namespace declaration is a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element it scopes, and never an xmlns: key`},
		{"two colons", pDoc + `"p:k:x":1}}`,
			"jsonsrc: instance.json:1:46: ", `member "p:k:x" is no QName`},
		{"empty prefix", pDoc + `":k":1}}`,
			"jsonsrc: instance.json:1:46: ", `member ":k" is no QName`},
		{"empty local part", pDoc + `"p:":1}}`,
			"jsonsrc: instance.json:1:46: ", `member "p:" is no QName`},
		{"local part no NCName", pDoc + `"p:1k":1}}`,
			"jsonsrc: instance.json:1:46: ", `member "p:1k" is no QName`},
		{"unprefixed root of two expanded names", `{"doc":{}}`,
			"jsonsrc: instance.json:1:2: ", `root member "doc" names top-level element declarations of 2 expanded names ({urn:p}doc and {urn:o}doc); an unprefixed key does not choose between them, and a prefixed one, "p:doc", names one`},
		{"unprefixed scalar root of two expanded names", `{"doc":"x"}`,
			"jsonsrc: instance.json:1:2: ", `root member "doc" names top-level element declarations of 2 expanded names ({urn:p}doc and {urn:o}doc); an unprefixed key does not choose between them, and a root whose value is a scalar or null has no object for a "$xmlns" member to lead, so "xml" is the one prefix bound there and no key names any of them with this value`},
		{"unprefixed child of two expanded names", pDoc + `"k":1}}`,
			"jsonsrc: instance.json:1:46: ", `member "k" names child element declarations of two expanded names in the type of {urn:p}doc ({urn:p}k and {urn:o}k); an unprefixed key does not choose between them`},
		{"unprefixed attribute of two expanded names", pDoc + `"a":1}}`,
			"jsonsrc: instance.json:1:46: ", `member "a" names attribute uses of two expanded names in the type of {urn:p}doc ({urn:p}a and {urn:o}a); an unprefixed key does not choose between them`},
		{"attribute, an array value", pDoc + `"p:a":["5"]}}`,
			"jsonsrc: instance.json:1:52: ", `attribute member "p:a" of {urn:p}doc is not a string, number or boolean`},
		{"attribute, an empty array value", pDoc + `"p:a":[]}}`,
			"jsonsrc: instance.json:1:52: ", `attribute member "p:a" of {urn:p}doc is not a string, number or boolean`},
		{"unbound prefix, an empty array value", pDoc + `"z:k":[]}}`,
			"jsonsrc: instance.json:1:46: ", `member "z:k": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`},
		{"unbound prefix, an array item", pDoc + `"p:k":[1],"z:k":[{"$xmlns":{"y":"urn:o"}}]}}`,
			"jsonsrc: instance.json:1:56: ", `member "z:k": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`},
		{"both kinds of wildcard", `{"p:both":{"$xmlns":{"p":"urn:p"},"p:w":5}}`,
			"jsonsrc: instance.json:1:35: ", `member "p:w" names no child element declaration and no attribute use of the type of {urn:p}both, which has both an element wildcard and an {attribute wildcard}`},
		{"member of an undeclared root", `{"p:none":{"$xmlns":{"p":"urn:p"},"p:w":5}}`,
			"jsonsrc: instance.json:1:35: ", `member "p:w" of {urn:p}none, an element no contained and no top-level element declaration names`},
		{"the xsi namespace under a wildcard", `{"p:any":{"$xmlns":{"p":"urn:p","i":"http://www.w3.org/2001/XMLSchema-instance"},"i:type":"p:empty"}}`,
			"jsonsrc: instance.json:1:82: ", `member "i:type" names no child element declaration and no attribute use of the type of {urn:p}any, and its prefix puts it in the namespace http://www.w3.org/2001/XMLSchema-instance`},
		{"the xml namespace under an element wildcard", `{"p:any":{"$xmlns":{"p":"urn:p"},"xml:lang":"en"}}`,
			"jsonsrc: instance.json:1:34: ", `member "xml:lang" names no child element declaration and no attribute use of the type of {urn:p}any, and its prefix puts it in the namespace http://www.w3.org/XML/1998/namespace; the mapping names no child element in the xml namespace`},
		{"the xml namespace with no wildcard", `{"p:plain":{"$xmlns":{"p":"urn:p"},"xml:lang":"en"}}`,
			"jsonsrc: instance.json:1:36: ", `and its prefix puts it in the namespace http://www.w3.org/XML/1998/namespace; the mapping names no child element in the xml namespace`},
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

// xmlRootSchema declares doc in urn:p and imports xmlRootXMLSchema, which
// declares doc and note in the xml namespace: doc at the root is carried by
// two expanded names, one of them the xml namespace's.
const xmlRootSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:p">
  <xs:import namespace="http://www.w3.org/XML/1998/namespace" schemaLocation="xml.xsd"/>
  <xs:element name="doc" type="xs:string"/>
</xs:schema>`

// xmlRootXMLSchema is xmlRootSchema's xml-namespace document.
const xmlRootXMLSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="http://www.w3.org/XML/1998/namespace">
  <xs:element name="doc" type="xs:string"/>
  <xs:element name="note" type="xs:string"/>
</xs:schema>`

// TestScalarRootXMLPrefix pins the "xml" prefix as the one bound at a root
// whose value is a scalar or null (walker.keyName, rootMember): an "xml:" key
// names the xml namespace's declaration, as its XML twin does, and the
// declines that a prefixed key such a root cannot bind, and an unprefixed key
// two expanded names carry, name that "xml:" key.
func TestScalarRootXMLPrefix(t *testing.T) {
	backend := strict.New()
	schema, err := parser.Parse("p.xsd", parser.WithResolver(loader.Map(map[string]string{
		"p.xsd":   xmlRootSchema,
		"xml.xsd": xmlRootXMLSchema,
	})), parser.WithBackend(backend))
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	twin(t, v, `{"xml:note":"x"}`, `<xml:note>x</xml:note>`, nil)
	twin(t, v, `{"xml:doc":"x"}`, `<xml:doc>x</xml:doc>`, nil)
	for _, tc := range []struct {
		name, json string
		says       string
	}{
		{"unbound prefix", `{"p:note":"x"}`,
			`member "p:note": no "$xmlns" in scope binds its prefix "p", and a root whose value is a scalar or null has no object for a "$xmlns" member to lead, so "xml" is the one prefix bound there; key it {"xml:note":...} for the element of that local name in the namespace http://www.w3.org/XML/1998/namespace, or by its local name alone, {"note":...}, which names the top-level element declaration of that local name where exactly one carries it`},
		{"unprefixed key of two expanded names", `{"doc":"x"}`,
			`root member "doc" names top-level element declarations of 2 expanded names ({urn:p}doc and {http://www.w3.org/XML/1998/namespace}doc); an unprefixed key does not choose between them, and a root whose value is a scalar or null has no object for a "$xmlns" member to lead, so "xml" is the one prefix bound there: key it "xml:doc" for {http://www.w3.org/XML/1998/namespace}doc, and no key names any other of them with this value`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, nil error; want the decline", tc.json, rules(res))
			}
			if want := "jsonsrc: instance.json:1:2: " + tc.says; err.Error() != want {
				t.Errorf("Validate(%s) = %q, want %q", tc.json, err, want)
			}
		})
	}
}
