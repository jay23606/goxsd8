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
			wantCharges(t, c.schema, c.doc, c.want)
		})
	}
}

// wantCharges validates doc against schema and fails unless its violations are
// want, in order: each one's rule and location, and the clause from the first
// "; JSON member" on, "" where its message carries none.
func wantCharges(t *testing.T, schema, doc string, want []charge) {
	t.Helper()
	res, err := Validate(validatorOver(t, schema), strings.NewReader(doc), WithURI(jsonURI))
	if err != nil {
		t.Fatalf("Validate(%s): %v", doc, err)
	}
	v := res.Violations()
	if len(v) != len(want) {
		t.Fatalf("Validate(%s) violations = %v, want %d", doc, v, len(want))
	}
	for i, w := range want {
		if v[i].Rule != w.rule || v[i].Loc.String() != jsonURI+w.loc {
			t.Fatalf("Validate(%s) violation %d = %v, want a %s charge at %s", doc, i+1, v[i], w.rule, jsonURI+w.loc)
		}
		_, hint, found := strings.Cut(v[i].Msg, "; JSON member ")
		if found {
			hint = "; JSON member " + hint
		}
		if hint != w.hint {
			t.Errorf("Validate(%s) violation %d message %q\nends with %q, want %q", doc, i+1, v[i].Msg, hint, w.hint)
		}
	}
}

// childSchema holds the parents whose content a scalarExpected or unnamed note
// answers the charge of, and the children whose own charges share the
// member's rule and location. item is of xs:string and num of xs:int; t is of
// abs, an abstract complex type, and conc extends abs with an optional c; eo
// has element-only content and e0 empty content; r's one particle is an
// optional n; w's is a lax ##local wildcard and sk's a skip one; lst holds r
// items. kT has an attribute use m and an optional child n, kExt extends it
// with an optional y, and kR restricts it to the same keys. tt's child k is of
// kT, and its {type table} selects kExt for an element with an attribute m;
// tr's k selects kR so, and tw's k selects kR by a {test} outside the engine's
// §3.12.6 compiler's subset, so that the engine withholds its selection. st is
// of kT and ab of abs; uT, from which neither kT nor aW is derived, has the
// attribute use u; sa is of aW, whose {attribute wildcard} is a lax ##local
// one. Under p5 the second x is ·attributed to· the lax wildcard, so the
// top-level x, of kExt, governs it, while its members are classified under kT,
// the local x's type. xu's type has an attribute use of xsi:type, cu's an
// attribute use and a child declaration both named kind, and pr's simple
// content an attribute use kind.
const childSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <xs:import namespace="http://www.w3.org/2001/XMLSchema-instance"/>
  <xs:complexType name="abs" abstract="true">
    <xs:sequence><xs:element name="z" type="xs:string"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="conc"><xs:complexContent><xs:extension base="abs">
    <xs:sequence><xs:element name="c" type="xs:string" minOccurs="0"/></xs:sequence>
  </xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="t" type="abs"/>
  <xs:element name="item" type="xs:string"/>
  <xs:element name="num" type="xs:int"/>
  <xs:element name="eo">
    <xs:complexType><xs:sequence><xs:element name="q" type="xs:string"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="e0"><xs:complexType/></xs:element>
  <xs:element name="r">
    <xs:complexType><xs:sequence><xs:element name="n" type="xs:int" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="w">
    <xs:complexType><xs:sequence><xs:any namespace="##local" processContents="lax"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="sk">
    <xs:complexType><xs:sequence><xs:any namespace="##local" processContents="skip"/></xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="lst"><xs:complexType><xs:sequence><xs:element ref="r" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>
  <xs:complexType name="kT">
    <xs:sequence><xs:element name="n" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:attribute name="m"/>
  </xs:complexType>
  <xs:complexType name="kExt"><xs:complexContent><xs:extension base="kT">
    <xs:sequence><xs:element name="y" type="xs:string" minOccurs="0"/></xs:sequence>
  </xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="kR"><xs:complexContent><xs:restriction base="kT">
    <xs:sequence><xs:element name="n" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:attribute name="m"/>
  </xs:restriction></xs:complexContent></xs:complexType>
  <xs:element name="tt">
    <xs:complexType><xs:sequence>
      <xs:element name="k" type="kT"><xs:alternative test="@m" type="kExt"/></xs:element>
    </xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="tr">
    <xs:complexType><xs:sequence>
      <xs:element name="k" type="kT"><xs:alternative test="@m" type="kR"/></xs:element>
    </xs:sequence></xs:complexType>
  </xs:element>
  <xs:element name="tw">
    <xs:complexType><xs:sequence>
      <xs:element name="k" type="kT"><xs:alternative test="count(@m) > 0" type="kR"/></xs:element>
    </xs:sequence></xs:complexType>
  </xs:element>
  <xs:complexType name="uT">
    <xs:sequence><xs:element name="n" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:attribute name="u"/>
  </xs:complexType>
  <xs:complexType name="aW">
    <xs:sequence><xs:element name="n" type="xs:string" minOccurs="0"/></xs:sequence>
    <xs:anyAttribute namespace="##local" processContents="lax"/>
  </xs:complexType>
  <xs:element name="sa" type="aW"/>
  <xs:element name="st" type="kT"/>
  <xs:element name="ab" type="abs"/>
  <xs:element name="x" type="kExt"/>
  <xs:element name="p5"><xs:complexType><xs:sequence>
    <xs:element name="x" type="kT" minOccurs="0"/>
    <xs:any namespace="##local" processContents="lax" maxOccurs="unbounded"/>
  </xs:sequence></xs:complexType></xs:element>
  <xs:element name="xu">
    <xs:complexType>
      <xs:sequence><xs:element name="c" type="xs:string" minOccurs="0"/></xs:sequence>
      <xs:attribute ref="xsi:type"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="cu">
    <xs:complexType>
      <xs:sequence><xs:element name="kind" type="xs:string" minOccurs="0"/></xs:sequence>
      <xs:attribute name="kind"/>
    </xs:complexType>
  </xs:element>
  <xs:element name="pr">
    <xs:complexType><xs:simpleContent><xs:extension base="xs:string">
      <xs:attribute name="kind"/>
    </xs:extension></xs:simpleContent></xs:complexType>
  </xs:element>
</xs:schema>`

// selectedSimpleSchema declares a, of ·xs:anyType·, and ta, whose {type
// table} selects xs:string for an element with no attribute m, and pt, whose
// child c carries that table.
const selectedSimpleSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"/>
  <xs:element name="ta"><xs:alternative test="@m" type="xs:anyType"/><xs:alternative type="xs:string"/></xs:element>
  <xs:element name="pt">
    <xs:complexType><xs:sequence>
      <xs:element name="c"><xs:alternative test="@m" type="xs:anyType"/><xs:alternative type="xs:string"/></xs:element>
    </xs:sequence></xs:complexType>
  </xs:element>
</xs:schema>`

// kindsSchema declares, in urn:t, du, whose type has an optional child c and
// the attribute uses kind, unqualified, and t:kind, of two expanded names.
const kindsSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns:t="urn:t">
  <xs:element name="du">
    <xs:complexType>
      <xs:sequence><xs:element name="c" type="xs:string" minOccurs="0"/></xs:sequence>
      <xs:attribute name="kind"/>
      <xs:attribute ref="t:kind"/>
    </xs:complexType>
  </xs:element>
  <xs:attribute name="kind"/>
</xs:schema>`

// scalarExpectedHint is the clause a scalarExpected note appends for the
// member key of the object of owner.
func scalarExpectedHint(key, owner string) string {
	return `; JSON member "` + key + `" is a member of the object of "` + owner + `", whose type is a simple type: scalar expected, {"` + owner + `":<string, number or boolean>}`
}

// unnamedHint is the clause an unnamed note appends for the member key of the
// object of owner, whose type carries keys, each quoted, "" for none.
func unnamedHint(key, owner, keys string) string {
	hint := `; JSON member "` + key + `" names no child element declaration and no attribute use of the type of "` + owner + `"`
	if keys != "" {
		hint += ", whose members are keyed " + keys
	}
	return hint
}

// TestChildNoteHints pins the clause Validate appends to the charge the
// parent's content takes at a child member: an unnamed note's at a child the
// type of the parent names none of, and a scalarExpected note's at the first
// child of a parent whose type is a simple type; and that it appends neither
// to any other charge, the child's own at the same rule and location, the
// parent's at a later child, a parent's whose type names the child or has a
// wildcard, and one whose content is empty or simple, included. The key list
// is the keys of the type that governs the parent — the type its {type table}
// selects, the one its "$type" names where that ·overrides·, the top-level
// declaration's a lax wildcard's child resolves to — and not those of the type
// its members are classified under where the two differ; it omits a key the
// mapping declines, and the "_" clause names only a use the mapping names by
// the key.
func TestChildNoteHints(t *testing.T) {
	const (
		content = ruleCvcComplexContent
		simple  = ruleCvcType
	)
	for _, c := range []struct {
		name, schema, doc string
		want              []charge
	}{
		{"unknown key", readmeSchema, `{"order":{"kind":"x","colour":"red","item":"a"}}`, []charge{
			{content, ":1:22", unnamedHint("colour", "order", `"kind", "item"`)},
		}},
		{"attribute key behind _", readmeSchema, `{"order":{"_kind":"x","item":"a"}}`, []charge{
			{content, ":1:11", unnamedHint("_kind", "order", `"kind", "item"`) + `; an attribute member is keyed by the attribute's own name, "kind", not "_kind"`},
		}},
		{"object where a scalar is expected", readmeSchema, `{"order":{"item":"a","item":{"y":"b"}}}`, []charge{
			{simple, ":1:30", scalarExpectedHint("y", "item")},
		}},
		{"object where a scalar is expected, as an array item", readmeSchema, `{"order":{"item":["a",{"y":"b"}]}}`, []charge{
			{simple, ":1:24", scalarExpectedHint("y", "item")},
		}},
		{"own cvc-type at a second member of a simple parent", childSchema, `{"item":{"y":"b","num":"abc"}}`, []charge{
			{simple, ":1:10", scalarExpectedHint("y", "item")},
			{simple, ":1:18", ""},
		}},
		{"own cvc-type clause 3.1.3 at the first member", childSchema, `{"item":{"num":"abc"}}`, []charge{
			{simple, ":1:10", scalarExpectedHint("num", "item")},
			{simple, ":1:10", ""},
		}},
		{"own cvc-type clause 2 at the first member", childSchema, `{"item":{"t":{"z":"1"}}}`, []charge{
			{simple, ":1:10", scalarExpectedHint("t", "item")},
			{simple, ":1:10", ""},
		}},
		{"own cvc-complex-content at an unnamed member", childSchema, `{"r":{"n":1,"eo":{}}}`, []charge{
			{content, ":1:13", unnamedHint("eo", "r", `"n"`)},
			{content, ":1:13", ""},
		}},
		{"own cvc-type and cvc-complex-content at an unnamed member", childSchema, `{"r":{"n":1,"t":{}}}`, []charge{
			{content, ":1:13", unnamedHint("t", "r", `"n"`)},
			{simple, ":1:13", ""},
			{content, ":1:13", ""},
		}},
		{"unnamed member after the parent's content is charged", childSchema, `{"r":{"n":1,"n":2,"eo":{}}}`, []charge{
			{content, ":1:13", ""},
			{content, ":1:19", ""},
		}},
		{"unnamed array item with a scalar", childSchema, `{"r":{"n":1,"eo":["x"]}}`, []charge{
			{content, ":1:19", unnamedHint("eo", "r", `"n"`)},
			{ruleCvcComplexType, ":1:19", charactersHint("eo", "x", "element-only")},
		}},
		{"array item with a scalar under a simple parent", childSchema, `{"item":{"eo":["x"]}}`, []charge{
			{simple, ":1:16", scalarExpectedHint("eo", "item")},
			{ruleCvcComplexType, ":1:16", charactersHint("eo", "x", "element-only")},
		}},
		{"a \"$type\" naming a simple type", selectedSimpleSchema, `{"a":{"$xmlns":{"xs":"http://www.w3.org/2001/XMLSchema"},"$type":"xs:string","y":"1","z":"2"}}`, []charge{
			{simple, ":1:78", scalarExpectedHint("y", "a")},
		}},
		{"a root {type table} selecting a simple type", selectedSimpleSchema, `{"ta":{"y":"1","z":"2"}}`, []charge{
			{simple, ":1:8", scalarExpectedHint("y", "ta")},
		}},
		{"a {type table} below the root selecting a simple type", selectedSimpleSchema, `{"pt":{"c":{"y":"1","z":"2"}}}`, []charge{
			{simple, ":1:13", scalarExpectedHint("y", "c")},
		}},
		{"unknown key under an empty parent", childSchema, `{"e0":{"zz":"1"}}`, []charge{
			{ruleCvcComplexType, ":1:8", ""},
		}},
		{"attribute key behind _ under a simple-content parent", childSchema, `{"pr":{"_kind":"1"}}`, []charge{
			{ruleCvcComplexType, ":1:8", ""},
		}},
		{"a {type table} below the root selecting a type with more keys", childSchema, `{"tt":{"k":{"m":"1","zz":"1"}}}`, []charge{
			{content, ":1:21", unnamedHint("zz", "k", `"m", "n", "y"`)},
		}},
		{"a {type table} below the root selecting the declared type", childSchema, `{"tt":{"k":{"zz":"1"}}}`, []charge{
			{content, ":1:13", unnamedHint("zz", "k", `"m", "n"`)},
		}},
		{"a {type table} below the root selecting a type with the same keys", childSchema, `{"tr":{"k":{"m":"1","zz":"1"}}}`, []charge{
			{content, ":1:21", unnamedHint("zz", "k", `"m", "n"`)},
		}},
		{"a withheld {type table} selection among types with the same keys", childSchema, `{"tw":{"k":{"m":"1","zz":"1"}}}`, nil},
		{"declared child repeated", childSchema, `{"r":{"n":1,"n":2}}`, []charge{
			{content, ":1:13", ""},
		}},
		{"wildcard-named child rejected", childSchema, `{"w":{"a":"1","b":"2"}}`, []charge{
			{content, ":1:15", ""},
		}},
		{"cvc-type at an unnamed member of a complex parent", childSchema, `{"r":{"t":{"z":"1"}}}`, []charge{
			{content, ":1:7", unnamedHint("t", "r", `"n"`)},
			{simple, ":1:7", ""},
		}},
		{"a \"$type\" that overrides", childSchema, `{"st":{"$type":"kExt","zz":"1"}}`, []charge{
			{content, ":1:23", unnamedHint("zz", "st", `"m", "n", "y"`)},
		}},
		{"a \"$type\" that does not override", childSchema, `{"st":{"$type":"abs","zz":"1"}}`, []charge{
			{"cvc-elt", ":1:2", ""},
			{content, ":1:22", unnamedHint("zz", "st", `"m", "n"`)},
		}},
		{"_ before a use of the \"$type\" that does not override", childSchema, `{"st":{"$type":"uT","_u":"1"}}`, []charge{
			{"cvc-elt", ":1:2", ""},
			{content, ":1:21", unnamedHint("_u", "st", `"m", "n"`)},
		}},
		{"a \"$type\" that does not override a type with an {attribute wildcard}", childSchema, `{"sa":{"$type":"uT","zz":"1"}}`, []charge{
			{"cvc-elt", ":1:2", ""},
			{content, ":1:21", ""},
		}},
		{"an abstract parent with \"$type\"", childSchema, `{"ab":{"$type":"conc","zz":"1"}}`, []charge{
			{content, ":1:23", unnamedHint("zz", "ab", `"z", "c"`)},
		}},
		{"an unbound \"$type\" prefix", childSchema, `{"st":{"$type":"u:kExt","zz":"1"}}`, []charge{
			{"cvc-attribute", ":1:8", ""},
			{content, ":1:25", unnamedHint("zz", "st", `"m", "n"`)},
		}},
		{"a skipped parent", childSchema, `{"sk":{"r":{"zz":"1"}}}`, nil},
		{"a parent a lax wildcard's child resolves", childSchema, `{"w":{"r":{"zz":"1"}}}`, []charge{
			{content, ":1:12", unnamedHint("zz", "r", `"n"`)},
		}},
		{"a parent governed by the top-level declaration a lax wildcard resolves to", childSchema, `{"p5":{"x":[{},{"zz":"1"}]}}`, []charge{
			{content, ":1:17", unnamedHint("zz", "x", `"m", "n", "y"`)},
		}},
		{"a child the governing type names and the classifying type does not", childSchema, `{"p5":{"x":[{},{"y":"1","y":"2"}]}}`, []charge{
			{content, ":1:25", ""},
		}},
		{"unnamed members of array items", childSchema, `{"lst":{"r":[{"n":1,"zz":2},{"zz":3}]}}`, []charge{
			{content, ":1:21", unnamedHint("zz", "r", `"n"`)},
			{content, ":1:30", unnamedHint("zz", "r", `"n"`)},
		}},
		{"_ before an xsi-namespace use's name", childSchema, `{"xu":{"_type":"1"}}`, []charge{
			{content, ":1:8", unnamedHint("_type", "xu", `"c"`)},
		}},
		{"_ before a name a child declaration and a use carry", childSchema, `{"cu":{"_kind":"1"}}`, []charge{
			{content, ":1:8", unnamedHint("_kind", "cu", "")},
		}},
		{"_ before the local name of uses of two expanded names", kindsSchema, `{"t:du":{"$xmlns":{"t":"urn:t"},"_kind":"1"}}`, []charge{
			{content, ":1:33", unnamedHint("_kind", "t:du", `"c"`)},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			wantCharges(t, c.schema, c.doc, c.want)
		})
	}
}
