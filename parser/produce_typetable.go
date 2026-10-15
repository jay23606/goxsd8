package parser

import (
	"fmt"

	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file maps the <alternative> children of an <element> into the element
// declaration's {type table} (§3.3.2.1 dcl.elt.common over §3.12.2 declare-ta)
// and charges §3.12.3 src-ta, §3.3.3 src-element clause 5 and §3.12.6
// ta-props-correct clause 2 over the same children — behind §5.1's first bullet
// over each <alternative>'s own children (checkS4SChildOrder, s4sAlternative,
// produce_s4sorder.go). It serves the GLOBAL <element> path (produceElement)
// and both LOCAL ones (produceLocalElement) from one implementation (STYLE T4):
// §3.3.2.1's {type table} row is a COMMON mapping rule and §3.3.2.2
// dcl.elt.global supplements {target namespace} and {scope} alone, so a
// top-level <element> has nothing to map differently.

// ruleSrcTA is Type Alternative Representation OK (Structures §3.12.3,
// id="src-ta"): "each <alternative> element must have one (and only one) of the
// following: a type attribute, or a complexType child element, or a simpleType
// child element". The rule is a single unnumbered sentence, so messages cite no
// clause.
const ruleSrcTA xsderr.Rule = "src-ta"

// ruleTAPropsCorrect is Type Alternative Properties Correct (Structures §3.12.6,
// id="ta-props-correct"), whose clause 2 is "If the {test} is not absent, then
// it satisfies the constraint XPath Valid (§3.13.6.2)" — and xpath-valid clause
// 2 in turn is "X does not produce any static error".
//
// The rule ID is BARE, with no clause suffix: xsderr's catalog is generated from
// the specs and reserves exactly this ID, so a message names its clause inline
// (STYLE E4) instead.
const ruleTAPropsCorrect xsderr.Rule = "ta-props-correct"

// ctaStaticTypes is the ·in-scope schema definitions· of the static context
// xpath-valid clause 2.2.5 (§3.13.6.2) fixes for the {test} whose static errors
// this file charges: "those components that are present in every schema by
// definition", clause 2.2.5's own three sources (Built-in Attribute
// Declarations §3.2.7, the Built-in Complex Type Definition §3.4.7, and the
// Built-in Simple Type Definitions §3.16.7) — and NOT this schema's own {type
// definitions}, which clause 2.2.5 does not put in scope. No schema is
// finalized at production time to hold those anyway. A CTA cast target is
// always a simple type (§3.12.6 clause 4), so only §3.16.7's Simple Type
// Definitions are ever read here — §3.2.7 and §3.4.7 name no site this
// package resolves against.
//
// It answers from symbols.builtins, the fixed [builtin.Seed] set newSymbols
// indexes once — NOT from the build-once memo symbols.built, which starts as a
// copy of that set and then grows with every named simple type the assembly
// builds. Answering from the memo would make the answer depend on how far the
// parse had reached, and a schema whose own targetNamespace is the XSD namespace
// (unusual, but no rule forbids it) would put its top-level types in scope for
// some <alternative> tests and not others, purely by declaration order — a false
// reject of a legal schema in the over-charging direction.
type ctaStaticTypes struct{ syms *symbols }

// Type resolves a datatype name to the builtin declared under it, reporting
// false for every other name. Every key in the index carries
// [xsd.XMLSchemaNS] as its {target namespace}, so the lookup needs no
// namespace test of its own.
func (t ctaStaticTypes) Type(name xsd.QName) (xsd.TypeDefinition, bool) {
	st, seeded := t.syms.builtins[name]
	if !seeded {
		return nil, false
	}
	return st, true
}

// typeTableOf maps the <alternative> children of el into the {type table} of
// the element declaration el produces (§3.3.2.1 dcl.elt.common), returning nil
// for the ·absent· property. edID is that declaration's minted identity, the
// {context} every anonymous type an <alternative> owns points back at, and
// declaredType is its own {type definition}, which §3.3.2.1 makes the {default
// type definition}'s {type definition} whenever the final <alternative> carries
// a test.
//
// Each <alternative>'s OWN children are ordered against s4sAlternative here
// (checkS4SChildOrder, #1275), the one content model Appendix A gives every
// occurrence of the element. The walk is charged AHEAD of checkSrcTA, the run
// order checkS4SChildOrder's doc records for every src-* charge: a document
// whose <alternative> children the content model does not admit is answered by
// the grammar fault and no src-ta verdict is reached over it. Both faults can be
// live on one element — a <simpleType> followed by a second <simpleType> repeats
// s4sAlternative's single type position AND is two forms to checkSrcTA's count —
// and this order decides which is reported.
//
// The mapping, once the s4s walk, src-ta and src-element clause 5 have passed
// over every <alternative>:
//
//   - {alternatives} takes the <alternative> children WITH a test attribute, in
//     document order, each through declare-ta. A TRAILING untested one is not
//     among them — it feeds {default type definition} instead. An untested one
//     ANYWHERE ELSE is named by neither property, since {alternatives} is
//     defined over the tested children and {default type definition} over the
//     final child, so it would map to nothing at all; src-element clause 5
//     (§3.3.3) forbids that document and the loop below charges it before any
//     child is mapped.
//   - {default type definition} is the final <alternative> through declare-ta
//     when it has no test attribute, and otherwise a Type Alternative
//     SYNTHESIZED with an ·absent· {test}, declaredType as its {type
//     definition}, and an empty {annotations}. §3.3.2.1 makes that synthesized
//     slot "the {type definition} property of the parent Element Declaration",
//     so declaredType is passed through WHOLE — the same component, not a copy,
//     and whichever arm it holds, including the SubstitutionGroupHeadTypeRef a
//     substitutionGroup=-typed element carries.
//
// ta-props-correct clause 2 is charged here, over each {test} as it is built,
// because this is the only site in the module that builds one and alt.Loc() is
// in hand. The constraint it delegates to — xpath-valid clause 2 (§3.13.6.2),
// "does not produce any static error" — is a Schema Component Constraint, so it
// is decided at construction and independent of whether any instance is ever
// ·assessed·; xpath.CTATestStaticError proves the static error and this charges
// it, wrapping that verdict as the charge's cause so the XPath code it carries
// (err:XPST0003, err:XPST0081, or err:XPST0051 or err:XPST0080 for a cast
// target outside ctaStaticTypes' builtin atomic types) stays readable with
// xsderr.RuleOf — the engine mints XPath's code, never this schema rule. An
// {expression} that engine merely cannot EVALUATE, and in which it proves no
// static error, is charged nothing and passes straight through, on the terms
// xpath/cta.go argues (PRINCIPLES 20). The type knowledge it is asked against
// is ctaStaticTypes, clause 2.2.5's set and not this schema's.
func (p *producer) typeTableOf(el *Element, edID xsd.ComponentID, declaredType xsd.TypeDefinitionOrRef) (*xsd.TypeTable, error) {
	alternatives := childElements(el, xsd.XMLSchemaNS, "alternative")
	if len(alternatives) == 0 {
		return nil, nil // §3.3.2.1: "otherwise ·absent·"
	}
	last := len(alternatives) - 1
	for i, alt := range alternatives {
		if err := checkS4SChildOrder(alt, s4sAlternative); err != nil {
			return nil, err
		}
		if err := checkSrcTA(alt); err != nil {
			return nil, err
		}
		if _, hasTest := alt.Attr("test"); !hasTest && i < last {
			return nil, xsderr.New(ruleSrcElement, alt.Loc(),
				"<alternative> %d of %d has no test attribute, but src-element clause 5 requires one on every <alternative> but the last", i+1, len(alternatives))
		}
	}
	types, err := p.alternativeTypes(alternatives, edID)
	if err != nil {
		return nil, err
	}
	defaultType := types[last] // §3.3.2.1 case 1
	if _, lastTested := alternatives[last].Attr("test"); lastTested {
		defaultType = declaredType // §3.3.2.1 case 2
	}
	dflt, err := xsd.NewTypeAlternative(el.Loc(), nil, defaultType)
	if err != nil {
		return nil, err
	}
	var alts []xsd.TypeAlternative
	for i, alt := range alternatives {
		// The test attribute is the whole membership rule, and clause 5 has left the
		// TRAILING child as the only untested one this can reach — it is already the
		// {default type definition} above.
		if _, hasTest := alt.Attr("test"); !hasTest {
			continue
		}
		// declare-ta: {test} is the XPath Expression property record §3.13.2
		// builds for the <alternative> host element and its test attribute — the
		// same record and the same xpathDefaultNamespace chain an <assert> gets,
		// which is why this reuses buildXPathExpression rather than restating it.
		test := p.buildXPathExpression(alt, "test")
		if serr := xpath.CTATestStaticError(test, ctaStaticTypes{syms: p.symbols}); serr != nil {
			// The XPath code is xpath's verdict and ta-props-correct is this
			// producer's, so the two travel as two layers: the delegated verdict is
			// the wrapped cause, which errors.Unwrap reaches and xsderr.RuleOf then
			// reads the XPath code off, instead of a consumer scraping the message
			// for it (validate/doc.go's delegated-verdict norm). It renders into
			// the message as well, for a reader holding only the string.
			charge := xsderr.New(ruleTAPropsCorrect, alt.Loc(),
				"the test attribute %q has an XPath static error (%s), but ta-props-correct clause 2 requires the {test} to satisfy xpath-valid, whose clause 2 admits none",
				test.Expression(), serr)
			charge.Err = serr
			return nil, charge
		}
		ta, terr := xsd.NewTypeAlternative(alt.Loc(), &test, types[i])
		if terr != nil {
			return nil, terr
		}
		alts = append(alts, ta)
	}
	tt, err := xsd.NewTypeTable(el.Loc(), alts, dflt)
	if err != nil {
		return nil, err
	}
	return &tt, nil
}

// alternativeTypes maps the {type definition} of each <alternative> in document
// order, over both arms §3.12.2 declare-ta states: "the type definition
// ·resolved· to by the ·actual value· of the type attribute, if one is present,
// otherwise the type definition corresponding to the complexType or simpleType
// among the children of the <alternative> element". checkSrcTA has already passed
// over every child, so exactly one of the three forms is present; the last arm
// reports the mismatch as a plain producer fault rather than a fabricated rule
// verdict, on the same footing newComplexType's redefining arm does.
//
// The inline arm OWNS the anonymous type it yields, so it is an
// xsd.InlineTypeDefinition, and the anonymous COMPLEX type carries two identities
// (typeAlternativeOwnedComplexType): edID, the enclosing element declaration
// §3.4.2.1 dcl.ctd.common makes its {context} — the <alternative> is not an
// <element>, so the ancestor walk passes straight over it — and a container token
// minted per <alternative>, which its own nested local declarations report as
// {scope}.{parent}. An anonymous SIMPLE type goes through constructSimpleType,
// the same entry point an inline <simpleType> child of an <element> or
// <attribute> takes, and its {context} (§3.16.1 std-context) is unmodeled here as
// it is everywhere else in this producer (#206).
//
// The anonymous complex type this builds is the FOURTH owning slot that receives
// no Phase D verdict and neither §3.4.2.4 nor §3.4.2.5 attribute fold; the
// direction argument and the trackers are stated once, at the GAP marker in
// xsd/complexderivation.go's checkComplexDerivations.
func (p *producer) alternativeTypes(alternatives []*Element, edID xsd.ComponentID) ([]xsd.TypeDefinitionOrRef, error) {
	types := make([]xsd.TypeDefinitionOrRef, len(alternatives))
	for i, alt := range alternatives {
		typeLex, hasType := alt.Attr("type")
		if hasType {
			qn, err := p.resolveQName(alt, typeLex, "type")
			if err != nil {
				return nil, err
			}
			types[i] = xsd.TypeDefinitionRef{Name: qn}
			continue
		}
		if inlineComplex := childElement(alt, xsd.XMLSchemaNS, "complexType"); inlineComplex != nil {
			ct, err := p.produceComplexType(newTypeAlternativeOwned(edID), inlineComplex)
			if err != nil {
				return nil, err
			}
			types[i] = xsd.InlineTypeDefinition{Definition: ct}
			continue
		}
		inlineSimple := childElement(alt, xsd.XMLSchemaNS, "simpleType")
		if inlineSimple == nil {
			return nil, fmt.Errorf("parser: the <alternative> at %s carries none of a type attribute, a <complexType> child and a <simpleType> child, but src-ta requires exactly one and checkSrcTA charges it before this mapping runs", alt.Loc())
		}
		st, err := p.constructSimpleType(xsd.QName{}, inlineSimple)
		if err != nil {
			return nil, err
		}
		types[i] = xsd.InlineTypeDefinition{Definition: st}
	}
	return types, nil
}

// checkSrcTA charges Type Alternative Representation OK (§3.12.3) for an
// <alternative> carrying none, or more than one, of a type attribute, a
// <complexType> child and a <simpleType> child.
//
// It runs BEFORE alternativeTypes maps any child, so the "exactly one form" it
// establishes is what that mapping's arm order relies on: src-ta is a constraint
// on the XML representation, and an <alternative> holding exactly one inline type
// child satisfies it outright.
//
// It runs BEHIND checkS4SChildOrder over the same element (s4sAlternative,
// #1275), so the children it counts are already position-valid: at most one of
// <simpleType> and <complexType> stands under the <alternative>, and nothing but
// a leading <annotation> stands beside it. That leaves the type attribute paired
// with an inline child as the ONLY shape reaching the more-than-one branch —
// two inline children repeat one position and the walk answers them first.
func checkSrcTA(alt *Element) error {
	forms := 0
	if _, hasType := alt.Attr("type"); hasType {
		forms++
	}
	if childElement(alt, xsd.XMLSchemaNS, "complexType") != nil {
		forms++
	}
	if childElement(alt, xsd.XMLSchemaNS, "simpleType") != nil {
		forms++
	}
	if forms == 1 {
		return nil
	}
	return xsderr.New(ruleSrcTA, alt.Loc(),
		"<alternative> has %d of a type attribute, a <complexType> child and a <simpleType> child, but src-ta requires exactly one", forms)
}
