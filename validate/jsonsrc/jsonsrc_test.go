package jsonsrc

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsd"
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

// declinesSchema adds to order's shapes the two a key cannot be mapped
// through: two child declarations of one local name in two namespaces (an
// unqualified local a, and a reference to the top-level urn:t a), and a lax
// ##any wildcard, which fixes no namespace for a top-level w it would admit.
const declinesSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns:t="urn:t">
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="a" type="xs:string" minOccurs="0"/>
        <xs:element ref="t:a" minOccurs="0"/>
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
// at the construct it names and naming the arm it is not. Each decline's
// GAP(jsonsrc) marker, or its having no XML twin, is at the function that
// declines it (package doc, "Declines").
func TestOutOfSliceInputDeclines(t *testing.T) {
	v := validatorOver(t, declinesSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string // the error's opening, location included
		says       string // the arm or shape it names
	}{
		{"member matching no child declaration, under a ##any wildcard", `{"r":{"x":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "x" names no child element declaration and no attribute use of the type of {urn:t}r, and its element wildcard set includes one of variety any, which fixes no namespace for it`},
		{"member a ##any wildcard admits under a top-level declaration", `{"r":{"w":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "w" names no child element declaration and no attribute use of the type of {urn:t}r, and its element wildcard set includes one of variety any, which fixes no namespace for it`},
		{"reserved member", `{"r":{"$id":"x"}}`,
			"jsonsrc: instance.json:1:7: ", `reserved member "$id"; only the "$type" and "$xmlns" reserved members are mapped, and a member naming an attribute or a child is keyed by its own name, which takes no "$"`},
		{"reserved root member", `{"$xmlns":{}}`,
			"jsonsrc: instance.json:1:2: ", `root member "$xmlns" is a reserved member; the top-level object's one member names the validation root, and neither "$xmlns" nor "$type" is a member of the top-level object: move it into the root element's own object, which it leads, {"<root>":{"$xmlns":...}}`},
		{"reserved root member $type", `{"$type":"r"}`,
			"jsonsrc: instance.json:1:2: ", `root member "$type" is a reserved member; the top-level object's one member names the validation root, and neither "$xmlns" nor "$type" is a member of the top-level object: move it into the root element's own object, after any "$xmlns" and before every other member, {"<root>":{"$xmlns":...,"$type":...}}`},
		{"reserved root member, neither $xmlns nor $type", `{"$r":{}}`,
			"jsonsrc: instance.json:1:2: ", `root member "$r" is a reserved member; the top-level object's one member names the validation root, keyed by the root element's own name, which takes no "$"`},
		{"key of two expanded names", `{"r":{"a":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "a" names child element declarations of two expanded names in the type of {urn:t}r (a and {urn:t}a)`},
		{"nested array", `{"r":{"s":[["x"]]}}`,
			"jsonsrc: instance.json:1:12: ", `an array is an item of the array of member "s"`},
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

// TestRootNaming pins the top-level shapes that name no validation root, none
// of which has an XML twin (walker.root), and the shapes after the root's
// value, each with its class: a decline is an *Error of class Decline, and an
// absent or malformed document and content after the top-level object are not
// (#2710). How the root's key names it is TestPrefixedKey's and
// TestRootOfTwoExpandedNames'.
func TestRootNaming(t *testing.T) {
	v := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
		decline    bool // whether the error is an *Error of class Decline
	}{
		{"top level not an object", `["order"]`,
			"jsonsrc: instance.json:1:1: ", "the top-level value is not an object", true},
		{"top level empty", `{}`,
			"jsonsrc: instance.json:1:2: ", "the top-level object has no member", true},
		{"no top-level value", ``,
			"jsonsrc: instance.json:1:1: ", `the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {"<root>":...}`, false},
		{"root value an array", `{"order":[]}`,
			"jsonsrc: instance.json:1:10: ", `the value of root member "order" is an array; the validation root is one element, so give it an object, a scalar or null`, true},
		{"second root member", `{"order":{"size":1,"item":{"sku":"ABC"}},"order":{}}`,
			"jsonsrc: instance.json:1:42: ", `the top-level object has a second member "order"`, true},
		{"trailing value", `{"order":{"size":1,"item":{"sku":"ABC"}}} {}`,
			"jsonsrc: instance.json:1:43: ", "content follows the top-level object", false},
		{"malformed", `{"order":{"size":1,}}`,
			"jsonsrc: instance.json:1:", "malformed JSON", false},
		{"truncated", `{"order":{"size":1`,
			"jsonsrc: instance.json:1:", "malformed JSON", false},
		// The decoder reports this as io.ErrUnexpectedEOF, no
		// *json.SyntaxError: malformed all the same, as no read faulted.
		{"cut off mid-token", `{"order":"ab`,
			"jsonsrc: instance.json:1:", "malformed JSON", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, strings.NewReader(tc.json), WithURI(jsonURI))
			if err == nil {
				t.Fatalf("Validate(%s) = %v violations, nil error; want the decline", tc.json, rules(res))
			}
			if !strings.HasPrefix(err.Error(), tc.prefix) || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Validate(%s) = %q, want it to open %q and say %q", tc.json, err, tc.prefix, tc.says)
			}
			var e *Error
			if got := errors.As(err, &e) && e.Class() == Decline; got != tc.decline {
				t.Errorf("Validate(%s): a decline = %v, want %v", tc.json, got, tc.decline)
			}
		})
	}
}

// TestReadFault pins a fault reading the input reported as one, never as
// malformed JSON, still wrapping the reader's error, at the location the walk
// had reached (walker.token, #2655).
func TestReadFault(t *testing.T) {
	v := validatorOver(t, orderSchema)
	boom := errors.New("disk on fire")
	for _, tc := range []struct {
		name   string
		r      io.Reader
		prefix string
	}{
		{"first read", iotest.ErrReader(boom), "jsonsrc: instance.json:1:1: "},
		{"mid-document", io.MultiReader(strings.NewReader(`{"order":{"si`), iotest.ErrReader(boom)),
			"jsonsrc: instance.json:1:11: "},
		// The input goes on past the final LF read, so it opens line 2,
		// unlike a final LF the input ends with (TestEndOfInputLocation).
		{"after a line feed", io.MultiReader(strings.NewReader("{\"order\":{\"size\":1\n"), iotest.ErrReader(boom)),
			"jsonsrc: instance.json:2:1: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(v, tc.r, WithURI(jsonURI))
			if err == nil {
				t.Fatal("Validate returned no error; want the read fault")
			}
			if want := tc.prefix + "fault reading the input: disk on fire"; err.Error() != want {
				t.Errorf("Validate = %q, want %q", err, want)
			}
			if strings.Contains(err.Error(), "malformed JSON") {
				t.Errorf("Validate = %q; a read fault is no malformed JSON", err)
			}
			if !errors.Is(err, boom) {
				t.Errorf("Validate = %q; errors.Is does not reach the reader's error", err)
			}
		})
	}
}

// TestRootOfTwoExpandedNames pins an unprefixed root key declining when two
// top-level declarations share its local name in two namespaces, RULED
// permanent by #2623 (walker.rootMember).
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

// TestEndOfInputLocation pins the location of an error at the end of the
// input, as the package doc's "Locations" section places it, whatever sizes
// the reader returns (positions.start, #2723).
func TestEndOfInputLocation(t *testing.T) {
	v := validatorOver(t, orderSchema)
	const (
		truncated = `{"order":{"size":1` // 18 bytes
		eof       = ": malformed JSON: unexpected EOF"
		empty     = `: the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {"<root>":...}`
	)
	for _, tc := range []struct {
		name, doc, want string
	}{
		{"truncated", truncated, "1:19" + eof},
		{"truncated, then LF", truncated + "\n", "1:19" + eof},
		{"truncated, then CRLF", truncated + "\r\n", "1:20" + eof},
		{"truncated, then CR", truncated + "\r", "1:20" + eof},
		{"truncated, then LF LF", truncated + "\n\n", "2:1" + eof},
		{"truncated, then spaces", truncated + "  ", "1:21" + eof},
		{"truncated, then LF and spaces", truncated + "\n  ", "2:3" + eof},
		{"truncated on line 2, then LF", "{\n" + truncated[1:] + "\n", "2:18" + eof},
		{"columns in bytes", `{"order":{"size":"é"` + "\n", "1:22" + eof},
		{"empty", "", "1:1" + empty},
		{"a space", " ", "1:2" + empty},
		{"LF", "\n", "1:1" + empty},
		{"LF LF", "\n\n", "2:1" + empty},
		{"CRLF", "\r\n", "1:2" + empty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := "jsonsrc: instance.json:" + tc.want
			for _, r := range []io.Reader{strings.NewReader(tc.doc), iotest.OneByteReader(strings.NewReader(tc.doc))} {
				_, err := Validate(v, r, WithURI(jsonURI))
				if err == nil || err.Error() != want {
					t.Errorf("Validate(%q) = %v, want %q", tc.doc, err, want)
				}
			}
		})
	}
}

// TestDrainClassifies pins the walk's end: a subtree the assessment never
// descended into is still read, under the same classification, so a decline
// inside it is reported rather than read past.
func TestDrainClassifies(t *testing.T) {
	v := validatorOver(t, orderSchema)
	w := newWalker(jsonURI, strings.NewReader(`{"order":{"size":1,"item":[{"sku":"ABC"},{"sku":[["x"]]}]}}`), v.Schema(), v.ConditionallySelected, v.Governing)
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	err = w.end(root)
	const want = `jsonsrc: instance.json:1:50: an array is an item of the array of member "sku"`
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("end = %v, want it to open %q", err, want)
	}
}

// TestChildrenAfterTheWalkLeftPanics pins the late-descent guard: once the
// parent's cursor moved past an element, its members are a later element's.
func TestChildrenAfterTheWalkLeftPanics(t *testing.T) {
	v := validatorOver(t, orderSchema)
	w := newWalker(jsonURI, strings.NewReader(`{"order":{"size":1,"item":{"sku":"ABC"}}}`), v.Schema(), v.ConditionallySelected, v.Governing)
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

// TestError pins every located class Validate returns as an *Error that
// errors.As reaches (#2792): its Class, its Loc, and its Msg, the message
// alone, with no "jsonsrc: " and no location, under a URI carrying colons.
// Error() is "jsonsrc: " + Loc + ": " + Msg, a read fault still reaches the
// reader's error, and none is an *xsderr.Error. The nil-argument errors are no
// *Error. TestRootNaming pins the class of the top-level shapes.
func TestError(t *testing.T) {
	const uri = "dir:a/order:1.json"
	boom := errors.New("disk on fire")
	at := func(line, col int) xsderr.Loc { return xsderr.Loc{URI: uri, Line: line, Col: col} }
	orders := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name  string
		v     *validate.Validator
		r     io.Reader
		class Class
		loc   xsderr.Loc
		msg   string
	}{
		{"malformed JSON", orders, strings.NewReader(`{"order":{"size":1,}}`), Malformed, at(1, 20),
			"malformed JSON: invalid character '}' looking for beginning of object key string"},
		{"malformed JSON at the end of the input", orders, strings.NewReader(`{"order":{"size":1`), Malformed, at(1, 19),
			"malformed JSON: unexpected EOF"},
		{"no JSON value", orders, strings.NewReader(``), Malformed, at(1, 1),
			`the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {"<root>":...}`},
		{"content follows", orders, strings.NewReader(`{"order":null}{}`), Malformed, at(1, 15),
			"content follows the top-level object"},
		// Bytes the decoder rejects before it reads on to the reader's error:
		// the decoder's error is not r's, so the class is Malformed.
		{"malformed bytes, then a fault reading the input", orders,
			io.MultiReader(strings.NewReader(`{"order",`), iotest.ErrReader(boom)), Malformed, at(1, 10),
			"malformed JSON: invalid character ',' after object key"},
		{"malformed bytes in the read that faults", orders, &bytesThenErr{b: []byte(`{"order",`), err: boom}, Malformed, at(1, 10),
			"malformed JSON: invalid character ',' after object key"},
		{"read fault", orders, iotest.ErrReader(boom), ReadFault, at(1, 1),
			"fault reading the input: disk on fire"},
		{"read fault mid-document", orders, io.MultiReader(strings.NewReader(`{"order":{"si`), iotest.ErrReader(boom)), ReadFault, at(1, 11),
			"fault reading the input: disk on fire"},
		{"decline", orders, strings.NewReader(`{"z:nosuch":{}}`), Decline, at(1, 2),
			`member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`},
		{"decline after a child", validatorOver(t, attributeDeclinesSchema), strings.NewReader(`{"r":{"price":1,"a":"x"}}`), Decline, at(1, 17),
			`attribute member "a" follows a child member of {urn:t}r; move attribute members ahead of the first child member, or declare a struct's attribute fields before its child fields, as an element's attributes are read before its children`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(tc.v, tc.r, WithURI(uri))
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("Validate = %v; errors.As does not reach an *Error", err)
			}
			if e.Class() != tc.class {
				t.Errorf("Class() = %d, want %d", e.Class(), tc.class)
			}
			if e.Loc() != tc.loc {
				t.Errorf("Loc() = %v, want %v", e.Loc(), tc.loc)
			}
			if e.Msg() != tc.msg {
				t.Errorf("Msg() = %q, want %q", e.Msg(), tc.msg)
			}
			if want := "jsonsrc: " + tc.loc.String() + ": " + tc.msg; err.Error() != want {
				t.Errorf("Error() = %q, want %q", err, want)
			}
			if tc.class == ReadFault && !errors.Is(err, boom) {
				t.Errorf("Validate = %q; errors.Is does not reach the reader's error", err)
			}
			var x *xsderr.Error
			if errors.As(err, &x) {
				t.Errorf("Validate = %v is an *xsderr.Error; no jsonsrc error is a validity verdict", err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		v    *validate.Validator
		r    io.Reader
	}{
		{"nil validator", nil, strings.NewReader(`{}`)},
		{"nil reader", orders, nil},
	} {
		_, err := Validate(tc.v, tc.r, WithURI(uri))
		var e *Error
		if err == nil || errors.As(err, &e) {
			t.Errorf("%s: Validate = %v, want an error that is no *Error", tc.name, err)
		}
	}
}

// bytesThenErr returns b, and err from the Read that returns b's last byte and
// every Read after it.
type bytesThenErr struct {
	b   []byte
	err error
}

func (r *bytesThenErr) Read(p []byte) (int, error) {
	n := copy(p, r.b)
	r.b = r.b[n:]
	if len(r.b) > 0 {
		return n, nil
	}
	return n, r.err
}

// TestErrorWraps pins the error classes Validate's doc states as wrapping
// nothing: a nil argument, the two malformed-JSON spellings with no decoder
// error behind them, and a decline. Each row's message is pinned too, so a row
// reaching another class fails rather than passes. TestReadFault pins the one
// class that wraps, and no row asserts what a "malformed JSON: …" error wraps.
func TestErrorWraps(t *testing.T) {
	v := validatorOver(t, orderSchema)
	for _, tc := range []struct {
		name string
		v    *validate.Validator
		r    io.Reader
		says string
	}{
		{"nil validator", nil, strings.NewReader(`{}`), "jsonsrc: Validate: nil *validate.Validator"},
		{"nil reader", v, nil, "jsonsrc: Validate: nil io.Reader"},
		{"no JSON value", v, strings.NewReader(``), "the document holds no JSON value"},
		{"content after the top-level object", v, strings.NewReader(`{"order":null}{}`),
			"content follows the top-level object"},
		{"decline", v, strings.NewReader(`[]`), "the top-level value is not an object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(tc.v, tc.r, WithURI(jsonURI))
			if err == nil {
				t.Fatal("Validate returned no error")
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Validate = %q, want it to say %q", err, tc.says)
			}
			if inner := errors.Unwrap(err); inner != nil {
				t.Errorf("Validate = %q wraps %v; this class wraps nothing", err, inner)
			}
		})
	}
}

// attributeSchema declares one root, shipment, whose element-only type holds a
// required unqualified id (xs:NCName), an optional priority (xs:integer), an
// optional flag (xs:boolean), a reference to the qualified top-level attribute
// code, and one child, dest. A key's namespace comes from the attribute use it
// matches, so id and code are spelled alike in JSON and differently in XML.
const attributeSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:element name="shipment">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="dest" type="xs:string"/>
      </xs:sequence>
      <xs:attribute name="id" type="xs:NCName" use="required"/>
      <xs:attribute name="priority" type="xs:integer"/>
      <xs:attribute name="flag" type="xs:boolean"/>
      <xs:attribute ref="t:code"/>
    </xs:complexType>
  </xs:element>
  <xs:attribute name="code" type="xs:string"/>
</xs:schema>`

var attr = []xsderr.Rule{"cvc-attribute"}

// TestAttributeMember pins the attribute arm: a member whose key is the local
// name of an attribute use of the enclosing type is that attribute, under the
// use's expanded name, its value the scalar's lexical form. The empty string is
// an attribute with an empty value, not an absent one: "id":"" fails xs:NCName
// under cvc-attribute where an omitted id is cvc-complex-type clause 3's.
func TestAttributeMember(t *testing.T) {
	v := validatorOver(t, attributeSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"valid", `{"shipment":{"id":"s1","priority":2,"dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" id="s1" priority="2"><dest>Oslo</dest></shipment>`, none},
		{"qualified use", `{"shipment":{"id":"s1","code":"c","dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" xmlns:t="urn:t" id="s1" t:code="c"><dest>Oslo</dest></shipment>`, none},
		{"boolean", `{"shipment":{"id":"s1","flag":true,"dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" id="s1" flag="true"><dest>Oslo</dest></shipment>`, none},
		{"required use absent", `{"shipment":{"priority":2,"dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" priority="2"><dest>Oslo</dest></shipment>`, complexType},
		{"value not of its type", `{"shipment":{"id":"s1","priority":"high","dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" id="s1" priority="high"><dest>Oslo</dest></shipment>`, attr},
		{"empty string is an empty value", `{"shipment":{"id":"","dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" id=""><dest>Oslo</dest></shipment>`, attr},
		{"escaped tab is kept", `{"shipment":{"id":"a\tb","dest":"Oslo"}}`,
			`<shipment xmlns="urn:t" id="a&#9;b"><dest>Oslo</dest></shipment>`, attr},
		{"attributes alone", `{"shipment":{"id":"s1"}}`,
			`<shipment xmlns="urn:t" id="s1"/>`, content},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestAttributesReadWhole pins what the engine reads off an element's
// attributes: each under its use's expanded name, its value unnormalized — an
// escaped tab, LF and CR kept as a character reference would be — at its key's
// Loc, and the same before and after the element's children are read.
func TestAttributesReadWhole(t *testing.T) {
	v := validatorOver(t, attributeSchema)
	w := newWalker(jsonURI, strings.NewReader("{\"shipment\":{\"id\":\"s1\",\n \"code\":\"a\\tb\\nc\\rd\",\"dest\":\"Oslo\"}}"), v.Schema(), v.ConditionallySelected, v.Governing)
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	type seen struct {
		name  xsd.QName
		value string
		loc   string
	}
	read := func() []seen {
		var out []seen
		for _, a := range root.Attributes() {
			out = append(out, seen{a.Name(), a.Value(), a.Loc().String()})
		}
		return out
	}
	want := []seen{
		{xsd.QName{Local: "id"}, "s1", "instance.json:1:14"},
		{xsd.QName{Space: "urn:t", Local: "code"}, "a\tb\nc\rd", "instance.json:2:2"},
	}
	if got := read(); !slices.Equal(got, want) {
		t.Fatalf("Attributes() at entry = %v, want %v", got, want)
	}
	if err := w.end(root); err != nil {
		t.Fatalf("end: %v", err)
	}
	if got := read(); !slices.Equal(got, want) {
		t.Fatalf("Attributes() after the children = %v, want %v", got, want)
	}
}

// attributeDeclinesSchema holds the keys the attribute arm declines: n,
// declared both as a child and as an attribute; k, the local name of an
// unqualified use and of a reference to the top-level {urn:t}k; w, which r's
// ##any {attribute wildcard} alone admits and fixes no namespace for;
// price's currency, a use of a type with simple content; x's reference to
// xsi:type; and s's references to xsi:schemaLocation and
// xsi:noNamespaceSchemaLocation. a is a plain use.
const attributeDeclinesSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    targetNamespace="urn:t" xmlns:t="urn:t" elementFormDefault="qualified">
  <xs:import namespace="http://www.w3.org/2001/XMLSchema-instance"/>
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="n" type="xs:string" minOccurs="0"/>
        <xs:element name="price" minOccurs="0">
          <xs:complexType>
            <xs:simpleContent>
              <xs:extension base="xs:decimal">
                <xs:attribute name="currency" type="xs:string"/>
              </xs:extension>
            </xs:simpleContent>
          </xs:complexType>
        </xs:element>
        <xs:element name="x" minOccurs="0">
          <xs:complexType>
            <xs:attribute ref="xsi:type"/>
          </xs:complexType>
        </xs:element>
        <xs:element name="s" minOccurs="0">
          <xs:complexType>
            <xs:attribute ref="xsi:schemaLocation"/>
            <xs:attribute ref="xsi:noNamespaceSchemaLocation"/>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
      <xs:attribute name="n" type="xs:string"/>
      <xs:attribute name="k" type="xs:string"/>
      <xs:attribute ref="t:k"/>
      <xs:attribute name="a" type="xs:string"/>
      <xs:anyAttribute processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:attribute name="k" type="xs:string"/>
  <xs:attribute name="w" type="xs:string"/>
</xs:schema>`

// TestAttributeMemberDeclines pins every attribute-shaped member the arm does
// not map: each is (nil, err), err opening with "jsonsrc: <uri>:<line>:<col>: "
// at the construct it names. Each decline's GAP(jsonsrc) marker, or its having
// no XML twin, is at the function that declines it (package doc, "Declines").
func TestAttributeMemberDeclines(t *testing.T) {
	v := validatorOver(t, attributeDeclinesSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"attribute after a child", `{"r":{"price":1,"a":"x"}}`,
			"jsonsrc: instance.json:1:17: ", `attribute member "a" follows a child member of {urn:t}r; move attribute members ahead of the first child member, or declare a struct's attribute fields before its child fields`},
		{"repeated attribute", `{"r":{"a":"x","a":"y"}}`,
			"jsonsrc: instance.json:1:15: ", `attribute member "a" repeats in the object of {urn:t}r; an element carries one attribute of each expanded name, so give it once`},
		{"null value", `{"r":{"a":null}}`,
			"jsonsrc: instance.json:1:11: ", `attribute member "a" of {urn:t}r is not a string, number or boolean`},
		{"object value", `{"r":{"a":{}}}`,
			"jsonsrc: instance.json:1:11: ", `attribute member "a" of {urn:t}r is not a string, number or boolean`},
		{"array value", `{"r":{"a":["x"]}}`,
			"jsonsrc: instance.json:1:11: ", `attribute member "a" of {urn:t}r is not a string, number or boolean`},
		{"both a child and an attribute", `{"r":{"n":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "n" names both the child element declaration {urn:t}n and the attribute use n of the type of {urn:t}r`},
		{"uses of two expanded names", `{"r":{"k":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "k" names attribute uses of two expanded names in the type of {urn:t}r (k and {urn:t}k)`},
		{"a key the ##any attribute wildcard alone admits", `{"r":{"w":"1"}}`,
			"jsonsrc: instance.json:1:7: ", `member "w" names no child element declaration and no attribute use of the type of {urn:t}r, and its {attribute wildcard} set includes one of variety any, which fixes no namespace for it`},
		{"a use of a simple-content type", `{"r":{"price":{"currency":"EUR"}}}`,
			"jsonsrc: instance.json:1:16: ", `member "currency" names an attribute use of the type of {urn:t}price, whose {content type} is simple`},
		{"a use in the xsi namespace", `{"r":{"x":{"type":"t:r"}}}`,
			"jsonsrc: instance.json:1:12: ", `member "type" names the attribute use {http://www.w3.org/2001/XMLSchema-instance}type of the type of {urn:t}x; an xsi: attribute is presented only by`},
		{"an xsi:schemaLocation use", `{"r":{"s":{"schemaLocation":"urn:a a.xsd"}}}`,
			"jsonsrc: instance.json:1:12: ", `member "schemaLocation" names the attribute use {http://www.w3.org/2001/XMLSchema-instance}schemaLocation of the type of {urn:t}s; an xsi: attribute is presented only by`},
		{"an xsi:noNamespaceSchemaLocation use", `{"r":{"s":{"noNamespaceSchemaLocation":"a.xsd"}}}`,
			"jsonsrc: instance.json:1:12: ", `member "noNamespaceSchemaLocation" names the attribute use {http://www.w3.org/2001/XMLSchema-instance}noNamespaceSchemaLocation of the type of {urn:t}s; an xsi: attribute is presented only by`},
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

// nilSchema declares one nillable root, order, whose sequence of optional
// children holds every shape cvc-elt clause 3 tells apart: qty, a nillable
// xs:integer; size, an xs:integer that is not nillable; unit, a nillable
// xs:integer with a fixed {value constraint}; line, a repeating nillable
// xs:integer; and box, a nillable complex type requiring one child w.
const nilSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:t" xmlns="urn:t" elementFormDefault="qualified">
  <xs:element name="order" nillable="true">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="qty" type="xs:integer" nillable="true" minOccurs="0"/>
        <xs:element name="size" type="xs:integer" minOccurs="0"/>
        <xs:element name="unit" type="xs:integer" nillable="true" fixed="1" minOccurs="0"/>
        <xs:element name="line" type="xs:integer" nillable="true" minOccurs="0" maxOccurs="unbounded"/>
        <xs:element name="box" nillable="true" minOccurs="0">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="w" type="xs:integer"/>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// xsiDecl binds the xsi prefix in an XML twin.
const xsiDecl = `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`

var (
	elt = []xsderr.Rule{"cvc-elt"}
	// notNillable is cvc-elt clause 3.1, then cvc-type clause 3.1.3: an
	// element whose declaration is not nillable is not ·nilled·, so its empty
	// content is an xs:integer's to read.
	notNillable = []xsderr.Rule{"cvc-elt", "cvc-type"}
)

// TestNullIsNilled pins the null arm: a null value is an element carrying
// xsi:nil="true" and no [[children]] — at the root, as a member and as an
// array item, {"qty":null} ≡ <qty xsi:nil="true"/> — and cvc-elt clause 3 is
// the engine's: clause 3.1 charges a null where {nillable} is false, and clause
// 3.2.3.2 one under a fixed {value constraint}, which an empty, un-nilled
// element would take as its value instead (clause 5.1). An object value is
// never nilled: box given {} is assessed as empty content, which
// cvc-complex-type clause 1.4 charges.
func TestNullIsNilled(t *testing.T) {
	v := validatorOver(t, nilSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"nillable member", `{"order":{"qty":null}}`,
			`<order xmlns="urn:t" ` + xsiDecl + `><qty xsi:nil="true"/></order>`, none},
		{"root", `{"order":null}`,
			`<order xmlns="urn:t" ` + xsiDecl + ` xsi:nil="true"/>`, none},
		{"array item", `{"order":{"line":[1,null,3]}}`,
			`<order xmlns="urn:t" ` + xsiDecl + `><line>1</line><line xsi:nil="true"/><line>3</line></order>`, none},
		{"nillable complex member", `{"order":{"box":null}}`,
			`<order xmlns="urn:t" ` + xsiDecl + `><box xsi:nil="true"/></order>`, none},
		{"not nillable", `{"order":{"size":null}}`,
			`<order xmlns="urn:t" ` + xsiDecl + `><size xsi:nil="true"/></order>`, notNillable},
		{"not nillable array item", `{"order":{"size":[null]}}`,
			`<order xmlns="urn:t" ` + xsiDecl + `><size xsi:nil="true"/></order>`, notNillable},
		{"fixed", `{"order":{"unit":null}}`,
			`<order xmlns="urn:t" ` + xsiDecl + `><unit xsi:nil="true"/></order>`, elt},
		{"empty object is not nilled", `{"order":{"box":{}}}`,
			`<order xmlns="urn:t"><box/></order>`, content},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestNilledAttribute pins what the engine reads off a null value's element:
// one attribute, xsi:nil in the XSI namespace with the lexical form true,
// located at the null token and not at the key, the same before and after the
// walk ends; and no attribute at all on a scalar item's element or on an
// element given {}.
func TestNilledAttribute(t *testing.T) {
	v := validatorOver(t, nilSchema)
	type seen struct {
		name  xsd.QName
		value string
		loc   string
	}
	read := func(e validate.Element) []seen {
		var out []seen
		for _, a := range e.Attributes() {
			out = append(out, seen{a.Name(), a.Value(), a.Loc().String()})
		}
		return out
	}
	nilAt := func(loc string) []seen {
		return []seen{{xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, "true", loc}}
	}

	w := newWalker(jsonURI, strings.NewReader("{\"order\":\n null}"), v.Schema(), v.ConditionallySelected, v.Governing)
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if got, want := read(root), nilAt("instance.json:2:2"); !slices.Equal(got, want) {
		t.Fatalf("root Attributes() = %v, want %v", got, want)
	}
	if err := w.end(root); err != nil {
		t.Fatalf("end: %v", err)
	}
	if got, want := read(root), nilAt("instance.json:2:2"); !slices.Equal(got, want) {
		t.Fatalf("root Attributes() after the walk = %v, want %v", got, want)
	}

	w = newWalker(jsonURI, strings.NewReader(`{"order":{"line":[1, null],"box":{}}}`), v.Schema(), v.ConditionallySelected, v.Governing)
	root, err = w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	kids := root.Children()
	var got [][]seen
	for {
		c, ok := kids.Next()
		if !ok {
			break
		}
		e, _ := c.Element()
		got = append(got, read(e))
	}
	if err := kids.Err(); err != nil {
		t.Fatalf("children: %v", err)
	}
	want := [][]seen{nil, nilAt("instance.json:1:22"), nil}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("children's Attributes() = %v, want %v", got, want)
	}
}

// wildcardSchema declares, in urn:w, the top-level element w and attribute p,
// both xs:integer, and one root per shape the wildcard arm meets: box, a
// strict ##targetNamespace element wildcard; bag, a lax ##local one; oc, a
// strict ##targetNamespace {open content}; att, a lax ##targetNamespace
// {attribute wildcard} over element-only content; pre and preatt, the strict
// element and lax attribute wildcards beside an unqualified local w and use p
// of xs:string, which name the key instead; preel, a strict element wildcard
// beside an unqualified use w of xs:string, which names the key instead; ref
// and def, whose wildcards reject p and w by name; and the shapes the arm
// declines — mix, a skip urn:x wildcard beside box's; ocmix, a ##local {open
// content} beside it; not and attnot, an element and an {attribute wildcard}
// of variety not; two and atttwo, an element and an {attribute wildcard}
// enumerating two namespaces; any, of xs:anyType; both, with a
// ##targetNamespace element wildcard and {attribute wildcard}; xsi and lang,
// an {attribute wildcard} of the xsi or the xml namespace alone, and xsiel and
// langel, an element wildcard of the one or the other; price, simple
// content under an {attribute wildcard}; and tt, a top-level declaration with
// a {type table} box's wildcard admits.
const wildcardSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:w" xmlns="urn:w" xmlns:w="urn:w" elementFormDefault="qualified">
  <xs:element name="box">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##targetNamespace" processContents="strict" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="bag">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##local" processContents="lax" minOccurs="0" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="oc">
    <xs:complexType>
      <xs:openContent mode="suffix">
        <xs:any namespace="##targetNamespace" processContents="strict"/>
      </xs:openContent>
      <xs:sequence>
        <xs:element name="label" type="xs:string" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="att">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="text" type="xs:string" minOccurs="0"/>
      </xs:sequence>
      <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pre">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="w" form="unqualified" type="xs:string" minOccurs="0"/>
        <xs:any namespace="##targetNamespace" processContents="strict" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="preatt">
    <xs:complexType>
      <xs:attribute name="p" type="xs:string"/>
      <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="ref">
    <xs:complexType>
      <xs:anyAttribute namespace="##targetNamespace" notQName="w:p" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="def">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##targetNamespace" notQName="##defined" processContents="lax" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="mix">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##targetNamespace" processContents="strict" minOccurs="0"/>
        <xs:any namespace="urn:x" processContents="skip" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="ocmix">
    <xs:complexType>
      <xs:openContent mode="interleave">
        <xs:any namespace="##local" processContents="lax"/>
      </xs:openContent>
      <xs:sequence>
        <xs:any namespace="##targetNamespace" processContents="strict" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="not">
    <xs:complexType>
      <xs:sequence>
        <xs:any notNamespace="urn:x" processContents="lax" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="two">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##targetNamespace urn:x" processContents="lax" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="attnot">
    <xs:complexType>
      <xs:anyAttribute notNamespace="urn:x" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="atttwo">
    <xs:complexType>
      <xs:anyAttribute namespace="##targetNamespace urn:x" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="preel">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##targetNamespace" processContents="strict" minOccurs="0"/>
      </xs:sequence>
      <xs:attribute name="w" type="xs:string"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="any"/>
  <xs:element name="both">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="##targetNamespace" processContents="lax" minOccurs="0"/>
      </xs:sequence>
      <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="xsi">
    <xs:complexType>
      <xs:anyAttribute namespace="http://www.w3.org/2001/XMLSchema-instance" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="lang">
    <xs:complexType>
      <xs:anyAttribute namespace="http://www.w3.org/XML/1998/namespace" processContents="lax"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="xsiel">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="http://www.w3.org/2001/XMLSchema-instance" processContents="lax" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="langel">
    <xs:complexType>
      <xs:sequence>
        <xs:any namespace="http://www.w3.org/XML/1998/namespace" processContents="lax" minOccurs="0"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="price">
    <xs:complexType>
      <xs:simpleContent>
        <xs:extension base="xs:decimal">
          <xs:anyAttribute namespace="##targetNamespace" processContents="lax"/>
        </xs:extension>
      </xs:simpleContent>
    </xs:complexType>
  </xs:element>
  <xs:element name="w" type="xs:integer"/>
  <xs:element name="tt" type="xs:anyType">
    <xs:alternative test="@k" type="xs:string"/>
  </xs:element>
  <xs:attribute name="p" type="xs:integer"/>
</xs:schema>`

var assessElt = []xsderr.Rule{"cvc-assess-elt"}

// TestWildcardMember pins the wildcard arm: a key no contained declaration and
// no attribute use carries is the child or attribute the one namespace its
// type's wildcards enumerate gives it, and the engine assesses that name as it
// assesses the XML twin, admission and {process contents} included.
func TestWildcardMember(t *testing.T) {
	v := validatorOver(t, wildcardSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"strict element, declared", `{"box":{"w":5}}`,
			`<box xmlns="urn:w"><w>5</w></box>`, none},
		{"strict element, not of its type", `{"box":{"w":"x"}}`,
			`<box xmlns="urn:w"><w>x</w></box>`, typ},
		{"strict element, undeclared", `{"box":{"u":{}}}`,
			`<box xmlns="urn:w"><u/></box>`, assessElt},
		{"lax ##local element, undeclared", `{"bag":{"q":"any","q":null}}`,
			`<bag xmlns="urn:w" ` + xsiDecl + `><q xmlns="">any</q><q xmlns="" xsi:nil="true"/></bag>`, none},
		{"lax ##local element, not the declared w", `{"bag":{"w":"x"}}`,
			`<bag xmlns="urn:w"><w xmlns="">x</w></bag>`, none},
		{"open content element", `{"oc":{"label":"l","w":"x"}}`,
			`<oc xmlns="urn:w"><label>l</label><w>x</w></oc>`, typ},
		{"lax attribute, declared", `{"att":{"p":5,"text":"t"}}`,
			`<att xmlns="urn:w" xmlns:w="urn:w" w:p="5"><text>t</text></att>`, none},
		{"lax attribute, not of its type", `{"att":{"p":"x"}}`,
			`<att xmlns="urn:w" xmlns:w="urn:w" w:p="x"/>`, attr},
		{"lax attribute, undeclared", `{"att":{"q":"x"}}`,
			`<att xmlns="urn:w" xmlns:w="urn:w" w:q="x"/>`, none},
		{"attribute the wildcard rejects by name", `{"ref":{"p":5}}`,
			`<ref xmlns="urn:w" xmlns:w="urn:w" w:p="5"/>`, complexType},
		{"element the wildcard rejects as defined", `{"def":{"w":5}}`,
			`<def xmlns="urn:w"><w>5</w></def>`, content},
		{"strict element, a top-level declaration with a type table", `{"box":{"tt":"1"}}`,
			`<box xmlns="urn:w"><tt>1</tt></box>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// strayChildSchema declares, in urn:n, the root r, whose types carry no
// wildcard of either kind: r's element-only content holds s, of xs:integer,
// e, of empty content, m, of simple content, and u, an unqualified local of
// element-only content; g, of xs:integer, t, holding y, and tt, carrying a
// {type table}, are top-level declarations no type contains.
const strayChildSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    targetNamespace="urn:n" xmlns="urn:n" elementFormDefault="qualified">
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="s" type="xs:integer" minOccurs="0"/>
        <xs:element name="e" minOccurs="0">
          <xs:complexType/>
        </xs:element>
        <xs:element name="m" minOccurs="0">
          <xs:complexType>
            <xs:simpleContent>
              <xs:extension base="xs:decimal"/>
            </xs:simpleContent>
          </xs:complexType>
        </xs:element>
        <xs:element name="u" form="unqualified" minOccurs="0">
          <xs:complexType>
            <xs:sequence>
              <xs:element name="s" form="unqualified" type="xs:integer" minOccurs="0"/>
            </xs:sequence>
          </xs:complexType>
        </xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="g" type="xs:integer"/>
  <xs:element name="t">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="y" type="xs:integer"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="tt" type="xs:anyType">
    <xs:alternative test="@k" type="xs:string"/>
  </xs:element>
</xs:schema>`

// TestStrayChildMember pins the no-wildcard arm: a key no contained
// declaration and no attribute use of a type with no wildcard of either kind
// carries is the child element in the owner's own namespace, and the engine
// charges it as it charges the XML twin — cvc-complex-content under
// element-only content, cvc-type clause 3.1.2 under a simple type, and
// cvc-complex-type clauses 1.1 and 1.2 under empty and simple content.
func TestStrayChildMember(t *testing.T) {
	v := validatorOver(t, strayChildSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"element-only content", `{"r":{"s":1,"x":"v"}}`,
			`<r xmlns="urn:n"><s>1</s><x>v</x></r>`, content},
		{"element-only content, a top-level declaration's name", `{"r":{"g":"five"}}`,
			`<r xmlns="urn:n"><g>five</g></r>`, []xsderr.Rule{"cvc-complex-content", "cvc-type"}},
		{"element-only content, a top-level declaration's object", `{"r":{"t":{"y":"five"}}}`,
			`<r xmlns="urn:n"><t><y>five</y></t></r>`, []xsderr.Rule{"cvc-complex-content", "cvc-type"}},
		{"null value", `{"r":{"x":null}}`,
			`<r xmlns="urn:n" ` + xsiDecl + `><x xsi:nil="true"/></r>`, content},
		{"empty object value", `{"r":{"x":{}}}`,
			`<r xmlns="urn:n"><x/></r>`, content},
		{"simple type", `{"r":{"s":{"x":1}}}`,
			`<r xmlns="urn:n"><s><x>1</x></s></r>`, typ},
		{"empty content", `{"r":{"e":{"x":1}}}`,
			`<r xmlns="urn:n"><e><x>1</x></e></r>`, complexType},
		{"simple content", `{"r":{"m":{"x":1}}}`,
			`<r xmlns="urn:n"><m><x>1</x></m></r>`, complexType},
		{"unqualified owner", `{"r":{"u":{"x":1}}}`,
			`<r xmlns="urn:n"><u xmlns=""><x>1</x></u></r>`, content},
		{"element-only content, a top-level declaration with a type table", `{"r":{"tt":"1"}}`,
			`<r xmlns="urn:n"><tt>1</tt></r>`, content},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestStrayChildDeclines pins the no-wildcard arm's decline: a stray child no
// top-level declaration carries the name of is a bareElement, whose object
// value maps only empty, so a member of it declines, as a wildcard's
// undeclared child's does (RULED permanent by #2634).
func TestStrayChildDeclines(t *testing.T) {
	v := validatorOver(t, strayChildSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"member of an undeclared stray child", `{"r":{"x":{"y":1}}}`,
			"jsonsrc: instance.json:1:12: ", `member "y" of {urn:n}x, an element no contained and no top-level element declaration names; only an empty object maps under it`},
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

// TestStrayChildName pins the expanded name the no-wildcard arm gives a key:
// the owner's own namespace name, urn:n under r and none under the
// unqualified u, whatever namespace the schema's target is.
func TestStrayChildName(t *testing.T) {
	v := validatorOver(t, strayChildSchema)
	w := newWalker(jsonURI, strings.NewReader(`{"r":{"x":1,"u":{"y":1}}}`), v.Schema(), v.ConditionallySelected, v.Governing)
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
		e, ok := c.Element()
		if !ok {
			continue
		}
		got = append(got, e.Name())
		grand := e.Children()
		for {
			g, ok := grand.Next()
			if !ok {
				break
			}
			if ge, ok := g.Element(); ok {
				got = append(got, ge.Name())
			}
		}
		if err := grand.Err(); err != nil {
			t.Fatalf("grandchildren: %v", err)
		}
	}
	if err := kids.Err(); err != nil {
		t.Fatalf("children: %v", err)
	}
	want := []xsd.QName{{Space: "urn:n", Local: "x"}, {Local: "u"}, {Local: "y"}}
	if !slices.Equal(got, want) {
		t.Fatalf("children's names = %v, want %v", got, want)
	}
}

// TestDeclarationNamesKey pins the mapping's own rule that a key a contained
// declaration or an attribute use carries is named by it and never by a
// wildcard of the type (package doc, "The mapping"), RULED permanent by #2634.
// The first two rows' declaration or use is the unqualified one of xs:string,
// where the wildcard of its own kind would name urn:w's xs:integer: two
// expanded names, between which no spec Note chooses, and "x" is valid only
// under the declaration's or use's. The last two cross kinds: att's text, an
// element declaration, beside an {attribute wildcard}, where an attribute
// would decline its object value; and preel's use w beside a strict element
// wildcard, where the child {urn:w}w of xs:integer would charge cvc-type.
func TestDeclarationNamesKey(t *testing.T) {
	v := validatorOver(t, wildcardSchema)
	for _, tc := range []struct {
		name, json, xml string
		want            []xsderr.Rule
	}{
		{"element", `{"pre":{"w":"x"}}`, `<pre xmlns="urn:w"><w xmlns="">x</w></pre>`, none},
		{"attribute", `{"preatt":{"p":"x"}}`, `<preatt xmlns="urn:w" p="x"/>`, none},
		{"element over an attribute wildcard", `{"att":{"text":{}}}`, `<att xmlns="urn:w"><text/></att>`, none},
		{"attribute over an element wildcard", `{"preel":{"w":"x"}}`, `<preel xmlns="urn:w" w="x"/>`, none},
	} {
		t.Run(tc.name, func(t *testing.T) { twin(t, v, tc.json, tc.xml, tc.want) })
	}
}

// TestWildcardMemberDeclines pins every wildcard-only key the arm does not
// name: each is (nil, err), err opening with "jsonsrc: <uri>:<line>:<col>: "
// at the key and naming what declines it. The two-namespace, variety-not,
// both-kinds and undeclared-wildcard-child rows are wildcardMember's declines
// RULED permanent by #2634, whose variety-any rows are
// TestOutOfSliceInputDeclines' and TestAttributeMemberDeclines'; the four
// xsi- and xml-namespace rows, both kinds of each, are its declines RULED
// permanent by #2635.
func TestWildcardMemberDeclines(t *testing.T) {
	v := validatorOver(t, wildcardSchema)
	for _, tc := range []struct {
		name, json string
		prefix     string
		says       string
	}{
		{"element wildcards of two namespaces, one skip", `{"mix":{"w":5}}`,
			"jsonsrc: instance.json:1:9: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}mix, and its element wildcard set enumerates 2 namespaces`},
		{"element wildcard and open content of two namespaces", `{"ocmix":{"w":5}}`,
			"jsonsrc: instance.json:1:11: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}ocmix, and its element wildcard set enumerates 2 namespaces`},
		{"element wildcard of variety not", `{"not":{"w":5}}`,
			"jsonsrc: instance.json:1:9: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}not, and its element wildcard set includes one of variety not, which fixes no namespace for it`},
		{"element wildcard of two namespaces", `{"two":{"w":5}}`,
			"jsonsrc: instance.json:1:9: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}two, and its element wildcard set enumerates 2 namespaces`},
		{"attribute wildcard of variety not", `{"attnot":{"w":5}}`,
			"jsonsrc: instance.json:1:12: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}attnot, and its {attribute wildcard} set includes one of variety not, which fixes no namespace for it`},
		{"attribute wildcard of two namespaces", `{"atttwo":{"w":5}}`,
			"jsonsrc: instance.json:1:12: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}atttwo, and its {attribute wildcard} set enumerates 2 namespaces`},
		{"both kinds of wildcard, xs:anyType", `{"any":{"w":5}}`,
			"jsonsrc: instance.json:1:9: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}any, which has both an element wildcard and an {attribute wildcard}`},
		{"both kinds of wildcard, one namespace each", `{"both":{"w":5}}`,
			"jsonsrc: instance.json:1:10: ", `member "w" names no child element declaration and no attribute use of the type of {urn:w}both, which has both an element wildcard and an {attribute wildcard}`},
		{"the xsi namespace", `{"xsi":{"type":"w:box"}}`,
			"jsonsrc: instance.json:1:9: ", `and its {attribute wildcard} set enumerates the namespace http://www.w3.org/2001/XMLSchema-instance alone`},
		{"the xml namespace", `{"lang":{"lang":"en"}}`,
			"jsonsrc: instance.json:1:10: ", `and its {attribute wildcard} set enumerates the namespace http://www.w3.org/XML/1998/namespace alone`},
		{"the xsi namespace, element kind", `{"xsiel":{"type":"w:box"}}`,
			"jsonsrc: instance.json:1:11: ", `member "type" names no child element declaration and no attribute use of the type of {urn:w}xsiel, and its element wildcard set enumerates the namespace http://www.w3.org/2001/XMLSchema-instance alone`},
		{"the xml namespace, element kind", `{"langel":{"lang":"en"}}`,
			"jsonsrc: instance.json:1:12: ", `member "lang" names no child element declaration and no attribute use of the type of {urn:w}langel, and its element wildcard set enumerates the namespace http://www.w3.org/XML/1998/namespace alone`},
		{"simple content", `{"price":{"p":5}}`,
			"jsonsrc: instance.json:1:11: ", `member "p" names only the {attribute wildcard} of the type of {urn:w}price, whose {content type} is simple`},
		{"member of an undeclared wildcard child", `{"box":{"u":{"w":5}}}`,
			"jsonsrc: instance.json:1:14: ", `member "w" of {urn:w}u, an element no contained and no top-level element declaration names; only an empty object maps under it`},
		{"wildcard attribute after a child", `{"att":{"text":"t","p":5}}`,
			"jsonsrc: instance.json:1:20: ", `attribute member "p" follows a child member of {urn:w}att; move attribute members ahead of the first child member`},
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
