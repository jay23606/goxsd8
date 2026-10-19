package validate

import (
	"testing"

	"github.com/kud360/goxsd8/xsd"
)

// governingSchema carries one shape per row of [Validator.Governing]'s
// contract: Base and Derived, an extension of it, and Other, unrelated to
// either; plain, blocked (which blocks extension) and tabled (whose {type
// table} selects Derived for @k = 'd', and Base otherwise); and four parent
// types, SkipAny and LaxAny each holding one wildcard particle, and SkipOpen
// and LaxOpen each an {open content}, LaxOpen also declaring a local loc of
// type Base, which is the ·locally declared type· of a loc within it.
const governingSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="Base"><xs:sequence><xs:element name="a" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="Base"/></xs:complexContent></xs:complexType>
  <xs:complexType name="Other"/>
  <xs:element name="plain" type="Base"/>
  <xs:element name="blocked" type="Base" block="extension"/>
  <xs:element name="tabled" type="Base">
    <xs:alternative test="@k = 'd'" type="Derived"/>
  </xs:element>
  <xs:complexType name="SkipAny"><xs:sequence><xs:any processContents="skip"/></xs:sequence></xs:complexType>
  <xs:complexType name="LaxAny"><xs:sequence><xs:any processContents="lax"/></xs:sequence></xs:complexType>
  <xs:complexType name="SkipOpen">
    <xs:openContent><xs:any processContents="skip"/></xs:openContent>
    <xs:sequence><xs:element name="b" type="xs:string" minOccurs="0"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="LaxOpen">
    <xs:openContent><xs:any processContents="lax"/></xs:openContent>
    <xs:sequence><xs:element name="loc" type="Base" minOccurs="0"/></xs:sequence>
  </xs:complexType>
</xs:schema>`

// governingRow is one call of [Validator.Governing] and the answer its doc
// gives for it: the declaration's name, "" for none, and the type's, "" for
// nil. slot is what the selection callback answers, a nil one withholding the
// selection, and calls how many times Governing must call it.
type governingRow struct {
	name       string
	parent     string
	a          func(t *testing.T, s *xsd.Schema) xsd.Attribution
	element    string
	instance   string
	slot       xsd.TypeDefinitionOrRef
	wantDecl   string
	wantType   string
	unassessed bool
	calls      int
}

// TestGoverning pins [Validator.Governing]'s rows over governingSchema, each
// by its declaration, type, assessed bit and the number of times it calls the
// selection callback.
func TestGoverning(t *testing.T) {
	s := parsedSchema(t, map[string]string{"main.xsd": governingSchema})
	v, err := New(s, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ref := func(n string) xsd.TypeDefinitionOrRef { return xsd.TypeDefinitionRef{Name: local(n)} }
	rows := []governingRow{
		{name: "skip wildcard skips", parent: "SkipAny", a: wildcardOf("SkipAny"), element: "plain", unassessed: true},
		{name: "skip open content skips", parent: "SkipOpen", a: openContentOf("SkipOpen"), element: "plain", unassessed: true},
		{name: "context-determined declaration", a: declarationOf("plain"), element: "plain", wantDecl: "plain", wantType: "Base"},
		{name: "instance overrides S", a: declarationOf("plain"), element: "plain", instance: "Derived", wantDecl: "plain", wantType: "Derived"},
		{name: "instance does not override S", a: declarationOf("blocked"), element: "blocked", instance: "Derived", wantDecl: "blocked", wantType: "Base"},
		{name: "instance unrelated to S", a: declarationOf("plain"), element: "plain", instance: "Other", wantDecl: "plain", wantType: "Base"},
		{name: "substituting declaration resolves", a: declarationOf("plain"), element: "blocked", wantDecl: "blocked", wantType: "Base"},
		{name: "lax wildcard resolves", parent: "LaxAny", a: wildcardOf("LaxAny"), element: "plain", wantDecl: "plain", wantType: "Base"},
		{name: "withheld selection", a: declarationOf("tabled"), element: "tabled", wantDecl: "tabled", calls: 1},
		{name: "selection read", a: declarationOf("tabled"), element: "tabled", slot: ref("Derived"), wantDecl: "tabled", wantType: "Derived", calls: 1},
		{name: "instance overrides the selection", a: declarationOf("tabled"), element: "tabled", instance: "Derived", slot: ref("Base"), wantDecl: "tabled", wantType: "Derived", calls: 1},
		{name: "instance with the selection withheld", a: declarationOf("tabled"), element: "tabled", instance: "Derived", wantDecl: "tabled", calls: 1},
		{name: "locally declared type", parent: "LaxOpen", a: openContentOf("LaxOpen"), element: "loc", wantType: "Base"},
		{name: "instance overrides L", parent: "LaxOpen", a: openContentOf("LaxOpen"), element: "loc", instance: "Derived", wantType: "Derived"},
		{name: "instance does not override L", parent: "LaxOpen", a: openContentOf("LaxOpen"), element: "loc", instance: "Other", wantType: "Base"},
		{name: "unattributed, no L, resolves", parent: "LaxOpen", element: "plain", wantDecl: "plain", wantType: "Base"},
		{name: "unresolved name is lax", element: "nothing"},
		{name: "unresolved name, instance governs", element: "nothing", instance: "Other", wantType: "Other"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var parent *xsd.ComplexType
			if row.parent != "" {
				parent = complexNamed(t, s, row.parent)
			}
			var a xsd.Attribution
			if row.a != nil {
				a = row.a(t, s)
			}
			var instance xsd.TypeDefinition
			if row.instance != "" {
				instance = *complexNamed(t, s, row.instance)
			}
			calls := 0
			selected := func(xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool) {
				calls++
				return row.slot, row.slot != nil
			}
			decl, declared, typ, assessed, err := v.Governing(parent, a, local(row.element), instance, selected)
			wantGoverning(t, governingAnswer{decl, declared, typ, assessed, err}, row.wantDecl, row.wantType, !row.unassessed)
			if calls != row.calls {
				t.Errorf("selected called %d times, want %d", calls, row.calls)
			}
		})
	}
}

// TestGoverningMissingSubcomponents pins the rows only a directly built
// schema reaches: a nil {type definition}, which leaves the declaration with
// no type, and an ·absent· head or {substitution group affiliations} member,
// which §5.3 sends to ·lax assessment·.
func TestGoverningMissingSubcomponents(t *testing.T) {
	never := func(xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool) {
		t.Errorf("selected called for a declaration with no {type table}")
		return nil, false
	}
	cases := []struct {
		name     string
		schema   *xsd.Schema
		wantDecl string
	}{
		{"nil slot", selectionSchema(t, nil, nil, func() (xsd.TypeDefinitionOrRef, *xsd.TypeTable) { return nil, nil }), "root"},
		{"absent head", absentHeadSchema(t, func() (xsd.TypeDefinitionOrRef, *xsd.TypeTable) { return absentHead, nil }), ""},
		{"absent affiliation", affiliationSchema(t, absentHead.Head), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := New(c.schema, testBackend())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			decl, declared, typ, assessed, err := v.Governing(nil, nil, local("root"), nil, never)
			wantGoverning(t, governingAnswer{decl, declared, typ, assessed, err}, c.wantDecl, "", true)
		})
	}
}

// TestGoverningNilSelectedPanics pins Governing's nil-callback contract.
func TestGoverningNilSelectedPanics(t *testing.T) {
	s := parsedSchema(t, map[string]string{"main.xsd": governingSchema})
	v, err := New(s, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Errorf("Governing with a nil selected did not panic")
		}
	}()
	_, _, _, _, _ = v.Governing(nil, nil, local("plain"), nil, nil) // panics before it returns
}

// governingAnswer is Governing's five results.
type governingAnswer struct {
	decl     xsd.ElementDeclaration
	declared bool
	typ      xsd.TypeDefinition
	assessed bool
	err      error
}

// wantGoverning fails unless got declares the declaration named wantDecl, or
// none for "", is typed by the type named wantType, or nil for "", is
// assessed as wantAssessed says, and carries no error.
func wantGoverning(t *testing.T, got governingAnswer, wantDecl, wantType string, wantAssessed bool) {
	t.Helper()
	if got.err != nil {
		t.Fatalf("err = %v, want nil", got.err)
	}
	if got.assessed != wantAssessed {
		t.Errorf("assessed = %v, want %v", got.assessed, wantAssessed)
	}
	if got.declared != (wantDecl != "") || (got.declared && got.decl.Name() != local(wantDecl)) {
		t.Errorf("decl = %v (declared %v), want %q", got.decl.Name(), got.declared, wantDecl)
	}
	if wantType == "" && got.typ != nil {
		t.Errorf("typ = %v, want nil", got.typ.Name())
	}
	if wantType != "" && (got.typ == nil || got.typ.Name() != local(wantType)) {
		t.Errorf("typ = %v, want %s", got.typ, wantType)
	}
}

// complexNamed is the top-level complex type of governingSchema named name.
func complexNamed(t *testing.T, s *xsd.Schema, name string) *xsd.ComplexType {
	t.Helper()
	td, ok := s.Type(local(name))
	if !ok {
		t.Fatalf("no type %s", name)
	}
	ct, ok := td.(xsd.ComplexType)
	if !ok {
		t.Fatalf("type %s is %T, want a complex type", name, td)
	}
	return &ct
}

// declarationOf attributes to the top-level element declaration named name.
func declarationOf(name string) func(*testing.T, *xsd.Schema) xsd.Attribution {
	return func(t *testing.T, s *xsd.Schema) xsd.Attribution {
		t.Helper()
		d, ok := s.Element(local(name))
		if !ok {
			t.Fatalf("no element %s", name)
		}
		return d
	}
}

// wildcardOf attributes to the one wildcard particle of the type named name.
func wildcardOf(name string) func(*testing.T, *xsd.Schema) xsd.Attribution {
	return func(t *testing.T, s *xsd.Schema) xsd.Attribution {
		t.Helper()
		ws := s.ContainedWildcards(*complexNamed(t, s, name))
		if len(ws) != 1 {
			t.Fatalf("type %s contains %d wildcards, want 1", name, len(ws))
		}
		return ws[0]
	}
}

// openContentOf attributes to the {open content} of the type named name.
func openContentOf(name string) func(*testing.T, *xsd.Schema) xsd.Attribution {
	return func(t *testing.T, s *xsd.Schema) xsd.Attribution {
		t.Helper()
		ec, ok := complexNamed(t, s, name).ContentType().(xsd.ElementContent)
		if !ok || ec.OpenContent == nil {
			t.Fatalf("type %s has no {open content}", name)
		}
		return ec.OpenContent
	}
}
