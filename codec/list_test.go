package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// listSchema declares s:r of one local element per shape a list field takes
// and an item of each kind: strs, a list of xs:string ([]string); uris, an
// optional list of xs:anyURI (*[]string); ints, a repeated s:ints, a named
// list of xs:int ([][]int32); bools, a list of xs:boolean ([]bool); res, of
// s:ints3, a restriction of s:ints taking its item (Structures §3.16.2.3);
// and the attributes at, an optional list of xs:string, and dflt, of s:ints
// with a default.
const listSchema = `<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType>
<xs:simpleType name="ints3"><xs:restriction base="s:ints"><xs:maxLength value="3"/></xs:restriction></xs:simpleType>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="strs"><xs:simpleType><xs:list itemType="xs:string"/></xs:simpleType></xs:element>
<xs:element name="uris" minOccurs="0"><xs:simpleType><xs:list itemType="xs:anyURI"/></xs:simpleType></xs:element>
<xs:element name="ints" type="s:ints" minOccurs="0" maxOccurs="unbounded"/>
<xs:element name="bools"><xs:simpleType><xs:list itemType="xs:boolean"/></xs:simpleType></xs:element>
<xs:element name="res" type="s:ints3"/>
</xs:sequence>
<xs:attribute name="at"><xs:simpleType><xs:list itemType="xs:string"/></xs:simpleType></xs:attribute>
<xs:attribute name="dflt" type="s:ints" default="4 5"/>
</xs:complexType></xs:element>`

// The list schemas of one construct each: listRootSchema's s:r is a simple
// root of a list of xs:string, listRestrictionRootSchema's one of a
// restriction of a named list of xs:int, listRefSchema's s:r holds a repeated
// <element ref> to h, of that list, and listChoiceSchema's choice holds x, of
// xs:int, and X, a list of xs:int, one Go field name over two Go types;
// listRefChoiceSchema's choice holds a repeated ref to a, of xs:int, and a ref
// to A, of a list of xs:int: []N over int32 beside N over []int32.
const (
	listRootSchema            = `<xs:element name="r"><xs:simpleType><xs:list itemType="xs:string"/></xs:simpleType></xs:element>`
	listRestrictionRootSchema = `<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType><xs:element name="r"><xs:simpleType><xs:restriction base="s:ints"><xs:minLength value="1"/></xs:restriction></xs:simpleType></xs:element>`
	listRefSchema             = `<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType><xs:element name="h" type="s:ints"/>
<xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:h" maxOccurs="2"/></xs:sequence></xs:complexType></xs:element>`
	listChoiceSchema    = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="x" type="xs:int"/><xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element></xs:choice></xs:complexType></xs:element>`
	listRefChoiceSchema = `<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType><xs:element name="a" type="xs:int"/><xs:element name="A" type="s:ints"/>
<xs:element name="r"><xs:complexType><xs:choice><xs:element ref="s:a" maxOccurs="2"/><xs:element ref="s:A"/></xs:choice></xs:complexType></xs:element>`
)

// lists to listChoiceX2 are the types codegen emits for the list schemas:
// listSchema's R, listRootSchema's R, listRestrictionRootSchema's R,
// listRefSchema's H and R, listChoiceSchema's R, RX and RX2, and
// listRefChoiceSchema's R, RA, RA2, A and A2, which TestListTypesAreCodegens
// pins.
type (
	lists struct {
		Strs  []string
		Uris  *[]string
		Ints  [][]int32
		Bools []bool
		Res   []int32
		At    *[]string
		Dflt  []int32
	}
	listRoot       []string
	listInts       []int32
	listHead       []int32
	listHolder     struct{ H []listHead }
	listChoice     interface{ isListChoice() }
	listChoiceX    struct{ X int32 }
	listChoiceX2   struct{ X []int32 }
	listRefChoice  interface{ isListRefChoice() }
	listRefChoiceA struct{ A []listRefA }
	listRefChoiceB struct{ A listRefB }
	listRefA       int32
	listRefB       []int32
)

func (listChoiceX) isListChoice()       {}
func (listChoiceX2) isListChoice()      {}
func (listRefChoiceA) isListRefChoice() {}
func (listRefChoiceB) isListRefChoice() {}

var (
	listChoiceBranches    = WithBranches[listChoice](listChoiceX{}, listChoiceX2{})
	listRefChoiceBranches = WithBranches[listRefChoice](listRefChoiceA{}, listRefChoiceB{})
)

// TestListTypesAreCodegens pins the types above to those codegen emits for
// each list schema, in shape: a list's Go type is a slice of its item's
// (codegen/doc.go "# Simple types"), under each field shape.
func TestListTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{listSchema, []string{"type R struct {\n\tStrs  []string\n\tUris  *[]string\n\tInts  [][]int32\n\tBools []bool\n\tRes   []int32\n\tAt    *[]string\n\tDflt  []int32\n}\n"}},
		{listRootSchema, []string{"type R []string\n"}},
		{listRestrictionRootSchema, []string{"type R []int32\n"}},
		{listRefSchema, []string{"type H []int32\n", "type R struct {\n\tH []H\n}\n"}},
		{listChoiceSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX int32\n}\n\nfunc (RX) isR() {}\n",
			"type RX2 struct {\n\tX []int32\n}\n\nfunc (RX2) isR() {}\n",
		}},
		{listRefChoiceSchema, []string{
			"type A int32\n",
			"type A2 []int32\n",
			"type RA struct {\n\tA []A\n}\n\nfunc (RA) isR() {}\n",
			"type RA2 struct {\n\tA A2\n}\n\nfunc (RA2) isR() {}\n",
		}},
	}
	for _, row := range rows {
		gen := generated(t, schemaOf(t, row.decls))
		for _, decl := range row.want {
			if !bytes.Contains(gen, []byte(decl)) {
				t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
			}
		}
	}
}

// TestDecodeLists pins the list lift: a list of an item Decode decodes
// decodes into codegen's slice of the item's Go type, its value collapsed
// (Datatypes §4.3.6 rf-whiteSpace, §2.4.1.2) and split at U+0020 alone, so a
// no-break space stays inside its item, before each item is mapped under the
// item type (cvc-datatype-valid clause 2.2): an xs:string, xs:anyURI, xs:int
// and xs:boolean item, a restriction of a list taking its base's item
// (Structures §3.16.2.3), in each field shape, an attribute's, a supplied
// default's, a simple root's into a named slice and an unnamed one, the
// builtin xs:NMTOKENS's, an <element ref>'s into the head's named slice, a
// choice branch's beside an xs:int alternative of its Go field name, and a
// ref branch's beside a repeated ref branch over xs:int of its Go field
// name. CheckDecode returns nil for each.
func TestDecodeLists(t *testing.T) {
	strs := func(s ...string) *[]string { return &s }
	rows := []struct {
		name  string
		decls string
		attrs string
		body  string
		into  target
		want  any
	}{
		{"every item kind and field shape", listSchema, ` at=" x  y " dflt="6"`,
			"<s:strs>  a\n b  </s:strs><s:uris> http://e.x/a \t b </s:uris><s:ints>1 2</s:ints><s:ints> 3 </s:ints><s:bools>true 0 1 false</s:bools><s:res> 7\n\t8 </s:res>", into[lists](),
			lists{Strs: []string{"a", "b"}, Uris: strs("http://e.x/a", "b"), Ints: [][]int32{{1, 2}, {3}}, Bools: []bool{true, false, true, false}, Res: []int32{7, 8}, At: strs("x", "y"), Dflt: []int32{6}}},
		{"a no-break space inside an item, optional lists absent, the default supplied", listSchema, "",
			"<s:strs>a b c</s:strs><s:bools>1</s:bools><s:res>-1</s:res>", into[lists](),
			lists{Strs: []string{"a b", "c"}, Bools: []bool{true}, Res: []int32{-1}, Dflt: []int32{4, 5}}},
		{"a simple root, into a named slice", listRootSchema, "", "  x \n y  ", into[listRoot](), listRoot{"x", "y"}},
		{"a simple root, into an unnamed slice", listRootSchema, "", "x", into[[]string](), []string{"x"}},
		{"a simple root of xs:NMTOKENS", `<xs:element name="r" type="xs:NMTOKENS"/>`, "", " a:1  b ", into[[]string](), []string{"a:1", "b"}},
		{"a simple root of a restriction of a list", listRestrictionRootSchema, "", "1 2 3", into[listInts](), listInts{1, 2, 3}},
		{"a repeated <element ref> to a head of a list type", listRefSchema, "", "<s:h>1 2</s:h><s:h> 3 </s:h>", into[listHolder](), listHolder{H: []listHead{{1, 2}, {3}}}},
		{"a choice's list alternative", listChoiceSchema, "", "<s:X> 1  2 </s:X>", into[listChoice](listChoiceBranches), listChoice(listChoiceX2{X: []int32{1, 2}})},
		{"a choice's xs:int alternative beside it", listChoiceSchema, "", "<s:x> 3 </s:x>", into[listChoice](listChoiceBranches), listChoice(listChoiceX{X: 3})},
		{"a choice's repeated ref over xs:int", listRefChoiceSchema, "", "<s:a>1</s:a><s:a>2</s:a>", into[listRefChoice](listRefChoiceBranches), listRefChoice(listRefChoiceA{A: []listRefA{1, 2}})},
		{"a choice's ref over a list of xs:int beside it", listRefChoiceSchema, "", "<s:A>1 2</s:A>", into[listRefChoice](listRefChoiceBranches), listRefChoice(listRefChoiceB{A: listRefB{1, 2}})},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf(row.attrs, row.body)
			mustAccept(t, schema, data)
			got, err := row.into.decode(schema, data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, row.want) {
				t.Errorf("Decode = %#v, want %#v", got, row.want)
			}
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil for a row Decode decodes", err)
			}
		})
	}
}

// TestDecodeEmptyLists pins the empty list (Datatypes §4.1.1 dt-list, a value
// of no items): empty and whitespace-only content and attribute values decode
// into a slice of length 0; a present optional field, element or attribute,
// holds a non-nil pointer to one, and an absent one nil; a present attribute's
// empty value is its own, never the default; and an absent repeated element
// is never appended to.
func TestDecodeEmptyLists(t *testing.T) {
	schema := schemaOf(t, listSchema)
	present := instanceOf(` at="" dflt=" "`, "<s:strs/><s:uris></s:uris><s:bools>  </s:bools><s:res>\n</s:res>")
	mustAccept(t, schema, present)
	got, err := Decode[lists](schema, strict.New(), present)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Strs) != 0 || len(got.Bools) != 0 || len(got.Res) != 0 || len(got.Dflt) != 0 || got.Ints != nil {
		t.Errorf("Decode = %#v, want Strs, Bools, Res and Dflt of length 0 and Ints nil", *got)
	}
	if got.Uris == nil || len(*got.Uris) != 0 || got.At == nil || len(*got.At) != 0 {
		t.Errorf("Decode's Uris = %#v, At = %#v, want each a pointer to a slice of length 0", got.Uris, got.At)
	}
	absent := instanceOf("", "<s:strs/><s:bools/><s:res/>")
	mustAccept(t, schema, absent)
	got, err = Decode[lists](schema, strict.New(), absent)
	if err != nil {
		t.Fatal(err)
	}
	if got.Uris != nil || got.At != nil {
		t.Errorf("Decode's Uris = %#v, At = %#v, want nil for an absent element and attribute", got.Uris, got.At)
	}
}

// TestDecodeListTypeMismatch pins item 6 of Decode's doc for a list: a T
// whose Go type is not codegen's slice is ErrTypeMismatch, no decline, and
// nothing is filled: string for a simple root's []string, string for a
// field's []string, []int64 for a field's []int32, and the unnamed [][]int32
// for an <element ref>'s []N, N a named type over []int32.
func TestDecodeListTypeMismatch(t *testing.T) {
	type flat struct {
		Strs  string
		Uris  *[]string
		Ints  [][]int32
		Bools []bool
		Res   []int32
		At    *[]string
		Dflt  []int32
	}
	type wide struct {
		Strs  []string
		Uris  *[]string
		Ints  [][]int32
		Bools []bool
		Res   []int64
		At    *[]string
		Dflt  []int32
	}
	type unnamedRef struct{ H [][]int32 }
	data := instanceOf("", "<s:strs>a</s:strs><s:bools>1</s:bools><s:res>1</s:res>")
	rows := []struct {
		name  string
		decls string
		data  []byte
		into  target
		want  string
	}{
		{"string for a simple root's []string", listRootSchema, instanceOf("", "x"), into[string](),
			"the underlying type of string is not []string"},
		{"string for a field's []string", listSchema, data, into[flat](),
			"field 0 of codec.flat is Strs string where codegen emits Strs []string"},
		{"[]int64 for a field's []int32", listSchema, data, into[wide](),
			"field 4 of codec.wide is Res []int64 where codegen emits Res []int32"},
		{"[][]int32 for a ref's []N", listRefSchema, instanceOf("", "<s:h>1</s:h>"), into[unnamedRef](),
			"field 0 of codec.unnamedRef is H [][]int32 where codegen emits H []N for a named type N over []int32"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, row.data)
			got, err := row.into.decode(schema, row.data)
			if got != nil || err == nil {
				t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
			}
			if want := ErrTypeMismatch.Error() + ": " + row.want; err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
			var de *DeclineError
			if !errors.Is(err, ErrTypeMismatch) || errors.As(err, &de) {
				t.Errorf("error %v: want ErrTypeMismatch alone", err)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}

// noMappings is a value.Backend with no mapping at all.
type noMappings struct{}

func (noMappings) Mapping(xsd.QName) (value.Mapping, bool) { return value.Mapping{}, false }

// TestListMappingReadOnlyForItems pins that a list's value stages read the
// item type's governing mapping only when the value has an item: on a backend
// governing no type, an empty list decodes, and a list of one item is item
// 8's stage error naming that item, at position 1.
func TestListMappingReadOnlyForItems(t *testing.T) {
	schema := schemaOf(t, listSchema)
	td, ok := schema.Type(xsd.QName{Space: "urn:s", Local: "ints"})
	if !ok {
		t.Fatal("listSchema declares no type s:ints")
	}
	st := td.(*xsd.SimpleType)
	item, err := st.Item(schema)
	if err != nil {
		t.Fatal(err)
	}
	s := newScalar(simpleGo{atom: atomicInt32, item: item}, st, "the item", xsd.ValueConstraint{}, false)
	start, err := documentElement(xmltree.NewReader("", bytes.NewReader(instanceOf("", ""))))
	if err != nil {
		t.Fatal(err)
	}
	p := &pipeline{schema: schema, backend: noMappings{}, log: slog.New(slog.DiscardHandler)}
	src := contentSource{start: start}
	got, err := s.decode(p, " \n ", src)
	if err != nil || got.Len() != 0 {
		t.Errorf("decode(whitespace) = %v, %v; want a slice of length 0 and no error", got, err)
	}
	_, err = s.decode(p, " 1 ", src)
	var se *stageError
	if !errors.As(err, &se) || se.stage != stageLexicalMapping || se.fragment != "1" || se.token != 1 {
		t.Errorf("decode(1) = %v, want the lexical mapping's stage error on list item 1, \"1\"", err)
	}
}

// TestDecodeListStageErrors pins item 8 of Decode's doc for a list: the stage
// error names the failing list item and its 1-based position, and the item
// type, its QName and then the list's label, or for an anonymous item the
// list's label alone; the byte offset is the content's first character
// data's.
func TestDecodeListStageErrors(t *testing.T) {
	const tail = "; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance"
	data := instanceOf("", "<s:a> 7 01 </s:a>")
	offset := bytes.Index(data, []byte("<s:a>")) + len("<s:a>")
	rows := []struct {
		name, decls string
		want        string
	}{
		{"a named list of a named item", `<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType>` + sequenceOf(`<xs:element name="a" type="s:ints"/>`),
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on list item 2, "01", a value of {http://www.w3.org/2001/XMLSchema}int, the item type of {urn:s}ints in the character content of element {urn:s}a at byte offset %d: ?: [cvc-datatype-valid] codec's Context`, offset+1, offset)},
		{"an anonymous list of an anonymous item", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:list></xs:simpleType></xs:element>`),
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on list item 2, "01", a value of the item type of the anonymous simple type of element {urn:s}a declared at s.xsd:1:231 in the character content of element {urn:s}a at byte offset %d: ?: [cvc-datatype-valid] codec's Context`, offset+1, offset)},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := decodeErr[struct{ A []int32 }](schemaOf(t, row.decls), codecContext, data)
			var se *stageError
			if !errors.As(err, &se) || se.stage != stageLexicalMapping {
				t.Fatalf("Decode error = %v, want a stage error of the lexical mapping", err)
			}
			if want := row.want + tail; err.Error() != want {
				t.Errorf("error = %q\nwant    %q", err, want)
			}
		})
	}
}

// TestWithLoggerList pins the traces of a list value: whiteSpace once, under
// the list type, with the raw text, then the lexical mapping and the
// conversion once per item, under the item type, each with that item's input.
func TestWithLoggerList(t *testing.T) {
	schema := schemaOf(t, `<xs:simpleType name="ints"><xs:list itemType="xs:int"/></xs:simpleType>`+sequenceOf(`<xs:element name="a" type="s:ints"/>`))
	data := instanceOf("", "<s:a> 01\n2 </s:a>")
	var buf bytes.Buffer
	if _, err := Decode[struct{ A []int32 }](schema, strict.New(), data, WithLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))); err != nil {
		t.Fatal(err)
	}
	var got []traceRecord
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var tr traceRecord
		if err := json.Unmarshal(line, &tr); err != nil {
			t.Fatalf("record %s: %v", line, err)
		}
		if tr.Codec != nil {
			got = append(got, tr)
		}
	}
	const list, item = "{urn:s}ints", "{http://www.w3.org/2001/XMLSchema}int, the item type of {urn:s}ints"
	loc := fmt.Sprintf("?:1:%d", bytes.Index(data, []byte("<s:a>"))+len("<s:a>")+1)
	want := []traceRecord{
		{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": list, "input": " 01\n2 ", "loc": loc}},
		{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": item, "input": "01", "loc": loc}},
		{"conversion", map[string]string{"rule": "dt-canonical-mapping", "type": item, "input": "1", "loc": loc}},
		{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": item, "input": "2", "loc": loc}},
		{"conversion", map[string]string{"rule": "dt-canonical-mapping", "type": item, "input": "2", "loc": loc}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("codec records = %+v\nwant %+v", got, want)
	}
}

// TestDeclineErrorTextListItem pins the message of a decline by a list's {item
// type definition}: after the declining declaration, the item, its QName where
// it has one, and its schema Loc. No built-in item declines: no built-in type
// is a union, and a special type is no list's item (cos-st-restricts clause
// 2.1).
func TestDeclineErrorTextListItem(t *testing.T) {
	const construct = "a union simple type"
	const union = `<xs:union memberTypes="xs:string xs:int"/>`
	rows := []struct {
		name, decls, want string
	}{
		{"a named item", `<xs:simpleType name="u">` + union + `</xs:simpleType>` + sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list itemType="s:u"/></xs:simpleType></xs:element>`),
			"element {urn:s}a declared at s.xsd:1:313, through the list type's {item type definition} {urn:s}u, a type defined at s.xsd:1:181"},
		{"an anonymous item", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list><xs:simpleType>` + union + `</xs:simpleType></xs:list></xs:simpleType></xs:element>`),
			"element {urn:s}a declared at s.xsd:1:231, through the list type's anonymous {item type definition}, a type defined at s.xsd:1:276"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, err := Decode[struct{}](schemaOf(t, row.decls), strict.New(), instanceOf("", `<s:a>1.0</s:a>`))
			if want := "?:1:1: codec does not decode " + construct + ": " + row.want + "; a decoder refusal, not a schema or instance fault"; err == nil || err.Error() != want {
				t.Errorf("error = %v\nwant    %q", err, want)
			}
		})
	}
}
