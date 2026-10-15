package parser_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// typeTableOf produces doc and returns the {type table} of its top-level
// element named name, reporting whether the property is present.
func typeTableOf(t *testing.T, doc string, name xsd.QName) (xsd.TypeTable, bool) {
	t.Helper()
	s, err := produce(t, doc)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(name)
	if !ok {
		t.Fatalf("element %s not found", name)
	}
	return ed.TypeTable()
}

// altTypeNames returns the local part of the {type definition} REFERENCE of
// every {alternatives} member, in order, so a document-order assertion reads as
// one comparison. An entry on §3.12.2's inline arm has no name and reports
// "<inline>", which no schema-document type name can collide with.
func altTypeNames(t *testing.T, alts []xsd.TypeAlternative) []string {
	t.Helper()
	names := make([]string, 0, len(alts))
	for _, a := range alts {
		names = append(names, altTypeName(t, a))
	}
	return names
}

// altTypeName is altTypeNames for one alternative.
func altTypeName(t *testing.T, alt xsd.TypeAlternative) string {
	t.Helper()
	switch ref := alt.TypeDefinition().(type) {
	case xsd.TypeDefinitionRef:
		return ref.Name.Local
	case xsd.InlineTypeDefinition:
		return "<inline>"
	default:
		t.Fatalf("{type definition} = %T, want a reference or an inline definition", ref)
		return ""
	}
}

const typeTableTypes = `<xs:complexType name="B"><xs:sequence/></xs:complexType>
	<xs:complexType name="T"><xs:complexContent><xs:extension base="B"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType>
	<xs:complexType name="U"><xs:complexContent><xs:extension base="B"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType>
	<xs:complexType name="V"><xs:complexContent><xs:extension base="B"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`

// A trailing <alternative> with no test attribute maps to {default type
// definition} and is NOT an {alternatives} member (§3.3.2.1, case 1).
func TestProduceTypeTableTrailingUntestedAlternativeIsTheDefault(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='t'" type="T"/>
	  <xs:alternative test="@k='u'" type="U"/>
	  <xs:alternative type="V"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	alts := tt.Alternatives()
	if got := altTypeNames(t, alts); len(got) != 2 || got[0] != "T" || got[1] != "U" {
		t.Fatalf("{alternatives} = %v, want [T U] in document order", got)
	}
	test, hasTest := alts[0].Test()
	if !hasTest || test.Expression() != "@k='t'" {
		t.Errorf("{alternatives}[0].{test} = %q/%t, want %q present", test.Expression(), hasTest, "@k='t'")
	}
	dflt := tt.DefaultTypeDefinition()
	if _, hasTest := dflt.Test(); hasTest {
		t.Error("{default type definition}.{test} is present, want ·absent·")
	}
	if got := altTypeName(t, dflt); got != "V" {
		t.Errorf("{default type definition}.{type definition} = %s, want V", got)
	}
}

// With every <alternative> tested, {default type definition} is SYNTHESIZED
// from the declaration's own {type definition} (§3.3.2.1, case 2).
func TestProduceTypeTableSynthesizesDefaultFromDeclaredType(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='t'" type="T"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	if got := altTypeNames(t, tt.Alternatives()); len(got) != 1 || got[0] != "T" {
		t.Fatalf("{alternatives} = %v, want [T]", got)
	}
	dflt := tt.DefaultTypeDefinition()
	if _, hasTest := dflt.Test(); hasTest {
		t.Error("{default type definition}.{test} is present, want ·absent·")
	}
	if got := altTypeName(t, dflt); got != "B" {
		t.Errorf("{default type definition}.{type definition} = %s, want the declaration's own B", got)
	}
}

// An <element> with no <alternative> child keeps {type table} ·absent·.
func TestProduceTypeTableAbsentWithoutAlternatives(t *testing.T) {
	if _, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B"/>`), xsd.QName{Local: "e"}); present {
		t.Error("{type table} is present, want ·absent· for an <element> with no <alternative>")
	}
}

// §3.12.2 declare-ta's INLINE arm — a <complexType> child in place of a type
// attribute — maps to a {type definition} the alternative OWNS, and the table
// carries it beside a by-name entry rather than being withheld (#851).
func TestProduceTypeTableInlineComplexAlternative(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='t'" type="T"/>
	  <xs:alternative test="@k='i'">`+inlineExtensionOfB+`</xs:alternative>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present for an <alternative> on the inline arm")
	}
	if got := altTypeNames(t, tt.Alternatives()); len(got) != 2 || got[0] != "T" || got[1] != "<inline>" {
		t.Fatalf("{alternatives} = %v, want [T <inline>]", got)
	}
	inline, ok := tt.Alternatives()[1].TypeDefinition().(xsd.InlineTypeDefinition)
	if !ok {
		t.Fatalf("{alternatives}[1].{type definition} = %T, want an InlineTypeDefinition", tt.Alternatives()[1].TypeDefinition())
	}
	ct, ok := inline.Definition.(xsd.ComplexType)
	if !ok {
		t.Fatalf("the owned definition is %T, want a ComplexType", inline.Definition)
	}
	if ct.Name() != (xsd.QName{}) {
		t.Errorf("the owned definition is NAMED %v, want anonymous", ct.Name())
	}
	if _, hasContext := ct.Context(); !hasContext {
		t.Error("the owned definition has an ·absent· {context}, want the enclosing element declaration (§3.4.2.1 dcl.ctd.common)")
	}
}

// inlineExtensionOfB is the inline <complexType> the alternative fixtures own.
// It EXTENDS B rather than deriving from xs:anyType so that e-props-correct
// clause 7 — every entry ·validly substitutable· for the declaration's own
// {type definition} — is satisfied by the fixture and does not reject a document
// whose subject is the mapping.
const inlineExtensionOfB = `<xs:complexType><xs:complexContent><xs:extension base="B"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType>`

// The inline arm also serves the SIMPLE type child, through the same
// constructSimpleType entry point an inline <simpleType> child of an <element>
// takes.
func TestProduceTypeTableInlineSimpleAlternative(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", `
	<xs:element name="e" type="xs:string">
	  <xs:alternative test="@k='i'"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:alternative>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present for an <alternative> on the inline simple-type arm")
	}
	inline, ok := tt.Alternatives()[0].TypeDefinition().(xsd.InlineTypeDefinition)
	if !ok {
		t.Fatalf("{alternatives}[0].{type definition} = %T, want an InlineTypeDefinition", tt.Alternatives()[0].TypeDefinition())
	}
	st, ok := inline.Definition.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("the owned definition is %T, want a *SimpleType", inline.Definition)
	}
	if st.Name() != (xsd.QName{}) {
		t.Errorf("the owned definition is NAMED %v, want anonymous", st.Name())
	}
}

// A synthesized {default type definition} takes the declaring element's own
// {type definition} WHOLE (§3.3.2.1 case 2), so an <element> owning an anonymous
// type gets a table whose default owns that same type — where the whole table
// used to be withheld for want of a name.
//
// The tested alternative names ·xs:error·, and that is forced rather than
// incidental: e-props-correct clause 7 requires every entry to be ·validly
// substitutable· for the declaration's own {type definition}, and nothing an
// <alternative> can NAME is substitutable for the anonymous COMPLEX type
// declared here — key-val-sub-type reaches a target only as cos-ct-derived-ok /
// cos-st-derived-ok clause 1's identity or through the candidate's {base type
// definition} chain, and an anonymous type has no name for either to reach it
// by. Clause 7.2 is the only entry THIS declaration admits, so a named-type
// fixture here would be an invalid schema whose rejection has nothing to do with
// the mapping under test.
func TestProduceTypeTableAnonymousDeclaredType(t *testing.T) {
	const inline = `<xs:complexType><xs:sequence/></xs:complexType>`
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e">`+inline+`
	  <xs:alternative test="@k='t'" type="xs:error"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present for a synthesized default over an anonymous declared type")
	}
	if got := altTypeName(t, tt.DefaultTypeDefinition()); got != "<inline>" {
		t.Errorf("{default type definition}.{type definition} = %s, want the declaration's own anonymous type", got)
	}
	tt, present = typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e">`+inline+`
	  <xs:alternative test="@k='t'" type="xs:error"/>
	  <xs:alternative type="xs:error"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present when the trailing <alternative> names the default")
	}
	if got := altTypeName(t, tt.DefaultTypeDefinition()); got != "error" {
		t.Errorf("{default type definition}.{type definition} = %s, want error", got)
	}
}

// An <element> whose own {type definition} is INHERITED from its substitution
// group head (§3.3.2.1 dcl.elt.common clause 3) passes that arm WHOLE into the
// synthesized {default type definition}, which is what makes the shape
// representable at all.
func TestProduceTypeTableSynthesizedDefaultInheritsHeadType(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="head"><xs:complexType><xs:sequence/></xs:complexType></xs:element>
	<xs:element name="e" substitutionGroup="head">
	  <xs:alternative test="@k='t'" type="xs:error"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present for a substitutionGroup=-typed declaration")
	}
	dflt, ok := tt.DefaultTypeDefinition().TypeDefinition().(xsd.SubstitutionGroupHeadTypeRef)
	if !ok {
		t.Fatalf("{default type definition}.{type definition} = %T, want a SubstitutionGroupHeadTypeRef", tt.DefaultTypeDefinition().TypeDefinition())
	}
	if dflt.Head != (xsd.QName{Local: "head"}) {
		t.Errorf("{default type definition} inherits from %v, want head", dflt.Head)
	}
}

// Two <alternative> children each owning an inline <complexType> get DISTINCT
// container tokens, so the local declarations nested in one do not report the
// other as their {scope}.{parent} — the two-mint split
// typeAlternativeOwnedComplexType exists for. Their {context} is the SAME
// element declaration, which is §3.4.2.1 dcl.ctd.common's answer for both.
func TestProduceTypeTableInlineAlternativesGetDistinctContainers(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='1'"><xs:complexType><xs:complexContent><xs:extension base="B"><xs:sequence><xs:element name="kid" type="xs:string"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:alternative>
	  <xs:alternative test="@k='2'"><xs:complexType><xs:complexContent><xs:extension base="B"><xs:sequence><xs:element name="kid" type="xs:string"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType></xs:alternative>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	alts := tt.Alternatives()
	if len(alts) != 2 {
		t.Fatalf("{alternatives} len = %d, want 2", len(alts))
	}
	firstCtx, firstParent := ownedContextAndKidParent(t, alts[0])
	secondCtx, secondParent := ownedContextAndKidParent(t, alts[1])
	if firstCtx != secondCtx {
		t.Error("the two owned types carry DIFFERENT {context}s, but §3.4.2.1 dcl.ctd.common gives both the enclosing element declaration")
	}
	if firstParent == secondParent {
		t.Error("the two owned types share one container token, so their nested local declarations are indistinguishable by {scope}.{parent}")
	}
	if firstParent == firstCtx {
		t.Error("the container token equals the {context}, but one element declaration owns several containers here and each needs its own mint")
	}
}

// ownedContextAndKidParent reports the {context} identity of the anonymous
// complex type an alternative owns, and the {scope}.{parent} identity its nested
// local element declaration reports.
func ownedContextAndKidParent(t *testing.T, alt xsd.TypeAlternative) (xsd.ComponentID, xsd.ComponentID) {
	t.Helper()
	inline, ok := alt.TypeDefinition().(xsd.InlineTypeDefinition)
	if !ok {
		t.Fatalf("{type definition} = %T, want an InlineTypeDefinition", alt.TypeDefinition())
	}
	ct, ok := inline.Definition.(xsd.ComplexType)
	if !ok {
		t.Fatalf("the owned definition is %T, want a ComplexType", inline.Definition)
	}
	context, present := ct.Context()
	if !present {
		t.Fatal("the owned definition has an ·absent· {context}")
	}
	content, ok := ct.ContentType().(xsd.ElementContent)
	if !ok {
		t.Fatalf("the owned definition's {content type} is %T, want element-only content", ct.ContentType())
	}
	group, ok := content.Particle.Term().(xsd.ResolvedTerm).Term.(xsd.ModelGroup)
	if !ok {
		t.Fatal("the owned definition's particle is not a model group")
	}
	for _, part := range group.Particles() {
		term, ok := part.Term().(xsd.ResolvedTerm)
		if !ok {
			continue
		}
		decl, ok := term.Term.(xsd.ElementDeclaration)
		if !ok {
			continue
		}
		parent, ok := decl.Scope().Parent()
		if !ok {
			t.Fatalf("the nested declaration %s has a global {scope}", decl.Name())
		}
		anon, ok := parent.(xsd.AnonymousComplexTypeScopeParent)
		if !ok {
			t.Fatalf("the nested declaration's {scope}.{parent} is %T, want an AnonymousComplexTypeScopeParent", parent)
		}
		return context.ID(), anon.Owner
	}
	t.Fatal("the owned definition declares no local element")
	return xsd.ComponentID{}, xsd.ComponentID{}
}

// ·xs:error· (§3.16.7.3) is present in every schema by definition (builtin.Seed
// prepends it), so an <alternative> naming it maps like any other named type and
// finalize resolves the reference instead of charging src-resolve clause 1.1.
func TestProduceTypeTableForXSError(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='t'" type="T"/>
	  <xs:alternative type="xs:error"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present where an <alternative> names xs:error")
	}
	dflt := tt.DefaultTypeDefinition().TypeDefinition()
	if want := (xsd.TypeDefinitionRef{Name: xsd.QName{Space: xsd.XMLSchemaNS, Local: "error"}}); dflt != want {
		t.Errorf("{default type definition}.{type definition} = %v, want %v", dflt, want)
	}
}

// The LOCAL <element> path maps the table on the same terms as the global one.
func TestProduceTypeTableOnLocalElement(t *testing.T) {
	s, err := produce(t, wrap("", typeTableTypes+`
	<xs:complexType name="ct">
	  <xs:sequence>
	    <xs:element name="local" type="B">
	      <xs:alternative test="@k='t'" type="T"/>
	    </xs:element>
	  </xs:sequence>
	</xs:complexType>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	tt, present := localElementTypeTable(t, s, xsd.QName{Local: "ct"}, "local")
	if !present {
		t.Fatal("local declaration's {type table} is ·absent·, want present")
	}
	if got := altTypeNames(t, tt.Alternatives()); len(got) != 1 || got[0] != "T" {
		t.Fatalf("{alternatives} = %v, want [T]", got)
	}
	if got := altTypeName(t, tt.DefaultTypeDefinition()); got != "B" {
		t.Errorf("{default type definition}.{type definition} = %s, want B", got)
	}
}

// localElementTypeTable returns the {type table} of the local element
// declaration named local in the content model of the complex type typeName.
func localElementTypeTable(t *testing.T, s *xsd.Schema, typeName xsd.QName, local string) (xsd.TypeTable, bool) {
	t.Helper()
	def, ok := s.Type(typeName)
	if !ok {
		t.Fatalf("type %s not found", typeName)
	}
	ct, ok := def.(xsd.ComplexType)
	if !ok {
		t.Fatalf("type %s is not a complex type", typeName)
	}
	content, ok := ct.ContentType().(xsd.ElementContent)
	if !ok {
		t.Fatalf("type %s has no element-only content", typeName)
	}
	group, ok := content.Particle.Term().(xsd.ResolvedTerm).Term.(xsd.ModelGroup)
	if !ok {
		t.Fatalf("type %s's {content type} particle is not a model group", typeName)
	}
	for _, part := range group.Particles() {
		term, ok := part.Term().(xsd.ResolvedTerm)
		if !ok {
			continue
		}
		decl, ok := term.Term.(xsd.ElementDeclaration)
		if !ok || decl.Name().Local != local {
			continue
		}
		return decl.TypeTable()
	}
	t.Fatalf("no local element %q in type %s", local, typeName)
	return xsd.TypeTable{}, false
}

// {test} is the §3.13.2 XPath Expression property record, so the <alternative>
// host element's own xpathDefaultNamespace and namespace bindings govern it —
// the same construction an <assert> gets, not a bespoke one.
func TestProduceTypeTableTestIsAnXPathExpressionRecord(t *testing.T) {
	tt, present := typeTableOf(t, wrap("urn:t", `
	<xs:complexType name="B"><xs:sequence/></xs:complexType>
	<xs:complexType name="T"><xs:complexContent><xs:extension base="tns:B"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType>
	<xs:element name="e" type="tns:B">
	  <xs:alternative test="@k='t'" type="tns:T" xpathDefaultNamespace="##targetNamespace" xmlns:p="urn:p"/>
	</xs:element>`), xsd.QName{Space: "urn:t", Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	test, hasTest := tt.Alternatives()[0].Test()
	if !hasTest {
		t.Fatal("{alternatives}[0].{test} is ·absent·, want present")
	}
	ns, nsPresent := test.DefaultNamespace()
	if !nsPresent || ns != "urn:t" {
		t.Errorf("{default namespace} = %q/%t, want urn:t from ##targetNamespace", ns, nsPresent)
	}
	var bound bool
	for _, b := range test.NamespaceBindings() {
		if b.Prefix() == "p" && b.Namespace() == "urn:p" {
			bound = true
		}
	}
	if !bound {
		t.Errorf("{namespace bindings} = %v, want the host element's p→urn:p binding", test.NamespaceBindings())
	}
}

// src-ta (§3.12.3) counts the two INLINE forms as present, so an <alternative>
// with exactly one type child is fine and only zero or two-or-more is charged.
//
// Since #1275 the two-or-more branch is reached by the type attribute paired with
// ONE inline child alone: checkS4SChildOrder walks the same children against
// s4sAlternative first, and a second inline child repeats the one position
// xs:altType gives them both. That shape is pinned where the walk charges it,
// beside the other s4s rows (produce_s4sorder_test.go).
func TestProduceSrcTA(t *testing.T) {
	cases := []struct {
		name string
		// declared is the <element>'s own type=, chosen so that the accepted
		// fixtures also satisfy e-props-correct clause 7 (every entry ·validly
		// substitutable· for it) and are rejected, if at all, by src-ta alone.
		declared   string
		alternates string
		wantRule   bool
	}{
		{name: "type attribute alone", declared: "B", alternates: `<xs:alternative test="@k='t'" type="T"/>`},
		{name: "complexType child alone", declared: "B", alternates: `<xs:alternative test="@k='t'">` + inlineExtensionOfB + `</xs:alternative>`},
		{name: "simpleType child alone", declared: "xs:string", alternates: `<xs:alternative test="@k='t'"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:alternative>`},
		{name: "no form at all", declared: "B", alternates: `<xs:alternative test="@k='t'"/>`, wantRule: true},
		{name: "type attribute and complexType child", declared: "B", alternates: `<xs:alternative test="@k='t'" type="T"><xs:complexType><xs:sequence/></xs:complexType></xs:alternative>`, wantRule: true},
		{name: "type attribute and simpleType child", declared: "B", alternates: `<xs:alternative test="@k='t'" type="T"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:alternative>`, wantRule: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", typeTableTypes+`
			<xs:element name="e" type="`+tc.declared+`">`+tc.alternates+`</xs:element>`))
			if !tc.wantRule {
				if err != nil {
					t.Fatalf("Produce: %v, want no src-ta charge", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Produce succeeded, want a src-ta charge")
			}
			assertRule(t, err, "src-ta")
		})
	}
}

// src-element clause 5 (§3.3.3) — "Every <alternative> element but the last has
// a test attribute; the last <alternative> element may have such an attribute" —
// is charged over the same children src-ta passes. An untested child anywhere but
// last is named by neither {alternatives} nor {default type definition}
// (§3.3.2.1), so leaving it uncharged ACCEPTS a document that maps it to nothing
// (#957). The last child is untested in the legal fixtures below, which is the
// {default type definition} form and stays accepted.
func TestProduceSrcElementClause5(t *testing.T) {
	cases := []struct {
		name       string
		alternates string
		wantRule   bool
	}{
		{name: "one untested alternative, first and last at once", alternates: `<xs:alternative type="T"/>`},
		{name: "trailing untested alternative", alternates: `<xs:alternative test="@k='t'" type="T"/><xs:alternative type="V"/>`},
		{name: "every alternative tested", alternates: `<xs:alternative test="@k='t'" type="T"/><xs:alternative test="@k='u'" type="U"/>`},
		{name: "untested first of two", alternates: `<xs:alternative type="T"/><xs:alternative test="@k='u'" type="U"/>`, wantRule: true},
		{name: "untested middle of three", alternates: `<xs:alternative test="@k='t'" type="T"/><xs:alternative type="U"/><xs:alternative type="V"/>`, wantRule: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", typeTableTypes+`
			<xs:element name="e" type="B">`+tc.alternates+`</xs:element>`))
			if !tc.wantRule {
				if err != nil {
					t.Fatalf("Produce: %v, want no src-element charge", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Produce succeeded, want a src-element charge")
			}
			assertRule(t, err, "src-element")
		})
	}
}

// A dangling alternative type name is charged src-resolve at finalize, which is
// only reachable once the table is actually built.
func TestProduceTypeTableAlternativeTypeResolves(t *testing.T) {
	_, err := produce(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='t'" type="Missing"/>
	</xs:element>`))
	if err == nil {
		t.Fatal("Produce succeeded, want src-resolve for an alternative naming no type")
	}
	assertRule(t, err, "src-resolve")
}

// An <alternative> on the INLINE arm has no QName of its own for src-resolve
// clause 1.1 to charge, but the anonymous type it owns has its own references and
// they are resolved like any other — resolveTypeTable reaches them by routing the
// slot through the shared descent's walkTypeDefinition rather than through a name
// lookup.
func TestProduceTypeTableInlineAlternativeBaseResolves(t *testing.T) {
	_, err := produce(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='i'"><xs:complexType><xs:complexContent><xs:extension base="Missing"><xs:sequence/></xs:extension></xs:complexContent></xs:complexType></xs:alternative>
	</xs:element>`))
	if err == nil {
		t.Fatal("Produce succeeded, want src-resolve for a dangling base= inside an <alternative>'s inline type")
	}
	assertRule(t, err, "src-resolve")
}

// e-props-correct clause 7 charges an INLINE-armed alternative like any other:
// the constraint quantifies over the {type definition} COMPONENT, which every arm
// of the slot supplies, so an owned type that does not derive from the
// declaration's own type is rejected rather than skipped.
func TestProduceTypeTableInlineAlternativeChargedClause7(t *testing.T) {
	_, err := produce(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@k='i'"><xs:complexType><xs:sequence/></xs:complexType></xs:alternative>
	</xs:element>`))
	if err == nil {
		t.Fatal("Produce succeeded, want e-props-correct clause 7 for an inline alternative type that does not derive from B")
	}
	assertRule(t, err, "e-props-correct")
}

// ta-props-correct clause 2 (§3.12.6) is charged at CONSTRUCTION for a test=
// carrying an XPath static error, over §3.13.6.2 xpath-valid clause 2's "does
// not produce any static error". It is a Schema Component Constraint, so the
// schema is rejected here rather than silently accepted with a {test} validate
// would later withhold — and the charge lands on the <alternative>, whose Loc a
// user needs to find it.
func TestProduceTypeTableStaticErrorInTestIsCharged(t *testing.T) {
	doc := wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@p:k='t'" type="T"/>
	  <xs:alternative type="V"/>
	</xs:element>`)
	_, err := produce(t, doc)
	assertRule(t, err, "ta-props-correct")
	if !strings.Contains(err.Error(), "err:XPST0081") {
		t.Errorf("error %v does not name the XPath static error it charges", err)
	}
	// The two verdicts are two LAYERS, not one message: ta-props-correct is this
	// producer's and err:XPST0081 is xpath's, so the delegated one is reachable
	// as the wrapped cause and a consumer reads its rule off that instead of
	// scraping the rendering (validate/doc.go's delegated-verdict norm, #978).
	inner := errors.Unwrap(err)
	if inner == nil {
		t.Fatalf("error %v wraps no cause; the XPath verdict must travel as one", err)
	}
	if rule, ok := xsderr.RuleOf(inner); !ok || rule != "err:XPST0081" {
		t.Errorf("wrapped cause %v carries rule %q (found %t), want err:XPST0081", inner, rule, ok)
	}
	loc, ok := xsderr.LocOf(err)
	if !ok {
		t.Fatalf("error %v carries no location", err)
	}
	if want := 1 + strings.Count(doc[:strings.Index(doc, "@p:k")], "\n"); loc.Line != want {
		t.Errorf("charged at line %d, want %d — the <alternative> carrying the test", loc.Line, want)
	}
}

// A test= that is not an XPath 2.0 expression at all is err:XPST0003
// (xpath20.md §2.2.3.1), a static error, so ta-props-correct clause 2 is charged
// over it exactly as over an unbound prefix — not withheld as a {test} the
// subset merely declines. The rows are the suite's s3_12si05 and s3_12si06
// expressions; the cause carries err:XPST0003 as its own rule.
func TestProduceTypeTableSyntaxErrorInTestIsCharged(t *testing.T) {
	for _, test := range []string{
		"@numberOfChildren &lt; 5 AND @numberOfChildren > 0",
		"(@type cast as xs1::double)='double'",
	} {
		_, err := produce(t, wrap("", typeTableTypes+`
		<xs:element name="e" type="B">
		  <xs:alternative test="`+test+`" type="T"/>
		  <xs:alternative type="V"/>
		</xs:element>`))
		assertRule(t, err, "ta-props-correct")
		inner := errors.Unwrap(err)
		if rule, ok := xsderr.RuleOf(inner); !ok || rule != "err:XPST0003" {
			t.Errorf("test=%q: wrapped cause %v carries rule %q (found %t), want err:XPST0003", test, inner, rule, ok)
		}
	}
}

// A CAST TAIL no longer declines the expression carrying it, so a {test} whose
// unbound prefix sits under one now parses to a complete [8] ta-Test and its
// err:XPST0081 reaches the charge instead of dying with the parse. This is the
// one place the charge and the cast evaluation meet, and it moved.
func TestProduceTypeTableStaticErrorUnderACastIsCharged(t *testing.T) {
	for _, test := range []string{
		"@p:k cast as xs:string = 't'",
		"xs:string(@p:k) = 't'",
	} {
		_, err := produce(t, wrap("", typeTableTypes+`
		<xs:element name="e" type="B">
		  <xs:alternative test="`+test+`" type="T"/>
		  <xs:alternative type="V"/>
		</xs:element>`))
		assertRule(t, err, "ta-props-correct")
		if err != nil && !strings.Contains(err.Error(), "err:XPST0081") {
			t.Errorf("test=%q: error %v does not name the XPath static error it charges", test, err)
		}
	}
}

// A WILDCARD NameTest reaches the charge for the same reason a QName one does,
// and it became reachable only when the wildcard stopped declining lexically
// (#859): `@p:*` is [37]'s one prefix-resolving spelling, so an unbound p is
// err:XPST0081 and the schema is rejected, where the same {test} was previously
// declined as unsupported and charged nowhere.
//
// The second document is the discriminating half: the two wildcard spellings
// that resolve NO prefix are complete ta-Test productions with nothing static
// wrong with them, so they must reach the {type table} rather than a charge.
func TestProduceTypeTableWildcardStaticErrorIsCharged(t *testing.T) {
	_, err := produce(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@p:* = 't'" type="T"/>
	  <xs:alternative type="V"/>
	</xs:element>`))
	assertRule(t, err, "ta-props-correct")
	if err != nil && !strings.Contains(err.Error(), "err:XPST0081") {
		t.Errorf("error %v does not name the XPath static error it charges", err)
	}
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@* = 't'" type="T"/>
	  <xs:alternative test="@*:kind = 't'" type="U"/>
	  <xs:alternative type="V"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	if got := altTypeNames(t, tt.Alternatives()); !slices.Equal(got, []string{"T", "U"}) {
		t.Fatalf("{alternatives} = %v, want [T U]", got)
	}
}

// A cast TARGET that is err:XPST0051 or err:XPST0080 is an xpath-valid clause 2
// failure, and ta-props-correct clause 2 is charged over it: xpath-valid clause
// 2.2.5 puts only the built-ins in scope, and §3.12.6's Note licenses declining
// only an expression "containing no static errors" (#894). An unbound prefix
// on the target itself, or on the operand before it, is err:XPST0081 — the
// first static error in expression order, which the target's own err:XPST0051
// or err:XPST0080 does not overwrite. Each row is its own document, because a
// schema is rejected at its first charge.
func TestProduceTypeTableCastTargetStaticErrorIsCharged(t *testing.T) {
	for _, tc := range []struct{ test, rule string }{
		{"@k cast as xs:Missing = 't'", "err:XPST0051"},
		{"@k cast as xs:anyAtomicType = 't'", "err:XPST0080"},
		{"@k cast as q:Missing = 't'", "err:XPST0081"},
		{"@p:k cast as xs:anyAtomicType = 't'", "err:XPST0081"},
	} {
		t.Run(tc.test, func(t *testing.T) {
			_, err := produce(t, wrap("", typeTableTypes+`
			<xs:element name="e" type="B">
			  <xs:alternative test="`+tc.test+`" type="T"/>
			  <xs:alternative type="V"/>
			</xs:element>`))
			assertRule(t, err, "ta-props-correct")
			if rule, ok := xsderr.RuleOf(errors.Unwrap(err)); !ok || string(rule) != tc.rule {
				t.Errorf("wrapped cause of %v carries rule %q (found %t), want %s", err, rule, ok, tc.rule)
			}
		})
	}
}

// A static error a {test} this engine declines carries is charged all the
// same, wherever in the text it sits: §3.12.6 clause 2's Note licenses
// declining only an expression "containing no static errors", so
// ta-props-correct clause 2 is charged over the declined text exactly as over
// a complete ta-Test (#2631) — before the construct the decline is at, past
// it, in a text the subset's tokenizer never reads (`..`), err:XPST0008 for
// `$value` (xpath-valid clause 2.2.6 leaves the in-scope variables empty), and
// err:XPST0051 for the AtomicType of an `instance of` or `treat as` and the
// SingleType of a `castable as`.
func TestProduceTypeTableStaticErrorBeforeADeclineIsCharged(t *testing.T) {
	for _, tc := range []struct{ test, rule string }{
		{"count(//p:x) &gt; 1", "err:XPST0081"},
		{"@k cast as xs:Missing = 't' and count(//x) &gt; 1", "err:XPST0051"},
		{"count(//x) &gt; 1 and @k cast as xs:Missing = 't'", "err:XPST0051"},
		{".. and @k cast as xs:Missing = 't'", "err:XPST0051"},
		{"$value = 't'", "err:XPST0008"},
		{"@k instance of xs:Missing", "err:XPST0051"},
		{"@k treat as xs:Missing = 't'", "err:XPST0051"},
		{"@k castable as xs:Missing", "err:XPST0051"},
	} {
		t.Run(tc.test, func(t *testing.T) {
			_, err := produce(t, wrap("", typeTableTypes+`
			<xs:element name="e" type="B">
			  <xs:alternative test="`+tc.test+`" type="T"/>
			  <xs:alternative type="V"/>
			</xs:element>`))
			assertRule(t, err, "ta-props-correct")
			if rule, ok := xsderr.RuleOf(errors.Unwrap(err)); !ok || string(rule) != tc.rule {
				t.Errorf("wrapped cause of %v carries rule %q (found %t), want %s", err, rule, ok, tc.rule)
			}
		})
	}
}

// The CONSTRUCTOR spelling names a function, not a cast target: err:XPST0051
// and err:XPST0080 are cast-target errors, and a name with no constructor
// signature is err:XPST0017 at most, which xpath-valid clause 2.2.8 leaves
// implementation-defined. So neither call is charged, and both are withheld
// at ·assessment· (#894). Each row is its own document, so a charge of the
// first does not hide the second.
func TestProduceTypeTableConstructorTargetIsNotCharged(t *testing.T) {
	for _, test := range []string{"string(@k) = 't'", "xs:anyAtomicType(@k) = 't'"} {
		t.Run(test, func(t *testing.T) {
			tt, present := typeTableOf(t, wrap("", typeTableTypes+`
			<xs:element name="e" type="B">
			  <xs:alternative test="`+test+`" type="T"/>
			  <xs:alternative type="V"/>
			</xs:element>`), xsd.QName{Local: "e"})
			if !present {
				t.Fatal("{type table} is ·absent·, want present")
			}
			if got := altTypeNames(t, tt.Alternatives()); !slices.Equal(got, []string{"T"}) {
				t.Fatalf("{alternatives} = %v, want [T]", got)
			}
		})
	}
}

// A test= this engine merely cannot EVALUATE is no fault: §3.12.6 clause 2's
// Note lets a processor decline an expression outside the required subset, and
// charging one would reject a conforming schema. The withhold that decline
// costs is validate's, at ·assessment· time, and is not this producer's
// business.
func TestProduceTypeTableUnsupportedTestIsNotCharged(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="count(//x) &gt; 1" type="T"/>
	  <xs:alternative type="V"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	if got := altTypeNames(t, tt.Alternatives()); !slices.Equal(got, []string{"T"}) {
		t.Fatalf("{alternatives} = %v, want [T]", got)
	}
}

// The reserved xml prefix is ALWAYS in scope (Namespaces in XML, and xmltree's
// InScopePrefixes with it), so a {test} naming @xml:lang resolves with no
// xmlns:xml declaration anywhere and must not be charged. Regressing this
// manufactures a false reject on ordinary schemas.
func TestProduceTypeTableReservedXMLPrefixInTestIsNotCharged(t *testing.T) {
	tt, present := typeTableOf(t, wrap("", typeTableTypes+`
	<xs:element name="e" type="B">
	  <xs:alternative test="@xml:lang = 'en'" type="T"/>
	  <xs:alternative type="V"/>
	</xs:element>`), xsd.QName{Local: "e"})
	if !present {
		t.Fatal("{type table} is ·absent·, want present")
	}
	if got := altTypeNames(t, tt.Alternatives()); !slices.Equal(got, []string{"T"}) {
		t.Fatalf("{alternatives} = %v, want [T]", got)
	}
}

// The ·in-scope schema definitions· xpath-valid clause 2.2.5 fixes for a {test}
// are the Built-in Simple Type Definitions ALONE, so what this schema declares
// never joins them — not even when the schema targets the XSD namespace itself,
// which no rule forbids. Both documents below declare xs:myAtomic and differ
// only in whether they declare it BEFORE or AFTER the <element> whose test casts
// to it, so a resolver reading the parser's build-once memo instead of the fixed
// seeded set answers differently for the two and charges ta-props-correct on one
// alone. Same schema components, same verdict, whatever the declaration order.
func TestProduceTypeTableStaticTypesAreOrderIndependent(t *testing.T) {
	const declaration = `<xs:simpleType name="myAtomic"><xs:restriction base="xs:string"/></xs:simpleType>`
	// The cast target is the schema's OWN type, which clause 2.2.5 leaves out of
	// scope, so it is err:XPST0051 — the first static error in expression order,
	// ahead of the unbound zz: prefix beside it (#894).
	const element = `<xs:element name="e" type="xs:string">
	  <xs:alternative test="@a cast as xs:myAtomic and @zz:b = '1'" type="xs:string"/>
	  <xs:alternative type="xs:string"/>
	</xs:element>`
	const head = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.w3.org/2001/XMLSchema">`
	for _, tc := range []struct {
		name string
		doc  string
	}{
		{name: "declared before the element", doc: head + declaration + element + `</xs:schema>`},
		{name: "declared after the element", doc: head + element + declaration + `</xs:schema>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, tc.doc)
			assertRule(t, err, "ta-props-correct")
			if rule, ok := xsderr.RuleOf(errors.Unwrap(err)); !ok || rule != "err:XPST0051" {
				t.Errorf("wrapped cause of %v carries rule %q (found %t), want err:XPST0051 — xs:myAtomic is this schema's own type and clause 2.2.5 does not put it in scope", err, rule, ok)
			}
		})
	}
}
