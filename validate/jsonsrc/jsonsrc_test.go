package jsonsrc

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsderr"
)

const jsonURI = "instance.json"

// orderSchema declares one root, order, whose sequence holds every shape the
// arms map: size, a scalar of xs:integer; item, an element-only child that
// repeats once or twice and holds a patterned sku; and an optional reference
// to the head note, whose substitution group memo joins. Local declarations
// are qualified, so a JSON key's namespace comes from its declaration.
const orderSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns="urn:t" elementFormDefault="qualified">
  <xs:element name="order">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="size" type="xs:integer"/>
        <xs:element name="item" maxOccurs="2">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="sku" type="sku"/>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
        <xs:element ref="note" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:simpleType name="sku">
    <xs:restriction base="xs:string">
      <xs:pattern value="[A-Z]{3}"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:element name="note" type="xs:string"/>
  <xs:element name="memo" type="xs:string" substitutionGroup="note"/>
</xs:schema>`

// validatorOver is a validator over the one schema document doc.
func validatorOver(t *testing.T, doc string) *validate.Validator {
	t.Helper()
	backend := strict.New()
	schema, err := parser.Parse("main.xsd", parser.WithResolver(loader.Map(map[string]string{"main.xsd": doc})),
		parser.WithBackend(backend))
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	return v
}

// rules is a result's violation rule IDs, in the order the result lists them.
func rules(res *validate.Result) []xsderr.Rule {
	var out []xsderr.Rule
	for _, v := range res.Violations() {
		out = append(out, v.Rule)
	}
	return out
}

// twin pins one JSON instance against the XML document it is equivalent to:
// both are assessed against v, neither faults, and both charge want, the same
// rule IDs in the same order.
func twin(t *testing.T, v *validate.Validator, jsonDoc, xmlDoc string, want []xsderr.Rule) {
	t.Helper()
	xres, err := xmlsrc.Validate(v, strings.NewReader(xmlDoc), xmlsrc.WithURI("instance.xml"))
	if err != nil || xres.Err() != nil {
		t.Fatalf("xmlsrc.Validate(%s): %v / %v", xmlDoc, err, xres.Err())
	}
	if got := rules(xres); !slices.Equal(got, want) {
		t.Fatalf("XML twin %s charges %v, want %v: the fixture does not say what the test claims", xmlDoc, got, want)
	}
	jres, err := Validate(v, strings.NewReader(jsonDoc), WithURI(jsonURI))
	if err != nil {
		t.Fatalf("Validate(%s): %v", jsonDoc, err)
	}
	if jres.Err() != nil {
		t.Fatalf("Validate(%s): Result.Err = %v; a jsonsrc result never carries one", jsonDoc, jres.Err())
	}
	if got := rules(jres); !slices.Equal(got, want) {
		t.Fatalf("Validate(%s) charges %v, want %v, as its XML twin %s does", jsonDoc, got, want, xmlDoc)
	}
	if u := jres.Unevaluated(); len(u) != len(xres.Unevaluated()) {
		t.Fatalf("Validate(%s) left %d checks unevaluated, its twin %d", jsonDoc, len(u), len(xres.Unevaluated()))
	}
}

var (
	none        []xsderr.Rule
	complexType = []xsderr.Rule{"cvc-complex-type"}
	content     = []xsderr.Rule{"cvc-complex-content"}
	typ         = []xsderr.Rule{"cvc-type"}
)

// TestScalarMemberIsSimpleContent pins the scalar arm: a scalar-valued member
// is its element's simple content, {"size":10} ≡ <size>10</size>. A number's
// lexical form is its source text — 12345678901234567890 is an xs:integer,
// and a float64 round trip would spell it 1.2345678901234567e+19 — and the
// empty string is no text at all, as <size/> holds none.
func TestScalarMemberIsSimpleContent(t *testing.T) {
	v := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"valid number", `{"order":{"size":10,"item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size>10</size><item><sku>ABC</sku></item></order>`, none},
		{"valid integer past float64", `{"order":{"size":12345678901234567890,"item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size>12345678901234567890</size><item><sku>ABC</sku></item></order>`, none},
		{"valid string spelling a number", `{"order":{"size":"10","item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size>10</size><item><sku>ABC</sku></item></order>`, none},
		{"invalid lexical", `{"order":{"size":"ten","item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size>ten</size><item><sku>ABC</sku></item></order>`, typ},
		{"invalid boolean", `{"order":{"size":true,"item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size>true</size><item><sku>ABC</sku></item></order>`, typ},
		{"invalid fraction", `{"order":{"size":1.5,"item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size>1.5</size><item><sku>ABC</sku></item></order>`, typ},
		{"empty string is no text", `{"order":{"size":"","item":{"sku":"ABC"}}}`,
			`<order xmlns="urn:t"><size/><item><sku>ABC</sku></item></order>`, typ},
		{"scalar to element-only content", `{"order":{"size":1,"item":"ABC"}}`,
			`<order xmlns="urn:t"><size>1</size><item>ABC</item></order>`, complexType},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestObjectMemberIsChildElement pins the child arm: a member whose key is
// the local name of a child element declaration of the enclosing type is that
// child, in member order, and an object value carries its children. Members
// out of the content model's order are children out of it.
func TestObjectMemberIsChildElement(t *testing.T) {
	v := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"valid", `{"order":{"size":1,"item":{"sku":"ABC"},"note":"n"}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item><note>n</note></order>`, none},
		{"invalid grandchild", `{"order":{"size":1,"item":{"sku":"abc"}}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>abc</sku></item></order>`, typ},
		{"invalid order", `{"order":{"item":{"sku":"ABC"},"size":1}}`,
			`<order xmlns="urn:t"><item><sku>ABC</sku></item><size>1</size></order>`, content},
		{"duplicate keys map in order", `{"order":{"size":1,"item":{"sku":"ABC"},"size":2}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item><size>2</size></order>`, content},
		{"substitution-group member", `{"order":{"size":1,"item":{"sku":"ABC"},"memo":"m"}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item><memo>m</memo></order>`, none},
		{"empty object is no children", `{"order":{"size":1,"item":{}}}`,
			`<order xmlns="urn:t"><size>1</size><item/></order>`, content},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestArrayMemberIsRepeatedChildren pins the array arm: an array value is one
// child per item under the member's key, in item order, and an empty array is
// no child at all.
func TestArrayMemberIsRepeatedChildren(t *testing.T) {
	v := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"valid", `{"order":{"size":1,"item":[{"sku":"ABC"},{"sku":"DEF"}]}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item><item><sku>DEF</sku></item></order>`, none},
		{"valid scalar items", `{"order":{"size":[1],"item":{"sku":["ABC"]}}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item></order>`, none},
		{"invalid, too many", `{"order":{"size":1,"item":[{"sku":"ABC"},{"sku":"DEF"},{"sku":"GHI"}]}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item><item><sku>DEF</sku></item><item><sku>GHI</sku></item></order>`, content},
		{"invalid, empty array", `{"order":{"size":1,"item":[]}}`,
			`<order xmlns="urn:t"><size>1</size></order>`, content},
		{"invalid item, in order", `{"order":{"size":1,"item":[{"sku":"ABC"},{"sku":"d"}]}}`,
			`<order xmlns="urn:t"><size>1</size><item><sku>ABC</sku></item><item><sku>d</sku></item></order>`, typ},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// declinesSchema adds to order's shapes the three a key cannot be mapped
// through: two child declarations of one local name in two namespaces (an
// unqualified local a, and a reference to the top-level urn:t a), a lax
// wildcard beside a top-level w it would admit, and a child declaration with a
// {type table}.
const declinesSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t">
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="a" type="xs:string" minOccurs="0"/>
        <xs:element ref="t:a" minOccurs="0"/>
        <xs:element name="tt" minOccurs="0" type="xs:anyType">
          <xs:alternative test="@k" type="xs:string"/>
        </xs:element>
        <xs:element name="s" type="xs:string" minOccurs="0" maxOccurs="unbounded"/>
        <xs:element name="c" minOccurs="0">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="s" type="xs:string" minOccurs="0"/>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
        <xs:any processContents="lax" namespace="##any" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="a" type="xs:string"/>
  <xs:element name="w" type="xs:string"/>
</xs:schema>`

// TestOutOfSliceInputDeclines pins every input the implemented arms do not
// map: each is (nil, err), err opening with "jsonsrc: <uri>:<line>:<col>: "
// at the construct it names and naming the arm it is not.
func TestOutOfSliceInputDeclines(t *testing.T) {
	v := validatorOver(t, declinesSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string // the error's opening, location included
		says       string // the arm or shape it names
	}{
		{"member matching no child declaration", `{"r":{"x":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "x" names no child element declaration of the type of {urn:t}r; the attribute and wildcard member arms are not implemented`},
		{"member a wildcard would admit", `{"r":{"w":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "w" names no child element declaration`},
		{"null", `{"r":{"s":null}}`,
			"jsonsrc: instance.json:1:11: ", `member "s" is null; the null => xsi:nil arm is not implemented`},
		{"null array item", `{"r":{"s":["x",null]}}`,
			"jsonsrc: instance.json:1:16: ", `member "s" is null; the null => xsi:nil arm`},
		{"reserved member", `{"r":{"$type":"t:x"}}`,
			"jsonsrc: instance.json:1:7: ", `reserved member "$type"; the "$type" and "$xmlns" reserved-member arms are not implemented`},
		{"reserved root member", `{"$xmlns":{}}`,
			"jsonsrc: instance.json:1:2: ", `reserved member "$xmlns"`},
		{"key of two expanded names", `{"r":{"a":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "a" names child element declarations of two expanded names in the type of {urn:t}r (a and {urn:t}a)`},
		{"declaration with a type table", `{"r":{"tt":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `element tt carries a {type table}; conditional type assignment is not implemented`},
		{"nested array", `{"r":{"s":[["x"]]}}`,
			"jsonsrc: instance.json:1:12: ", `an array is an item of the array of member "s"`},
		{"member of a simple-typed element", `{"r":{"s":{"x":1}}}`,
			"jsonsrc: instance.json:1:12: ", `member "x" names no child element declaration of the type of s;`},
		{"decline after a mapped sibling", `{"r":{"s":"1","a":"1"}}`,
			"jsonsrc: instance.json:1:15: ", `member "a" names child element declarations of two expanded names`},
		{"second member after an assessed root", `{"r":{"c":{"s":"1"}},"x":null}`,
			"jsonsrc: instance.json:1:22: ", `the top-level object has a second member "x"`},
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

// TestRootNaming pins how the validation root is named: by the one member of
// a top-level object, matched by local name against the top-level element
// declarations, exactly one of which must carry it.
func TestRootNaming(t *testing.T) {
	v := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"top level not an object", `["order"]`,
			"jsonsrc: instance.json:1:1: ", "the top-level value is not an object"},
		{"top level empty", `{}`,
			"jsonsrc: instance.json:1:2: ", "the top-level object has no member"},
		{"no top-level value", ``,
			"jsonsrc: instance.json:1:1: ", "the document holds no JSON value"},
		{"root naming no declaration", `{"size":1}`,
			"jsonsrc: instance.json:1:2: ", `root member "size" names no top-level element declaration`},
		{"root value an array", `{"order":[]}`,
			"jsonsrc: instance.json:1:10: ", `the value of root member "order" is an array`},
		{"root value null", `{"order":null}`,
			"jsonsrc: instance.json:1:10: ", `member "order" is null; the null => xsi:nil arm is not implemented`},
		{"second root member", `{"order":{"size":1,"item":{"sku":"ABC"}},"order":{}}`,
			"jsonsrc: instance.json:1:42: ", `the top-level object has a second member "order"`},
		{"trailing value", `{"order":{"size":1,"item":{"sku":"ABC"}}} {}`,
			"jsonsrc: instance.json:1:43: ", "content follows the top-level object"},
		{"malformed", `{"order":{"size":1,}}`,
			"jsonsrc: instance.json:1:", "malformed JSON"},
		{"truncated", `{"order":{"size":1`,
			"jsonsrc: instance.json:1:", "malformed JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, nil error; want the decline", tc.json, rules(res))
			}
			if !strings.HasPrefix(err.Error(), tc.prefix) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Validate(%s) = %q, want it to open %q and say %q", tc.json, err, tc.prefix, tc.says)
			}
		})
	}
}

// TestRootOfTwoExpandedNames pins the root declining when two top-level
// declarations share its local name in two namespaces.
func TestRootOfTwoExpandedNames(t *testing.T) {
	b := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a">
  <xs:import namespace="urn:b" schemaLocation="b.xsd"/>
  <xs:element name="r" type="xs:string"/>
</xs:schema>`
	backend := strict.New()
	schema, err := parser.Parse("a.xsd", parser.WithResolver(loader.Map(map[string]string{
		"a.xsd": b,
		"b.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:b"><xs:element name="r" type="xs:string"/></xs:schema>`,
	})), parser.WithBackend(backend))
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	_, err = Validate(v, strings.NewReader(`{"r":"x"}`), WithURI(jsonURI))
	const want = `jsonsrc: instance.json:1:2: root member "r" names top-level element declarations of 2 expanded names ({urn:a}r and {urn:b}r)`
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("Validate = %v, want it to open %q", err, want)
	}
}

// TestLocations pins the Loc of every item the mapping yields: an element at
// its key, or at its item in an array, and a text run at its scalar, counted
// by line and byte column from 1.
func TestLocations(t *testing.T) {
	v := validatorOver(t, orderSchema)
	doc := "{\n  \"order\": {\n    \"size\": \"x\",\n    \"item\": [ {\"sku\": \"ABC\"},\n      {\"sku\": 7} ]\n  }\n}\n"
	res, err := Validate(v, strings.NewReader(doc), WithURI(jsonURI))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var got []string
	for _, e := range res.Violations() {
		got = append(got, e.Loc.String())
	}
	// size's value fails xs:integer at the element, which the key locates;
	// the second sku's 7 fails the pattern at its element, inside the
	// array's second item.
	want := []string{"instance.json:3:5", "instance.json:5:8"}
	if !slices.Equal(got, want) {
		t.Fatalf("violation locations = %v, want %v (violations %v)", got, want, res.Violations())
	}
}

// TestDrainClassifies pins the walk's end: a subtree the assessment never
// descended into is still read, under the same classification, so a decline
// inside it is reported rather than read past.
func TestDrainClassifies(t *testing.T) {
	v := validatorOver(t, orderSchema)
	w := newWalker(jsonURI, strings.NewReader(`{"order":{"size":1,"item":[{"sku":"ABC"},{"sku":null}]}}`), v.Schema())
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	err = w.end(root)
	const want = `jsonsrc: instance.json:1:49: member "sku" is null`
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("end = %v, want it to open %q", err, want)
	}
}

// TestChildrenAfterTheWalkLeftPanics pins the late-descent guard: once the
// parent's cursor moved past an element, its members are a later element's.
func TestChildrenAfterTheWalkLeftPanics(t *testing.T) {
	v := validatorOver(t, orderSchema)
	w := newWalker(jsonURI, strings.NewReader(`{"order":{"size":1,"item":{"sku":"ABC"}}}`), v.Schema())
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	kids := root.Children()
	first, ok := kids.Next()
	if !ok {
		t.Fatalf("no first child: %v", kids.Err())
	}
	if _, ok := kids.Next(); !ok {
		t.Fatalf("no second child: %v", kids.Err())
	}
	size, _ := first.Element()
	defer func() {
		if recover() == nil {
			t.Fatal("Children on a left element did not panic")
		}
	}()
	size.Children()
}

// TestNilArguments pins the two caller faults.
func TestNilArguments(t *testing.T) {
	v := validatorOver(t, orderSchema)
	if _, err := Validate(nil, strings.NewReader(`{}`)); err == nil || !strings.HasPrefix(err.Error(), "jsonsrc: Validate: nil *validate.Validator") {
		t.Errorf("nil validator: %v", err)
	}
	if _, err := Validate(v, nil); err == nil || !strings.HasPrefix(err.Error(), "jsonsrc: Validate: nil io.Reader") {
		t.Errorf("nil reader: %v", err)
	}
}
