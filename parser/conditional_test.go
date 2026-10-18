package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// vcWrap wraps body in a <schema> with both the XSD and the versioning prefix
// bound, so a test body can write vc:minVersion without repeating the namespace.
// attrs goes on the <schema> element itself, which is how the root-stub case
// (§4.2.2's carve-out) is written.
func vcWrap(attrs, body string) string {
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"` +
		` xmlns:vc="http://www.w3.org/2007/XMLSchema-versioning" ` + attrs + `>` + body + `</xs:schema>`
}

// markedDeclaration writes one top-level <element name="e"> carrying marked,
// beside an unmarked sibling of another name so the document declares something
// whichever way the verdict goes.
func markedDeclaration(marked string) string {
	return vcWrap("", `<xs:element name="e" type="xs:string" `+marked+`/>`+
		`<xs:element name="unmarked" type="xs:int"/>`)
}

// declarationRetained produces doc and reports whether the declaration named e
// reached the schema — whether, that is, §4.2.2 retained it. The unmarked sibling
// is asserted too, so a fixture that produced nothing at all cannot read as an
// ignored element.
func declarationRetained(t *testing.T, doc string) bool {
	t.Helper()
	s, err := produce(t, doc)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Local: "unmarked"}); !ok {
		t.Fatalf("the unmarked sibling declaration is missing, so nothing here reads as a §4.2.2 verdict")
	}
	_, ok := s.Element(xsd.QName{Local: "e"})
	return ok
}

// TestConditionalInclusionVersionBand pins the band this processor applies,
// vc:minVersion ≤ V < vc:maxVersion with V = 1.1, one boundary at a time — and
// the comparison being over ·actual values· rather than lexicals, which is where
// "1.10" and "1.1" have to be one value.
//
// The row "maxVersion equal to V ignores" is the one that fails if the max arm
// compares with > rather than >=; no suite case catches that once
// VC/vc002/instance/vc002.n1.xml is banked fail.
func TestConditionalInclusionVersionBand(t *testing.T) {
	for _, tc := range []struct {
		name     string
		marked   string
		retained bool
	}{
		{"minVersion equal to V retains, the end being inclusive", `vc:minVersion="1.1"`, true},
		{"minVersion above V ignores", `vc:minVersion="1.2"`, false},
		{"minVersion below V retains", `vc:minVersion="1.0"`, true},
		{"a trailing zero is no different value", `vc:minVersion="1.10"`, true},
		{"a leading zero is no different value", `vc:minVersion="01.2"`, false},
		{"a signed value compares numerically", `vc:minVersion="+2"`, false},
		{"a negative minVersion retains", `vc:minVersion="-1.5"`, true},
		{"a fraction-only lexical is a legal decimal", `vc:minVersion=".9"`, true},
		{"an integral lexical is a legal decimal", `vc:minVersion="2"`, false},
		{"whitespace around the value is collapsed away", `vc:minVersion="  1.2  "`, false},
		{"an unrecognized vc: attribute is not one of the six", `vc:minversion="9.9"`, true},
		{"maxVersion equal to V ignores, the end being exclusive", `vc:maxVersion="1.1"`, false},
		{"maxVersion above V retains", `vc:maxVersion="1.2"`, true},
		{"maxVersion below V ignores", `vc:maxVersion="1.0"`, false},
		{"a trailing zero on maxVersion is no different value", `vc:maxVersion="1.10"`, false},
		{"a leading zero on maxVersion is no different value", `vc:maxVersion="01.2"`, true},
		{"a fraction-only maxVersion compares numerically", `vc:maxVersion=".9"`, false},
		{"a signed maxVersion compares numerically", `vc:maxVersion="+2"`, true},
		{"a negative maxVersion ignores", `vc:maxVersion="-1.5"`, false},
		{"whitespace around maxVersion is collapsed away", `vc:maxVersion="  1.2  "`, true},
		{"an unrecognized vc:maxversion is not one of the six", `vc:maxversion="0.1"`, true},
		{"a band containing V retains", `vc:minVersion="1.0" vc:maxVersion="1.2"`, true},
		{"a band with both ends at V is empty", `vc:minVersion="1.1" vc:maxVersion="1.1"`, false},
		{"a band with maxVersion below minVersion is empty", `vc:minVersion="1.0" vc:maxVersion="0.9"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := declarationRetained(t, markedDeclaration(tc.marked)); got != tc.retained {
				t.Fatalf("declaration retained = %v, want %v", got, tc.retained)
			}
		})
	}
}

// TestConditionalInclusionNoNamespaceNameIsNotVersioning pins the other half of
// the namespace discrimination the band table above ends on: an attribute whose
// LOCAL name is one of §4.2.2's six but which stands in NO namespace is not a
// versioning attribute, so the declaration carrying it is never pruned.
//
// The evidence is a REJECTION and is the same evidence retention always was: a
// pruned element reaches no producer and no s4s guard at all, so the
// undeclared-attribute fault can only be charged over an element §4.2.2 kept.
// This case cannot be written as a retained-declaration row any more, because an
// unprefixed minVersion is admitted by no production of <element> and the
// document is now rejected rather than produced (#1369).
func TestConditionalInclusionNoNamespaceNameIsNotVersioning(t *testing.T) {
	_, err := produce(t, markedDeclaration(`minVersion="9.9"`))
	if err == nil {
		t.Fatal("the declaration was produced, so nothing here shows whether §4.2.2 pruned it")
	}
	if !strings.Contains(err.Error(), "carries a minVersion attribute") {
		t.Fatalf("error = %v, want the s4s fault charged over the retained <element>", err)
	}
}

// TestConditionalInclusionMaxVersionRetiresOneOfTwoSiblings is the shape
// §4.2.2's own example is written in: two declarations of ONE expanded name, one
// for each side of the vc:maxVersion boundary. Exactly the vc:minVersion="1.1"
// one survives, so sch-props-correct clause 2 sees no duplicate.
func TestConditionalInclusionMaxVersionRetiresOneOfTwoSiblings(t *testing.T) {
	s, err := produce(t, vcWrap("",
		`<xs:element name="e" type="xs:string" vc:maxVersion="1.1"/>`+
			`<xs:element name="e" type="xs:int" vc:minVersion="1.1"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatalf("element e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got.Local != "int" {
		t.Fatalf("surviving element type = %s, want xs:int — the declaration inside the retained band", got)
	}
}

// TestConditionalInclusionReferenceToPrunedDeclaration pins what pruning costs
// a retained construct: a reference to a declaration §4.2.2 removed resolves to
// nothing in S2 and is treated as any unresolved QName is — an <element ref> is
// charged src-resolve, while a substitutionGroup= naming it is kept as an
// ·absent· {substitution group affiliations} member and the schema is accepted
// (§5.3; xsd/resolve.go's resolveElementDecl).
func TestConditionalInclusionReferenceToPrunedDeclaration(t *testing.T) {
	t.Run("element ref", func(t *testing.T) {
		_, err := produce(t, vcWrap("",
			`<xs:element name="gone" type="xs:string" vc:maxVersion="1.1"/>`+
				`<xs:element name="r"><xs:complexType><xs:sequence>`+
				`<xs:element ref="gone"/>`+
				`</xs:sequence></xs:complexType></xs:element>`))
		var xerr *xsderr.Error
		if !errors.As(err, &xerr) {
			t.Fatalf("Produce error = %v, want an *xsderr.Error", err)
		}
		if xerr.Rule != "src-resolve" {
			t.Fatalf("rule = %s, want src-resolve: %v", xerr.Rule, xerr)
		}
	})
	t.Run("substitutionGroup", func(t *testing.T) {
		s, err := produce(t, vcWrap("",
			`<xs:element name="head" type="xs:string" vc:maxVersion="1.1"/>`+
				`<xs:element name="m" type="xs:string" substitutionGroup="head"/>`))
		if err != nil {
			t.Fatalf("Produce: %v, want the dangling affiliation kept as ·absent· (§5.3)", err)
		}
		if _, ok := s.Element(xsd.QName{Local: "head"}); ok {
			t.Fatalf("head reached the schema, but vc:maxVersion ignores it")
		}
		m, ok := s.Element(xsd.QName{Local: "m"})
		if !ok {
			t.Fatalf("element m not found")
		}
		names := m.SubstitutionGroupAffiliationNames()
		if len(names) != 1 || names[0] != (xsd.QName{Local: "head"}) {
			t.Fatalf("m's affiliation names = %v, want [head] kept as an ·absent· member", names)
		}
	})
}

// TestConditionalInclusionAvailability pins §4.2.2's four availability clauses
// over the types and facets this processor knows, including the inversion its own
// note calls out: an empty vc:typeAvailable list ignores nothing, an empty
// vc:typeUnavailable list ignores the element.
func TestConditionalInclusionAvailability(t *testing.T) {
	for _, tc := range []struct {
		name     string
		marked   string
		retained bool
	}{
		{"typeAvailable naming a builtin retains", `vc:typeAvailable="xs:integer"`, true},
		{"typeAvailable naming xs:error retains", `vc:typeAvailable="xs:error"`, true},
		{"typeAvailable naming xs:anySimpleType retains", `vc:typeAvailable="xs:anySimpleType"`, true},
		{"typeAvailable naming xs:anyType retains", `vc:typeAvailable="xs:anyType"`, true},
		{"typeAvailable with one unknown item ignores", `vc:typeAvailable="xs:integer xs:bananaSkin"`, false},
		{"typeAvailable naming a type outside the XSD namespace ignores", `vc:typeAvailable="vc:myType"`, false},
		{"typeAvailable empty retains", `vc:typeAvailable=""`, true},
		{"typeUnavailable naming a builtin ignores", `vc:typeUnavailable="xs:integer"`, false},
		{"typeUnavailable with one unknown item retains", `vc:typeUnavailable="xs:integer vc:myType"`, true},
		{"typeUnavailable empty ignores", `vc:typeUnavailable=""`, false},
		{"facetAvailable naming a facet element retains", `vc:facetAvailable="xs:pattern"`, true},
		{"facetAvailable naming the assertion element retains", `vc:facetAvailable="xs:assertion"`, true},
		{"facetAvailable naming the enumeration element retains", `vc:facetAvailable="xs:enumeration"`, true},
		{"facetAvailable naming the facet's plural spelling ignores", `vc:facetAvailable="xs:assertions"`, false},
		{"facetAvailable with one unknown item ignores", `vc:facetAvailable="xs:pattern xs:bananaSkin"`, false},
		{"facetAvailable empty retains", `vc:facetAvailable=""`, true},
		{"facetUnavailable naming a facet element ignores", `vc:facetUnavailable="xs:minLength"`, false},
		{"facetUnavailable with one unknown item retains", `vc:facetUnavailable="xs:minLength xs:bananaSkin"`, true},
		{"facetUnavailable empty ignores", `vc:facetUnavailable=""`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := declarationRetained(t, markedDeclaration(tc.marked)); got != tc.retained {
				t.Fatalf("declaration retained = %v, want %v", got, tc.retained)
			}
		})
	}
}

// TestConditionalInclusionRetiresOneOfTwoSiblings is the shape the mechanism
// exists for and the shape the reported bug had: two declarations of ONE expanded
// name, the first marked for a version this processor is not. Exactly one reaches
// S2, so sch-props-correct clause 2 sees no duplicate — and the survivor is the
// unmarked one, which a fixture that merely parsed would not show.
func TestConditionalInclusionRetiresOneOfTwoSiblings(t *testing.T) {
	s, err := produce(t, vcWrap("",
		`<xs:element name="e" type="xs:string" vc:minVersion="3.2"/>`+
			`<xs:element name="e" type="xs:int" vc:minVersion="1.0"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatalf("element e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got.Local != "int" {
		t.Fatalf("surviving element type = %s, want xs:int — the declaration inside the retained band", got)
	}
}

// TestConditionalInclusionIgnoresWholeSubtree pins the scope of removal: "the
// element on which the attribute appears is to be ignored, along with all its
// attributes and descendants". A DESCENDANT of the ignored element is what makes
// the cascade observable — a named <xs:key> registers a schema-level
// {identity-constraint definitions} member (§3.17.1) from inside the declaration,
// so it is visible in the finished schema wherever the subtree survived.
func TestConditionalInclusionIgnoresWholeSubtree(t *testing.T) {
	s, err := produce(t, vcWrap("", `<xs:element name="gone" vc:minVersion="3.2">`+
		`<xs:complexType><xs:sequence><xs:element name="c" type="xs:string"/></xs:sequence></xs:complexType>`+
		`<xs:key name="k"><xs:selector xpath="c"/><xs:field xpath="."/></xs:key>`+
		`</xs:element>`+
		`<xs:element name="kept" type="xs:string"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Local: "gone"}); ok {
		t.Fatalf("element gone was declared, but §4.2.2 removes it with its whole subtree")
	}
	for _, ic := range s.IdentityConstraints() {
		if ic.Name().Local == "k" {
			t.Fatalf("identity constraint k reached the schema from inside an ignored element's subtree")
		}
	}
	if _, ok := s.Element(xsd.QName{Local: "kept"}); !ok {
		t.Fatalf("element kept not found")
	}
}

// TestConditionalInclusionRootStub pins §4.2.2's carve-out for an ignored
// <schema> element: S2 keeps the root with an empty [children], so the document
// declares nothing and follows none of the directives it was written with. No
// suite fixture reaches the carve-out through vc:minVersion — vc006.xsd reaches
// it through vc:maxVersion — so the min row is the only place the min arm's stub
// is exercised. What the stub RETAINS is
// TestConditionalIncludeRootStubAttributes's, one layer down.
func TestConditionalInclusionRootStub(t *testing.T) {
	for _, marked := range []string{`vc:minVersion="3.2"`, `vc:maxVersion="0.9"`} {
		t.Run(marked, func(t *testing.T) {
			s, err := produce(t, vcWrap(`targetNamespace="urn:a" `+marked,
				`<xs:include schemaLocation="nowhere.xsd"/>`+
					`<xs:element name="e" type="xs:string"/>`))
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "e"}); ok {
				t.Fatalf("element {urn:a}e was declared, but its <schema> is ignored and S2's [children] is empty")
			}
			if _, ok := s.Element(xsd.QName{Local: "e"}); ok {
				t.Fatalf("element e was declared in no namespace, but S2's [children] is empty")
			}
		})
	}
}

// TestConditionalInclusionParseOverAStubRoot pins the contract parser/doc.go
// states for Parse: a root document whose own <schema> vc:maxVersion ignores
// assembles as the stub — the call succeeds, follows none of its directives and
// yields none of its components.
func TestConditionalInclusionParseOverAStubRoot(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": vcWrap(`targetNamespace="urn:a" vc:maxVersion="1.1"`,
			`<xs:include schemaLocation="nowhere.xsd"/>`+
				`<xs:element name="e" type="xs:int"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v, but the stub root has no children to fail on", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "e"}); ok {
		t.Fatalf("element {urn:a}e was declared, but its <schema> is ignored and S2's [children] is empty")
	}
}

// TestConditionalInclusionMaxVersionInComposedDocument pins the max arm on the
// documents a closure fetches: an <include> it ignores is never followed, and an
// included document whose own <schema> it ignores contributes the stub and
// nothing else.
func TestConditionalInclusionMaxVersionInComposedDocument(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": vcWrap(`targetNamespace="urn:a"`,
			`<xs:include schemaLocation="nowhere.xsd" vc:maxVersion="1.1"/>`+
				`<xs:include schemaLocation="stub.xsd"/>`+
				`<xs:element name="e" type="xs:int"/>`),
		"stub.xsd": vcWrap(`targetNamespace="urn:a" vc:maxVersion="1.1"`,
			`<xs:element name="e" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "e"})
	if !ok {
		t.Fatalf("element {urn:a}e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got.Local != "int" {
		t.Fatalf("surviving element type = %s, want xs:int — the root's own declaration", got)
	}
}

// TestConditionalInclusionInComposedDocument is the composed half of the
// mechanism, which no single-document test reaches: pre-processing runs on EVERY
// document of the assembly as it is read, before its own directives are followed
// (§4.2.2's "always performed first"). Four documents in one closure, each
// carrying a different half of it, all declaring the same expanded name.
func TestConditionalInclusionInComposedDocument(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		// The root declares e itself and includes documents that must contribute
		// nothing.
		"main.xsd": vcWrap(`targetNamespace="urn:a"`,
			`<xs:include schemaLocation="stub.xsd"/>`+
				`<xs:include schemaLocation="marked.xsd"/>`+
				`<xs:include schemaLocation="third.xsd" vc:minVersion="3.2"/>`+
				`<xs:element name="e" type="xs:int"/>`),
		// Its own <schema> is ignored, so it neither declares e nor follows the
		// <include> that would bring a further declaration of it in.
		"stub.xsd": vcWrap(`targetNamespace="urn:a" vc:minVersion="3.2"`,
			`<xs:include schemaLocation="third.xsd"/>`+
				`<xs:element name="e" type="xs:string"/>`),
		// A declaration marked for a version this processor is not.
		"marked.xsd": vcWrap(`targetNamespace="urn:a"`,
			`<xs:element name="e" type="xs:string" vc:minVersion="2.0"/>`),
		"third.xsd": vcWrap(`targetNamespace="urn:a"`,
			`<xs:element name="e" type="xs:boolean"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "e"})
	if !ok {
		t.Fatalf("element {urn:a}e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got.Local != "int" {
		t.Fatalf("surviving element type = %s, want xs:int — the root's own declaration", got)
	}
}

// TestConditionalInclusionIgnoredRedefineIsNotFollowed pins the directive half of
// the composed case from the other side: a <redefine> the transform removes is
// not a <redefine> of S2 at all, so neither its own children nor the document it
// names contributes anything — and src-redefine clause 1, which would reject this
// unresolvable location outright, is never charged.
func TestConditionalInclusionIgnoredRedefineIsNotFollowed(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": vcWrap(`targetNamespace="urn:a"`,
			`<xs:redefine schemaLocation="nowhere.xsd" vc:typeUnavailable="xs:integer">`+
				`<xs:simpleType name="s"><xs:restriction base="xs:string"/></xs:simpleType>`+
				`</xs:redefine>`+
				`<xs:element name="e" type="xs:int"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := s.Type(xsd.QName{Space: "urn:a", Local: "s"}); ok {
		t.Fatalf("the redefining <simpleType> contributed a type, but its <redefine> is absent from S2")
	}
}

// TestConditionalInclusionSrcCIP pins src-cip: an ill-formed value is a
// schema-document error, never a silent prune or a silent keep. Each case would
// otherwise be a legal-looking marking that quietly decides which declaration
// survives. An ill-formed vc:minVersion is charged although vc:maxVersion would
// have ignored the element, since §4.2.2's conditions are read in the order it
// states them and the first is unreadable.
func TestConditionalInclusionSrcCIP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marked string
		want   string
	}{
		{"minVersion outside xs:decimal", `vc:minVersion="1.1.3"`, "xs:decimal"},
		{"minVersion with a trailing letter", `vc:minVersion="10g"`, "xs:decimal"},
		{"minVersion empty", `vc:minVersion=""`, "xs:decimal"},
		{"minVersion in exponent notation", `vc:minVersion="1e1"`, "xs:decimal"},
		{"minVersion a bare sign", `vc:minVersion="-"`, "xs:decimal"},
		{"minVersion a bare point", `vc:minVersion="."`, "xs:decimal"},
		{"maxVersion outside xs:decimal", `vc:maxVersion="1.1.3"`, "xs:decimal"},
		{"maxVersion empty", `vc:maxVersion=""`, "xs:decimal"},
		{"maxVersion in exponent notation", `vc:maxVersion="1e1"`, "xs:decimal"},
		{"minVersion ill-formed beside a pruning maxVersion", `vc:minVersion="x" vc:maxVersion="1.0"`, "vc:minVersion"},
		{"maxVersion ill-formed beside a retaining minVersion", `vc:minVersion="1.0" vc:maxVersion="x"`, "vc:maxVersion"},
		{"minVersion ill-formed after a pruning maxVersion in the tag", `vc:maxVersion="1.0" vc:minVersion="x"`, "vc:minVersion"},
		{"typeAvailable item is no NCName", `vc:typeAvailable="xs:integer 23"`, "xs:QName"},
		{"typeAvailable item has an empty local part", `vc:typeAvailable="xs:"`, "xs:QName"},
		{"typeUnavailable item has an unbound prefix", `vc:typeUnavailable="vx:t"`, "in-scope namespace"},
		{"facetAvailable item is no NCName", `vc:facetAvailable="-nope"`, "xs:QName"},
		{"facetUnavailable item has two colons", `vc:facetUnavailable="a:b:c"`, "xs:QName"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, markedDeclaration(tc.marked))
			var xerr *xsderr.Error
			if !errors.As(err, &xerr) {
				t.Fatalf("Produce error = %v, want an *xsderr.Error", err)
			}
			if xerr.Rule != "src-cip" {
				t.Fatalf("rule = %s, want src-cip", xerr.Rule)
			}
			if !strings.Contains(xerr.Error(), tc.want) {
				t.Fatalf("message %q does not mention %q", xerr.Error(), tc.want)
			}
			if xerr.Loc.Line == 0 || xerr.Loc.Col == 0 || xerr.Loc.URI == "" {
				t.Fatalf("src-cip location = %s, want file:line:column", xerr.Loc)
			}
		})
	}
}

// TestConditionalInclusionIgnoredSubtreeEscapesSrcCIP pins the ordering §4.2.1
// states — Schema Representation Constraints are "enforced after, not before, the
// ·conditional-inclusion pre-processing·" — over src-cip itself: an ill-formed
// vc: value inside a subtree the transform removes is not in S2 and is charged
// nothing.
//
// The same holds on the ignored element itself: a condition that fires ends the
// scan, so an ill-formed availability value LATER in §4.2.2's order is not read.
func TestConditionalInclusionIgnoredSubtreeEscapesSrcCIP(t *testing.T) {
	for _, gone := range []string{`vc:minVersion="3.2"`, `vc:maxVersion="1.1"`} {
		t.Run(gone, func(t *testing.T) {
			if _, err := produce(t, vcWrap("", `<xs:element name="gone" `+gone+`>`+
				`<xs:complexType><xs:sequence vc:minVersion="not-a-decimal"/></xs:complexType>`+
				`</xs:element>`+
				`<xs:element name="kept" type="xs:string"/>`)); err != nil {
				t.Fatalf("Produce: %v, but the ill-formed value sits inside a subtree S2 does not contain", err)
			}
		})
	}
	for _, later := range []string{
		`vc:typeAvailable="23"`, `vc:typeUnavailable="23"`,
		`vc:facetAvailable="23"`, `vc:facetUnavailable="23"`,
	} {
		t.Run("maxVersion before "+later, func(t *testing.T) {
			if declarationRetained(t, markedDeclaration(`vc:maxVersion="1.1" `+later)) {
				t.Fatalf("the declaration was retained, but vc:maxVersion ignores it")
			}
		})
	}
	// §4.2.2's order, not the start tag's: written first, the ill-formed value
	// still comes after the version attribute that ignores the element.
	for _, gone := range []string{`vc:minVersion="2.0"`, `vc:maxVersion="1.1"`} {
		t.Run(`vc:typeAvailable="23" before `+gone+" in the tag", func(t *testing.T) {
			if declarationRetained(t, markedDeclaration(`vc:typeAvailable="23" `+gone)) {
				t.Fatalf("the declaration was retained, but %s ignores it", gone)
			}
		})
	}
}

// TestConditionalInclusionLeavesAnUnmarkedDocumentAlone pins §4.2.2's own no-op
// note, "if S1 contains no elements or attributes to be ignored, then S1 and S2
// are identical", at the level a consumer can see: a document with no versioning
// attribute keeps every child, mixed content included.
func TestConditionalInclusionLeavesAnUnmarkedDocumentAlone(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` +
		`<xs:annotation><xs:documentation>kept verbatim</xs:documentation></xs:annotation>` +
		`<xs:element name="e" type="xs:string"/></xs:schema>`
	s, err := produce(t, doc)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Local: "e"}); !ok {
		t.Fatalf("element e not found")
	}
}

// TestConditionalQNamesSplitOnXMLSpace pins conditionalQNames' split of the
// availability lists on XML white space alone (cvc-datatype-valid, Datatypes
// §4.1.4 clause 2.2; xml.md [3] S). Two QNames joined by U+00A0 or U+2028 are
// one item with two colons, which src-cip charges. U+1680 is an NCName
// character, so xs:int&#x1680;string is ONE QName, {XSD}int&#x1680;string,
// naming no available type, and the declaration is ignored; split at the
// U+1680 it would read as xs:int and string — xs:string under the default
// namespace bound on the element — both available, and be retained.
func TestConditionalQNamesSplitOnXMLSpace(t *testing.T) {
	for _, sp := range []string{"&#xA0;", "&#x2028;"} {
		t.Run("two QNames joined by "+sp+" are one item, no QName", func(t *testing.T) {
			_, err := produce(t, markedDeclaration(`vc:typeAvailable="xs:string`+sp+`xs:int"`))
			var xerr *xsderr.Error
			if !errors.As(err, &xerr) {
				t.Fatalf("Produce error = %v, want an *xsderr.Error", err)
			}
			if xerr.Rule != "src-cip" || !strings.Contains(xerr.Error(), "xs:QName") {
				t.Fatalf("Produce error = %v, want src-cip naming xs:QName", xerr)
			}
		})
	}
	t.Run("a U+1680 inside a name is one item", func(t *testing.T) {
		marked := `xmlns="http://www.w3.org/2001/XMLSchema" vc:typeAvailable="xs:int&#x1680;string"`
		if declarationRetained(t, markedDeclaration(marked)) {
			t.Errorf("the declaration was retained, so the one item {XSD}int\\u1680string was read as two available types")
		}
	})
}
