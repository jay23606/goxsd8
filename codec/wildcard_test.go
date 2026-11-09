package codec

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// anyElement, anyType, anyAttribute and anyChild are of the shapes of
// codegen's AnyElement, AnyType, AnyAttribute and AnyChild, which
// TestWildcardTypesAreCodegens pins; their names differ from codegen's,
// which item 6 never compares.
type (
	anyElement struct {
		Space, Local string
		Content      anyType
	}
	anyType struct {
		Attributes []anyAttribute
		Text       string
		Children   []anyChild
	}
	anyAttribute struct {
		Space, Local, Value string
	}
	anyChild struct {
		Element anyElement
		Tail    string
	}
)

// The wildcard schemas, each declaring s:r. seqStrictSchema's wildcard is
// strict over the target namespace, whose s:w it declares; seqLaxSchema's
// and seqSkipSchema's follow a; seqRepeatedSchema's repeats; seqOptionalSchema
// is codegen's attribution example, sequence(a, b?, any?); seqLeadingSchema's
// optional wildcard precedes a; nestedWildSchema's wildcard is n's particle;
// pickWildSchema and manyWildSchema are codegen's choicewildcard pick and
// many, lax; nameAnySchema's choice has an element named any beside the
// wildcard, and wildDecimalSchema's a wildcard beside an element named any
// of xs:decimal; openWildSchema is codegen's wildcard fixture, whose top-level
// element anyElement makes codegen name the raw-content element AnyElement2.
// The rest hold several wildcard particles: twoWildSchema's sequence an
// element e between wildcards over urn:a and urn:b; equalWildSchema's two
// ##any wildcards whose {term}s are equal, mgF019's; moreWildSchema's
// repeated urn:b and optional urn:c wildcards after twoWildSchema's first
// two particles; and shapeWildSchema's choice a wildcard over urn:a once and
// one over urn:b repeated.
const (
	seqStrictSchema   = `<xs:element name="w" type="xs:string"/>` + `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:any namespace="##targetNamespace"/></xs:sequence></xs:complexType></xs:element>`
	seqLaxSchema      = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:any processContents="lax"/></xs:sequence></xs:complexType></xs:element>`
	seqSkipSchema     = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:any processContents="skip"/></xs:sequence></xs:complexType></xs:element>`
	seqRepeatedSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:any processContents="lax" maxOccurs="unbounded"/></xs:sequence></xs:complexType></xs:element>`
	seqOptionalSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:string" minOccurs="0"/><xs:any processContents="lax" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`
	seqLeadingSchema  = `<xs:element name="r"><xs:complexType><xs:sequence><xs:any processContents="lax" minOccurs="0"/><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`
	nestedWildSchema  = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="n"><xs:complexType><xs:sequence><xs:any processContents="lax"/></xs:sequence></xs:complexType></xs:element></xs:sequence></xs:complexType></xs:element>`
	pickWildSchema    = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a" type="xs:string"/><xs:any processContents="lax"/></xs:choice></xs:complexType></xs:element>`
	manyWildSchema    = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="b" type="xs:string"/><xs:any namespace="##other" processContents="lax" maxOccurs="2"/></xs:choice></xs:complexType></xs:element>`
	nameAnySchema     = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="any" type="xs:string"/><xs:any namespace="##other" processContents="lax"/></xs:choice></xs:complexType></xs:element>`
	wildDecimalSchema = `<xs:element name="r"><xs:complexType><xs:choice><xs:any namespace="##other" processContents="lax"/><xs:element name="any" type="xs:decimal"/></xs:choice></xs:complexType></xs:element>`
	openWildSchema    = `<xs:element name="anyElement" type="xs:string"/><xs:element name="r"><xs:complexType><xs:sequence><xs:element name="id" type="xs:string"/><xs:any minOccurs="0" maxOccurs="unbounded" namespace="##other" processContents="lax"/></xs:sequence></xs:complexType></xs:element>`
	twoWildSchema     = `<xs:element name="r"><xs:complexType><xs:sequence><xs:any namespace="urn:a" processContents="lax"/><xs:element name="e" type="xs:string"/><xs:any namespace="urn:b" processContents="lax"/></xs:sequence></xs:complexType></xs:element>`
	equalWildSchema   = `<xs:element name="r"><xs:complexType><xs:sequence><xs:any processContents="skip"/><xs:any processContents="skip"/></xs:sequence></xs:complexType></xs:element>`
	moreWildSchema    = `<xs:element name="r"><xs:complexType><xs:sequence><xs:any namespace="urn:a" processContents="lax"/><xs:element name="e" type="xs:string"/><xs:any namespace="urn:b" processContents="lax" maxOccurs="unbounded"/><xs:any namespace="urn:c" processContents="lax" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`
	shapeWildSchema   = `<xs:element name="r"><xs:complexType><xs:choice><xs:any namespace="urn:a" processContents="lax"/><xs:any namespace="urn:b" processContents="lax" maxOccurs="unbounded"/></xs:choice></xs:complexType></xs:element>`
)

// The types codegen emits for those schemas' s:r, each named here by its
// schema, which TestWildcardTypesAreCodegens pins.
type (
	seqWild struct {
		A   string
		Any anyElement
	}
	seqRepeated struct {
		A   string
		Any []anyElement
	}
	seqOptional struct {
		A   string
		B   *string
		Any *anyElement
	}
	seqLeading struct {
		Any *anyElement
		A   string
	}
	nestedWild struct{ N struct{ Any anyElement } }
	twoWild    struct {
		Any  anyElement
		E    string
		Any2 anyElement
	}
	equalWild struct{ Any, Any2 anyElement }
	moreWild  struct {
		Any  anyElement
		E    string
		Any2 []anyElement
		Any3 *anyElement
	}
	open struct {
		Id  string
		Any []anyElement
	}

	pickWild     interface{ isPickWild() }
	pickWildA    struct{ A string }
	pickWildAny  struct{ Any anyElement }
	manyWild     interface{ isManyWild() }
	manyWildB    struct{ B string }
	manyWildAny  struct{ Any []anyElement }
	nameAny      interface{ isNameAny() }
	nameAnyAny   struct{ Any string }
	nameAnyAny2  struct{ Any anyElement }
	pickWildAny2 struct{ Any anyElement2 }

	shapeWild     interface{ isShapeWild() }
	shapeWildAny  struct{ Any anyElement }
	shapeWildAny2 struct{ Any []anyElement }

	wildDecimal     interface{ isWildDecimal() }
	wildDecimalAny  struct{ Any anyElement }
	wildDecimalAny2 struct{ Any big.Rat }
)

func (pickWildA) isPickWild()    {}
func (pickWildAny) isPickWild()  {}
func (manyWildB) isManyWild()    {}
func (manyWildAny) isManyWild()  {}
func (nameAnyAny) isNameAny()    {}
func (nameAnyAny2) isNameAny()   {}
func (pickWildAny2) isPickWild() {}

func (shapeWildAny) isShapeWild()  {}
func (shapeWildAny2) isShapeWild() {}

func (wildDecimalAny) isWildDecimal()  {}
func (wildDecimalAny2) isWildDecimal() {}

// anyElement2 is anyElement's shape over a second set of structs, so that
// its Children's Element is anyElement2 itself.
type (
	anyElement2 struct {
		Space, Local string
		Content      anyType2
	}
	anyType2 struct {
		Attributes []anyAttribute
		Text       string
		Children   []anyChild2
	}
	anyChild2 struct {
		Element anyElement2
		Tail    string
	}
)

// The branch sets of the choice schemas' interfaces.
var (
	pickWildBranches  = WithBranches[pickWild](pickWildA{}, pickWildAny{})
	manyWildBranches  = WithBranches[manyWild](manyWildB{}, manyWildAny{})
	nameAnyBranches   = WithBranches[nameAny](nameAnyAny{}, nameAnyAny2{})
	shapeWildBranches = WithBranches[shapeWild](shapeWildAny{}, shapeWildAny2{})

	wildDecimalBranches = WithBranches[wildDecimal](wildDecimalAny{}, wildDecimalAny2{})
)

// TestWildcardTypesAreCodegens pins the types above to those codegen emits
// for each wildcard schema, in shape, so that the rows decoding into them
// decode into codegen's types: the raw-content structs, and each s:r.
func TestWildcardTypesAreCodegens(t *testing.T) {
	raw := func(element string) []string {
		return []string{
			"type AnyType struct {\n\tAttributes []AnyAttribute\n\tText       string\n\tChildren   []AnyChild\n}\n",
			"type AnyAttribute struct {\n\tSpace, Local, Value string\n}\n",
			"type AnyChild struct {\n\tElement " + element + "\n\tTail    string\n}\n",
			"type " + element + " struct {\n\tSpace, Local string\n\tContent      AnyType\n}\n",
		}
	}
	rows := []struct {
		decls string
		want  []string
	}{
		{seqStrictSchema, append(raw("AnyElement"), "type R struct {\n\tA   string\n\tAny AnyElement\n}\n")},
		{seqLaxSchema, []string{"type R struct {\n\tA   string\n\tAny AnyElement\n}\n"}},
		{seqSkipSchema, []string{"type R struct {\n\tA   string\n\tAny AnyElement\n}\n"}},
		{seqRepeatedSchema, []string{"type R struct {\n\tA   string\n\tAny []AnyElement\n}\n"}},
		{seqOptionalSchema, []string{"type R struct {\n\tA   string\n\tB   *string\n\tAny *AnyElement\n}\n"}},
		{seqLeadingSchema, []string{"type R struct {\n\tAny *AnyElement\n\tA   string\n}\n"}},
		{nestedWildSchema, []string{"type R struct {\n\tN N\n}\n", "type N struct {\n\tAny AnyElement\n}\n"}},
		{pickWildSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RA struct {\n\tA string\n}\n\nfunc (RA) isR() {}\n",
			"type RAny struct {\n\tAny AnyElement\n}\n\nfunc (RAny) isR() {}\n",
		}},
		{manyWildSchema, []string{
			"type RB struct {\n\tB string\n}\n\nfunc (RB) isR() {}\n",
			"type RAny struct {\n\tAny []AnyElement\n}\n\nfunc (RAny) isR() {}\n",
		}},
		{nameAnySchema, []string{
			"type RAny struct {\n\tAny string\n}\n\nfunc (RAny) isR() {}\n",
			"type RAny2 struct {\n\tAny AnyElement\n}\n\nfunc (RAny2) isR() {}\n",
		}},
		{wildDecimalSchema, []string{
			"type RAny struct {\n\tAny AnyElement\n}\n\nfunc (RAny) isR() {}\n",
			"type RAny2 struct {\n\tAny big.Rat\n}\n\nfunc (RAny2) isR() {}\n",
		}},
		{openWildSchema, append(raw("AnyElement2"), "type R struct {\n\tId  string\n\tAny []AnyElement2\n}\n")},
		{twoWildSchema, []string{"type R struct {\n\tAny  AnyElement\n\tE    string\n\tAny2 AnyElement\n}\n"}},
		{equalWildSchema, []string{"type R struct {\n\tAny  AnyElement\n\tAny2 AnyElement\n}\n"}},
		{moreWildSchema, []string{"type R struct {\n\tAny  AnyElement\n\tE    string\n\tAny2 []AnyElement\n\tAny3 *AnyElement\n}\n"}},
		{shapeWildSchema, []string{
			"type RAny struct {\n\tAny AnyElement\n}\n\nfunc (RAny) isR() {}\n",
			"type RAny2 struct {\n\tAny []AnyElement\n}\n\nfunc (RAny2) isR() {}\n",
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

// TestDecodeWildcard pins the wildcard lift: each child a wildcard particle
// matches (cvc-wildcard) decodes into the field codegen emits for it,
// AnyElement by its occurrence, its expanded name and its content raw, under
// a strict, lax or skip {process contents} alike. Each child is attributed
// as validation attributes it (§3.4.4.4 key-att-to), an element particle
// taking a name before the wildcard (cvc-accept, §3.8.6.4 cos-nonambig), so
// of sequence(a, b?, any?) the second <b/> is the wildcard's, and of
// sequence(a, any*) the second <a/>. Of several wildcard particles, each
// child is the field or branch of the particle it is attributed to, two
// wildcards of equal {term}s told apart by position. A match's xsi:type, at
// any depth, is one of its raw attributes and no decline.
func TestDecodeWildcard(t *testing.T) {
	const other = ` xmlns:o="urn:o"`
	const abc = ` xmlns:a="urn:a" xmlns:b="urn:b" xmlns:c="urn:c"`
	str := func(s string) *string { return &s }
	el := func(space, local, text string) anyElement {
		return anyElement{Space: space, Local: local, Content: anyType{Text: text}}
	}
	raw := anyElement{Space: "urn:o", Local: "w", Content: anyType{
		Attributes: []anyAttribute{{Local: "k", Value: "1"}, {Space: "urn:o", Local: "m", Value: " 2 3"}},
		Text:       "lead",
		Children: []anyChild{
			{Element: anyElement{Space: "urn:o", Local: "c", Content: anyType{
				Attributes: []anyAttribute{{Local: "j", Value: "x"}},
				Text:       "in",
				Children: []anyChild{{Element: anyElement{Space: "urn:o", Local: "d", Content: anyType{
					Attributes: []anyAttribute{{Space: "http://www.w3.org/2001/XMLSchema-instance", Local: "type", Value: "xs:int"}},
					Text:       "1",
				}}}},
			}}, Tail: "mid"},
			{Element: anyElement{Local: "e"}, Tail: " end "},
		},
	}}
	rows := []struct {
		name  string
		decls string
		body  string
		into  target
		want  any
	}{
		{"strict", seqStrictSchema, `<s:a>x</s:a><s:w> y </s:w>`, into[seqWild](), seqWild{A: "x", Any: el("urn:s", "w", " y ")}},
		{"lax", seqLaxSchema, `<s:a>x</s:a><o:w` + other + `>y</o:w>`, into[seqWild](), seqWild{A: "x", Any: el("urn:o", "w", "y")}},
		{"skip", seqSkipSchema, `<s:a>x</s:a><w/>`, into[seqWild](), seqWild{A: "x", Any: el("", "w", "")}},
		{"repeated, in instance order, a second a the wildcard's", seqRepeatedSchema, `<s:a>x</s:a><o:w` + other + `/><s:a>z</s:a><w>v</w>`, into[seqRepeated](),
			seqRepeated{A: "x", Any: []anyElement{el("urn:o", "w", ""), el("urn:s", "a", "z"), el("", "w", "v")}}},
		{"attributes, text and children read raw", seqSkipSchema,
			`<s:a>x</s:a><o:w` + other + ` k="1" o:m=" 2 3"><!--c-->le<!--c-->ad<o:c j="x">in<o:d xsi:type="xs:int">1</o:d></o:c>mid<e/> end </o:w>`,
			into[seqWild](), seqWild{A: "x", Any: raw}},
		{"a match carrying xsi:type", seqLaxSchema, `<s:a>x</s:a><o:w` + other + ` xsi:type="xs:string">v</o:w>`, into[seqWild](),
			seqWild{A: "x", Any: anyElement{Space: "urn:o", Local: "w", Content: anyType{
				Attributes: []anyAttribute{{Space: "http://www.w3.org/2001/XMLSchema-instance", Local: "type", Value: "xs:string"}}, Text: "v"}}}},
		{"codegen's attribution example, a and b", seqOptionalSchema, `<s:a>1</s:a><s:b>2</s:b>`, into[seqOptional](), seqOptional{A: "1", B: str("2")}},
		{"codegen's attribution example, a, b and b", seqOptionalSchema, `<s:a>1</s:a><s:b>2</s:b><s:b>3</s:b>`, into[seqOptional](),
			seqOptional{A: "1", B: str("2"), Any: &anyElement{Space: "urn:s", Local: "b", Content: anyType{Text: "3"}}}},
		{"an element particle after an optional wildcard takes its name", seqLeadingSchema, `<s:a>x</s:a>`, into[seqLeading](), seqLeading{A: "x"}},
		{"an optional wildcard before an element particle", seqLeadingSchema, `<o:a` + other + `>w</o:a><s:a>x</s:a>`, into[seqLeading](),
			seqLeading{Any: &anyElement{Space: "urn:o", Local: "a", Content: anyType{Text: "w"}}, A: "x"}},
		{"a nested type's wildcard", nestedWildSchema, `<s:n><o:w` + other + `/></s:n>`, into[nestedWild](), nestedWild{N: struct{ Any anyElement }{Any: el("urn:o", "w", "")}}},
		{"choice, the element alternative", pickWildSchema, `<s:a>x</s:a>`, into[pickWild](pickWildBranches), pickWild(pickWildA{A: "x"})},
		{"choice, the wildcard alternative", pickWildSchema, `<o:a` + other + `>x</o:a>`, into[pickWild](pickWildBranches), pickWild(pickWildAny{Any: el("urn:o", "a", "x")})},
		{"choice, a repeated wildcard alternative", manyWildSchema, `<o:w` + other + `/><o:v` + other + `/>`, into[manyWild](manyWildBranches),
			manyWild(manyWildAny{Any: []anyElement{el("urn:o", "w", ""), el("urn:o", "v", "")}})},
		{"choice, an element any beside the wildcard", nameAnySchema, `<s:any>v</s:any>`, into[nameAny](nameAnyBranches), nameAny(nameAnyAny{Any: "v"})},
		{"choice, the wildcard beside an element any", nameAnySchema, `<o:any` + other + `/>`, into[nameAny](nameAnyBranches), nameAny(nameAnyAny2{Any: el("urn:o", "any", "")})},
		{"choice, an element any of xs:decimal beside the wildcard", wildDecimalSchema, `<s:any>1</s:any>`, into[wildDecimal](wildDecimalBranches),
			wildDecimal(wildDecimalAny2{Any: *big.NewRat(1, 1)})},
		{"codegen's wildcard fixture, no match", openWildSchema, `<s:id>i</s:id>`, into[open](), open{Id: "i"}},
		{"codegen's wildcard fixture, two matches", openWildSchema, `<s:id>i</s:id><o:x` + other + `/><o:y` + other + `/>`, into[open](),
			open{Id: "i", Any: []anyElement{el("urn:o", "x", ""), el("urn:o", "y", "")}}},
		{"two wildcards around an element", twoWildSchema, `<a:x` + abc + `>1</a:x><s:e>2</s:e><b:y` + abc + `>3</b:y>`, into[twoWild](),
			twoWild{Any: el("urn:a", "x", "1"), E: "2", Any2: el("urn:b", "y", "3")}},
		{"two wildcards of equal terms", equalWildSchema, `<o:x` + other + `>1</o:x><o:y` + other + `>2</o:y>`, into[equalWild](),
			equalWild{Any: el("urn:o", "x", "1"), Any2: el("urn:o", "y", "2")}},
		{"a repeated and an optional wildcard after the second particle", moreWildSchema,
			`<a:x` + abc + `/><s:e>2</s:e><b:y` + abc + `>3</b:y><b:z` + abc + `>4</b:z><c:w` + abc + `>5</c:w>`, into[moreWild](),
			moreWild{Any: el("urn:a", "x", ""), E: "2", Any2: []anyElement{el("urn:b", "y", "3"), el("urn:b", "z", "4")}, Any3: &anyElement{Space: "urn:c", Local: "w", Content: anyType{Text: "5"}}}},
		{"a repeated wildcard after the second particle, the optional one absent", moreWildSchema, `<a:x` + abc + `/><s:e>2</s:e><b:y` + abc + `/>`, into[moreWild](),
			moreWild{Any: el("urn:a", "x", ""), E: "2", Any2: []anyElement{el("urn:b", "y", "")}}},
		{"choice of two wildcards, the first", shapeWildSchema, `<a:x` + abc + `>1</a:x>`, into[shapeWild](shapeWildBranches), shapeWild(shapeWildAny{Any: el("urn:a", "x", "1")})},
		{"choice of two wildcards, the repeated second", shapeWildSchema, `<b:x` + abc + `>1</b:x><b:y` + abc + `/>`, into[shapeWild](shapeWildBranches),
			shapeWild(shapeWildAny2{Any: []anyElement{el("urn:b", "x", "1"), el("urn:b", "y", "")}})},
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

// TestDecodeWildcardTypeMismatch pins item 6 of Decode's doc for a wildcard
// particle's field: E is matched by AnyElement's shape, so a struct missing
// a field, a field of another Go type, and an AnyChild whose Element is a
// struct of E's shape but not E each mismatch, Decode and CheckDecode alike,
// each naming the field; a choice's branch struct holding such an E is the
// wildcard alternative's mismatch, and one holding anyElement2, of E's shape
// over structs of its own, matches.
func TestDecodeWildcardTypeMismatch(t *testing.T) {
	type (
		noTail struct {
			Element anyElement
		}
		noTailType struct {
			Attributes []anyAttribute
			Text       string
			Children   []noTail
		}
		noTailElement struct {
			Space, Local string
			Content      noTailType
		}
		intSpace struct {
			Space   int
			Local   string
			Content anyType
		}
	)
	const want = " where codegen emits Any E for a struct E of codegen's AnyElement shape"
	rows := []struct {
		name string
		into target
		got  string
	}{
		{"an AnyChild with no Tail", into[struct {
			A   string
			Any noTailElement
		}](), "Any codec.noTailElement"},
		{"a Space of int", into[struct {
			A   string
			Any intSpace
		}](), "Any codec.intSpace"},
		{"an AnyChild whose Element is another struct of E's shape", into[struct {
			A   string
			Any otherBack
		}](), "Any codec.otherBack"},
	}
	pick := schemaOf(t, pickWildSchema)
	_, err := Decode[pickWild](pick, strict.New(), instanceOf("", `<s:a>x</s:a>`), WithBranches[pickWild](pickWildA{}, pickWildOther{}))
	if wantBranch := ErrTypeMismatch.Error() + ": field 0 of codec.pickWildOther is Any codec.otherBack" + want; err == nil || err.Error() != wantBranch {
		t.Errorf("Decode(a branch of otherBack) = %v, want %q", err, wantBranch)
	}
	assertProbeMismatch(t, CheckDecode[pickWild](pick, rootName, WithBranches[pickWild](pickWildA{}, pickWildOther{})), err)
	if _, err := Decode[pickWild](pick, strict.New(), instanceOf("", `<s:a>x</s:a>`), WithBranches[pickWild](pickWildA{}, pickWildAny2{})); err != nil {
		t.Errorf("Decode(a branch of anyElement2) = %v, want nil: anyElement2's own Children hold anyElement2", err)
	}
	schema := schemaOf(t, seqLaxSchema)
	data := instanceOf("", `<s:a>x</s:a><w/>`)
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			_, err := row.into.decode(schema, data)
			if !errors.Is(err, ErrTypeMismatch) {
				t.Fatalf("Decode = %v, want ErrTypeMismatch", err)
			}
			if prefix := ErrTypeMismatch.Error() + ": field 1 of "; !bytes.HasPrefix([]byte(err.Error()), []byte(prefix)) || !bytes.HasSuffix([]byte(err.Error()), []byte(" is "+row.got+want)) {
				t.Errorf("Decode = %q, want %q...%q", err, prefix, " is "+row.got+want)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}

// otherBack is of anyElement's fields, but its Children hold anyElement, a
// struct of its shape that is not otherBack itself; pickWildOther is a
// branch of pickWild holding one.
type (
	otherBack struct {
		Space, Local string
		Content      anyType
	}
	pickWildOther struct{ Any otherBack }
)

func (pickWildOther) isPickWild() {}

// TestDecodeWildcardSharedBranch pins that a choice of two wildcard
// alternatives occurring alike declines as a shared branch, which no branch
// struct could tell apart (field.sharesBranch), the decline's detail opening
// with the second wildcard particle, Decode and CheckDecode alike.
func TestDecodeWildcardSharedBranch(t *testing.T) {
	const want = "codec does not decode two branches of a sealed interface whose structs have one shape: wildcard particle 2 of "
	schema := schemaOf(t, `<xs:element name="r"><xs:complexType><xs:choice><xs:any namespace="urn:a" processContents="lax"/><xs:any namespace="urn:b" processContents="lax"/></xs:choice></xs:complexType></xs:element>`)
	data := instanceOf("", `<b:x xmlns:b="urn:b"/>`)
	mustAccept(t, schema, data)
	_, decodeErr := Decode[struct{}](schema, strict.New(), data)
	for i, err := range []error{decodeErr, CheckDecode[struct{}](schema, rootName)} {
		var de *DeclineError
		if !errors.As(err, &de) || de.Construct() != (Construct{constructSharedBranch}) {
			t.Fatalf("error %d = %v, want the shared-branch *DeclineError", i, err)
		}
		if got := err.Error(); !strings.Contains(got, want) {
			t.Errorf("error %d = %q, want it to hold %q", i, got, want)
		}
	}
}
