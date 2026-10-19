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
// wildcard, and one whose content is empty or simple, included. A "$type"
// charge in its rows carries the clause TestSpellingNoteHints pins. The key list
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
			{ruleCvcElt, ":1:2", overrideHint("st", "abs")},
			{content, ":1:22", unnamedHint("zz", "st", `"m", "n"`)},
		}},
		{"_ before a use of the \"$type\" that does not override", childSchema, `{"st":{"$type":"uT","_u":"1"}}`, []charge{
			{ruleCvcElt, ":1:2", overrideHint("st", "uT")},
			{content, ":1:21", unnamedHint("_u", "st", `"m", "n"`)},
		}},
		{"a \"$type\" that does not override a type with an {attribute wildcard}", childSchema, `{"sa":{"$type":"uT","zz":"1"}}`, []charge{
			{ruleCvcElt, ":1:2", overrideHint("sa", "uT")},
		}},
		{"an abstract parent with \"$type\"", childSchema, `{"ab":{"$type":"conc","zz":"1"}}`, []charge{
			{content, ":1:23", unnamedHint("zz", "ab", `"z", "c"`)},
		}},
		{"an unbound \"$type\" prefix", childSchema, `{"st":{"$type":"u:kExt","zz":"1"}}`, []charge{
			{ruleCvcAttribute, ":1:8", typeLexicalHint("st", "u")},
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

// spellSchema is the schema of #2829: in urn:o, order is of base, a sequence
// of one qty of xs:int, and other is a sequence of one z of xs:string.
const spellSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:o="urn:o" targetNamespace="urn:o">
  <xs:element name="order" type="o:base"/>
  <xs:complexType name="base"><xs:sequence><xs:element name="qty" type="xs:int"/></xs:sequence></xs:complexType>
  <xs:complexType name="other"><xs:sequence><xs:element name="z" type="xs:string"/></xs:sequence></xs:complexType>
</xs:schema>`

// spellProbeSchema holds, in urn:o, the shapes whose charges share a rule and
// location with a "$type", null or array-item charge. base has an attribute
// use m and a sequence of up to two qty of xs:int, then an optional ref to ab,
// abstract, of xs:int, an optional ref to fx, nillable and fixed "3", of
// xs:int, and an optional line, a sequence of one a. order is of base and ao
// abstract of base; baseM extends base with nothing and abs, abstract, with
// nothing too; other is a sequence of an optional z, derived from no base. tt
// is of base and its {type table} selects baseM for an element with an
// attribute m; tw's selects it by a {test} outside the engine's §3.12.6
// compiler's subset, so that the engine withholds its selection; ta is
// abstract and carries tt's table, and twa is abstract and carries tw's; tw1's
// table selects base itself by tw's {test}, so that it can select one type,
// and hw's selects its own type hold so, a sequence of an x holding a tt. dq is
// of ·xs:anySimpleType· with the default "abc", and its {type table} selects
// xs:int for an element with no attribute m. w's one particle is a lax ##local
// wildcard.
const spellProbeSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:o="urn:o" targetNamespace="urn:o">
  <xs:element name="order" type="o:base"/>
  <xs:element name="ao" type="o:base" abstract="true"/>
  <xs:element name="ab" type="xs:int" abstract="true"/>
  <xs:element name="fx" type="xs:int" nillable="true" fixed="3"/>
  <xs:element name="tt" type="o:base"><xs:alternative test="@m" type="o:baseM"/></xs:element>
  <xs:element name="tw" type="o:base"><xs:alternative test="count(@m) > 0" type="o:baseM"/></xs:element>
  <xs:element name="ta" type="o:base" abstract="true"><xs:alternative test="@m" type="o:baseM"/></xs:element>
  <xs:element name="tw1" type="o:base"><xs:alternative test="count(@m) > 0" type="o:base"/></xs:element>
  <xs:element name="hw" type="o:hold"><xs:alternative test="count(@m) > 0" type="o:hold"/></xs:element>
  <xs:complexType name="hold"><xs:sequence><xs:element name="x" minOccurs="0">
    <xs:complexType><xs:sequence><xs:element ref="o:tt" minOccurs="0"/></xs:sequence></xs:complexType>
  </xs:element></xs:sequence></xs:complexType>
  <xs:element name="twa" type="o:base" abstract="true"><xs:alternative test="count(@m) > 0" type="o:baseM"/></xs:element>
  <xs:element name="dq" type="xs:anySimpleType" default="abc"><xs:alternative test="not(@m)" type="xs:int"/></xs:element>
  <xs:element name="w"><xs:complexType><xs:sequence><xs:any namespace="##local" processContents="lax" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>
  <xs:complexType name="base">
    <xs:sequence>
      <xs:element name="qty" type="xs:int" minOccurs="0" maxOccurs="2"/>
      <xs:element ref="o:ab" minOccurs="0"/>
      <xs:element ref="o:fx" minOccurs="0"/>
      <xs:element name="line" minOccurs="0"><xs:complexType><xs:sequence><xs:element name="a" type="xs:int"/></xs:sequence></xs:complexType></xs:element>
    </xs:sequence>
    <xs:attribute name="m"/>
  </xs:complexType>
  <xs:complexType name="baseM"><xs:complexContent><xs:extension base="o:base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="abs" abstract="true"><xs:complexContent><xs:extension base="o:base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="other"><xs:sequence><xs:element name="z" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
</xs:schema>`

// overrideHint is the clause a notOverriding note appends for the object
// member key whose "$type" has the value lexical.
func overrideHint(key, lexical string) string {
	return `; JSON member "$type" of "` + key + `" spells its xsi:type, "` + lexical + `": the type it names governs the element only where it ·overrides· the type the element's declaration selects, so name one that does, or drop the "$type"`
}

// typeLexicalHint is the clause a typeLexical note appends for the "$type" of
// the object member key, whose value's one fault is the unbound prefix, ""
// where its fault is another.
func typeLexicalHint(key, prefix string) string {
	if prefix == "" {
		return `; JSON member "$type" of "` + key + `" spells its xsi:type, an xs:QName: give it as "local" or "p:local", each part an NCName`
	}
	return `; JSON member "$type" of "` + key + `" spells its xsi:type, an xs:QName, and no "$xmlns" in scope binds its prefix "` + prefix + `": bind it in a "$xmlns" member leading this object or one enclosing it, {"$xmlns":{"` + prefix + `":"<namespace name>"}}`
}

// nilHint is the clause a nonNillable note appends for the null member key.
func nilHint(key string) string {
	return `; JSON member "` + key + `" is null, which spells xsi:nil="true", but the declaration of its element is not nillable: give it a value`
}

// arrayItemHint is the clause an arrayItem note appends for item n of the
// array member key.
func arrayItemHint(key, n string) string {
	return `; JSON member "` + key + `" is an array, each of whose items is one element of that name: the element charged is item ` + n + ` of it`
}

// TestSpellingNoteHints pins the clause Validate appends to a charge against a
// JSON spelling: the element's cvc-elt clause 4 charge where its "$type" does
// not ·override·, the "$type"'s cvc-attribute clause 3 charge, the element's
// cvc-elt clause 3.1 charge at a null under a declaration that is not
// nillable, and the parent's cvc-complex-content charge at an array item; and
// that it appends none to any other charge at the same rule and location — the
// element's cvc-elt clause 2 charge before or after its clause 3.1 or 4 one,
// an array item's own content charge, an unnamed note's charge at an item, the
// cvc-type charge over the empty value that follows — nor where the engine
// makes no such charge: a "$type" that ·overrides·, one naming no type
// (cvc-attribute clause 5), one whose element's own {type table} selection
// the engine withholds, a nillable fixed declaration's clause 3.2.3.2 charge,
// a parent's content charged at an earlier child, an element the engine walks
// undecided below a withheld selection or an unattributed parent, and a null
// no declaration governs. Each charge's rule and location are the engine's.
func TestSpellingNoteHints(t *testing.T) {
	const (
		content = ruleCvcComplexContent
		order   = `{"o:order":{"$xmlns":{"o":"urn:o"},`
	)
	for _, c := range []struct {
		name, schema, doc string
		want              []charge
	}{
		{"a \"$type\" that does not override", spellSchema, order + `"$type":"o:other","qty":1}}`, []charge{
			{ruleCvcElt, ":1:2", overrideHint("o:order", "o:other")},
		}},
		{"a \"$type\" prefix no \"$xmlns\" binds", spellSchema, order + `"qty":{"$type":"xs:short"}}}`, []charge{
			{ruleCvcAttribute, ":1:43", typeLexicalHint("qty", "xs")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"a \"$type\" that is no QName", spellSchema, order + `"qty":{"$type":"1bad"}}}`, []charge{
			{ruleCvcAttribute, ":1:43", typeLexicalHint("qty", "")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"a \"$type\" whose prefix is no NCName", spellSchema, order + `"qty":{"$type":"1x:short"}}}`, []charge{
			{ruleCvcAttribute, ":1:43", typeLexicalHint("qty", "")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"a \"$type\" of two colons", spellSchema, order + `"qty":{"$type":"a:b:c"}}}`, []charge{
			{ruleCvcAttribute, ":1:43", typeLexicalHint("qty", "")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"an empty \"$type\"", spellSchema, order + `"qty":{"$type":" "}}}`, []charge{
			{ruleCvcAttribute, ":1:43", typeLexicalHint("qty", "")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"a null under a declaration that is not nillable", spellSchema, order + `"qty":null}}`, []charge{
			{ruleCvcElt, ":1:36", nilHint("qty")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"an array item the parent's content model rejects", spellSchema, order + `"qty":[1,2]}}`, []charge{
			{content, ":1:45", arrayItemHint("qty", "2")},
		}},
		{"an unnamed array item", spellSchema, order + `"zz":[1,2]}}`, []charge{
			{content, ":1:42", unnamedHint("zz", "o:order", `"qty"`)},
		}},
		{"a \"$type\" naming no type", spellSchema, order + `"$type":"o:nosuch","qty":1}}`, []charge{
			{ruleCvcAttribute, ":1:36", ""},
		}},
		{"a \"$type\" that overrides", spellSchema, order + `"$type":"o:base","qty":1}}`, nil},
		{"a \"$type\" that does not override, of an abstract declaration", spellProbeSchema, `{"o:ao":{"$xmlns":{"o":"urn:o"},"$type":"o:other"}}`, []charge{
			{ruleCvcElt, ":1:2", overrideHint("o:ao", "o:other")},
			{ruleCvcElt, ":1:2", ""},
		}},
		{"a \"$type\" that overrides, of an abstract declaration", spellProbeSchema, `{"o:ao":{"$xmlns":{"o":"urn:o"},"$type":"o:baseM"}}`, []charge{
			{ruleCvcElt, ":1:2", ""},
		}},
		{"a null under an abstract declaration", spellProbeSchema, order + `"o:ab":null}}`, []charge{
			{ruleCvcElt, ":1:36", ""},
			{ruleCvcElt, ":1:36", nilHint("o:ab")},
			{ruleCvcType, ":1:36", ""},
		}},
		{"a null under a nillable fixed declaration", spellProbeSchema, order + `"o:fx":null}}`, []charge{
			{ruleCvcElt, ":1:36", ""},
		}},
		{"an array item charged by its parent and by its own content", spellProbeSchema, order + `"line":[{"a":1},{}]}}`, []charge{
			{content, ":1:52", arrayItemHint("line", "2")},
			{content, ":1:52", ""},
		}},
		{"an array item past maxOccurs", spellProbeSchema, order + `"qty":[1,2,3]}}`, []charge{
			{content, ":1:47", arrayItemHint("qty", "3")},
		}},
		{"array items after the parent's content is charged", spellProbeSchema, order + `"zz":1,"qty":[1,2,3]}}`, []charge{
			{content, ":1:36", unnamedHint("zz", "o:order", `"m", "qty", "ab", "fx", "line"`)},
		}},
		{"a null array item the parent's content model rejects", spellProbeSchema, order + `"qty":[1,2,null]}}`, []charge{
			{content, ":1:47", arrayItemHint("qty", "3")},
			{ruleCvcType, ":1:47", ""},
		}},
		{"a null array item", spellProbeSchema, order + `"qty":[1,null]}}`, []charge{
			{ruleCvcElt, ":1:45", nilHint("qty")},
			{ruleCvcType, ":1:45", ""},
		}},
		{"a \"$type\" that is no QName, on an array item", spellProbeSchema, order + `"qty":[{"$type":"1bad"}]}}`, []charge{
			{ruleCvcAttribute, ":1:44", typeLexicalHint("qty", "")},
			{ruleCvcType, ":1:43", ""},
		}},
		{"a \"$type\" that does not override, on an array item", spellProbeSchema, order + `"line":[{"$type":"o:other"}]}}`, []charge{
			{ruleCvcElt, ":1:44", overrideHint("line", "o:other")},
			{content, ":1:44", ""},
		}},
		{"a \"$type\" that does not override the type a {type table} selects", spellProbeSchema, `{"o:tt":{"$xmlns":{"o":"urn:o"},"$type":"o:other"}}`, []charge{
			{ruleCvcElt, ":1:2", overrideHint("o:tt", "o:other")},
		}},
		{"a \"$type\" that does not override the type a {type table} selects, of an abstract declaration", spellProbeSchema, `{"o:ta":{"$xmlns":{"o":"urn:o"},"$type":"o:other"}}`, []charge{
			{ruleCvcElt, ":1:2", overrideHint("o:ta", "o:other")},
			{ruleCvcElt, ":1:2", ""},
		}},
		{"a \"$type\" that overrides the type a {type table} selects, of an abstract declaration", spellProbeSchema, `{"o:ta":{"$xmlns":{"o":"urn:o"},"$type":"o:baseM"}}`, []charge{
			{ruleCvcElt, ":1:2", ""},
		}},
		{"a \"$type\" under a withheld {type table} selection", spellProbeSchema, `{"o:tw":{"$xmlns":{"o":"urn:o"},"$type":"o:other"}}`, nil},
		{"a \"$type\" under a withheld {type table} selection, of an abstract declaration", spellProbeSchema, `{"o:twa":{"$xmlns":{"o":"urn:o"},"$type":"o:other"}}`, []charge{
			{ruleCvcElt, ":1:2", ""},
		}},
		{"a null below a withheld {type table} selection", spellProbeSchema, `{"o:tw":{"$xmlns":{"o":"urn:o"},"qty":null}}`, nil},
		{"a null of an abstract declaration under a lax wildcard", spellProbeSchema, `{"o:w":{"$xmlns":{"o":"urn:o"},"o:ao":null}}`, []charge{
			{content, ":1:32", ""},
			{ruleCvcElt, ":1:32", ""},
			{ruleCvcElt, ":1:32", nilHint("o:ao")},
		}},
		{"a \"$type\" that does not override, under a lax wildcard", spellProbeSchema, `{"o:w":{"$xmlns":{"o":"urn:o"},"o:order":{"$type":"o:other"}}}`, []charge{
			{content, ":1:32", ""},
			{ruleCvcElt, ":1:32", overrideHint("o:order", "o:other")},
		}},
		{"an undeclared null", spellProbeSchema, `{"o:w":{"$xmlns":{"o":"urn:o"},"zz":null}}`, nil},
		{"a null root", childSchema, `{"st":null}`, []charge{
			{ruleCvcElt, ":1:2", nilHint("st")},
		}},
		{"a null root whose {type table} selects a type its default is invalid for", spellProbeSchema, `{"dq":null}`, []charge{
			{ruleCvcElt, ":1:2", nilHint("dq")},
			{ruleCvcType, ":1:2", ""},
		}},
		{"a null below an unattributed parent", inheritSchema, `{"big":{"mid":null,"b":""}}`, nil},
		{"array items below an unattributed parent", inheritSchema, `{"big":{"mid":[{},{}],"b":""}}`, nil},
		{"a \"$type\" that is no QName below an unattributed parent", inheritSchema, `{"big":{"mid":{"$type":"1bad"},"b":""}}`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			wantCharges(t, c.schema, c.doc, c.want)
		})
	}
}

// TestNotesKept pins how many notes the mapping keeps for a document, as
// Validate keeps them, beside the charges they answer: one per charge where
// the engine assesses the elements, and none where it walks them undecided —
// below an element whose {type table} selection it withholds, or a parent
// whose content model xsd.Schema.ContentMatcher declines — or skips them, so
// that the notes kept there do not grow with the document (STYLE P4).
func TestNotesKept(t *testing.T) {
	const tw = `{"o:tw":{"$xmlns":{"o":"urn:o"},`
	for _, c := range []struct {
		name, schema, doc string
		notes, charges    int
	}{
		{"nulls the engine charges", spellProbeSchema, `{"o:order":{"$xmlns":{"o":"urn:o"},"qty":[null,null]}}`, 2, 4},
		{"nulls below a withheld selection", spellProbeSchema, tw + `"qty":[null,null]}}`, 0, 0},
		{"\"$type\"s of no QName below a withheld selection", spellProbeSchema, tw + `"qty":[{"$type":"1bad"},{"$type":"1bad"}]}}`, 0, 0},
		{"\"$type\"s that do not override below a withheld selection", spellProbeSchema, tw + `"qty":[{"$type":"o:other"},{"$type":"o:other"}]}}`, 0, 0},
		{"nulls two levels below a withheld selection", spellProbeSchema, tw + `"line":{"a":[null,null]}}}`, 0, 0},
		{"nulls below a withheld selection of one type", spellProbeSchema, `{"o:tw1":{"$xmlns":{"o":"urn:o"},"qty":[null,null]}}`, 0, 0},
		{"nulls below a \"$type\" over a withheld selection of one type", spellProbeSchema, `{"o:tw1":{"$xmlns":{"o":"urn:o"},"$type":"o:baseM","qty":[null,null]}}`, 0, 0},
		{"a scalar of element-only content below a withheld selection of one type", spellProbeSchema, `{"o:tw1":{"$xmlns":{"o":"urn:o"},"line":"x"}}`, 0, 0},
		{"a \"$type\" over a read selection below a withheld selection of one type", spellProbeSchema, `{"o:hw":{"$xmlns":{"o":"urn:o"},"x":{"o:tt":{"$type":"o:other"}}}}`, 0, 0},
		{"unnamed members two levels below a withheld selection", spellProbeSchema, tw + `"line":{"zz":1,"zy":2}}}`, 0, 0},
		{"scalars of element-only content below a withheld selection", spellProbeSchema, tw + `"line":["x","y"]}}`, 0, 0},
		{"\"$type\"s two levels below a withheld selection", spellProbeSchema, tw + `"line":{"a":{"$type":"1bad"}}}}`, 0, 0},
		{"nulls below an unattributed parent", inheritSchema, `{"big":{"mid":[null,null],"b":""}}`, 0, 0},
		{"a skipped \"$type\" of no QName", childSchema, `{"sk":{"r":{"$type":"1bad"}}}`, 0, 0},
		{"a skipped null", childSchema, `{"sk":{"r":null}}`, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := validatorOver(t, c.schema)
			w := newWalker(jsonURI, strings.NewReader(c.doc), v.Schema(), v.ConditionallySelected, v.Governing)
			root, err := w.root()
			if err != nil {
				t.Fatalf("root(%s): %v", c.doc, err)
			}
			res := v.Assess(root)
			if err := w.end(root); err != nil {
				t.Fatalf("end(%s): %v", c.doc, err)
			}
			if len(w.notes) != c.notes || len(res.Violations()) != c.charges {
				t.Errorf("%s keeps %d notes beside %d charges, want %d beside %d", c.doc, len(w.notes), len(res.Violations()), c.notes, c.charges)
			}
		})
	}
}
