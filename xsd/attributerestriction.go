package xsd

import "github.com/kud360/goxsd8/xsderr"

// This file renders the ATTRIBUTE half of Derivation Valid (Restriction,
// Complex) clause 3 (Structures §3.4.6.3, c-ran) over the two properties the
// clause actually reads — {attribute uses} and {attribute wildcard} — rather
// than over the Complex Type Definition that usually carries them. Two
// constraints invoke that clause, and only one of them has a complex type to
// hand:
//
//   - derivation-ok-restriction itself, for a complex type restricting its
//     {base type definition} (checkRestrictionAttributes, complexderivation.go);
//   - src-redefine clause 7.2.2 (§4.2.4), which words the invocation as a ROLE
//     SUBSTITUTION — the redefining attribute group's two properties "viewed as
//     the {attribute uses} and {attribute wildcard} of a Complex Type
//     Definition", the redefined document's definition's "viewed as … of the
//     {base type definition}" — and mints no component to view them through
//     (checkAttributeGroupRedefinitions, redefinition.go).
//
// One encoding serves both (STYLE T4): the two property pairs, the rule and
// position to charge, and the prose each side is named by travel together as an
// attributeRestriction, and nothing below reads a type definition. What each
// caller supplies, and whether either side's sets are materialised by a fold,
// is the CALLER's fact and is stated in the caller's own doc — the two differ,
// and c-ran clause 3 is indifferent to the difference.

// attributeRestrictionSide is one side of c-ran clause 3: the {attribute uses}
// and {attribute wildcard} it quantifies over, plus the phrase a message names
// their owner by. The label is not decoration (STYLE E1): on the src-redefine
// path the two sides share an expanded name — a redefinition redefines the name
// it replaces — so the phrase is the only thing in the message that tells them
// apart.
//
// The wildcard is a value plus a present flag, mirroring the AttributeWildcard()
// (Wildcard, bool) accessor both component kinds already expose. The flag is the
// property's ·absent· and is not derivable from the value, since a present
// wildcard may be the zero record.
type attributeRestrictionSide struct {
	uses        []AttributeUse
	wildcard    Wildcard
	hasWildcard bool
	label       string
}

// attributeRestriction bundles everything that differs between the two
// constraints invoking c-ran clause 3, the way locallyDeclaredTypeCheck
// (complexderivation.go) bundles what differs between the two constraints
// quantifying over ·locally declared types·.
//
// derived is the spec's T and base its B, and the direction is the whole
// content of the check: T's attributes must stay valid against B, never the
// reverse. verb renders that direction in the message rather than leaving a
// reader to infer it from argument order.
type attributeRestriction struct {
	rule    xsderr.Rule              // the rule a failure is charged to
	loc     xsderr.Loc               // the position it is charged at, always the derived side's
	verb    string                   // "restricts" | "must restrict": how the message states the obligation
	clause  string                   // the constraint(s) the message cites
	derived attributeRestrictionSide // the spec's T
	base    attributeRestrictionSide // the spec's B
}

// complexTypeAttributeSide is the c-ran clause 3 view of a Complex Type
// Definition: the two properties, read off the component as they stand. Both are
// the MATERIALISED ones by the time any caller here runs — see
// checkRestrictionAttributes for what that buys and why it is load-bearing.
func complexTypeAttributeSide(c ComplexType, label string) attributeRestrictionSide {
	w, has := c.AttributeWildcard()
	return attributeRestrictionSide{uses: c.attributeUses, wildcard: w, hasWildcard: has, label: label}
}

// attributeGroupAttributeSide is the same view of an Attribute Group Definition,
// which src-redefine clause 7.2.2 instructs be read as a complex type's. No fold
// applies to it and none is missing; see checkAttributeGroupRedefinitions
// (redefinition.go) for why.
func attributeGroupAttributeSide(g AttributeGroupDefinition, label string) attributeRestrictionSide {
	w, has := g.AttributeWildcard()
	return attributeRestrictionSide{uses: g.attributeUses, wildcard: w, hasWildcard: has, label: label}
}

// checkAttributeRestriction is c-ran clause 3: for every element information
// item whose [attributes] satisfy cvc-complex-type (§3.4.4.2) clauses 2 and 3
// with respect to T, they must satisfy the same clauses with respect to B, and
// B's ·default binding· for each attribute must ·subsume· T's (loc-testSubP).
// Rendered statically over the component model, that is three obligations:
//
//   - every expanded name T admits through an {attribute use} must be admitted
//     by B at all (cvc-complex-type clause 2.1: otherwise an element carrying it
//     is valid against T and not against B), and B's binding for it must
//     ·subsume· T's, which for an {attribute use} is loc-testSubP clause 5;
//   - every expanded name T admits through its {attribute wildcard} AND no
//     {attribute use} of T's already claims must be admitted by B too
//     (cvc-complex-type clause 2.2, c-avaw, which clause 2.1 pre-empts per name),
//     and B's binding for such a name must ·subsume· T's, which for two
//     wildcard keywords is loc-testSubP clauses 1-3
//     (checkAttributeRestrictionWildcard);
//   - every attribute B marks {required} must stay required in T
//     (cvc-complex-type clause 3, checkAttributeRestrictionRequired).
//
// The three run in that order, which is cvc-complex-type's own: clause 2's two
// sub-cases before clause 3's. The uses are walked in document order, so the
// first reported failure is deterministic (STYLE D2).
func (s *Schema) checkAttributeRestriction(r attributeRestriction) error {
	for _, u := range r.derived.uses {
		name := u.DeclarationName()
		general, ok := s.attributeDefaultBinding(r.base, name)
		if !ok {
			return xsderr.New(r.rule, r.loc,
				"%s %s %s but declares an attribute use for %s, which %s neither declares nor admits through an {attribute wildcard}, so an element valid against the restriction can carry an attribute the base rejects (%s)", r.derived.label, r.verb, r.base.label, name, r.base.label, r.clause)
		}
		if err := s.checkBindingSubsumes(name, r, general, attributeUseBinding{use: u}); err != nil {
			return err
		}
	}
	if err := checkAttributeRestrictionWildcard(r); err != nil {
		return err
	}
	return checkAttributeRestrictionRequired(r)
}

// checkAttributeRestrictionWildcard is the cvc-complex-type clause 2.2 (c-avaw)
// half of c-ran: the names an element valid against T may carry WITHOUT a
// matching {attribute use} are exactly those T's {attribute wildcard} admits, and
// B must admit each of them too — through its own {attribute wildcard}, since no
// finite {attribute uses} set covers the open name set a wildcard admits.
//
// So the obligation is a relation between the two wildcards, and it is decided by
// wildcardSubset — Wildcard Subset (§3.10.6.2, cos-ns-subset), the ONE encoding of
// that relation in this package (namespaceconstraint_subset.go, shared with the
// element-side transition test in contentrestricts.go; STYLE T4). The direction
// is T's constraint as sub and B's as super: T may admit fewer names than B, never
// more. A T with no {attribute wildcard} admits nothing this way and discharges
// the clause vacuously, which is why the absent case returns before B is read.
//
// Neither side walks a base chain, and for the complex-type caller §3.4.2.5 is
// what makes that exact: a restriction's {attribute wildcard} is its ·complete
// wildcard· (clause 2.1) and an extension's is already the union of its own with
// its base's, materialised at finalize (attributewildcardfold.go, clause 2.2).
// Before that fold, a B that inherited its wildcard read as having none and this
// check FALSELY rejected its restrictions; the fold is what makes the comparison
// sound, not merely more complete.
//
// What the comparison ranges over is names WITHOUT a matching {attribute use},
// and that restriction is cvc-complex-type clause 2's own dispatch rather than a
// refinement of it: clause 2.2 is reached only "otherwise", once clause 2.1
// (c-ctma) has failed to find an attribute use of the item's expanded name, and
// §3.4.4.2's Note states the precedence unconditionally — "the attribute use
// always takes precedence, and the assessment of such items stands or falls
// entirely on the basis of the attribute use and its {attribute declaration}".
// wildcardSubset decides cos-ns-subset over {namespace constraint}s alone and
// knows nothing of {attribute uses}, so the gate is applied HERE: the names
// clause 2.1 claims are dropped from B's {disallowed names}
// (sharedAttributeUseNames, withoutDisallowedNames) before the record is handed
// to the relation. Teaching wildcardSubset the name set instead would put
// cos-ns-subset in two encodings, which is what T4 forbids and what #262
// declined to build.
//
// The exempt set is the INTERSECTION of the two {attribute uses} sets, never
// either side alone, because the two directions are not symmetric:
//
//   - a name BOTH sides hold a use for is assessed by clause 2.1 against T AND
//     by clause 2.1 against B, so clause 2.2 fires on neither side and B's
//     {disallowed names} entry for it cannot be charged however it got there.
//     This is the shape #430 fixed: B declaring <xs:attribute name="foo"/>
//     alongside <anyAttribute namespace="##any" notQName="foo"/>, T restricting
//     it with <anyAttribute namespace="##any"/> and inheriting the foo use whole
//     (§3.4.2.4 clause 3.2, attributeusefold.go), was a FALSE REJECT of a valid
//     schema — Finalize returns this error to its caller in place of a *Schema.
//   - a name only T holds a use for is NOT exempt: c-ran clause 3 still requires
//     the item to satisfy clause 2 with respect to B, and B, having no use for
//     it, can only satisfy it through clause 2.2 — so a B whose wildcard
//     disallows that name is a genuine violation and stays charged. (The loop in
//     checkAttributeRestriction charges the same shape first, through
//     attributeDefaultBinding; this half is what holds when a caller reaches it
//     directly.)
//
// Past that gate, c-ran clause 3's ·default binding· conjunct is charged over
// the same names: B's binding must ·subsume· T's (loc-testSubP), and here both
// are the wildcards' {process contents} keywords, compared by keywordSubsumes
// through checkKeywordSubsumes (defaultbinding.go, the one encoding; STYLE T4).
// That comparison is exact, by keywordSubsumes' own argument: a T wildcard that
// admits a namespace admits some name that no {attribute use} on either side
// claims and that has no ·governing attribute declaration·, so T binds an item
// carrying it to T's keyword (key-dft-binding cases 4-6), and B, which admits it
// too once the subset gate has passed, binds it to B's. A T wildcard that admits
// NO namespace — an empty enumeration, namespace="" — has no item ·attributed·
// to it (cvc-wildcard clause 1), so it binds nothing, and admitsSomeNamespace
// returns before the comparison. That includes an empty ·complete wildcard·
// intersected out of <attributeGroup ref> wildcards (§3.4.2.5 clause 1,
// §3.6.2.2).
func checkAttributeRestrictionWildcard(r attributeRestriction) error {
	if !r.derived.hasWildcard {
		return nil
	}
	if !r.base.hasWildcard {
		return xsderr.New(r.rule, r.loc,
			"%s %s %s and declares an {attribute wildcard}, but %s has none, so an element valid against the restriction can carry a wildcard-admitted attribute the base rejects (%s, via cvc-complex-type clause 2.2, c-avaw)", r.derived.label, r.verb, r.base.label, r.base.label, r.clause)
	}
	bnc := r.base.wildcard.NamespaceConstraint().withoutDisallowedNames(sharedAttributeUseNames(r))
	if !wildcardSubset(r.derived.wildcard.NamespaceConstraint(), bnc) {
		return xsderr.New(r.rule, r.loc,
			"%s %s %s but its {attribute wildcard} admits expanded names %s's does not, so an element valid against the restriction can carry a wildcard-admitted attribute the base rejects (%s, via cvc-complex-type clause 2.2, c-avaw, and cos-ns-subset)", r.derived.label, r.verb, r.base.label, r.base.label, r.clause)
	}
	if !admitsSomeNamespace(r.derived.wildcard.NamespaceConstraint()) {
		return nil // no item is ·attributed· to it, so it binds nothing
	}
	return checkKeywordSubsumes(wildcardWitness, r,
		wildcardKeywordBinding{keyword: r.base.wildcard.ProcessContents()},
		wildcardKeywordBinding{keyword: r.derived.wildcard.ProcessContents()})
}

// wildcardWitness is how checkKeywordSubsumes' message names the attribute
// checkAttributeRestrictionWildcard compares bindings for: no one expanded name,
// but the witness that function's doc argues exists.
const wildcardWitness = "an attribute both {attribute wildcard}s admit and no declaration governs"

// sharedAttributeUseNames is the set of expanded names cvc-complex-type clause
// 2.1 claims on BOTH sides of the comparison: the {attribute declaration} name of
// every member of T.{attribute uses} that B holds a use for as well. An item
// carrying such a name is assessed against an attribute use whichever of the two
// it is validated against, so clause 2.2 governs it on neither side.
//
// For the complex-type caller both sides are the MATERIALISED sets (§3.4.2.4
// clause 3, attributeusefold.go), which is what makes the intersection the
// ordinary case rather than a corner: clause 3.2 hands T the base's use unchanged
// whenever T neither re-declares nor prohibits the name, so a restriction that
// touches no attribute at all shares every one of B's uses. Read off B's own
// <attribute> children instead, an inherited use would go missing and the
// exemption would silently not apply.
//
// The walk is over T's set in document order (STYLE D2), and the result feeds a
// membership test only.
func sharedAttributeUseNames(r attributeRestriction) []QName {
	var names []QName
	for _, u := range r.derived.uses {
		name := u.DeclarationName()
		if !hasAttributeUseNamed(r.base.uses, name) {
			continue
		}
		names = append(names, name)
	}
	return names
}

// checkAttributeRestrictionRequired is the cvc-complex-type clause 3 half of
// c-ran: an attribute B marks {required} must also be required in T, or an
// attribute set that satisfies clause 3 with respect to T by omitting it fails
// with respect to B.
//
// The two ways the clause can fail are both charged. A name T relaxes to optional
// is one. A base-required name with NO member in T at all is the other, and it is
// live for both callers, for different reasons: for a complex type, clause 3.2
// inherits every base use T does not declare itself, so the only way a required
// name goes missing is clause 3.2.2's <attribute use="prohibited"> blocking it
// (attributeusefold.go) — exactly the shape this half is meant to charge; for a
// redefining attribute group, nothing is inherited at all (the Note under
// src-redefine clause 7.2.2), so a required attribute simply left out of the
// redefinition lands here.
func checkAttributeRestrictionRequired(r attributeRestriction) error {
	for _, bu := range r.base.uses {
		if !bu.Required() {
			continue
		}
		name := bu.DeclarationName()
		tu, ok := findAttributeUse(r.derived.uses, name)
		if !ok {
			return xsderr.New(r.rule, r.loc,
				"%s %s %s but its {attribute uses} carry no use for attribute %s, which the base requires, so an element omitting it is valid against the restriction and not against the base (%s, via cvc-complex-type clause 3)", r.derived.label, r.verb, r.base.label, name, r.clause)
		}
		if tu.Required() {
			continue
		}
		return xsderr.New(r.rule, r.loc,
			"%s %s %s but declares attribute %s as optional where the base requires it, so an element omitting it is valid against the restriction and not against the base (%s, via cvc-complex-type clause 3)", r.derived.label, r.verb, r.base.label, name, r.clause)
	}
	return nil
}
