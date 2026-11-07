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

// The shared-name schemas, each holding two alternatives of one sealed
// interface whose branch fields codegen names alike and no Go type both
// admit: shareSchema's r is a choice of a qualified x of complex type and an
// unqualified x of xs:string; shareGroupSchema's r holds an <element ref> to
// the abstract head h, whose members are x of complex type and X of
// xs:string; shareRefSchema's r is a choice of an <element ref> to g, whose
// group codegen gives a sealed interface, and a local G of complex type;
// shareOccursSchema's r is a choice of x of complex type and X of the same
// shape repeated; shareSelfSchema is ifaceSelfSchema's C1 with a local E1 of
// xs:string beside e1, of type C1, so e1 reaches C1's interface while it is
// unbound; shareListSchema's r is a choice of x of xs:int occurring once and
// an <element ref> to X, a list of xs:int, occurring once; shareListsSchema's
// r is the same choice with both repeated; and shareStringsSchema's r is the
// same choice with x of xs:string repeated.
var (
	shareSchema      = root(`<xs:choice>` + vStruct("x", "") + `<xs:element name="x" form="unqualified" type="xs:string"/></xs:choice>`)
	shareGroupSchema = `<xs:element name="h" abstract="true"/>` + vStruct("x", ` substitutionGroup="s:h"`) +
		`<xs:element name="X" type="xs:string" substitutionGroup="s:h"/>` + sequenceOf(`<xs:element ref="s:h"/>`)
	shareRefSchema = `<xs:element name="g" type="xs:string"/><xs:element name="m" type="xs:string" substitutionGroup="s:g"/>` +
		root(`<xs:choice><xs:element ref="s:g"/>`+vStruct("G", "")+`</xs:choice>`)
	shareOccursSchema = root(`<xs:choice>` + vStruct("x", "") + vStruct("X", ` maxOccurs="unbounded"`) + `</xs:choice>`)
	shareSelfSchema   = `<xs:complexType name="C1"><xs:choice><xs:element name="e1" type="s:C1"/><xs:element name="E1" type="xs:string"/></xs:choice></xs:complexType>
  <xs:element name="r" type="s:C1"/>`
	shareListSchema = `<xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element>` +
		root(`<xs:choice><xs:element name="x" type="xs:int"/><xs:element ref="s:X"/></xs:choice>`)
	shareListsSchema = `<xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element>` +
		root(`<xs:choice><xs:element name="x" type="xs:int" maxOccurs="unbounded"/><xs:element ref="s:X" maxOccurs="unbounded"/></xs:choice>`)
	shareStringsSchema = `<xs:element name="X"><xs:simpleType><xs:list itemType="xs:int"/></xs:simpleType></xs:element>` +
		root(`<xs:choice><xs:element name="x" type="xs:string" maxOccurs="unbounded"/><xs:element ref="s:X"/></xs:choice>`)
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

// shareR to shareXS are shareSchema's R, RX, RX2 and RX3; shareGR to
// shareX2 are shareGroupSchema's R, HSubstitutionGroup, HSubstitutionGroupX,
// HSubstitutionGroupX2, X and X2, shareXV standing for X; shareRR to shareM
// are shareRefSchema's R, RG, RG2, GSubstitutionGroup, GSubstitutionGroupG,
// GSubstitutionGroupM, G and M, shareXV standing for RG3; shareOR to shareOXs
// are shareOccursSchema's R, RX and RX3, shareXV standing for RX2 and RX4;
// shareSR to shareC1S are shareSelfSchema's R, RE1, RE12, C1, C1E1 and
// C1E12; shareLR to shareLX are shareListSchema's R, RX, RX2 and X; and
// shareLsR to shareLsRX2 and shareSsR to shareSsRX2 are shareListsSchema's
// and shareStringsSchema's R, RX and RX2, shareLX standing for X in each.
// Each pin is TestNestedTypesAreCodegens'.
type (
	shareR  interface{ isShareR() }
	shareX  struct{ X shareXV }
	shareXV struct{ V string }
	shareXS struct{ X string }

	shareGR  struct{ H shareH }
	shareH   interface{ isShareH() }
	shareHX  struct{ X shareXV }
	shareHX2 struct{ X shareX2 }
	shareX2  string

	shareRR  interface{ isShareRR() }
	shareRG  struct{ G shareGG }
	shareRG2 struct{ G shareXV }
	shareGG  interface{ isShareGG() }
	shareGGG struct{ G shareG }
	shareGGM struct{ M shareM }
	shareG   string
	shareM   string

	shareOR  interface{ isShareOR() }
	shareOX  struct{ X shareXV }
	shareOXs struct{ X []shareXV }

	shareSR  interface{ isShareSR() }
	shareSRE struct{ E1 shareC1 }
	shareSRS struct{ E1 string }
	shareC1  interface{ isShareC1() }
	shareC1E struct{ E1 shareC1 }
	shareC1S struct{ E1 string }

	shareLR   interface{ isShareLR() }
	shareLRX  struct{ X int32 }
	shareLRX2 struct{ X shareLX }
	shareLX   []int32

	shareLsR   interface{ isShareLsR() }
	shareLsRX  struct{ X []int32 }
	shareLsRX2 struct{ X []shareLX }

	shareSsR   interface{ isShareSsR() }
	shareSsRX  struct{ X []string }
	shareSsRX2 struct{ X shareLX }
)

func (shareX) isShareR()    {}
func (shareXS) isShareR()   {}
func (shareHX) isShareH()   {}
func (shareHX2) isShareH()  {}
func (shareRG) isShareRR()  {}
func (shareRG2) isShareRR() {}
func (shareGGG) isShareGG() {}
func (shareGGM) isShareGG() {}
func (shareOX) isShareOR()  {}
func (shareOXs) isShareOR() {}
func (shareSRE) isShareSR() {}
func (shareSRS) isShareSR() {}
func (shareC1E) isShareC1() {}
func (shareC1S) isShareC1() {}

func (shareLRX) isShareLR()    {}
func (shareLRX2) isShareLR()   {}
func (shareLsRX) isShareLsR()  {}
func (shareLsRX2) isShareLsR() {}
func (shareSsRX) isShareSsR()  {}
func (shareSsRX2) isShareSsR() {}

// The branch sets of the shared-name schemas' interfaces, shareSchema's in
// both orders.
var (
	shareStructFirst    = WithBranches[shareR](shareX{}, shareXS{})
	shareScalarFirst    = WithBranches[shareR](shareXS{}, shareX{})
	shareGroupBranches  = WithBranches[shareH](shareHX{}, shareHX2{})
	shareRefBranches    = []Option{WithBranches[shareRR](shareRG{}, shareRG2{}), WithBranches[shareGG](shareGGG{}, shareGGM{})}
	shareOccursBranches = WithBranches[shareOR](shareOX{}, shareOXs{})
	shareSelfBranches   = []Option{WithBranches[shareSR](shareSRE{}, shareSRS{}), WithBranches[shareC1](shareC1E{}, shareC1S{})}

	shareListBranches    = WithBranches[shareLR](shareLRX{}, shareLRX2{})
	shareListsBranches   = WithBranches[shareLsR](shareLsRX{}, shareLsRX2{})
	shareStringsBranches = WithBranches[shareSsR](shareSsRX{}, shareSsRX2{})
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
		{shareSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX RX2\n}\n\nfunc (RX) isR() {}\n",
			"type RX2 struct {\n\tV string\n}\n",
			"type RX3 struct {\n\tX string\n}\n\nfunc (RX3) isR() {}\n",
		}},
		{shareGroupSchema, []string{
			"type X struct {\n\tV string\n}\n",
			"type X2 string\n",
			"type R struct {\n\tH HSubstitutionGroup\n}\n",
			"type HSubstitutionGroup interface {\n\tisHSubstitutionGroup()\n}\n",
			"type HSubstitutionGroupX struct {\n\tX X\n}\n\nfunc (HSubstitutionGroupX) isHSubstitutionGroup() {}\n",
			"type HSubstitutionGroupX2 struct {\n\tX X2\n}\n\nfunc (HSubstitutionGroupX2) isHSubstitutionGroup() {}\n",
		}},
		{shareRefSchema, []string{
			"type G string\n",
			"type M string\n",
			"type R interface {\n\tisR()\n}\n",
			"type RG struct {\n\tG GSubstitutionGroup\n}\n\nfunc (RG) isR() {}\n",
			"type GSubstitutionGroup interface {\n\tisGSubstitutionGroup()\n}\n",
			"type GSubstitutionGroupG struct {\n\tG G\n}\n\nfunc (GSubstitutionGroupG) isGSubstitutionGroup() {}\n",
			"type GSubstitutionGroupM struct {\n\tM M\n}\n\nfunc (GSubstitutionGroupM) isGSubstitutionGroup() {}\n",
			"type RG2 struct {\n\tG RG3\n}\n\nfunc (RG2) isR() {}\n",
			"type RG3 struct {\n\tV string\n}\n",
		}},
		{shareOccursSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX RX2\n}\n\nfunc (RX) isR() {}\n",
			"type RX2 struct {\n\tV string\n}\n",
			"type RX3 struct {\n\tX []RX4\n}\n\nfunc (RX3) isR() {}\n",
			"type RX4 struct {\n\tV string\n}\n",
		}},
		{shareSelfSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RE1 struct {\n\tE1 C1\n}\n\nfunc (RE1) isR() {}\n",
			"type C1 interface {\n\tisC1()\n}\n",
			"type C1E1 struct {\n\tE1 C1\n}\n\nfunc (C1E1) isC1() {}\n",
			"type C1E12 struct {\n\tE1 string\n}\n\nfunc (C1E12) isC1() {}\n",
			"type RE12 struct {\n\tE1 string\n}\n\nfunc (RE12) isR() {}\n",
		}},
		{shareListSchema, []string{
			"type X []int32\n",
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX int32\n}\n\nfunc (RX) isR() {}\n",
			"type RX2 struct {\n\tX X\n}\n\nfunc (RX2) isR() {}\n",
		}},
		{shareListsSchema, []string{
			"type X []int32\n",
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX []int32\n}\n\nfunc (RX) isR() {}\n",
			"type RX2 struct {\n\tX []X\n}\n\nfunc (RX2) isR() {}\n",
		}},
		{shareStringsSchema, []string{
			"type X []int32\n",
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX []string\n}\n\nfunc (RX) isR() {}\n",
			"type RX2 struct {\n\tX X\n}\n\nfunc (RX2) isR() {}\n",
		}},
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
// reaches itself, four levels deep; a struct of the same shape as an
// element's own, codegen's R for e's document element, decodes as E does;
// and two alternatives of one sealed interface whose branch fields codegen
// names alike, which no Go type both admit (key-e-d-match: the child's
// expanded name selects its alternative), each decode into their own
// branch, shareSchema's whichever of the two WithBranches supplies first.
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
		{"a choice's alternatives of a struct and a string named alike, the struct's branch", shareSchema, instanceOf("", `<s:x><s:v>v</s:v></s:x>`),
			into[shareR](shareStructFirst), nil, shareR(shareX{X: shareXV{V: "v"}})},
		{"a choice's alternatives of a struct and a string named alike, the string's branch", shareSchema, instanceOf("", `<x>s</x>`),
			into[shareR](shareStructFirst), nil, shareR(shareXS{X: "s"})},
		{"a choice's alternatives of a struct and a string named alike, the struct's branch, the string's supplied first", shareSchema, instanceOf("", `<s:x><s:v>v</s:v></s:x>`),
			into[shareR](shareScalarFirst), nil, shareR(shareX{X: shareXV{V: "v"}})},
		{"a choice's alternatives of a struct and a string named alike, the string's branch, the string's supplied first", shareSchema, instanceOf("", `<x>s</x>`),
			into[shareR](shareScalarFirst), nil, shareR(shareXS{X: "s"})},
		{"a substitution group's members of a struct and a string named alike, the struct's branch", shareGroupSchema, instanceOf("", `<s:x><s:v>v</s:v></s:x>`),
			into[shareGR](shareGroupBranches), nil, shareGR{H: shareHX{X: shareXV{V: "v"}}}},
		{"a substitution group's members of a struct and a string named alike, the string's branch", shareGroupSchema, instanceOf("", `<s:X>s</s:X>`),
			into[shareGR](shareGroupBranches), nil, shareGR{H: shareHX2{X: "s"}}},
		{"a choice's alternatives of a substitution group's interface and a struct named alike, a member's branch", shareRefSchema, instanceOf("", `<s:m>m</s:m>`),
			into[shareRR](shareRefBranches...), nil, shareRR(shareRG{G: shareGGM{M: "m"}})},
		{"a choice's alternatives of a substitution group's interface and a struct named alike, the struct's branch", shareRefSchema, instanceOf("", `<s:G><s:v>v</s:v></s:G>`),
			into[shareRR](shareRefBranches...), nil, shareRR(shareRG2{G: shareXV{V: "v"}})},
		{"a choice's alternatives of a struct once and repeated named alike, the one's branch", shareOccursSchema, instanceOf("", `<s:x><s:v>v</s:v></s:x>`),
			into[shareOR](shareOccursBranches), nil, shareOR(shareOX{X: shareXV{V: "v"}})},
		{"a choice's alternatives of a struct once and repeated named alike, the repeated's branch", shareOccursSchema, instanceOf("", `<s:X><s:v>a</s:v></s:X><s:X><s:v>b</s:v></s:X>`),
			into[shareOR](shareOccursBranches), nil, shareOR(shareOXs{X: []shareXV{{V: "a"}, {V: "b"}}})},
		{"a choice's alternatives of its own interface and a string named alike", shareSelfSchema, instanceOf("", `<s:e1><s:E1>s</s:E1></s:e1>`),
			into[shareSR](shareSelfBranches...), nil, shareSR(shareSRE{E1: shareC1S{E1: "s"}})},
		{"a choice's alternatives of an xs:int once and a ref to a list of xs:int named alike, the xs:int's branch", shareListSchema, instanceOf("", `<s:x>1</s:x>`),
			into[shareLR](shareListBranches), nil, shareLR(shareLRX{X: 1})},
		{"a choice's alternatives of an xs:int once and a ref to a list of xs:int named alike, the list's branch", shareListSchema, instanceOf("", `<s:X>1 2</s:X>`),
			into[shareLR](shareListBranches), nil, shareLR(shareLRX2{X: shareLX{1, 2}})},
		{"a choice's alternatives of an xs:int and a ref to a list of xs:int, both repeated, named alike, the xs:int's branch", shareListsSchema, instanceOf("", `<s:x>1</s:x><s:x>2</s:x>`),
			into[shareLsR](shareListsBranches), nil, shareLsR(shareLsRX{X: []int32{1, 2}})},
		{"a choice's alternatives of an xs:int and a ref to a list of xs:int, both repeated, named alike, the list's branch", shareListsSchema, instanceOf("", `<s:X>1 2</s:X><s:X>3</s:X>`),
			into[shareLsR](shareListsBranches), nil, shareLsR(shareLsRX2{X: []shareLX{{1, 2}, {3}}})},
		{"a choice's alternatives of a repeated xs:string and a ref to a list of xs:int named alike, the xs:string's branch", shareStringsSchema, instanceOf("", `<s:x>a</s:x><s:x>b</s:x>`),
			into[shareSsR](shareStringsBranches), nil, shareSsR(shareSsRX{X: []string{"a", "b"}})},
		{"a choice's alternatives of a repeated xs:string and a ref to a list of xs:int named alike, the list's branch", shareStringsSchema, instanceOf("", `<s:X>1 2</s:X>`),
			into[shareSsR](shareStringsBranches), nil, shareSsR(shareSsRX2{X: shareLX{1, 2}})},
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

// The abstract-type schemas, each declaring s:r of the abstract complex type ab,
// so that no document element validates against ab itself (ctd-abstract,
// cvc-type clause 2) and each instance names a concrete member by xsi:type,
// which overrides r's type (cvc-elt clause 4): abstractTypeTwoSchema's ab, of a
// sequence of a, has the two members c1 and c2, each adding an attribute;
// abstractTypeOneSchema's has c1 alone; and abstractTypeEmptySchema's ab, of empty
// content, has e1 alone, adding k.
const (
	abstractTypeTwoSchema = `<xs:complexType name="ab" abstract="true"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
  <xs:complexType name="c1"><xs:complexContent><xs:extension base="s:ab"><xs:attribute name="k" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="c2"><xs:complexContent><xs:extension base="s:ab"><xs:attribute name="m" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="r" type="s:ab"/>`
	abstractTypeOneSchema = `<xs:complexType name="ab" abstract="true"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
  <xs:complexType name="c1"><xs:complexContent><xs:extension base="s:ab"><xs:attribute name="k" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="r" type="s:ab"/>`
	abstractTypeEmptySchema = `<xs:complexType name="ab" abstract="true"/>
  <xs:complexType name="e1"><xs:complexContent><xs:extension base="s:ab"><xs:attribute name="k" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="r" type="s:ab"/>`
)

// abR to abC2 are abstractTypeTwoSchema's R, RC1, RC2, C1 and C2; abOneR is
// abstractTypeOneSchema's R and abEmptyR abstractTypeEmptySchema's R. Each pin is
// TestAbstractTypesAreCodegens'.
type (
	abR   interface{ isAbR() }
	abRC1 struct{ C1 abC1 }
	abRC2 struct{ C2 abC2 }
	abC1  struct {
		A string
		K *string
	}
	abC2 struct {
		A string
		M *string
	}
	abOneR struct {
		A string
		K *string
	}
	abEmptyR struct{ K *string }
)

func (abRC1) isAbR() {}
func (abRC2) isAbR() {}

// TestAbstractTypesAreCodegens pins the types above to those codegen emits
// for each abstract-type schema, in shape: the governing types' sealed interface
// for two members, and the one member's struct, which adds a field to ab's,
// for one (codegen/doc.go "An element declaration's governing types").
func TestAbstractTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{abstractTypeTwoSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RC1 struct {\n\tC1 C1\n}\n\nfunc (RC1) isR() {}\n",
			"type RC2 struct {\n\tC2 C2\n}\n\nfunc (RC2) isR() {}\n",
			"type C1 struct {\n\tA string\n\tK *string\n}\n",
			"type C2 struct {\n\tA string\n\tM *string\n}\n",
		}},
		{abstractTypeOneSchema, []string{"type R struct {\n\tA string\n\tK *string\n}\n"}},
		{abstractTypeEmptySchema, []string{"type R struct {\n\tK *string\n}\n"}},
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

// TestDecodeAbstractTypeElement pins the document element's abstract type
// declining into codegen's own T: ab has no valid instance (ctd-abstract,
// cvc-type clause 2), codegen emits no struct for it, and CheckDecode and
// Decode, the instance naming a concrete member by xsi:type (cvc-elt clause
// 4), each return the constructAbstractTypeElement decline, never
// ErrTypeMismatch. Without the abstract test in (*bindRun).complex every row
// returns ErrTypeMismatch: "codec.abR is no struct" for the interface, "has 2
// fields where codegen emits 1" and "has 1 fields where codegen emits 0" for
// the members' structs.
func TestDecodeAbstractTypeElement(t *testing.T) {
	rows := []struct {
		name  string
		decls string
		data  []byte
		into  target
	}{
		{"two members: the governing types' interface", abstractTypeTwoSchema,
			instanceOf(` xsi:type="s:c1" k="v"`, `<s:a>x</s:a>`), into[abR](WithBranches[abR](abRC1{}, abRC2{}))},
		{"one member of element-only content: its struct", abstractTypeOneSchema,
			instanceOf(` xsi:type="s:c1" k="v"`, `<s:a>x</s:a>`), into[abOneR]()},
		{"one member of empty content: its struct", abstractTypeEmptySchema,
			instanceOf(` xsi:type="s:e1" k="v"`, ``), into[abEmptyR]()},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, row.data)
			got, err := row.into.decode(schema, row.data)
			if got != nil {
				t.Fatalf("Decode = %v, want no value", got)
			}
			if errors.Is(err, ErrTypeMismatch) {
				t.Fatalf("Decode error = %v, want the decline, not ErrTypeMismatch", err)
			}
			assertDecline(t, err, constructAbstractTypeElement, 1, 1)
			probe := row.into.check(schema)
			if errors.Is(probe, ErrTypeMismatch) {
				t.Fatalf("CheckDecode = %v, want the decline, not ErrTypeMismatch", probe)
			}
			assertProbeDecline(t, probe, err)
		})
	}
}
