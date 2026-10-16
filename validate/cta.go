package validate

import (
	"fmt"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file is §3.3.4.1's ·selected type definition· — the half of the
// ·governing type definition· that a {type table} decides — and nothing else.
// The ·overriding· xsi:type read that sits above it stays in assess.go, where
// key-governing-type-elem's own clause order lives.

// ruleKeyCTATASelect is ·successfully selects· (§3.12.4, key-cta-ta-select),
// the check conditionallySelected reaches for each Type Alternative and does
// not perform when the alternative's {test} cannot be compiled. It is a
// [Definition:] anchor and NOT a Validation Rule, because §3.12.4 defines
// conditional type assignment through definitions and gives it no cvc-* rule of
// its own — so this is the only ID the spec text offers for the check, and an
// invented cvc-cta* one would name nothing (#56).
//
// It is a Rule only ever carried by an [Unevaluated], never by an
// [xsderr.Error]: a failed key-cta-ta-select charges no element. The
// alternative simply does not ·successfully select·, and key-cta-select's scan
// moves to the next one or to the {default type definition} — which is why the
// package charges nothing under this ID and must not start.
const ruleKeyCTATASelect xsderr.Rule = "key-cta-ta-select"

// selectedType is the ·selected type definition· S of an element information
// item E whose ·governing element declaration· is d (§3.3.4.1,
// key-selected-type). ok is false wherever this package cannot determine it,
// which is [walk.declaredGovernance]'s decline and carries its consequences
// unchanged. ok true with a nil S is S determined and ·absent· (§5.3), which
// resolvedSelection has already charged and [walk.declaredGovernance] answers
// with ·lax assessment·.
//
// The rule has exactly two cases and they are taken in its order: clause 1,
// a declaration WITH a {type table}, whose table ·conditionally selects· S;
// clause 2, one without, whose {type definition} is S outright. inherited is
// E's [inherited attributes], which only clause 1 reads (conditionallySelected).
func (w *walk) selectedType(e Element, d xsd.ElementDeclaration, inherited []inheritedAttribute) (xsd.TypeDefinition, bool) {
	table, tabled := d.TypeTable()
	if !tabled {
		return w.resolvedSelection(e, d.TypeDefinition(), "the {type definition} of its ·governing element declaration·")
	}
	return w.conditionallySelected(e, table, inherited)
}

// resolvedSelection is the type definition a {type definition} slot that
// key-selected-type reads for e names, through [xsd.Schema.ResolvedType]; slot
// names that slot for the message. A finalized Schema reaches a slot that
// resolves to nothing two ways, and they end differently.
//
// A [xsd.SubstitutionGroupHeadTypeRef] whose head names no declaration, which
// Finalize accepts (xsd's resolveTypeDefinitionSlot), is an ·absent· value
// where a component is mandated (§5.3 Missing Sub-components), so S is ·absent·
// and §5.3 makes validating e "as if clause 1 of Element Locally Valid
// (Element) had failed", falling back to ·lax assessment·. That is cvc-elt
// clause 1 CHARGED at e's own location, and a nil S returned with ok true for
// [walk.declaredGovernance] to assess e laxly. The parser never builds this
// shape: it gives the member of an unresolvable substitutionGroup head
// xs:anyType.
//
// A nil {type definition}, the ·absent· slot of a declaration built without the
// parser's §3.3.2.1 defaulting (the declaration's own slot, or the slot of a
// head a SubstitutionGroupHeadTypeRef names; xsd.NewTypeAlternative rejects
// nil), is declined instead: RECORDED as one [Unevaluated] under cvc-elt clause
// 1 at e's own location, with ok false. GAP(validate): that slot is the
// sch-props-correct clause 1 fault xsd's Finalize defers, so the instance walk
// neither charges cvc-elt clause 1 for it nor assesses e laxly, and e is
// assessed against nothing. RULED permanent by #2166 (STYLE P3b), on #774's
// terms: the charge belongs to xsd's deferred schema check, not to the
// instance walk.
func (w *walk) resolvedSelection(e Element, ref xsd.TypeDefinitionOrRef, slot string) (xsd.TypeDefinition, bool) {
	t, ok := w.schema.ResolvedType(ref)
	if ok {
		return t, true
	}
	if w.absentHead(ref) {
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
			"the ·selected type definition· of the element %s is ·absent·: %s names a substitution group head with no element declaration in the schema, a missing sub-component that Structures §5.3 (Missing Sub-components) treats as if cvc-elt clause 1 had failed, so the element is ·laxly assessed·",
			e.Name(), slot))
		w.logDecision("assessing element", e.Name(), e.Loc(), ruleCvcElt, "1", "charged")
		return nil, true
	}
	w.decline("assessing element", e.Name(), e.Loc(), ruleCvcElt, "1",
		"the ·selected type definition· of the element %s was not determined, so it has no ·governing type definition· and was assessed against nothing: %s resolves to no type definition, an ·absent· (nil) {type definition} that sch-props-correct clause 1 leaves to the schema check, so cvc-elt clause 1 (Structures §5.3) is not charged here",
		e.Name(), slot)
	return nil, false
}

// absentHead reports whether ref is a [xsd.SubstitutionGroupHeadTypeRef] whose
// head names no element declaration in the schema: §5.3's ·absent· head, as
// against a head that exists and whose own slot is nil.
func (w *walk) absentHead(ref xsd.TypeDefinitionOrRef) bool {
	head, isHead := ref.(xsd.SubstitutionGroupHeadTypeRef)
	if !isHead {
		return false
	}
	_, found := w.schema.Element(head.Head)
	return !found
}

// conditionallySelected is the type a Type Table ·conditionally selects· for e
// (§3.3.4.1, key-cta-select): the {alternatives} are tried in document order
// until one ·successfully selects· a type definition (§3.12.4,
// key-cta-ta-select — its {test} evaluates to true), and if none does, the
// {default type definition}'s {type definition} is the answer. Every {test}
// reads e's own [[attributes]] and inherited, e's [inherited attributes]
// (ctaAttributes). The scan is selectAlternative's, the one
// [Validator.ConditionallySelected] makes too; this method adds the walk's
// record of a withhold and resolvedSelection's reading of the slot selected.
//
// The scan is LAZY in both directions the rule quantifies in. It compiles and
// evaluates one alternative at a time, because "if any Type Alternative
// ·successfully selects· a type definition, none of the following Type
// Alternatives are tried" — so an alternative this engine cannot evaluate,
// sitting BEHIND one whose test is true, never costs e its type.
//
// An alternative this engine cannot evaluate, sitting BEFORE any that
// succeeds, withholds the whole element's ·governing type definition· and
// stops the scan. It does NOT fall through to the next alternative or to the
// {default type definition}: the undecided test might have been true, so
// continuing would select a type the rule may not have selected, and
// assessing an element against the wrong type manufactures a false reject in
// both directions (the charge governingType's own doc rules out). Withholding
// can only cost a rejection.
//
// That withhold is RECORDED, as one [Unevaluated] under ruleKeyCTATASelect at
// the element's own location, and logged as declined at clause 2, the {test}
// evaluation the compiler declined ([walk.decline]): nothing downstream charges
// the element, so without the record an element whose ·governing type
// definition· was withheld is byte-identical at the [Result] API to one that
// passed every rule (#56). A selected {type definition} that resolves to
// nothing is resolvedSelection's: an ·absent· head charged there under cvc-elt
// clause 1, a nil slot withheld and recorded there under cvc-elt.
//
// A DYNAMIC OR TYPE ERROR INSIDE AN EVALUABLE {test} IS NOT RECORDED, because
// it is not a withhold: key-cta-ta-select clause 2 says such a {test} "is
// treated as if it had evaluated (without error) to false", so the alternative
// has been tried and did not select, and the scan continues to the next one or
// to the {default type definition} with a real type in hand. [xpath.CTATest]
// evaluation therefore reports a bool and no error at all, and the two
// outcomes stay apart at the type level rather than by a check here.
//
// w.schema is the xsd.TypeResolver both halves of the engine take, and it is
// passed rather than stored on either side: the compile reads the {type
// definitions} to classify the datatype a {test} casts to, and the evaluation
// reads them again to walk that type's {base type definition} chain.
//
// e-props-correct clause 6, which xsd.NewTypeTable enforces at construction,
// makes every {alternatives} member's {test} present, so the presence flag
// carries nothing here; a table that reached this package without it declines
// anyway, because a zero XPath Expression record has an empty {expression} and
// no {expression} is a Test production.
func (w *walk) conditionallySelected(e Element, table xsd.TypeTable, inherited []inheritedAttribute) (xsd.TypeDefinition, bool) {
	alts := table.Alternatives()
	i, decided := selectAlternative(table, w.backend, w.schema, ctaAttributes(e.Attributes(), inherited))
	if !decided {
		test, _ := alts[i].Test()
		w.decline("assessing element", e.Name(), e.Loc(), ruleKeyCTATASelect, "2",
			"alternative %d of %d in the {alternatives} of the {type table} of %s's ·governing element declaration·, whose {test} is %q, was not evaluated: this engine's §3.12.6 required-subset compiler declined it, which ta-props-correct clause 2 licenses (a conforming processor may but is not required to support XPath outside that subset), so whether it ·successfully selects· its {type definition} is undecided and the type the table ·conditionally selects· (§3.3.4.1, key-cta-select) is withheld along with it, together with every alternative behind it",
			i+1, len(alts), e.Name(), test.Expression())
		return nil, false
	}
	if i < len(alts) {
		return w.resolvedSelection(e, alts[i].TypeDefinition(), fmt.Sprintf(
			"the {type definition} of alternative %d of %d in the {alternatives} of the {type table} of its ·governing element declaration·, which ·successfully selects· it,",
			i+1, len(alts)))
	}
	return w.resolvedSelection(e, table.DefaultTypeDefinition().TypeDefinition(),
		"the {type definition} of the {default type definition} of the {type table} of its ·governing element declaration·, which the table ·conditionally selects· because no alternative ·successfully selects· a type definition,")
}

// ConditionallySelected is the {type definition} slot table ·conditionally
// selects· (§3.3.4.1, key-cta-select) for an element E whose [attributes] are
// attributes, in source order, and whose [inherited attributes] are inherited
// (§3.3.5.6, e-inherited_attributes): the slot of the first of table's
// {alternatives} whose {test} evaluates to true (§3.12.4, key-cta-ta-select),
// or, where none does, the slot of its {default type definition}, with ok
// true. The slot is unresolved; [SchemaView]'s ResolvedType resolves it.
//
// Each {test} reads E's attributes, then each of inherited whose ·expanded
// name· no member of attributes has (key-cta-ta-select clauses 1.1.2 and
// 1.1.3); inherited is required, and nil only where E has no [inherited
// attributes], as at the validation root (§3.3.5.6, key-p-inherited clause 1).
// It is the selection [Validator.Assess] makes for E, by the same scan: a
// dynamic or type error inside a {test} counts as false (key-cta-ta-select
// clause 2), and ok is false exactly where that scan withholds the selection —
// an alternative tried before any succeeds has a {test} this engine's §3.12.6
// compiler declines — never where no alternative succeeds.
//
// ok true does not promise a type: the slot may resolve to none (nil, or a
// substitution-group head that names no declaration or whose own slot does
// not resolve), where ResolvedType answers false. Assess, resolving that same
// slot, then withholds E's ·selected type definition· under cvc-elt clause 1,
// or charges that clause where the head is ·absent· (§5.3).
//
// The one consumer is validate/jsonsrc, which settles which members of a JSON
// object are attributes by the type their selection gives.
func (v *Validator) ConditionallySelected(table xsd.TypeTable, attributes, inherited []Attribute) (xsd.TypeDefinitionOrRef, bool) {
	copied := make([]inheritedAttribute, 0, len(inherited))
	for _, a := range inherited {
		copied = append(copied, inheritedAttribute{name: a.Name(), value: a.Value()})
	}
	i, decided := selectAlternative(table, v.backend, v.schema, ctaAttributes(attributes, copied))
	if !decided {
		return nil, false
	}
	alts := table.Alternatives()
	if i < len(alts) {
		return alts[i].TypeDefinition(), true
	}
	return table.DefaultTypeDefinition().TypeDefinition(), true
}

// selectAlternative is key-cta-select's scan of table over attrs: the index of
// the first of its {alternatives} that ·successfully selects· its {type
// definition} (key-cta-ta-select), or len of them where none does and the
// {default type definition} is selected, with decided true; and, decided
// false, the index of the alternative whose {test} [xpath.CompileCTATest]
// declined, which the scan reached before any success and stopped at. types
// is the xsd.TypeResolver both halves of the engine take, and backend the
// value space the evaluation casts in. It is the one scan
// [walk.conditionallySelected] and [Validator.ConditionallySelected] share.
func selectAlternative(table xsd.TypeTable, backend value.Backend, types xsd.TypeResolver, attrs xpath.Attributes) (int, bool) {
	alts := table.Alternatives()
	for i, alt := range alts {
		test, _ := alt.Test()
		compiled, evaluable := xpath.CompileCTATest(test, types)
		if !evaluable {
			return i, false
		}
		if compiled.Evaluate(backend, types, attrs) {
			return i, true
		}
	}
	return len(alts), true
}

// ctaAttributes is the attribute sequence a {test} evaluates against, the two
// halves §3.12.4 key-cta-ta-select copies into the XDM instance the test runs
// over, in that order. First clause 1.1.2's: attrs, the [[attributes]] of E
// itself, in SOURCE order. Then clause 1.1.3's: each member of inherited, E's
// own [inherited attributes] (§3.3.5.6, e-inherited_attributes,
// [walk.handedDown]), whose ·expanded name· no attribute of E has — so E's own
// attribute wins outright over a same-named inherited one. That filter is E's
// alone and is a second one: the shadowing among E's ancestors is already
// settled in inherited. The walk hands inherited as the []inheritedAttribute
// it computes, and [Validator.ConditionallySelected] copies its caller's
// []Attribute into one, so both meet this one filter.
//
// The xsi: attributes are among the first half, because clause 1.1.2 copies E's
// [[attributes]] whole and names no exception; the namespace declarations are
// not, because [Element]'s Attributes excludes them — which is what keeps a
// wildcard NameTest from ranging over xmlns bindings as if they were attribute
// nodes.
//
// attrs is read once, not per {test}: [Element]'s Attributes may build it, and
// a {test} naming three attributes would otherwise build it three times.
func ctaAttributes(attrs []Attribute, inherited []inheritedAttribute) xpath.Attributes {
	return func(yield func(xsd.QName, string) bool) {
		for _, a := range attrs {
			if !yield(a.Name(), a.Value()) {
				return
			}
		}
		for _, a := range inherited {
			if hasAttributeNamed(attrs, a.name) {
				continue
			}
			if !yield(a.name, a.value) {
				return
			}
		}
	}
}
