package parser_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// mustRule fails unless err is an *xsderr.Error charging want whose message
// names every clause given — the clause number is asserted because several of
// src-redefine's clauses share a rule ID and a test that only checked the ID
// would pass on the wrong verdict.
func mustRule(t *testing.T, err error, want xsderr.Rule, clauses ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Parse succeeded, want a %s rejection", want)
	}
	var xe *xsderr.Error
	if !errors.As(err, &xe) {
		t.Fatalf("Parse error = %v (%T), want an *xsderr.Error charging %s", err, err, want)
	}
	if xe.Rule != want {
		t.Fatalf("Parse error rule = %s, want %s (%v)", xe.Rule, want, err)
	}
	for _, clause := range clauses {
		if !strings.Contains(xe.Error(), clause) {
			t.Fatalf("error %q does not name %s clause %s", xe.Error(), want, clause)
		}
	}
}

// mustModelGroup returns the assembled schema's model group definition named
// name. xsd.Schema exposes {model group definitions} only as a document-ordered
// slice (no by-name lookup has a consumer yet), so the scan is the reader.
func mustModelGroup(t *testing.T, s *xsd.Schema, name xsd.QName) xsd.ModelGroupDefinition {
	t.Helper()
	for _, mgd := range s.ModelGroups() {
		if mgd.Name() == name {
			return mgd
		}
	}
	t.Fatalf("model group definition %s not found in assembled schema", name)
	return xsd.ModelGroupDefinition{}
}

// mustAttributeGroup is mustModelGroup for {attribute group definitions}.
func mustAttributeGroup(t *testing.T, s *xsd.Schema, name xsd.QName) xsd.AttributeGroupDefinition {
	t.Helper()
	for _, ag := range s.AttributeGroups() {
		if ag.Name() == name {
			return ag
		}
	}
	t.Fatalf("attribute group definition %s not found in assembled schema", name)
	return xsd.AttributeGroupDefinition{}
}

// mustSimpleType returns the assembled schema's simple type named name.
func mustSimpleType(t *testing.T, s *xsd.Schema, name xsd.QName) *xsd.SimpleType {
	t.Helper()
	td, ok := s.Type(name)
	if !ok {
		t.Fatalf("type %s not found in assembled schema", name)
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("type %s is %T, want *xsd.SimpleType", name, td)
	}
	return st
}

// TestParseRedefineSimpleTypeResolvesToOriginal is the src-expredef pairing, and
// the test the whole slice turns on. The redefining <simpleType> restricts a
// type of its OWN name, so a naive symbol-table lookup would find the
// redefinition and report a circular derivation (st-props-correct clause 2).
// Clause 1.1 makes the base "one component which corresponds to the top-level
// definition item with the same name in the <redefine>d schema document … except
// that its {name} is ·absent·", so the assembly must instead build the ORIGINAL
// and hand it over as the base — a valid derivation, not a self-loop.
func TestParseRedefineSimpleTypeResolvesToOriginal(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code">`+
			`<xs:maxLength value="2"/></xs:restriction></xs:simpleType>`+
			`</xs:redefine>`+
			`<xs:element name="root" type="tns:code"/>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"><xs:maxLength value="8"/></xs:restriction>`+
			`</xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The VISIBLE {urn:a}code is the redefinition (§4.2.4 clause 4.1.2 excepts the
	// original from the components lib.xsd contributes), and a reference in the
	// redefining document reaches it.
	code := mustSimpleType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
	if got := facetValue(t, code, xsd.FacetMaxLength); got != "2" {
		t.Fatalf("{urn:a}code own maxLength = %q, want the redefinition's 2", got)
	}
	// Its base is the hidden original — the same name, {name} absent — carrying
	// lib.xsd's own facet. Anything else (a nil base, a base named {urn:a}code) is
	// the false circularity src-expredef's pairing exists to prevent.
	base := mustBase(t, s, code)
	if base == nil {
		t.Fatalf("redefined {urn:a}code has no {base type definition}")
	}
	if got := base.Name(); got != (xsd.QName{}) {
		t.Fatalf("hidden original's {name} = %s, want ·absent· (src-expredef clause 1.1)", got)
	}
	if got := facetValue(t, base, xsd.FacetMaxLength); got != "8" {
		t.Fatalf("hidden original maxLength = %q, want lib.xsd's 8", got)
	}
	mustElementType(t, s, xsd.QName{Space: "urn:a", Local: "root"}, xsd.QName{Space: "urn:a", Local: "code"})
}

// TestParseRedefineIncludesUnmentionedComponents covers §4.2.4 clause 4.1.2's
// other half: everything the redefined document declares that the <redefine>
// does NOT mention comes through unchanged, exactly as a plain <include> brings
// it. A redefine that only replaced would silently lose the rest of the library.
func TestParseRedefineIncludesUnmentionedComponents(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code"/></xs:simpleType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a",
			`<xs:simpleType name="code"><xs:restriction base="xs:string"/></xs:simpleType>`+
				`<xs:element name="untouched" type="xs:date"/>`+
				`<xs:simpleType name="other"><xs:restriction base="xs:int"/></xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "other"})
	mustElementType(t, s, xsd.QName{Space: "urn:a", Local: "untouched"}, xsType("date"))
}

// TestParseRedefineChameleon covers src-redefine clause 3.3: a redefined
// document with no targetNamespace of its own is coerced into the redefining
// document's namespace (§F.1), on the same terms an <include>d one is — so both
// the redefinition and the hidden original it is paired with live in urn:a.
func TestParseRedefineChameleon(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code">`+
			`<xs:maxLength value="2"/></xs:restriction></xs:simpleType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"><xs:maxLength value="8"/></xs:restriction>`+
			`</xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	code := mustSimpleType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
	if got := facetValue(t, code, xsd.FacetMaxLength); got != "2" {
		t.Fatalf("{urn:a}code maxLength = %q, want the redefinition's 2", got)
	}
	if _, ok := s.Type(xsd.QName{Local: "code"}); ok {
		t.Fatalf("the chameleon-redefined type must not also appear in the ·absent· namespace")
	}
}

// TestParseRedefineUnresolvableIsAnError is the semantic <redefine> does NOT
// share with <include>: src-redefine clause 1 makes a failed de-reference an
// error whenever the element has children other than <annotation>, where
// src-include clause 2.4 says in as many words that failing to resolve "is not
// an error". The two mechanisms are asserted side by side, on the same
// unresolvable location, so the difference cannot silently collapse.
func TestParseRedefineUnresolvableIsAnError(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="gone.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("an unresolvable <include> is not an error (src-include clause 2.4), got: %v", err)
	}

	_, err = parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="gone.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code"/></xs:simpleType>`+
			`</xs:redefine>`),
	})
	mustRule(t, err, "src-redefine", "clause 1")
}

// TestParseRedefineNoOriginalRejected covers src-expredef's closing sentence:
// "In all cases there must be a top-level definition item of the appropriate
// name and kind in the <redefine>d schema document." It is charged before
// clause 5, so redefining a name the library never declared is reported as the
// missing pairing rather than as a circular derivation.
func TestParseRedefineNoOriginalRejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="absent"><xs:restriction base="tns:absent"/></xs:simpleType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	mustRule(t, err, "src-expredef")
}

// TestParseRedefineWrongKindRejected pins that the pairing is per (element type,
// name), not per name: a <group> named like a top-level <simpleType> of the
// redefined document redefines nothing, so src-expredef's "of the appropriate
// name AND KIND" fires.
func TestParseRedefineWrongKindRejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="code"><xs:sequence><xs:element name="x" type="xs:string"/></xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	mustRule(t, err, "src-expredef")
}

// TestParseRedefineSimpleTypeClause5Rejected covers src-redefine clause 5's
// simple-type half in both its failure modes: a <restriction> whose base is some
// OTHER type, and no <restriction> at all.
func TestParseRedefineSimpleTypeClause5Rejected(t *testing.T) {
	lib := wrap("urn:a", `<xs:simpleType name="code">`+
		`<xs:restriction base="xs:string"/></xs:simpleType>`)
	for _, tc := range []struct {
		name  string
		child string
	}{
		{"restricts another type", `<xs:simpleType name="code"><xs:restriction base="xs:string"/></xs:simpleType>`},
		{"no restriction child", `<xs:simpleType name="code"><xs:list itemType="xs:string"/></xs:simpleType>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+tc.child+`</xs:redefine>`),
				"lib.xsd":  lib,
			})
			mustRule(t, err, "src-redefine")
		})
	}
}

// TestParseRedefineComplexTypeClause5Rejected covers src-redefine clause 5's
// complex-type half — "a restriction or extension among its grand-children the
// ·actual value· of whose base [attribute] must be the same as … its own name
// attribute plus target namespace". The clause is charged BEFORE the
// src-expredef pairing is attempted, and the ordering is what this test
// discriminates: base="tns:other" names a type that DOES exist, so a producer
// that paired first would build an ordinary extension of it and accept the
// document instead of charging the rule it breaks.
func TestParseRedefineComplexTypeClause5Rejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:other">`+
			`<xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a",
			`<xs:complexType name="ct"><xs:sequence/></xs:complexType>`+
				`<xs:complexType name="other"><xs:sequence/></xs:complexType>`),
	})
	mustRule(t, err, "src-redefine")
}

// mustComplexType returns the assembled schema's complex type definition named
// name.
func mustComplexType(t *testing.T, s *xsd.Schema, name xsd.QName) xsd.ComplexType {
	t.Helper()
	td, ok := s.Type(name)
	if !ok {
		t.Fatalf("type definition %s not found in assembled schema", name)
	}
	ct, ok := td.(xsd.ComplexType)
	if !ok {
		t.Fatalf("type definition %s is a %T, want an xsd.ComplexType", name, td)
	}
	return ct
}

// mustOwnedBase returns the anonymous {base type definition} c owns, failing
// unless the slot really is the InlineTypeDefinition arm holding an ANONYMOUS
// complex type — src-expredef clause 1.1's original.
func mustOwnedBase(t *testing.T, c xsd.ComplexType) xsd.ComplexType {
	t.Helper()
	inline, owns := c.Base().(xsd.InlineTypeDefinition)
	if !owns {
		t.Fatalf("%s {base type definition} = %#v, want the InlineTypeDefinition holding src-expredef clause 1.1's original", c.Name(), c.Base())
	}
	base, isComplex := inline.Definition.(xsd.ComplexType)
	if !isComplex {
		t.Fatalf("%s owns a %T as its base, want an xsd.ComplexType", c.Name(), inline.Definition)
	}
	if base.Name() != (xsd.QName{}) {
		t.Fatalf("%s's owned base is named %s, but src-expredef clause 1.1 makes its {name} absent", c.Name(), base.Name())
	}
	return base
}

// TestParseRedefineComplexTypePairsWithOriginal is the src-expredef clause
// 1.1/1.2 pairing itself, on the shape the pairing exists to make work: a
// redefining <complexType> whose <extension> grand-child names ITSELF as base.
//
// What is pinned is that the derivation is a real one and not a self-loop — the
// {base type definition} is the anonymous, {name}-·absent· component built from
// the REDEFINED document's own declaration, carrying that declaration's content —
// and that the pairing passes finalize rather than tripping ct-props-correct
// clause 3, which is exactly what a naive base-names-itself encoding would do.
func TestParseRedefineComplexTypePairsWithOriginal(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">`+
			`<xs:sequence><xs:element name="extra" type="xs:string"/></xs:sequence>`+
			`</xs:extension></xs:complexContent></xs:complexType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:complexType name="ct">`+
			`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ct := mustComplexType(t, s, xsd.QName{Space: "urn:a", Local: "ct"})
	if ct.DerivationMethod() != xsd.DerivationExtension {
		t.Fatalf("{derivation method} = %s, want extension", ct.DerivationMethod())
	}
	base := mustOwnedBase(t, ct)

	// The original is S2's declaration, so its OWN base is the ordinary
	// xs:anyType of the implicit-content form — never the redefinition, which
	// is the false circularity the pairing prevents.
	wantAnyType := xsd.TypeDefinitionOrRef(xsd.TypeDefinitionRef{Name: xsd.QName{Space: xsdNS, Local: "anyType"}})
	if base.Base() != wantAnyType {
		t.Fatalf("the clause-1.1 original's own {base type definition} = %#v, want a TypeDefinitionRef naming xs:anyType", base.Base())
	}
	// Its {context} is the redefining component (clause 1.1), not a declaration.
	context, present := base.Context()
	if !present {
		t.Fatal("the clause-1.1 original has no {context}, which the §3.4.1 tableau makes Required when {name} is absent")
	}
	if _, isCTD := context.(xsd.ComplexTypeDefinitionContext); !isCTD {
		t.Fatalf("the clause-1.1 original's {context} = %T, want a ComplexTypeDefinitionContext naming the redefining component", context)
	}
	// The original carries S2's content, the redefinition S2's plus its own.
	if got := elementNamesOf(t, base); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("the clause-1.1 original's content model declares %v, want the redefined document's [a]", got)
	}
	if got := elementNamesOf(t, ct); !slices.Equal(got, []string{"a", "extra"}) {
		t.Fatalf("the redefinition's content model declares %v, want [a extra] (§3.4.2.3.3 clause 4.2)", got)
	}
}

// elementNamesOf lists, in document order, the local names of the element
// declarations directly in c's {content type} particle tree.
func elementNamesOf(t *testing.T, c xsd.ComplexType) []string {
	t.Helper()
	ec, ok := c.ContentType().(xsd.ElementContent)
	if !ok {
		t.Fatalf("{content type} = %T, want element-only content", c.ContentType())
	}
	var names []string
	var walk func(term xsd.TermOrRef)
	walk = func(term xsd.TermOrRef) {
		resolved, ok := term.(xsd.ResolvedTerm)
		if !ok {
			return
		}
		switch inner := resolved.Term.(type) {
		case xsd.ElementDeclaration:
			names = append(names, inner.Name().Local)
		case xsd.ModelGroup:
			for _, p := range inner.Particles() {
				walk(p.Term())
			}
		}
	}
	walk(ec.Particle.Term())
	return names
}

// TestParseRedefineComplexTypeReferencesResolveToRedefinition is src-expredef's
// stated purpose, pinned apart from the base wiring: "references to names of
// redefined components in BOTH the <redefine>ing and the <redefine>d schema
// documents ·resolve· to the redefined component". D1's own reference and D2's
// must both reach the redefinition, so both elements' declared type must be the
// name of a type carrying the redefinition's extra particle.
func TestParseRedefineComplexTypeReferencesResolveToRedefinition(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">`+
			`<xs:sequence><xs:element name="extra" type="xs:string"/></xs:sequence>`+
			`</xs:extension></xs:complexContent></xs:complexType>`+
			`</xs:redefine>`+
			`<xs:element name="inD1" type="tns:ct"/>`),
		"lib.xsd": wrap("urn:a", `<xs:complexType name="ct">`+
			`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`+
			`<xs:element name="inD2" type="tns:ct"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// One component holds the name, and it is the redefinition: nothing else
	// could carry both particles.
	ct := mustComplexType(t, s, xsd.QName{Space: "urn:a", Local: "ct"})
	if got := elementNamesOf(t, ct); !slices.Equal(got, []string{"a", "extra"}) {
		t.Fatalf("{urn:a}ct declares %v, want the REDEFINITION's [a extra]", got)
	}
	for _, name := range []string{"inD1", "inD2"} {
		ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: name})
		if !ok {
			t.Fatalf("element declaration %s not found in assembled schema", name)
		}
		ref, byName := ed.TypeDefinition().(xsd.TypeDefinitionRef)
		if !byName || ref.Name != (xsd.QName{Space: "urn:a", Local: "ct"}) {
			t.Fatalf("%s {type definition} = %#v, want a TypeDefinitionRef naming {urn:a}ct", name, ed.TypeDefinition())
		}
	}
}

// TestParseRedefineComplexTypeInheritsOriginalAttributes pins the two finalize
// folds across the anonymous base: §3.4.2.4 clause 3 and §3.4.2.5 clause 2.2 both
// name the {base type definition}'s property, and the base here is in no by-name
// index, so a fold that only followed names would leave the redefinition without
// the attribute use and the wildcard its original declares — which REJECTS
// instances carrying them (#505).
func TestParseRedefineComplexTypeInheritsOriginalAttributes(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">`+
			`<xs:sequence/><xs:attribute name="own" type="xs:string"/>`+
			`</xs:extension></xs:complexContent></xs:complexType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:complexType name="ct"><xs:sequence/>`+
			`<xs:attribute name="inherited" type="xs:string"/>`+
			`<xs:anyAttribute namespace="urn:w"/>`+
			`</xs:complexType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ct := mustComplexType(t, s, xsd.QName{Space: "urn:a", Local: "ct"})
	var got []string
	for _, u := range ct.AttributeUses() {
		got = append(got, attributeUseLocalName(t, u))
	}
	if !slices.Equal(got, []string{"own", "inherited"}) {
		t.Fatalf("redefinition {attribute uses} = %v, want its own then the clause-1.1 original's ([own inherited], §3.4.2.4 clause 3.1)", got)
	}
	if _, present := ct.AttributeWildcard(); !present {
		t.Fatal("redefinition {attribute wildcard} is absent, but §3.4.2.5 clause 2.2 unions in the ·base wildcard· its original declares")
	}
}

// TestParseRedefineRestrictionOverInheritingOriginal pins the OTHER half of the
// same two folds: not the redefinition's own properties, but the ORIGINAL's.
//
// The original here declares no attribute of its own — it extends a named type
// that does — so its {attribute uses} and {attribute wildcard} are entirely
// clause-3/clause-2.2 inherited. derivation-ok-restriction clause 3 (c-ran) reads
// exactly those two properties OFF THE BASE to charge the redefining restriction:
// checkRestrictionAttributes demands the base declare or admit every use the
// restriction declares, and checkAttributeRestrictionWildcard demands the base
// have a wildcard when the restriction does. A fold that computed the original's
// values without STORING them back into the {base type definition} slot that owns
// it left both readers seeing a base that declares nothing, and each rejected a
// legal redefinition — fail-CLOSED, not the under-rejection a skipped constraint
// gives (#505).
//
// The control is the same shape written without <redefine> at all, which this
// implementation has always accepted: the two must agree, since <redefine>
// changes where the base component comes from and not whether the derivation is
// valid.
func TestParseRedefineRestrictionOverInheritingOriginal(t *testing.T) {
	for _, c := range []struct{ name, inherited, restricted string }{
		{
			name:       "attribute use",
			inherited:  `<xs:attribute name="gattr" type="xs:string"/>`,
			restricted: `<xs:attribute name="gattr" type="xs:string"/>`,
		},
		{
			name:       "attribute wildcard",
			inherited:  `<xs:anyAttribute namespace="urn:w"/>`,
			restricted: `<xs:anyAttribute namespace="urn:w"/>`,
		},
		{
			// The restriction declares NO wildcard of its own, so its own
			// folded {attribute wildcard} is ·absent· — and the base's
			// inherited one is the only thing that admits the name it
			// declares a use for (·default binding·, defaultbinding.go). A
			// storeback that wrote only the types whose own folded value was
			// present would drop the base re-seat on exactly this shape.
			name:       "attribute use admitted only by the inherited wildcard",
			inherited:  `<xs:anyAttribute namespace="##any"/>`,
			restricted: `<xs:attribute name="loc" type="xs:string"/>`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			lib := wrap("urn:a", `<xs:complexType name="g"><xs:sequence/>`+c.inherited+`</xs:complexType>`+
				`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:g">`+
				`<xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`)
			if _, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
					`<xs:complexType name="ct"><xs:complexContent><xs:restriction base="tns:ct">`+
					`<xs:sequence/>`+c.restricted+
					`</xs:restriction></xs:complexContent></xs:complexType>`+
					`</xs:redefine>`),
				"lib.xsd": lib,
			}); err != nil {
				t.Fatalf("Parse: %v\nwant the redefining restriction ACCEPTED: the clause 1.1 original inherits the %s c-ran reads off the base", err, c.name)
			}
			// Control: the identical derivation with the original named, so the
			// base is an ordinary by-name component and no anonymous slot is
			// involved.
			if _, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:complexType name="g"><xs:sequence/>`+c.inherited+`</xs:complexType>`+
					`<xs:complexType name="orig"><xs:complexContent><xs:extension base="tns:g">`+
					`<xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`+
					`<xs:complexType name="ct"><xs:complexContent><xs:restriction base="tns:orig">`+
					`<xs:sequence/>`+c.restricted+
					`</xs:restriction></xs:complexContent></xs:complexType>`),
			}); err != nil {
				t.Fatalf("control (no <redefine>): %v\nthe redefine form must not be judged more harshly than this", err)
			}
		})
	}
}

// attributeUseLocalName reads the local part of an attribute use's declared name,
// through whichever arm of the {attribute declaration} slot it carries.
func attributeUseLocalName(t *testing.T, u xsd.AttributeUse) string {
	t.Helper()
	switch d := u.AttributeDeclaration().(type) {
	case xsd.AttributeDeclarationRef:
		return d.Name.Local
	case xsd.LocalAttributeDeclaration:
		return d.Declaration.Name().Local
	default:
		t.Fatalf("attribute use carries a %T, which is neither arm of AttributeDeclarationOrRef", u.AttributeDeclaration())
		return ""
	}
}

// TestParseRedefineComplexTypeMutualBaseTerminates is the termination case the
// pairing creates and nothing else does: D2 declares ct base="u" and u base="ct",
// and D1 redefines ct. The chain runs redefinition → anonymous original → u →
// redefinition, so it CLOSES through an anonymous hop that no by-name walk can
// see. It must terminate — a walk that recursed through the hop without counting
// it would not — and it must be REJECTED as the circularity it is, under
// ct-props-correct clause 3.
func TestParseRedefineComplexTypeMutualBaseTerminates(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">`+
			`<xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a",
			`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:u">`+
				`<xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`+
				`<xs:complexType name="u"><xs:complexContent><xs:extension base="tns:ct">`+
				`<xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`),
	})
	mustRule(t, err, "ct-props-correct", "clause 3")
}

// chainedDocs is the two-level redefine chain both chained tests are built on:
// d1 redefines d2 for one (kind, name), and d2 redefines d3 for the SAME one.
// §4.2.4 clause 4.1.1 makes d2's redefining child a top-level definition of d2,
// so d1's redefinition pairs with it under src-expredef clause 1.1 and it in turn
// pairs with d3's declaration under clause 1.2 applied at d2's level.
func chainedDocs(d1Child, d2Child, d3Decl string) map[string]string {
	return map[string]string{
		"d1.xsd": wrap("urn:a", `<xs:redefine schemaLocation="d2.xsd">`+d1Child+`</xs:redefine>`),
		"d2.xsd": wrap("urn:a", `<xs:redefine schemaLocation="d3.xsd">`+d2Child+`</xs:redefine>`),
		"d3.xsd": wrap("urn:a", d3Decl),
	}
}

// TestParseRedefineChainedSimpleType pins the two-level chain for a
// <simpleType>: d1's redefinition must reach d3's declaration THROUGH d2's, not
// past it. §4.2.4 clause 4.1.2 excludes d3's own {urn:a}code from the components
// d2 contributes — only d2's redefining component and the original inside it
// exist there — so a base chain that skipped a level would silently drop d2's
// redefinition from the type hierarchy d1 sees.
func TestParseRedefineChainedSimpleType(t *testing.T) {
	s, err := parseMap(t, "d1.xsd", chainedDocs(
		`<xs:simpleType name="code"><xs:restriction base="tns:code">`+
			`<xs:maxLength value="2"/></xs:restriction></xs:simpleType>`,
		`<xs:simpleType name="code"><xs:restriction base="tns:code">`+
			`<xs:maxLength value="4"/></xs:restriction></xs:simpleType>`,
		`<xs:simpleType name="code"><xs:restriction base="xs:string">`+
			`<xs:maxLength value="8"/></xs:restriction></xs:simpleType>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// One visible {urn:a}code, d1's, and each hidden original below it is
	// {name}-·absent· (clause 1.1) and carries its own document's facet.
	code := mustSimpleType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
	if got := facetValue(t, code, xsd.FacetMaxLength); got != "2" {
		t.Fatalf("visible {urn:a}code maxLength = %q, want d1's 2", got)
	}
	fromD2 := mustBase(t, s, code)
	if fromD2 == nil {
		t.Fatal("d1's redefinition has no {base type definition}")
	}
	if got := fromD2.Name(); got != (xsd.QName{}) {
		t.Fatalf("d1's clause-1.1 original is named %s, want ·absent·", got)
	}
	if got := facetValue(t, fromD2, xsd.FacetMaxLength); got != "4" {
		t.Fatalf("d1's clause-1.1 original maxLength = %q, want d2's 4 — the chain skipped d2", got)
	}
	fromD3 := mustBase(t, s, fromD2)
	if fromD3 == nil {
		t.Fatal("d2's redefining declaration, built as d1's original, has no {base type definition}")
	}
	if got := fromD3.Name(); got != (xsd.QName{}) {
		t.Fatalf("d2's clause-1.1 original is named %s, want ·absent·", got)
	}
	if got := facetValue(t, fromD3, xsd.FacetMaxLength); got != "8" {
		t.Fatalf("d2's clause-1.1 original maxLength = %q, want d3's 8", got)
	}
}

// TestParseRedefineChainedComplexType is the same chain for a <complexType>, the
// shape MS-Additional2006-07-15/addB007 exercises. Both hops own their base, so
// the middle component is at once d1's anonymous clause-1.1 original AND a
// clause-1.2 redefining type over d3's — and the two anonymous levels must stay
// DISTINCT containers, which is what the {context} comparison pins.
func TestParseRedefineChainedComplexType(t *testing.T) {
	s, err := parseMap(t, "d1.xsd", chainedDocs(
		`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">`+
			`<xs:sequence><xs:element name="c" type="xs:string"/></xs:sequence>`+
			`</xs:extension></xs:complexContent></xs:complexType>`,
		`<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">`+
			`<xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence>`+
			`</xs:extension></xs:complexContent></xs:complexType>`,
		`<xs:complexType name="ct">`+
			`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ct := mustComplexType(t, s, xsd.QName{Space: "urn:a", Local: "ct"})
	if got := elementNamesOf(t, ct); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("the visible {urn:a}ct declares %v, want [a b c] — one particle per level of the chain", got)
	}
	fromD2 := mustOwnedBase(t, ct)
	if got := elementNamesOf(t, fromD2); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("d1's clause-1.1 original declares %v, want [a b] — the chain skipped d2", got)
	}
	fromD3 := mustOwnedBase(t, fromD2)
	if got := elementNamesOf(t, fromD3); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("d2's clause-1.1 original declares %v, want d3's [a]", got)
	}
	// Each original's {context} is the component that owns it, and the two are
	// different components: one identity per ownership edge. Sharing one token
	// would make the two anonymous levels indistinguishable containers.
	outer := mustComplexTypeContext(t, fromD2)
	inner := mustComplexTypeContext(t, fromD3)
	if outer.ID() == inner.ID() {
		t.Fatal("both chained originals carry the SAME {context} identity, but each names a different owner (src-expredef clause 1.1)")
	}
}

// mustComplexTypeContext returns c's {context}, failing unless it is present and
// is the ComplexTypeDefinitionContext arm src-expredef clause 1.1 requires.
func mustComplexTypeContext(t *testing.T, c xsd.ComplexType) xsd.ComplexTypeDefinitionContext {
	t.Helper()
	context, present := c.Context()
	if !present {
		t.Fatal("clause-1.1 original has no {context}, which §3.4.1 makes Required when {name} is absent")
	}
	ctd, isCTD := context.(xsd.ComplexTypeDefinitionContext)
	if !isCTD {
		t.Fatalf("clause-1.1 original's {context} = %T, want a ComplexTypeDefinitionContext naming the component that owns it", context)
	}
	return ctd
}

// TestParseRedefineChainedGroupStillRefused pins the deliberate fail-CLOSED half
// of the chain (the GAP( at chainedOriginal, parser/redefine.go): a chained
// <group> — src-expredef clause 2's single-component kind — is refused under the
// closing requirement even though §4.2.4 makes the schema valid.
//
// The reason is no longer that clause 6.2.2 is fail-open — this test's own file
// pins the clause being charged. It is that a chained redefining declaration
// contributes NO component (§4.2.4 clause 4.1.2, the outer redefinition owns the
// name), while 6.2.2 is charged over a pairing that adds one, so a MIDDLE level's
// obligation has nothing to attach to. Retire this test with the marker (#744).
func TestParseRedefineChainedGroupStillRefused(t *testing.T) {
	_, err := parseMap(t, "d1.xsd", chainedDocs(
		`<xs:group name="g"><xs:sequence><xs:group ref="tns:g"/>`+
			`<xs:element name="c" type="xs:string"/></xs:sequence></xs:group>`,
		`<xs:group name="g"><xs:sequence><xs:group ref="tns:g"/>`+
			`<xs:element name="b" type="xs:string"/></xs:sequence></xs:group>`,
		`<xs:group name="g"><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`))
	mustRule(t, err, "src-expredef")
}

// modelGroupElementNames lists, in document order, the local names of every
// element declaration reachable through g's resolved particle tree — the leaves a
// redefining <group> ends up with once its self-reference has been inlined.
func modelGroupElementNames(g xsd.ModelGroup) []string {
	var names []string
	for _, p := range g.Particles() {
		resolved, ok := p.Term().(xsd.ResolvedTerm)
		if !ok {
			continue
		}
		switch inner := resolved.Term.(type) {
		case xsd.ElementDeclaration:
			names = append(names, inner.Name().Local)
		case xsd.ModelGroup:
			names = append(names, modelGroupElementNames(inner)...)
		}
	}
	return names
}

// ctNamed is a plain top-level <complexType name="ct"> declaring one element
// named child. The duplicate-original tests below turn on how MANY declarations
// of {urn:a}ct one document carries, never on what any of them contains, so the
// element name is only there to keep the declarations distinguishable.
func ctNamed(child string) string {
	return `<xs:complexType name="ct"><xs:sequence>` +
		`<xs:element name="` + child + `" type="xs:string"/></xs:sequence></xs:complexType>`
}

// ctRedefining is the <redefine> child that redefines ct by extending it with one
// element named child — the self-reference src-expredef clause 1.1 pairs with the
// original.
func ctRedefining(child string) string {
	return `<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct">` +
		`<xs:sequence><xs:element name="` + child + `" type="xs:string"/></xs:sequence>` +
		`</xs:extension></xs:complexContent></xs:complexType>`
}

// TestParseRedefinedDocumentChainedPlusTopLevelRejected covers a redefined
// document that carries BOTH a nested <redefine> child for ct — which §4.2.4
// clause 4.1.1 makes a top-level definition of that document — and its own
// top-level ct, while d1 redefines ct. Clause 4.1.2 excepts both from the
// components d2 contributes, so neither reaches the by-name tables indexByName
// polices and the pair is charged where they are collected instead
// (redefineSet.recordOriginal), under sch-props-correct clause 2.
func TestParseRedefinedDocumentChainedPlusTopLevelRejected(t *testing.T) {
	_, err := parseMap(t, "d1.xsd", map[string]string{
		"d1.xsd": wrap("urn:a", `<xs:redefine schemaLocation="d2.xsd">`+ctRedefining("c")+`</xs:redefine>`),
		"d2.xsd": wrap("urn:a", `<xs:redefine schemaLocation="d3.xsd">`+ctRedefining("b")+`</xs:redefine>`+
			ctNamed("dup")),
		"d3.xsd": wrap("urn:a", ctNamed("a")),
	})
	mustRule(t, err, "sch-props-correct", "clause 2", "{urn:a}ct")
}

// TestParseRedefinedDocumentTwoTopLevelRejected is the same clause-2 collision
// with no chain in it: the redefined document declares ct twice at top level, and
// d1's redefinition excepts both under §4.2.4 clause 4.1.2, so the pre-scan alone
// writes one key twice.
func TestParseRedefinedDocumentTwoTopLevelRejected(t *testing.T) {
	_, err := parseMap(t, "d1.xsd", map[string]string{
		"d1.xsd": wrap("urn:a", `<xs:redefine schemaLocation="d2.xsd">`+ctRedefining("c")+`</xs:redefine>`),
		"d2.xsd": wrap("urn:a", ctNamed("a")+ctNamed("dup")),
	})
	mustRule(t, err, "sch-props-correct", "clause 2", "{urn:a}ct")
}

// TestParseDuplicateTopLevelOutsideRedefinitionUnchanged pins the two shapes
// recordOriginal must NOT displace. Nothing redefines the document declaring ct
// twice, so §4.2.4 clause 4.1.2 excepts neither declaration, both become named
// components, and indexByName (xsd/schema.go) charges clause 2 at finalize with
// its own {type definitions} message. recordOriginal is unreachable here — both
// its callers are gated on a redefinition being in force — and a duplicate check
// written at the wrong seam would double-charge or replace these verdicts.
func TestParseDuplicateTopLevelOutsideRedefinitionUnchanged(t *testing.T) {
	cases := []struct {
		name string
		docs map[string]string
	}{
		{
			name: "two top-level declarations",
			docs: map[string]string{
				"d1.xsd": wrap("urn:a", ctNamed("a")+ctNamed("dup")),
			},
		},
		{
			name: "redefine child plus a top-level declaration",
			docs: map[string]string{
				"d1.xsd": wrap("urn:a", `<xs:redefine schemaLocation="d2.xsd">`+ctRedefining("c")+`</xs:redefine>`+
					ctNamed("dup")),
				"d2.xsd": wrap("urn:a", ctNamed("a")),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMap(t, "d1.xsd", c.docs)
			mustRule(t, err, "sch-props-correct", "schema {type definitions}",
				"repeats the expanded name {urn:a}ct")
		})
	}
}

// TestParseRedefineGroupResolvesToOriginal is src-expredef clause 2 for a model
// group definition: the self-reference "is ·resolved·" to "a component which
// corresponds to the top-level definition item of that name and the appropriate
// kind in S2". The redefinition therefore contains the ORIGINAL's particles
// followed by its own — an extension by redefinition, the canonical use — rather
// than a <group ref> that would read as a circular group at finalize
// (mg-props-correct clause 2).
func TestParseRedefineGroupResolvesToOriginal(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence>`+
			`<xs:group ref="tns:g"/>`+
			`<xs:element name="b" type="xs:string"/>`+
			`</xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mgd := mustModelGroup(t, s, xsd.QName{Space: "urn:a", Local: "g"})
	particles := mgd.ModelGroup().Particles()
	if len(particles) != 2 {
		t.Fatalf("redefined {urn:a}g has %d particles, want 2", len(particles))
	}
	// The first particle is the self-reference, and it must be a RESOLVED inline
	// model group — the original's — not a by-name ModelGroupRef back to {urn:a}g.
	resolved, ok := particles[0].Term().(xsd.ResolvedTerm)
	if !ok {
		t.Fatalf("self-reference {term} = %#v, want a resolved inline model group (the original's)", particles[0].Term())
	}
	inner, ok := resolved.Term.(xsd.ModelGroup)
	if !ok {
		t.Fatalf("self-reference {term} = %#v, want an xsd.ModelGroup", resolved.Term)
	}
	if got := len(inner.Particles()); got != 1 {
		t.Fatalf("inlined original has %d particles, want lib.xsd's 1", got)
	}
}

// TestParseRedefineGroupBodylessOriginalRejected pins the src-expredef clause 2
// path onto #884's rejection: the ORIGINAL a redefining <group>'s self-reference
// resolves to is built by buildDefinitionModelGroup directly, never through
// produceModelGroupDefinition, because clause 4.1.2 keeps the redefined name out
// of the top-level tables. A bodyless original therefore reaches nothing that
// used to charge mgd-props-correct — it produced a Particle over a ZERO
// xs:ModelGroup, which xsd.NewParticle admits (its {term} is a non-nil
// ResolvedTerm) and the content automaton then PANICKED on, "non-exhaustive
// Compositor switch" over the zero compositor.
//
// The named-group body check closes it at the document fault, in the redefined
// document and at its own <group>, which is the only Loc that names the file the
// author must edit.
func TestParseRedefineGroupBodylessOriginalRejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence>`+
			`<xs:group ref="tns:g"/>`+
			`<xs:element name="b" type="xs:string"/>`+
			`</xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"/>`),
	})
	if err == nil {
		t.Fatal("Parse accepted a redefinition over a bodyless original, want a grammar fault")
	}
	var xe *xsderr.Error
	if errors.As(err, &xe) {
		t.Fatalf("error = %v (rule %s), want a plain Go error rather than a rule verdict", err, xe.Rule)
	}
	if !strings.Contains(err.Error(), "xs:namedGroup") {
		t.Fatalf("error = %v, want it to name the xs:namedGroup production the original violates", err)
	}
	if !strings.Contains(err.Error(), "lib.xsd:") {
		t.Fatalf("error = %v, want it positioned in lib.xsd, where the bodyless <group> is written (E3)", err)
	}
}

// TestParseRedefineGroupClause611Rejected covers src-redefine clause 6.1.1: with
// a qualifying self-reference present, "it has exactly one such group".
func TestParseRedefineGroupClause611Rejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence>`+
			`<xs:group ref="tns:g"/><xs:group ref="tns:g"/>`+
			`</xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
	})
	mustRule(t, err, "src-redefine", "6.1.1")
}

// TestParseRedefineGroupClause612Rejected covers src-redefine clause 6.1.2: "the
// ·actual value· of both that group's minOccurs and maxOccurs [attribute] is 1
// (or ·absent·)". Both directions are exercised — an optional self-reference and
// a repeatable one — since a check written for only one of them would pass half
// the clause.
func TestParseRedefineGroupClause612Rejected(t *testing.T) {
	for _, occurs := range []string{`minOccurs="0"`, `maxOccurs="2"`, `maxOccurs="unbounded"`} {
		t.Run(occurs, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
					`<xs:group name="g"><xs:sequence>`+
					`<xs:group ref="tns:g" `+occurs+`/>`+
					`</xs:sequence></xs:group>`+
					`</xs:redefine>`),
				"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
					`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
			})
			mustRule(t, err, "src-redefine", "6.1.2")
		})
	}
}

// TestParseRedefineGroupSelfReferenceUnderElementNotCounted is the trap a naive
// "grep for a ref= matching my own name" implementation falls into. Clause 6.1's
// precondition requires the self-referencing <group> to have NO <element>
// ancestor, so the two <group ref="tns:g"> below — one at top level, one buried
// under a local element's inline complex type — are ONE counted self-reference,
// not two: clause 6.1.1 is satisfied, and the case is accepted.
//
// The uncounted reference is nonetheless RESOLVED to the original, per
// src-expredef clause 2, whose rule carries no such exclusion — otherwise it
// would name the redefinition and be rejected as a circular <group ref> graph.
func TestParseRedefineGroupSelfReferenceUnderElementNotCounted(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence>`+
			`<xs:group ref="tns:g"/>`+
			`<xs:element name="nested"><xs:complexType><xs:sequence>`+
			`<xs:group ref="tns:g"/>`+
			`</xs:sequence></xs:complexType></xs:element>`+
			`</xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v — a self-reference under an <element> ancestor is not counted by src-redefine clause 6.1", err)
	}
	mgd := mustModelGroup(t, s, xsd.QName{Space: "urn:a", Local: "g"})
	if got := len(mgd.ModelGroup().Particles()); got != 2 {
		t.Fatalf("redefined {urn:a}g has %d particles, want 2", got)
	}
}

// TestParseRedefineAttributeGroupResolvesToOriginal is src-expredef clause 2's
// attributeGroup half: the self-reference contributes the ORIGINAL group's
// {attribute uses}, which is the whole point of redefining an attribute group by
// self-reference. Resolved by name instead, it would name the redefinition
// itself, a §3.6.2.1 self-cycle the closure takes as contributing nothing at all.
func TestParseRedefineAttributeGroupResolvesToOriginal(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:attributeGroup name="ag">`+
			`<xs:attributeGroup ref="tns:ag"/>`+
			`<xs:attribute name="b" type="xs:string"/>`+
			`</xs:attributeGroup>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:attributeGroup name="ag">`+
			`<xs:attribute name="a" type="xs:string"/></xs:attributeGroup>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ag := mustAttributeGroup(t, s, xsd.QName{Space: "urn:a", Local: "ag"})
	if got := len(ag.AttributeUses()); got != 2 {
		t.Fatalf("redefined {urn:a}ag has %d attribute uses, want 2 (the original's a plus the redefinition's b)", got)
	}
}

// TestParseRedefineAttributeGroupClause71Rejected covers src-redefine clause
// 7.1's "then it has exactly one such group". Clause 7.1 carries NEITHER of
// clause 6.1's extra conditions, so the sibling test for 6.1.2 has deliberately
// no attributeGroup twin.
func TestParseRedefineAttributeGroupClause71Rejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:attributeGroup name="ag">`+
			`<xs:attributeGroup ref="tns:ag"/><xs:attributeGroup ref="tns:ag"/>`+
			`</xs:attributeGroup>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:attributeGroup name="ag">`+
			`<xs:attribute name="a" type="xs:string"/></xs:attributeGroup>`),
	})
	mustRule(t, err, "src-redefine", "7.1")
}

// TestParseRedefineAttributeGroupClause722Accepts is clause 7.2's control: a
// redefining <attributeGroup> with NO self-reference that genuinely restricts
// the original — one attribute narrowed to a derived type, one OPTIONAL
// attribute dropped — is accepted, and the component the schema carries is the
// redefinition's own <attribute> children alone (the Note under clause 7.2.2:
// "No inheritance from the <redefine>d attribute group occurs").
func TestParseRedefineAttributeGroupClause722Accepts(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:attributeGroup name="ag">`+
			`<xs:attribute name="a" type="xs:token"/>`+
			`</xs:attributeGroup>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:attributeGroup name="ag">`+
			`<xs:attribute name="a" type="xs:string"/>`+
			`<xs:attribute name="b" type="xs:string"/></xs:attributeGroup>`),
	})
	if err != nil {
		t.Fatalf("a restrictive clause-7.2 redefinition was rejected: %v", err)
	}
	ag := mustAttributeGroup(t, s, xsd.QName{Space: "urn:a", Local: "ag"})
	if got := len(ag.AttributeUses()); got != 1 {
		t.Fatalf("redefined {urn:a}ag has %d attribute uses, want 1: clause 7.2 inherits nothing from the original", got)
	}
}

// TestParseRedefineGroupClause622 covers src-redefine clause 6.2.2 over the
// producer: a redefining <group> with NO self-reference must accept a subset of
// the element sequences the original in S2 accepts, charged to src-redefine at
// finalize over the pairing produceRedefinition records.
//
// The widening cases are also the regression test for the pairing's own hazard:
// B and R must be two components. Were the original fetched from a memo that
// produceRedefinition had already filled for this name, B would BE R, every case
// below would decide V(R) ⊆ V(R), and the widening rows would pass as "valid"
// with nothing else in the suite noticing. The memo is keyed by declaration
// (symbols.builtGroups) so it no longer confuses the two, and the original is
// built through the UNMEMOISED produceModelGroupDefinition regardless — see
// redefinedGroupRestricted for what a memo entry from that throwaway build would
// hand run.
func TestParseRedefineGroupClause622(t *testing.T) {
	for _, tc := range []struct {
		name       string
		redefining string
		original   string
		wantSubset bool
	}{
		{"an added element the original never admits",
			`<xs:element name="a" type="xs:string"/><xs:element name="c" type="xs:string"/>`,
			`<xs:element name="a" type="xs:string"/>`, false},
		{"an occurrence range widened",
			`<xs:element name="a" type="xs:string" maxOccurs="5"/>`,
			`<xs:element name="a" type="xs:string" maxOccurs="3"/>`, false},
		{"an optional member dropped",
			`<xs:element name="a" type="xs:string"/>`,
			`<xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:string" minOccurs="0"/>`, true},
		{"an occurrence range narrowed inside a wide one",
			`<xs:element name="a" type="xs:string" minOccurs="3" maxOccurs="6"/>`,
			`<xs:element name="a" type="xs:string" minOccurs="0" maxOccurs="100"/>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
					`<xs:group name="g"><xs:sequence>`+tc.redefining+`</xs:sequence></xs:group>`+
					`</xs:redefine>`),
				"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
					tc.original+`</xs:sequence></xs:group>`),
			})
			if !tc.wantSubset {
				mustRule(t, err, "src-redefine", "6.2.2", "{urn:a}g")
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v — the redefinition accepts a subset of the original's sequences", err)
			}
			// Clause 6.2 inherits nothing: the redefinition's {model group} is its own
			// <sequence> and the original contributes no component (clause 4.1.2).
			mgd := mustModelGroup(t, s, xsd.QName{Space: "urn:a", Local: "g"})
			if got := modelGroupElementNames(mgd.ModelGroup()); !slices.Equal(got, []string{"a"}) {
				t.Fatalf("the redefinition's {model group} declares %v, want its own [a]", got)
			}
		})
	}
}

// TestParseRedefineGroupClause622OriginalRefResolved pins the src-resolve
// coverage the pairing's B side would otherwise have none of. The ORIGINAL is in
// no property and no index (§4.2.4 clause 4.1.2), so nothing but the pairing
// reaches it, and addTerm answers an unresolvable ref inside it with an EMPTY
// fragment: the dangling name would silently shrink B instead of being reported —
// accepting the schema in the <element ref> case below, and in the <group ref>
// one manufacturing a clause-6.2.2 rejection of a redefinition that violates
// nothing. Both are charged src-resolve against the name that does not resolve.
func TestParseRedefineGroupClause622OriginalRefResolved(t *testing.T) {
	for _, tc := range []struct{ name, original string }{
		// The miss swallowed: B still accepts (a), so the schema would be accepted
		// with a name that resolves to nothing.
		{"element ref", `<xs:element name="a" type="xs:string"/><xs:element ref="tns:nope"/>`},
		// The miss turned into a verdict: B would accept the empty sequence ALONE,
		// so the redefinition's own (a) would fail clause 6.2.2.
		{"group ref", `<xs:group ref="tns:nope"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
					`<xs:group name="g"><xs:sequence>`+
					`<xs:element name="a" type="xs:string"/>`+
					`</xs:sequence></xs:group>`+
					`</xs:redefine>`),
				"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
					tc.original+`</xs:sequence></xs:group>`),
			})
			mustRule(t, err, "src-resolve", "{urn:a}nope")
		})
	}
}

// TestParseRedefineGroupClause622OriginalRegistersNothing pins the other half of
// how the original is built (discardingComponents): producing it must register
// NO component. §4.2.4 clause 4.1.2 excepts the original from what D1's schema
// includes, so a named <key> on a local element of its body would be registered
// for the FIRST time by the 6.2.2 pairing and collide sch-props-correct clause 2
// against the redefinition's own copy of that key — the commonest redefine shape,
// a redefinition that keeps the body it narrows.
func TestParseRedefineGroupClause622OriginalRegistersNothing(t *testing.T) {
	keyed := `<xs:element name="e"><xs:key name="k">` +
		`<xs:selector xpath="a"/><xs:field xpath="@id"/></xs:key></xs:element>`
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence>`+keyed+`</xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+keyed+
			`<xs:element name="opt" type="xs:string" minOccurs="0"/>`+
			`</xs:sequence></xs:group>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := len(s.IdentityConstraints()); got != 1 {
		t.Fatalf("{identity-constraint definitions} has %d members, want 1: only the redefinition's <key> is registered", got)
	}
}

// TestParseRedefineGroupClause622NotChargedOnSelfReference is clause 6's
// dispatch: with a counted self-reference the redefinition is on clause 6.1's
// branch, which states NO containment obligation, so a body that widens what it
// redefines — the original's particles plus one more — is accepted. Charging
// 6.2.2 there would reject every canonical extension-by-redefinition.
func TestParseRedefineGroupClause622NotChargedOnSelfReference(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name="g"><xs:sequence><xs:group ref="tns:g"/>`+
			`<xs:element name="b" type="xs:string"/></xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name="g"><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v — clause 6.1's branch states no containment obligation", err)
	}
	mgd := mustModelGroup(t, s, xsd.QName{Space: "urn:a", Local: "g"})
	if got := modelGroupElementNames(mgd.ModelGroup()); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("the redefinition's {model group} declares %v, want [a b]", got)
	}
}

// TestParseRedefineAttributeGroupClause722Rejected covers src-redefine clause
// 7.2.2 over the producer: a redefining <attributeGroup> with no self-reference
// whose {attribute uses}/{attribute wildcard} do NOT satisfy clause 3 of
// derivation-ok-restriction (§3.4.6.3, c-ran) against the original's is
// rejected, charged to src-redefine — never to derivation-ok-restriction, which
// is a rule about a complex type that does not exist here.
//
// The first three shapes are the three the W3C suite's MS-AttributeGroup
// redefine cohort mixes: an added attribute the original neither declares nor
// admits, a re-typing that is not a derivation, and a required attribute
// dropped. The fourth is c-ran clause 3's keyword comparison over the two
// {attribute wildcard}s (loc-testSubP clause 3, checkAttributeRestrictionWildcard).
func TestParseRedefineAttributeGroupClause722Rejected(t *testing.T) {
	for _, tc := range []struct {
		name         string
		redefining   string
		original     string
		messageNames string
	}{
		{"an added attribute the original does not admit",
			`<xs:attribute name="a" type="xs:string"/><xs:attribute name="extra" type="xs:string"/>`,
			`<xs:attribute name="a" type="xs:string"/>`,
			"extra"},
		{"a re-typing that is not a derivation",
			`<xs:attribute name="a" type="xs:string"/>`,
			`<xs:attribute name="a" type="xs:int"/>`,
			"a"},
		{"a required attribute the redefinition drops",
			`<xs:attribute name="a" type="xs:string"/>`,
			`<xs:attribute name="a" type="xs:string"/><xs:attribute name="r" type="xs:string" use="required"/>`,
			"r"},
		{"a lax wildcard over a strict one",
			`<xs:anyAttribute processContents="lax"/>`,
			`<xs:anyAttribute/>`,
			"the redefining attribute group {urn:a}ag must restrict the original attribute group {urn:a}ag, but the original attribute group {urn:a}ag binds an attribute both {attribute wildcard}s admit and no declaration governs to a strict wildcard while the restriction binds it to a lax wildcard, and loc-testSubP clause 3 "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
					`<xs:attributeGroup name="ag">`+tc.redefining+`</xs:attributeGroup>`+
					`</xs:redefine>`),
				"lib.xsd": wrap("urn:a", `<xs:attributeGroup name="ag">`+tc.original+`</xs:attributeGroup>`),
			})
			mustRule(t, err, "src-redefine", "7.2.2", tc.messageNames,
				"the redefining attribute group {urn:a}ag", "the original attribute group {urn:a}ag")
		})
	}
}

// TestParseRedefineNamelessGroupRejected reaches produceModelGroupDefinition's
// own nameless-{name} guard, which since #305 no longer lies on run's
// document-order dispatch path (that fault is topLevelName's a step earlier).
// The remaining live entry is THIS one: produceRedefinition mints the redefining
// declaration's name from the <redefine> child's own name attribute and never
// consults topLevelName, so a <group name=""> child reaches
// buildModelGroupDefinition with an empty local part. The redefined document
// declares the same nameless <group> because src-expredef's closing requirement
// is charged first — without an original of the same kind and name the case
// would be rejected there instead, one step short of the guard. An ABSENT name
// cannot get this far either: newRedefineSet rejects a <redefine> child with no
// name attribute while reading the set.
//
// The redefining body holds a local <element> for the #206 reason — deleting the
// guard makes the verdict content-dependent, and this body turns it into a bogus
// e-props-correct against the {scope}.{parent} rather than the grammar fault
// below.
func TestParseRedefineNamelessGroupRejected(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:group name=""><xs:sequence>`+
			`<xs:element name="b" type="xs:string"/></xs:sequence></xs:group>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:group name=""><xs:sequence>`+
			`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`),
	})
	if err == nil {
		t.Fatalf("Parse succeeded, want a grammar fault for the redefining <group>'s empty name")
	}
	var xe *xsderr.Error
	if errors.As(err, &xe) {
		t.Fatalf("error = %v (rule %s), want a plain Go error rather than a rule verdict", err, xe.Rule)
	}
	if !strings.Contains(err.Error(), "top-level <group>") || !strings.Contains(err.Error(), "no usable name") {
		t.Fatalf("error = %v, want the <group> grammar fault reporting the unusable name", err)
	}
	if !strings.Contains(err.Error(), "xs:namedGroup") {
		t.Fatalf("error = %v, want it to name xs:namedGroup, the production whose required name is missing (#975)", err)
	}
}

// TestParseRedefineContentModelRejectsOverrideOnlyKinds pins the load-bearing
// structural difference between the two mechanisms: §4.2.4's content model is
// (annotation | (simpleType | complexType | group | attributeGroup))*, so the
// three kinds only <override> admits — <element>, <attribute>, <notation> — are
// a grammar fault here and are reported, never silently dropped.
func TestParseRedefineContentModelRejectsOverrideOnlyKinds(t *testing.T) {
	for _, child := range []string{
		`<xs:element name="doc" type="xs:string"/>`,
		`<xs:attribute name="a" type="xs:string"/>`,
		`<xs:notation name="n" public="p"/>`,
	} {
		t.Run(child, func(t *testing.T) {
			_, err := parseMap(t, "main.xsd", map[string]string{
				"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+child+`</xs:redefine>`),
				"lib.xsd":  wrap("urn:a", `<xs:element name="doc" type="xs:date"/>`),
			})
			if err == nil {
				t.Fatalf("Parse succeeded, want a content-model fault for a <redefine> child §4.2.4 does not admit")
			}
			var xe *xsderr.Error
			if errors.As(err, &xe) {
				t.Fatalf("error = %v, want a plain Go error rather than a rule verdict", err)
			}
		})
	}
}

// TestParseRedefineAndIncludeOfOneDocumentCollide covers the cross-mechanism
// conflict the load-once index deliberately does NOT paper over: the document is
// reached once plainly and once under a redefinition that names code, so the
// plain reading contributes lib's own code and the redefining document
// contributes the replacement §4.2.4 clause 4.1.1 makes ITS definition. The
// duplicate expanded name is a genuine sch-props-correct clause 2 (c-nmd)
// rejection — not something to silently pick a winner for.
func TestParseRedefineAndIncludeOfOneDocumentCollide(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code"/></xs:simpleType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	mustRule(t, err, "sch-props-correct")
}

// TestParseRedefineTargetAlsoReadPlainlyComposesOnce is the other half of that
// shape: what the <redefine> does NOT name (#1349). §4.2.4's construction gives
// the redefining schema "components identical to all the schema components of
// S2, with the exception of those explicitly redefined" — §4.2.3 clause 3.1.2's
// wording for a plain <include> verbatim, over the same schema(D2), which §4.2.1
// makes a pure function of D2 — so ct is ONE component both readings draw in and
// is contributed once. It is NOT two equal components that collide under
// sch-props-correct clause 2, which is what composing lib.xsd twice made of every
// definition the <redefine> left alone.
//
// The assembly is still rejected, and on g: the plain reading contributes lib's
// own g and the redefinition contributes its replacement. That is what makes the
// charge readable — it names the definition the two readings genuinely disagree
// about — and it is why the assertion is on the expanded name, not on the rule
// alone.
//
// Both orders are run because the readings merge into whichever discovery came
// first: a plain reading landing on a redefined one must widen it back to
// contributing everything, exactly as a redefinition landing on a plain one must
// not narrow it.
func TestParseRedefineTargetAlsoReadPlainlyComposesOnce(t *testing.T) {
	lib := wrap("urn:a", `<xs:group name="g"><xs:sequence/></xs:group>`+
		`<xs:complexType name="ct"><xs:sequence><xs:element name="e" type="xs:string"/></xs:sequence></xs:complexType>`)
	redefining := wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
		`<xs:group name="g"><xs:sequence><xs:element name="d" type="xs:string"/></xs:sequence></xs:group>`+
		`</xs:redefine>`)
	cases := []struct {
		name string
		docs map[string]string
	}{
		{
			name: "plainly first",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
					`<xs:include schemaLocation="red.xsd"/>`),
				"lib.xsd": lib,
				"red.xsd": redefining,
			},
		},
		{
			name: "redefined first",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:include schemaLocation="red.xsd"/>`+
					`<xs:include schemaLocation="lib.xsd"/>`),
				"lib.xsd": lib,
				"red.xsd": redefining,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := readingsOf(t, "main.xsd", "lib.xsd", c.docs)
			if n != 1 {
				t.Fatalf("lib.xsd read %d times, want 1: a document reached plainly and as a <redefine> target is one composition", n)
			}
			mustRule(t, err, "sch-props-correct", "repeats the expanded name {urn:a}g")
			if strings.Contains(err.Error(), "{urn:a}ct") {
				t.Fatalf("error %q charges {urn:a}ct, which nothing redefines: the untouched definition must be minted once", err)
			}
		})
	}
}

// mergedPlainAndRedefine is the assembly #1361 is about: root.xsd includes
// lib.xsd plainly AND includes r.xsd, which redefines lib.xsd. Every reading of
// lib.xsd must except a definition for §4.2.4 clause 4.1.2 to withhold it
// (redefinitions.excepts is the INTERSECTION of the readings), so the plain
// reading makes lib.xsd contribute the ORIGINAL while the <redefine> contributes
// its replacement, and both claim the one expanded name.
//
// The layout is load-bearing: each document's body opens with a newline, so the
// original stands at lib.xsd:2:3 and the redefining declaration at r.xsd:3:5.
func mergedPlainAndRedefine(original, redefining string) map[string]string {
	return map[string]string{
		"root.xsd": wrap("urn:t", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:include schemaLocation="r.xsd"/>`),
		"lib.xsd": wrap("urn:t", "\n  "+original),
		"r.xsd": wrap("urn:t", "\n<xs:redefine schemaLocation=\"lib.xsd\">\n    "+
			redefining+"</xs:redefine>"),
	}
}

// TestParseRedefineMergedWithPlainIncludeRegistersTheRedefinition pins WHICH
// component produceRedefinition registers under a redefined name when the
// redefined document also contributes that name plainly. The verdict —
// sch-props-correct (§3.17.6.1) clause 2 — is the same either way and is not
// what is at issue; the two POSITIONS the charge carries are, because they are
// the only observable of the component's provenance in an assembly clause 2
// rejects before anything else can read it.
//
// A name-keyed build memo answered produceRedefinition's own build with the
// ORIGINAL, which then went to the builder as the redefinition: the charge named
// lib.xsd twice and, for <group>, the pairing handed
// xsd.SchemaBuilder.AddRedefiningModelGroup was (original, original), so
// src-redefine (§4.2.4) clause 6.2.2 would have decided V(R) ⊆ V(R) (#1361).
// <attributeGroup> is the control: buildAttributeGroup memoises nothing
// (redefinedAttributeGroupRestricted), and it named both positions throughout.
//
// Each position is pinned as ONE contiguous substring, the offending one as the
// message's own prefix through the property name (#1048), so a message that
// swapped its two locations satisfies neither.
//
// Clause 6.2.2 itself cannot be the charged rule in this shape, and no shape
// exposes it: the memo answers with a component built from another declaration
// only when that declaration contributes a component of the same expanded name,
// which is exactly when clause 2 fires — and indexByName charges clause 2 in
// SchemaBuilder.finalize, before Schema.resolve reaches
// checkModelGroupRedefinitions. The clause 6.2.2 pairing is exercised where it
// is reachable instead, by TestParseRedefineGroupClause622.
func TestParseRedefineMergedWithPlainIncludeRegistersTheRedefinition(t *testing.T) {
	for _, c := range []struct {
		name, property     string
		original, redefine string
	}{
		{
			name:     "group",
			property: "{model group definitions}",
			original: `<xs:group name="g"><xs:sequence>` +
				`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`,
			redefine: `<xs:group name="g"><xs:sequence>` +
				`<xs:element name="a" type="xs:token"/></xs:sequence></xs:group>`,
		},
		{
			// A redefining <group> that WIDENS its original: the body clause
			// 6.2.2 rejects wherever the pairing is real, so registering the
			// original in its place would make the pair (original, original)
			// and the obligation vacuous.
			name:     "group that does not restrict its original",
			property: "{model group definitions}",
			original: `<xs:group name="g"><xs:sequence>` +
				`<xs:element name="a" type="xs:string"/></xs:sequence></xs:group>`,
			redefine: `<xs:group name="g"><xs:sequence>` +
				`<xs:element name="a" type="xs:string"/>` +
				`<xs:element name="c" type="xs:string"/></xs:sequence></xs:group>`,
		},
		{
			name:     "complexType",
			property: "{type definitions}",
			original: `<xs:complexType name="ct"><xs:sequence>` +
				`<xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`,
			redefine: `<xs:complexType name="ct"><xs:complexContent>` +
				`<xs:restriction base="tns:ct"><xs:sequence>` +
				`<xs:element name="a" type="xs:string"/></xs:sequence>` +
				`</xs:restriction></xs:complexContent></xs:complexType>`,
		},
		{
			name:     "attributeGroup",
			property: "{attribute group definitions}",
			original: `<xs:attributeGroup name="ag">` +
				`<xs:attribute name="a" type="xs:string"/></xs:attributeGroup>`,
			redefine: `<xs:attributeGroup name="ag">` +
				`<xs:attribute name="a" type="xs:token"/></xs:attributeGroup>`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMap(t, "root.xsd", mergedPlainAndRedefine(c.original, c.redefine))
			mustRule(t, err, "sch-props-correct")
			offending := "r.xsd:3:5: [sch-props-correct] schema " + c.property
			if !strings.Contains(err.Error(), offending) {
				t.Fatalf("error %q does not open %q: the component registered under the redefined name is the ORIGINAL, not the redefining declaration", err, offending)
			}
			if !strings.Contains(err.Error(), "(first declared at lib.xsd:2:3)") {
				t.Fatalf("error %q does not cite the original at lib.xsd:2:3 as the first declaration", err)
			}
		})
	}
}

// TestParseRedefineComplexTypeRestrictionPairsWithOriginal is src-expredef
// clause 1.2 on the RESTRICTION alternant: the component registered under the
// redefined name derives by restriction from the clause-1.1 original — the
// anonymous, {name}-·absent· component built from the redefined document's own
// declaration — and never carries that original's own ordinary xs:anyType base
// in its place. It is TestParseRedefineComplexTypePairsWithOriginal's sibling,
// which pins the extension alternant.
func TestParseRedefineComplexTypeRestrictionPairsWithOriginal(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:complexType name="ct"><xs:complexContent><xs:restriction base="tns:ct">`+
			`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>`+
			`</xs:restriction></xs:complexContent></xs:complexType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:complexType name="ct">`+
			`<xs:sequence><xs:element name="a" type="xs:string"/>`+
			`<xs:element name="b" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ct := mustComplexType(t, s, xsd.QName{Space: "urn:a", Local: "ct"})
	if ct.DerivationMethod() != xsd.DerivationRestriction {
		t.Fatalf("{derivation method} = %s, want restriction", ct.DerivationMethod())
	}
	base := mustOwnedBase(t, ct)
	if got := elementNamesOf(t, base); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("the clause-1.1 original's content model declares %v, want the redefined document's [a b]", got)
	}
	if got := elementNamesOf(t, ct); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("the redefinition's content model declares %v, want its own [a]", got)
	}
}

// TestParseRedefineSameDocumentTwiceDiffers pins what survives dropping the
// redefinition from the load-once identity (#1349): ONE document redefined two
// different ways is one composition, and the two redefinitions still collide.
// The redefined document contributes nothing for code — every reading excepts it
// under §4.2.4 clause 4.1.2 — while both <redefine> elements contribute a
// replacement of that name to the SAME document, which is the sch-props-correct
// clause 2 collision here. Were the second <redefine>'s original left unrecorded
// by the reading that merged onto the first, src-expredef's closing requirement
// would be charged in its place against a definition that is there.
func TestParseRedefineSameDocumentTwiceDiffers(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code">`+
			`<xs:maxLength value="2"/></xs:restriction></xs:simpleType>`+
			`</xs:redefine>`+
			`<xs:redefine schemaLocation="lib.xsd">`+
			`<xs:simpleType name="code"><xs:restriction base="tns:code">`+
			`<xs:maxLength value="3"/></xs:restriction></xs:simpleType>`+
			`</xs:redefine>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	mustRule(t, err, "sch-props-correct")
}

// TestParseRedefineCycleRejected covers the one shape composing a <redefine>
// target once made reachable (#1349): two documents that redefine EACH OTHER for
// one (kind, name). §4.2.4 clause 4.1.1 makes each document's redefining child
// the "top-level definition item of that name and kind in the <redefine>d schema
// document" the other one pairs with, so the base chain src-expredef clause 1.1
// builds runs a.ct ⊳ b.ct ⊳ a.ct ⊳ … and never reaches xs:anyType.
//
// Every hop of that chain is a clause 1.1 original, built by a direct call to
// produceComplexType or constructSimpleType rather than through a named build
// (buildComplexType's memo is keyed by the declaration, which contributes the
// ordinary named component too, and the original's ·absent· {name} gives
// buildSimpleType's by-name memo nothing to hold), so buildComplexType's
// on-stack sentinel never sees a hop; enterOriginal is what bounds it, and the
// verdict is the acyclicity rule for the kind — the same rule the finalize-side
// walks charge. Without the bound the walk recurses until the stack dies, which
// is what the W3C suite's ibmData/schema_invalid/S4_2_4 cyclic-redefine cases
// meet.
func TestParseRedefineCycleRejected(t *testing.T) {
	cases := []struct {
		name   string
		rule   xsderr.Rule
		child  string
		clause string
	}{
		{
			name:   "complexType",
			rule:   "ct-props-correct",
			child:  `<xs:complexType name="ct"><xs:complexContent><xs:extension base="tns:ct"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`,
			clause: "clause 3",
		},
		{
			name:   "simpleType",
			rule:   "st-props-correct",
			child:  `<xs:simpleType name="ct"><xs:restriction base="tns:ct"><xs:maxLength value="4"/></xs:restriction></xs:simpleType>`,
			clause: "clause 2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMap(t, "a.xsd", map[string]string{
				"a.xsd": wrap("urn:a", `<xs:redefine schemaLocation="b.xsd">`+c.child+`</xs:redefine>`),
				"b.xsd": wrap("urn:a", `<xs:redefine schemaLocation="a.xsd">`+c.child+`</xs:redefine>`),
			})
			mustRule(t, err, c.rule, "{urn:a}ct", c.clause)
		})
	}
}

// TestProduceRedefineIsSkipped pins that [Produce] — the single-document entry
// point, which follows no ·inter-schema-document reference· — still skips a
// <redefine> entirely rather than half-applying it. There is no redefined
// document, hence no original to pair with, so producing the children would
// fabricate a src-expredef verdict against a document that is fine.
func TestProduceRedefineIsSkipped(t *testing.T) {
	s, err := produce(t, wrap("urn:a", `<xs:redefine schemaLocation="lib.xsd">`+
		`<xs:simpleType name="code"><xs:restriction base="tns:code"/></xs:simpleType>`+
		`</xs:redefine>`+
		`<xs:element name="root" type="xs:string"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"}); !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	if _, ok := s.Type(xsd.QName{Space: "urn:a", Local: "code"}); ok {
		t.Fatalf("Produce must contribute nothing for an unfollowed <redefine>")
	}
}
