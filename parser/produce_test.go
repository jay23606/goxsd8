package parser_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

const produceURI = "mem://produce.xsd"

// produce reads doc and runs Produce with the strict backend, returning the
// finalized schema or the first error.
func produce(t *testing.T, doc string) (*xsd.Schema, error) {
	t.Helper()
	d, err := parser.ReadDocument(produceURI, strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	return parser.Produce(d, strict.New())
}

// wrap wraps body children inside a <schema> with the xs prefix bound and an
// optional targetNamespace.
func wrap(target, body string) string {
	tns := ""
	if target != "" {
		tns = ` targetNamespace="` + target + `" xmlns:tns="` + target + `"`
	}
	return `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"` + tns + `>` + body + `</xs:schema>`
}

const xsdNS = "http://www.w3.org/2001/XMLSchema"

// declaredTypeName returns the expanded name a declaration's {type definition}
// slot references, failing the test when the slot is not the by-name arm — the
// shape every type=/default assertion in this package makes. An inline anonymous
// type (xsd.InlineTypeDefinition) is asserted on directly instead, never through
// this helper.
func declaredTypeName(t *testing.T, ref xsd.TypeDefinitionOrRef) xsd.QName {
	t.Helper()
	byName, ok := ref.(xsd.TypeDefinitionRef)
	if !ok {
		t.Fatalf("{type definition} = %#v, want a by-name xsd.TypeDefinitionRef", ref)
	}
	return byName.Name
}

func TestProduceTopLevelElement(t *testing.T) {
	s, err := produce(t, wrap("", `<xs:element name="root" type="xs:string"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "root"})
	if !ok {
		t.Fatalf("element root not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("type = %s, want {xs}string", got)
	}
	if ed.ScopeVariety() != xsd.ScopeGlobal {
		t.Fatalf("scope = %s, want global", ed.ScopeVariety())
	}
}

func TestProduceTopLevelAttribute(t *testing.T) {
	s, err := produce(t, wrap("", `<xs:attribute name="count" type="xs:int"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ad, ok := s.Attribute(xsd.QName{Local: "count"})
	if !ok {
		t.Fatalf("attribute count not found")
	}
	if got := declaredTypeName(t, ad.TypeDefinition()); got != (xsd.QName{Space: xsdNS, Local: "int"}) {
		t.Fatalf("type = %s, want {xs}int", got)
	}
}

// TestProduceTopLevelAttributeInheritable pins §3.2.2.1 dcl.att.global's
// {inheritable} row on the global form: the ·actual value· of inheritable, if
// present, otherwise false. The true row is the one a hardcoded false fails
// (#831); the false and absent rows fail a hardcoded true.
func TestProduceTopLevelAttributeInheritable(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2).
	cases := []struct {
		name string
		attr string
		want bool
	}{
		{`inheritable="true"`, ` inheritable="true"`, true},
		{`inheritable="false"`, ` inheritable="false"`, false},
		{"inheritable absent", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := produce(t, wrap("", `<xs:attribute name="lang" type="xs:language"`+tc.attr+`/>`))
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			ad, ok := s.Attribute(xsd.QName{Local: "lang"})
			if !ok {
				t.Fatal("attribute lang not found")
			}
			if got := ad.Inheritable(); got != tc.want {
				t.Errorf("{inheritable} = %t, want %t (§3.2.2.1)", got, tc.want)
			}
		})
	}
}

func TestProduceElementTargetNamespace(t *testing.T) {
	s, err := produce(t, wrap("urn:x", `<xs:element name="root" type="xs:string"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Space: "urn:x", Local: "root"}); !ok {
		t.Fatalf("element {urn:x}root not found; targetNamespace not applied to {name}")
	}
}

func TestProduceSimpleTypeWithFacetAndBackReference(t *testing.T) {
	// A named simpleType, then an element referencing it (backward reference),
	// proving resolution through finalize.
	body := `<xs:simpleType name="Foo"><xs:restriction base="xs:string"><xs:minLength value="1"/></xs:restriction></xs:simpleType>` +
		`<xs:element name="e" type="tns:Foo"/>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Space: "urn:x", Local: "Foo"})
	if !ok {
		t.Fatalf("type {urn:x}Foo not found")
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("Foo is not a *SimpleType")
	}
	if base := mustBase(t, s, st); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("Foo base = %v, want {xs}string", base)
	}
	// {variety} and {primitive type definition} must derive off the base chain
	// (§3.16.2.1), with no producer-side propagation.
	if _, ok := mustVariety(t, s, st).(xsd.Atomic); !ok {
		t.Fatalf("Foo variety = %T, want Atomic", mustVariety(t, s, st))
	}
	if prim := mustPrimitive(t, s, st); prim == nil || prim.Name() != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("Foo {primitive} = %v, want {xs}string", prim)
	}
	if fs := st.OwnFacets(); len(fs) != 1 || fs[0].Kind() != xsd.FacetMinLength {
		t.Fatalf("Foo own facets = %v, want one minLength", fs)
	}
}

// simpleTypeOf produces doc's body and returns the named simple type.
func simpleTypeOf(t *testing.T, name, body string) (*xsd.SimpleType, *xsd.Schema) {
	t.Helper()
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Local: name})
	if !ok {
		t.Fatalf("type %s not found", name)
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("type %s = %T, want *xsd.SimpleType", name, td)
	}
	// The Schema comes back with the type: a produced base= is an
	// xsd.SimpleTypeRef, so every derived-property read on st is a lookup against
	// this schema (fixture_ext_test.go).
	return st, s
}

// TestProduceSiblingPatternsFoldIntoOneFacet pins xr-pattern (§4.3.4.2): the
// <pattern> children of one <restriction> are branches of a SINGLE pattern facet
// (same-step OR), not one facet each — one-per-sibling both misreads them as
// separate ANDed steps (cvc-pattern-valid §4.3.4.4) and, being two same-kind
// ownFacets, is rejected by st-props-correct clause 4 before that.
func TestProduceSiblingPatternsFoldIntoOneFacet(t *testing.T) {
	st, sch := simpleTypeOf(t, "st", `<xs:simpleType name="st">
	  <xs:restriction base="xs:string">
	    <xs:minLength value="1"/>
	    <xs:pattern value="[a-z]+"/>
	    <xs:pattern value="[0-9]+"/>
	    <xs:maxLength value="8"/>
	  </xs:restriction>
	</xs:simpleType>`)

	facets := st.OwnFacets()
	kinds := make([]xsd.FacetKind, 0, len(facets))
	for _, f := range facets {
		kinds = append(kinds, f.Kind())
	}
	want := []xsd.FacetKind{xsd.FacetMinLength, xsd.FacetPattern, xsd.FacetMaxLength}
	if !slices.Equal(kinds, want) {
		t.Fatalf("facet kinds = %v, want %v (one folded pattern facet, in document order)", kinds, want)
	}
	if got := facets[1].Values(); !slices.Equal(got, []string{"[a-z]+", "[0-9]+"}) {
		t.Fatalf("pattern {value} = %q, want both branches in document order", got)
	}

	for _, lex := range []string{"abc", "123"} {
		if _, err := value.ValidateLexical(strict.New(), sch, st, lex, nil, xpath.FacetAssertions(time.Time{})); err != nil {
			t.Errorf("ValidateLexical(%q) = %v, want accept (matches one same-step pattern)", lex, err)
		}
	}
	if _, err := value.ValidateLexical(strict.New(), sch, st, "a1", nil, xpath.FacetAssertions(time.Time{})); err == nil {
		t.Error(`ValidateLexical("a1") = nil, want a cvc-pattern-valid rejection (matches neither branch)`)
	}
}

// TestProduceSinglePatternUnchanged keeps the one-<pattern> case at exactly one
// {value} member (xr-pattern case 1: R is that value).
func TestProduceSinglePatternUnchanged(t *testing.T) {
	st, sch := simpleTypeOf(t, "st", `<xs:simpleType name="st">
	  <xs:restriction base="xs:string"><xs:pattern value="[a-z]+"/></xs:restriction>
	</xs:simpleType>`)

	facets := st.OwnFacets()
	if len(facets) != 1 || facets[0].Kind() != xsd.FacetPattern {
		t.Fatalf("own facets = %v, want one pattern facet", facets)
	}
	if got := facets[0].Values(); !slices.Equal(got, []string{"[a-z]+"}) {
		t.Fatalf("pattern {value} = %q, want [[a-z]+]", got)
	}
	if _, err := value.ValidateLexical(strict.New(), sch, st, "abc", nil, xpath.FacetAssertions(time.Time{})); err != nil {
		t.Errorf(`ValidateLexical("abc") = %v, want accept`, err)
	}
	if _, err := value.ValidateLexical(strict.New(), sch, st, "ABC", nil, xpath.FacetAssertions(time.Time{})); err == nil {
		t.Error(`ValidateLexical("ABC") = nil, want a cvc-pattern-valid rejection`)
	}
}

// TestProduceCrossStepPatternsStillANDed guards the other half of xr-pattern:
// folding same-step siblings must not fold ACROSS derivation steps, which stay
// separate effective facets and are ANDed.
func TestProduceCrossStepPatternsStillANDed(t *testing.T) {
	st, sch := simpleTypeOf(t, "derived", `<xs:simpleType name="base">
	  <xs:restriction base="xs:string">
	    <xs:pattern value="[a-z]+"/>
	    <xs:pattern value="[0-9]+"/>
	  </xs:restriction>
	</xs:simpleType>
	<xs:simpleType name="derived">
	  <xs:restriction base="base"><xs:pattern value=".{3}"/></xs:restriction>
	</xs:simpleType>`)

	patterns := 0
	for _, ef := range mustEffectiveFacets(t, sch, st) {
		if ef.Facet().Kind() == xsd.FacetPattern {
			patterns++
		}
	}
	if patterns != 2 {
		t.Fatalf("effective pattern facets = %d, want 2 (one per derivation step)", patterns)
	}
	if _, err := value.ValidateLexical(strict.New(), sch, st, "abc", nil, xpath.FacetAssertions(time.Time{})); err != nil {
		t.Errorf(`ValidateLexical("abc") = %v, want accept (matches both steps)`, err)
	}
	// "ab" satisfies the base step's OR-set but not the derived step's .{3}.
	if _, err := value.ValidateLexical(strict.New(), sch, st, "ab", nil, xpath.FacetAssertions(time.Time{})); err == nil {
		t.Error(`ValidateLexical("ab") = nil, want rejection: cross-step patterns are ANDed`)
	}
	// "A1c" satisfies the derived step but neither base branch.
	if _, err := value.ValidateLexical(strict.New(), sch, st, "A1c", nil, xpath.FacetAssertions(time.Time{})); err == nil {
		t.Error(`ValidateLexical("A1c") = nil, want rejection: the base step's patterns still apply`)
	}
}

// TestProduceRestrictionFacetInterleaving pins the document-order guarantee
// restrictionFacets gives when its TWO folds — every <pattern> child into one
// pattern facet (xr-pattern §4.3.4.2) and every <assertion> child into one
// assertions facet (Datatypes §4.3.13) — are interleaved with plain one-to-one
// facets: each fold lands at the position of its KIND's FIRST child, and the
// pattern facet's {value} branches stay in document order across arbitrarily
// many intervening siblings.
//
// The cross-KIND ordering is a determinism choice of this codebase (STYLE D2),
// not a spec rule: the oracle's grounding on #314 confirms {facets} is
// spec-defined as "A set of Constraining Facet components" (std-facets) and
// §4.3 is silent on where a folded facet sits among other kinds. What IS
// spec-mandated is the ordering WITHIN each fold's own {value} — xr-pattern's
// concatenation "in order" and xr-assertions clause 2's "in document order" —
// and this test proves interleaving leaves that untouched.
//
// FAILURE CAPABILITY: reworking restrictionFacets to collect pattern values in
// the loop and splice them with a SECOND post-loop slices.Insert — mirroring
// the assertions insert, the design #214 deliberately rejected — fails the
// "assertion before patterns" and "minLength, assertion, patterns" cases: two
// independent post-loop inserts invalidate each other's recorded index, so the
// second one inserts against a slice the first already shifted. The in-loop
// facets[patternAt] rewrite is what keeps exactly one insert with nothing to
// shift against. Load-bearing across the package boundary: xsd.NewFacet COPIES
// its values argument, so the repeated NewFacet(kind, patterns, ...) inside the
// loop never aliases the growing patterns slice; if that ever changed, the
// Values() assertions here are where it surfaces.
func TestProduceRestrictionFacetInterleaving(t *testing.T) {
	// The pattern values need only be distinguishable, not meaningful regexes:
	// only their accumulation order into the folded {value} is under test.
	const (
		patternA = `<xs:pattern value="a"/>`
		patternB = `<xs:pattern value="b"/>`
		patternC = `<xs:pattern value="c"/>`
		assert   = `<xs:assertion test="true()"/>`
		minLen   = `<xs:minLength value="1"/>`
		maxLen   = `<xs:maxLength value="8"/>`
	)
	cases := []struct {
		name     string
		children string
		kinds    []xsd.FacetKind
		values   []string
	}{{
		name:     "pattern assertion pattern minLength",
		children: patternA + assert + patternB + minLen,
		kinds:    []xsd.FacetKind{xsd.FacetPattern, xsd.FacetAssertions, xsd.FacetMinLength},
		values:   []string{"a", "b"},
	}, {
		name:     "assertion pattern pattern",
		children: assert + patternA + patternB,
		kinds:    []xsd.FacetKind{xsd.FacetAssertions, xsd.FacetPattern},
		values:   []string{"a", "b"},
	}, {
		name:     "minLength assertion pattern pattern",
		children: minLen + assert + patternA + patternB,
		kinds:    []xsd.FacetKind{xsd.FacetMinLength, xsd.FacetAssertions, xsd.FacetPattern},
		values:   []string{"a", "b"},
	}, {
		name:     "minLength pattern pattern assertion",
		children: minLen + patternA + patternB + assert,
		kinds:    []xsd.FacetKind{xsd.FacetMinLength, xsd.FacetPattern, xsd.FacetAssertions},
		values:   []string{"a", "b"},
	}, {
		name:     "pattern minLength pattern maxLength pattern",
		children: patternA + minLen + patternB + maxLen + patternC,
		kinds:    []xsd.FacetKind{xsd.FacetPattern, xsd.FacetMinLength, xsd.FacetMaxLength},
		values:   []string{"a", "b", "c"},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, _ := simpleTypeOf(t, "st", `<xs:simpleType name="st">
			  <xs:restriction base="xs:string">`+tc.children+`</xs:restriction>
			</xs:simpleType>`)

			facets := st.OwnFacets()
			kinds := make([]xsd.FacetKind, 0, len(facets))
			for _, f := range facets {
				kinds = append(kinds, f.Kind())
			}
			if !slices.Equal(kinds, tc.kinds) {
				t.Fatalf("facet kinds = %v, want %v (each fold at its kind's first child)", kinds, tc.kinds)
			}
			at := slices.Index(kinds, xsd.FacetPattern)
			if at < 0 {
				t.Fatalf("facet kinds = %v, want one folded pattern facet", kinds)
			}
			if got := facets[at].Values(); !slices.Equal(got, tc.values) {
				t.Fatalf("pattern {value} = %q, want %q in document order", got, tc.values)
			}
		})
	}
}

func TestProduceSimpleTypeForwardReferenceChain(t *testing.T) {
	// B is declared before A in document order, but A restricts B and B restricts
	// xs:string. Additionally a C forward-references A. Proves the topological
	// build resolves both directions.
	body := `<xs:simpleType name="B"><xs:restriction base="tns:A"><xs:maxLength value="9"/></xs:restriction></xs:simpleType>` +
		`<xs:simpleType name="A"><xs:restriction base="xs:string"><xs:minLength value="1"/></xs:restriction></xs:simpleType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	bTD, _ := s.Type(xsd.QName{Space: "urn:x", Local: "B"})
	bST := bTD.(*xsd.SimpleType)
	aTD, _ := s.Type(xsd.QName{Space: "urn:x", Local: "A"})
	aST := aTD.(*xsd.SimpleType)
	// B's base= is a by-name xsd.SimpleTypeRef, so component identity is asserted
	// by RESOLVING it through the finalized Schema — which is the only way it can
	// be asserted now, and the stronger claim: the reference resolves to the very
	// component {type definitions} holds, not to a rebuilt twin.
	if mustBase(t, s, bST) != aST {
		t.Fatalf("B's resolved {base type definition} is not the same *SimpleType as A (component identity broken)")
	}
	if base := mustBase(t, s, aST); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("A base = %v, want {xs}string", base)
	}
}

func TestProduceSimpleTypeCircularRejected(t *testing.T) {
	// A restricts B, B restricts A: a circular base chain.
	body := `<xs:simpleType name="A"><xs:restriction base="tns:B"/></xs:simpleType>` +
		`<xs:simpleType name="B"><xs:restriction base="tns:A"/></xs:simpleType>`
	_, err := produce(t, wrap("urn:x", body))
	assertRule(t, err, "st-props-correct")
	// The rule ID alone no longer identifies WHICH check spoke: the producer's own
	// on-stack cycle guard is gone (a by-name base= is deferred, so constructing a
	// simple type recurses into no other), and st-props-correct has several
	// clauses. Naming the clause is what keeps this test testing the circularity
	// rather than passing on some earlier clause-1 rejection reached first.
	if got := err.Error(); !strings.Contains(got, "st-props-correct clause 2") ||
		!strings.Contains(got, "circular {base type definition} chain") {
		t.Fatalf("rejection = %q, want finalize's checkSimpleBaseAcyclic charging st-props-correct clause 2", got)
	}
}

// TestProducedNamedBaseIsAlwaysARef is ruling 4(a)'s pin, and it is the only
// thing that detects the drift it forbids: EVERY by-name base= must be stored as
// an xsd.SimpleTypeRef, never as an xsd.OwnedSimpleType wrapping the resolved
// component. The owned arm is legal — an inline <simpleType> base and the
// src-expredef original both take it — so nothing about the type system stops a
// producer from "helpfully" resolving a name and storing the result, which would
// opt that one call site out of deferred resolution silently.
//
// The base is read through a resolver that resolves NOTHING: an owned arm would
// answer with its component, a by-name arm must fail. That is the observation
// the exported surface allows — the arm itself is not readable — and it is
// exact, because those are the only two arms.
func TestProducedNamedBaseIsAlwaysARef(t *testing.T) {
	// A names a BUILTIN (already constructed and memoized, the case most likely
	// to tempt a producer into storing the live pointer) and B names a local type.
	body := `<xs:simpleType name="A"><xs:restriction base="xs:string"/></xs:simpleType>` +
		`<xs:simpleType name="B"><xs:restriction base="tns:A"/></xs:simpleType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	for _, local := range []string{"A", "B"} {
		td, ok := s.Type(xsd.QName{Space: "urn:x", Local: local})
		if !ok {
			t.Fatalf("type {urn:x}%s not found", local)
		}
		st := td.(*xsd.SimpleType)
		if _, err := st.Base(noResolver{}); err == nil {
			t.Fatalf("%s's produced base= resolved without a schema, so it is stored as an OwnedSimpleType; ruling 4(a) requires every by-name base to be an xsd.SimpleTypeRef", local)
		}
		// And it IS resolvable through the schema, so the ref is not merely broken.
		if base := mustBase(t, s, st); base == nil {
			t.Fatalf("%s's base= does not resolve through the finalized schema", local)
		}
	}
}

// noResolver resolves nothing. It is the probe TestProducedNamedBaseIsAlwaysARef
// uses to tell the two {base type definition} arms apart from outside package
// xsd: an owned arm needs no resolver and answers, a by-name arm cannot.
type noResolver struct{}

func (noResolver) Type(xsd.QName) (xsd.TypeDefinition, bool) { return nil, false }

func TestProduceRestrictionBaseAndInlineRejected(t *testing.T) {
	body := `<xs:simpleType name="Bad"><xs:restriction base="xs:string"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:restriction></xs:simpleType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-simple-type")
}

func TestProduceRestrictionNeitherBaseNorInlineRejected(t *testing.T) {
	body := `<xs:simpleType name="Bad"><xs:restriction/></xs:simpleType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-simple-type")
}

// TestProduceListItemTypeIsARef pins §3.16.2.3 map.std.list case 1a on the
// itemType= form: the produced type's {variety} is list, its {base type
// definition} is xs:anySimpleType directly (map.std.common case 2 — ONE
// component, no anonymous intermediate), and its {item type definition} is the
// DEFERRED by-name arm, resolved against the schema and not at mapping time.
func TestProduceListItemTypeIsARef(t *testing.T) {
	body := `<xs:simpleType name="L"><xs:list itemType="tns:It"/></xs:simpleType>` +
		`<xs:simpleType name="It"><xs:restriction base="xs:string"/></xs:simpleType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Space: "urn:x", Local: "L"})
	if !ok {
		t.Fatal("type L not found")
	}
	list := td.(*xsd.SimpleType)
	if _, isList := mustVariety(t, s, list).(xsd.List); !isList {
		t.Errorf("L {variety} = %#v, want xsd.List", mustVariety(t, s, list))
	}
	if base := mustBase(t, s, list); base != xsd.AnySimpleType() {
		t.Errorf("L {base type definition} = %v, want xs:anySimpleType", base.Name())
	}
	// The by-name arm cannot be followed without the schema — which is what makes
	// a forward-referenced itemType= legal (the item is declared AFTER the list
	// here, and Produce still succeeded).
	if _, err := list.Item(noResolver{}); err == nil {
		t.Fatal("L's itemType= resolved without a schema, so it is stored as an OwnedSimpleType; every by-name item must be an xsd.SimpleTypeRef")
	}
	if item := mustItem(t, s, list); item.Name() != (xsd.QName{Space: "urn:x", Local: "It"}) {
		t.Errorf("L {item type definition} = %v, want urn:x It", item.Name())
	}
}

// TestProduceListMintsWhiteSpaceFacet pins §3.16.2.1 map.std.common {facets}
// case 3: a produced list carries exactly one whiteSpace facet, collapse and
// fixed. Without it xsd's checkConstructedListFacets refuses the type at
// finalize under cos-st-restricts clause 2.2.1.2, so this asserts on the facet
// itself and not merely on Produce succeeding.
func TestProduceListMintsWhiteSpaceFacet(t *testing.T) {
	list, s := simpleTypeOf(t, "L", `<xs:simpleType name="L"><xs:list itemType="xs:string"/></xs:simpleType>`)
	eff := mustEffectiveFacets(t, s, list)
	if len(eff) != 1 {
		t.Fatalf("L {facets} has %d members, want exactly the minted whiteSpace (%v)", len(eff), eff)
	}
	f := eff[0].Facet()
	if f.Kind() != xsd.FacetWhiteSpace {
		t.Fatalf("L {facets}[0] is %s, want whiteSpace", f.Kind())
	}
	if got := f.Values(); len(got) != 1 || got[0] != "collapse" {
		t.Errorf("minted whiteSpace {value} = %v, want [collapse]", got)
	}
	if fixed, ok := f.Fixed(); !ok || !fixed {
		t.Errorf("minted whiteSpace {fixed} = %v (present %v), want true", fixed, ok)
	}
}

// TestProduceListInlineItem pins map.std.list case 1b: the inline <simpleType>
// child of a <list> is the {item type definition}, built here and owned by the
// slot, so it resolves with no schema at all.
func TestProduceListInlineItem(t *testing.T) {
	body := `<xs:simpleType name="L"><xs:list><xs:simpleType>` +
		`<xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction>` +
		`</xs:simpleType></xs:list></xs:simpleType>`
	list, s := simpleTypeOf(t, "L", body)
	item := mustItem(t, s, list)
	if item.Name() != (xsd.QName{}) {
		t.Errorf("inline item {name} = %v, want absent", item.Name())
	}
	if _, err := list.Item(noResolver{}); err != nil {
		t.Errorf("inline item is not the owned arm: %v", err)
	}
	// The inline type's own body reached the component: its maxLength is in force
	// alongside whatever xs:string contributes up the chain.
	var maxLength []string
	for _, f := range mustEffectiveFacets(t, s, item) {
		if f.Facet().Kind() == xsd.FacetMaxLength {
			maxLength = f.Facet().Values()
		}
	}
	if len(maxLength) != 1 || maxLength[0] != "4" {
		t.Errorf("inline item maxLength {value} = %v, want [4]", maxLength)
	}
}

// TestProduceListItemTypeAndInlineRejected pins src-simple-type clause 3: the
// two item sources are mutually exclusive.
func TestProduceListItemTypeAndInlineRejected(t *testing.T) {
	body := `<xs:simpleType name="L"><xs:list itemType="xs:string"><xs:simpleType>` +
		`<xs:restriction base="xs:string"/></xs:simpleType></xs:list></xs:simpleType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-simple-type")
}

// TestProduceListNeitherItemTypeNorInlineRejected pins the other half of
// src-simple-type clause 3: one of the two sources must be present.
func TestProduceListNeitherItemTypeNorInlineRejected(t *testing.T) {
	body := `<xs:simpleType name="L"><xs:list/></xs:simpleType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-simple-type")
}

// TestProduceSimpleTypeTwoAlternativesRejected pins that a <simpleType> naming
// two of the three §3.16.2.1 alternatives is rejected rather than silently
// mapped from whichever the dispatch reaches first, which would drop the other's
// whole mapping.
//
// The verdict carries NO rule ID since #1076: the second alternative repeats the
// single position xs:simpleType's content model (xmlschema11-2.md:2743) gives the
// three, which is §5.1's first bullet, and src-simple-type's four clauses
// (§3.16.3) say nothing about which alternative is chosen or how many — charging
// it would be a fabricated verdict (STYLE E2).
func TestProduceSimpleTypeTwoAlternativesRejected(t *testing.T) {
	body := `<xs:simpleType name="B"><xs:restriction base="xs:string"/><xs:list itemType="xs:string"/></xs:simpleType>`
	_, err := produce(t, wrap("", body))
	if err == nil {
		t.Fatal("Produce accepted a <simpleType> naming two of the three §3.16.2.1 alternatives")
	}
	if rule, ok := xsderr.RuleOf(err); ok {
		t.Errorf("error = %v, charged %s; want a plain grammar fault carrying no rule ID", err, rule)
	}
	if !strings.Contains(err.Error(), "repeats a position") {
		t.Errorf("error = %v, want the %q fault", err, "repeats a position")
	}
	if !strings.Contains(err.Error(), "<list> at "+produceURI+":1:") {
		t.Errorf("error = %v, want it to name the offending <list>", err)
	}
	if !strings.Contains(err.Error(), "<simpleType> at "+produceURI+":1:") {
		t.Errorf("error = %v, want it to name the owning <simpleType>", err)
	}
}

// TestProduceSimpleTypeNoAlternativeRejected pins the branch of simpleTypeBody
// that a schema document DOES reach. checkS4SChildOrder walks only the children
// that are present — s4sSlot carries no required/minOccurs notion — so an
// unfilled alternative position is invisible to it, leaving simpleTypeBody the
// one site enforcing "at least one of <restriction>, <list> and <union>".
//
// The rejection carries NO rule ID since #1097: an unfilled position of
// xs:simpleType's content model (xmlschema11-2.md:2743) is §5.1's first bullet,
// and src-simple-type's four clauses (§3.16.3) each govern a condition within an
// alternative already chosen — charging one would be a fabricated verdict (STYLE
// E2). Unlike the two-alternative shape this rejection is user-reachable, so the
// wrong rule ID reached real callers until it was corrected.
func TestProduceSimpleTypeNoAlternativeRejected(t *testing.T) {
	body := `<xs:simpleType name="B"><xs:annotation/></xs:simpleType>`
	_, err := produce(t, wrap("", body))
	if err == nil {
		t.Fatal("Produce accepted a <simpleType> naming none of the three §3.16.2.1 alternatives")
	}
	if rule, ok := xsderr.RuleOf(err); ok {
		t.Errorf("error = %v, charged %s; want a plain grammar fault carrying no rule ID", err, rule)
	}
	if !strings.Contains(err.Error(), "has no <restriction>, <list> or <union> child") {
		t.Errorf("error = %v, want the missing-alternative fault", err)
	}
	if !strings.Contains(err.Error(), "<simpleType> at "+produceURI+":1:") {
		t.Errorf("error = %v, want it to position the offending <simpleType>", err)
	}
}

// TestProduceListUnresolvableItemTypeRejected pins that a by-name item joins the
// same graph layer a by-name base does: an itemType= with nothing behind it is
// charged src-resolve clause 1.1 at finalize, never silently accepted.
func TestProduceListUnresolvableItemTypeRejected(t *testing.T) {
	body := `<xs:simpleType name="L"><xs:list itemType="tns:Missing"/></xs:simpleType>`
	_, err := produce(t, wrap("urn:x", body))
	assertRule(t, err, "src-resolve")
}

// TestProduceListOfListRejected pins cos-st-restricts clause 2.1 end to end over
// a by-name item, which is also this landing's termination argument for the
// unguarded item walks: an itemType= naming a list type is refused, so no
// itemType= cycle survives finalize.
func TestProduceListOfListRejected(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"a list of a named list", `<xs:simpleType name="Inner"><xs:list itemType="xs:string"/></xs:simpleType>` +
			`<xs:simpleType name="Outer"><xs:list itemType="tns:Inner"/></xs:simpleType>`},
		// The self-referencing item is the cycle the unguarded item walks would
		// otherwise run forever on: it is refused by the same clause, after one
		// O(1) {variety} read and no descent.
		{"a list whose itemType names itself", `<xs:simpleType name="L"><xs:list itemType="tns:L"/></xs:simpleType>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:x", tc.body))
			assertRule(t, err, "cos-st-restricts")
		})
	}
}

// TestProduceUnionMemberTypesAreRefs pins §3.16.2.4 map.std.union case 1a on the
// memberTypes= form: the produced type's {variety} is union, its {base type
// definition} is xs:anySimpleType directly (map.std.common case 2 — ONE
// component, no anonymous intermediate), and every member is the DEFERRED
// by-name arm, resolved against the schema and not at mapping time.
func TestProduceUnionMemberTypesAreRefs(t *testing.T) {
	body := `<xs:simpleType name="U"><xs:union memberTypes="tns:A xs:int"/></xs:simpleType>` +
		`<xs:simpleType name="A"><xs:restriction base="xs:string"/></xs:simpleType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Space: "urn:x", Local: "U"})
	if !ok {
		t.Fatal("type U not found")
	}
	union := td.(*xsd.SimpleType)
	if _, isUnion := mustVariety(t, s, union).(xsd.Union); !isUnion {
		t.Errorf("U {variety} = %#v, want xsd.Union", mustVariety(t, s, union))
	}
	if base := mustBase(t, s, union); base != xsd.AnySimpleType() {
		t.Errorf("U {base type definition} = %v, want xs:anySimpleType", base.Name())
	}
	// The by-name arms cannot be followed without the schema — which is what makes
	// a forward-referenced memberTypes= item legal (A is declared AFTER the union
	// here, and Produce still succeeded).
	if _, err := union.Members(noResolver{}); err == nil {
		t.Fatal("U's memberTypes= resolved without a schema, so a member is stored as an OwnedSimpleType; every by-name member must be an xsd.SimpleTypeRef")
	}
	want := []xsd.QName{{Space: "urn:x", Local: "A"}, {Space: xsdNS, Local: "int"}}
	got := memberNames(mustMembers(t, s, union))
	if !slices.Equal(got, want) {
		t.Errorf("U {member type definitions} = %v, want %v in memberTypes= order", got, want)
	}
}

// TestProduceUnionCarriesNoFacets pins §3.16.2.1 map.std.common {facets} case 4:
// a produced union's {facets} is the EMPTY set, with no whiteSpace manufactured
// the way the <list> alternative's case 3 manufactures one. Any facet here is
// refused at finalize by xsd's checkUnionGraph under cos-st-restricts clause
// 3.2.1.2, so this asserts on the emptiness itself and not merely on Produce
// succeeding.
func TestProduceUnionCarriesNoFacets(t *testing.T) {
	union, s := simpleTypeOf(t, "U", `<xs:simpleType name="U"><xs:union memberTypes="xs:string xs:int"/></xs:simpleType>`)
	if own := union.OwnFacets(); len(own) != 0 {
		t.Errorf("U own {facets} = %v, want empty", own)
	}
	if eff := mustEffectiveFacets(t, s, union); len(eff) != 0 {
		t.Errorf("U {facets} = %v, want empty", eff)
	}
}

// TestProduceUnionInlineMembers pins map.std.union case 1b: every inline
// <simpleType> child of a <union> is a member, in document order, built here and
// owned by its slot, so each resolves with no schema at all.
func TestProduceUnionInlineMembers(t *testing.T) {
	body := `<xs:simpleType name="U"><xs:union>` +
		`<xs:simpleType><xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction></xs:simpleType>` +
		`<xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType>` +
		`</xs:union></xs:simpleType>`
	union, s := simpleTypeOf(t, "U", body)
	if _, err := union.Members(noResolver{}); err != nil {
		t.Errorf("an inline member is not the owned arm: %v", err)
	}
	members := mustMembers(t, s, union)
	if len(members) != 2 {
		t.Fatalf("U has %d members, want the two inline children", len(members))
	}
	for i, m := range members {
		if m.Name() != (xsd.QName{}) {
			t.Errorf("inline member %d {name} = %v, want absent", i, m.Name())
		}
	}
	// Document order, and each inline type's own body reached its component: the
	// first member carries the maxLength, the second is the xs:int one.
	var maxLength []string
	for _, f := range mustEffectiveFacets(t, s, members[0]) {
		if f.Facet().Kind() == xsd.FacetMaxLength {
			maxLength = f.Facet().Values()
		}
	}
	if len(maxLength) != 1 || maxLength[0] != "4" {
		t.Errorf("first inline member maxLength {value} = %v, want [4]", maxLength)
	}
	if base := mustBase(t, s, members[1]); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "int"}) {
		t.Errorf("second inline member restricts %v, want {xs}int", base)
	}
}

// TestProduceUnionBothMemberSourcesInOrder is map.std.union case 1's ORDER pin,
// and the only thing that detects the drift it forbids: the memberTypes= items
// come first, in attribute-list order, and the inline children after them, in
// document order — "(a) ·resolved· to by the items in the ·actual value· of the
// memberTypes attribute … and (b) corresponding to the <simpleType>s among the
// <union> children … in order". Both sources at once is legal (src-simple-type
// clause 4 is not exclusive, unlike clause 3 for <list>), and the two never
// interleave. cos-st-restricts clause 3.2.2.3 pairs a restricting union's
// members with its base's POSITIONALLY, so any other order is a wrong component.
func TestProduceUnionBothMemberSourcesInOrder(t *testing.T) {
	// The inline members are declared with distinguishable bases, and the
	// memberTypes= items are deliberately NOT in document-order-of-declaration, so
	// a producer that sorted, or that appended sources the other way round, fails.
	body := `<xs:simpleType name="U"><xs:union memberTypes="tns:B tns:A">` +
		`<xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType>` +
		`<xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType>` +
		`</xs:union></xs:simpleType>` +
		`<xs:simpleType name="A"><xs:restriction base="xs:string"/></xs:simpleType>` +
		`<xs:simpleType name="B"><xs:restriction base="xs:decimal"/></xs:simpleType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, _ := s.Type(xsd.QName{Space: "urn:x", Local: "U"})
	members := mustMembers(t, s, td.(*xsd.SimpleType))
	if len(members) != 4 {
		t.Fatalf("U has %d members, want 2 named + 2 inline", len(members))
	}
	// Positions 0-1 are the memberTypes= items, by name; positions 2-3 the inline
	// children, anonymous and told apart by what each restricts.
	wantNamed := []xsd.QName{{Space: "urn:x", Local: "B"}, {Space: "urn:x", Local: "A"}}
	if got := memberNames(members[:2]); !slices.Equal(got, wantNamed) {
		t.Errorf("U members[0:2] = %v, want %v — memberTypes= items first, in attribute order", got, wantNamed)
	}
	wantInlineBases := []xsd.QName{{Space: xsdNS, Local: "int"}, {Space: xsdNS, Local: "boolean"}}
	var gotInlineBases []xsd.QName
	for _, m := range members[2:] {
		if m.Name() != (xsd.QName{}) {
			t.Fatalf("U members[2:] holds the named %v, so the two sources interleaved", m.Name())
		}
		gotInlineBases = append(gotInlineBases, mustBase(t, s, m).Name())
	}
	if !slices.Equal(gotInlineBases, wantInlineBases) {
		t.Errorf("U members[2:4] restrict %v, want %v — inline children last, in document order", gotInlineBases, wantInlineBases)
	}
}

// memberNames returns the {name} of each member, for the order assertions above.
func memberNames(members []*xsd.SimpleType) []xsd.QName {
	names := make([]xsd.QName, 0, len(members))
	for _, m := range members {
		names = append(names, m.Name())
	}
	return names
}

// TestProduceUnionNeitherMemberTypesNorInlineRejected pins src-simple-type clause
// 4: a <union> must carry a non-empty memberTypes= or at least one <simpleType>
// child. The whitespace-only attribute is the same failure — it contributes no
// item, so it is not "non-empty" — and not a union of nothing.
func TestProduceUnionNeitherMemberTypesNorInlineRejected(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"no memberTypes and no child", `<xs:simpleType name="U"><xs:union/></xs:simpleType>`},
		{"empty memberTypes", `<xs:simpleType name="U"><xs:union memberTypes=""/></xs:simpleType>`},
		{"whitespace-only memberTypes", `<xs:simpleType name="U"><xs:union memberTypes="   "/></xs:simpleType>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", tc.body))
			assertRule(t, err, "src-simple-type")
		})
	}
}

// TestProduceUnionUnresolvableMemberRejected pins that a by-name member joins the
// same graph layer a by-name base and itemType= do: a memberTypes= item with
// nothing behind it is charged src-resolve clause 1.1 at finalize, never silently
// accepted.
func TestProduceUnionUnresolvableMemberRejected(t *testing.T) {
	body := `<xs:simpleType name="U"><xs:union memberTypes="xs:string tns:Missing"/></xs:simpleType>`
	_, err := produce(t, wrap("urn:x", body))
	assertRule(t, err, "src-resolve")
}

// TestProduceUnionMembershipCycleRejected is this landing's termination argument
// end to end, and the reason the guard had to ship in the producer's own commit:
// a by-name membership makes a cycle representable, and the membership walks in
// xsd/derivation.go and value/valuespace.go carry no visited set. Finalize's
// checkUnionMembershipAcyclic charges cos-st-restricts clause 3.3
// (no-self-membership) instead of the walks running forever.
func TestProduceUnionMembershipCycleRejected(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"a union naming itself", `<xs:simpleType name="U"><xs:union memberTypes="tns:U"/></xs:simpleType>`},
		{"two unions naming each other", `<xs:simpleType name="U"><xs:union memberTypes="tns:V"/></xs:simpleType>` +
			`<xs:simpleType name="V"><xs:union memberTypes="tns:U"/></xs:simpleType>`},
		// The cycle that is NOT member edges alone: V restricts U, so V's own
		// membership is U's (map.std.union case 2) and V is derived from U — clause
		// 3.3's second conjunct, "nor any type derived from it".
		{"a union whose member restricts it", `<xs:simpleType name="U"><xs:union memberTypes="tns:V"/></xs:simpleType>` +
			`<xs:simpleType name="V"><xs:restriction base="tns:U"/></xs:simpleType>`},
		// Through an anonymous member, which no index holds: the inline union's own
		// memberTypes= names the type that owns it.
		{"a cycle through an inline member", `<xs:simpleType name="U"><xs:union>` +
			`<xs:simpleType><xs:union memberTypes="tns:U"/></xs:simpleType></xs:union></xs:simpleType>`},
		// The union is the ANONYMOUS inline base of M, so it is no root of the
		// walk; the verdict is charged against M, whose membership is that union's
		// and whose member X derives from it. This is the coverage claim
		// checkUnionMembershipAcyclic's roots paragraph makes.
		{"a cycle through an anonymous union base", `<xs:simpleType name="M"><xs:restriction>` +
			`<xs:simpleType><xs:union memberTypes="tns:X"/></xs:simpleType></xs:restriction></xs:simpleType>` +
			`<xs:simpleType name="X"><xs:restriction base="tns:M"/></xs:simpleType>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:x", tc.body))
			assertRule(t, err, "cos-st-restricts")
			if got := err.Error(); !strings.Contains(got, "cos-st-restricts clause 3.3") {
				t.Fatalf("rejection = %q, want finalize's checkUnionMembershipAcyclic charging clause 3.3", got)
			}
		})
	}
}

// TestProduceUnionMembershipDiamondAccepted guards the other polarity: one type
// reached twice through different member paths is not a cycle, and the walk must
// not report one just because a node is visited more than once.
func TestProduceUnionMembershipDiamondAccepted(t *testing.T) {
	body := `<xs:simpleType name="Top"><xs:union memberTypes="tns:L tns:R"/></xs:simpleType>` +
		`<xs:simpleType name="L"><xs:union memberTypes="tns:Leaf"/></xs:simpleType>` +
		`<xs:simpleType name="R"><xs:union memberTypes="tns:Leaf"/></xs:simpleType>` +
		`<xs:simpleType name="Leaf"><xs:restriction base="xs:string"/></xs:simpleType>`
	if _, err := produce(t, wrap("urn:x", body)); err != nil {
		t.Fatalf("Produce(a diamond membership) = %v, want acceptance", err)
	}
}

// TestProduceEnumerationFacetFolded pins xr-enumeration (§4.3.5.2): every
// <enumeration> child of ONE <restriction> contributes a member to a SINGLE
// enumeration facet, whose {value} lists them in document order and which lands
// at the FIRST <enumeration> child's position among the sibling facets (STYLE
// D2). One facet per child would be two same-kind own facets, which
// st-props-correct clause 4 rejects; src-simple-type clause 1 excepts
// xs:enumeration from its no-two-same-named-children rule, so the several-child
// schema is legal and charging that rule on it was fabricated (#740).
func TestProduceEnumerationFacetFolded(t *testing.T) {
	body := `<xs:simpleType name="E"><xs:restriction base="xs:string">` +
		`<xs:minLength value="1"/>` +
		`<xs:enumeration value="b"/>` +
		`<xs:enumeration value="a"/>` +
		`<xs:maxLength value="9"/>` +
		`<xs:enumeration value="c"/>` +
		`</xs:restriction></xs:simpleType>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Local: "E"})
	if !ok {
		t.Fatal("type E not found")
	}
	st, ok := td.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("type E = %T, want *xsd.SimpleType", td)
	}
	facets := st.OwnFacets()
	kinds := make([]xsd.FacetKind, 0, len(facets))
	for _, f := range facets {
		kinds = append(kinds, f.Kind())
	}
	wantKinds := []xsd.FacetKind{xsd.FacetMinLength, xsd.FacetEnumeration, xsd.FacetMaxLength}
	if !slices.Equal(kinds, wantKinds) {
		t.Fatalf("facet kinds = %v, want %v", kinds, wantKinds)
	}
	members, ok := facets[1].EnumerationMembers()
	if !ok {
		t.Fatal("facets[1] carries no enumeration members")
	}
	lexicals := make([]string, 0, len(members))
	for _, m := range members {
		lexicals = append(lexicals, m.Lexical())
	}
	if want := []string{"b", "a", "c"}; !slices.Equal(lexicals, want) {
		t.Fatalf("enumeration {value} = %v, want %v in document order", lexicals, want)
	}
	if got := facets[1].Values(); !slices.Equal(got, lexicals) {
		t.Fatalf("Values() = %v, want the members' lexical forms %v", got, lexicals)
	}
}

// TestProduceCrossStepEnumerationReplaces guards the half of xr-enumeration that
// is NOT xr-pattern's: §4.3.5.2 carries no union-with-the-base-facet clause, so a
// derived restriction's enumeration is composed by the generic same-kind overlay
// (st-restrict-facets §3.16.6.4, key-facets-overlay) and REPLACES the base's
// outright. Under a union the base's other members would stay valid.
func TestProduceCrossStepEnumerationReplaces(t *testing.T) {
	st, sch := simpleTypeOf(t, "derived", `<xs:simpleType name="base">
	  <xs:restriction base="xs:string">
	    <xs:enumeration value="a"/>
	    <xs:enumeration value="b"/>
	    <xs:enumeration value="c"/>
	  </xs:restriction>
	</xs:simpleType>
	<xs:simpleType name="derived">
	  <xs:restriction base="base"><xs:enumeration value="a"/></xs:restriction>
	</xs:simpleType>`)

	var enums []xsd.Facet
	for _, ef := range mustEffectiveFacets(t, sch, st) {
		if ef.Facet().Kind() == xsd.FacetEnumeration {
			enums = append(enums, ef.Facet())
		}
	}
	if len(enums) != 1 {
		t.Fatalf("effective enumeration facets = %d, want 1 (the derived step replaces the base's)", len(enums))
	}
	if got := enums[0].Values(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("effective enumeration {value} = %v, want [a] — a union with the base's would carry b and c", got)
	}
	if _, err := value.ValidateLexical(strict.New(), sch, st, "a", nil, xpath.FacetAssertions(time.Time{})); err != nil {
		t.Errorf(`ValidateLexical("a") = %v, want accept`, err)
	}
	if _, err := value.ValidateLexical(strict.New(), sch, st, "b", nil, xpath.FacetAssertions(time.Time{})); err == nil {
		t.Error(`ValidateLexical("b") = nil, want a cvc-enumeration-valid rejection: the base's member is not the derived's`)
	}
}

func TestProduceElementTypeAndInlineRejected(t *testing.T) {
	body := `<xs:element name="e" type="xs:string"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:element>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-element")
}

// TestProduceElementInlineSimpleType pins §3.3.2.1 dcl.elt.common clause 1 on the
// GLOBAL path (#442): a top-level <element> with an inline <simpleType> child
// takes that anonymous type as its {type definition}, through the same
// xsd.InlineTypeDefinition arm the local form uses, and the type carries the
// restriction's own base and facets rather than a defaulted xs:anyType. Its
// {scope} stays global — §3.3.2.2 dcl.elt.global supplements {scope} and {target
// namespace} alone, so mapping tier 1 must not disturb it.
func TestProduceElementInlineSimpleType(t *testing.T) {
	body := `<xs:element name="e"><xs:simpleType><xs:restriction base="xs:string"><xs:maxLength value="3"/></xs:restriction></xs:simpleType></xs:element>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatal("element e not found")
	}
	if ed.ScopeVariety() != xsd.ScopeGlobal {
		t.Fatalf("{scope}.{variety} = %v, want global", ed.ScopeVariety())
	}
	st := inlineSimpleType(t, ed.TypeDefinition())
	if base := mustBase(t, s, st); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("inline type base = %v, want {xs}string", base)
	}
	if got := st.OwnFacets(); len(got) != 1 || got[0].Kind() != xsd.FacetMaxLength {
		t.Fatalf("inline type own facets = %v, want one maxLength", got)
	}
	// The anonymous type is NOT registered in {type definitions}: it has no name
	// to be resolved by.
	if _, ok := s.Type(xsd.QName{}); ok {
		t.Fatal("the anonymous inline type was registered in {type definitions}")
	}
}

func TestProduceElementDefaultAndFixedRejected(t *testing.T) {
	body := `<xs:element name="e" type="xs:string" default="a" fixed="b"/>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-element")
}

func TestProduceAttributeTypeAndInlineRejected(t *testing.T) {
	body := `<xs:attribute name="a" type="xs:string"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:attribute>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-attribute")
}

// TestProduceAttributeInlineSimpleType pins tier 1 of §3.2.2.1 dcl.att.global
// (#733): a top-level <attribute> with an inline <simpleType> child takes that
// anonymous type as its {type definition}, through the xsd.InlineTypeDefinition
// arm §3.2.2.2 dcl.att.local's identically-worded chain already used, carrying
// the restriction's own base and facets rather than a defaulted xs:anySimpleType.
// Its {scope} stays global.
func TestProduceAttributeInlineSimpleType(t *testing.T) {
	body := `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:int">` +
		`<xs:minInclusive value="1"/></xs:restriction></xs:simpleType></xs:attribute>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ad, ok := s.Attribute(xsd.QName{Local: "a"})
	if !ok {
		t.Fatal("attribute a not found")
	}
	if ad.ScopeVariety() != xsd.ScopeGlobal {
		t.Fatalf("{scope}.{variety} = %v, want global", ad.ScopeVariety())
	}
	st := inlineSimpleType(t, ad.TypeDefinition())
	if base := mustBase(t, s, st); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "int"}) {
		t.Fatalf("inline type base = %v, want {xs}int", base)
	}
	if got := st.OwnFacets(); len(got) != 1 || got[0].Kind() != xsd.FacetMinInclusive {
		t.Fatalf("inline type own facets = %v, want one minInclusive", got)
	}
	// The anonymous type is registered in no symbol table: it has no name to be
	// resolved by.
	if _, ok := s.Type(xsd.QName{}); ok {
		t.Fatal("the anonymous inline type was registered in {type definitions}")
	}
}

func TestProduceBadPrefixRejected(t *testing.T) {
	body := `<xs:element name="e" type="nope:string"/>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-resolve")
}

func TestProduceUnresolvableBaseRejected(t *testing.T) {
	body := `<xs:simpleType name="S"><xs:restriction base="tns:Missing"/></xs:simpleType>`
	_, err := produce(t, wrap("urn:x", body))
	assertRule(t, err, "src-resolve")
}

func TestProduceElementNoTypeDefaultsAnyType(t *testing.T) {
	// A bare <element> defaults its {type definition} to xs:anyType (§3.3.2.1
	// case 4). xs:anyType is now seeded as a Complex Type Definition (§3.4.7), so
	// the deferred reference discharges at finalize and the schema is accepted;
	// the element's {type definition} resolves to the seeded xs:anyType.
	s, err := produce(t, wrap("", `<xs:element name="e"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatalf("element e not found")
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != anyTypeQN {
		t.Fatalf("type = %s, want {xs}anyType", got)
	}
	td, ok := s.Type(anyTypeQN)
	if !ok {
		t.Fatalf("xs:anyType not present in {type definitions}")
	}
	ct, ok := td.(xsd.ComplexType)
	if !ok {
		t.Fatalf("xs:anyType is %T, want xsd.ComplexType", td)
	}
	if ct.ContentType().Variety() != xsd.ContentMixed {
		t.Fatalf("xs:anyType {content type} variety = %s, want mixed", ct.ContentType().Variety())
	}
}

var anyTypeQN = xsd.QName{Space: xsdNS, Local: "anyType"}

func TestProduceAttributeNoTypeDefaultsAnySimpleType(t *testing.T) {
	s, err := produce(t, wrap("", `<xs:attribute name="a"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ad, _ := s.Attribute(xsd.QName{Local: "a"})
	if got := declaredTypeName(t, ad.TypeDefinition()); got != (xsd.QName{Space: xsdNS, Local: "anySimpleType"}) {
		t.Fatalf("type = %s, want {xs}anySimpleType", got)
	}
}

func TestProduceAnonymousInlineBaseRestriction(t *testing.T) {
	// A restriction whose base is an inline anonymous <simpleType>.
	body := `<xs:simpleType name="Wrap"><xs:restriction><xs:simpleType><xs:restriction base="xs:string"><xs:minLength value="2"/></xs:restriction></xs:simpleType><xs:maxLength value="5"/></xs:restriction></xs:simpleType>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, _ := s.Type(xsd.QName{Local: "Wrap"})
	st := td.(*xsd.SimpleType)
	anon := mustBase(t, s, st)
	if anon == nil || anon.Name() != (xsd.QName{}) {
		t.Fatalf("Wrap base is not an anonymous (zero-QName) simple type: %v", anon)
	}
	if base := mustBase(t, s, anon); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("anonymous base's base = %v, want {xs}string", base)
	}
}

// TestProduceNonSchemaRootRejected pins the whole message, its subject and
// <file>:<line>:<col> leading, so a swapped or dropped argument fails (#1048).
func TestProduceNonSchemaRootRejected(t *testing.T) {
	d, err := parser.ReadDocument(produceURI, strings.NewReader(`<notschema/>`))
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	_, err = parser.Produce(d, strict.New())
	if err == nil {
		t.Fatalf("Produce accepted a non-<schema> root")
	}
	if want := "parser: <notschema> at " + produceURI + ":1:1 is not a <schema> document root: Produce requires one"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestProduceSkipsOutOfScope(t *testing.T) {
	// annotation, group and friends are skipped, not rejected. (complexType is no
	// longer out of scope — it is produced; see the complex-type tests below.)
	body := `<xs:annotation><xs:documentation>hi</xs:documentation></xs:annotation>` +
		`<xs:group name="g"><xs:sequence/></xs:group>` +
		`<xs:element name="e" type="xs:string"/>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if _, ok := s.Element(xsd.QName{Local: "e"}); !ok {
		t.Fatalf("element e not produced alongside skipped out-of-scope elements")
	}
	if _, ok := s.Type(xsd.QName{Local: "g"}); ok {
		t.Fatalf("group g should have been skipped, not produced")
	}
}

// complexType reads the produced complex type named local (no namespace) from a
// schema built from body, failing on any Produce error or a missing/ wrong-kind
// type. Use complexTypeIn where the assembled Schema is needed too — reading a
// produced simple type's {base type definition} is a lookup against it.
func complexType(t *testing.T, body, local string) xsd.ComplexType {
	t.Helper()
	ct, _ := complexTypeIn(t, body, local)
	return ct
}

// complexTypeIn is complexType plus the assembled Schema the type was read from.
func complexTypeIn(t *testing.T, body, local string) (xsd.ComplexType, *xsd.Schema) {
	t.Helper()
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Local: local})
	if !ok {
		t.Fatalf("complexType %s not found", local)
	}
	ct, ok := td.(xsd.ComplexType)
	if !ok {
		t.Fatalf("%s is %T, want xsd.ComplexType", local, td)
	}
	return ct, s
}

// topGroup extracts the top model group of an element-content complex type.
func topGroup(t *testing.T, ct xsd.ComplexType) xsd.ModelGroup {
	t.Helper()
	ec, ok := ct.ContentType().(xsd.ElementContent)
	if !ok {
		t.Fatalf("content type = %T, want ElementContent", ct.ContentType())
	}
	rt, ok := ec.Particle.Term().(xsd.ResolvedTerm)
	if !ok {
		t.Fatalf("top term = %T, want ResolvedTerm", ec.Particle.Term())
	}
	mg, ok := rt.Term.(xsd.ModelGroup)
	if !ok {
		t.Fatalf("top term inner = %T, want ModelGroup", rt.Term)
	}
	return mg
}

func TestProduceComplexTypeEmpty(t *testing.T) {
	ct := complexType(t, `<xs:complexType name="CT"/>`, "CT")
	if ct.ContentType().Variety() != xsd.ContentEmpty {
		t.Fatalf("variety = %s, want empty", ct.ContentType().Variety())
	}
	if ct.DerivationMethod() != xsd.DerivationRestriction {
		t.Fatalf("derivation = %s, want restriction", ct.DerivationMethod())
	}
	if ct.Base() != (xsd.TypeDefinitionOrRef(xsd.TypeDefinitionRef{Name: anyTypeQN})) {
		t.Fatalf("base = %#v, want a TypeDefinitionRef naming xs:anyType", ct.Base())
	}
}

func TestProduceComplexTypeSequence(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence>` +
		`<xs:element name="a" type="xs:string"/>` +
		`<xs:element name="b" type="xs:int" minOccurs="0" maxOccurs="unbounded"/>` +
		`</xs:sequence></xs:complexType>`
	ct := complexType(t, body, "CT")
	if ct.ContentType().Variety() != xsd.ContentElementOnly {
		t.Fatalf("variety = %s, want element-only", ct.ContentType().Variety())
	}
	mg := topGroup(t, ct)
	if mg.Compositor() != xsd.CompositorSequence {
		t.Fatalf("compositor = %s, want sequence", mg.Compositor())
	}
	ps := mg.Particles()
	if len(ps) != 2 {
		t.Fatalf("particles = %d, want 2", len(ps))
	}
	// Second particle b: 0..unbounded, local element decl.
	if ps[1].Occurs().Min() != 0 || !ps[1].Occurs().IsUnbounded() {
		t.Fatalf("b occurs = %s, want 0..unbounded", ps[1].Occurs())
	}
	rt := ps[0].Term().(xsd.ResolvedTerm)
	ed := rt.Term.(xsd.ElementDeclaration)
	if ed.Name() != (xsd.QName{Local: "a"}) || ed.ScopeVariety() != xsd.ScopeLocal {
		t.Fatalf("a decl = %s / %s, want {}a / local", ed.Name(), ed.ScopeVariety())
	}
}

func TestProduceComplexTypeChoiceAndAll(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		compositor xsd.Compositor
	}{
		{"choice", `<xs:complexType name="CT"><xs:choice><xs:element name="a" type="xs:string"/></xs:choice></xs:complexType>`, xsd.CompositorChoice},
		{"all", `<xs:complexType name="CT"><xs:all><xs:element name="a" type="xs:string"/></xs:all></xs:complexType>`, xsd.CompositorAll},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mg := topGroup(t, complexType(t, tc.body, "CT"))
			if mg.Compositor() != tc.compositor {
				t.Fatalf("compositor = %s, want %s", mg.Compositor(), tc.compositor)
			}
		})
	}
}

func TestProduceComplexTypeMixed(t *testing.T) {
	body := `<xs:complexType name="CT" mixed="true"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`
	ct := complexType(t, body, "CT")
	if ct.ContentType().Variety() != xsd.ContentMixed {
		t.Fatalf("variety = %s, want mixed", ct.ContentType().Variety())
	}
}

func TestProduceComplexTypeMixedEmptySynthesizesSequence(t *testing.T) {
	// mixed with no content model → an empty 1..1 sequence stands in (§3.4.2.3.3
	// clause 3.1.1), and the variety is mixed, not empty.
	ct := complexType(t, `<xs:complexType name="CT" mixed="true"/>`, "CT")
	if ct.ContentType().Variety() != xsd.ContentMixed {
		t.Fatalf("variety = %s, want mixed", ct.ContentType().Variety())
	}
	mg := topGroup(t, ct)
	if mg.Compositor() != xsd.CompositorSequence || len(mg.Particles()) != 0 {
		t.Fatalf("mixed-empty group = %s/%d, want empty sequence", mg.Compositor(), len(mg.Particles()))
	}
}

func TestProduceComplexContentRestriction(t *testing.T) {
	body := `<xs:complexType name="Base"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>` +
		`<xs:complexType name="CT"><xs:complexContent><xs:restriction base="tns:Base"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, _ := s.Type(xsd.QName{Space: "urn:x", Local: "CT"})
	ct := td.(xsd.ComplexType)
	if ct.Base() != (xsd.TypeDefinitionOrRef(xsd.TypeDefinitionRef{Name: xsd.QName{Space: "urn:x", Local: "Base"}})) {
		t.Fatalf("base = %#v, want a TypeDefinitionRef naming {urn:x}Base", ct.Base())
	}
	if ct.ContentType().Variety() != xsd.ContentElementOnly {
		t.Fatalf("variety = %s, want element-only", ct.ContentType().Variety())
	}
}

func TestProduceElementZeroOccursElided(t *testing.T) {
	// An element and a nested group each with minOccurs=maxOccurs=0 map to no
	// component at all (§3.9.2/§3.8.2): they must not appear in {particles}.
	body := `<xs:complexType name="CT"><xs:sequence>` +
		`<xs:element name="keep" type="xs:string"/>` +
		`<xs:element name="drop" type="xs:string" minOccurs="0" maxOccurs="0"/>` +
		`<xs:choice minOccurs="0" maxOccurs="0"><xs:element name="x" type="xs:string"/></xs:choice>` +
		`</xs:sequence></xs:complexType>`
	mg := topGroup(t, complexType(t, body, "CT"))
	ps := mg.Particles()
	if len(ps) != 1 {
		t.Fatalf("particles = %d, want 1 (zero-occurs element and group elided)", len(ps))
	}
	ed := ps[0].Term().(xsd.ResolvedTerm).Term.(xsd.ElementDeclaration)
	if ed.Name() != (xsd.QName{Local: "keep"}) {
		t.Fatalf("surviving particle = %s, want keep", ed.Name())
	}
}

func TestProduceLocalAttributeUses(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="a" type="xs:string" use="required"/>` +
		`<xs:attribute name="b" type="xs:int"/>` +
		`<xs:attribute name="gone" type="xs:string" use="prohibited"/>` +
		`</xs:complexType>`
	ct := complexType(t, body, "CT")
	uses := ct.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("attribute uses = %d, want 2 (prohibited elided)", len(uses))
	}
	if !uses[0].Required() {
		t.Fatalf("use a should be required")
	}
	if uses[1].Required() {
		t.Fatalf("use b should be optional")
	}
	decl := uses[0].AttributeDeclaration().(xsd.LocalAttributeDeclaration).Declaration
	if decl.ScopeVariety() != xsd.ScopeLocal || decl.Name() != (xsd.QName{Local: "a"}) {
		t.Fatalf("a decl = %s / %s, want {}a / local", decl.Name(), decl.ScopeVariety())
	}
}

// TestProduceLocalAttributeScopeParent is the §3.2.1 sc_a round trip at the
// parser level, for both discriminants of §3.2.2.2 dcl.att.local's {parent} row:
// an <attribute> with a <complexType> ancestor names that complex type, and one
// with none — a child of a top-level <attributeGroup>, reached here through a
// ref — names the attribute group instead, NOT the complex type that referenced
// it. The two are read back off the same produced Schema, so the parent each
// declaration reports is the expanded name the container is registered under.
func TestProduceLocalAttributeScopeParent(t *testing.T) {
	body := `<xs:attributeGroup name="G"><xs:attribute name="grouped" type="xs:string"/></xs:attributeGroup>` +
		`<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="own" type="xs:string"/>` +
		`<xs:attributeGroup ref="tns:G"/>` +
		`</xs:complexType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Space: "urn:x", Local: "CT"})
	if !ok {
		t.Fatal("complex type {urn:x}CT not found")
	}
	ct := td.(xsd.ComplexType)
	uses := ct.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("{attribute uses} has %d members, want 2 (CT's own and G's)", len(uses))
	}
	for _, tc := range []struct {
		name string
		use  xsd.AttributeUse
		want xsd.AttributeScopeParent
	}{
		{"declared in the complex type", uses[0], xsd.AttributeComplexTypeScopeParent{Name: ct.Name()}},
		{"declared in the attribute group", uses[1], xsd.AttributeGroupScopeParent{Name: xsd.QName{Space: "urn:x", Local: "G"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decl, ok := tc.use.AttributeDeclaration().(xsd.LocalAttributeDeclaration)
			if !ok {
				t.Fatalf("{attribute declaration} = %T, want a local declaration", tc.use.AttributeDeclaration())
			}
			if decl.Declaration.ScopeVariety() != xsd.ScopeLocal {
				t.Errorf("ScopeVariety() = %s, want local (§3.2.2.2)", decl.Declaration.ScopeVariety())
			}
			got, ok := decl.Declaration.Scope().Parent()
			if !ok {
				t.Fatal("Scope().Parent() ok = false, which §3.2.1 makes Required when {variety} is local")
			}
			if got != tc.want {
				t.Errorf("Scope().Parent() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestProduceAttributeRefUse(t *testing.T) {
	body := `<xs:attribute name="g" type="xs:string"/>` +
		`<xs:complexType name="CT"><xs:sequence/><xs:attribute ref="tns:g"/></xs:complexType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, _ := s.Type(xsd.QName{Space: "urn:x", Local: "CT"})
	uses := td.(xsd.ComplexType).AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("uses = %d, want 1", len(uses))
	}
	ref, ok := uses[0].AttributeDeclaration().(xsd.AttributeDeclarationRef)
	if !ok || ref.Name != (xsd.QName{Space: "urn:x", Local: "g"}) {
		t.Fatalf("attr use decl = %v, want ref {urn:x}g", uses[0].AttributeDeclaration())
	}
}

// TestProduceLocalAttributeUseValueConstraints pins §3.5.1 vc_au for the
// dcl.att.local mapping (§3.2.2.2): default=/fixed= on a local <attribute>
// populate the Attribute USE's own {value constraint}, in document order, while
// the co-produced sibling local declaration's own {value constraint} stays
// ·absent· unconditionally.
func TestProduceLocalAttributeUseValueConstraints(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="a" type="xs:string" default="dv"/>` +
		`<xs:attribute name="b" type="xs:string" fixed="fv"/>` +
		`<xs:attribute name="c" type="xs:string"/>` +
		`</xs:complexType>`
	uses := complexType(t, body, "CT").AttributeUses()
	want := []struct {
		local   string
		present bool
		kind    xsd.ValueConstraintKind
		lexical string
	}{
		{local: "a", present: true, kind: xsd.ValueDefault, lexical: "dv"},
		{local: "b", present: true, kind: xsd.ValueFixed, lexical: "fv"},
		{local: "c"},
	}
	if len(uses) != len(want) {
		t.Fatalf("attribute uses = %d, want %d", len(uses), len(want))
	}
	for i, w := range want {
		if got := attrUseLocal(uses[i]); got != w.local {
			t.Fatalf("use %d names %q, want %q (document order)", i, got, w.local)
		}
		vc, ok := uses[i].ValueConstraint()
		if ok != w.present {
			t.Fatalf("use %s {value constraint} present = %t, want %t", w.local, ok, w.present)
		}
		if ok && (vc.Kind() != w.kind || vc.LexicalForm() != w.lexical) {
			t.Fatalf("use %s {value constraint} = %s/%q, want %s/%q", w.local, vc.Kind(), vc.LexicalForm(), w.kind, w.lexical)
		}
		decl := uses[i].AttributeDeclaration().(xsd.LocalAttributeDeclaration).Declaration
		if _, ok := decl.ValueConstraint(); ok {
			t.Fatalf("local declaration %s carries a {value constraint}, but dcl.att.local leaves it absent", w.local)
		}
	}
}

// TestProduceValueConstraintCapturesNamespaceContext pins the capture a QName
// or NOTATION {lexical form} needs to denote a {value} at all (§3.3.18, adopted
// by §3.3.19): every value constraint carries the namespace bindings in scope
// at the element that wrote it, prefixed ones in document order plus the
// default namespace an unprefixed literal binds to. It covers an element
// declaration's constraint (a-props-correct clause 2's side) and an attribute
// use's (au-props-correct clause 2's) together, and a prefix declared on the
// <attribute> element itself, which only the element's OWN scope has.
func TestProduceValueConstraintCapturesNamespaceContext(t *testing.T) {
	doc := `<xs:schema xmlns:xs="` + xsdNS + `" xmlns:outer="urn:outer" xmlns="urn:default">` +
		`<xs:element name="e" type="xs:string" fixed="outer:x"/>` +
		`<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="a" type="xs:string" xmlns:inner="urn:inner" default="inner:y"/>` +
		`</xs:complexType></xs:schema>`
	s, err := produce(t, doc)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}

	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatal("element e not found")
	}
	evc, ok := ed.ValueConstraint()
	if !ok {
		t.Fatal("element e {value constraint} absent, want fixed")
	}
	wantElem := []string{"outer=urn:outer", "xml=" + xmlNS, "xs=" + xsdNS}
	if got := bindingStrings(evc.NamespaceBindings()); !slices.Equal(got, wantElem) {
		t.Errorf("element e captured bindings = %v, want %v", got, wantElem)
	}
	if ns, ok := evc.DefaultNamespace(); !ok || ns != "urn:default" {
		t.Errorf("element e captured {default namespace} = (%q, %t), want (\"urn:default\", true)", ns, ok)
	}

	td, _ := s.Type(xsd.QName{Local: "CT"})
	uses := td.(xsd.ComplexType).AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("attribute uses = %d, want 1", len(uses))
	}
	avc, ok := uses[0].ValueConstraint()
	if !ok {
		t.Fatal("attribute use a {value constraint} absent, want default")
	}
	wantAttr := []string{"inner=urn:inner", "outer=urn:outer", "xml=" + xmlNS, "xs=" + xsdNS}
	if got := bindingStrings(avc.NamespaceBindings()); !slices.Equal(got, wantAttr) {
		t.Errorf("attribute use a captured bindings = %v, want %v", got, wantAttr)
	}
	if ns, ok := avc.DefaultNamespace(); !ok || ns != "urn:default" {
		t.Errorf("attribute use a captured {default namespace} = (%q, %t), want (\"urn:default\", true)", ns, ok)
	}
}

// With no default namespace in scope, the captured {default namespace} is
// ABSENT rather than "": an unprefixed QName literal then binds to no namespace,
// which is a different {value} from one bound to a namespace declared empty.
func TestProduceValueConstraintWithoutDefaultNamespace(t *testing.T) {
	s, err := produce(t, wrap("", `<xs:element name="e" type="xs:string" fixed="x"/>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "e"})
	if !ok {
		t.Fatal("element e not found")
	}
	vc, ok := ed.ValueConstraint()
	if !ok {
		t.Fatal("element e {value constraint} absent, want fixed")
	}
	if ns, present := vc.DefaultNamespace(); present {
		t.Errorf("captured {default namespace} = (%q, %t), want absent", ns, present)
	}
}

// TestProduceQNameValueConstraintsComparedAsValues is the captured context
// reaching all the way to a verdict, through the real QName mapping: two fixed
// QName {value}s are compared as {namespace name, local part} tuples (§3.3.18),
// so au-props-correct (§3.5.6) clause 3 REJECTS one prefix bound differently on
// the use and on the declaration, and ACCEPTS two different prefixes naming one
// namespace — which a lexical comparison decides wrongly in both directions.
func TestProduceQNameValueConstraintsComparedAsValues(t *testing.T) {
	schema := func(declPrefix, declNS, usePrefix, useNS string) string {
		return `<xs:schema xmlns:xs="` + xsdNS + `" targetNamespace="urn:x" xmlns:tns="urn:x">` +
			`<xs:attribute name="g" type="xs:QName" fixed="` + declPrefix + `:x" xmlns:` + declPrefix + `="` + declNS + `"/>` +
			`<xs:complexType name="CT"><xs:sequence/>` +
			`<xs:attribute ref="tns:g" fixed="` + usePrefix + `:x" xmlns:` + usePrefix + `="` + useNS + `"/>` +
			`</xs:complexType></xs:schema>`
	}

	t.Run("one prefix bound differently is a different value", func(t *testing.T) {
		_, err := produce(t, schema("p", "urn:one", "p", "urn:two"))
		assertRule(t, err, "au-props-correct")
	})
	t.Run("two prefixes naming one namespace are the same value", func(t *testing.T) {
		if _, err := produce(t, schema("a", "urn:one", "b", "urn:one")); err != nil {
			t.Fatalf("Produce: %v, want the two fixed QNames accepted as one {value}", err)
		}
	})
	t.Run("the same binding and a different local part is a different value", func(t *testing.T) {
		doc := `<xs:schema xmlns:xs="` + xsdNS + `" targetNamespace="urn:x" xmlns:tns="urn:x" xmlns:p="urn:one">` +
			`<xs:attribute name="g" type="xs:QName" fixed="p:x"/>` +
			`<xs:complexType name="CT"><xs:sequence/><xs:attribute ref="tns:g" fixed="p:y"/></xs:complexType>` +
			`</xs:schema>`
		_, err := produce(t, doc)
		assertRule(t, err, "au-props-correct")
	})
}

// TestProduceTypelessFixedValuesComparedOverTheUnion pins the ·special· pair
// reaching a schema verdict (#1379): an <attribute> with no type= is typed
// xs:anySimpleType (§3.2.2.2), so two fixed {value}s on it are compared over
// that type's mapping union — under identity for au-props-correct (§3.5.6)
// clause 3, which addB108 and attO025 exercise, and under equal-or-identical
// for loc-testSubP (§3.4.6.4) clause 5.2.2, which derivation-ok-restriction
// clause 3 charges.
func TestProduceTypelessFixedValuesComparedOverTheUnion(t *testing.T) {
	use := func(declFixed, useFixed string) string {
		return wrap("urn:x", `<xs:attribute name="att" fixed="`+declFixed+`"/>`+
			`<xs:complexType name="CT"><xs:sequence/><xs:attribute ref="tns:att" fixed="`+useFixed+`"/></xs:complexType>`)
	}
	restriction := func(baseFixed, restrFixed string) string {
		return wrap("urn:x", `<xs:complexType name="B"><xs:attribute name="a" fixed="`+baseFixed+`"/></xs:complexType>`+
			`<xs:complexType name="R"><xs:complexContent><xs:restriction base="tns:B">`+
			`<xs:attribute name="a" fixed="`+restrFixed+`"/></xs:restriction></xs:complexContent></xs:complexType>`)
	}
	rejects := func(t *testing.T, doc string, rule xsderr.Rule, opening string) {
		t.Helper()
		_, err := produce(t, doc)
		assertRule(t, err, rule)
		if !strings.Contains(err.Error(), opening) {
			t.Fatalf("error = %v, want it to contain %q", err, opening)
		}
	}

	t.Run("au-props-correct 3: no member makes 123 and abc identical (addB108)", func(t *testing.T) {
		rejects(t, use("123", "abc"), "au-props-correct",
			`] complex type {urn:x}CT fixes attribute {urn:x}att to "abc", but the attribute declaration fixes it to "123"`)
	})
	t.Run("au-props-correct 3: nor 123 and 456 (attO025)", func(t *testing.T) {
		rejects(t, use("123", "456"), "au-props-correct",
			`] complex type {urn:x}CT fixes attribute {urn:x}att to "456", but the attribute declaration fixes it to "123"`)
	})
	t.Run("au-props-correct 3: decimal makes 1 and 1.0 identical", func(t *testing.T) {
		if _, err := produce(t, use("1", "1.0")); err != nil {
			t.Fatalf("Produce: %v, want 1 and 1.0 accepted as one identical {value}", err)
		}
	})
	t.Run("au-props-correct 3: one instant at two offsets is not identical", func(t *testing.T) {
		rejects(t, use("2000-01-01T12:00:00Z", "2000-01-01T13:00:00+01:00"), "au-props-correct",
			`] complex type {urn:x}CT fixes attribute {urn:x}att to "2000-01-01T13:00:00+01:00", but the attribute declaration fixes it to "2000-01-01T12:00:00Z"`)
	})
	t.Run("loc-testSubP 5.2.2: one instant at two offsets is equal", func(t *testing.T) {
		if _, err := produce(t, restriction("2000-01-01T12:00:00Z", "2000-01-01T13:00:00+01:00")); err != nil {
			t.Fatalf("Produce: %v, want the equal {value}s accepted", err)
		}
	})
	t.Run("loc-testSubP 5.2.2: no member makes 123 and abc equal", func(t *testing.T) {
		rejects(t, restriction("123", "abc"), "derivation-ok-restriction",
			`] complex type {urn:x}R restricts {urn:x}B and fixes attribute a to "abc", but the base fixes it to "123"`)
	})
}

// xmlNS is the reserved prefix bound with no declaration anywhere (Namespaces
// in XML §3), so it is in scope at — and captured by — every value constraint.
const xmlNS = "http://www.w3.org/XML/1998/namespace"

// bindingStrings renders captured bindings as "prefix=namespace" for comparison,
// preserving the order they were captured in.
func bindingStrings(bindings []xsd.NamespaceBinding) []string {
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.Prefix()+"="+b.Namespace())
	}
	return out
}

// TestProduceAttributeRefUseValueConstraint pins §3.5.1 vc_au for the
// ref.att.local mapping (§3.2.2.3): fixed= on the <attribute ref="..."> element
// populates that USE's own {value constraint}, leaving the referenced top-level
// declaration's own {value constraint} untouched.
func TestProduceAttributeRefUseValueConstraint(t *testing.T) {
	body := `<xs:attribute name="g" type="xs:string"/>` +
		`<xs:complexType name="CT"><xs:sequence/><xs:attribute ref="tns:g" fixed="fv"/></xs:complexType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, _ := s.Type(xsd.QName{Space: "urn:x", Local: "CT"})
	uses := td.(xsd.ComplexType).AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("uses = %d, want 1", len(uses))
	}
	vc, ok := uses[0].ValueConstraint()
	if !ok {
		t.Fatalf("ref use {value constraint} absent, want fixed/%q", "fv")
	}
	if vc.Kind() != xsd.ValueFixed || vc.LexicalForm() != "fv" {
		t.Fatalf("ref use {value constraint} = %s/%q, want fixed/%q", vc.Kind(), vc.LexicalForm(), "fv")
	}
	decl, ok := s.Attribute(xsd.QName{Space: "urn:x", Local: "g"})
	if !ok {
		t.Fatalf("top-level attribute {urn:x}g not found")
	}
	if _, ok := decl.ValueConstraint(); ok {
		t.Fatalf("top-level declaration g gained a {value constraint} from the ref use, want absent")
	}
}

// TestProduceLocalAttributeDefaultAndFixedRejected is src-attribute clause 1
// (§3.2.3) on the attribute-use path: default and fixed must not both appear.
func TestProduceLocalAttributeDefaultAndFixedRejected(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="a" type="xs:string" default="dv" fixed="fv"/></xs:complexType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-attribute")
}

// TestProduceAttributeUseValueConstraintClauses pins the default/fixed × use=
// corner of src-attribute (§3.2.3): clause 2 (default present ⇒ use="optional"),
// clause 5 (fixed present ⇒ use not "prohibited"), and clause 1 charged even under
// use="prohibited", which maps the <attribute> to no component at all (§3.2.2) yet
// leaves the representation constraint over the element item itself in force.
//
// The TOP-LEVEL form is deliberately absent: both clauses fire only on a use=
// that is present, and #652 made a top-level use= the grammar fault
// xs:topLevelAttribute demands (xmlschema11-1.md:4712) a step earlier in run, so
// no top-level <attribute> can reach either clause. That pairing is pinned by
// TestProduceTopLevelProhibitedAttrsRejected's own row instead, which asserts the
// grammar fault WINS over the clause the same element would otherwise take.
func TestProduceAttributeUseValueConstraintClauses(t *testing.T) {
	inComplexType := func(attr string) string {
		return "\n<xs:complexType name=\"CT\"><xs:sequence/>\n" + attr + "\n</xs:complexType>"
	}
	inAttributeGroup := func(attr string) string {
		return "\n<xs:attributeGroup name=\"AG\">\n" + attr + "\n</xs:attributeGroup>"
	}
	for _, tc := range []struct {
		name   string
		body   string
		clause string
	}{
		{"default+required", inComplexType(`<xs:attribute name="a" type="xs:string" default="dv" use="required"/>`), "clause 2"},
		{"default+prohibited", inComplexType(`<xs:attribute name="a" type="xs:string" default="dv" use="prohibited"/>`), "clause 2"},
		{"fixed+prohibited", inComplexType(`<xs:attribute name="a" type="xs:string" fixed="fv" use="prohibited"/>`), "clause 5"},
		{"default+fixed+prohibited", inComplexType(`<xs:attribute name="a" type="xs:string" default="dv" fixed="fv" use="prohibited"/>`), "clause 1"},
		{"attributeGroup default+required", inAttributeGroup(`<xs:attribute name="a" type="xs:string" default="dv" use="required"/>`), "clause 2"},
		{"attributeGroup fixed+prohibited", inAttributeGroup(`<xs:attribute name="a" type="xs:string" fixed="fv" use="prohibited"/>`), "clause 5"},
		{"ref fixed+prohibited", inComplexType(`<xs:attribute ref="tns:g" fixed="fv" use="prohibited"/>`) + `<xs:attribute name="g" type="xs:string"/>`, "clause 5"},
		{"ref default+required", inComplexType(`<xs:attribute ref="tns:g" default="dv" use="required"/>`) + `<xs:attribute name="g" type="xs:string"/>`, "clause 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:x", tc.body))
			assertRule(t, err, "src-attribute")
			if want := "src-attribute " + tc.clause; !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %v, want it to cite %s", err, want)
			}
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no position", err)
			}
			if loc.URI != produceURI || loc.Line != 3 {
				t.Fatalf("position = %s:%d:%d, want the <attribute>'s own line 3 of %s", loc.URI, loc.Line, loc.Col, produceURI)
			}
		})
	}
}

// TestProduceAttributeUseValueConstraintAccepted is the reverse hazard of the
// clauses above: every pairing src-attribute clauses 2 and 5 leave legal still
// produces. Clause 5 forbids only "prohibited", never "required".
//
// The nested form takes every row. The TOP-LEVEL form takes only the rows with
// no use= at all, because xs:topLevelAttribute prohibits the attribute outright
// (#652) — a row carrying use= is not a legal top-level pairing to begin with,
// and running it here would pin the opposite of what the grammar says.
func TestProduceAttributeUseValueConstraintAccepted(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2).
	for _, tc := range []struct {
		attr string
		// topLevel is true for the rows the top-level form admits — those with no
		// use=, since that attribute is prohibited there.
		topLevel bool
	}{
		{`<xs:attribute name="a" type="xs:string" default="dv" use="optional"/>`, false},
		{`<xs:attribute name="a" type="xs:string" default="dv"/>`, true},
		{`<xs:attribute name="a" type="xs:string" fixed="fv" use="required"/>`, false},
		{`<xs:attribute name="a" type="xs:string" fixed="fv" use="optional"/>`, false},
		{`<xs:attribute name="a" type="xs:string" fixed="fv"/>`, true},
		{`<xs:attribute name="a" type="xs:string" use="prohibited"/>`, false},
	} {
		t.Run(tc.attr, func(t *testing.T) {
			body := `<xs:complexType name="CT"><xs:sequence/>` + tc.attr + `</xs:complexType>`
			if _, err := produce(t, wrap("", body)); err != nil {
				t.Fatalf("Produce rejected a legal use=/value-constraint pairing: %v", err)
			}
		})
		if !tc.topLevel {
			continue
		}
		t.Run("top-level "+tc.attr, func(t *testing.T) {
			if _, err := produce(t, wrap("", tc.attr)); err != nil {
				t.Fatalf("Produce rejected a legal top-level use=/value-constraint pairing: %v", err)
			}
		})
	}
}

// TestProduceAttributeUseClause3ChargedUnderProhibited pins src-attribute
// clause 3 (§3.2.3) against a prohibited local <attribute>: clause 3 has no
// use= precondition, so it rejects exactly as it does under every other use=
// value, not silently accepted because use="prohibited" maps to no component
// at all (§3.2.2) (#358 defect 4). The neither-ref-nor-name case is a
// DELIBERATE reject here, not a skip: clause 3.1's "exactly one of ref or
// name" holds unconditionally, so <attribute use="prohibited"/> is rejected
// the same way <attribute use="optional"/> already is.
func TestProduceAttributeUseClause3ChargedUnderProhibited(t *testing.T) {
	global := `<xs:attribute name="x" type="xs:string"/>`
	for _, tc := range []struct {
		name string
		attr string
	}{
		{"ref and name", `<xs:attribute ref="tns:x" name="x" use="prohibited"/>`},
		{"ref and type", `<xs:attribute ref="tns:x" type="xs:string" use="prohibited"/>`},
		{"ref and form", `<xs:attribute ref="tns:x" form="qualified" use="prohibited"/>`},
		{"ref and inline simpleType", `<xs:attribute ref="tns:x" use="prohibited">` +
			`<xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:attribute>`},
		{"neither ref nor name", `<xs:attribute use="prohibited"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "\n<xs:complexType name=\"CT\"><xs:sequence/>\n" + tc.attr + "\n</xs:complexType>\n" + global
			_, err := produce(t, wrap("urn:x", body))
			assertRule(t, err, "src-attribute")
			if want := "src-attribute clause 3"; !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %v, want it to cite %s", err, want)
			}
			loc, ok := xsderr.LocOf(err)
			if !ok || loc.URI != produceURI {
				t.Fatalf("error %v carries no position in %s", err, produceURI)
			}
		})
	}
}

func TestProduceAnyAttributeWildcard(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence/><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>`
	ct := complexType(t, body, "CT")
	wc, ok := ct.AttributeWildcard()
	if !ok {
		t.Fatalf("attribute wildcard absent, want present")
	}
	if wc.ProcessContents() != xsd.ProcessLax {
		t.Fatalf("processContents = %s, want lax", wc.ProcessContents())
	}
	// ##other in a no-target-namespace schema admits any present namespace but not
	// ·absent· (unqualified) names.
	if wc.NamespaceConstraint().AllowsName(xsd.QName{Local: "x"}) {
		t.Fatalf("##other should reject an unqualified (absent-namespace) name")
	}
	if !wc.NamespaceConstraint().AllowsName(xsd.QName{Space: "urn:z", Local: "x"}) {
		t.Fatalf("##other should admit a foreign-namespace name")
	}
}

func TestProduceAnyElementWildcardParticle(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence><xs:any namespace="##any" minOccurs="0" maxOccurs="unbounded"/></xs:sequence></xs:complexType>`
	mg := topGroup(t, complexType(t, body, "CT"))
	ps := mg.Particles()
	if len(ps) != 1 {
		t.Fatalf("particles = %d, want 1", len(ps))
	}
	wc, ok := ps[0].Term().(xsd.ResolvedTerm).Term.(xsd.Wildcard)
	if !ok {
		t.Fatalf("term = %T, want Wildcard", ps[0].Term().(xsd.ResolvedTerm).Term)
	}
	if !wc.NamespaceConstraint().AllowsName(xsd.QName{Space: "urn:z", Local: "x"}) {
		t.Fatalf("##any wildcard should admit any name")
	}
}

func TestProduceComplexContentMixedMismatchRejected(t *testing.T) {
	// src-ct clause 5: mixed on both <complexType> and <complexContent> must agree.
	body := `<xs:complexType name="Base"><xs:sequence/></xs:complexType>` +
		`<xs:complexType name="CT" mixed="true"><xs:complexContent mixed="false"><xs:restriction base="tns:Base"><xs:sequence/></xs:restriction></xs:complexContent></xs:complexType>`
	_, err := produce(t, wrap("urn:x", body))
	assertRule(t, err, "src-ct")
}

func TestProduceAllNestedRejected(t *testing.T) {
	// cos-all-limited: an <all> may not be nested inside a <sequence>/<choice>.
	body := `<xs:complexType name="CT"><xs:sequence><xs:all><xs:element name="a" type="xs:string"/></xs:all></xs:sequence></xs:complexType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "cos-all-limited")
}

func TestProduceAllOccursGrammar(t *testing.T) {
	// The schema for schema documents restricts <all>'s OWN minOccurs/maxOccurs by
	// enumeration to 0 and 1 (§3.8.2 XML representation summary, Appendix A
	// xs:complexType "all"). §3.8.3 gives <all> no Schema Representation
	// Constraint, so an excluded value is charged the generic cvc-datatype-valid —
	// independent of cos-all-limited, which is about nesting.
	for _, tc := range []struct {
		name   string
		occurs string
		want   xsderr.Rule // empty: the document must be accepted
	}{
		{name: "absent defaults to 1", occurs: ``},
		{name: "one one", occurs: ` minOccurs="1" maxOccurs="1"`},
		{name: "zero one", occurs: ` minOccurs="0" maxOccurs="1"`},
		{name: "zero zero elides the particle", occurs: ` minOccurs="0" maxOccurs="0"`},
		{name: "minOccurs two", occurs: ` minOccurs="2" maxOccurs="2"`, want: "cvc-datatype-valid"},
		{name: "maxOccurs five", occurs: ` maxOccurs="5"`, want: "cvc-datatype-valid"},
		{name: "maxOccurs unbounded", occurs: ` maxOccurs="unbounded"`, want: "cvc-datatype-valid"},
		{name: "minOccurs two elided by maxOccurs zero", occurs: ` minOccurs="2" maxOccurs="0"`, want: "cvc-datatype-valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `<xs:complexType name="CT"><xs:all` + tc.occurs +
				`><xs:element name="a" type="xs:string"/></xs:all></xs:complexType>`
			_, err := produce(t, wrap("", body))
			if tc.want != "" {
				assertRule(t, err, tc.want)
				return
			}
			if err != nil {
				t.Fatalf("produce: %v, want the document accepted", err)
			}
		})
	}
}

func TestProduceWildcardBothNamespaceFormsRejected(t *testing.T) {
	// src-wildcard: namespace and notNamespace must not both be present.
	body := `<xs:complexType name="CT"><xs:sequence><xs:any namespace="##any" notNamespace="urn:z"/></xs:sequence></xs:complexType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-wildcard")
}

func TestProduceSimpleContentRestrictionProduced(t *testing.T) {
	// <simpleContent><restriction> synthesizes a NEW anonymous simple type from
	// the <restriction>'s facet children (§3.4.2.2 cases 1-2) and is produced
	// (#909) — this is the smoke test that no limitation error survives on the
	// form; the tableau itself is pinned in produce_simplerestriction_test.go.
	body := `<xs:complexType name="B"><xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent></xs:complexType>` +
		`<xs:complexType name="CT"><xs:simpleContent><xs:restriction base="tns:B">` +
		`<xs:maxLength value="4"/></xs:restriction></xs:simpleContent></xs:complexType>`
	if _, err := produce(t, wrap("urn:x", body)); err != nil {
		t.Fatalf("Produce: %v", err)
	}
}

// assertRule fails unless err is non-nil and its first *xsderr.Error carries the
// expected rule.
func assertRule(t *testing.T, err error, want xsderr.Rule) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error charged %s, got nil", want)
	}
	got, ok := xsderr.RuleOf(err)
	if !ok {
		t.Fatalf("error %v carries no xsderr rule, want %s", err, want)
	}
	if got != want {
		t.Fatalf("error charged %s, want %s (%v)", got, want, err)
	}
}

// TestProduceNotQNameKeywords pins the §3.10.2.2 notQName mapping: the ##defined
// and ##definedSibling keyword tokens are mapped (no longer skipped) alongside
// the literal QName members, which keep their cvc-wildcard-name clause-2 effect.
func TestProduceNotQNameKeywords(t *testing.T) {
	// A target namespace is needed so the tns: prefix of the literal member
	// resolves; an unresolvable one is a hard src-resolve error, pinned below.
	body := `<xs:complexType name="CT"><xs:sequence>` +
		`<xs:any notQName="##defined ##definedSibling tns:foo"/>` +
		`</xs:sequence></xs:complexType>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Space: "urn:x", Local: "CT"})
	if !ok {
		t.Fatalf("complex type CT not found")
	}
	mg := topGroup(t, td.(xsd.ComplexType))
	wc, ok := mg.Particles()[0].Term().(xsd.ResolvedTerm).Term.(xsd.Wildcard)
	if !ok {
		t.Fatalf("term = %T, want Wildcard", mg.Particles()[0].Term())
	}
	// The literal member still governs cvc-wildcard-name (§3.10.4.2) clause 2;
	// the keywords are resolved by cvc-wildcard (§3.10.4.1) clauses 2-3, a
	// different rule at the declaration-graph layer, so they leave AllowsName
	// alone.
	if wc.NamespaceConstraint().AllowsName(xsd.QName{Space: "urn:x", Local: "foo"}) {
		t.Errorf("AllowsName admitted the literal notQName member {urn:x}foo")
	}
	if !wc.NamespaceConstraint().AllowsName(xsd.QName{Space: "urn:x", Local: "bar"}) {
		t.Errorf("AllowsName rejected a name in no half of {disallowed names}")
	}
}

// TestProduceNotQNameRejections pins that an unrecognized ## token — including
// ##definedSibling on an <anyAttribute>, whose notQName is typed xs:qnameListA
// (§3.10.2, the machine-checkable form of w-props-correct clause 5) — is a
// datatype-validity rejection, never a silent skip.
func TestProduceNotQNameRejections(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unknown-token-on-any", `<xs:complexType name="CT"><xs:sequence><xs:any notQName="##foo"/></xs:sequence></xs:complexType>`},
		{"unknown-token-on-anyAttribute", `<xs:complexType name="CT"><xs:sequence/><xs:anyAttribute notQName="##foo"/></xs:complexType>`},
		{"definedSibling-on-anyAttribute", `<xs:complexType name="CT"><xs:sequence/><xs:anyAttribute notQName="##definedSibling"/></xs:complexType>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:x", c.body))
			if err == nil {
				t.Fatalf("Produce accepted %s, want a cvc-datatype-valid rejection", c.body)
			}
			if rule, ok := xsderr.RuleOf(err); !ok || rule != xsderr.Rule("cvc-datatype-valid") {
				t.Errorf("RuleOf = (%q, %v), want (%q, true)", rule, ok, "cvc-datatype-valid")
			}
		})
	}
	// ##defined stays legal on an attribute wildcard (cvc-wildcard clause 2.2).
	if _, err := produce(t, wrap("urn:x", `<xs:complexType name="CT"><xs:sequence/><xs:anyAttribute notQName="##defined"/></xs:complexType>`)); err != nil {
		t.Errorf("Produce rejected notQName=\"##defined\" on <anyAttribute>: %v", err)
	}
}

// TestProduceNotQNameLiteralMembers pins the literal-QName arm of the
// §3.10.2.2 notQName mapping: every literal member with a bound prefix lands in
// {disallowed names}, and a member whose prefix has no in-scope binding is a
// hard src-resolve (§3.17.6.2) rejection rather than a silently dropped member
// that would leave the wildcard more permissive than declared.
func TestProduceNotQNameLiteralMembers(t *testing.T) {
	t.Run("all-prefixes-bound", func(t *testing.T) {
		body := `<xs:complexType name="CT"><xs:sequence>` +
			`<xs:any notQName="tns:foo tns:bar xs:string"/>` +
			`</xs:sequence></xs:complexType>`
		s, err := produce(t, wrap("urn:x", body))
		if err != nil {
			t.Fatalf("Produce: %v", err)
		}
		td, ok := s.Type(xsd.QName{Space: "urn:x", Local: "CT"})
		if !ok {
			t.Fatalf("complex type CT not found")
		}
		mg := topGroup(t, td.(xsd.ComplexType))
		wc, ok := mg.Particles()[0].Term().(xsd.ResolvedTerm).Term.(xsd.Wildcard)
		if !ok {
			t.Fatalf("term = %T, want Wildcard", mg.Particles()[0].Term())
		}
		for _, name := range []xsd.QName{
			{Space: "urn:x", Local: "foo"},
			{Space: "urn:x", Local: "bar"},
			{Space: xsdNS, Local: "string"},
		} {
			if wc.NamespaceConstraint().AllowsName(name) {
				t.Errorf("AllowsName admitted the literal notQName member %v", name)
			}
		}
		if !wc.NamespaceConstraint().AllowsName(xsd.QName{Space: "urn:x", Local: "other"}) {
			t.Errorf("AllowsName rejected a name in no half of {disallowed names}")
		}
	})
	unbound := []struct {
		name string
		body string
	}{
		{"any", `<xs:complexType name="CT"><xs:sequence><xs:any notQName="nope:foo"/></xs:sequence></xs:complexType>`},
		{"anyAttribute", `<xs:complexType name="CT"><xs:sequence/><xs:anyAttribute notQName="nope:foo"/></xs:complexType>`},
	}
	for _, c := range unbound {
		t.Run("unbound-prefix-on-"+c.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:x", c.body))
			assertRule(t, err, xsderr.Rule("src-resolve"))
		})
	}
}

// TestProduceChargesFacetValueRestriction proves the producer installs
// builtin.NewRestrictionChecker at finalize: a widening bound (cos-st-restricts
// clause 1.3.2 via minInclusive-valid-restriction §4.3.10.4) and an inapplicable
// facet (clause 1.3.1 via cos-applicable-facets §4.1.5) are BOTH rejected out of
// produce(), neither of which xsd.NewSimpleType can decide on its own. The charge
// moved from the sole xsd.NewSimpleType call site to xsd's finalize pass, so what
// this pins is the SEAM being wired, not where the call sits.
func TestProduceChargesFacetValueRestriction(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantRule xsderr.Rule
	}{
		{
			"bound widens the base value space",
			`<xs:simpleType name="wide">
			   <xs:restriction base="xs:byte">
			     <xs:minInclusive value="-200"/>
			   </xs:restriction>
			 </xs:simpleType>`,
			"minInclusive-valid-restriction",
		},
		{
			"facet inapplicable to the primitive",
			`<xs:simpleType name="bad">
			   <xs:restriction base="xs:string">
			     <xs:maxInclusive value="5"/>
			   </xs:restriction>
			 </xs:simpleType>`,
			"cos-st-restricts",
		},
		{
			// #219/#323 regression guard: maxScale is a precisionDecimal-only
			// extension facet (xsd-precisionDecimal.md §4.2), so on xs:int it must be
			// RECOGNIZED as a facet, land in {facets}, and only then be rejected on
			// applicability grounds. A lookup that silently dropped it — the defect
			// two hand-typed name↔kind tables already shipped once — would produce no
			// error at all, so a nil err here is the false-accept, not a pass.
			"precisionDecimal extension facet inapplicable to the primitive",
			`<xs:simpleType name="scaled">
			   <xs:restriction base="xs:int">
			     <xs:maxScale value="2"/>
			   </xs:restriction>
			 </xs:simpleType>`,
			"cos-st-restricts",
		},
		{
			"length may not move across a restriction",
			`<xs:simpleType name="lenbase">
			   <xs:restriction base="xs:string">
			     <xs:length value="5"/>
			   </xs:restriction>
			 </xs:simpleType>
			 <xs:simpleType name="lenderived">
			   <xs:restriction base="tns:lenbase">
			     <xs:length value="4"/>
			   </xs:restriction>
			 </xs:simpleType>`,
			"length-valid-restriction",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:facets", c.body))
			if err == nil {
				t.Fatalf("Produce accepted an invalid restriction, want %s", c.wantRule)
			}
			got, ok := xsderr.RuleOf(err)
			if !ok || got != c.wantRule {
				t.Fatalf("rule = %q (ok=%v), want %q; err=%v", got, ok, c.wantRule, err)
			}
		})
	}
}

// TestProduceSpecialBaseRejected pins #480: neither ·special· datatype may be
// named as the {base type definition} of a user-defined simple type
// (xmlschema11-2.md §2.4.2 "No ·user-defined· datatype may have anyAtomicType as
// its ·base type·", Structures §3.16.1's parallel statement for both). Both
// rejections are st-props-correct clause 1 against the Datatypes §4.1.1 property
// tableau, reached through the {variety} the XML mapping copies off the base
// (§3.16.2.1): xs:anySimpleType's is ·absent·, which only xs:anySimpleType may
// be, and xs:anyAtomicType's carries an ·absent· {primitive type definition},
// which only xs:anyAtomicType may have. Both fire with NO facet present — the
// pre-existing facet-applicability path (cos-st-restricts clause 1.3.1) only saw
// a schema that named one, which is why three of these four cases were accepted
// before.
func TestProduceSpecialBaseRejected(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
	}{
		{"anyAtomicType, no facets",
			`<xs:simpleType name="t"><xs:restriction base="xs:anyAtomicType"/></xs:simpleType>`},
		{"anyAtomicType, facet present",
			`<xs:simpleType name="t"><xs:restriction base="xs:anyAtomicType"><xs:pattern value="a"/></xs:restriction></xs:simpleType>`},
		{"anySimpleType, no facets",
			`<xs:simpleType name="t"><xs:restriction base="xs:anySimpleType"/></xs:simpleType>`},
		{"anySimpleType, facet present",
			`<xs:simpleType name="t"><xs:restriction base="xs:anySimpleType"><xs:pattern value="a"/></xs:restriction></xs:simpleType>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := produce(t, wrap("urn:special", c.body))
			assertRule(t, err, xsderr.Rule("st-props-correct"))
		})
	}
}

// TestProduceSpecialTypesStayUsable is the false-reject guard for #480's
// rejection: it is scoped to the {base type definition} of a ·restriction·, so
// the built-in primitives — which legitimately DO have xs:anyAtomicType as their
// base (xmlschema11-2.md §2.4.2) — must still seed and resolve, a restriction of
// one must still produce, and naming either ·special· type where a type is merely
// REFERENCED (an element's type=) must stay legal.
func TestProduceSpecialTypesStayUsable(t *testing.T) {
	s, err := produce(t, wrap("urn:special",
		`<xs:simpleType name="short"><xs:restriction base="xs:string"><xs:maxLength value="4"/></xs:restriction></xs:simpleType>`+
			`<xs:element name="a" type="xs:anyAtomicType"/>`+
			`<xs:element name="b" type="xs:anySimpleType"/>`))
	if err != nil {
		t.Fatalf("Produce rejected a schema that only REFERENCES the special types: %v", err)
	}
	prim, ok := s.Type(xsd.QName{Space: xsdNS, Local: "string"})
	if !ok {
		t.Fatal("xs:string is not in {type definitions}: the primitives must still seed")
	}
	st, ok := prim.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("xs:string = %T, want *xsd.SimpleType", prim)
	}
	if !st.IsPrimitive() {
		t.Error("xs:string.IsPrimitive() = false, want true: a primitive's base is still xs:anyAtomicType")
	}
}

// TestProduceAcceptsNarrowingRestriction is the false-reject guard for the same
// seam: a legitimately narrowing restriction of a builtin must still produce.
func TestProduceAcceptsNarrowingRestriction(t *testing.T) {
	_, err := produce(t, wrap("urn:facets", `<xs:simpleType name="small">
	   <xs:restriction base="xs:byte">
	     <xs:minInclusive value="0"/>
	     <xs:maxInclusive value="100"/>
	   </xs:restriction>
	 </xs:simpleType>`))
	if err != nil {
		t.Fatalf("narrowing restriction rejected: %v", err)
	}
}

// TestProduceScaleFacetsReachTheFacetSet pins xr-maxScale/xr-minScale
// (xsd-precisionDecimal.md §4.2.2/§4.3.2): the two precisionDecimal extension
// facets are plain-lexical value=/fixed= facets, so <maxScale>/<minScale>
// children must land in {facets} carrying both properties. Before this, they hit
// facetKindOf's unknown-name branch and were silently dropped, which also left
// every scale Schema Component Constraint unreachable from a parsed schema.
func TestProduceScaleFacetsReachTheFacetSet(t *testing.T) {
	st, _ := simpleTypeOf(t, "Scaled", `<xs:simpleType name="Scaled">
	   <xs:restriction base="xs:precisionDecimal">
	     <xs:maxScale value="4" fixed="true"/>
	     <xs:minScale value="2"/>
	   </xs:restriction>
	 </xs:simpleType>`)
	want := []struct {
		kind  xsd.FacetKind
		value string
		fixed bool
	}{
		{xsd.FacetMaxScale, "4", true},
		{xsd.FacetMinScale, "2", false},
	}
	facets := st.OwnFacets()
	if len(facets) != len(want) {
		t.Fatalf("own facets = %v, want %d (maxScale, minScale)", facets, len(want))
	}
	for i, w := range want {
		got := facets[i]
		if got.Kind() != w.kind {
			t.Errorf("facet %d kind = %v, want %v", i, got.Kind(), w.kind)
		}
		if vs := got.Values(); len(vs) != 1 || vs[0] != w.value {
			t.Errorf("facet %d {value} = %v, want [%q]", i, vs, w.value)
		}
		fixed, ok := got.Fixed()
		if !ok {
			t.Errorf("facet %d reports no {fixed}, but %v carries one", i, w.kind)
		}
		if fixed != w.fixed {
			t.Errorf("facet %d {fixed} = %v, want %v", i, fixed, w.fixed)
		}
	}
}

// TestProduceChargesScaleRestriction is the end-to-end proof that the
// construction-time scale SCCs (xsd/derivation.go, issue #157) now see parsed
// input: a derived maxScale WIDER than the base's violates
// maxScale-valid-restriction (xsd-precisionDecimal.md §4.2.4) and must be
// rejected at produce time. While the facet was dropped in the producer this
// schema was silently accepted.
func TestProduceChargesScaleRestriction(t *testing.T) {
	_, err := produce(t, wrap("urn:facets", `<xs:simpleType name="tight">
	   <xs:restriction base="xs:precisionDecimal">
	     <xs:maxScale value="2"/>
	   </xs:restriction>
	 </xs:simpleType>
	 <xs:simpleType name="wider">
	   <xs:restriction base="tns:tight">
	     <xs:maxScale value="5"/>
	   </xs:restriction>
	 </xs:simpleType>`))
	if err == nil {
		t.Fatalf("Produce accepted a widening maxScale, want maxScale-valid-restriction")
	}
	got, ok := xsderr.RuleOf(err)
	if !ok || got != xsderr.Rule("maxScale-valid-restriction") {
		t.Fatalf("rule = %q (ok=%v), want %q; err=%v", got, ok, "maxScale-valid-restriction", err)
	}
}

// fixedFacetSchemas renders one single-facet <simpleType> per {fixed}-bearing
// facet family the shared reader serves — a Part-2 facet (totalDigits, §4.3.11)
// and a precisionDecimal scale facet (maxScale, xsd-precisionDecimal.md §4.2) —
// with attrs spliced verbatim into the facet's start tag. The facet element sits
// on line 3 of the document so a charge's position can be pinned. The result is a
// slice, not a map: subtest order is output (STYLE D2).
func fixedFacetSchemas(attrs string) []struct{ facet, body string } {
	schema := func(base, facet string) string {
		return "\n" + `<xs:simpleType name="st"><xs:restriction base="` + base + `">` + "\n" +
			`<xs:` + facet + ` value="4"` + attrs + "/>\n</xs:restriction></xs:simpleType>"
	}
	return []struct{ facet, body string }{
		{"totalDigits", schema("xs:decimal", "totalDigits")},
		{"maxScale", schema("xs:precisionDecimal", "maxScale")},
	}
}

// TestProduceFacetFixedActualValue pins the {fixed} property mapping shared by
// all thirteen {fixed}-bearing facets: {fixed} is "the actual value of the fixed
// [attribute], if present, otherwise false" (xsd-precisionDecimal.md §4.2.2,
// §4.3.2; Datatypes §4.3.x). "Actual value" is the xs:boolean VALUE, so the
// pre-lexical whiteSpace = collapse xs:boolean fixes (§3.3.2.3, §4.3.6) runs
// before the four-literal lexical space booleanRep (§3.3.2.2) is tested: a padded
// " true " is the value true, not a non-literal. The producer compared the RAW
// attribute string against two literals, so every padded form silently became
// false — and on a base maxScale that silently disabled f-ms-fixed.
func TestProduceFacetFixedActualValue(t *testing.T) {
	cases := []struct {
		name  string
		attrs string
		want  bool
	}{
		{"absent", "", false},
		{"true", ` fixed="true"`, true},
		{"one", ` fixed="1"`, true},
		{"false", ` fixed="false"`, false},
		{"zero", ` fixed="0"`, false},
		{"space padded true", ` fixed=" true "`, true},
		{"tab and newline padded true", ` fixed="&#x9;true&#xA;"`, true},
		{"padded zero", ` fixed="  0  "`, false},
	}
	for _, c := range cases {
		for _, s := range fixedFacetSchemas(c.attrs) {
			t.Run(s.facet+"/"+c.name, func(t *testing.T) {
				st, _ := simpleTypeOf(t, "st", s.body)
				facets := st.OwnFacets()
				if len(facets) != 1 {
					t.Fatalf("own facets = %v, want exactly one <%s>", facets, s.facet)
				}
				fixed, ok := facets[0].Fixed()
				if !ok {
					t.Fatalf("%s reports no {fixed}, but it is a {fixed}-bearing facet", s.facet)
				}
				if fixed != c.want {
					t.Errorf("fixed=%q → {fixed} = %v, want %v", c.attrs, fixed, c.want)
				}
			})
		}
	}
}

// TestProduceFacetFixedOutOfLexicalSpaceRejected is the other half of the same
// mapping: a fixed value outside xs:boolean's lexical space is Datatype Valid
// against nothing (§4.1.4 cvc-datatype-valid) and there is no clause letting it
// default — so it is a positioned rejection, not a silent {fixed} = false. Case
// matters ("TRUE" is not a booleanRep), and collapse never rescues an empty
// value. The &#xA0; case pins §4.3.6's whitespace class: NBSP is a legal XML Char
// that collapse PRESERVES, so " true" stays outside booleanRep even though
// Go's unicode.IsSpace (and hence strings.TrimSpace) would cut it.
func TestProduceFacetFixedOutOfLexicalSpaceRejected(t *testing.T) {
	for _, lexical := range []string{"yes", "TRUE", "True", "", "  ", "2", "true false", "&#xA0;true"} {
		for _, s := range fixedFacetSchemas(` fixed="` + lexical + `"`) {
			t.Run(s.facet+"/"+lexical, func(t *testing.T) {
				_, err := produce(t, wrap("", s.body))
				assertRule(t, err, xsderr.Rule("cvc-datatype-valid"))
				loc, ok := xsderr.LocOf(err)
				if !ok {
					t.Fatalf("error %v carries no position", err)
				}
				if loc.URI != produceURI || loc.Line != 3 {
					t.Fatalf("position = %s:%d:%d, want the facet element's own line 3 of %s", loc.URI, loc.Line, loc.Col, produceURI)
				}
			})
		}
	}
}

// inlineSimpleType asserts that a {type definition} slot is the InlineTypeDefinition
// arm and returns the anonymous simple type it owns (#229).
func inlineSimpleType(t *testing.T, ref xsd.TypeDefinitionOrRef) *xsd.SimpleType {
	t.Helper()
	inline, ok := ref.(xsd.InlineTypeDefinition)
	if !ok {
		t.Fatalf("{type definition} = %#v, want an xsd.InlineTypeDefinition", ref)
	}
	st, ok := inline.Definition.(*xsd.SimpleType)
	if !ok {
		t.Fatalf("inline {type definition} is %T, want *xsd.SimpleType", inline.Definition)
	}
	if st.Name() != (xsd.QName{}) {
		t.Fatalf("inline {type definition} {name} = %s, want the absent (zero) QName", st.Name())
	}
	return st
}

// TestProduceLocalElementInlineSimpleType is §3.3.2.1 dcl.elt.common clause 1 for
// a LOCAL element: the <simpleType> child maps to the anonymous type that becomes
// the declaration's {type definition}, reachable only through the declaration
// (it is in no symbol table).
func TestProduceLocalElementInlineSimpleType(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence>` +
		`<xs:element name="code"><xs:simpleType><xs:restriction base="xs:string">` +
		`<xs:maxLength value="4"/></xs:restriction></xs:simpleType></xs:element>` +
		`</xs:sequence></xs:complexType>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Local: "CT"})
	if !ok {
		t.Fatalf("complexType CT not found")
	}
	mg := topGroup(t, td.(xsd.ComplexType))
	ed := mg.Particles()[0].Term().(xsd.ResolvedTerm).Term.(xsd.ElementDeclaration)
	if ed.Name() != (xsd.QName{Local: "code"}) {
		t.Fatalf("local element = %s, want {}code", ed.Name())
	}
	st := inlineSimpleType(t, ed.TypeDefinition())
	if base := mustBase(t, s, st); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "string"}) {
		t.Fatalf("inline type base = %v, want {xs}string", base)
	}
	if got := st.OwnFacets(); len(got) != 1 || got[0].Kind() != xsd.FacetMaxLength {
		t.Fatalf("inline type own facets = %v, want one maxLength", got)
	}
	// The anonymous type is NOT registered in {type definitions}: it has no name
	// to be resolved by.
	if _, ok := s.Type(xsd.QName{}); ok {
		t.Fatalf("the anonymous inline type was registered in {type definitions}")
	}
}

// TestProduceLocalAttributeInlineSimpleType is the §3.2.2.2 dcl.att.local half:
// the first tier of the attribute's three-tier {type definition} chain.
func TestProduceLocalAttributeInlineSimpleType(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:int">` +
		`<xs:minInclusive value="1"/></xs:restriction></xs:simpleType></xs:attribute>` +
		`</xs:complexType>`
	ct, s := complexTypeIn(t, body, "CT")
	uses := ct.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("attribute uses = %d, want 1", len(uses))
	}
	decl := uses[0].AttributeDeclaration().(xsd.LocalAttributeDeclaration).Declaration
	if decl.ScopeVariety() != xsd.ScopeLocal {
		t.Fatalf("decl scope = %s, want local", decl.ScopeVariety())
	}
	st := inlineSimpleType(t, decl.TypeDefinition())
	if base := mustBase(t, s, st); base == nil || base.Name() != (xsd.QName{Space: xsdNS, Local: "int"}) {
		t.Fatalf("inline type base = %v, want {xs}int", base)
	}
}

// TestProduceLocalElementTypeAndInlineRejected is src-element clause 3 (§3.3.3)
// on the LOCAL path: a type= attribute and an inline <simpleType> child together
// are a schema-representation violation, not a case where type= silently wins.
func TestProduceLocalElementTypeAndInlineRejected(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence>` +
		`<xs:element name="e" type="xs:string"><xs:simpleType>` +
		`<xs:restriction base="xs:string"/></xs:simpleType></xs:element>` +
		`</xs:sequence></xs:complexType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-element")
}

// TestProduceLocalAttributeTypeAndInlineRejected is the src-attribute clause 4
// (§3.2.3) half of the same rule.
func TestProduceLocalAttributeTypeAndInlineRejected(t *testing.T) {
	body := `<xs:complexType name="CT"><xs:sequence/>` +
		`<xs:attribute name="a" type="xs:string"><xs:simpleType>` +
		`<xs:restriction base="xs:string"/></xs:simpleType></xs:attribute>` +
		`</xs:complexType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-attribute")
}

// TestProduceElementSubstitutionGroupHeads pins §3.3.2.1's {substitution group
// affiliations} mapping: substitutionGroup is `List of QName` (§3.3.2), so EVERY
// item is ·resolved· and carried, in the list's own lexical order (STYLE D2). The
// two heads are named through different prefixes — the target namespace's own
// prefix and the default binding — so the assertion also pins that each item goes
// through the in-scope-bindings resolver rather than being defaulted wholesale to
// the target namespace.
func TestProduceElementSubstitutionGroupHeads(t *testing.T) {
	body := `<xs:element name="h1" type="xs:string"/>` +
		`<xs:element name="h2" type="xs:string"/>` +
		`<xs:element name="member" type="xs:string" substitutionGroup="tns:h1 h2"/>`
	s, err := produce(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns="urn:x"`+
		` targetNamespace="urn:x" xmlns:tns="urn:x">`+body+`</xs:schema>`)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:x", Local: "member"})
	if !ok {
		t.Fatalf("element {urn:x}member not found")
	}
	want := []xsd.QName{{Space: "urn:x", Local: "h1"}, {Space: "urn:x", Local: "h2"}}
	if got := ed.SubstitutionGroupAffiliationNames(); !slices.Equal(got, want) {
		t.Fatalf("{substitution group affiliations} = %v, want %v in document order", got, want)
	}
}

// TestProduceElementSubstitutionGroupSingleHead is the one-item case, and pins
// that a bare <element> with no substitutionGroup gets the EMPTY set rather than
// a one-element slice holding the absent QName.
func TestProduceElementSubstitutionGroupSingleHead(t *testing.T) {
	body := `<xs:element name="head" type="xs:string"/>` +
		`<xs:element name="member" type="xs:string" substitutionGroup="head"/>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	member, ok := s.Element(xsd.QName{Local: "member"})
	if !ok {
		t.Fatalf("element member not found")
	}
	want := []xsd.QName{{Local: "head"}}
	if got := member.SubstitutionGroupAffiliationNames(); !slices.Equal(got, want) {
		t.Fatalf("{substitution group affiliations} = %v, want %v", got, want)
	}
	head, ok := s.Element(xsd.QName{Local: "head"})
	if !ok {
		t.Fatalf("element head not found")
	}
	if got := head.SubstitutionGroupAffiliationNames(); len(got) != 0 {
		t.Fatalf("head {substitution group affiliations} = %v, want the empty set", got)
	}
}

// TestProduceElementEmptySubstitutionGroupAccepted pins that an EMPTY
// substitutionGroup on a top-level <element> (elemE007) is accepted and maps to
// the empty set, so no later session files a rejection for it. The attribute's
// declared type is an anonymous `<xs:list itemType="xs:QName"/>` with no
// minLength facet (§A, xmlschema11-1.md:5068-5072) and a list type's value space
// is "the set of finite-length sequences of zero or more atomic values"
// (xmlschema11-2.md:333), so "" is a well-formed zero-length list rather than a
// QName-lexical fault, and §3.3.2.1's {substitution group affiliations} has no
// item to ·resolve· (:1144). xs:localElement prohibits the attribute outright
// (:5124), which is why the declaration here is a <schema> child.
func TestProduceElementEmptySubstitutionGroupAccepted(t *testing.T) {
	body := `<xs:element name="foo" type="xs:string" abstract="true"/>` +
		`<xs:element name="myElem" type="xs:string" substitutionGroup=""/>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v, want an empty substitutionGroup accepted as a zero-length list", err)
	}
	member, ok := s.Element(xsd.QName{Local: "myElem"})
	if !ok {
		t.Fatalf("element myElem not found")
	}
	if got := member.SubstitutionGroupAffiliationNames(); len(got) != 0 {
		t.Fatalf("{substitution group affiliations} = %v, want the empty set", got)
	}
}

// TestProduceElementSubstitutionGroupUnknownHeadAccepted pins §5.3 (Missing
// Sub-components) for this one slot: a substitutionGroup naming no declaration
// resolves to ·absent·, which is NOT a schema-construction error — the schema
// stands and the affiliation simply carries a name nothing answers to. W3C
// saxonData/Missing missing002 is this case, expected valid.
func TestProduceElementSubstitutionGroupUnknownHeadAccepted(t *testing.T) {
	body := `<xs:element name="member" type="xs:string" substitutionGroup="tns:missing"/>`
	s, err := produce(t, wrap("urn:x", body))
	if err != nil {
		t.Fatalf("Produce rejected an unresolvable substitutionGroup head, but §5.3 makes it ·absent·, not an error: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:x", Local: "member"})
	if !ok {
		t.Fatalf("element {urn:x}member not found")
	}
	want := []xsd.QName{{Space: "urn:x", Local: "missing"}}
	if got := ed.SubstitutionGroupAffiliationNames(); !slices.Equal(got, want) {
		t.Fatalf("{substitution group affiliations} = %v, want the unresolved %v retained", got, want)
	}
	// The contrast that makes the exemption specific to this slot rather than a
	// blanket one: an unresolvable type= is still charged src-resolve at finalize.
	_, err = produce(t, wrap("urn:x", `<xs:element name="e" type="tns:missing"/>`))
	assertRule(t, err, "src-resolve")
}

// TestProduceElementSubstitutionGroupWrongKindHeadAccepted pins the WRONG-KIND
// half of the same §5.3 exemption: a substitutionGroup naming a top-level
// <simpleType> is accepted exactly as one naming nothing is. src-resolve
// (§3.17.6.2) condition 1 is kind-parameterized — the candidate must be "a member
// of the value of the appropriate property" — so a name answered by a
// <simpleType> and a name answered by nothing fail it identically, and the rule
// carries no clause that reads differently for the two. Both are ·absent· under
// §5.3, which e-props-correct clause 1 (§3.3.6.1) defers to by reading "modulo
// the impact of Missing Sub-components (§5.3)".
//
// The untyped case is the one that catches a mis-fix resolving the head out of
// the TYPE index rather than the element index: the member would then inherit
// {urn:x}foo, the simple type itself, instead of falling through to xs:anyType
// under §3.3.2.1 dcl.elt.common clause 4.
//
// The head is spelled with a <restriction> child rather than as a bare
// <xs:simpleType name="foo"/>, which §3.16.2.1 rejects on its own account: the
// acceptance asserted here has to be attributable to the substitutionGroup slot.
func TestProduceElementSubstitutionGroupWrongKindHeadAccepted(t *testing.T) {
	const head = `<xs:simpleType name="foo"><xs:restriction base="xs:string"/></xs:simpleType>`
	want := []xsd.QName{{Space: "urn:x", Local: "foo"}}

	s, err := produce(t, wrap("urn:x", head+`<xs:element name="member" type="xs:string" substitutionGroup="tns:foo"/>`))
	if err != nil {
		t.Fatalf("Produce rejected a substitutionGroup naming a simple type, but src-resolve cannot tell that from naming nothing and §5.3 makes both ·absent·: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Space: "urn:x", Local: "member"})
	if !ok {
		t.Fatalf("element {urn:x}member not found")
	}
	if got := ed.SubstitutionGroupAffiliationNames(); !slices.Equal(got, want) {
		t.Fatalf("{substitution group affiliations} = %v, want the unresolved %v retained", got, want)
	}

	s, err = produce(t, wrap("urn:x", head+`<xs:element name="member" substitutionGroup="tns:foo"/>`))
	if err != nil {
		t.Fatalf("Produce: %v, want an untyped member with a wrong-kind head accepted", err)
	}
	ed, ok = s.Element(xsd.QName{Space: "urn:x", Local: "member"})
	if !ok {
		t.Fatalf("element {urn:x}member not found")
	}
	if got := ed.SubstitutionGroupAffiliationNames(); !slices.Equal(got, want) {
		t.Fatalf("{substitution group affiliations} = %v, want the unresolved %v retained", got, want)
	}
	if got := declaredTypeName(t, ed.TypeDefinition()); got != anyTypeQN {
		t.Fatalf("{type definition} = %s, want %s — the head resolves out of the ELEMENT index, so the simple type named foo supplies nothing", got, anyTypeQN)
	}

	// The exemption is this one slot's, not a blanket one: the very same schema
	// that excuses the wrong-kind head still charges a dangling type=.
	_, err = produce(t, wrap("urn:x", head+
		`<xs:element name="member" substitutionGroup="tns:foo"/>`+
		`<xs:element name="e" type="tns:nosuch"/>`))
	assertRule(t, err, "src-resolve")
}

// TestProduceElementSubstitutionGroupBadPrefixRejected pins the half the producer
// DOES decide: an item whose prefix has no in-scope binding cannot be mapped to a
// QName value at all, so it is charged src-resolve here, at the referring
// element's own position.
func TestProduceElementSubstitutionGroupBadPrefixRejected(t *testing.T) {
	body := `<xs:element name="head" type="xs:string"/>` +
		`<xs:element name="member" type="xs:string" substitutionGroup="head nope:other"/>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "src-resolve")
}

// TestProduceElementSubstitutionGroupCircularRejected proves the whole pipe is
// live end to end: with real affiliation edges flowing, finalize's Phase B
// circularity check (e-props-correct clause 5) has a graph to walk, which it
// never did while every producer passed a nil {substitution group affiliations}.
func TestProduceElementSubstitutionGroupCircularRejected(t *testing.T) {
	body := `<xs:element name="a" type="xs:string" substitutionGroup="b"/>` +
		`<xs:element name="b" type="xs:string" substitutionGroup="a"/>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "e-props-correct")
}

// TestProduceElementBlockMapped pins §3.3.2.1's {disallowed substitutions} row:
// the ·effective block value· is block=, else the <schema>'s blockDefault, else
// the empty string; "#all" names all three keywords; any other value names the
// keywords its list contains (a token outside xs:blockSet is rejected, pinned in
// produce_derivationset_test.go). The result is in the spec's canonical order
// whatever order the attribute spells it in, so one set has one encoding.
func TestProduceElementBlockMapped(t *testing.T) {
	all := []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction, xsd.DerivationSubstitution}
	for _, tc := range []struct {
		name         string
		blockDefault string
		block        string
		want         []xsd.DerivationMethod
	}{
		{name: "absent is the empty set"},
		{name: "empty block is the empty set", block: `block=""`},
		{name: "#all", block: `block="#all"`, want: all},
		{name: "one keyword", block: `block="substitution"`, want: []xsd.DerivationMethod{xsd.DerivationSubstitution}},
		{name: "canonical order not lexical", block: `block="substitution extension"`,
			want: []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationSubstitution}},
		{name: "blockDefault fallback", blockDefault: ` blockDefault="restriction"`,
			want: []xsd.DerivationMethod{xsd.DerivationRestriction}},
		{name: "block overrides blockDefault", blockDefault: ` blockDefault="#all"`, block: `block="extension"`,
			want: []xsd.DerivationMethod{xsd.DerivationExtension}},
		// An EMPTY block= is present, so it wins over blockDefault and maps to the
		// empty set — the case that would be lost by treating "" as absent.
		{name: "empty block overrides blockDefault", blockDefault: ` blockDefault="#all"`, block: `block=""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"` + tc.blockDefault + `>` +
				`<xs:element name="e" type="xs:string" ` + tc.block + `/></xs:schema>`
			s, err := produce(t, doc)
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			ed, ok := s.Element(xsd.QName{Local: "e"})
			if !ok {
				t.Fatalf("element e not found")
			}
			if got := ed.DisallowedSubstitutions(); !slices.Equal(got, tc.want) {
				t.Fatalf("{disallowed substitutions} = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestProduceElementBlockSubstitutionNarrowsGroup is why {disallowed
// substitutions} is mapped in the same slice as {substitution group
// affiliations}: cos-equiv-derived-ok-rec clause 2.1 reads it, so with it
// unmapped a blocking head would still admit its members and two names that do
// NOT ·overlap· would be charged cos-nonambig. It is the shape of W3C MS-Element
// elemZ028a, reduced to two declarations.
func TestProduceElementBlockSubstitutionNarrowsGroup(t *testing.T) {
	schema := func(block string) string {
		return `<xs:element name="head" type="xs:anyType" ` + block + `/>` +
			`<xs:element name="member" type="xs:anyType" substitutionGroup="head"/>` +
			// minOccurs="0" on the first makes both particles live at the start
			// state, so the two ·compete· exactly when they ·overlap·.
			`<xs:complexType name="CT"><xs:sequence>` +
			`<xs:element ref="member" minOccurs="0"/><xs:element ref="head"/>` +
			`</xs:sequence></xs:complexType>`
	}
	if _, err := produce(t, wrap("", schema(`block="substitution"`))); err != nil {
		t.Fatalf("head blocks substitution, so member is in no group of head and the two do not ·overlap·: %v", err)
	}
	// The control: drop the block and the two genuinely do overlap.
	_, err := produce(t, wrap("", schema("")))
	assertRule(t, err, "cos-nonambig")
}

// TestProduceElementFinalMapped pins §3.3.2.1's {substitution group exclusions}
// row, which states itself entirely by reference: "As for {disallowed
// substitutions} above, but using the final and finalDefault [attributes] in
// place of the block and blockDefault [attributes] and with the relevant set
// being {extension, restriction}". So it is the same ·effective value· case
// analysis over a different attribute pair, and the one thing that is NOT the
// same is the size of the set — "#all" is TWO keywords here, and substitution is
// not one of them.
func TestProduceElementFinalMapped(t *testing.T) {
	all := []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction}
	for _, tc := range []struct {
		name         string
		finalDefault string
		final        string
		want         []xsd.DerivationMethod
	}{
		{name: "absent is the empty set"},
		{name: "empty final is the empty set", final: `final=""`},
		{name: "#all is two keywords, not three", final: `final="#all"`, want: all},
		{name: "one keyword", final: `final="restriction"`, want: []xsd.DerivationMethod{xsd.DerivationRestriction}},
		{name: "canonical order not lexical", final: `final="restriction extension"`, want: all},
		{name: "the same set spelled the other way", final: `final="extension restriction"`, want: all},
		// finalDefault is xs:fullDerivationSet, so list and union are valid there
		// and name no member of this set: they are dropped, never rejected.
		{name: "finalDefault list and union ignored", finalDefault: ` finalDefault="list union restriction"`,
			want: []xsd.DerivationMethod{xsd.DerivationRestriction}},
		{name: "finalDefault fallback", finalDefault: ` finalDefault="extension"`,
			want: []xsd.DerivationMethod{xsd.DerivationExtension}},
		{name: "final overrides finalDefault", finalDefault: ` finalDefault="#all"`, final: `final="restriction"`,
			want: []xsd.DerivationMethod{xsd.DerivationRestriction}},
		// An EMPTY final= is PRESENT, so it is the ·effective value· and wins over
		// finalDefault — the case lost by treating "" as absent.
		{name: "empty final overrides finalDefault", finalDefault: ` finalDefault="#all"`, final: `final=""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"` + tc.finalDefault + `>` +
				`<xs:element name="e" type="xs:string" ` + tc.final + `/></xs:schema>`
			s, err := produce(t, doc)
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			ed, ok := s.Element(xsd.QName{Local: "e"})
			if !ok {
				t.Fatalf("element e not found")
			}
			if got := ed.SubstitutionGroupExclusions(); !slices.Equal(got, tc.want) {
				t.Fatalf("{substitution group exclusions} = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestProduceElementFinalBlocksSubstitution is why the mapping above exists at
// all: {substitution group exclusions} is read by exactly one rule,
// e-props-correct clause 4 (c-vs-sg), and with the property left universally
// empty that rule would admit every member of every head whatever final= says.
// The two schemas differ only in the head's final=.
func TestProduceElementFinalBlocksSubstitution(t *testing.T) {
	schema := func(final string) string {
		return `<xs:complexType name="H"><xs:sequence/></xs:complexType>` +
			`<xs:complexType name="M"><xs:complexContent><xs:restriction base="H">` +
			`<xs:sequence/></xs:restriction></xs:complexContent></xs:complexType>` +
			`<xs:element name="head" type="H" ` + final + `/>` +
			`<xs:element name="member" type="M" substitutionGroup="head"/>`
	}
	if _, err := produce(t, wrap("", schema(""))); err != nil {
		t.Fatalf("an unblocked restriction-derived member was rejected: %v", err)
	}
	_, err := produce(t, wrap("", schema(`final="restriction"`)))
	assertRule(t, err, "e-props-correct")
}

// TestProduceElementNillableAndAbstractMapped pins §3.3.2.1 dcl.elt.common's
// {nillable} and {abstract} rows — "the ·actual value· of the [attribute], if
// present, otherwise false" — on the TOP-LEVEL form, whose two construction
// calls (the plain one and the inline-<complexType> one) both used to pass a
// hardcoded false and so violated e-props-correct clause 1 (#761). cvc-elt
// clause 2 reads {abstract} and clause 3 reads {nillable}.
func TestProduceElementNillableAndAbstractMapped(t *testing.T) {
	for _, tc := range []struct {
		name         string
		attrs        string
		wantNillable bool
		wantAbstract bool
	}{
		{name: "absent is false"},
		{name: "nillable true", attrs: ` nillable="true"`, wantNillable: true},
		{name: "abstract true", attrs: ` abstract="true"`, wantAbstract: true},
		{name: "both true", attrs: ` nillable="true" abstract="true"`, wantNillable: true, wantAbstract: true},
		// xs:boolean's other lexical spelling of each value, which boolAttr must
		// read as the ·actual value· and not as the literal string.
		{name: "the 1/0 spelling", attrs: ` nillable="1" abstract="0"`, wantNillable: true},
		{name: "explicit false", attrs: ` nillable="false" abstract="false"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Both arms in one schema: "byName" takes the plain construction call,
			// "inline" the one that also owns its anonymous complex type.
			s, err := produce(t, wrap("", `<xs:element name="byName" type="xs:string"`+tc.attrs+`/>`+
				`<xs:element name="inline"`+tc.attrs+`><xs:complexType/></xs:element>`))
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			for _, name := range []string{"byName", "inline"} {
				ed, ok := s.Element(xsd.QName{Local: name})
				if !ok {
					t.Fatalf("element %s not found", name)
				}
				if got := ed.Nillable(); got != tc.wantNillable {
					t.Errorf("%s {nillable} = %t, want %t", name, got, tc.wantNillable)
				}
				if got := ed.Abstract(); got != tc.wantAbstract {
					t.Errorf("%s {abstract} = %t, want %t", name, got, tc.wantAbstract)
				}
			}
		})
	}
}

// TestProduceLocalElementNillableMapped pins the local half of the same
// {nillable} row — nillable is legal on xs:localElement, unlike abstract, which
// the schema for schema documents gives use="prohibited" there (§A) so no local
// declaration can carry it.
func TestProduceLocalElementNillableMapped(t *testing.T) {
	s, err := produce(t, wrap("", `<xs:complexType name="CT"><xs:sequence>`+
		`<xs:element name="e" type="xs:string" nillable="true"/>`+
		`</xs:sequence></xs:complexType>`))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Local: "CT"})
	if !ok {
		t.Fatalf("complexType CT not found")
	}
	local, ok := topGroup(t, td.(xsd.ComplexType)).Particles()[0].Term().(xsd.ResolvedTerm).Term.(xsd.ElementDeclaration)
	if !ok {
		t.Fatalf("first particle {term} is not a local element declaration")
	}
	if !local.Nillable() {
		t.Errorf("local {nillable} = false, want true")
	}
	if local.Abstract() {
		t.Errorf("local {abstract} = true, want false — the attribute is prohibited on xs:localElement")
	}
}

// TestProduceElementLocalFinalNotMapped pins the deliberate confinement of the
// mapping to the TOP-LEVEL form: a local <element> carries no final attribute
// (§3.3.2 gives it to the top-level form alone), so only finalDefault could
// reach it, and the property is unreadable on a local declaration — clause 4
// reads it on a substitution group HEAD, and an affiliation names a top-level
// declaration. Mapping it there would be a fact with no reader.
func TestProduceElementLocalFinalNotMapped(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" finalDefault="#all">` +
		`<xs:complexType name="CT"><xs:sequence>` +
		`<xs:element name="e" type="xs:string"/>` +
		`</xs:sequence></xs:complexType></xs:schema>`
	s, err := produce(t, doc)
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	td, ok := s.Type(xsd.QName{Local: "CT"})
	if !ok {
		t.Fatalf("complexType CT not found")
	}
	local, ok := topGroup(t, td.(xsd.ComplexType)).Particles()[0].Term().(xsd.ResolvedTerm).Term.(xsd.ElementDeclaration)
	if !ok {
		t.Fatalf("first particle {term} is not a local element declaration")
	}
	if got := local.SubstitutionGroupExclusions(); got != nil {
		t.Fatalf("local declaration {substitution group exclusions} = %v, want empty", got)
	}
}

// TestProduceElementTypeFromSubstitutionGroupHead pins §3.3.2.1's {type
// definition} clause 3 — "the declared {type definition} of the Element
// Declaration ·resolved· to by the FIRST QName in the ·actual value· of the
// substitutionGroup attribute" — and its place in the four-tier chain: an inline
// child and type= both outrank it, and it outranks clause 4's xs:anyType.
func TestProduceElementTypeFromSubstitutionGroupHead(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want xsd.QName
	}{
		{
			name: "inherited from the head",
			body: `<xs:element name="head" type="xs:string"/>` +
				`<xs:element name="member" substitutionGroup="head"/>`,
			want: xsd.QName{Space: xsdNS, Local: "string"},
		},
		{
			// §3.1.3 allows forward reference, and the pre-scan is what makes it
			// resolve: the head is declared AFTER the member here.
			name: "head declared later",
			body: `<xs:element name="member" substitutionGroup="head"/>` +
				`<xs:element name="head" type="xs:string"/>`,
			want: xsd.QName{Space: xsdNS, Local: "string"},
		},
		{
			// The head has no type= of its own, so clause 3 applies to IT too and
			// the declared type comes from further up the chain.
			name: "inherited transitively",
			body: `<xs:element name="top" type="xs:string"/>` +
				`<xs:element name="head" substitutionGroup="top"/>` +
				`<xs:element name="member" substitutionGroup="head"/>`,
			want: xsd.QName{Space: xsdNS, Local: "string"},
		},
		{
			// "the FIRST QName" is literal: every item feeds {substitution group
			// affiliations}, only the first feeds {type definition}. Both heads are
			// typed xs:anyType so the affiliation to the second still stands.
			name: "first affiliation only",
			body: `<xs:element name="h1" type="xs:anyType"/>` +
				`<xs:element name="h2" type="xs:anyType"/>` +
				`<xs:element name="member" substitutionGroup="h1 h2"/>`,
			want: anyTypeQN,
		},
		{
			// type= is clause 2 and outranks clause 3: the member is xs:string, not
			// the head's xs:anyType. (It must still satisfy clause 4, which it does
			// — every type is ·validly substitutable· for xs:anyType.)
			name: "type= outranks the head",
			body: `<xs:element name="head" type="xs:anyType"/>` +
				`<xs:element name="member" type="xs:string" substitutionGroup="head"/>`,
			want: xsd.QName{Space: xsdNS, Local: "string"},
		},
		{
			// A head naming no declaration is an ·absent· member under §5.3, so
			// there is no declared type to inherit and clause 4 applies.
			name: "absent head falls through to xs:anyType",
			body: `<xs:element name="member" substitutionGroup="nosuchhead"/>`,
			want: anyTypeQN,
		},
		{
			// The head is itself a clause-4 declaration, so the member inherits
			// exactly xs:anyType — not a fallback but the head's real type.
			name: "untyped head is xs:anyType",
			body: `<xs:element name="head"/>` +
				`<xs:element name="member" substitutionGroup="head"/>`,
			want: anyTypeQN,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := produce(t, wrap("", tc.body))
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			ed, ok := s.Element(xsd.QName{Local: "member"})
			if !ok {
				t.Fatalf("element member not found")
			}
			if got := declaredTypeName(t, ed.TypeDefinition()); got != tc.want {
				t.Fatalf("{type definition} = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestProduceElementInlineTypeOutranksSubstitutionGroupHead pins the top of the
// four-tier chain against clause 3: an inline <complexType> child is tier 1 and
// wins, so the member keeps its own anonymous type rather than inheriting the
// head's.
func TestProduceElementInlineTypeOutranksSubstitutionGroupHead(t *testing.T) {
	body := `<xs:element name="head" type="xs:anyType"/>` +
		`<xs:element name="member" substitutionGroup="head">` +
		`<xs:complexType><xs:sequence/></xs:complexType></xs:element>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "member"})
	if !ok {
		t.Fatalf("element member not found")
	}
	if _, inline := ed.TypeDefinition().(xsd.InlineTypeDefinition); !inline {
		t.Fatalf("{type definition} = %#v, want the member's own inline anonymous type", ed.TypeDefinition())
	}
}

// TestProduceElementTypeFromSubstitutionGroupHeadInline pins the clause-3 shape
// #342 closed: the head's own {type definition} is the anonymous type of its
// inline <complexType>, which the member's {type definition} must BE and not
// merely equal (§3.4.6.5's no-identity Note names this very case as one where
// component identity IS determined). The member's slot references the OWNING
// head — no name could reach the type, and no second declaration could own it —
// and the schema is accepted rather than declined.
//
// Two edges are pinned, direct and transitive, because the walk goes to the
// TERMINAL head: an intermediate head is skipped over, so both members name the
// same owner and neither slot is an owner-of-owner chain.
func TestProduceElementTypeFromSubstitutionGroupHeadInline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		member string
		want   xsd.QName
	}{
		{
			name: "direct",
			body: `<xs:element name="head"><xs:complexType><xs:sequence/></xs:complexType></xs:element>` +
				`<xs:element name="member" substitutionGroup="head"/>`,
			member: "member",
			want:   xsd.QName{Local: "head"},
		},
		{
			// The chain's middle link inherits the same anonymous type, so the
			// walk reports the terminal owner for BOTH declarations.
			name: "transitive, through a head that inherits too",
			body: `<xs:element name="top"><xs:complexType><xs:sequence/></xs:complexType></xs:element>` +
				`<xs:element name="head" substitutionGroup="top"/>` +
				`<xs:element name="member" substitutionGroup="head"/>`,
			member: "member",
			want:   xsd.QName{Local: "top"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := produce(t, wrap("", tc.body))
			if err != nil {
				t.Fatalf("Produce: %v", err)
			}
			ed, ok := s.Element(xsd.QName{Local: tc.member})
			if !ok {
				t.Fatalf("element %s not found", tc.member)
			}
			inherited, ok := ed.TypeDefinition().(xsd.SubstitutionGroupHeadTypeRef)
			if !ok {
				t.Fatalf("{type definition} = %#v, want an xsd.SubstitutionGroupHeadTypeRef", ed.TypeDefinition())
			}
			if inherited.Head != tc.want {
				t.Fatalf("{type definition} head = %s, want the owning head %s", inherited.Head, tc.want)
			}
			owner, ok := s.Element(tc.want)
			if !ok {
				t.Fatalf("owning head %s not found", tc.want)
			}
			if _, owns := owner.TypeDefinition().(xsd.InlineTypeDefinition); !owns {
				t.Fatalf("the named head's own {type definition} = %#v, want the inline anonymous type it owns", owner.TypeDefinition())
			}
		})
	}
}

// TestProduceElementTypeFromSubstitutionGroupHeadInlineSimple pins the clause-3
// half #442 closed alongside its own: a head typed by an inline <simpleType>
// yields the same xsd.SubstitutionGroupHeadTypeRef arm a <complexType> head
// yields, since the member's {type definition} must BE the head's anonymous
// component and no name reaches one. The arm was previously declined only because
// the head itself was unproduced.
func TestProduceElementTypeFromSubstitutionGroupHeadInlineSimple(t *testing.T) {
	body := `<xs:element name="head"><xs:simpleType><xs:restriction base="xs:string"/></xs:simpleType></xs:element>` +
		`<xs:element name="member" substitutionGroup="head"/>`
	s, err := produce(t, wrap("", body))
	if err != nil {
		t.Fatalf("Produce: %v", err)
	}
	ed, ok := s.Element(xsd.QName{Local: "member"})
	if !ok {
		t.Fatal("element member not found")
	}
	inherited, isHeadRef := ed.TypeDefinition().(xsd.SubstitutionGroupHeadTypeRef)
	if !isHeadRef {
		t.Fatalf("{type definition} = %#v, want an xsd.SubstitutionGroupHeadTypeRef", ed.TypeDefinition())
	}
	if inherited.Head != (xsd.QName{Local: "head"}) {
		t.Fatalf("{type definition} head = %s, want the owning head", inherited.Head)
	}
}

// TestProduceElementTypeFromSubstitutionGroupNonFirstHeadAnonymous is the
// residual #342 deliberately did NOT close, pinned so a later widening is a
// choice rather than an accident: with substitutionGroup="h1 h2", clause 3 reads
// h1 alone, so the member is typed by h1's NAMED type while h2 owns an anonymous
// one. The construction-identity shortcut in e-props-correct clause 4
// (sameAnonymousTypeByConstruction) must not admit that edge — the two types are
// genuinely different components and the ordinary anonymous rule rejects it.
func TestProduceElementTypeFromSubstitutionGroupNonFirstHeadAnonymous(t *testing.T) {
	body := `<xs:element name="h1" type="xs:anyType"/>` +
		`<xs:element name="h2"><xs:complexType><xs:sequence/></xs:complexType></xs:element>` +
		`<xs:element name="member" substitutionGroup="h1 h2"/>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "e-props-correct")
}

// TestProduceElementTypeFromCircularSubstitutionGroup pins that clause 3's walk
// TERMINATES on a circular affiliation chain rather than looping: the cycle is
// e-props-correct clause 5's to charge, at finalize, over the whole graph. It is
// the shape of W3C MS-Additional test111869.
func TestProduceElementTypeFromCircularSubstitutionGroup(t *testing.T) {
	body := `<xs:element name="e1" substitutionGroup="e2"/>` +
		`<xs:element name="e2" substitutionGroup="e1"/>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "e-props-correct")
}

// TestProduceLocalElementSubstitutionGroupRejected is e-props-correct clause 3
// (§3.3.6.1) on the LOCAL path: the attribute is use="prohibited" on
// xs:localElement (§3.3.2), and this producer runs no meta-schema validation pass
// ahead of mapping, so silently ignoring it would be a false accept. The charge
// is positioned on the local <element> itself.
func TestProduceLocalElementSubstitutionGroupRejected(t *testing.T) {
	body := `<xs:element name="head" type="xs:string"/>` +
		"\n" + `<xs:complexType name="CT"><xs:sequence>` +
		`<xs:element name="e" type="xs:string" substitutionGroup="head"/>` +
		`</xs:sequence></xs:complexType>`
	_, err := produce(t, wrap("", body))
	assertRule(t, err, "e-props-correct")
	loc, ok := xsderr.LocOf(err)
	if !ok {
		t.Fatalf("error %v carries no position", err)
	}
	if loc.URI != produceURI || loc.Line != 2 {
		t.Fatalf("position = %s:%d:%d, want the local <element>'s own line 2 of %s", loc.URI, loc.Line, loc.Col, produceURI)
	}
}

// TestProduceLocalElementProhibitedAttrRejected pins final and abstract on the
// INLINE local form: xs:localElement restricts both to use="prohibited"
// (xmlschema11-1.md:5125-:5126), this producer runs no meta-schema validation
// pass ahead of mapping, and before this charge both attributes were read by
// nothing here and the document was accepted outright.
//
// Carrying NO rule ID is the assertion that matters, and it is what separates
// this from TestProduceLocalElementSubstitutionGroupRejected next to it: the
// third attribute the same grammar line prohibits draws e-props-correct clause 3
// because the Common Mapping Rule for {substitution group affiliations} really
// would build the value that clause forbids at local scope, while §3.3.2.1 maps
// {abstract} and {substitution group exclusions} unconditionally and leaves no
// numbered clause for these two to violate. Charging one anyway would be a
// fabricated verdict (STYLE E2). The ref= form is pinned to src-element clause
// 2.2 instead, by TestProduceRefElementUseProhibitedAttrRejected.
//
// The last row asserts the walk is in the GRAMMAR's declaration order, not the
// document's (STYLE D2).
func TestProduceLocalElementProhibitedAttrRejected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		attrs    string
		wantAttr string
	}{
		{name: "final", attrs: `final="restriction"`, wantAttr: "final"},
		{name: "abstract", attrs: `abstract="true"`, wantAttr: "abstract"},
		{
			// abstract="false" is prohibited as squarely as abstract="true": the
			// grammar rejects the ATTRIBUTE, and never reads its value.
			name: "abstract=false", attrs: `abstract="false"`, wantAttr: "abstract",
		},
		{
			name:     "declaration order wins over document order",
			attrs:    `abstract="true" final="restriction"`,
			wantAttr: "final",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "\n" + `<xs:complexType name="CT"><xs:sequence>` +
				`<xs:element name="e" type="xs:string" ` + tc.attrs + `/>` +
				`</xs:sequence></xs:complexType>`
			_, err := produce(t, wrap("", body))
			if err == nil {
				t.Fatalf("Produce accepted a local <element> carrying %s, want the §5.1 grammar fault", tc.wantAttr)
			}
			want := "carries a " + tc.wantAttr + ` attribute, which the schema for schema documents prohibits on the local form: xs:localElement restricts ` + tc.wantAttr + ` to use="prohibited"`
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want it to state %q", err, want)
			}
			if rule, ok := xsderr.RuleOf(err); ok {
				t.Errorf("error = %v, charged %s; want a plain grammar fault carrying no rule ID", err, rule)
			}
			if at := produceURI + ":2:"; !strings.Contains(err.Error(), at) {
				t.Errorf("error = %v, want it to name the local <element> at %s", err, at)
			}
		})
	}
}

// TestProduceLocalElementProhibitedAttrAccepted is the other side: the
// prohibition is xs:localElement's alone, so the TOP-LEVEL form declares both
// attributes legitimately (xmlschema11-1.md:5086-:5107) and a check hoisted out
// of the local path would reject the whole language; and it reaches unqualified
// attributes only, since xs:localElement admits foreign ones outright
// (<anyAttribute namespace="##other"> at xmlschema11-1.md:5127).
func TestProduceLocalElementProhibitedAttrAccepted(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "final and abstract on the top-level form",
			body: `<xs:element name="top" type="xs:string" final="restriction" abstract="true"/>`,
		},
		{
			name: "foreign-namespace final and abstract on the local form",
			body: `<xs:complexType name="CT"><xs:sequence>` +
				`<xs:element xmlns:o="urn:other" name="e" type="xs:string" o:final="restriction" o:abstract="true"/>` +
				`</xs:sequence></xs:complexType>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := produce(t, wrap("", tc.body)); err != nil {
				t.Fatalf("Produce rejected an <element> the schema for schema documents admits: %v", err)
			}
		})
	}
}
