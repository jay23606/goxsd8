package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// anyTypeName is xs:anyType's expanded name, the base the built complex types
// restrict.
var anyTypeName = xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}

// TestRankedOrdersByCountThenString pins ranked's order, and printReport's
// construct lines in it, on three tallies first seen as uncovered primitive
// (1 document), particle with {max occurs} 0 (2) and local element declaration
// with {abstract} true (2): the first-seen order, the count order with a
// first-seen tie-break and the count order with the String tie-break the tool
// owes are three different orders. Each Construct comes from Generate over a
// schema built through the xsd constructors, since a codegen.Construct admits
// no literal and those three constructs decline from no schema document.
// Dropping ranked's sort keeps the first-seen order, and dropping its String
// tie-break puts particle with {max occurs} 0 first; each fails here.
func TestRankedOrdersByCountThenString(t *testing.T) {
	uncovered := declined(t, uncoveredSchema(t))
	vacuous := declined(t, vacuousSchema(t))
	abstract := declined(t, abstractLocalSchema(t))
	// The fixture's precondition: three constructs, whose Strings order
	// abstract before vacuous, the reverse of their first-seen order.
	if uncovered == vacuous || vacuous == abstract || uncovered == abstract {
		t.Fatalf("Constructs %v, %v, %v are not three", uncovered, vacuous, abstract)
	}
	if abstract.String() >= vacuous.String() {
		t.Fatalf("%q does not sort before %q", abstract, vacuous)
	}
	tallies := []tally{
		{construct: uncovered, docs: []string{"a.xsd"}},
		{construct: vacuous, docs: []string{"b.xsd", "c.xsd"}},
		{construct: abstract, docs: []string{"d.xsd", "e.xsd"}},
	}
	want := []codegen.Construct{abstract, vacuous, uncovered}
	got := ranked(tallies)
	if len(got) != len(want) {
		t.Fatalf("ranked returned %d tallies, want %d", len(got), len(want))
	}
	for i, tl := range got {
		if tl.construct != want[i] {
			t.Errorf("ranked[%d] = %v, want %v", i, tl.construct, want[i])
		}
	}
	var out bytes.Buffer
	if err := printReport(&out, report{root: "r", found: 5, tallies: tallies}, false); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	lines := "=== First decline, by construct: 3 construct(s) ===\n" +
		"       2  " + abstract.String() + "\n" +
		"       2  " + vacuous.String() + "\n" +
		"       1  " + uncovered.String() + "\n"
	if !strings.Contains(out.String(), lines) {
		t.Errorf("report\n%s\nlacks the construct lines\n%s", out.String(), lines)
	}
}

// declined returns the Construct of schema's first decline, as census counts
// it.
func declined(t *testing.T, schema *xsd.Schema) codegen.Construct {
	t.Helper()
	// Generate refuses a Dir with no go.mod at or above it.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errs := codegen.Generate([]codegen.Target{{Schema: schema, Dir: filepath.Join(root, "out"), Package: "p"}})
	var de *codegen.DeclineError
	for _, e := range errs {
		if errors.As(e, &de) {
			break
		}
	}
	if de == nil {
		t.Fatalf("Generate = %v, want a decline", errs)
	}
	return de.Construct()
}

// uncoveredSchema is a top-level element of an ·implementation-defined·
// primitive, minted through xsd.NewPrimitiveType.
func uncoveredSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	money := xsd.QName{Local: "money"}
	primitive, err := xsd.NewPrimitiveType(xsderr.Loc{}, money, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := seeded(t)
	b.AddType(primitive)
	b.AddElement(global(t, "e", money))
	return finalized(t, b)
}

// vacuousSchema is a top-level element of the complex type c, whose content
// particle is a sequence with {max occurs} 0.
func vacuousSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	none, err := xsd.NewOccurs(xsderr.Loc{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	group, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := seeded(t)
	b.AddType(complexType(t, xsd.QName{Local: "c"}, none, group))
	b.AddElement(global(t, "e", xsd.QName{Local: "c"}))
	return finalized(t, b)
}

// abstractLocalSchema is a top-level element of the complex type c, whose
// sequence holds a local element declaration with {abstract} true.
func abstractLocalSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	c := xsd.QName{Local: "c"}
	scope, err := xsd.NewLocalScope(xsderr.Loc{}, xsd.ComplexTypeScopeParent{Name: c})
	if err != nil {
		t.Fatal(err)
	}
	a, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "a"},
		xsd.TypeDefinitionRef{Name: xsd.QName{Space: xsd.XMLSchemaNS, Local: "string"}}, nil, scope, nil, false, nil, nil, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	once, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, once, xsd.ResolvedTerm{Term: a})
	if err != nil {
		t.Fatal(err)
	}
	group, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{p})
	if err != nil {
		t.Fatal(err)
	}
	b := seeded(t)
	b.AddType(complexType(t, c, once, group))
	b.AddElement(global(t, "e", c))
	return finalized(t, b)
}

// complexType builds the complex type name, restricting xs:anyType, whose
// element-only content particle, occurring occurs times, has group as term.
func complexType(t *testing.T, name xsd.QName, occurs xsd.Occurs, group xsd.ModelGroup) xsd.ComplexType {
	t.Helper()
	p, err := xsd.NewParticle(xsderr.Loc{}, occurs, xsd.ResolvedTerm{Term: group})
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

// global builds a top-level element declaration named local of the type
// named typ.
func global(t *testing.T, local string, typ xsd.QName) xsd.ElementDeclaration {
	t.Helper()
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: local},
		xsd.TypeDefinitionRef{Name: typ}, nil, xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// seeded returns a SchemaBuilder holding the builtins and an xs:anyType
// stand-in, which the types it is given resolve against.
func seeded(t *testing.T) *xsd.SchemaBuilder {
	t.Helper()
	anyType, err := xsd.NewComplexType(xsderr.Loc{}, anyTypeName, anyTypeName, nil, xsd.DerivationRestriction, false,
		nil, nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	types, err := builtin.Seed(strict.New())
	if err != nil {
		t.Fatal(err)
	}
	b := xsd.NewSchemaBuilder()
	for _, st := range types {
		b.AddType(st)
	}
	b.AddType(anyType)
	return b
}

// finalized returns b's schema.
func finalized(t *testing.T, b *xsd.SchemaBuilder) *xsd.Schema {
	t.Helper()
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return schema
}
