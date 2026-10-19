package jsonsrc

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// notedSchema holds the shapes a characters note must answer, and the ones
// whose charges share a scalar's location with the element's own text
// charge. eo has element-only content, e0 empty content, and sc simple
// content; lst holds ra items, ra having element-only content and a required
// attribute. tt's {type definition} is ·xs:anyType·, whose content is mixed,
// and its {type table} selects eoT's element-only content for an element
// with no attribute m; p's child c carries the same table. cw is of eoT, and
// its {type table}'s {test} is outside the engine's §3.12.6 compiler's subset,
// so the engine withholds its selection, though both types it can select have
// element-only content. Under p5 the second x is ·attributed to· the lax
// wildcard, so the top-level x governs it, whose empty e0T is not ·validly
// substitutable· for eoT, x's ·locally declared type· within p5
// (cvc-complex-type clause 5). Under pdfl, dfl's ·defaulted attribute· n of
// xs:ENTITY names no unparsed entity, a JSON document having no DTD
// (cvc-complex-type clause 4). abd is an abstract declaration (cvc-elt
// clause 2), and as carries a false assertion (cvc-assertion). mx has mixed
// content, any is of ·xs:anyType·, whose content is mixed, and str is of
// xs:string: none charges its text.
const notedSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="eoT"><xs:sequence><xs:element name="q" type="xs:string"/></xs:sequence></xs:complexType>
  <xs:complexType name="eoT2"><xs:complexContent><xs:extension base="eoT"><xs:attribute name="a"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="e0T"/>
  <xs:element name="eo" type="eoT"/>
  <xs:element name="e0" type="e0T"/>
  <xs:element name="sc"><xs:complexType><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType></xs:element>
  <xs:element name="mx">
    <xs:complexType mixed="true"><xs:sequence><xs:element name="q" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="any"/>
  <xs:element name="str" type="xs:string"/>
  <xs:element name="ra">
    <xs:complexType>
      <xs:sequence><xs:element name="q" type="xs:string"/></xs:sequence>
      <xs:attribute name="req" use="required"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="lst"><xs:complexType><xs:sequence><xs:element ref="ra" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="tt"><xs:alternative test="@m" type="xs:anyType"/><xs:alternative type="eoT"/></xs:element>
  <xs:element name="p">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="c" maxOccurs="unbounded"><xs:alternative test="@m" type="xs:anyType"/><xs:alternative type="eoT"/></xs:element>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="cw" type="eoT"><xs:alternative test="count(@a) > 0" type="eoT2"/></xs:element>
  <xs:element name="x" type="e0T"/>
  <xs:element name="p5">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="x" type="eoT" minOccurs="0"/>
        <xs:any namespace="##local" processContents="lax" maxOccurs="unbounded"/>
      </xs:sequence>
    </xs:complexType>
  </xs:element>
  <xs:element name="dfl">
    <xs:complexType>
      <xs:sequence><xs:element name="q" type="xs:string" minOccurs="0"/></xs:sequence>
      <xs:attribute name="n" type="xs:ENTITY" default="x"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pdfl"><xs:complexType><xs:sequence><xs:element ref="dfl" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="abd" abstract="true" type="eoT"/>
  <xs:complexType name="asT">
    <xs:sequence><xs:element name="q" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:assert test="false()"/>
  </xs:complexType>
  <xs:element name="as" type="asT"/>
  <xs:element name="pas"><xs:complexType><xs:sequence><xs:element ref="as" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>
</xs:schema>`

// charge is one violation a TestNoteHints row expects: its rule, its location
// and the clause after the engine's message, "" where none follows it.
type charge struct {
	rule      xsderr.Rule
	loc, hint string
}

// charactersHint is the clause a characters note appends for the member key
// whose scalar's lexical form is lexical, where its element's type has variety
// content.
func charactersHint(key, lexical, variety string) string {
	return `; JSON member "` + key + `" has the scalar value "` + lexical + `", but the type of its element has ` + variety + ` content: object expected, {"` + key + `":{...}}`
}

// TestNoteHints pins the clause Validate appends to the element's own text
// charge at a scalar member, and that it appends none to any other charge,
// the parent's clause 1.1, 1.2 and 5 charges and the element's clause 3 and 4
// charges at the scalar's location, an array item's, included, nor where the
// engine charges no text: an element-only element whose scalar is white space
// (cvc-complex-type clause 1.3), and one whose {type table} selection the
// engine withholds. Each charge's rule and location are the engine's.
func TestNoteHints(t *testing.T) {
	const complexContent xsderr.Rule = "cvc-complex-content"
	for _, c := range []struct {
		name, schema, doc string
		want              []charge
	}{
		{"scalar where an object is expected", readmeSchema, `{"order":"just text"}`, []charge{
			{ruleCvcComplexType, ":1:10", charactersHint("order", "just text", "element-only")},
		}},
		{"white space under empty content", notedSchema, `{"e0":"  "}`, []charge{
			{ruleCvcComplexType, ":1:7", charactersHint("e0", "  ", "empty")},
		}},
		{"mixed-content item under an empty parent", notedSchema, `{"e0":{"mx":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
		}},
		{"xs:anyType item under an empty parent", notedSchema, `{"e0":{"any":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:15", ""},
		}},
		{"simple-content item under an empty parent", notedSchema, `{"e0":{"sc":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
		}},
		{"mixed-content item under a simple-content parent", notedSchema, `{"sc":{"mx":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
		}},
		{"simple-type item under an empty parent", notedSchema, `{"e0":{"str":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:15", ""},
		}},
		{"white space under element-only content", notedSchema, `{"eo":"  "}`, []charge{
			{complexContent, ":1:2", ""},
		}},
		{"the empty string under empty content", notedSchema, `{"e0":""}`, nil},
		{"the empty string as an item under an empty parent", notedSchema, `{"e0":{"e0":[""]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
		}},
		{"array item with a scalar under an empty parent", notedSchema, `{"e0":{"eo":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
			{ruleCvcComplexType, ":1:14", charactersHint("eo", "x", "element-only")},
		}},
		{"array item with a scalar under a simple-content parent", notedSchema, `{"sc":{"eo":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
			{ruleCvcComplexType, ":1:14", charactersHint("eo", "x", "element-only")},
		}},
		{"array item with a scalar missing a required attribute", notedSchema, `{"lst":{"ra":["x"]}}`, []charge{
			{ruleCvcComplexType, ":1:15", ""},
			{ruleCvcComplexType, ":1:15", charactersHint("ra", "x", "element-only")},
		}},
		{"scalar root whose {type table} selects element-only content", notedSchema, `{"tt":"x"}`, []charge{
			{ruleCvcComplexType, ":1:7", charactersHint("tt", "x", "element-only")},
		}},
		{"white-space item under an empty parent", notedSchema, `{"e0":{"eo":["  "]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
			{complexContent, ":1:14", ""},
		}},
		{"white-space item under a simple-content parent", notedSchema, `{"sc":{"eo":["  "]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
			{complexContent, ":1:14", ""},
		}},
		{"white-space item missing a required attribute", notedSchema, `{"lst":{"ra":["  "]}}`, []charge{
			{ruleCvcComplexType, ":1:15", ""},
			{complexContent, ":1:15", ""},
		}},
		{"white-space items missing a required attribute", notedSchema, `{"lst":{"ra":[" "," "]}}`, []charge{
			{ruleCvcComplexType, ":1:15", ""},
			{complexContent, ":1:15", ""},
			{ruleCvcComplexType, ":1:19", ""},
			{complexContent, ":1:19", ""},
		}},
		{"white-space item with empty content under an empty parent", notedSchema, `{"e0":{"e0":["  "]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
			{ruleCvcComplexType, ":1:14", charactersHint("e0", "  ", "empty")},
		}},
		{"a {type table} below the root selecting element-only content", notedSchema, `{"p":{"c":"txt"}}`, []charge{
			{ruleCvcComplexType, ":1:11", charactersHint("c", "txt", "element-only")},
		}},
		{"a {type table} below the root, as an array item", notedSchema, `{"p":{"c":["txt"]}}`, []charge{
			{ruleCvcComplexType, ":1:12", charactersHint("c", "txt", "element-only")},
		}},
		{"a {type table} below the root, a white-space item", notedSchema, `{"p":{"c":["  "]}}`, []charge{
			{complexContent, ":1:12", ""},
		}},
		{"a withheld {type table} selection under an empty parent", notedSchema, `{"e0":{"cw":["txt"]}}`, []charge{
			{ruleCvcComplexType, ":1:14", ""},
		}},
		{"array item charged cvc-complex-type clause 5 against its parent", notedSchema, `{"p5":{"x":[{"q":"a"},"txt"]}}`, []charge{
			{ruleCvcComplexType, ":1:23", ""},
			{ruleCvcComplexType, ":1:23", charactersHint("x", "txt", "empty")},
		}},
		{"array item charged cvc-complex-type clause 4", notedSchema, `{"pdfl":{"dfl":["txt"]}}`, []charge{
			{ruleCvcComplexType, ":1:17", ""},
			{ruleCvcComplexType, ":1:17", charactersHint("dfl", "txt", "element-only")},
		}},
		{"array item of an abstract declaration under an empty parent", notedSchema, `{"e0":{"abd":["txt"]}}`, []charge{
			{ruleCvcComplexType, ":1:15", ""},
			{"cvc-elt", ":1:15", ""},
			{ruleCvcComplexType, ":1:15", charactersHint("abd", "txt", "element-only")},
		}},
		{"array item charged an assertion after its text", notedSchema, `{"pas":{"as":["txt"]}}`, []charge{
			{ruleCvcComplexType, ":1:15", charactersHint("as", "txt", "element-only")},
			{"cvc-assertion", ":1:15", ""},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := Validate(validatorOver(t, c.schema), strings.NewReader(c.doc), WithURI(jsonURI))
			if err != nil {
				t.Fatalf("Validate(%s): %v", c.doc, err)
			}
			v := res.Violations()
			if len(v) != len(c.want) {
				t.Fatalf("Validate(%s) violations = %v, want %d", c.doc, v, len(c.want))
			}
			for i, w := range c.want {
				if v[i].Rule != w.rule || v[i].Loc.String() != jsonURI+w.loc {
					t.Fatalf("Validate(%s) violation %d = %v, want a %s charge at %s", c.doc, i+1, v[i], w.rule, jsonURI+w.loc)
				}
				_, hint, found := strings.Cut(v[i].Msg, "; JSON member ")
				if found {
					hint = "; JSON member " + hint
				}
				if hint != w.hint {
					t.Errorf("Validate(%s) violation %d message %q\nends with %q, want %q", c.doc, i+1, v[i].Msg, hint, w.hint)
				}
			}
		})
	}
}
