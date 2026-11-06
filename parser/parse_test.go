package parser_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// parseMap runs Parse over an in-memory document set, resolving locations by
// map key.
func parseMap(t *testing.T, root string, docs map[string]string) (*xsd.Schema, error) {
	t.Helper()
	return parser.Parse(root, parser.WithResolver(loader.Map(docs)))
}

// mustType fails unless the schema holds a type definition with the given
// expanded name.
func mustType(t *testing.T, s *xsd.Schema, name xsd.QName) {
	t.Helper()
	if _, ok := s.Type(name); !ok {
		t.Fatalf("type %s not found in assembled schema", name)
	}
}

// TestParseSingleDocument checks the degenerate assembly: a root with no
// <include> is schema(D) = immed(D) (§4.2.1).
func TestParseSingleDocument(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:element name="root" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"}); !ok {
		t.Fatalf("element {urn:a}root not found")
	}
}

// TestParseIncludeSameNamespace covers src-include clause 2.1 (c-normi): the
// included document's components join the including schema (clause 3.1.2,
// c-incl-incl), and a reference in the includer resolves to a definition
// contributed by the includee.
func TestParseIncludeSameNamespace(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:element name="root" type="tns:code"/>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction>`+
			`</xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"})
	if !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: "urn:a", Local: "code"}) {
		t.Fatalf("root type = %s, want {urn:a}code", got)
	}
}

// TestParseIncludeCrossDocumentBase exercises the ASSEMBLY-WIDE pre-scan, which
// runs for every document before any document is produced: a base= is discharged
// eagerly at production time (xsd.NewSimpleType needs a live base pointer), so
// each direction of a cross-document base chain needs the other document already
// registered (§4.2.3 c-incl-incl). The forward direction — the INCLUDING
// document, produced first, deriving from a type the INCLUDED one defines — is
// the one a per-document pre-scan cannot serve.
func TestParseIncludeCrossDocumentBase(t *testing.T) {
	restriction := func(name, base string) string {
		return `<xs:simpleType name="` + name + `">` +
			`<xs:restriction base="` + base + `"><xs:minLength value="2"/></xs:restriction>` +
			`</xs:simpleType>`
	}
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			restriction("fromLib", "tns:libBase")+
			`<xs:simpleType name="mainBase">`+
			`<xs:restriction base="xs:string"><xs:maxLength value="8"/></xs:restriction>`+
			`</xs:simpleType>`),
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="libBase">`+
			`<xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction>`+
			`</xs:simpleType>`+
			restriction("fromMain", "tns:mainBase")),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "fromLib"})
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "fromMain"})
}

// TestParseChameleonCoercion covers src-include clause 2.3 (c-chami) and the
// §F.1 transformation's BOTH tasks: (a) the included no-namespace document's
// components are minted in the including namespace, and (b) its unqualified
// QName references — here an intra-document sibling reference — are coerced to
// that same namespace rather than left ·absent·.
func TestParseChameleonCoercion(t *testing.T) {
	chameleon := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` +
		`<xs:simpleType name="code">` +
		`<xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction>` +
		`</xs:simpleType>` +
		`<xs:element name="local" type="code"/>` +
		`</xs:schema>`
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="cham.xsd"/>`+
			`<xs:element name="root" type="tns:code"/>`),
		"cham.xsd": chameleon,
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Task (a): the coerced component carries the including namespace, and the
	// no-namespace name it would otherwise have carried is absent.
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
	if _, ok := s.Type(xsd.QName{Local: "code"}); ok {
		t.Fatalf("type {}code exists, but §F.1 task (a) coerces it to {urn:a}code")
	}
	// Task (b): the chameleon document's own unqualified type= reference resolves
	// to the coerced name, not to the absent namespace.
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "local"})
	if !ok {
		t.Fatalf("element {urn:a}local not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: "urn:a", Local: "code"}) {
		t.Fatalf("chameleon type reference = %s, want {urn:a}code", got)
	}
}

// TestParseChameleonTransitive covers §4.2.3's recursion note: A includes B and
// B includes C, only A has a targetNamespace, so C's components are coerced too.
func TestParseChameleonTransitive(t *testing.T) {
	bare := func(body string) string {
		return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` + body + `</xs:schema>`
	}
	s, err := parseMap(t, "a.xsd", map[string]string{
		"a.xsd": wrap("urn:a", `<xs:include schemaLocation="b.xsd"/>`),
		"b.xsd": bare(`<xs:include schemaLocation="c.xsd"/><xs:element name="inB" type="deep"/>`),
		"c.xsd": bare(`<xs:simpleType name="deep">` +
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "deep"})
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "inB"})
	if !ok {
		t.Fatalf("element {urn:a}inB not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: "urn:a", Local: "deep"}) {
		t.Fatalf("cross-document chameleon reference = %s, want {urn:a}deep", got)
	}
}

// assembledSimpleType returns the *xsd.SimpleType an assembled schema holds
// under name, failing the test when there is none or it is not a simple type.
func assembledSimpleType(t *testing.T, s *xsd.Schema, name xsd.QName) *xsd.SimpleType {
	t.Helper()
	td, ok := s.Type(name)
	if !ok {
		t.Fatalf("type %s not found in assembled schema", name)
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("type %s = %T, want *xsd.SimpleType", name, td)
	}
	return st
}

// TestParseSimpleTypeBaseBuiltUnderItsOwnProducer is the simple-type twin of
// TestParseExtensionBaseBuiltUnderItsOwnProducer: a <simpleType> reached through
// a base= in ANOTHER document must be mapped by the producer of the document that
// DECLARES it, never by the referring one.
//
// base.xsd is a chameleon (§4.2.3 clause 2.3), so §F.1 task (b) coerces ITS OWN
// unqualified base="Root" to urn:x. That coercion belongs to the document holding
// the attribute — §3.16.2.1 takes {base type definition} from "the actual value
// of the base attribute", and src-resolve (§3.17.6.2) clause 4.1.1 scopes its
// absent-namespace default to "the schema document containing the QName" — while
// c-incl-incl only makes the component VISIBLE to the referring document, never
// transfers resolution authority to it. Built under the referring producer,
// unqualifiedRefNS would leave that reference ·absent· and the parse would falsely
// fail src-resolve on {}Root.
//
// Both document orders are forced, because only one of them takes the on-demand
// path: production follows discovery order (the root, then depth-first), so
// "referrer first" is the root doing the referring, and "declarer first" is the
// root including base.xsd ahead of a sibling document that refers to it.
func TestParseSimpleTypeBaseBuiltUnderItsOwnProducer(t *testing.T) {
	const xs = `xmlns:xs="http://www.w3.org/2001/XMLSchema"`
	// Declares Base by restricting the UNQUALIFIED sibling reference Root, which
	// §F.1 task (b) coerces to {urn:x}Root.
	const chameleon = `<xs:schema ` + xs + `>` +
		`<xs:simpleType name="Base"><xs:restriction base="Root">` +
		`<xs:maxLength value="6"/></xs:restriction></xs:simpleType></xs:schema>`
	const rootDecl = `<xs:simpleType name="Root"><xs:restriction base="xs:string">` +
		`<xs:maxLength value="8"/></xs:restriction></xs:simpleType>`
	const refDecl = `<xs:simpleType name="D"><xs:restriction base="tns:Base">` +
		`<xs:maxLength value="4"/></xs:restriction></xs:simpleType>`

	cases := []struct {
		name string
		docs map[string]string
	}{{
		// main is produced FIRST and is itself the referrer, so Base is unbuilt
		// when D's base="tns:Base" is resolved: the on-demand build under a
		// FOREIGN producer is the path under test.
		name: "referrer produced before declarer",
		docs: map[string]string{
			"main.xsd": wrap("urn:x", `<xs:include schemaLocation="base.xsd"/>`+rootDecl+refDecl),
			"base.xsd": chameleon,
		},
	}, {
		// base.xsd is discovered — and so produced — before the sibling that refers
		// to it, so Base is built by its own producer at its own document-order
		// position and D's base= is a pure memo hit.
		name: "declarer produced before referrer",
		docs: map[string]string{
			"main.xsd": wrap("urn:x", `<xs:include schemaLocation="base.xsd"/>`+
				`<xs:include schemaLocation="ref.xsd"/>`+rootDecl),
			"base.xsd": chameleon,
			"ref.xsd":  wrap("urn:x", refDecl),
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseMap(t, "main.xsd", tc.docs)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			base := assembledSimpleType(t, s, xsd.QName{Space: "urn:x", Local: "Base"})
			if got := mustBase(t, s, base).Name(); got != (xsd.QName{Space: "urn:x", Local: "Root"}) {
				t.Fatalf("{urn:x}Base's {base type definition} = %s, want {urn:x}Root — the base= was resolved under the wrong document", got)
			}
			// Component identity (xsd/typedefinition.go): whichever producer asked
			// for the build, the assembly holds ONE component per name.
			if root := assembledSimpleType(t, s, xsd.QName{Space: "urn:x", Local: "Root"}); mustBase(t, s, base) != root {
				t.Errorf("{urn:x}Base's base is a rebuilt twin, not {type definitions}' own {urn:x}Root")
			}
			if d := assembledSimpleType(t, s, xsd.QName{Space: "urn:x", Local: "D"}); mustBase(t, s, d) != base {
				t.Errorf("{urn:x}D's base is a rebuilt twin, not {type definitions}' own {urn:x}Base")
			}
		})
	}
}

// TestParseAttributeGroupFoldedUnderItsOwnProducer is the attribute-group twin of
// TestParseSimpleTypeBaseBuiltUnderItsOwnProducer and
// TestParseExtensionBaseBuiltUnderItsOwnProducer: an <attributeGroup> reached
// through an <attributeGroup ref> in ANOTHER document must be folded by the
// producer of the document that DECLARES it, never by the referring one.
//
// This site carries BOTH failure flavours at once, because an <attributeGroup>
// body holds local declarations as well as unqualified references. base.xsd is a
// chameleon (§4.2.3 clause 2.3) with its own attributeFormDefault="qualified", so
// §3.2.2.2's "the ancestor <schema> element information item" makes its local
// <attribute name="a"> land in urn:x, and §F.1 task (b) coerces its own
// unqualified type="Local" to {urn:x}Local. root.xsd declares neither, so folded
// under the REFERRING producer the use would be minted as {}a typed {}Local and
// the parse would falsely fail src-resolve. c-incl-incl only makes the group
// VISIBLE to root.xsd; it transfers no resolution authority.
//
// Both document orders are forced. They differ in whether base.xsd's own
// run() — which builds the top-level AttributeGroupDefinition component — has
// happened before the referring complex type folds the group, so the pair pins
// that the fold is order-independent rather than accidentally correct in the
// order the fixture happens to discover documents in.
func TestParseAttributeGroupFoldedUnderItsOwnProducer(t *testing.T) {
	const xs = `xmlns:xs="http://www.w3.org/2001/XMLSchema"`
	// A chameleon that qualifies its local attributes and names a sibling type
	// WITHOUT a prefix, so both properties under test come from this document.
	const chameleon = `<xs:schema ` + xs + ` attributeFormDefault="qualified">` +
		`<xs:simpleType name="Local"><xs:restriction base="xs:string">` +
		`<xs:maxLength value="6"/></xs:restriction></xs:simpleType>` +
		`<xs:attributeGroup name="G"><xs:attribute name="a" type="Local"/></xs:attributeGroup>` +
		`</xs:schema>`
	const refDecl = `<xs:complexType name="T"><xs:sequence/>` +
		`<xs:attributeGroup ref="tns:G"/></xs:complexType>`

	cases := []struct {
		name string
		docs map[string]string
	}{{
		// main is produced FIRST and is itself the referrer, so T folds G before
		// base.xsd's run() has built G's own component.
		name: "referrer produced before declarer",
		docs: map[string]string{
			"main.xsd": wrap("urn:x", `<xs:include schemaLocation="base.xsd"/>`+refDecl),
			"base.xsd": chameleon,
		},
	}, {
		// base.xsd is discovered — and so produced — before the sibling that refers
		// to it, so G's own component exists by the time T folds it.
		name: "declarer produced before referrer",
		docs: map[string]string{
			"main.xsd": wrap("urn:x", `<xs:include schemaLocation="base.xsd"/>`+
				`<xs:include schemaLocation="ref.xsd"/>`),
			"base.xsd": chameleon,
			"ref.xsd":  wrap("urn:x", refDecl),
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseMap(t, "main.xsd", tc.docs)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			uses := topComplexTypeIn(t, s, xsd.QName{Space: "urn:x", Local: "T"}).AttributeUses()
			if len(uses) != 1 {
				t.Fatalf("complex type {urn:x}T has %d attribute uses, want the referenced group's one", len(uses))
			}
			decl, ok := uses[0].AttributeDeclaration().(xsd.LocalAttributeDeclaration)
			if !ok {
				t.Fatalf("attribute use declaration is %T, want a local declaration", uses[0].AttributeDeclaration())
			}
			if got := decl.Declaration.Name(); got != (xsd.QName{Space: "urn:x", Local: "a"}) {
				t.Errorf("folded attribute name = %s, want {urn:x}a from base.xsd's own attributeFormDefault", got)
			}
			if got := declaredTypeName(t, decl.Declaration.TypeDefinition()); got != (xsd.QName{Space: "urn:x", Local: "Local"}) {
				t.Errorf("folded attribute type = %s, want {urn:x}Local from base.xsd's own §F.1 coercion", got)
			}
		})
	}
}

// TestParseChameleonIncludesCoercedNamespace pins §4.2.3's recursion note: A
// (targetNamespace urn:a) includes chameleon B, which itself includes C declaring
// targetNamespace urn:a. Clause 2 is evaluated against the COERCED namespace —
// the effect is as if A included B' and B' included C', with B' carrying A's
// targetNamespace — so this is legal, not a src-include violation.
func TestParseChameleonIncludesCoercedNamespace(t *testing.T) {
	s, err := parseMap(t, "a.xsd", map[string]string{
		"a.xsd": wrap("urn:a", `<xs:include schemaLocation="b.xsd"/>`),
		"b.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` +
			`<xs:include schemaLocation="c.xsd"/></xs:schema>`,
		"c.xsd": wrap("urn:a", `<xs:element name="inC" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "inC"}); !ok {
		t.Fatalf("element {urn:a}inC not found")
	}
}

// TestParseChameleonKeepsQualifiedReferences guards the other half of §F.1 task
// (b): only UNQUALIFIED references are coerced. A reference qualified through an
// in-scope default namespace keeps that namespace.
func TestParseChameleonKeepsQualifiedReferences(t *testing.T) {
	// The chameleon document binds the XSD namespace as its DEFAULT namespace, so
	// its unprefixed base="string" is already qualified as {xs}string.
	cham := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns="` + xsdNS + `">` +
		`<xs:simpleType name="code"><xs:restriction base="string"/></xs:simpleType>` +
		`</xs:schema>`
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="cham.xsd"/>`),
		"cham.xsd": cham,
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
}

// TestParseIncludeIdempotent covers §4.2.3's closing note: <include>ing the same
// schema document more than once — here both directly twice and through a
// cycle — must not violate sch-props-correct clause 2. Cycles are spec-legal.
func TestParseIncludeIdempotent(t *testing.T) {
	for _, tc := range []struct {
		name string
		docs map[string]string
	}{
		{
			name: "twice from one document",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
					`<xs:include schemaLocation="lib.xsd"/>`),
				"lib.xsd": wrap("urn:a", `<xs:element name="shared" type="xs:string"/>`),
			},
		},
		{
			name: "through a cycle",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`),
				"lib.xsd": wrap("urn:a", `<xs:include schemaLocation="main.xsd"/>`+
					`<xs:element name="shared" type="xs:string"/>`),
			},
		},
		{
			name: "diamond",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:include schemaLocation="left.xsd"/>`+
					`<xs:include schemaLocation="right.xsd"/>`),
				"left.xsd":  wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`),
				"right.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`),
				"lib.xsd":   wrap("urn:a", `<xs:element name="shared" type="xs:string"/>`),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseMap(t, "main.xsd", tc.docs)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "shared"}); !ok {
				t.Fatalf("element {urn:a}shared not found")
			}
		})
	}
}

// TestParseIncludeNamespaceMismatch covers the one src-include clause 2 failure:
// the included document resolves but declares a DIFFERENT non-absent
// targetNamespace, so none of clauses 2.1-2.4 holds.
func TestParseIncludeNamespaceMismatch(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="other.xsd"/>`),
		"other.xsd": wrap("urn:b", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	var xe *xsderr.Error
	if !errors.As(err, &xe) {
		t.Fatalf("Parse error = %v, want an *xsderr.Error", err)
	}
	if xe.Rule != "src-include" {
		t.Fatalf("rule = %q, want src-include", xe.Rule)
	}
	if xe.Loc.URI != "main.xsd" {
		t.Fatalf("loc = %s, want the <include> element in main.xsd", xe.Loc)
	}
}

// TestParseIncludeNoNamespaceIncluderMismatch is the mirror case: the includer
// has no targetNamespace and the includee does, so clause 2.1 fails for want of
// a targetNamespace on D1.
func TestParseIncludeNoNamespaceIncluderMismatch(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd":  wrap("", `<xs:include schemaLocation="other.xsd"/>`),
		"other.xsd": wrap("urn:b", `<xs:element name="e" type="xs:string"/>`),
	})
	var xe *xsderr.Error
	if !errors.As(err, &xe) || xe.Rule != "src-include" {
		t.Fatalf("Parse error = %v, want an *xsderr.Error with rule src-include", err)
	}
}

// TestParseIncludeNeitherNamespace covers src-include clause 2.2 (c-normi2):
// neither document has a targetNamespace, so the components merge unqualified
// and NO chameleon coercion happens.
func TestParseIncludeNeitherNamespace(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("", `<xs:include schemaLocation="lib.xsd"/>`),
		"lib.xsd": wrap("", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`+
			`<xs:element name="e" type="code"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Local: "code"})
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatalf("element {}e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Local: "code"}) {
		t.Fatalf("type = %s, want the no-namespace {}code", got)
	}
}

// TestParseIncludeUnresolvable covers src-include clause 2.4: a schemaLocation
// that does not resolve is NOT an error; the inclusion is simply not performed.
func TestParseIncludeUnresolvable(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="missing.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v, want the unresolvable inclusion to be skipped silently", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"}); !ok {
		t.Fatalf("element {urn:a}root not found")
	}
}

// TestParseIncludeNotWellFormed covers src-include clause 1.1: a location that
// DOES resolve but yields a document that is not well-formed is an error, unlike
// a location that fails to resolve.
func TestParseIncludeNotWellFormed(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="broken.xsd"/>`),
		"broken.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">` +
			`<xs:element name="e"/>`, // unclosed <xs:schema>
	})
	var xe *xsderr.Error
	if !errors.As(err, &xe) || xe.Rule != "src-include" {
		t.Fatalf("Parse error = %v, want an *xsderr.Error with rule src-include", err)
	}
}

// TestParseIncludeNotASchema covers src-include clause 1: the resolved document
// element must be <schema>.
func TestParseIncludeNotASchema(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd":       wrap("urn:a", `<xs:include schemaLocation="notaschema.xsd"/>`),
		"notaschema.xsd": `<html/>`,
	})
	var xe *xsderr.Error
	if !errors.As(err, &xe) || xe.Rule != "src-include" {
		t.Fatalf("Parse error = %v, want an *xsderr.Error with rule src-include", err)
	}
}

// TestParseIncludeMissingSchemaLocation checks the grammar-level case: the
// attribute the schema for schema documents requires is absent. It is distinct
// from an unresolvable location (which is not an error at all).
func TestParseIncludeMissingSchemaLocation(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include/>`),
	})
	if err == nil {
		t.Fatalf("Parse succeeded, want an error for <include> without schemaLocation")
	}
	var xe *xsderr.Error
	if errors.As(err, &xe) {
		t.Fatalf("error = %v, want a plain error: no Schema Representation Constraint governs a missing required attribute", err)
	}
}

// TestParseIncludeRelativeToBase checks that a nested <include> resolves its
// location against the base URI in scope at the <include> element, not against
// the root document's location (§4.3.2 clause 4).
func TestParseIncludeRelativeToBase(t *testing.T) {
	s, err := parseMap(t, "top/main.xsd", map[string]string{
		"top/main.xsd":    wrap("urn:a", `<xs:include schemaLocation="sub/mid.xsd"/>`),
		"top/sub/mid.xsd": wrap("urn:a", `<xs:include schemaLocation="../leaf.xsd"/>`),
		"top/leaf.xsd":    wrap("urn:a", `<xs:element name="leaf" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "leaf"}); !ok {
		t.Fatalf("element {urn:a}leaf not found: nested include did not resolve against its own base")
	}
}

// TestParseDuplicateAcrossDocuments checks that assembly feeds sch-props-correct
// clause 2 correctly: two documents declaring the same expanded name collide,
// while the once-seeded builtins do not collide with themselves.
func TestParseDuplicateAcrossDocuments(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:element name="dup" type="xs:string"/>`),
		"lib.xsd": wrap("urn:a", `<xs:element name="dup" type="xs:string"/>`),
	})
	var xe *xsderr.Error
	if !errors.As(err, &xe) || xe.Rule != "sch-props-correct" {
		t.Fatalf("Parse error = %v, want an *xsderr.Error with rule sch-props-correct", err)
	}
}

// TestParseDuplicateTypeLocatesTheSecondDeclaration pins the two positions a
// sch-props-correct (§3.17.6.1) clause 2 charge over {type definitions} carries
// when one document declares a name twice: the charge is located at the SECOND
// declaration and names the first one's position. Each position is pinned as one
// contiguous substring, the offending one as the message's own prefix (#1048).
//
// A name-keyed simple-type memo handed the second <simpleType> the first one's
// component, so both positions were the first declaration's, and for a builtin's
// name the seeded builtin's, which has no location (#3008). The <complexType>
// row is the control: its memo is keyed by declaration (#1361). The XSD-namespace
// row pins only the offending position, since the first occurrence is the seeded
// builtin.
func TestParseDuplicateTypeLocatesTheSecondDeclaration(t *testing.T) {
	for _, c := range []struct {
		name, target, body, offending, first string
	}{
		{
			name:   "simpleType",
			target: "urn:t",
			body: "\n  " + `<xs:simpleType name="foo"><xs:restriction base="xs:string"/></xs:simpleType>` +
				"\n  " + `<xs:simpleType name="foo"><xs:restriction base="xs:string"/></xs:simpleType>`,
			offending: "m.xsd:3:3: [sch-props-correct] schema {type definitions}",
			first:     "{urn:t}foo (first declared at m.xsd:2:3)",
		},
		{
			name:   "complexType",
			target: "urn:t",
			body: "\n  " + `<xs:complexType name="foo"/>` +
				"\n  " + `<xs:complexType name="foo"/>`,
			offending: "m.xsd:3:3: [sch-props-correct] schema {type definitions}",
			first:     "{urn:t}foo (first declared at m.xsd:2:3)",
		},
		{
			name:      "simpleType named for a builtin in the XSD namespace",
			target:    xsdNS,
			body:      "\n  " + `<xs:simpleType name="int"><xs:restriction base="xs:integer"/></xs:simpleType>`,
			offending: "m.xsd:2:3: [sch-props-correct] schema {type definitions}",
			first:     "{" + xsdNS + "}int (first declared at ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMap(t, "m.xsd", map[string]string{"m.xsd": wrap(c.target, c.body)})
			mustRule(t, err, "sch-props-correct")
			if !strings.HasPrefix(err.Error(), c.offending) {
				t.Fatalf("error %q does not open %q: the charge is not located at the second declaration", err, c.offending)
			}
			if !strings.Contains(err.Error(), c.first) {
				t.Fatalf("error %q does not contain %q", err, c.first)
			}
		})
	}
}

// TestParseDuplicateSimpleTypeIsItselfConstructed pins that the second
// <simpleType> of a repeated name is mapped in its own right: a facet value only
// it gets wrong is charged at mapping time, at that declaration, ahead of the
// sch-props-correct clause 2 collision finalize would charge. A name-keyed memo
// handed it the first declaration's component and never read its children
// (#3008).
func TestParseDuplicateSimpleTypeIsItselfConstructed(t *testing.T) {
	_, err := parseMap(t, "m.xsd", map[string]string{"m.xsd": wrap("urn:t", "\n  "+
		`<xs:simpleType name="foo"><xs:restriction base="xs:string"/></xs:simpleType>`+"\n  "+
		`<xs:simpleType name="foo"><xs:restriction base="xs:string">`+
		`<xs:maxLength value="abc"/></xs:restriction></xs:simpleType>`)})
	mustRule(t, err, "cvc-datatype-valid")
	if !strings.HasPrefix(err.Error(), "m.xsd:3:") {
		t.Fatalf("error %q is not located on the second declaration's line", err)
	}
}

// TestParseIdentityConstraintRefAcrossDocuments exercises the ASSEMBLY-WIDE
// identity-constraint pre-scan: a <key ref="…"> in the INCLUDING document, which
// is produced first, resolves to a definition the INCLUDED document declares
// (§4.2.3 c-incl-incl, §3.1.3). The definition sits on a local <element> nested
// in a complex type, which §3.17.2's "anywhere within the [[children]]" makes as
// referenceable as a top-level one.
func TestParseIdentityConstraintRefAcrossDocuments(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:element name="root"><xs:key ref="tns:k"/></xs:element>`),
		"lib.xsd": wrap("urn:a", `<xs:complexType name="ct"><xs:sequence>`+
			`<xs:element name="inner"><xs:key name="k">`+
			`<xs:selector xpath="a"/><xs:field xpath="@id"/></xs:key></xs:element>`+
			`</xs:sequence></xs:complexType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"})
	if !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	constraints := ed.IdentityConstraints()
	if len(constraints) != 1 {
		t.Fatalf("got %d identity constraints on root, want 1", len(constraints))
	}
	if got := constraints[0].Name(); got != (xsd.QName{Space: "urn:a", Local: "k"}) {
		t.Errorf("name = %s, want {urn:a}k", got)
	}
	// The reference borrowed the DEFINITION's mapping, built under lib.xsd's own
	// producer, rather than an empty shell built under main.xsd's.
	if got := constraints[0].Selector().Expression(); got != "a" {
		t.Errorf("selector = %q, want the definition's %q", got, "a")
	}
}

// TestParseRootUnresolvable checks that a root location that does not resolve is
// a plain error: unlike an <include>, the caller named a document that must
// exist, so §4.2.3's "not an error to fail to resolve" does not apply.
func TestParseRootUnresolvable(t *testing.T) {
	_, err := parseMap(t, "missing.xsd", map[string]string{})
	if !errors.Is(err, loader.ErrNotFound) {
		t.Fatalf("Parse error = %v, want it to wrap loader.ErrNotFound", err)
	}
	if rule, ok := xsderr.RuleOf(err); ok {
		t.Fatalf("Parse error = %v charged %q, want a plain error with no *xsderr.Error in its chain", err, rule)
	}
}

// TestParseRootNotASchema checks the caller-precondition fault: a root whose
// document element is not <schema> is a plain error, not a validity verdict
// (§3.17.2 allows <schema> not to be the document element).
func TestParseRootNotASchema(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{"main.xsd": `<html/>`})
	if err == nil {
		t.Fatalf("Parse succeeded on a non-schema root")
	}
	var xe *xsderr.Error
	if errors.As(err, &xe) {
		t.Fatalf("error = %v, want a plain error, not a validity verdict", err)
	}
}

// TestWithResolverNilPanics pins the nil-argument convention: a nil resolver is
// a caller bug, guarded like xsd.SchemaBuilder.AddType's nil component.
func TestWithResolverNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("WithResolver(nil) did not panic")
		}
	}()
	parser.WithResolver(nil)
}

// TestWithBackendNilPanics pins the same convention for the backend.
func TestWithBackendNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("WithBackend(nil) did not panic")
		}
	}()
	parser.WithBackend(nil)
}

// TestWithLoggerNilIsSilent checks that a nil logger selects the silent default
// rather than panicking or dereferencing nil (STYLE L1).
func TestWithLoggerNilIsSilent(t *testing.T) {
	_, err := parser.Parse("main.xsd",
		parser.WithResolver(loader.Map(map[string]string{"main.xsd": wrap("urn:a", "")})),
		parser.WithLogger(nil))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
}

// TestParseEmptyRedefineUnresolvedIsLoggedNotRejected pins the ONE <redefine>
// shape that keeps <include>'s silent-skip semantics: an EMPTY one. src-redefine
// clause 1 obliges the schemaLocation to resolve only "if there are any element
// information items among the [children] other than <annotation>", so an empty
// <redefine> naming nothing is not an error, and the debug logger is how a
// caller observes that the composition did not happen. The non-empty case is the
// opposite and is pinned in redefine_test.go.
func TestParseEmptyRedefineUnresolvedIsLoggedNotRejected(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s, err := parser.Parse("main.xsd",
		parser.WithResolver(loader.Map(map[string]string{
			"main.xsd": wrap("urn:a", `<xs:redefine schemaLocation="absent.xsd"/>`+
				`<xs:element name="root" type="xs:string"/>`),
		})),
		parser.WithLogger(log))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The rest of the document is still assembled: the skip is silent in the
	// components, which is precisely why the log record has to exist.
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"}); !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	got := buf.String()
	for _, want := range []string{"level=DEBUG", "rule=src-redefine", "directive=redefine", "location=absent.xsd", "at=main.xsd:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output does not mention %q, want a debug record for the unresolved empty <redefine>:\n%s", want, got)
		}
	}
}

// wrapImporting wraps body inside a <schema> with targetNamespace target — the
// xs and tns prefixes bound exactly as wrap binds them — plus the prefix "imp"
// bound to imported, so a cross-namespace QName reference can be written.
func wrapImporting(target, imported, body string) string {
	return `<xs:schema xmlns:xs="` + xsdNS + `" targetNamespace="` + target +
		`" xmlns:tns="` + target + `" xmlns:imp="` + imported + `">` + body + `</xs:schema>`
}

// mustXSDRule fails unless err is an *xsderr.Error charging rule and positioned
// in the document wantURI — the offending <import> is always in a NAMED document,
// so pinning it keeps a test from passing on a verdict reached elsewhere in the
// assembly.
func mustXSDRule(t *testing.T, err error, rule xsderr.Rule, wantURI string) {
	t.Helper()
	var xe *xsderr.Error
	if !errors.As(err, &xe) {
		t.Fatalf("Parse error = %v, want an *xsderr.Error charging %s", err, rule)
	}
	if xe.Rule != rule {
		t.Fatalf("rule = %q, want %s", xe.Rule, rule)
	}
	if xe.Loc.URI != wantURI {
		t.Fatalf("loc = %s, want the offending element in %s", xe.Loc, wantURI)
	}
}

// TestParseImportCrossNamespace is src-import's whole point (§4.2.6.2): the
// imported document's components enter the assembly IN THEIR OWN namespace, so a
// cross-namespace type= reference in the importing document resolves at finalize.
func TestParseImportCrossNamespace(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrapImporting("urn:a", "urn:b",
			`<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`+
				`<xs:element name="root" type="imp:code"/>`),
		"b.xsd": wrap("urn:b", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction>`+
			`</xs:simpleType>`+
			`<xs:element name="inB" type="tns:code"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:b", Local: "code"})
	if _, ok := s.Type(xsd.QName{Space: "urn:a", Local: "code"}); ok {
		t.Fatalf("type {urn:a}code exists: an <import> must not mint D2's components in the importer's namespace")
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"})
	if !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: "urn:b", Local: "code"}) {
		t.Fatalf("root type = %s, want {urn:b}code", got)
	}
	// D2's own components and its own intra-namespace reference are unaffected.
	if _, ok := s.Element(xsd.QName{Space: "urn:b", Local: "inB"}); !ok {
		t.Fatalf("element {urn:b}inB not found")
	}
}

// TestParseImportNoChameleonCoercion pins the difference between <import> and
// <include>: §F.1's coercion is src-include clause 2.3's alone. A bare <import>
// (no namespace attribute, src-import clause 1.2 satisfied by the importer's own
// targetNamespace) of a no-targetNamespace document leaves that document's
// components — and its unqualified QName references — in NO namespace, even
// though the importing document has one.
func TestParseImportNoChameleonCoercion(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import schemaLocation="lib.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
		"lib.xsd": wrap("", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`+
			`<xs:element name="e" type="code"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Local: "code"})
	if _, ok := s.Type(xsd.QName{Space: "urn:a", Local: "code"}); ok {
		t.Fatalf("type {urn:a}code exists: <import> must never apply §F.1 chameleon coercion")
	}
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatalf("element {}e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Local: "code"}) {
		t.Fatalf("imported unqualified reference = %s, want the no-namespace {}code", got)
	}
}

// TestParseImportLicensesOnlyItsOwnDocument covers src-resolve clause 4.2.2
// (cl.qnr.nsdeclared, §3.17.6.2): the <import> that licenses a cross-namespace
// reference must sit in the schema document making the reference — "some
// <import> element information item contained in the <schema> element
// information item of THAT schema document" — not merely somewhere in the
// assembly.
//
// Both runs assemble the SAME three documents and both put {urn:b}code in the
// schema (main.xsd imports urn:b either way), so clauses 1-3 resolve the
// reference in both and only clause 4 can tell them apart: without its own
// <import>, a2.xsd may not reach into urn:b however its siblings were composed
// (§4.2.6.1).
func TestParseImportLicensesOnlyItsOwnDocument(t *testing.T) {
	docs := func(ownImport string) map[string]string {
		return map[string]string{
			"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`+
				`<xs:include schemaLocation="a2.xsd"/>`),
			"a2.xsd": wrapImporting("urn:a", "urn:b", ownImport+
				`<xs:element name="root" type="imp:code"/>`),
			"b.xsd": wrap("urn:b", `<xs:simpleType name="code">`+
				`<xs:restriction base="xs:string"/></xs:simpleType>`),
		}
	}
	_, err := parseMap(t, "main.xsd", docs(""))
	mustXSDRule(t, err, "src-resolve", "a2.xsd")

	s, err := parseMap(t, "main.xsd", docs(`<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`))
	if err != nil {
		t.Fatalf("Parse: %v, want a2.xsd's OWN <import> to license the same reference", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"})
	if !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: "urn:b", Local: "code"}) {
		t.Fatalf("root type = %s, want {urn:b}code", got)
	}
}

// TestParseUnqualifiedReferenceNeedsBareImport covers src-resolve clause 4.1: a
// reference left in the ·absent· namespace is licensed only by a document that
// declares no targetNamespace (4.1.1) or carries an <import> with no namespace
// attribute (4.1.2). a2.xsd declares urn:a, so 4.1.1 is out and its own bare
// <import> is the only thing that can license type="code".
//
// The no-namespace {}code is in the assembly for both runs — main.xsd's bare
// <import> of lib.xsd contributes it, and <import> applies no §F.1 coercion — so
// again only clause 4 separates them.
func TestParseUnqualifiedReferenceNeedsBareImport(t *testing.T) {
	docs := func(ownImport string) map[string]string {
		return map[string]string{
			"main.xsd": wrap("urn:a", `<xs:import schemaLocation="lib.xsd"/>`+
				`<xs:include schemaLocation="a2.xsd"/>`),
			"a2.xsd": wrap("urn:a", ownImport+`<xs:element name="root" type="code"/>`),
			"lib.xsd": wrap("", `<xs:simpleType name="code">`+
				`<xs:restriction base="xs:string"/></xs:simpleType>`),
		}
	}
	_, err := parseMap(t, "main.xsd", docs(""))
	mustXSDRule(t, err, "src-resolve", "a2.xsd")

	s, err := parseMap(t, "main.xsd", docs(`<xs:import schemaLocation="lib.xsd"/>`))
	if err != nil {
		t.Fatalf("Parse: %v, want a2.xsd's own bare <import> to license the unqualified reference", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"})
	if !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Local: "code"}) {
		t.Fatalf("root type = %s, want the no-namespace {}code", got)
	}
}

// TestParseChameleonReferenceLicensedByCoercedNamespace pins clause 4 against
// §F.1: the coercion runs BEFORE the licensing test, so a chameleon document's
// unqualified reference — which task (b) has already moved into the including
// namespace — is licensed by clause 4.2.1 against that COERCED namespace, even
// though the document itself declares no targetNamespace and imports nothing.
// The reference names a sibling contributed by a THIRD document so that the
// coerced namespace, not the document's own contents, is doing the work.
func TestParseChameleonReferenceLicensedByCoercedNamespace(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="cham.xsd"/>`+
			`<xs:include schemaLocation="lib.xsd"/>`),
		"cham.xsd": `<xs:schema xmlns:xs="` + xsdNS + `">` +
			`<xs:element name="root" type="code"/></xs:schema>`,
		"lib.xsd": wrap("urn:a", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v, want §F.1's coercion to license the reference under clause 4.2.1", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"})
	if !ok {
		t.Fatalf("element {urn:a}root not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: "urn:a", Local: "code"}) {
		t.Fatalf("chameleon reference = %s, want {urn:a}code", got)
	}
}

// TestParseImportSelfNamespace covers src-import clause 1.1
// (src-import-noselfimport): the namespace attribute must not name the importing
// schema's own target namespace.
func TestParseImportSelfNamespace(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import namespace="urn:a" schemaLocation="lib.xsd"/>`),
		"lib.xsd":  wrap("urn:a", `<xs:element name="e" type="xs:string"/>`),
	})
	mustXSDRule(t, err, "src-import-noselfimport", "main.xsd")
}

// TestParseImportSelfNamespaceUnderChameleon pins that clause 1.1 is judged
// against the EFFECTIVE (post-§F.1) target namespace: a chameleon-included
// document is produced as if it declared the includer's targetNamespace, so its
// <import> of that namespace is a self-import (§4.2.3's note on the coercion).
func TestParseImportSelfNamespaceUnderChameleon(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd":  wrap("urn:a", `<xs:include schemaLocation="cham.xsd"/>`),
		"cham.xsd":  wrap("", `<xs:import namespace="urn:a" schemaLocation="other.xsd"/>`),
		"other.xsd": wrap("urn:a", `<xs:element name="e" type="xs:string"/>`),
	})
	mustXSDRule(t, err, "src-import-noselfimport", "cham.xsd")
}

// TestParseImportBareFromNoNamespaceDocument covers src-import clause 1.2: with
// no namespace attribute the enclosing <schema> must have a targetNamespace,
// since a bare <import> from a no-namespace document would import the
// no-namespace it is already in.
func TestParseImportBareFromNoNamespaceDocument(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("", `<xs:import schemaLocation="lib.xsd"/>`),
		"lib.xsd":  wrap("", `<xs:element name="e" type="xs:string"/>`),
	})
	mustXSDRule(t, err, "src-import-noselfimport", "main.xsd")
}

// TestParseImportNamespaceMismatch covers src-import clause 3.1: the namespace
// attribute's value must be identical to the resolved document's own
// targetNamespace.
func TestParseImportNamespaceMismatch(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="c.xsd"/>`),
		"c.xsd":    wrap("urn:c", `<xs:element name="e" type="xs:string"/>`),
	})
	mustXSDRule(t, err, "src-import", "main.xsd")
}

// TestParseImportNamespaceAbsentButD2HasOne covers src-import clause 3.2: with
// no namespace attribute the resolved document must have no targetNamespace.
// Clause 3 branches on the PRESENCE of namespace, not on its value.
func TestParseImportNamespaceAbsentButD2HasOne(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import schemaLocation="b.xsd"/>`),
		"b.xsd":    wrap("urn:b", `<xs:element name="e" type="xs:string"/>`),
	})
	mustXSDRule(t, err, "src-import", "main.xsd")
}

// TestParseImportWithoutSchemaLocation covers the bare <import> §4.2.6.2 calls
// out as legal: with no schemaLocation there is nothing for the reference
// strategy to succeed at, so clauses 2 and 3 do not apply and no document is
// read — in particular the importing document must NOT be re-read as its own D2.
func TestParseImportWithoutSchemaLocation(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b"/>`+
			`<xs:element name="root" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"}); !ok {
		t.Fatalf("element {urn:a}root not found")
	}
}

// TestParseImportUnresolvable covers §4.2.6.2's "It is not an error for the
// application schema component reference strategy to fail": a schemaLocation
// that does not resolve leaves clauses 2 and 3 vacuous, so the import is simply
// not performed.
func TestParseImportUnresolvable(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="missing.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v, want the unresolvable import to be skipped silently", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:a", Local: "root"}); !ok {
		t.Fatalf("element {urn:a}root not found")
	}
}

// TestParseImportNotWellFormed covers the other half of src-import clause 2: a
// location that DOES resolve must yield a <schema> in a well-formed information
// set, so a well-formedness fault there is a real violation — unlike a location
// that fails to resolve.
func TestParseImportNotWellFormed(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="broken.xsd"/>`),
		"broken.xsd": `<xs:schema xmlns:xs="` + xsdNS + `" targetNamespace="urn:b">` +
			`<xs:element name="e"/>`, // unclosed <xs:schema>
	})
	mustXSDRule(t, err, "src-import", "main.xsd")
}

// TestParseImportNotASchema covers src-import clause 2 as well: the resolved
// document element must be <schema>.
func TestParseImportNotASchema(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd":       wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="notaschema.xsd"/>`),
		"notaschema.xsd": `<html/>`,
	})
	mustXSDRule(t, err, "src-import", "main.xsd")
}

// TestParseImportIdempotent covers §4.2.6.2's note that the component-inclusion
// wording is "carefully worded so that multiple <import>ing of the same schema
// document will not constitute a violation of clause 2 [c-nmd] of Schema
// Properties Correct": the same document reached twice contributes its
// components ONCE, whether from one document, from two, or around a cycle.
func TestParseImportIdempotent(t *testing.T) {
	bImportingA := wrapImporting("urn:b", "urn:a",
		`<xs:import namespace="urn:a" schemaLocation="main.xsd"/>`+
			`<xs:element name="shared" type="xs:string"/>`)
	for _, tc := range []struct {
		name string
		docs map[string]string
	}{
		{
			name: "twice from one document",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`+
					`<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`),
				"b.xsd": wrap("urn:b", `<xs:element name="shared" type="xs:string"/>`),
			},
		},
		{
			name: "through a cycle",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`),
				"b.xsd":    bImportingA,
			},
		},
		{
			name: "diamond through an include",
			docs: map[string]string{
				"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`+
					`<xs:include schemaLocation="lib.xsd"/>`),
				"lib.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`),
				"b.xsd":   wrap("urn:b", `<xs:element name="shared" type="xs:string"/>`),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parseMap(t, "main.xsd", tc.docs)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if _, ok := s.Element(xsd.QName{Space: "urn:b", Local: "shared"}); !ok {
				t.Fatalf("element {urn:b}shared not found")
			}
		})
	}
}

// TestParseSameDocumentIncludedAndImported is why the load-once key is a
// (resolved location, namespace) pair and not a location alone: ONE
// no-targetNamespace document reached both as a chameleon <include> (coerced into
// the includer's namespace, §F.1) and as a bare <import> (staying in no
// namespace) is two different component sets, and both are part of the assembly.
func TestParseSameDocumentIncludedAndImported(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`+
			`<xs:import schemaLocation="lib.xsd"/>`),
		"lib.xsd": wrap("", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustType(t, s, xsd.QName{Space: "urn:a", Local: "code"})
	mustType(t, s, xsd.QName{Local: "code"})
}

// TestParseImportClause3OnDedupHit pins that src-import clause 3 is judged per
// <import> ELEMENT, not per document read. §4.2.6.2's note licenses skipping the
// re-composition of an already-read D2's components; it does not license skipping
// clause 3, whose "If D2 exists, that is, clause 2.1 or clause 2.2 above were
// satisfied" is a fact about the resolved document.
//
// A chameleon <include> is what makes the two paths diverge: it keys the
// no-targetNamespace lib.xsd under the includer's urn:b, so x.xsd's
// <import namespace="urn:b"> of that same document is a dedup hit — and must
// still be rejected, since lib.xsd has no targetNamespace to be identical to
// urn:b (clause 3.1). x.xsd is byte-identical in both assemblies, so the two
// verdicts must be too.
func TestParseImportClause3OnDedupHit(t *testing.T) {
	x := wrap("urn:x", `<xs:import namespace="urn:b" schemaLocation="lib.xsd"/>`)
	lib := wrap("", `<xs:simpleType name="code">`+
		`<xs:restriction base="xs:string"/></xs:simpleType>`)

	// Control: nothing has read lib.xsd, so x.xsd's <import> reads it itself.
	_, control := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:main", `<xs:import namespace="urn:x" schemaLocation="x.xsd"/>`),
		"x.xsd":    x,
		"lib.xsd":  lib,
	})
	mustXSDRule(t, control, "src-import", "x.xsd")

	// The reachability scenario: root.xsd chameleon-<include>s lib.xsd under urn:b
	// FIRST, so x.xsd's identical <import> lands on the load-once key instead.
	_, deduped := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:main", `<xs:import namespace="urn:b" schemaLocation="root.xsd"/>`+
			`<xs:import namespace="urn:x" schemaLocation="x.xsd"/>`),
		"root.xsd": wrap("urn:b", `<xs:include schemaLocation="lib.xsd"/>`),
		"x.xsd":    x,
		"lib.xsd":  lib,
	})
	mustXSDRule(t, deduped, "src-import", "x.xsd")
	if deduped.Error() != control.Error() {
		t.Fatalf("dedup-hit verdict = %q, want it identical to the control path's %q", deduped, control)
	}
}

// TestParseImportUnresolvableSkipsClause3 pins the OTHER direction of the same
// distinction: a schemaLocation that does not resolve means clause 2 was not
// satisfied, so D2 does not exist and clause 3 — gated on "If D2 exists" — must
// not be evaluated at all. §4.2.6.2: "It is not an error for the application
// schema component reference strategy to fail."
//
// The assembly is the dedup scenario's, minus the resolvable location: lib.xsd IS
// loaded under urn:b, so an implementation that treated an unresolved location as
// "D2 exists" would find a targetNamespace to compare and wrongly reject.
func TestParseImportUnresolvableSkipsClause3(t *testing.T) {
	s, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("urn:main", `<xs:import namespace="urn:b" schemaLocation="root.xsd"/>`+
			`<xs:import namespace="urn:x" schemaLocation="x.xsd"/>`),
		"root.xsd": wrap("urn:b", `<xs:include schemaLocation="lib.xsd"/>`),
		"lib.xsd": wrap("", `<xs:simpleType name="code">`+
			`<xs:restriction base="xs:string"/></xs:simpleType>`),
		"x.xsd": wrap("urn:x", `<xs:import namespace="urn:b" schemaLocation="missing.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v, want the unresolvable import to leave clause 3 vacuous", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:x", Local: "root"}); !ok {
		t.Fatalf("element {urn:x}root not found")
	}
}

// TestParseImportDedupHitComposesOnce pins that running clause 3 on a dedup hit
// did not turn the hit into a second COMPOSITION: the repeated <import> of one
// document under one namespace still contributes D2's components exactly once, so
// sch-props-correct clause 2 (c-nmd) is not tripped — which is precisely what
// §4.2.6.2's note is careful to permit.
func TestParseImportDedupHitComposesOnce(t *testing.T) {
	docs := map[string]string{
		"main.xsd": wrap("urn:a", `<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`+
			`<xs:import namespace="urn:b" schemaLocation="b.xsd"/>`+
			`<xs:element name="root" type="xs:string"/>`),
		"b.xsd": wrap("urn:b", `<xs:element name="shared" type="xs:string"/>`),
	}
	s, report, err := parser.ParseReport("main.xsd", parser.WithResolver(loader.Map(docs)))
	if err != nil {
		t.Fatalf("ParseReport: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:b", Local: "shared"}); !ok {
		t.Fatalf("element {urn:b}shared not found")
	}
	// main.xsd and b.xsd, b.xsd once: a re-composed dedup hit would show up here
	// before it showed up as a duplicate-component verdict.
	var got []string
	for _, d := range report.Documents() {
		got = append(got, d.Location)
	}
	if want := []string{"main.xsd", "b.xsd"}; !slices.Equal(got, want) {
		t.Fatalf("assembled documents = %v, want %v", got, want)
	}
	// A dedup hit whose clause 3 HOLDS is silent: the second <import> names the
	// same namespace b.xsd declares, so nothing is charged.
	if len(report.Unfollowed()) != 0 {
		t.Fatalf("unfollowed = %v, want none: a dedup hit is a followed reference", report.Unfollowed())
	}
}

// TestParseDirectiveDocumentOrder pins that <include> and <import> are followed
// in ONE document-order pass, depth-first and pre-order, rather than in two
// kind-segregated passes. The order documents enter the assembly is the order
// their components enter the builder, so it is user-visible in sch-props-correct
// duplicate reports and must not depend on which KIND of directive named a
// document (STYLE D1/D2).
func TestParseDirectiveDocumentOrder(t *testing.T) {
	docs := map[string]string{
		// import, include, import — interleaved, so a kind-segregated pass would
		// reorder them.
		"main.xsd": wrapImporting("urn:a", "urn:b",
			`<xs:import namespace="urn:b" schemaLocation="b1.xsd"/>`+
				`<xs:include schemaLocation="lib.xsd"/>`+
				`<xs:import namespace="urn:b" schemaLocation="b2.xsd"/>`),
		"lib.xsd": wrapImporting("urn:a", "urn:b",
			`<xs:import namespace="urn:b" schemaLocation="b3.xsd"/>`),
		"b1.xsd": wrap("urn:b", `<xs:element name="e1" type="xs:string"/>`),
		"b2.xsd": wrap("urn:b", `<xs:element name="e2" type="xs:string"/>`),
		"b3.xsd": wrap("urn:b", `<xs:element name="e3" type="xs:string"/>`),
	}
	var requested []string
	inner := loader.Map(docs)
	recording := loader.ResolverFunc(func(namespace, location string) (io.ReadCloser, string, error) {
		requested = append(requested, location)
		return inner.Resolve(namespace, location)
	})
	s, err := parser.Parse("main.xsd", parser.WithResolver(recording))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"main.xsd", "b1.xsd", "lib.xsd", "b3.xsd", "b2.xsd"}
	if !slices.Equal(requested, want) {
		t.Fatalf("resolution order = %v, want %v (single document-order pass, depth-first)", requested, want)
	}
	for _, local := range []string{"e1", "e2", "e3"} {
		if _, ok := s.Element(xsd.QName{Space: "urn:b", Local: local}); !ok {
			t.Fatalf("element {urn:b}%s not found", local)
		}
	}
}
