package codegen

import (
	"errors"
	"fmt"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	xsdparser "github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// parseString parses doc as the one schema document at location s.xsd.
func parseString(t *testing.T, doc string) *xsd.Schema {
	t.Helper()
	resolve := loader.ResolverFunc(func(_, location string) (io.ReadCloser, string, error) {
		if location != "s.xsd" {
			return nil, "", fmt.Errorf("no document %q", location)
		}
		return io.NopCloser(strings.NewReader(doc)), location, nil
	})
	schema, err := xsdparser.Parse("s.xsd", xsdparser.WithResolver(resolve))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return schema
}

// money is the name of the ·implementation-defined· primitive primitiveSchema
// mints.
var money = xsd.QName{Local: "money"}

// primitiveSchema parses body as line 2 of the one schema document s.xsd, line
// 3 declaring a simple type money that body may reference by name, and returns
// the schema built from the parsed components with money replaced by an
// ·implementation-defined· primitive of that name (Datatypes dt-primitive)
// minted through xsd.NewPrimitiveType: the uncovered primitive the package
// doc's "Coverage" defines, so every element or attribute declaration of type
// money that Generate reaches declines with that Construct. The rows that pin
// decline mechanics rather than one construct decline through it.
func primitiveSchema(t *testing.T, body string) *xsd.Schema {
	t.Helper()
	parsed := parseString(t, "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n"+body+"\n"+
		`<xs:simpleType name="money"><xs:restriction base="xs:string"/></xs:simpleType>`+"\n</xs:schema>\n")
	primitive, err := xsd.NewPrimitiveType(xsderr.Loc{}, money, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return replacedSchema(t, parsed, primitive)
}

// vacuous is the name of the complex type vacuousTypeSchema mints.
var vacuous = xsd.QName{Local: "vacuous"}

// vacuousTypeSchema parses body, followed on the same line by a declaration of
// a complex type vacuous that body may reference by name, as line 2 of the one
// schema document s.xsd, and returns the schema built from the parsed
// components with vacuous replaced by a complex type of that name, at the
// parsed declaration's Loc, whose content particle, built through
// xsd.NewParticle, is a sequence with {max occurs} 0: a particle no schema
// document yields (§3.4.2.3.3 dcl.ctd.ctcc.common clause 2.1.4), so every
// place Generate shapes vacuous declines it as a particle with {max occurs} 0
// (contentGroup). The rows that need a complex type outside the slice, and
// not one construct, decline through it.
func vacuousTypeSchema(t *testing.T, body string) *xsd.Schema {
	t.Helper()
	parsed := parseString(t, "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n"+body+
		`<xs:complexType name="vacuous"/>`+"\n</xs:schema>\n")
	declared, ok := parsed.Type(vacuous)
	if !ok {
		t.Fatalf("the parsed schema declares no type %s", vacuous)
	}
	none, err := xsd.NewOccurs(xsderr.Loc{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	group, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, nil)
	if err != nil {
		t.Fatal(err)
	}
	particle, err := xsd.NewParticle(xsderr.Loc{}, none, xsd.ResolvedTerm{Term: group})
	if err != nil {
		t.Fatal(err)
	}
	ct, err := xsd.NewComplexType(declared.Loc(), vacuous, anyTypeName, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.ElementContent{Particle: particle}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return replacedSchema(t, parsed, ct)
}

// replacedSchema returns the schema built from parsed's components with the
// type of replacement's name replaced by replacement.
func replacedSchema(t *testing.T, parsed *xsd.Schema, replacement xsd.TypeDefinition) *xsd.Schema {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	replaced := false
	for _, td := range parsed.Types() {
		if td.Name() != replacement.Name() {
			b.AddType(td)
			continue
		}
		b.AddType(replacement)
		replaced = true
	}
	if !replaced {
		t.Fatalf("the parsed schema declares no type %s", replacement.Name())
	}
	for _, d := range parsed.Elements() {
		b.AddElement(d)
	}
	for _, d := range parsed.Attributes() {
		b.AddAttribute(d)
	}
	for _, d := range parsed.AttributeGroups() {
		b.AddAttributeGroup(d)
	}
	for _, d := range parsed.ModelGroups() {
		b.AddModelGroup(d)
	}
	for _, d := range parsed.Notations() {
		b.AddNotation(d)
	}
	for _, d := range parsed.IdentityConstraints() {
		b.AddIdentityConstraint(d)
	}
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}

// pastDecline is the body of the "value cycle check past" rows: the content of
// r's anonymous complex type, the particles t holds after f1, and the
// declarations before root, in that order, fill its verbs.
const pastDecline = `<xs:complexType name="m"><xs:sequence><xs:element name="q" type="t"/><xs:element name="r"><xs:complexType>%s</xs:complexType></xs:element></xs:sequence></xs:complexType><xs:complexType name="t"><xs:sequence><xs:element name="f1" type="m" minOccurs="0"/>%s</xs:sequence></xs:complexType>%s<xs:element name="root"><xs:complexType><xs:sequence><xs:element name="x" type="t"/></xs:sequence></xs:complexType></xs:element>`

// TestGenerateDeclines pins each construct outside the slice: Generate
// returns a *DeclineError of the row's kind, wrapping errors.ErrUnsupported,
// whose text names the construct and its Loc, and writes nothing. Each body
// is one line, followed on it by vacuousTypeSchema's declaration of vacuous,
// so every Loc is on line 2.
func TestGenerateDeclines(t *testing.T) {
	const (
		str = `type="xs:string"`
		// cyc is a named complex type that reaches itself through s, which
		// occurs exactly once, and cycS the detail of its struct-value
		// recursion's decline: the rows that pin a decline's order against a
		// later one place an element of type cyc after it.
		cyc       = `<xs:complexType name="cyc"><xs:sequence><xs:element name="s" type="cyc"/></xs:sequence></xs:complexType>`
		cycS      = "local element s reaching complex type cyc again through elements that each occur exactly once"
		vacuousIn = "a particle of a sequence model group with {max occurs} 0 in complex type vacuous"
	)
	tests := []struct {
		name, body, detail string
		kind               construct
	}{
		// The group-cycle rows reach e again through a <group ref> to the
		// group declaring it (legal: mg-props-correct clause 2 bounds group
		// containment alone), as e's content particle and inside an inline
		// sequence, and as a named type's content.
		{"group cycle through content particle", `<xs:group name="g"><xs:sequence><xs:element name="e"><xs:complexType><xs:group ref="g"/></xs:complexType></xs:element></xs:sequence></xs:group><xs:element name="r"><xs:complexType><xs:group ref="g"/></xs:complexType></xs:element>`,
			"local element e reaching element e of model group g again through elements that each occur exactly once at s.xsd:2:33 ", constructValueCycle},
		{"group cycle through inline sequence", `<xs:group name="g"><xs:sequence><xs:element name="e"><xs:complexType><xs:sequence><xs:group ref="g"/></xs:sequence></xs:complexType></xs:element></xs:sequence></xs:group><xs:element name="r"><xs:complexType><xs:sequence><xs:group ref="g"/></xs:sequence></xs:complexType></xs:element>`,
			"local element e reaching element e of model group g again through elements that each occur exactly once at s.xsd:2:33 ", constructValueCycle},
		{"group cycle through named type", `<xs:complexType name="t"><xs:group ref="g"/></xs:complexType><xs:group name="g"><xs:sequence><xs:element name="e" type="t"/></xs:sequence></xs:group><xs:element name="r" type="t"/>`,
			"local element e reaching complex type t again through elements that each occur exactly once", constructValueCycle},
		// In the ref rows below, h is declared after e. Where a row places a
		// later decline after the ref, the ref's check is shown to come
		// first. h's type is checked where e's ref meets it, ahead of z's
		// decline.
		{"element ref to complex type outside the slice", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element ref="h"/><xs:element name="z" type="cyc"/></xs:sequence></xs:complexType></xs:element><xs:element name="h" type="vacuous"/>` + cyc,
			vacuousIn + " of element h at s.xsd:2:290 ", constructVacuousParticle},
		// A type with {assertions} still declines for its other properties.
		{"value cycle with assertion", `<xs:complexType name="c"><xs:sequence><xs:element name="x" type="c"/></xs:sequence><xs:assert test="true()"/></xs:complexType><xs:element name="e" type="c"/>`,
			"local element x reaching complex type c again through elements that each occur exactly once at s.xsd:2:39", constructValueCycle},
		// d, a member of the abstract type c, holds x of type d once: the
		// branch for d holds d as a struct value, which holds itself.
		{"value cycle through an abstract type member", `<xs:complexType name="c" abstract="true"/><xs:complexType name="f"><xs:complexContent><xs:restriction base="c"/></xs:complexContent></xs:complexType><xs:complexType name="d"><xs:complexContent><xs:extension base="c"><xs:sequence><xs:element name="x" type="d"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType><xs:element name="e" type="c"/>`,
			"local element x reaching complex type d again through elements that each occur exactly once", constructValueCycle},
		// The value-cycle rows each give Go types that would hold one another
		// as struct values.
		{"value cycle direct", `<xs:complexType name="c"><xs:sequence><xs:element name="x" type="c"/></xs:sequence></xs:complexType><xs:element name="e" type="c"/>`,
			"local element x reaching complex type c again through elements that each occur exactly once at s.xsd:2:39", constructValueCycle},
		{"value cycle mutual", `<xs:complexType name="c1"><xs:sequence><xs:element name="x" type="c2"/></xs:sequence></xs:complexType><xs:complexType name="c2"><xs:sequence><xs:element name="y" type="c1"/></xs:sequence></xs:complexType><xs:element name="e" type="c1"/>`,
			"local element y reaching complex type c1 again through elements that each occur exactly once", constructValueCycle},
		{"value cycle through anonymous type", `<xs:complexType name="c"><xs:sequence><xs:element name="x"><xs:complexType><xs:sequence><xs:element name="y" type="c"/></xs:sequence></xs:complexType></xs:element></xs:sequence></xs:complexType><xs:element name="e" type="c"/>`,
			"local element y reaching complex type c again through elements that each occur exactly once", constructValueCycle},
		{"value cycle in choice branch", `<xs:complexType name="t1"><xs:sequence><xs:element name="b" type="t2"/></xs:sequence></xs:complexType><xs:complexType name="t2"><xs:sequence><xs:element name="c" type="t1"/></xs:sequence></xs:complexType><xs:element name="e"><xs:complexType><xs:choice><xs:element name="a" type="t1"/></xs:choice></xs:complexType></xs:element>`,
			"local element c reaching complex type t1 again through elements that each occur exactly once", constructValueCycle},
		// An all group's member occurring exactly once is a value edge of the
		// emitting walk as a sequence's is, inline and spliced from an all
		// group nested through a <group ref> (cos-all-limited clause 1.3).
		{"value cycle through all", `<xs:complexType name="c"><xs:all><xs:element name="x" type="c"/></xs:all></xs:complexType><xs:element name="e" type="c"/>`,
			"local element x reaching complex type c again through elements that each occur exactly once at s.xsd:2:34", constructValueCycle},
		{"value cycle through nested all", `<xs:group name="g"><xs:all><xs:element name="x" type="c"/></xs:all></xs:group><xs:complexType name="c"><xs:all><xs:element name="y" ` + str + `/><xs:group ref="g"/></xs:all></xs:complexType><xs:element name="e" type="c"/>`,
			"local element x reaching complex type c again through elements that each occur exactly once at s.xsd:2:28", constructValueCycle},
		// The ref-cycle rows close a cycle through <element ref> particles,
		// which the emitting walk treats as leaves, so only the recursion check
		// of each ref finds it; each decline cites the Loc of the top-level
		// declaration the closing ref names.
		{"ref cycle through a named type", `<xs:element name="r" type="c"/><xs:complexType name="c"><xs:sequence><xs:element ref="r"/></xs:sequence></xs:complexType>`,
			"<element ref> particle to element r reaching element r again through elements that each occur exactly once at s.xsd:2:1 ", constructValueCycle},
		// The same holds for the recursion check (valueCycle), which alone
		// finds a cycle closed by an <element ref> particle: these rows fail
		// when it ends its walk at an all group.
		{"ref cycle through all", `<xs:element name="r" type="c"/><xs:complexType name="c"><xs:all><xs:element ref="r"/></xs:all></xs:complexType>`,
			"<element ref> particle to element r reaching element r again through elements that each occur exactly once at s.xsd:2:1 ", constructValueCycle},
		{"ref cycle through nested all", `<xs:element name="r" type="c"/><xs:group name="g"><xs:all><xs:element ref="r"/></xs:all></xs:group><xs:complexType name="c"><xs:all><xs:element name="y" ` + str + `/><xs:group ref="g"/></xs:all></xs:complexType>`,
			"<element ref> particle to element r reaching element r again through elements that each occur exactly once at s.xsd:2:1 ", constructValueCycle},
		{"ref cycle through anonymous types", `<xs:element name="a"><xs:complexType><xs:sequence><xs:element ref="b"/></xs:sequence></xs:complexType></xs:element><xs:element name="b"><xs:complexType><xs:sequence><xs:element ref="a"/></xs:sequence></xs:complexType></xs:element>`,
			"<element ref> particle to element a reaching element a again through elements that each occur exactly once at s.xsd:2:1 ", constructValueCycle},
		// g's own element node is new to the chain f starts, and the type t
		// it shares with f closes the cycle.
		{"ref cycle closing at a complex type", `<xs:element name="f" type="t"/><xs:complexType name="t"><xs:sequence><xs:element ref="g"/></xs:sequence></xs:complexType><xs:element name="g" type="t"/>`,
			"<element ref> particle to element g reaching complex type t again through elements that each occur exactly once", constructValueCycle},
		// Each ref sits in a nested sequence occurring exactly once, so the
		// recursion check finds the cycle only by walking a's and b's
		// particles as particleFields does (spliced).
		{"ref cycle through nested sequences", `<xs:element name="a"><xs:complexType><xs:sequence><xs:sequence><xs:element ref="b"/></xs:sequence></xs:sequence></xs:complexType></xs:element><xs:element name="b"><xs:complexType><xs:sequence><xs:sequence><xs:element ref="a"/></xs:sequence></xs:sequence></xs:complexType></xs:element>`,
			"<element ref> particle to element a reaching element a again through elements that each occur exactly once at s.xsd:2:1 ", constructValueCycle},
		// In the two extension rows each ref is spliced into a's or b's
		// fields from the base particle, or from the extension's own, of a
		// clause 4.2.3.3 sequence, so the recursion check finds the cycle
		// only by walking the particles spliced returns.
		{"ref cycle through base particles", `<xs:complexType name="ba"><xs:sequence><xs:element ref="b"/></xs:sequence></xs:complexType><xs:complexType name="bb"><xs:sequence><xs:element ref="a"/></xs:sequence></xs:complexType><xs:element name="a"><xs:complexType><xs:complexContent><xs:extension base="ba"><xs:sequence><xs:element name="s" ` + str + `/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:element name="b"><xs:complexType><xs:complexContent><xs:extension base="bb"><xs:sequence><xs:element name="s" ` + str + `/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`,
			"<element ref> particle to element a reaching element a again through elements that each occur exactly once", constructValueCycle},
		{"ref cycle through extension particles", `<xs:complexType name="be"><xs:sequence><xs:element name="s" ` + str + `/></xs:sequence></xs:complexType><xs:element name="a"><xs:complexType><xs:complexContent><xs:extension base="be"><xs:sequence><xs:element ref="b"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:element name="b"><xs:complexType><xs:complexContent><xs:extension base="be"><xs:sequence><xs:element ref="a"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:element>`,
			"<element ref> particle to element a reaching element a again through elements that each occur exactly once", constructValueCycle},
		{"ref cycle through a local element", `<xs:element name="a"><xs:complexType><xs:sequence><xs:element name="l"><xs:complexType><xs:sequence><xs:element ref="a"/></xs:sequence></xs:complexType></xs:element></xs:sequence></xs:complexType></xs:element>`,
			"<element ref> particle to element a reaching element a again through elements that each occur exactly once", constructValueCycle},
		// x's walk emits z through the pointer field p before it reaches the
		// value field v, and z's walk meets x already stored: a check carried
		// by the emitting walk, which never walks a stored type again, lets
		// the cycle v, w through.
		{"value cycle behind a pointer", `<xs:complexType name="x"><xs:sequence><xs:element name="p" type="z" minOccurs="0"/><xs:element name="v" type="z"/></xs:sequence></xs:complexType><xs:complexType name="z"><xs:sequence><xs:element name="w" type="x"/></xs:sequence></xs:complexType><xs:element name="e"><xs:complexType><xs:sequence><xs:element name="a" type="x"/></xs:sequence></xs:complexType></xs:element>`,
			"local element v reaching complex type z again through elements that each occur exactly once", constructValueCycle},
		// q reaches t while t's body is being emitted, so the recursion check
		// walks t again past f1 and meets a complex type outside the slice
		// as a decline it leaves to the emitting walk, which returns the
		// recursion r's element s closes first.
		{"value cycle check past a complex type", fmt.Sprintf(pastDecline, `<xs:sequence><xs:element name="s" type="cyc"/></xs:sequence>`, `<xs:element name="g" type="vacuous"/>`, cyc),
			cycS, constructValueCycle},
		// h's complex alternative is checked where e's ref meets it, ahead of
		// z's decline.
		{"element ref to type table outside the slice", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element ref="h"/><xs:element name="z" type="cyc"/></xs:sequence></xs:complexType></xs:element><xs:element name="h"><xs:alternative test="@k" type="vacuous"/><xs:alternative type="xs:string"/></xs:element>` + cyc,
			vacuousIn + " at s.xsd:2:363 ", constructVacuousParticle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFirstDecline(t, vacuousTypeSchema(t, tt.body), tt.kind, tt.detail)
		})
	}
}

// assertFirstDecline pins that Generate, given schema, returns failures none
// of which is an *xsderr.Error or holds a misused format verb, the first
// wrapping errors.ErrUnsupported and holding a *DeclineError that has
// Construct kind and a text holding detail, a Loc on line 2 of s.xsd and the
// refusal's wording, and writes nothing.
func assertFirstDecline(t *testing.T, schema *xsd.Schema, kind construct, detail string) {
	t.Helper()
	out := filepath.Join(moduleTempDir(t), "out")
	errs := Generate([]Target{{Schema: schema, Dir: out, Package: "p"}})
	err := firstFailure(t, errs)
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error %v does not wrap errors.ErrUnsupported", err)
	}
	for _, e := range errs {
		var xe *xsderr.Error
		if errors.As(e, &xe) {
			t.Errorf("error %v is an *xsderr.Error, want a plain error", e)
		}
		if strings.Contains(e.Error(), "%!") {
			t.Errorf("error %q holds a misused format verb", e)
		}
	}
	var de *DeclineError
	if !errors.As(err, &de) {
		t.Fatalf("error %v is not a *DeclineError", err)
	}
	if got, want := de.Construct(), (Construct{kind}); got != want {
		t.Errorf("Construct() = %v, want %v", got, want)
	}
	if !strings.HasPrefix(err.Error(), "codegen: target 0: ") {
		t.Errorf("error %q does not open with %q", err, "codegen: target 0: ")
	}
	for _, want := range []string{detail, " at s.xsd:2:", "a generator refusal, not a schema fault"} {
		if !strings.Contains(de.Error(), want) {
			t.Errorf("first decline %q does not contain %q", de, want)
		}
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Dir %s exists after a decline (stat: %v)", out, err)
	}
}

// TestGenerateDeclineMechanics pins which decline Generate meets first and how
// its text names the declining place, every decline but a struct-value
// recursion being primitiveSchema's money: Generate's first *DeclineError is
// the row's, as assertFirstDecline states. Each body is one line, so every Loc
// is on line 2.
func TestGenerateDeclineMechanics(t *testing.T) {
	const (
		amount    = `<xs:attribute name="amount" type="money"/>`
		anon      = `<xs:complexType>` + amount + `</xs:complexType>`
		str       = `type="xs:string"`
		primitive = " with {type definition} money, whose {primitive type definition} is money at s.xsd:2:"
		amountR   = "attribute amount of complex type (anonymous) of element r of complex type m" + primitive + "107 "
	)
	tests := []struct {
		name, body, detail string
		kind               construct
	}{
		// The decline under p precedes the cycle v closes
		// in particle order, so it is the decline returned.
		{"value cycle after an earlier decline", `<xs:complexType name="c"><xs:sequence><xs:element name="p" minOccurs="0">` + anon + `</xs:element><xs:element name="v" type="c"/></xs:sequence></xs:complexType><xs:element name="e"><xs:complexType><xs:sequence><xs:element name="l" type="c"/></xs:sequence></xs:complexType></xs:element>`,
			"attribute amount of complex type (anonymous) of element p of complex type c" + primitive + "90 ", constructUncoveredPrimitive},
		// In the two rows below, q reaches t while t's body is being emitted,
		// so the recursion check walks t again past f1: it passes the
		// wildcard, and a nillable element of type t itself, as leaves, and
		// the emitting walk returns r's decline first.
		{"value cycle check past a wildcard", fmt.Sprintf(pastDecline, amount, `<xs:any/>`, ""),
			amountR, constructUncoveredPrimitive},
		{"value cycle check past a nillable element", fmt.Sprintf(pastDecline, amount, `<xs:element name="g" nillable="true" type="t"/>`, ""),
			amountR, constructUncoveredPrimitive},
		// The same walk of t again meets the recursion s closes after f1, and
		// Generate's doc puts that recursion ahead of r's decline.
		{"value cycle check past a recursion", fmt.Sprintf(pastDecline, amount, `<xs:element name="g" type="cyc"/>`, `<xs:complexType name="cyc"><xs:sequence><xs:element name="s" type="cyc"/></xs:sequence></xs:complexType>`),
			"local element s reaching complex type cyc again through elements that each occur exactly once", constructValueCycle},
		// The ref to a is checked for recursion where it is met, ahead of the
		// decline of z after it.
		{"ref cycle ahead of a later decline", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element ref="a"/><xs:element name="z" type="money"/></xs:sequence></xs:complexType></xs:element><xs:element name="a"><xs:complexType><xs:sequence><xs:element ref="b"/></xs:sequence></xs:complexType></xs:element><xs:element name="b"><xs:complexType><xs:sequence><xs:element ref="a"/></xs:sequence></xs:complexType></xs:element>`,
			"<element ref> particle to element a reaching element a again through elements that each occur exactly once", constructValueCycle},
		// e's walk reaches t, declared after f, before f's walk begins.
		{"named type body before a later top-level element", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element name="l" type="t"/></xs:sequence></xs:complexType></xs:element><xs:element name="f" type="money"/><xs:complexType name="t">` + amount + `</xs:complexType>`,
			"attribute amount of complex type t" + primitive + "186 ", constructUncoveredPrimitive},
		// An element of the abstract type c is emitted from c's members f and
		// d; d's own attribute declines the element, charged to d as its own.
		{"abstract type member outside the slice", `<xs:complexType name="c" abstract="true"/><xs:complexType name="f"><xs:complexContent><xs:restriction base="c"/></xs:complexContent></xs:complexType><xs:complexType name="d"><xs:complexContent><xs:extension base="c">` + amount + `</xs:extension></xs:complexContent></xs:complexType><xs:element name="e" type="c"/>`,
			"attribute amount of complex type d" + primitive, constructUncoveredPrimitive},
		// The rows below pin that a decline in a type names the type's
		// declaring place: a named complex type a local element reaches by
		// its name alone, an anonymous one nested in a struct, in a struct
		// holding a choice, or in a choice's branch by the path to it.
		{"named complex local body", `<xs:complexType name="c">` + amount + `</xs:complexType><xs:element name="e"><xs:complexType><xs:sequence><xs:element name="l" type="c"/></xs:sequence></xs:complexType></xs:element>`,
			"attribute amount of complex type c" + primitive + "26 ", constructUncoveredPrimitive},
		{"nested struct", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element name="l"><xs:complexType><xs:sequence><xs:element name="a" ` + str + `/></xs:sequence>` + amount + `</xs:complexType></xs:element></xs:sequence></xs:complexType></xs:element>`,
			"attribute amount of complex type (anonymous) of element l of element e" + primitive + "154 ", constructUncoveredPrimitive},
		{"nested choice", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element name="l"><xs:complexType><xs:choice><xs:element name="a" ` + str + `/><xs:element name="b" ` + str + `/></xs:choice>` + amount + `</xs:complexType></xs:element></xs:sequence></xs:complexType></xs:element>`,
			"attribute amount of complex type (anonymous) of element l of element e" + primitive + "189 ", constructUncoveredPrimitive},
		{"branch struct", `<xs:element name="e"><xs:complexType><xs:choice><xs:element name="l"><xs:complexType><xs:sequence><xs:element name="a" ` + str + `/></xs:sequence>` + amount + `</xs:complexType></xs:element></xs:choice></xs:complexType></xs:element>`,
			"attribute amount of complex type (anonymous) of element l of element e" + primitive + "152 ", constructUncoveredPrimitive},
		// The {type table} rows pin that a decline in a type the table selects
		// names that type's slot: an anonymous complex type by its slot and
		// the place of the declaration.
		{"type table anonymous alternative", `<xs:element name="e"><xs:alternative test="@k">` + anon + `</xs:alternative><xs:alternative type="xs:string"/></xs:element>`,
			"attribute amount of complex type (anonymous) of {type table} alternative 1 of element e" + primitive + "64 ", constructUncoveredPrimitive},
		{"type table anonymous default", `<xs:element name="e"><xs:alternative test="@k" type="xs:string"/><xs:alternative><xs:complexType><xs:sequence><xs:element name="l">` + anon + `</xs:element></xs:sequence></xs:complexType></xs:alternative></xs:element>`,
			"attribute amount of complex type (anonymous) of element l of {default type definition} of element e" + primitive + "148 ", constructUncoveredPrimitive},
		{"type table local", `<xs:complexType name="c"><xs:sequence><xs:element name="x"><xs:alternative test="@k">` + anon + `</xs:alternative><xs:alternative type="xs:string"/></xs:element></xs:sequence></xs:complexType><xs:element name="e" type="c"/>`,
			"attribute amount of complex type (anonymous) of {type table} alternative 1 of element x of complex type c" + primitive + "102 ", constructUncoveredPrimitive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFirstDecline(t, primitiveSchema(t, tt.body), tt.kind, tt.detail)
		})
	}
}

// TestGenerateDeclinesLocalAnonymousPlace pins that a decline met in a local
// element's anonymous complex type names the type's declaring place, and that
// Generate reports each site, in document order, while one site reached by two
// walks stays one element. Two sites charged at one Loc, here the attribute of
// the complex type ab two top-level elements reference, print distinct texts
// and Generate reports each. Every decline is primitiveSchema's money.
func TestGenerateDeclinesLocalAnonymousPlace(t *testing.T) {
	const (
		amount = `<xs:attribute name="amount" type="money"/>`
		anon   = `<xs:complexType>` + amount + `</xs:complexType>`
		x      = `<xs:element name="x">` + anon + `</xs:element>`
		a      = `<xs:element name="a"><xs:complexType><xs:sequence>` + x + `</xs:sequence></xs:complexType></xs:element>`
	)
	top := func(name, content string) string {
		return `<xs:element name="` + name + `"><xs:complexType>` + content + `</xs:complexType></xs:element>`
	}
	seq := func(particle string) string { return `<xs:sequence>` + particle + `</xs:sequence>` }
	decline := func(subject, col string) string {
		return "codegen: target 0: attribute amount of " + subject + " with {type definition} money, whose {primitive type definition} is money at s.xsd:2:" + col + " is not supported by codegen, a generator refusal, not a schema fault"
	}
	tests := []struct {
		name, body string
		want       []string
	}{
		{"two top-level elements", top("p", seq(x)) + top("q", seq(x)), []string{
			decline("complex type (anonymous) of element x of element p", "88"),
			decline("complex type (anonymous) of element x of element q", "291"),
		}},
		{"one model group definition", `<xs:group name="m">` + seq(x) + `</xs:group>` + top("p", `<xs:group ref="m"/>`) + top("q", `<xs:group ref="m"/>`), []string{
			decline("complex type (anonymous) of element x of model group m", "70"),
		}},
		{"named complex type and top-level element", `<xs:complexType name="c">` + seq(x) + `</xs:complexType><xs:element name="r" type="c"/>` + top("p", seq(x)), []string{
			decline("complex type (anonymous) of element x of complex type c", "76"),
			decline("complex type (anonymous) of element x of element p", "297"),
		}},
		{"top-level and local element", `<xs:element name="x">` + anon + `</xs:element>` + top("p", seq(x)), []string{
			decline("complex type (anonymous) of element x", "38"),
			decline("complex type (anonymous) of element x of element p", "197"),
		}},
		{"two levels down", top("p", seq(a)) + top("q", seq(a)), []string{
			decline("complex type (anonymous) of element x of element a of element p", "138"),
			decline("complex type (anonymous) of element x of element a of element q", "435"),
		}},
		{"two top-level elements of one complex type", `<xs:complexType name="ab">` + amount + `</xs:complexType><xs:element name="r" type="ab"/><xs:element name="t" type="ab"/>`, []string{
			decline("complex type ab of element r", "27"),
			decline("complex type ab of element t", "27"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Generate([]Target{{Schema: primitiveSchema(t, tt.body), Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"}})
			got := make([]string, len(errs))
			for i, err := range errs {
				got[i] = err.Error()
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Generate's elements =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestGenerateDeclinesSubstitutionMemberOnce pins that a member of a
// substitution group whose type declines is charged once, though the walk
// meets it twice: at the branch of the interface e's ref to h emits
// (substitutionType), and at m's own turn. emit's dedup holds because both
// visits build the decline's text and Loc from m and its type alone; without
// it each row reports the decline twice. The head, whose {type definition}
// is never shaped where it has no branch, is charged nothing, abstract or
// not.
func TestGenerateDeclinesSubstitutionMemberOnce(t *testing.T) {
	const (
		holder = `<xs:element name="e"><xs:complexType><xs:sequence><xs:element ref="h"/></xs:sequence></xs:complexType></xs:element>`
		member = `<xs:element name="m" substitutionGroup="h" type="money"/>`
	)
	decline := func(loc string) []string {
		return []string{"codegen: target 0: element m with {type definition} money, whose {primitive type definition} is money at s.xsd:" + loc + " is not supported by codegen, a generator refusal, not a schema fault"}
	}
	for _, tt := range []struct {
		name, head string
		want       []string
	}{
		{"abstract head", `<xs:element name="h" abstract="true" type="xs:anyType"/>`, decline("2:172")},
		{"head with {abstract} false", `<xs:element name="h" type="xs:anyType"/>`, decline("2:156")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			errs := Generate([]Target{{Schema: primitiveSchema(t, holder+tt.head+member), Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"}})
			got := make([]string, len(errs))
			for i, err := range errs {
				got[i] = err.Error()
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Generate's elements =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestGenerateDeclinesVacuousParticle pins the decline of a particle whose
// {max occurs} is 0, naming its term: a local element, a wildcard, the
// sequence model group of a content particle, or a choice model group nested
// in it. No schema document yields one, since an <element> or <any> with
// minOccurs=maxOccurs=0 maps to no component at all (§3.3.2, §3.10.2) and a
// <sequence> or <choice> with maxOccurs=0 makes the content empty (§3.4.2.3.3
// dcl.ctd.ctcc.common 2.1.4), but xsd.NewParticle and xsd.NewComplexType
// accept a vacuous Occurs, so the schema is built through the constructors.
func TestGenerateDeclinesVacuousParticle(t *testing.T) {
	ctName := xsd.QName{Local: "c"}
	scope, err := xsd.NewLocalScope(xsderr.Loc{}, xsd.ComplexTypeScopeParent{Name: ctName})
	if err != nil {
		t.Fatal(err)
	}
	local, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "a"},
		xsd.TypeDefinitionRef{Name: stringPrimitive}, nil, scope, nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	anyNamespace, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wildcard, err := xsd.NewWildcard(xsderr.Loc{}, anyNamespace, xsd.ProcessLax)
	if err != nil {
		t.Fatal(err)
	}
	vacuous, err := xsd.NewOccurs(xsderr.Loc{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	localParticle, err := xsd.NewParticle(xsderr.Loc{}, exactlyOnce, xsd.ResolvedTerm{Term: local})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorChoice, []xsd.Particle{localParticle})
	if err != nil {
		t.Fatal(err)
	}
	unbounded, err := xsd.NewUnboundedOccurs(xsderr.Loc{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		term         xsd.Term
		inner, outer xsd.Occurs
		want         string
	}{
		{"element", local, vacuous, exactlyOnce, "codegen: target 0: a particle of element a with {max occurs} 0 in complex type c of element e at "},
		{"wildcard", wildcard, vacuous, exactlyOnce, "codegen: target 0: a particle of a wildcard with {max occurs} 0 in complex type c of element e at "},
		{"content particle", local, exactlyOnce, vacuous, "codegen: target 0: a particle of a sequence model group with {max occurs} 0 in complex type c of element e at "},
		{"nested group", nested, vacuous, exactlyOnce, "codegen: target 0: a particle of a choice model group with {max occurs} 0 in complex type c of element e at "},
		// Repeated without bound, the content sequence is emptiable and is
		// flattened: reach declines each vacuous particle in its place.
		{"element in a flattened group", local, vacuous, unbounded, "codegen: target 0: a particle of element a with {max occurs} 0 in complex type c of element e at "},
		{"wildcard in a flattened group", wildcard, vacuous, unbounded, "codegen: target 0: a particle of a wildcard with {max occurs} 0 in complex type c of element e at "},
		{"nested group in a flattened group", nested, vacuous, unbounded, "codegen: target 0: a particle of a choice model group with {max occurs} 0 in complex type c of element e at "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := vacuousSchema(t, ctName, tt.term, tt.inner, tt.outer)
			out := filepath.Join(moduleTempDir(t), "out")
			err := firstFailure(t, Generate([]Target{{Schema: schema, Dir: out, Package: "p"}}))
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Fatalf("Generate = %v, want a decline wrapping errors.ErrUnsupported", err)
			}
			if !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("error %q does not start with %q", err, tt.want)
			}
			var de *DeclineError
			if !errors.As(err, &de) || de.Construct() != (Construct{constructVacuousParticle}) {
				t.Fatalf("Generate = %v, want a *DeclineError of Construct %v", err, Construct{constructVacuousParticle})
			}
			// Every component is built at the zero Loc, so DeclineError.Loc's
			// doc makes the decline's Loc zero.
			if loc := de.Loc(); loc != (xsderr.Loc{}) {
				t.Errorf("Loc() = %+v, want the zero xsderr.Loc", loc)
			}
			if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Dir %s exists after a decline (stat: %v)", out, err)
			}
		})
	}
}

// vacuousSchema builds the schema TestGenerateDeclinesVacuousParticle
// generates from: a top-level element e of the complex type ctName, whose
// content particle, occurring outerOccurs times, is a sequence of one particle
// of term occurring innerOccurs times.
func vacuousSchema(t *testing.T, ctName xsd.QName, term xsd.Term, innerOccurs, outerOccurs xsd.Occurs) *xsd.Schema {
	t.Helper()
	inner, err := xsd.NewParticle(xsderr.Loc{}, innerOccurs, xsd.ResolvedTerm{Term: term})
	if err != nil {
		t.Fatal(err)
	}
	group, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{inner})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := xsd.NewParticle(xsderr.Loc{}, outerOccurs, xsd.ResolvedTerm{Term: group})
	if err != nil {
		t.Fatal(err)
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, ctName, anyTypeName, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.ElementContent{Particle: outer}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "e"},
		xsd.TypeDefinitionRef{Name: ctName}, nil, xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The builtins and an xs:anyType stand-in are what the local element's
	// type and c's base resolve against.
	anyType, err := xsd.NewComplexType(xsderr.Loc{}, anyTypeName, anyTypeName, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := builtin.Seed(strict.New())
	if err != nil {
		t.Fatal(err)
	}
	b := xsd.NewSchemaBuilder()
	for _, st := range seeded {
		b.AddType(st)
	}
	b.AddType(anyType)
	b.AddType(ct)
	b.AddElement(e)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}

// TestGenerateDeclinesAbstractLocal pins the decline of a local element
// declaration with {abstract} true, charged to that declaration's Loc, which no
// schema document yields (xs:localElement prohibits abstract, Structures
// Appendix A, the schema for schemas), so the schema is built through the
// constructors: declared by the complex type c holding its particle, and by a
// model group definition g a <group ref> particle in c names. The "cycle" row
// is a named complex type t whose sequence holds one such declaration a of type
// t: an <element ref> particle to r, of type t, in the earlier element x's type
// has valueCycle walk t before r's own turn, and the decline at a, from that
// turn, is the only one. The "flattened" row reaches a as a basic particle of
// a flattened choice (reach), at its branch.
func TestGenerateDeclinesAbstractLocal(t *testing.T) {
	aLoc := xsderr.Loc{URI: "s.xsd", Line: 3, Col: 5}
	tests := []struct {
		name   string
		schema *xsd.Schema
		want   []string
	}{
		{"complex type", abstractLocalSchema(t, aLoc, false),
			[]string{"codegen: target 0: local element a with {abstract} true in complex type c of element e at s.xsd:3:5 is not supported by codegen"}},
		{"model group", abstractLocalSchema(t, aLoc, true),
			[]string{"codegen: target 0: local element a with {abstract} true in complex type c of element e at s.xsd:3:5 is not supported by codegen"}},
		{"cycle", abstractCycleSchema(t, aLoc),
			[]string{"codegen: target 0: local element a with {abstract} true in complex type t of element r at s.xsd:3:5 is not supported by codegen"}},
		{"flattened", abstractFlatSchema(t, aLoc),
			[]string{"codegen: target 0: local element a with {abstract} true in complex type c of element e at s.xsd:3:5 is not supported by codegen"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := filepath.Join(moduleTempDir(t), "out")
			errs := Generate([]Target{{Schema: tt.schema, Dir: out, Package: "p"}})
			got := make([]string, len(errs))
			for i, err := range errs {
				got[i] = err.Error()
				var de *DeclineError
				if !errors.As(err, &de) || de.Construct() != (Construct{constructAbstractLocal}) || de.Loc() != aLoc {
					t.Errorf("element %d = %v, want a *DeclineError of Construct %v at %v", i, err, Construct{constructAbstractLocal}, aLoc)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Generate's elements =\n%q\nwant %d opening\n%q", got, len(tt.want), tt.want)
			}
			for i, want := range tt.want {
				if !strings.HasPrefix(got[i], want) {
					t.Errorf("element %d = %q, want it to open %q", i, got[i], want)
				}
			}
			if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Dir %s exists after a decline (stat: %v)", out, err)
			}
		})
	}
}

// abstractLocalSchema builds a schema TestGenerateDeclinesAbstractLocal
// generates from: a top-level element e of the complex type c, whose content is
// a sequence holding a local element a of xs:string with {abstract} true, at
// loc; when inGroup, the model group definition g declares a and c's sequence
// holds a <group ref> particle to g.
func abstractLocalSchema(t *testing.T, loc xsderr.Loc, inGroup bool) *xsd.Schema {
	t.Helper()
	ctName, gName := xsd.QName{Local: "c"}, xsd.QName{Local: "g"}
	var parent xsd.ElementScopeParent = xsd.ComplexTypeScopeParent{Name: ctName}
	if inGroup {
		parent = xsd.ModelGroupScopeParent{Name: gName}
	}
	a := abstractLocalDecl(t, loc, parent, stringPrimitive)
	b := seededBuilder(t)
	var term xsd.TermOrRef = xsd.ResolvedTerm{Term: sequenceOf(t, xsd.ResolvedTerm{Term: a})}
	if inGroup {
		g, err := xsd.NewModelGroupDefinition(xsderr.Loc{}, gName, sequenceOf(t, xsd.ResolvedTerm{Term: a}))
		if err != nil {
			t.Fatal(err)
		}
		b.AddModelGroup(g)
		term = xsd.ResolvedTerm{Term: sequenceOf(t, xsd.ModelGroupRef{Name: gName})}
	}
	b.AddType(complexOf(t, ctName, term))
	b.AddElement(globalDecl(t, "e", ctName))
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}

// abstractCycleSchema builds the "cycle" row's schema: the complex type t,
// whose sequence holds a local element a of type t with {abstract} true, at
// loc; the complex type u, whose sequence holds an <element ref> particle to
// r; and the top-level elements x of type u and r of type t, in that order.
func abstractCycleSchema(t *testing.T, loc xsderr.Loc) *xsd.Schema {
	t.Helper()
	tName, uName := xsd.QName{Local: "t"}, xsd.QName{Local: "u"}
	a := abstractLocalDecl(t, loc, xsd.ComplexTypeScopeParent{Name: tName}, tName)
	b := seededBuilder(t)
	b.AddType(complexOf(t, tName, xsd.ResolvedTerm{Term: sequenceOf(t, xsd.ResolvedTerm{Term: a})}))
	b.AddType(complexOf(t, uName, xsd.ResolvedTerm{Term: sequenceOf(t, xsd.ElementDeclarationRef{Name: xsd.QName{Local: "r"}})}))
	b.AddElement(globalDecl(t, "x", uName))
	b.AddElement(globalDecl(t, "r", tName))
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}

// abstractFlatSchema builds the "flattened" row's schema: a top-level element e
// of the complex type c, whose content is a choice of a local element a of
// xs:string with {abstract} true, at loc, and an empty sequence, an emptiable
// model group alternative, so that the choice is flattened and a is reached as
// a basic particle of its reach.
func abstractFlatSchema(t *testing.T, loc xsderr.Loc) *xsd.Schema {
	t.Helper()
	ctName := xsd.QName{Local: "c"}
	a := abstractLocalDecl(t, loc, xsd.ComplexTypeScopeParent{Name: ctName}, stringPrimitive)
	empty, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, nil)
	if err != nil {
		t.Fatal(err)
	}
	var particles []xsd.Particle
	for _, term := range []xsd.Term{a, empty} {
		p, err := xsd.NewParticle(xsderr.Loc{}, exactlyOnce, xsd.ResolvedTerm{Term: term})
		if err != nil {
			t.Fatal(err)
		}
		particles = append(particles, p)
	}
	choice, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorChoice, particles)
	if err != nil {
		t.Fatal(err)
	}
	b := seededBuilder(t)
	b.AddType(complexOf(t, ctName, xsd.ResolvedTerm{Term: choice}))
	b.AddElement(globalDecl(t, "e", ctName))
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}

// abstractLocalDecl builds a local element declaration a, at loc, of the type
// named typ, with {abstract} true and {scope}.{parent} parent.
func abstractLocalDecl(t *testing.T, loc xsderr.Loc, parent xsd.ElementScopeParent, typ xsd.QName) xsd.ElementDeclaration {
	t.Helper()
	scope, err := xsd.NewLocalScope(xsderr.Loc{}, parent)
	if err != nil {
		t.Fatal(err)
	}
	a, err := xsd.NewElementDeclaration(loc, xsd.QName{Local: "a"},
		xsd.TypeDefinitionRef{Name: typ}, nil, scope, nil, false, nil, nil, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// globalDecl builds a top-level element declaration named local of the type
// named typ.
func globalDecl(t *testing.T, local string, typ xsd.QName) xsd.ElementDeclaration {
	t.Helper()
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: local},
		xsd.TypeDefinitionRef{Name: typ}, nil, xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// sequenceOf builds a sequence model group of one particle of term, occurring
// exactly once.
func sequenceOf(t *testing.T, term xsd.TermOrRef) xsd.ModelGroup {
	t.Helper()
	p, err := xsd.NewParticle(xsderr.Loc{}, exactlyOnce, term)
	if err != nil {
		t.Fatal(err)
	}
	g, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{p})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// complexOf builds the complex type name, restricting xs:anyType, whose
// element-only content particle, occurring exactly once, has term.
func complexOf(t *testing.T, name xsd.QName, term xsd.TermOrRef) xsd.ComplexType {
	t.Helper()
	p, err := xsd.NewParticle(xsderr.Loc{}, exactlyOnce, term)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, name, anyTypeName, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.ElementContent{Particle: p}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ct
}

// seededBuilder returns a SchemaBuilder holding the builtins and an xs:anyType
// stand-in, which the types it is given resolve against.
func seededBuilder(t *testing.T) *xsd.SchemaBuilder {
	t.Helper()
	anyType, err := xsd.NewComplexType(xsderr.Loc{}, anyTypeName, anyTypeName, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := builtin.Seed(strict.New())
	if err != nil {
		t.Fatal(err)
	}
	b := xsd.NewSchemaBuilder()
	for _, st := range seeded {
		b.AddType(st)
	}
	b.AddType(anyType)
	return b
}

// TestGenerateEmptyUnion pins that a union with no {member type definitions},
// which §3.16.1 admits and xsd.NewSimpleType accepts though no schema document
// yields one (src-union-memberTypes-or-simpleTypes), generates its interface
// alone, which no variant implements, and so does v, whose one member is u.
func TestGenerateEmptyUnion(t *testing.T) {
	u, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Local: "u"}, xsd.UnionDerivation{},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Local: "v"},
		xsd.UnionDerivation{Members: []xsd.SimpleTypeOrRef{xsd.OwnedSimpleType{Definition: u}}},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "e"},
		xsd.TypeDefinitionRef{Name: u.Name()}, nil, xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "f"},
		xsd.TypeDefinitionRef{Name: v.Name()}, nil, xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := builtin.Seed(strict.New())
	if err != nil {
		t.Fatal(err)
	}
	b := xsd.NewSchemaBuilder()
	for _, st := range seeded {
		b.AddType(st)
	}
	b.AddType(u)
	b.AddType(v)
	b.AddElement(e)
	b.AddElement(f)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	src := generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"})
	typeCheck(t, src)
	for _, want := range []string{"type U interface {\n\tisU()\n}\n", "type V interface {\n\tisV()\n}\n"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("source does not declare %q:\n%s", want, src)
		}
	}
	if strings.Contains(string(src), "func (") {
		t.Errorf("source declares a method, want no variant implementing isU or isV:\n%s", src)
	}
}

// TestGenerateDeclinesUncoveredPrimitive pins the decline of an uncovered
// primitive, as the package doc's "Coverage" defines it, which no schema
// document yields, so the schema is built through the constructors: as an
// element's type, and, by the package doc's rule that a list or union declines
// with the Construct its item type or member declines with, as a list's item
// and as a union's basic member listed second, each named after the list or
// union.
func TestGenerateDeclinesUncoveredPrimitive(t *testing.T) {
	money, err := xsd.NewPrimitiveType(xsderr.Loc{}, xsd.QName{Space: "urn:x", Local: "money"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: "urn:x", Local: "l"},
		xsd.ListDerivation{Item: xsd.OwnedSimpleType{Definition: money}},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, []xsd.Facet{xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	union, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: "urn:x", Local: "u"},
		xsd.UnionDerivation{Members: []xsd.SimpleTypeOrRef{xsd.SimpleTypeRef{Name: intBuiltin}, xsd.OwnedSimpleType{Definition: money}}},
		xsd.OwnedSimpleType{Definition: xsd.AnySimpleType()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const primitive = ", whose {primitive type definition} is {urn:x}money at "
	tests := []struct {
		name   string
		typ    *xsd.SimpleType
		detail string
	}{
		{"primitive", money, "element e with {type definition} {urn:x}money" + primitive},
		{"list item", list, "element e with {type definition} {urn:x}l, whose {item type definition} is {urn:x}money" + primitive},
		{"union member", union, "element e with {type definition} {urn:x}u, whose basic member is {urn:x}money" + primitive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "e"},
				xsd.TypeDefinitionRef{Name: tt.typ.Name()}, nil, xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			seeded, err := builtin.Seed(strict.New())
			if err != nil {
				t.Fatal(err)
			}
			b := xsd.NewSchemaBuilder()
			for _, st := range seeded {
				b.AddType(st)
			}
			b.AddType(money)
			if tt.typ != money {
				b.AddType(tt.typ)
			}
			b.AddElement(e)
			schema, err := b.Finalize()
			if err != nil {
				t.Fatalf("Finalize: %v", err)
			}
			out := filepath.Join(moduleTempDir(t), "out")
			err = firstFailure(t, Generate([]Target{{Schema: schema, Dir: out, Package: "p"}}))
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Fatalf("Generate = %v, want a decline wrapping errors.ErrUnsupported", err)
			}
			var de *DeclineError
			if !errors.As(err, &de) || de.Construct() != (Construct{constructUncoveredPrimitive}) {
				t.Fatalf("Generate = %v, want a *DeclineError of Construct %v", err, Construct{constructUncoveredPrimitive})
			}
			if want := "codegen: target 0: " + tt.detail; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
			if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Dir %s exists after a decline (stat: %v)", out, err)
			}
		})
	}
}

// TestDeclineErrorAs pins that errors.As retrieves the *DeclineError from the
// failure Generate returns for a named and an unnamed target, with its Construct
// and Loc as values, and that the target's name opens the error outside the
// *DeclineError's own text.
func TestDeclineErrorAs(t *testing.T) {
	schema := primitiveSchema(t, `<xs:element name="e" type="money"/>`)
	tests := []struct {
		name, targetName, opening string
	}{
		{"unnamed", "", "codegen: target 0: "},
		{"named", "first", "first: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := firstFailure(t, Generate([]Target{{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p", Name: tt.targetName}}))
			var de *DeclineError
			if !errors.As(err, &de) {
				t.Fatalf("Generate = %v, want an error errors.As retrieves a *DeclineError from", err)
			}
			if got, want := de.Construct(), (Construct{constructUncoveredPrimitive}); got != want {
				t.Errorf("Construct() = %v, want %v", got, want)
			}
			if loc := de.Loc(); loc.URI != "s.xsd" || loc.Line != 2 {
				t.Errorf("Loc() = %+v, want URI s.xsd, Line 2", loc)
			}
			if want := tt.opening + de.Error(); err.Error() != want {
				t.Errorf("error %q, want %q", err, want)
			}
			const detail = "element e with {type definition} money, whose {primitive type definition} is money at "
			if !strings.HasPrefix(de.Error(), detail+de.Loc().String()+" is not supported by codegen") {
				t.Errorf("DeclineError.Error() = %q, want it to open %q followed by its Loc", de.Error(), detail)
			}
		})
	}
}

// TestConstructString pins that every construct's String is non-empty and
// distinct, and that the zero Construct and one past the last render a
// diagnostic form.
func TestConstructString(t *testing.T) {
	names := make(map[string]construct)
	for c := construct(1); int(c) < len(constructs); c++ {
		name := Construct{c}.String()
		if name == "" {
			t.Errorf("Construct{%d}.String() is empty", c)
		}
		if prior, ok := names[name]; ok {
			t.Errorf("Construct{%d}.String() = %q, the same as Construct{%d}'s", c, name, prior)
		}
		names[name] = c
		if constructs[c].format == "" {
			t.Errorf("construct %d (%s) has no format", c, name)
		}
	}
	if got := len(names); got != int(constructValueCycle) {
		t.Errorf("walked %d constructs, want %d", got, constructValueCycle)
	}
	for _, c := range []construct{0, construct(len(constructs))} {
		if got, want := (Construct{c}).String(), fmt.Sprintf("Construct(%d)", c); got != want {
			t.Errorf("Construct{%d}.String() = %q, want %q", c, got, want)
		}
	}
}

// TestCoverageDeclinedList pins the package doc's "Declined today" list to
// constructs (STYLE D3): its items are the names Construct.String returns for
// constructs[1:], one item each, in index order.
func TestCoverageDeclinedList(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "doc.go", nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("parse doc.go: %v", err)
	}
	if file.Doc == nil {
		t.Fatal("doc.go has no package comment")
	}
	blocks := new(comment.Parser).Parse(file.Doc.Text()).Content
	label := slices.IndexFunc(blocks, func(b comment.Block) bool {
		p, ok := b.(*comment.Paragraph)
		return ok && strings.HasPrefix(plainText(p.Text), "Declined today")
	})
	if label < 0 || label+1 == len(blocks) {
		t.Fatal(`package doc has no "Declined today" paragraph followed by a block`)
	}
	list, ok := blocks[label+1].(*comment.List)
	if !ok {
		t.Fatalf(`the block after "Declined today" is a %T, want a *comment.List`, blocks[label+1])
	}
	var got []string
	for i, item := range list.Items {
		if len(item.Content) != 1 {
			t.Fatalf("Declined item %d holds %d blocks, want 1", i, len(item.Content))
		}
		p, ok := item.Content[0].(*comment.Paragraph)
		if !ok {
			t.Fatalf("Declined item %d is a %T, want a *comment.Paragraph", i, item.Content[0])
		}
		got = append(got, plainText(p.Text))
	}
	var want []string
	for _, c := range constructs[1:] {
		want = append(want, c.name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Declined today list = %q, want the constructs names in index order %q", got, want)
	}
}

// plainText is the text of texts' leading plain runs, followed, at the first
// run that is not plain, by that run's type in angle brackets, so that a link
// or other markup in a Declined item never compares equal to a name.
func plainText(texts []comment.Text) string {
	var b strings.Builder
	for _, text := range texts {
		plain, ok := text.(comment.Plain)
		if !ok {
			return b.String() + fmt.Sprintf("<%T>", text)
		}
		b.WriteString(string(plain))
	}
	return b.String()
}

// parsedDecline parses body as line 2 of the one schema document s.xsd, its
// <xs:schema> start tag declaring the xs prefix, and returns the *DeclineError
// Generate returns for it.
func parsedDecline(t *testing.T, body string) *DeclineError {
	t.Helper()
	schema := parseString(t, "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n"+body+"\n</xs:schema>\n")
	err := firstFailure(t, Generate([]Target{{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"}}))
	var de *DeclineError
	if !errors.As(err, &de) {
		t.Fatalf("Generate = %v, want an error errors.As retrieves a *DeclineError from", err)
	}
	return de
}

// TestConstructMapKey pins Construct's doc: a Construct is comparable and may
// key a map. Each Construct comes from a decline, never a literal: two
// declines of primitiveSchema's money at different elements share one key, and
// the decline of a particle with {max occurs} 0, built as
// TestGenerateDeclinesVacuousParticle builds its wildcard row's, keys another.
func TestConstructMapKey(t *testing.T) {
	construct := func(schema *xsd.Schema) Construct {
		t.Helper()
		err := firstFailure(t, Generate([]Target{{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"}}))
		var de *DeclineError
		if !errors.As(err, &de) {
			t.Fatalf("Generate = %v, want an error errors.As retrieves a *DeclineError from", err)
		}
		return de.Construct()
	}
	anyNamespace, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wildcard, err := xsd.NewWildcard(xsderr.Loc{}, anyNamespace, xsd.ProcessLax)
	if err != nil {
		t.Fatal(err)
	}
	vacuous, err := xsd.NewOccurs(xsderr.Loc{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := construct(primitiveSchema(t, `<xs:element name="e" type="money"/>`))
	second := construct(primitiveSchema(t, `<xs:element name="f" type="money"/>`))
	third := construct(vacuousSchema(t, xsd.QName{Local: "c"}, wildcard, vacuous, exactlyOnce))
	counts := make(map[Construct]int)
	for _, c := range []Construct{first, second, third} {
		counts[c]++
	}
	if got := len(counts); got != 2 {
		t.Errorf("map holds %d keys, want 2: %v", got, counts)
	}
	if got := counts[first]; got != 2 {
		t.Errorf("counts[%v] = %d, want 2", first, got)
	}
	if got := counts[third]; got != 1 {
		t.Errorf("counts[%v] = %d, want 1", third, got)
	}
	if first != second {
		t.Errorf("two uncovered-primitive declines have Constructs %v and %v, want them ==", first, second)
	}
	if first == third {
		t.Errorf("an uncovered-primitive and a vacuous-particle decline both have Construct %v, want them !=", first)
	}
}

// TestDeclineErrorLoc pins DeclineError.Loc's doc for parsed schemas: a
// decline charged to a declaration read from the document has that
// declaration's location.
func TestDeclineErrorLoc(t *testing.T) {
	tests := []struct {
		name, body string
		kind       construct
		want       xsderr.Loc
	}{
		{"element reached again", `<xs:element name="r" type="c"/><xs:complexType name="c"><xs:sequence><xs:element ref="r"/></xs:sequence></xs:complexType>`,
			constructValueCycle, xsderr.Loc{URI: "s.xsd", Line: 2, Col: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			de := parsedDecline(t, tt.body)
			if got, want := de.Construct(), (Construct{tt.kind}); got != want {
				t.Errorf("Construct() = %v, want %v", got, want)
			}
			if got := de.Loc(); got != tt.want {
				t.Errorf("Loc() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
