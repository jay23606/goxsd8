package codec

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsd"
)

// The nested schemas, each declaring s:r whose particles' element declarations have
// complex types codegen gives a type of its own: nestedSchema's one, opt and rep
// anonymous, once, optionally and repeated; named, of the named type ct, which has a
// defaulted and an optional attribute; deep, two levels of anonymous nesting; and pick,
// of the named type pick, whose content is a choice. nestedRefSchema's r holds an
// <element ref> to h, a lone non-abstract head of anonymous complex type. selfSchema is
// MS-ComplexType2006-07-15/ctZ003's shape: r's type C1 reaches itself through an
// emptiable choice's repeated alternative. sameShapeSchema's e has type c, which holds an
// optional <element ref> to e, and r has type c too. ifaceSelfSchema's type c is a choice
// codegen gives the bare interface, whose alternative e is of type c, so it reaches
// itself through interfaces alone. selfOnceSchema's r holds a required <element ref> to
// itself, which no finite instance validates and codegen declines.
const (
	nestedSchema = `
  <xs:complexType name="ct"><xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence>
    <xs:attribute name="k" type="xs:token" default=" d  v "/><xs:attribute name="m" type="xs:string"/></xs:complexType>
  <xs:complexType name="pick"><xs:choice><xs:element name="p" type="xs:string"/><xs:element name="q" type="xs:int"/></xs:choice></xs:complexType>
  <xs:element name="r"><xs:complexType><xs:sequence>
    <xs:element name="one"><xs:complexType><xs:sequence><xs:element name="x" type="xs:string"/><xs:element name="y" type="xs:token" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>
    <xs:element name="opt" minOccurs="0"><xs:complexType><xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
    <xs:element name="rep" minOccurs="0" maxOccurs="unbounded"><xs:complexType><xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
    <xs:element name="named" type="s:ct" minOccurs="0"/>
    <xs:element name="deep" minOccurs="0"><xs:complexType><xs:sequence>
      <xs:element name="mid"><xs:complexType><xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
    </xs:sequence></xs:complexType></xs:element>
    <xs:element name="pick" type="s:pick" minOccurs="0"/>
  </xs:sequence></xs:complexType></xs:element>`
	nestedRefSchema = `<xs:element name="h"><xs:complexType><xs:sequence><xs:element name="x" type="xs:string"/><xs:element name="y" type="xs:int"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:h"/></xs:sequence></xs:complexType></xs:element>`
	selfSchema = `<xs:complexType name="C1"><xs:choice><xs:element name="e1" type="s:C1" minOccurs="0" maxOccurs="10"/></xs:choice></xs:complexType>
  <xs:element name="r" type="s:C1"/>`
	sameShapeSchema = `<xs:complexType name="c"><xs:sequence><xs:element ref="s:e" minOccurs="0"/><xs:element name="v" type="xs:string"/></xs:sequence></xs:complexType>
  <xs:element name="e" type="s:c"/><xs:element name="r" type="s:c"/>`
	ifaceSelfSchema = `<xs:complexType name="c"><xs:choice><xs:element name="e" type="s:c"/><xs:element name="v" type="xs:string"/></xs:choice></xs:complexType>
  <xs:element name="r" type="s:c"/>`
	selfOnceSchema = `<xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:r"/></xs:sequence></xs:complexType></xs:element>`
)

// nestedR to nestedPickQ are nestedSchema's R, One, Opt, Rep, Ct, Deep, Mid,
// Pick, PickP and PickQ; nestedRefR and nestedH are nestedRefSchema's R and
// H; selfR, selfChoice, selfE1 and selfC1 are selfSchema's R, C1Choice,
// C1ChoiceE1 and C1; sameR and sameE are sameShapeSchema's R and E; and ifR
// to ifCV are ifaceSelfSchema's R, RE, RV, C, CE and CV. Each pin is
// TestNestedTypesAreCodegens'.
type (
	nestedR struct {
		One   nestedOne
		Opt   *nestedOpt
		Rep   []nestedRep
		Named *nestedCt
		Deep  *nestedDeep
		Pick  nestedPick
	}
	nestedOne struct {
		X string
		Y *string
	}
	nestedOpt struct{ X string }
	nestedRep struct{ X string }
	nestedCt  struct {
		X string
		K string
		M *string
	}
	nestedDeep  struct{ Mid nestedMid }
	nestedMid   struct{ X string }
	nestedPick  interface{ isNestedPick() }
	nestedPickP struct{ P string }
	nestedPickQ struct{ Q int32 }
	nestedRefR  struct{ H nestedH }
	nestedH     struct {
		X string
		Y int32
	}
	selfR      struct{ Choice selfChoice }
	selfChoice interface{ isSelfChoice() }
	selfE1     struct{ E1 []selfC1 }
	selfC1     struct{ Choice selfChoice }
	sameR      struct {
		E *sameE
		V string
	}
	sameE struct {
		E *sameE
		V string
	}
	ifR  interface{ isIfR() }
	ifRE struct{ E ifC }
	ifRV struct{ V string }
	ifC  interface{ isIfC() }
	ifCE struct{ E ifC }
	ifCV struct{ V string }
)

func (ifRE) isIfR() {}
func (ifRV) isIfR() {}
func (ifCE) isIfC() {}
func (ifCV) isIfC() {}

func (nestedPickP) isNestedPick() {}
func (nestedPickQ) isNestedPick() {}
func (selfE1) isSelfChoice()      {}
func (selfBad) isSelfChoice()     {}

// selfBad is a branch of selfChoice whose one field is named like C1's
// alternative e1 and holds no struct.
type selfBad struct{ E1 []string }

// The branch sets of nestedSchema's and selfSchema's interfaces.
var (
	nestedPickBranches = WithBranches[nestedPick](nestedPickP{}, nestedPickQ{})
	selfBranches       = WithBranches[selfChoice](selfE1{})
	ifBranches         = []Option{WithBranches[ifR](ifRE{}, ifRV{}), WithBranches[ifC](ifCE{}, ifCV{})}
)

// TestNestedTypesAreCodegens pins the types above to those codegen emits for
// each nested schema, in shape, so that the rows decoding into them decode
// into codegen's types: a nested element's field holds the struct or
// interface codegen emits for its type, a pointer for an optional struct, the
// bare interface for an optional choice.
func TestNestedTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{nestedSchema, []string{
			"type R struct {\n\tOne   One\n\tOpt   *Opt\n\tRep   []Rep\n\tNamed *Ct\n\tDeep  *Deep\n\tPick  Pick\n}\n",
			"type One struct {\n\tX string\n\tY *string\n}\n",
			"type Opt struct {\n\tX string\n}\n",
			"type Rep struct {\n\tX string\n}\n",
			"type Ct struct {\n\tX string\n\tK string\n\tM *string\n}\n",
			"type Deep struct {\n\tMid Mid\n}\n",
			"type Mid struct {\n\tX string\n}\n",
			"type Pick interface {\n\tisPick()\n}\n",
			"type PickP struct {\n\tP string\n}\n\nfunc (PickP) isPick() {}\n",
			"type PickQ struct {\n\tQ int32\n}\n\nfunc (PickQ) isPick() {}\n",
		}},
		{nestedRefSchema, []string{"type H struct {\n\tX string\n\tY int32\n}\n", "type R struct {\n\tH H\n}\n"}},
		{selfSchema, []string{
			"type R struct {\n\tChoice C1Choice\n}\n",
			"type C1Choice interface {\n\tisC1Choice()\n}\n",
			"type C1ChoiceE1 struct {\n\tE1 []C1\n}\n\nfunc (C1ChoiceE1) isC1Choice() {}\n",
			"type C1 struct {\n\tChoice C1Choice\n}\n",
		}},
		{sameShapeSchema, []string{"type E struct {\n\tE *E\n\tV string\n}\n", "type R struct {\n\tE *E\n\tV string\n}\n"}},
		{ifaceSelfSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RE struct {\n\tE C\n}\n\nfunc (RE) isR() {}\n",
			"type C interface {\n\tisC()\n}\n",
			"type CE struct {\n\tE C\n}\n\nfunc (CE) isC() {}\n",
			"type CV struct {\n\tV string\n}\n\nfunc (CV) isC() {}\n",
			"type RV struct {\n\tV string\n}\n\nfunc (RV) isR() {}\n",
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

// TestDecodeNested pins the lift of a particle's element declaration of
// complex type (cvc-complex-type, cvc-elt: each child assessed against its
// own declaration's type): it decodes into the struct or interface codegen
// emits for that type, in each occurrence shape, anonymous or named, two
// levels deep, with its attribute fields, a defaulted one holding its
// supplied value (key-dflt-att) under its own whiteSpace, with a choice's
// branch, through an <element ref> to a lone head, and through a type that
// reaches itself, four levels deep; and a struct of the same shape as an
// element's own, codegen's R for e's document element, decodes as E does.
func TestDecodeNested(t *testing.T) {
	str := func(s string) *string { return &s }
	// self returns a C1 holding the given e1 children.
	self := func(children ...selfC1) selfC1 { return selfC1{Choice: selfE1{E1: children}} }
	rows := []struct {
		name  string
		decls string
		data  []byte
		into  target
		check func(*xsd.Schema) error
		want  any
	}{
		{"each shape present, two levels deep, a named type's attributes present, a choice's branch", nestedSchema,
			instanceOf("", `<s:one><s:x> a </s:x><s:y> b  c </s:y></s:one><s:opt><s:x>o</s:x></s:opt><s:rep><s:x>1</s:x></s:rep><s:rep><s:x>2</s:x></s:rep>`+
				`<s:named k=" kk " m=" mm "><s:x>n</s:x></s:named><s:deep><s:mid><s:x>d</s:x></s:mid></s:deep><s:pick><s:q> 7 </s:q></s:pick>`),
			into[nestedR](nestedPickBranches), nil,
			nestedR{One: nestedOne{X: " a ", Y: str("b c")}, Opt: &nestedOpt{X: "o"}, Rep: []nestedRep{{X: "1"}, {X: "2"}},
				Named: &nestedCt{X: "n", K: "kk", M: str(" mm ")}, Deep: &nestedDeep{Mid: nestedMid{X: "d"}}, Pick: nestedPickQ{Q: 7}}},
		{"optional and repeated absent, a named type's attribute defaulted, the other branch", nestedSchema,
			instanceOf("", `<s:one><s:x>a</s:x></s:one><s:named><s:x>n</s:x></s:named><s:pick><s:p>z</s:p></s:pick>`),
			into[nestedR](nestedPickBranches), nil,
			nestedR{One: nestedOne{X: "a"}, Named: &nestedCt{X: "n", K: "d v"}, Pick: nestedPickP{P: "z"}}},
		{"an optional choice absent", nestedSchema, instanceOf("", `<s:one><s:x>a</s:x></s:one>`),
			into[nestedR](nestedPickBranches), nil, nestedR{One: nestedOne{X: "a"}}},
		{"an <element ref> to a lone head of complex type", nestedRefSchema, instanceOf("", `<s:h><s:x>x</s:x><s:y> 3 </s:y></s:h>`),
			into[nestedRefR](), nil, nestedRefR{H: nestedH{X: "x", Y: 3}}},
		{"a type reaching itself, four levels deep", selfSchema, instanceOf("", `<s:e1/><s:e1><s:e1><s:e1/></s:e1></s:e1>`),
			into[selfR](selfBranches), nil, selfR{Choice: selfE1{E1: []selfC1{{}, self(self(selfC1{}))}}}},
		{"a type reaching itself through interfaces alone", ifaceSelfSchema, instanceOf("", `<s:e><s:e><s:v>x</s:v></s:e></s:e>`),
			into[ifR](ifBranches...), nil, ifR(ifRE{E: ifCE{E: ifCV{V: "x"}}})},
		{"a type reaching itself, its empty match", selfSchema, instanceOf("", ``), into[selfR](selfBranches), nil, selfR{}},
		{"codegen's own struct of a type reaching itself", sameShapeSchema, instanceOf("", `<s:e><s:e><s:v>i</s:v></s:e><s:v>m</s:v></s:e><s:v>o</s:v>`),
			into[sameR](), nil, sameR{E: &sameE{E: &sameE{V: "i"}, V: "m"}, V: "o"}},
		{"another element's struct of the same shape, through a type reaching itself", sameShapeSchema,
			[]byte(`<s:e xmlns:s="urn:s"><s:e><s:v>i</s:v></s:e><s:v>m</s:v></s:e>`), into[sameR](),
			func(schema *xsd.Schema) error {
				return CheckDecode[sameR](schema, xsd.QName{Space: "urn:s", Local: "e"})
			},
			sameR{E: &sameE{V: "i"}, V: "m"}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, row.data)
			got, err := row.into.decode(schema, row.data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, row.want) {
				t.Errorf("Decode = %#v, want %#v", got, row.want)
			}
			check := row.into.check
			if row.check != nil {
				check = row.check
			}
			if err := check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil for a row Decode decodes", err)
			}
		})
	}
}

// selfPtr is a struct whose one field points at its own type, for
// selfOnceSchema's r, whose one particle refers to r occurring once.
type selfPtr struct{ R *selfPtr }

// TestDecodeNestedTypeMismatch pins item 6 of Decode's doc at depth: a nested
// field whose Go type is not the struct or interface codegen emits, or a
// nested struct whose fields differ from codegen's, is ErrTypeMismatch naming
// the nested site, no decline, and nothing is filled; and a type that holds
// itself by value, which no Go type matches, returns ErrTypeMismatch from
// CheckDecode, never recursing without bound.
func TestDecodeNestedTypeMismatch(t *testing.T) {
	type badH struct {
		X string
		Y string
	}
	type badRef struct{ H badH }
	type plainRef struct{ H string }
	type pointerChoice struct {
		One   nestedOne
		Opt   *nestedOpt
		Rep   []nestedRep
		Named *nestedCt
		Deep  *nestedDeep
		Pick  *nestedPick
	}
	type plainMid struct {
		One   nestedOne
		Opt   *nestedOpt
		Rep   []nestedRep
		Named *nestedCt
		Deep  *struct{ Mid string }
		Pick  nestedPick
	}
	refData := instanceOf("", `<s:h><s:x>x</s:x><s:y>3</s:y></s:h>`)
	nestedData := instanceOf("", `<s:one><s:x>a</s:x></s:one>`)
	rows := []struct {
		name  string
		decls string
		data  []byte
		into  target
		want  string
	}{
		{"a nested struct's field retyped", nestedRefSchema, refData, into[badRef](),
			"field 1 of codec.badH is Y string where codegen emits Y int32"},
		{"a string where codegen emits a struct", nestedRefSchema, refData, into[plainRef](),
			"field 0 of codec.plainRef is H string where codegen emits H S for a struct S"},
		{"a string two levels deep", nestedSchema, nestedData, into[plainMid](nestedPickBranches),
			"field 0 of struct { Mid string } is Mid string where codegen emits Mid S for a struct S"},
		{"a pointer to an optional choice's interface", nestedSchema, nestedData, into[pointerChoice](nestedPickBranches),
			"field 5 of codec.pointerChoice is Pick *codec.nestedPick where codegen emits Pick I for a sealed interface I"},
		{"no WithBranches for a nested interface", nestedSchema, nestedData, into[nestedR](),
			"no WithBranches supplies the branches of codec.nestedPick, the interface of field Pick of codec.nestedR, where codegen emits 2"},
		{"a nested alternative's branch selected by its field's name", selfSchema, instanceOf("", ""), into[selfR](WithBranches[selfChoice](selfBad{})),
			"field 0 of codec.selfBad is E1 []string where codegen emits E1 []S for a struct S"},
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
	self := schemaOf(t, selfOnceSchema)
	for _, probe := range []struct {
		name string
		err  error
		want string
	}{
		{"struct{ R struct{} }", CheckDecode[struct{ R struct{} }](self, rootName), "struct {} has 0 fields where codegen emits 1"},
		{"selfPtr", CheckDecode[selfPtr](self, rootName), "field 0 of codec.selfPtr is R *codec.selfPtr where codegen emits R S for a struct S"},
	} {
		if want := ErrTypeMismatch.Error() + ": " + probe.want; probe.err == nil || probe.err.Error() != want || !errors.Is(probe.err, ErrTypeMismatch) {
			t.Errorf("CheckDecode[%s](a ref to itself occurring once) = %v, want %q", probe.name, probe.err, want)
		}
	}
}

// TestDecodeNestedXsiType pins item 7 at depth: an xsi:type on a grandchild,
// inside a nested element of complex type, declines at its own instance Loc.
func TestDecodeNestedXsiType(t *testing.T) {
	schema := schemaOf(t, nestedRefSchema)
	data := instanceOf("", `<s:h><s:x xsi:type="xs:string">x</s:x><s:y>3</s:y></s:h>`)
	mustAccept(t, schema, data)
	got, err := Decode[nestedRefR](schema, strict.New(), data)
	if got != nil {
		t.Fatalf("Decode = %v, want no value", got)
	}
	assertDecline(t, err, constructXsiType, 1, bytes.Index(data, []byte(`<s:x xsi`))+1)
}

// TestDeclineErrorTextNested pins a decline's whole message for a construct
// of a particle's complex type: charged to the document element, it names the
// particle's declaration and the type's schema Loc, labelled as that
// declaration's {type definition}, as a construct of the document element's
// type is, and no path from the document element.
func TestDeclineErrorTextNested(t *testing.T) {
	schema := schemaOf(t, sequenceOf(`<xs:element name="a"><xs:complexType><xs:choice maxOccurs="2"><xs:element name="b" type="xs:string"/></xs:choice></xs:complexType></xs:element>`))
	data := instanceOf("", `<s:a><s:b>x</s:b></s:a>`)
	mustAccept(t, schema, data)
	_, err := Decode[struct{}](schema, strict.New(), data)
	const want = "?:1:1: codec does not decode a content choice occurring other than exactly once: the {type definition} of element {urn:s}a declared at s.xsd:1:231, a type defined at s.xsd:1:252; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}
