package codec

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// The xs:anyType schemas, each declaring s:r. pairAnySchema's a has no type
// (§3.3.2.1 dcl.elt.common, item 4 of {type definition}) and its b
// type="xs:anyType"; refAnySchema's particle is an <element ref> to f, a
// top-level declaration of no type; occursAnySchema's a repeats and its b is
// optional; pickAnySchema's and pickRefAnySchema's choices hold a local and
// a ref alternative of xs:anyType beside b, an xs:string; wildAnySchema's
// choice an element named any of xs:anyType beside a wildcard; groupAnySchema's
// ref names h, an abstract head of xs:anyType whose members are m, taking its
// head's type (dcl.elt.common item 3), and n, an xs:string; bareNamedSchema's
// choice a local a and a ref to A, both of xs:anyType; defaultAnySchema's and
// fixedAnySchema's a carry a value constraint; and oneAnySchema's a is the
// one particle; derivedAnySchema's a is of an element-only restriction of
// xs:anyType, which is no xs:anyType itself (codegen/doc.go "# xs:anyType").
const (
	pairAnySchema    = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a"/><xs:element name="b" type="xs:anyType"/></xs:sequence></xs:complexType></xs:element>`
	refAnySchema     = `<xs:element name="f"/><xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:f"/></xs:sequence></xs:complexType></xs:element>`
	occursAnySchema  = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" maxOccurs="unbounded"/><xs:element name="b" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`
	pickAnySchema    = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a"/><xs:element name="b" type="xs:string"/></xs:choice></xs:complexType></xs:element>`
	pickRefAnySchema = `<xs:element name="f"/><xs:element name="r"><xs:complexType><xs:choice><xs:element ref="s:f"/><xs:element name="b" type="xs:string"/></xs:choice></xs:complexType></xs:element>`
	wildAnySchema    = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="any"/><xs:any namespace="##other" processContents="lax"/></xs:choice></xs:complexType></xs:element>`
	groupAnySchema   = `<xs:element name="h" abstract="true"/><xs:element name="m" substitutionGroup="s:h"/><xs:element name="n" substitutionGroup="s:h" type="xs:string"/>` +
		`<xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:h"/></xs:sequence></xs:complexType></xs:element>`
	bareNamedSchema  = `<xs:element name="A"/><xs:element name="r"><xs:complexType><xs:choice><xs:element name="a"/><xs:element ref="s:A"/></xs:choice></xs:complexType></xs:element>`
	defaultAnySchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" default="d"/></xs:sequence></xs:complexType></xs:element>`
	fixedAnySchema   = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" fixed="d"/></xs:sequence></xs:complexType></xs:element>`
	oneAnySchema     = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a"/></xs:sequence></xs:complexType></xs:element>`
	derivedAnySchema = `<xs:complexType name="dt"><xs:complexContent><xs:restriction base="xs:anyType"><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType>` +
		`<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="s:dt"/></xs:sequence></xs:complexType></xs:element>`
)

// The nillable xs:anyType schemas, each declaring s:r: nilOneAnySchema's a,
// nilRefAnySchema's f, nilPickAnySchema's choice alternative a and
// nilGroupAnySchema's member m are nillable declarations of xs:anyType,
// shaped as oneAnySchema's, refAnySchema's, pickAnySchema's and
// groupAnySchema's are, so they decode into the same types.
const (
	nilOneAnySchema   = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" nillable="true"/></xs:sequence></xs:complexType></xs:element>`
	nilRefAnySchema   = `<xs:element name="f" nillable="true"/><xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:f"/></xs:sequence></xs:complexType></xs:element>`
	nilPickAnySchema  = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a" nillable="true"/><xs:element name="b" type="xs:string"/></xs:choice></xs:complexType></xs:element>`
	nilGroupAnySchema = `<xs:element name="h" abstract="true"/><xs:element name="m" substitutionGroup="s:h" nillable="true"/><xs:element name="n" substitutionGroup="s:h" type="xs:string"/>` +
		`<xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:h"/></xs:sequence></xs:complexType></xs:element>`
)

// The schemas whose document element s:r is of type xs:anyType itself:
// rootAnySchema's of no type, typedRootAnySchema's type="xs:anyType",
// nilRootAnySchema's nillable, and defaultRootAnySchema's and
// fixedRootAnySchema's with a value constraint. Codegen emits rootAny, its R,
// for each.
const (
	rootAnySchema        = `<xs:element name="r"/>`
	typedRootAnySchema   = `<xs:element name="r" type="xs:anyType"/>`
	nilRootAnySchema     = `<xs:element name="r" nillable="true"/>`
	defaultRootAnySchema = `<xs:element name="r" default="d"/>`
	fixedRootAnySchema   = `<xs:element name="r" fixed="d"/>`
)

// rootAny is codegen's R for a document element of type xs:anyType.
type rootAny anyType

// The types codegen emits for those schemas' s:r, each named here by its
// schema, which TestAnyTypeTypesAreCodegens pins; free, member and capA are
// codegen's F, M and A, each a named type over AnyType.
type (
	pairAny   struct{ A, B anyType }
	free      anyType
	refAny    struct{ F free }
	occursAny struct {
		A []anyType
		B *anyType
	}
	oneAny     struct{ A anyType }
	derivedAny struct{ A struct{ B string } }

	pickAny      interface{ isPickAny() }
	pickAnyA     struct{ A anyType }
	pickAnyB     struct{ B string }
	pickRefAny   interface{ isPickRefAny() }
	pickRefAnyF  struct{ F free }
	pickRefAnyB  struct{ B string }
	wildAny      interface{ isWildAny() }
	wildAnyAny   struct{ Any anyType }
	wildAnyAny2  struct{ Any anyElement }
	groupAny     struct{ H groupAnyH }
	groupAnyH    interface{ isGroupAnyH() }
	member       anyType
	memberString string
	groupAnyM    struct{ M member }
	groupAnyN    struct{ N memberString }
	capA         anyType
	bareNamed    interface{ isBareNamed() }
	bareNamedA   struct{ A anyType }
	bareNamedA2  struct{ A capA }
)

func (pickAnyA) isPickAny()       {}
func (pickAnyB) isPickAny()       {}
func (pickRefAnyF) isPickRefAny() {}
func (pickRefAnyB) isPickRefAny() {}
func (wildAnyAny) isWildAny()     {}
func (wildAnyAny2) isWildAny()    {}
func (groupAnyM) isGroupAnyH()    {}
func (groupAnyN) isGroupAnyH()    {}
func (bareNamedA) isBareNamed()   {}
func (bareNamedA2) isBareNamed()  {}

// The branch sets of those schemas' interfaces.
var (
	pickAnyBranches    = WithBranches[pickAny](pickAnyA{}, pickAnyB{})
	pickRefAnyBranches = WithBranches[pickRefAny](pickRefAnyF{}, pickRefAnyB{})
	wildAnyBranches    = WithBranches[wildAny](wildAnyAny{}, wildAnyAny2{})
	groupAnyBranches   = WithBranches[groupAnyH](groupAnyM{}, groupAnyN{})
	bareNamedBranches  = WithBranches[bareNamed](bareNamedA{}, bareNamedA2{})
)

// TestAnyTypeTypesAreCodegens pins the types above to those codegen emits for
// each schema, in shape, so that the rows decoding into them decode into
// codegen's types; the raw-content structs are TestWildcardTypesAreCodegens'.
func TestAnyTypeTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{pairAnySchema, []string{"type R struct {\n\tA AnyType\n\tB AnyType\n}\n", "type AnyType struct {\n"}},
		{refAnySchema, []string{"type F AnyType\n", "type R struct {\n\tF F\n}\n"}},
		{occursAnySchema, []string{"type R struct {\n\tA []AnyType\n\tB *AnyType\n}\n"}},
		{pickAnySchema, []string{
			"type RA struct {\n\tA AnyType\n}\n\nfunc (RA) isR() {}\n",
			"type RB struct {\n\tB string\n}\n\nfunc (RB) isR() {}\n",
		}},
		{pickRefAnySchema, []string{
			"type F AnyType\n",
			"type RF struct {\n\tF F\n}\n\nfunc (RF) isR() {}\n",
			"type RB struct {\n\tB string\n}\n\nfunc (RB) isR() {}\n",
		}},
		{wildAnySchema, []string{
			"type RAny struct {\n\tAny AnyType\n}\n\nfunc (RAny) isR() {}\n",
			"type RAny2 struct {\n\tAny AnyElement\n}\n\nfunc (RAny2) isR() {}\n",
		}},
		{groupAnySchema, []string{
			"type M AnyType\n",
			"type N string\n",
			"type R struct {\n\tH HSubstitutionGroup\n}\n",
			"type HSubstitutionGroupM struct {\n\tM M\n}\n",
			"type HSubstitutionGroupN struct {\n\tN N\n}\n",
		}},
		{bareNamedSchema, []string{
			"type A AnyType\n",
			"type RA struct {\n\tA AnyType\n}\n\nfunc (RA) isR() {}\n",
			"type RA2 struct {\n\tA A\n}\n\nfunc (RA2) isR() {}\n",
		}},
		{defaultAnySchema, []string{"type R struct {\n\tA AnyType\n}\n"}},
		{fixedAnySchema, []string{"type R struct {\n\tA AnyType\n}\n"}},
		{oneAnySchema, []string{"type R struct {\n\tA AnyType\n}\n"}},
		{derivedAnySchema, []string{"type R struct {\n\tA Dt\n}\n", "type Dt struct {\n\tB string\n}\n"}},
		{nilOneAnySchema, []string{"type R struct {\n\tA AnyType\n}\n"}},
		{nilRefAnySchema, []string{"type F AnyType\n", "type R struct {\n\tF F\n}\n"}},
		{nilPickAnySchema, []string{
			"type RA struct {\n\tA AnyType\n}\n\nfunc (RA) isR() {}\n",
			"type RB struct {\n\tB string\n}\n\nfunc (RB) isR() {}\n",
		}},
		{nilGroupAnySchema, []string{
			"type M AnyType\n",
			"type R struct {\n\tH HSubstitutionGroup\n}\n",
			"type HSubstitutionGroupM struct {\n\tM M\n}\n",
			"type HSubstitutionGroupN struct {\n\tN N\n}\n",
		}},
		{rootAnySchema, []string{"type R AnyType\n"}},
		{typedRootAnySchema, []string{"type R AnyType\n"}},
		{nilRootAnySchema, []string{"type R AnyType\n"}},
		{defaultRootAnySchema, []string{"type R AnyType\n"}},
		{fixedRootAnySchema, []string{"type R AnyType\n"}},
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

// TestDecodeAnyType pins the xs:anyType lift: a particle's element
// declaration of type xs:anyType itself, local, of no type or
// type="xs:anyType", or an <element ref>'s, a choice's alternative and a
// substitution group's member included, decodes into the field codegen emits
// for it, AnyType or the named type over it by its occurrence, the element's
// attributes, text and children held raw as a wildcard's match's are,
// namespace declarations not represented. An xsi:schemaLocation or
// xsi:noNamespaceSchemaLocation on it, and an xsi:type inside it, are raw
// attributes, the xsi:type's value unresolved. A value constraint supplies
// nothing: an empty element's Text is "" (§3.3.5.4 sic-eltType, case 2). A
// restriction of xs:anyType decodes as its own complex type. A nillable one,
// local, referenced, a choice's alternative or a substitution group's member,
// decodes into the same unwrapped field, its xsi:nil, true or false, one of
// its raw attributes.
func TestDecodeAnyType(t *testing.T) {
	const xsi = "http://www.w3.org/2001/XMLSchema-instance"
	text := func(s string) anyType { return anyType{Text: s} }
	raw := anyType{
		Attributes: []anyAttribute{{Local: "k", Value: "1"}, {Space: "urn:o", Local: "m", Value: " 2 3"}},
		Text:       "lead",
		Children: []anyChild{
			{Element: anyElement{Space: "urn:o", Local: "c", Content: anyType{
				Attributes: []anyAttribute{{Local: "j", Value: "x"}},
				Text:       "in",
				Children:   []anyChild{{Element: anyElement{Local: "d"}}},
			}}, Tail: "mid"},
			{Element: anyElement{Local: "e"}, Tail: " end "},
		},
	}
	const rawBody = ` xmlns:o="urn:o" k="1" o:m=" 2 3"><!--c-->le<!--c-->ad<o:c j="x">in<d/></o:c>mid<e/> end `
	nilled := anyType{Attributes: []anyAttribute{{Space: xsi, Local: "nil", Value: "true"}}}
	rows := []struct {
		name  string
		decls string
		body  string
		into  target
		want  any
	}{
		{"a local element of no type and one of type xs:anyType, read raw", pairAnySchema, `<s:a` + rawBody + `</s:a><s:b>t</s:b>`, into[pairAny](), pairAny{A: raw, B: text("t")}},
		{"an element ref to a declaration of no type", refAnySchema, `<s:f` + rawBody + `</s:f>`, into[refAny](), refAny{F: free(raw)}},
		{"repeated and optional, the optional absent", occursAnySchema, `<s:a>1</s:a><s:a/><s:a>3</s:a>`, into[occursAny](), occursAny{A: []anyType{text("1"), {}, text("3")}}},
		{"repeated and optional, the optional present", occursAnySchema, `<s:a/><s:b>2</s:b>`, into[occursAny](), occursAny{A: []anyType{{}}, B: &anyType{Text: "2"}}},
		{"a choice's local alternative", pickAnySchema, `<s:a` + rawBody + `</s:a>`, into[pickAny](pickAnyBranches), pickAny(pickAnyA{A: raw})},
		{"a choice's string alternative beside it", pickAnySchema, `<s:b>v</s:b>`, into[pickAny](pickAnyBranches), pickAny(pickAnyB{B: "v"})},
		{"a choice's ref alternative", pickRefAnySchema, `<s:f>v</s:f>`, into[pickRefAny](pickRefAnyBranches), pickRefAny(pickRefAnyF{F: free(text("v"))})},
		{"an element any of xs:anyType beside a wildcard, the element's", wildAnySchema, `<s:any>v</s:any>`, into[wildAny](wildAnyBranches), wildAny(wildAnyAny{Any: text("v")})},
		{"an element any of xs:anyType beside a wildcard, the wildcard's", wildAnySchema, `<o:any xmlns:o="urn:o">v</o:any>`, into[wildAny](wildAnyBranches),
			wildAny(wildAnyAny2{Any: anyElement{Space: "urn:o", Local: "any", Content: text("v")}})},
		{"a substitution group's member of its head's xs:anyType", groupAnySchema, `<s:m k="1">t</s:m>`, into[groupAny](groupAnyBranches),
			groupAny{H: groupAnyM{M: member{Attributes: []anyAttribute{{Local: "k", Value: "1"}}, Text: "t"}}}},
		{"a local alternative beside a ref one, the local", bareNamedSchema, `<s:a>l</s:a>`, into[bareNamed](bareNamedBranches), bareNamed(bareNamedA{A: text("l")})},
		{"a local alternative beside a ref one, the ref", bareNamedSchema, `<s:A>n</s:A>`, into[bareNamed](bareNamedBranches), bareNamed(bareNamedA2{A: capA(text("n"))})},
		{"xsi:schemaLocation on the element, in start-tag order", oneAnySchema, `<s:a xsi:schemaLocation="urn:x x.xsd" k="1"/>`, into[oneAny](),
			oneAny{A: anyType{Attributes: []anyAttribute{{Space: xsi, Local: "schemaLocation", Value: "urn:x x.xsd"}, {Local: "k", Value: "1"}}}}},
		{"xsi:noNamespaceSchemaLocation on the element", oneAnySchema, `<s:a k="1" xsi:noNamespaceSchemaLocation="x.xsd"/>`, into[oneAny](),
			oneAny{A: anyType{Attributes: []anyAttribute{{Local: "k", Value: "1"}, {Space: xsi, Local: "noNamespaceSchemaLocation", Value: "x.xsd"}}}}},
		{"xsi:type on a child inside it, unresolved", oneAnySchema, `<s:a><c xsi:type="xs:int">1</c></s:a>`, into[oneAny](),
			oneAny{A: anyType{Children: []anyChild{{Element: anyElement{Local: "c", Content: anyType{
				Attributes: []anyAttribute{{Space: xsi, Local: "type", Value: "xs:int"}}, Text: "1"}}}}}}},
		{"an empty element with a default", defaultAnySchema, `<s:a/>`, into[oneAny](), oneAny{}},
		{"an empty element with a fixed value", fixedAnySchema, `<s:a/>`, into[oneAny](), oneAny{}},
		{"an element with a fixed value, holding it", fixedAnySchema, `<s:a>d</s:a>`, into[oneAny](), oneAny{A: text("d")}},
		{"a restriction of xs:anyType, its own struct", derivedAnySchema, `<s:a><s:b>x</s:b></s:a>`, into[derivedAny](), derivedAny{A: struct{ B string }{B: "x"}}},
		{"a nillable local element, nilled", nilOneAnySchema, `<s:a xsi:nil="true"/>`, into[oneAny](), oneAny{A: nilled}},
		{"a nillable local element, xsi:nil false", nilOneAnySchema, `<s:a xsi:nil="false">t</s:a>`, into[oneAny](),
			oneAny{A: anyType{Attributes: []anyAttribute{{Space: xsi, Local: "nil", Value: "false"}}, Text: "t"}}},
		{"a nillable local element, not nilled", nilOneAnySchema, `<s:a` + rawBody + `</s:a>`, into[oneAny](), oneAny{A: raw}},
		{"an element ref to a nillable declaration of no type, nilled", nilRefAnySchema, `<s:f xsi:nil="true"/>`, into[refAny](), refAny{F: free(nilled)}},
		{"a choice's nillable alternative, nilled", nilPickAnySchema, `<s:a xsi:nil="true"/>`, into[pickAny](pickAnyBranches), pickAny(pickAnyA{A: nilled})},
		{"a nillable substitution group's member, nilled", nilGroupAnySchema, `<s:m xsi:nil="true"/>`, into[groupAny](groupAnyBranches), groupAny{H: groupAnyM{M: member(nilled)}}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf("", row.body)
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

// TestDecodeAnyTypeRoot pins the document element of type xs:anyType itself,
// of no type or type="xs:anyType", nillable or not: it decodes into
// codegen's R, a named type over AnyType, and into AnyType itself, item 6
// matching T by shape alone, its attributes in start-tag order, xsi: ones
// included and namespace declarations not, its text and its children with
// their Tails held raw. A value constraint supplies nothing, so an empty
// one's Text is "".
func TestDecodeAnyTypeRoot(t *testing.T) {
	const xsi = "http://www.w3.org/2001/XMLSchema-instance"
	const body = `le<!--c-->ad<o:c xmlns:o="urn:o" j="x">in<d/></o:c>mid<e/> end `
	content := anyType{
		Attributes: []anyAttribute{{Local: "k", Value: "1"}, {Space: xsi, Local: "schemaLocation", Value: "urn:x x.xsd"}, {Local: "j", Value: "2"}},
		Text:       "lead",
		Children: []anyChild{
			{Element: anyElement{Space: "urn:o", Local: "c", Content: anyType{
				Attributes: []anyAttribute{{Local: "j", Value: "x"}},
				Text:       "in",
				Children:   []anyChild{{Element: anyElement{Local: "d"}}},
			}}, Tail: "mid"},
			{Element: anyElement{Local: "e"}, Tail: " end "},
		},
	}
	const attrs = ` k="1" xsi:schemaLocation="urn:x x.xsd" j="2"`
	nilAttr := func(v string) anyType {
		return anyType{Attributes: []anyAttribute{{Space: xsi, Local: "nil", Value: v}}}
	}
	rows := []struct {
		name        string
		decls       string
		attrs, body string
		into        target
		want        any
	}{
		{"of no type", rootAnySchema, attrs, body, into[rootAny](), rootAny(content)},
		{"of no type, into AnyType itself", rootAnySchema, attrs, body, into[anyType](), content},
		{"of type xs:anyType", typedRootAnySchema, attrs, body, into[rootAny](), rootAny(content)},
		{"empty", rootAnySchema, "", "", into[rootAny](), rootAny{}},
		{"nillable, nilled", nilRootAnySchema, ` xsi:nil="true"`, "", into[rootAny](), rootAny(nilAttr("true"))},
		{"nillable, xsi:nil false", nilRootAnySchema, ` xsi:nil="false"`, "t", into[rootAny](), rootAny(anyType{Attributes: nilAttr("false").Attributes, Text: "t"})},
		{"nillable, not nilled", nilRootAnySchema, attrs, body, into[rootAny](), rootAny(content)},
		{"an empty one with a default", defaultRootAnySchema, "", "", into[rootAny](), rootAny{}},
		{"an empty one with a fixed value", fixedRootAnySchema, "", "", into[rootAny](), rootAny{}},
		{"one with a fixed value, holding it", fixedRootAnySchema, "", "d", into[rootAny](), rootAny{Text: "d"}},
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

// TestDecodeAnyTypeRootXsiType pins that an xsi:type on a document element of
// type xs:anyType declines as xsi:type at the document element's instance
// Loc (item 7 of Decode's doc), past item 5, which it passes.
func TestDecodeAnyTypeRootXsiType(t *testing.T) {
	for _, decls := range []string{rootAnySchema, nilRootAnySchema} {
		schema := schemaOf(t, decls)
		data := instanceOf(` xsi:type="xs:string"`, "x")
		mustAccept(t, schema, data)
		got, err := Decode[rootAny](schema, strict.New(), data)
		if got != nil {
			t.Fatalf("Decode = %v, want no value", got)
		}
		assertDecline(t, err, constructXsiType, 1, 1)
		if err := CheckDecode[rootAny](schema, rootName); err != nil {
			t.Errorf("CheckDecode = %v, want nil, the decline being the instance's", err)
		}
	}
}

// TestDecodeAnyTypeXsiType pins that an xsi:type on the element of type
// xs:anyType itself, not inside its raw content, still declines as xsi:type
// at its own instance Loc (item 7 of Decode's doc).
func TestDecodeAnyTypeXsiType(t *testing.T) {
	schema := schemaOf(t, oneAnySchema)
	data := instanceOf("", `<s:a xsi:type="xs:string">x</s:a>`)
	mustAccept(t, schema, data)
	got, err := Decode[oneAny](schema, strict.New(), data)
	if got != nil {
		t.Fatalf("Decode = %v, want no value", got)
	}
	assertDecline(t, err, constructXsiType, 1, bytes.Index(data, []byte(`<s:a`))+1)
}

// TestDecodeAnyTypeMismatch pins item 6 of Decode's doc for a field of type
// xs:anyType: a local one's field holds X alone, so a named type over it
// mismatches, and a ref's N alone, so X mismatches; a struct of AnyElement's
// shape, a wildcard's E, mismatches either; each names the field, Decode and
// CheckDecode alike.
func TestDecodeAnyTypeMismatch(t *testing.T) {
	rows := []struct {
		name  string
		decls string
		body  string
		into  target
		want  string
	}{
		{"a local field of a named type over X", oneAnySchema, `<s:a/>`, into[struct{ A free }](), "field 0 of struct { A codec.free } is A codec.free where codegen emits A X for a struct X of codegen's AnyType shape"},
		{"a local field of E", oneAnySchema, `<s:a/>`, into[struct{ A anyElement }](), "field 0 of struct { A codec.anyElement } is A codec.anyElement where codegen emits A X for a struct X of codegen's AnyType shape"},
		{"a ref's field of X", refAnySchema, `<s:f/>`, into[struct{ F anyType }](), "field 0 of struct { F codec.anyType } is F codec.anyType where codegen emits F N for a named type N over a struct of AnyType's shape"},
		{"a repeated field of X, not []X", occursAnySchema, `<s:a/>`, into[struct {
			A anyType
			B *anyType
		}](), "field 0 of struct { A codec.anyType; B *codec.anyType } is A codec.anyType where codegen emits A []X for a struct X of codegen's AnyType shape"},
		{"a document element's T of E", rootAnySchema, ``, into[anyElement](), "codec.anyElement is neither a struct X of codegen's AnyType shape nor a named type over one"},
		{"a document element's T of no field", rootAnySchema, ``, into[struct{}](), "struct {} is neither a struct X of codegen's AnyType shape nor a named type over one"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf("", row.body)
			mustAccept(t, schema, data)
			_, err := row.into.decode(schema, data)
			if want := ErrTypeMismatch.Error() + ": " + row.want; err == nil || err.Error() != want {
				t.Fatalf("Decode = %v, want %q", err, want)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}

// TestAnyTypeSharesBranch pins field.sharesBranch's rows for an alternative
// of type xs:anyType, read through CheckDecode into struct{}, which item 5's
// shared-branch decline precedes and which mismatches the bare interface
// every one of these choices otherwise is: a pair sharing a branch declines,
// and any other pair passes item 5.
func TestAnyTypeSharesBranch(t *testing.T) {
	const cx = `<xs:complexType name="Cx"><xs:choice><xs:element name="p" type="xs:string"/><xs:element name="q" type="xs:int"/></xs:choice></xs:complexType>`
	rows := []struct {
		name   string
		decls  string
		shares bool
	}{
		{"two local alternatives, a and A", root(`<xs:choice><xs:element name="a"/><xs:element name="A"/></xs:choice>`), true},
		{"two ref alternatives, f and F", `<xs:element name="f"/><xs:element name="F"/>` + root(`<xs:choice><xs:element ref="s:f"/><xs:element ref="s:F"/></xs:choice>`), true},
		{"a local and a ref alternative", bareNamedSchema, false},
		{"one alternative and a repeated one", root(`<xs:choice><xs:element name="a"/><xs:element name="A" maxOccurs="unbounded"/></xs:choice>`), false},
		{"beside a wildcard", wildAnySchema, false},
		{"beside codegen's struct", root(`<xs:choice><xs:element name="v"/>` + vStruct("V", "") + `</xs:choice>`), true},
		{"beside codegen's struct, the struct first", root(`<xs:choice>` + vStruct("v", "") + `<xs:element name="V"/></xs:choice>`), true},
		{"beside an xs:date", root(`<xs:choice><xs:element name="a"/><xs:element name="A" type="xs:date"/></xs:choice>`), true},
		{"beside an xs:string", root(`<xs:choice><xs:element name="a"/><xs:element name="A" type="xs:string"/></xs:choice>`), false},
		{"beside a bare sealed interface", cx + root(`<xs:choice><xs:element name="x"/><xs:element name="X" type="s:Cx"/></xs:choice>`), false},
		{"beside a substitution group's interface", `<xs:element name="X" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:X"/>` +
			root(`<xs:choice><xs:element name="x"/><xs:element ref="s:X"/></xs:choice>`), false},
		{"two members of one group, a and A", `<xs:element name="h" abstract="true"/><xs:element name="a" substitutionGroup="s:h"/><xs:element name="A" substitutionGroup="s:h"/>` +
			sequenceOf(`<xs:element ref="s:h"/>`), true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := CheckDecode[struct{}](schemaOf(t, row.decls), rootName)
			var de *DeclineError
			declined := errors.As(err, &de)
			if declined && de.Construct() != (Construct{constructSharedBranch}) {
				t.Fatalf("CheckDecode = %v, want no decline but the %s one", err, constructSharedBranch.name())
			}
			if declined != row.shares {
				t.Errorf("CheckDecode = %v, want the %s decline: %t", err, constructSharedBranch.name(), row.shares)
			}
			if !declined && !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("CheckDecode = %v, want ErrTypeMismatch past item 5", err)
			}
		})
	}
}
