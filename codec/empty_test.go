package codec

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsderr"
)

// The empty-content schemas, each declaring s:r: emptyAttrsSchema's r has
// empty content and two attributes, o optional and d defaulted to 7;
// emptyBareSchema's r has empty content and no attribute; emptySeqSchema's r
// is a sequence of four particles whose element declarations have empty
// content: a local a of the named type ct, which has an optional attribute k,
// a local e of an anonymous type, an optional <element ref> to h, a lone
// non-abstract head of type ct, and an <element ref> to g, an abstract head
// of type ct whose one member m is of type ct; and emptyChoiceSchema's r is a
// choice whose alternative x is of type ct.
const (
	emptyAttrsSchema = `<xs:element name="r"><xs:complexType><xs:attribute name="o" type="xs:string"/><xs:attribute name="d" type="xs:int" default="7"/></xs:complexType></xs:element>`
	emptyBareSchema  = `<xs:element name="r"><xs:complexType/></xs:element>`
	emptyCt          = `<xs:complexType name="ct"><xs:attribute name="k" type="xs:string"/></xs:complexType>`
	emptySeqSchema   = emptyCt + `<xs:element name="h" type="s:ct"/><xs:element name="g" type="s:ct" abstract="true"/><xs:element name="m" type="s:ct" substitutionGroup="s:g"/>
		<xs:element name="r"><xs:complexType><xs:sequence><xs:element name="a" type="s:ct"/><xs:element name="e"><xs:complexType/></xs:element>
		<xs:element ref="s:h" minOccurs="0"/><xs:element ref="s:g"/></xs:sequence></xs:complexType></xs:element>`
	emptyChoiceSchema = emptyCt + `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="x" type="s:ct"/><xs:element name="y" type="xs:string"/></xs:choice></xs:complexType></xs:element>`
)

// emptyAttrs is emptyAttrsSchema's R; emptySeq, emptyGroup and emptyGroupM
// are emptySeqSchema's R, GSubstitutionGroup and GSubstitutionGroupM, its Ct,
// H and M each emptyCtT and its E struct{}; and emptyChoice to emptyChoiceY
// are emptyChoiceSchema's R, RX and RY, its Ct emptyCtT. Each pin is
// TestEmptyTypesAreCodegens'.
type (
	emptyAttrs struct {
		O *string
		D int32
	}
	emptySeq struct {
		A emptyCtT
		E struct{}
		H *emptyCtT
		G emptyGroup
	}
	emptyCtT     struct{ K *string }
	emptyGroup   interface{ isEmptyGroup() }
	emptyGroupM  struct{ M emptyCtT }
	emptyChoice  interface{ isEmptyChoice() }
	emptyChoiceX struct{ X emptyCtT }
	emptyChoiceY struct{ Y string }
)

func (emptyGroupM) isEmptyGroup()   {}
func (emptyChoiceX) isEmptyChoice() {}
func (emptyChoiceY) isEmptyChoice() {}

// TestEmptyTypesAreCodegens pins the types above to those codegen emits for
// each empty-content schema, in shape: empty content gives a struct with no
// particle field, struct{} for one with no attribute either (codegen/doc.go
// "# Complex types").
func TestEmptyTypesAreCodegens(t *testing.T) {
	ct := "type Ct struct {\n\tK *string\n}\n"
	rows := []struct {
		decls string
		want  []string
	}{
		{emptyAttrsSchema, []string{"type R struct {\n\tO *string\n\tD int32\n}\n"}},
		{emptyBareSchema, []string{"type R struct{}\n"}},
		{emptySeqSchema, []string{"type R struct {\n\tA Ct\n\tE E\n\tH *H\n\tG GSubstitutionGroup\n}\n", ct, "type E struct{}\n",
			"type H struct {\n\tK *string\n}\n", "type M struct {\n\tK *string\n}\n",
			"type GSubstitutionGroupM struct {\n\tM M\n}\n\nfunc (GSubstitutionGroupM) isGSubstitutionGroup() {}\n"}},
		{emptyChoiceSchema, []string{"type R interface {\n\tisR()\n}\n", ct, "type RX struct {\n\tX Ct\n}\n\nfunc (RX) isR() {}\n",
			"type RY struct {\n\tY string\n}\n\nfunc (RY) isR() {}\n"}},
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

// TestDecodeEmptyContent pins the lift of empty content ({variety} empty,
// §3.4.1), which admits no character or element child (cvc-complex-type clause
// 1.1): a document element's, or a particle's element declaration's, decodes
// into the struct codegen emits for it, its attribute fields filled by the
// attribute rules, an optional one nil when absent and a defaulted one its
// supplied value (key-dflt-att), and struct{} for one with no attribute; a
// particle's element of empty content does so local and named, local and
// anonymous, through an <element ref> to a lone head, as a member of an
// abstract head's substitution group, and as a choice's alternative.
func TestDecodeEmptyContent(t *testing.T) {
	str := func(s string) *string { return &s }
	groupBranches := WithBranches[emptyGroup](emptyGroupM{})
	choiceBranches := WithBranches[emptyChoice](emptyChoiceX{}, emptyChoiceY{})
	rows := []struct {
		name  string
		decls string
		data  []byte
		into  target
		want  any
	}{
		{"a document element, both attributes present", emptyAttrsSchema, instanceOf(` o=" v " d=" 3 "`, ""), into[emptyAttrs](), emptyAttrs{O: str(" v "), D: 3}},
		{"a document element, the optional present, the defaulted absent", emptyAttrsSchema, instanceOf(` o="v"`, ""), into[emptyAttrs](), emptyAttrs{O: str("v"), D: 7}},
		{"a document element, both absent", emptyAttrsSchema, instanceOf("", ""), into[emptyAttrs](), emptyAttrs{D: 7}},
		{"a document element of no attribute", emptyBareSchema, instanceOf("", ""), into[struct{}](), struct{}{}},
		{"particles of a sequence, each present", emptySeqSchema, instanceOf("", `<s:a k="1"/><s:e></s:e><s:h k="2"/><s:m k="3"/>`), into[emptySeq](groupBranches),
			emptySeq{A: emptyCtT{K: str("1")}, H: &emptyCtT{K: str("2")}, G: emptyGroupM{M: emptyCtT{K: str("3")}}}},
		{"particles of a sequence, the optional ref absent, no attribute", emptySeqSchema, instanceOf("", `<s:a/><s:e/><s:m/>`), into[emptySeq](groupBranches),
			emptySeq{G: emptyGroupM{}}},
		{"a choice's alternative", emptyChoiceSchema, instanceOf("", `<s:x k="v"/>`), into[emptyChoice](choiceBranches), emptyChoice(emptyChoiceX{X: emptyCtT{K: str("v")}})},
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
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil for a row Decode decodes", err)
			}
		})
	}
}

// TestDecodeEmptyContentRefusesCharacters pins that Decode never decodes a
// document element of empty content that holds a character child, white space
// included, or an element child: validate charges each (cvc-complex-type
// clause 1.1), and Decode returns that charge and no value, though the struct's
// fill would skip the characters.
func TestDecodeEmptyContentRefusesCharacters(t *testing.T) {
	schema := schemaOf(t, emptyBareSchema)
	for _, content := range []string{" ", "x", `<s:r/>`} {
		got, err := Decode[struct{}](schema, strict.New(), instanceOf("", content))
		if got != nil || err == nil {
			t.Fatalf("Decode(%q) = (%v, %v), want no value and an error", content, got, err)
		}
		if rule, _ := xsderr.RuleOf(err); rule != "cvc-complex-type" {
			t.Errorf("Decode(%q): rule = %q, want cvc-complex-type: %v", content, rule, err)
		}
	}
}
