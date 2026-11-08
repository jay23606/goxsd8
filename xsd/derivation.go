package xsd

import (
	"math/big"

	"github.com/kud360/goxsd8/xsderr"
)

// ruleCosSTRestricts is Derivation Valid (Restriction, Simple) (Structures
// §3.16.6.2, id="cos-st-restricts"): the per-variety constraints relating a
// Simple Type Definition D to its {base type definition} B. This package charges
// its structural/variety-shape sub-clauses (1.1, 2.1, 2.2.1.1, 2.2.1.2, 2.2.2.1,
// 2.2.2.3, 3.1, 3.2.1.1, 3.2.1.2, 3.2.2.1, 3.2.2.3) at construction time, plus
// the list/union applicable-facet clauses 2.2.2.4 and 3.2.2.4 and the part of
// the facet-constraint clauses 1.3.2 / 2.2.2.5 / 3.2.2.5 that reads counts,
// keyword tokens, or which bound facets one step specifies. Its remaining
// facet-value sub-clauses need an applicability table or a value space and are
// charged above this pure leaf — see SimpleType.CheckDerivation.
const ruleCosSTRestricts xsderr.Rule = "cos-st-restricts"

// The precisionDecimal scale-facet Schema Component Constraints, charged at
// schema construction against the abstract-model {facets} property
// (SimpleType.EffectiveFacets) — the construction-time complements of the
// instance-time cvc-maxScale-valid / cvc-minScale-valid facet stages (#133,
// package value). Only precisionDecimal and its restrictions carry maxScale /
// minScale (xsd-precisionDecimal.md §3.3), so these are vacuous on every other
// type.
const (
	// ruleMaxScaleValidRestriction is maxScale valid restriction
	// (xsd-precisionDecimal.md §4.2.4, id="maxScale-valid-restriction"): a
	// restriction's maxScale {value} may not be greater than the {value} of the
	// {base type definition}'s effective maxScale — maxScale may only move down.
	ruleMaxScaleValidRestriction xsderr.Rule = "maxScale-valid-restriction"
	// ruleMinScaleValidRestriction is minScale valid restriction
	// (xsd-precisionDecimal.md §4.3.4, id="minScale-valid-restriction"): the
	// mirror image — a restriction's minScale {value} may not be less than the
	// base's effective minScale — minScale may only move up.
	ruleMinScaleValidRestriction xsderr.Rule = "minScale-valid-restriction"
	// ruleMinScaleLEMaxScale is the "minScale less than or equal to maxScale"
	// consistency SCC (xsd-precisionDecimal.md §4.3.4). It is NOT
	// restriction-specific: it constrains any type's {facets}. WHY the constant
	// carries the string "minScale-totalDigits": the spec's anchor id for this
	// SCC is a copy-paste bug that names the totalDigits constraint, so that is
	// the only string extractable from the spec text for the catalog; error
	// messages cite the SCC by its true title instead, and the spec's own Note
	// disclaims any relation to totalDigits.
	ruleMinScaleLEMaxScale xsderr.Rule = "minScale-totalDigits"
	// ruleMaxScaleFixed is the maxScale {fixed}-inheritance SCC
	// (xsd-precisionDecimal.md §4.2.1 dc-maxScale, id="f-ms-fixed"): if the base's
	// effective maxScale is {fixed}, a restriction may not specify any maxScale
	// {value} other than the base's — checked independently of the value SCC, as a
	// further-narrowing value satisfies maxScale-valid-restriction yet still
	// violates {fixed}.
	ruleMaxScaleFixed xsderr.Rule = "f-ms-fixed"
	// ruleMinScaleFixed is the minScale {fixed}-inheritance SCC
	// (xsd-precisionDecimal.md §4.3.1 dc-minScale, id="f-mns-fixed"): the mirror
	// of ruleMaxScaleFixed for minScale.
	ruleMinScaleFixed xsderr.Rule = "f-mns-fixed"
)

// The §4.3 Constraining Facet Schema Component Constraints whose operands are
// plain counts or keyword tokens — the subset of cos-st-restricts clause 1.3.2 /
// 2.2.2.5 / 3.2.2.5 ("DF satisfies the constraints on facet components given in
// the appropriate subsection of Constraining Facets") that this pure-leaf package
// can decide without a value space — plus the two bound-facet SCCs whose operand
// is which facets a derivation step specifies, not what their {value}s are. The
// rules whose operands ARE value-space members — the four bound facets' {value}s
// and enumeration — live in package value (value/restriction.go), which this
// package must not depend on (PRINCIPLES 1).
const (
	// ruleLengthValidRestriction is length valid restriction (§4.3.1.4,
	// id="length-valid-restriction"): a restriction's length {value} must EQUAL
	// the base's — length may not move at all, in either direction.
	ruleLengthValidRestriction xsderr.Rule = "length-valid-restriction"
	// ruleMinLengthValidRestriction is minLength valid restriction (§4.3.2.4,
	// id="minLength-valid-restriction"): a restriction's minLength {value} may
	// not be less than the base's — minLength may only move up.
	ruleMinLengthValidRestriction xsderr.Rule = "minLength-valid-restriction"
	// ruleMaxLengthValidRestriction is maxLength valid restriction (§4.3.3.4,
	// id="maxLength-valid-restriction"): the mirror — a restriction's maxLength
	// {value} may not be greater than the base's.
	ruleMaxLengthValidRestriction xsderr.Rule = "maxLength-valid-restriction"
	// ruleTotalDigitsValidRestriction is totalDigits valid restriction
	// (§4.3.11.4, id="totalDigits-valid-restriction"): a restriction's
	// totalDigits {value} may not be greater than the base's.
	ruleTotalDigitsValidRestriction xsderr.Rule = "totalDigits-valid-restriction"
	// ruleFractionDigitsValidRestriction is fractionDigits valid restriction
	// (§4.3.12.4, id="fractionDigits-valid-restriction"): a restriction's
	// fractionDigits {value} may not be greater than the base's.
	ruleFractionDigitsValidRestriction xsderr.Rule = "fractionDigits-valid-restriction"
	// ruleWhiteSpaceValidRestriction is whiteSpace valid restriction (§4.3.6.4,
	// id="whiteSpace-valid-restriction"): a restriction may not move whiteSpace
	// to a LESS restrictive keyword. See whiteSpaceRank for why the ordering is
	// preserve < replace < collapse.
	ruleWhiteSpaceValidRestriction xsderr.Rule = "whiteSpace-valid-restriction"
	// ruleTimezoneValidRestriction is timezone valid restriction (§4.3.14.4,
	// id="timezone-valid-restriction"): once the base fixes explicitTimezone to
	// required or prohibited, a restriction must repeat that same {value}; only
	// a base {value} of optional may be narrowed.
	ruleTimezoneValidRestriction xsderr.Rule = "timezone-valid-restriction"
	// ruleLengthMinLengthMaxLength is "length and minLength or maxLength"
	// (§4.3.1.4, id="length-minLength-maxLength"): when length is in {facets},
	// any coexisting minLength may not exceed it and any coexisting maxLength may
	// not fall below it (clauses 1.1/2.1, checkLengthCoexistence), and length and
	// that coexisting minLength/maxLength may not have been specified together at
	// one derivation step (clauses 1.2/2.2, checkLengthDerivationHistory, which
	// declines the strictest reading of those two clauses under a tracked GAP).
	// All four clauses are charged.
	ruleLengthMinLengthMaxLength xsderr.Rule = "length-minLength-maxLength"
	// ruleMinLengthLEMaxLength is "minLength <= maxLength" (§4.3.2.4,
	// id="minLength-less-than-equal-to-maxLength"), a same-type consistency SCC.
	ruleMinLengthLEMaxLength xsderr.Rule = "minLength-less-than-equal-to-maxLength"
	// ruleFractionDigitsLETotalDigits is "fractionDigits less than or equal to
	// totalDigits" (§4.3.12.4, id="fractionDigits-totalDigits"), a same-type
	// consistency SCC.
	ruleFractionDigitsLETotalDigits xsderr.Rule = "fractionDigits-totalDigits"
	// ruleMaxInclusiveMaxExclusive is "maxInclusive and maxExclusive" (§4.3.8.4,
	// id="maxInclusive-maxExclusive"): the two may not both be specified in the
	// same derivation step.
	ruleMaxInclusiveMaxExclusive xsderr.Rule = "maxInclusive-maxExclusive"
	// ruleMinInclusiveMinExclusive is "minInclusive and minExclusive" (§4.3.9.4,
	// id="minInclusive-minExclusive"): the two may not both be specified in the
	// same derivation step.
	ruleMinInclusiveMinExclusive xsderr.Rule = "minInclusive-minExclusive"
)

// CheckDerivation enforces the cross-reference Simple Type Definition
// constraints that need t's RESOLVED {base type definition}, {item type
// definition} and {member type definitions} — the checks NewSimpleType cannot
// make, because the base slot may be a deferred SimpleTypeRef that only a
// resolver can follow. It reads the four derived properties through
// SimpleType.Variety/Primitive/Item/Members, which resolve them off the declared
// derivation and the base chain (§3.16.2.1) against r.
//
// It has two kinds of caller:
//
//   - finalize. Finalize runs it over every Simple Type Definition an assembled
//     Schema reaches, with the Schema itself as the resolver. That is where a
//     parsed schema's simple types are charged.
//   - a Schema-LESS assembler. A caller that builds a *SimpleType graph
//     programmatically and never finalizes — builtin.Seed is one — runs it
//     itself against a resolver of its own. Nothing else charges these
//     constraints on such a graph.
//
// An unresolvable base is returned VERBATIM as the src-resolve clause 1.1 error
// SimpleType.Base produces, not folded into one of the clauses below: the
// clauses each presuppose a base that exists, and charging "no such type" as,
// say, cos-st-restricts clause 1.1 would name a constraint with nothing to say
// about it.
//
// It charges, per clause:
//
//   - st-props-correct clause 5 (each member of {facets} is supported by the
//     processor).
//   - st-props-correct clause 1 for the property tableau's {variety} clause
//     (Datatypes §4.1.1 dc-defn: "Required for all Simple Type Definitions
//     except ·xs:anySimpleType·, in which it is ·absent·"): only the type that
//     IS xs:anySimpleType — the anchor, by identity (IsAnySimpleType) — may
//     carry an absent {variety}. This is what rejects a user-defined type on
//     xs:anySimpleType: the XML mapping gives a ·restriction· the {variety} of
//     its {base type definition} (§3.16.2.1), so such a type inherits
//     xs:anySimpleType's absent {variety} and violates the tableau.
//     cos-st-restricts clause 1.1 does NOT reach that schema — its case 1
//     selects on D.{variety} = atomic, which such a D does not have; 1.1 stays
//     live for a producer that asserts an atomic D over a non-atomic B.
//   - st-props-correct clause 3 (D.{base type definition}.{final} does not
//     contain restriction). This single check also discharges cos-st-restricts
//     clauses 1.2, 2.2.2.2, and 3.2.2.2 ("B.{final} does not contain
//     restriction"): B is by definition D's {base type definition}, so those
//     clauses are the identical predicate on the identical component, and a
//     violation is charged once, as st-props-correct clause 3.
//   - the per-variety shape and cos-st-restricts case constraints for an
//     atomic, list or union t.
//   - the precisionDecimal scale-facet Schema Component Constraints
//     (maxScale-valid-restriction, minScale-valid-restriction, minScale ≤
//     maxScale, f-ms-fixed, f-mns-fixed), comparing t's {facets} against the
//     base's through EffectiveFacets.
//   - the count- and token-valued §4.3 facet Schema Component Constraints
//     (length/minLength/maxLength/totalDigits/fractionDigits valid restriction,
//     whiteSpace valid restriction, timezone valid restriction, and the
//     consistency SCCs minLength ≤ maxLength and fractionDigits ≤ totalDigits).
//     These are the part of cos-st-restricts clause 1.3.2 / 2.2.2.5 / 3.2.2.5
//     decidable without a value space.
//   - maxInclusive-maxExclusive (§4.3.8.4) and minInclusive-minExclusive
//     (§4.3.9.4), which forbid specifying both bounds of one side at one
//     derivation step. They read which facets t's own step specifies and no
//     {value}, so they need no value space either.
//   - length-minLength-maxLength (§4.3.1.4): clauses 1.1/2.1, the same-{facets}
//     value ordering, and clauses 1.2/2.2, the derivation-history demand that
//     length and the coexisting minLength/maxLength not be specified at one
//     step, over every ·restriction· step of the base chain.
//     GAP(xsd): clauses 1.2/2.2 are read the accepting way, so a minLength or
//     maxLength added at or below the step that introduced length is not
//     rejected, as the strictest reading of those clauses would; owned by
//     #555.
//   - the list- and union-variety applicable-facet sets, cos-st-restricts
//     clauses 2.2.2.4 and 3.2.2.4, as the fixed literals cos-applicable-facets
//     (§4.1.5) gives for those two varieties.
//   - the constructed-variety facet-shape clauses, 2.2.1.2 for a list and its
//     union sibling 3.2.1.2. A named list datatype such as xs:NMTOKENS
//     restricts an anonymous intermediate list (§3.4.5/§3.4.10/§3.4.12), so
//     2.2.1.2 does not touch it.
//
// st-props-correct clause 2 (the {base} chain terminates at a primitive or
// xs:anySimpleType — no circular derivation) is NOT charged here, and its
// verdict is PRESUPPOSED by every chain walk this method makes. With a deferred
// {base type definition} a cycle is representable, so the clause is a real
// check: Finalize charges it before it calls this method. A Schema-less caller
// running this method owes the same guarantee, and gets it for free on an
// OwnedSimpleType-only chain, where a base must pre-exist the type holding it.
//
// cos-st-restricts clause 3.3 (no-self-membership) is NOT charged here either,
// and its verdict is PRESUPPOSED by every membership walk this method makes, on
// the same terms. UnionDerivation.Members is a SimpleTypeOrRef sequence, so a
// by-name member can name a union that reaches back, and a walk over such a
// membership would not terminate. Finalize charges the clause before it calls
// this method. A Schema-less caller running this method owes the same
// guarantee, and gets it for free on an OwnedSimpleType-only membership, where
// every member must pre-exist the union holding it.
//
// Still deferred here, and why:
//
//   - cos-st-restricts clause 1.3.1 (a facet is applicable to an ATOMIC D) and
//     the value-space half of 1.3.2 / 2.2.2.5 / 3.2.2.5 — the four bound
//     facets' {value}s and enumeration, which need a lexical→value mapping —
//     and, for the same reason, the §4.3.7.4–§4.3.10.4 opposite-bound
//     consistency SCCs, the value-typed counterpart of the count-valued
//     minLength ≤ maxLength charged here. All live above this pure leaf:
//     applicability against the generated per-primitive table, the value-space
//     comparisons in value.CheckFacetRestriction. They are taken back as a
//     finalize-time capability, [SimpleTypeRestrictionChecker], to which
//     [SchemaBuilder.FinalizeWith] puts every simple type an assembled Schema
//     reaches; builtin.NewRestrictionChecker is the implementation that wires
//     the two halves together.
func (t *SimpleType) CheckDerivation(r TypeResolver) error {
	// Where each clause above is charged: st-props-correct clause 5 in
	// checkFacetsSupported; the per-variety cos-st-restricts cases in
	// checkAtomicGraph / checkListGraph / checkUnionGraph (1.1 stays live in
	// checkAtomicGraph; 2.2.1.2 and 3.2.1.2 in checkListGraph and
	// checkUnionGraph, the former deferred until builtin.Seed modeled the
	// anonymous intermediate list); the scale facets in checkScaleFacets; the
	// count- and token-valued SCCs in checkFacetRestrictions, the same-step
	// bound pairs in its checkSameStepBoundPairs, length-minLength-maxLength
	// 1.1/2.1 in checkLengthCoexistence and 1.2/2.2 in
	// checkLengthDerivationHistory, whose GAP marker carries the readings and
	// the consumer list (1.2/2.2 waited on #319's anonymous intermediate list);
	// the list/union applicable sets in checkVarietyApplicableFacets. The
	// absent-{variety} rejection is #480's. The once-only charge and the
	// verbatim src-resolve error the doc states are STYLE E2's. The method is
	// exported because one implementation serving both callers keeps the rule
	// single (STYLE T5/T4); the conformance datatypes lane is the second
	// Schema-less caller.
	//
	// Ordering: Schema.checkSimpleBaseAcyclic charges st-props-correct clause 2
	// in finalize's Phase B, and Schema.checkUnionMembershipAcyclic charges
	// cos-st-restricts clause 3.3 immediately after it, both BEFORE the Phase D
	// pass (Schema.checkSimpleTypeDerivations, resolve.go) that calls this one
	// (PRINCIPLES 9 for the base chain; PRINCIPLES 9 again for the membership
	// walks unionMembershipHasList and derivedOKSimple, which would not
	// terminate on a cycle). The restriction checker is applied by the same
	// checkSimpleTypeDerivations (restrictionchecker.go).
	if err := checkFacetsSupported(t.loc, t.ownFacets); err != nil {
		return err
	}
	base, err := t.Base(r)
	if err != nil {
		return err
	}
	if base != nil && finalContains(base.final, DerivationRestriction) {
		return xsderr.New(ruleSTPropsCorrect, t.loc,
			"simple type {base type definition} %s has restriction in its {final}, which blocks derivation (st-props-correct clause 3)", base.name)
	}
	if err := checkScaleFacets(r, t, base); err != nil {
		return err
	}
	if err := checkFacetRestrictions(r, t, base); err != nil {
		return err
	}
	variety, err := t.Variety(r)
	if err != nil {
		return err
	}
	if err := checkVarietyApplicableFacets(r, t, base, variety); err != nil {
		return err
	}
	if variety == nil && !t.IsAnySimpleType() {
		return xsderr.New(ruleSTPropsCorrect, t.loc,
			"simple type restricting %s has an absent {variety}, which only xs:anySimpleType may have (st-props-correct clause 1)", base.name)
	}
	switch variety.(type) {
	case Atomic:
		return checkAtomicGraph(r, t, base)
	case List:
		return checkListGraph(r, t, base)
	case Union:
		return checkUnionGraph(r, t, base)
	}
	return nil
}

// checkFacetsSupported enforces st-props-correct clause 5: each member of
// {facets} is supported by the processor. goxsd8 supports exactly the closed
// FacetKind set (the 14 core Constraining Facets plus the two precisionDecimal
// extension facets), so a facet whose kind falls outside that contiguous enum is
// unsupported. Every Facet built through this package carries a supported kind
// under the current static facet catalog, so this rejection is expected to be
// unreachable; it is emitted rather than skipped so an implementation-defined
// facet introduced later fails here with the right rule (§3.16.6.1 Note) instead
// of silently.
func checkFacetsSupported(loc xsderr.Loc, facets []Facet) error {
	for _, f := range facets {
		if f.kind < FacetLength || f.kind > FacetMinScale {
			return xsderr.New(ruleSTPropsCorrect, loc,
				"simple type {facets} contains an unsupported facet %s (st-props-correct clause 5)", f.kind)
		}
	}
	return nil
}

// checkAtomicGraph enforces the atomic-variety constraints on t (whose {variety}
// is Atomic):
//
//   - cos-st-restricts clause 1.1 — either t is xs:anyAtomicType, or its {base
//     type definition} is itself an atomic simple type definition. This is the
//     same requirement as the Datatypes §4.1.1 shape prose ("if {variety} is
//     atomic then the {variety} of {base type definition} must be atomic, unless
//     the base is anySimpleType"): the sole base=anySimpleType exception applies
//     only to xs:anyAtomicType (whose base xs:anySimpleType has an absent
//     {variety}), and xs:anyAtomicType is a package singleton never built through
//     this constructor.
//   - st-props-correct clause 1 against the property tableau's {primitive type
//     definition} clause (Datatypes §4.1.1 dc-defn: "required if {variety} is
//     atomic ... The exception is ·anyAtomicType·, whose {primitive type
//     definition} is ·absent·"): xs:anyAtomicType may not be named as the {base
//     type definition} of anything but a primitive datatype (#480, Datatypes
//     §2.4.2: "No ·user-defined· datatype may have anyAtomicType as its ·base
//     type·"). st-restrict-facets clause 2 makes a restriction's {primitive type
//     definition} the same as its base's, so a type restricting xs:anyAtomicType
//     inherits that singleton's ·absent· one while the tableau requires it
//     present — a contradiction, and one that needs no facet to trigger, unlike
//     the narrower facet-applicability rejection (cos-st-restricts clause 1.3.1,
//     charged above this package) that already covered the faceted schemas. The
//     built-in primitives are exempt by the same tableau: a primitive datatype's
//     {primitive type definition} is ITSELF (§3.16.1), which it reports because
//     it declares the package-private primitive derivation arm — minted only by
//     NewPrimitiveType and forgeable by no producer — so the check is keyed on
//     that arm (IsPrimitive) rather than on a "this is a builtin" flag.
//
// The check is deliberately keyed on the base being xs:anyAtomicType and NOT
// generalized to "every atomic type carries a present {primitive type
// definition}". A caller whose type inherits an absent {primitive type
// definition} from a base that HAS one is violating st-restrict-facets clause 2
// ("if {variety} is atomic, the {primitive type definition} of R is the same as
// that of B"), a different rule with no site here yet; charging it under
// st-props-correct would be charge-imprecise (STYLE E2).
//
// Clause 1.2 (B.{final} does not contain restriction) is discharged by
// CheckDerivation's clause-3 site (B is D's {base}); clause 1.3.2 is charged in
// part by checkFacetRestrictions and clause 1.3.1 above this package (see
// CheckDerivation). The clause-1.1 test reads only base's own derived {variety} —
// never t's {primitive type definition}, which self-references on a primitive
// datatype (§3.16.1) and so cannot drive a terminating base walk.
//
// The non-atomic-base rejection above the #480 site is no longer reachable
// through this package's EXPORTED constructors now that {variety} is derived: a
// RestrictionDerivation reports atomic only when its base already does, and the
// two arms that mint atomic on their own fix their base (NewPrimitiveType to the
// anchor) or are the anchor. It is kept rather than deleted because
// CheckDerivation runs on any *SimpleType, including the struct-literal
// receivers this package builds directly (both anchors are literals) — the same
// reason checkFacetsSupported keeps its own expected-unreachable rejection
// instead of dropping it. base is never nil here: Base answers nil only for the
// xs:anySimpleType anchor, whose {variety} is absent, and charges any other
// absent base before CheckDerivation dispatches on {variety}.
//
// base is t's already-resolved {base type definition}, passed down rather than
// re-resolved: CheckDerivation resolves it once per type so a by-name base costs
// one lookup for the whole check (STYLE D3 — the same fact, not a second one).
func checkAtomicGraph(r TypeResolver, t, base *SimpleType) error {
	if t == anyAtomicType {
		return nil
	}
	baseVariety, err := base.Variety(r)
	if err != nil {
		return err
	}
	if _, ok := baseVariety.(Atomic); !ok {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"atomic simple type {base type definition} %s is not an atomic simple type definition (cos-st-restricts clause 1.1)", base.name)
	}
	if base == anyAtomicType && !t.IsPrimitive() {
		return xsderr.New(ruleSTPropsCorrect, t.loc,
			"atomic simple type may not name %s as its {base type definition} unless it is a primitive datatype: it inherits that type's absent {primitive type definition} (st-restrict-facets clause 2), which the property tableau requires present (st-props-correct clause 1)", base.name)
	}
	return nil
}

// checkListGraph enforces the list-variety constraints on t (whose {variety} is
// List): cos-st-restricts clause 2. Clause 2.1 fixes the {item type definition}
// shape (not a special type; {variety} atomic, or union with no list type in its
// transitive membership). Clause 2.2 then branches on the constructed-vs-restricted
// discriminant B == xs:anySimpleType (grounding part D: the abstract model keys
// this off the resolved base, not off which XML element produced the type):
//
//   - constructed (B is xs:anySimpleType): clause 2.2.1.1 — the item's {final}
//     does not contain list; clause 2.2.1.2 — the closed facet shape, in
//     checkConstructedListFacets.
//   - restricted (B is a real list): clause 2.2.2.1 — B.{variety} is list; clause
//     2.2.2.3 — the item is validly derived from B's item (cos-st-derived-ok,
//     §3.16.6.3). Clause 2.2.2.2 (B.{final}) is discharged by CheckDerivation's
//     clause-3 site; clause 2.2.2.4 (facet applicability) by
//     checkVarietyApplicableFacets and clause 2.2.2.5 in part by
//     checkFacetRestrictions, both from CheckDerivation.
//
// The item is never absent and this function makes no absence check: t's
// {variety} is list only because a ListDerivation is on its base chain, and that
// arm's Item slot rejects every encoding of absence at construction (STYLE T1 —
// the type says it; SimpleTypeOrRef's arm × slot table). An unresolvable by-name
// itemType= is an error out of t.Item, not a nil.
func checkListGraph(r TypeResolver, t, base *SimpleType) error {
	item, err := t.Item(r)
	if err != nil {
		return err
	}
	if item.IsSpecial() {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list {item type definition} %s is a special type definition (cos-st-restricts clause 2.1)", item.name)
	}
	itemVariety, err := item.Variety(r)
	if err != nil {
		return err
	}
	switch itemVariety.(type) {
	case Atomic:
	case Union:
		hasList, err := unionMembershipHasList(r, item)
		if err != nil {
			return err
		}
		if hasList {
			return xsderr.New(ruleCosSTRestricts, t.loc,
				"list {item type definition} %s is a union with a list type in its transitive membership (cos-st-restricts clause 2.1)", item.name)
		}
	default:
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list {item type definition} %s has a {variety} that is neither atomic nor union (cos-st-restricts clause 2.1)", item.name)
	}

	if base == anySimpleType {
		if finalContains(item.final, DerivationList) {
			return xsderr.New(ruleCosSTRestricts, t.loc,
				"list {item type definition} %s has list in its {final}, blocking its use as a list item (cos-st-restricts clause 2.2.1.1)", item.name)
		}
		return checkConstructedListFacets(t)
	}
	baseVariety, err := base.Variety(r)
	if err != nil {
		return err
	}
	if _, ok := baseVariety.(List); !ok {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list simple type restricts base %s whose {variety} is not list (cos-st-restricts clause 2.2.2.1)", base.name)
	}
	baseItem, err := base.Item(r)
	if err != nil {
		return err
	}
	ok, err := derivedOKSimple(r, item, baseItem, nil)
	if err != nil {
		return err
	}
	if !ok {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list {item type definition} %s is not validly derived from the base list's item type (cos-st-restricts clause 2.2.2.3 via cos-st-derived-ok §3.16.6.3)", item.name)
	}
	return nil
}

// checkConstructedListFacets enforces cos-st-restricts clause 2.2.1.2 on a
// FRESHLY-CONSTRUCTED list t (B is xs:anySimpleType): "D.{facets} contains only
// the whiteSpace facet component with {value} = collapse and {fixed} = true".
// The set is CLOSED — any additional facet, a missing whiteSpace, a whiteSpace
// that is not collapse, or an unfixed one is a violation.
//
// It reads t.ownFacets directly because for a constructed list {facets} IS the
// own facet set: xs:anySimpleType carries no facets (§3.16.1), so the §3.16.6.4
// overlay has nothing to inherit — the same argument checkUnionGraph makes for
// the union sibling 3.2.1.2. "Only" needs no duplicate scan either: two members
// of the same kind are already refused by st-props-correct clause 4 in
// checkSTProps.
//
// A conforming mapping always satisfies this, because Structures §3.16.2.1
// map.std.common case 3 manufactures exactly that one-member set for every
// <list> alternative — which is also why the clause bites only on a
// programmatically built component, never on one parsed from a schema document:
// parser/produce.go's constructListType mints exactly that facet and no other.
func checkConstructedListFacets(t *SimpleType) error {
	for _, f := range t.ownFacets {
		if f.kind == FacetWhiteSpace {
			continue
		}
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list simple type constructed directly from xs:anySimpleType carries facet %s, but its {facets} must contain only whiteSpace = collapse with {fixed} = true (cos-st-restricts clause 2.2.1.2)", f.kind)
	}
	ws, ok := findFacet(t.ownFacets, FacetWhiteSpace)
	if !ok {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list simple type constructed directly from xs:anySimpleType has no whiteSpace facet, but its {facets} must contain whiteSpace = collapse with {fixed} = true (cos-st-restricts clause 2.2.1.2)")
	}
	if v := ws.Values(); len(v) != 1 || v[0] != "collapse" {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list simple type constructed directly from xs:anySimpleType has whiteSpace %q, but its {facets} must contain whiteSpace = collapse with {fixed} = true (cos-st-restricts clause 2.2.1.2)", v)
	}
	if fixed, _ := ws.Fixed(); !fixed {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"list simple type constructed directly from xs:anySimpleType has an unfixed whiteSpace facet, but its {facets} must contain whiteSpace = collapse with {fixed} = true (cos-st-restricts clause 2.2.1.2)")
	}
	return nil
}

// checkUnionGraph enforces the union-variety constraints on t (whose {variety}
// is Union): cos-st-restricts clause 3. Clause 3.1 excludes special type
// definitions from {member type definitions}. Clause 3.2 branches on the
// constructed-vs-restricted discriminant B == xs:anySimpleType:
//
//   - constructed (B is xs:anySimpleType): clause 3.2.1.1 — every member's
//     {final} does not contain union; clause 3.2.1.2 — {facets} is empty. A
//     freshly-constructed union has nothing to inherit (xs:anySimpleType carries
//     no facets, §3.16.1), so its {facets} IS its own facet set and the clause
//     reads directly off t.ownFacets — the same argument its list sibling
//     2.2.1.2 makes in checkConstructedListFacets. ·xs:error· (§3.16.7.3) is the
//     one seeded component that reaches this branch — the generated builtin
//     table defines no union, and xs:error's tableau is Structures', not a
//     table row — and it passes: its base IS xs:anySimpleType, its membership is
//     empty so 3.2.1.1 is vacuous, and it carries no facets.
//   - restricted (B is a real union): clause 3.2.2.1 — B.{variety} is union;
//     clause 3.2.2.3 — each member is validly derived from the CORRESPONDING
//     (positional, PRINCIPLES 11) base member (cos-st-derived-ok, §3.16.6.3).
//     Clause 3.2.2.2 (B.{final}) is discharged by CheckDerivation's clause-3 site;
//     clause 3.2.2.4 (facet applicability) by checkVarietyApplicableFacets and
//     clause 3.2.2.5 in part by checkFacetRestrictions, both from CheckDerivation.
//
// No member is ever absent and this function makes no absence check, for the
// reason checkListGraph states about its item: the UnionDerivation arm rejects
// every encoding of absence per ENTRY at construction (an empty membership is
// legal, an absent member is not).
//
// Clause 3.3 (no-self-membership) is not charged here: it quantifies over a
// whole assembled membership graph rather than over one type's own properties,
// so it is Phase B's checkUnionMembershipAcyclic — see CheckDerivation.
func checkUnionGraph(r TypeResolver, t, base *SimpleType) error {
	members, err := t.Members(r)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.IsSpecial() {
			return xsderr.New(ruleCosSTRestricts, t.loc,
				"union {member type definitions} contains special type definition %s (cos-st-restricts clause 3.1)", m.name)
		}
	}

	if base == anySimpleType {
		for _, m := range members {
			if finalContains(m.final, DerivationUnion) {
				return xsderr.New(ruleCosSTRestricts, t.loc,
					"union member %s has union in its {final}, blocking its use as a union member (cos-st-restricts clause 3.2.1.1)", m.name)
			}
		}
		if len(t.ownFacets) > 0 {
			return xsderr.New(ruleCosSTRestricts, t.loc,
				"union simple type constructed directly from xs:anySimpleType carries facet %s, but its {facets} must be empty (cos-st-restricts clause 3.2.1.2)", t.ownFacets[0].kind)
		}
		return nil
	}
	baseVariety, err := base.Variety(r)
	if err != nil {
		return err
	}
	if _, ok := baseVariety.(Union); !ok {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"union simple type restricts base %s whose {variety} is not union (cos-st-restricts clause 3.2.2.1)", base.name)
	}
	baseMembers, err := base.Members(r)
	if err != nil {
		return err
	}
	if len(members) != len(baseMembers) {
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"union restriction has %d member type definitions but base union %s has %d (cos-st-restricts clause 3.2.2.3)",
			len(members), base.name, len(baseMembers))
	}
	for i, m := range members {
		ok, err := derivedOKSimple(r, m, baseMembers[i], nil)
		if err != nil {
			return err
		}
		if !ok {
			return xsderr.New(ruleCosSTRestricts, t.loc,
				"union member %s is not validly derived from the corresponding base member (cos-st-restricts clause 3.2.2.3 via cos-st-derived-ok §3.16.6.3)", m.name)
		}
	}
	return nil
}

// derivedOKSimple reports whether d is validly derived from b per Type
// Derivation OK (Simple) (Structures §3.16.6.3, cos-st-derived-ok) subject to
// the blocking keywords in blocked — the "validly derived" relation invoked by
// cos-st-restricts clauses 2.2.2.3 and 3.2.2.3 (with no set) and by
// key-val-sub-type (with the caller's). It is a relation, not a rejection point:
// a false result is charged by its caller.
//
// Clause 1 (same type) is decided first. Every other step, D ≠ B, must pass
// clause 2.1 (stRestrictionUnblocked) and then one of clause 2.2's alternatives:
// 2.2.1 d.{base} = b; 2.2.2 d.{base} (never xs:anyType, a Complex Type Definition
// absent from this package) is itself validly derived from b given the same set;
// 2.2.3 d is a list or union and b is xs:anySimpleType; 2.2.4 b is a union whose
// {facets} are empty and d is validly derived from a member of b given the same
// set (recursion descends b's transitive membership, checking each intervening
// union's {facets} emptiness at its own level, clause 2.2.4.3). Clause 2.1 is
// read on every step, so it also governs the outer step of a 2.2.4 descent.
// A nil blocked is the empty set, which is what §3.16.6.3's closing Note makes
// "validly derived" with no set mean.
//
// It walks d's {base} chain and b's members with no visited set (STYLE D4). The
// base-chain side rests on the acyclicity proof CheckDerivation's doc states —
// Phase B's checkSimpleBaseAcyclic for a finalized Schema, owned-only chains for
// a Schema-less caller. The membership side rests on the same proof's other
// half: Phase B's checkUnionMembershipAcyclic, which charges cos-st-restricts
// clause 3.3, and owned-only memberships for a Schema-less caller. See
// CheckDerivation's clause 3.3 paragraph for the full statement.
//
// r resolves each {base type definition} hop, so an unresolvable one is an
// ERROR rather than a false "not derived": answering false for a base that
// merely could not be found would reject a valid restriction.
func derivedOKSimple(r TypeResolver, d, b *SimpleType, blocked []DerivationMethod) (bool, error) {
	if d == nil || b == nil {
		return false, nil
	}
	if d == b {
		return true, nil
	}
	unblocked, err := stRestrictionUnblocked(r, d, blocked)
	if err != nil || !unblocked {
		return false, err // clause 2.1
	}
	dBase, err := d.Base(r)
	if err != nil {
		return false, err
	}
	if dBase == b {
		return true, nil
	}
	if b == anySimpleType {
		dVariety, err := d.Variety(r)
		if err != nil {
			return false, err
		}
		switch dVariety.(type) {
		case List, Union:
			return true, nil
		}
	}
	if dBase != nil {
		ok, err := derivedOKSimple(r, dBase, b, blocked)
		if err != nil || ok {
			return ok, err
		}
	}
	bVariety, err := b.Variety(r)
	if err != nil {
		return false, err
	}
	if _, ok := bVariety.(Union); !ok {
		return false, nil
	}
	bFacets, err := b.EffectiveFacets(r)
	if err != nil {
		return false, err
	}
	if len(bFacets) != 0 {
		return false, nil
	}
	members, err := b.Members(r)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		ok, err := derivedOKSimple(r, d, m, blocked)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// stRestrictionUnblocked is cos-st-derived-ok clause 2.1 (§3.16.6.3) for one
// step whose D is d and whose B is not d: "restriction is not in S, or in
// D.{base type definition}.{final}". It reads the clause as "in neither": the
// step passes only when restriction is in neither blocked nor d's base's {final}.
// The literal wording is elliptical, and the suite settles it this way: MS-Element
// elemT018.i, elemT046.i and elemT074.i and ElemDecl typedef00802m2 Negative
// (xsi:type a restriction of a base with an empty {final}, under a declaration
// blocking restriction) and MS-Element elemS001/elemS005 (a substitution group
// member whose type restricts the head's, under final="restriction"/"#all") are
// all invalid, which the "not in S, or not in {final}" reading would accept.
//
// When d is xs:anySimpleType its {base type definition} is xs:anyType, whose
// {final} is empty (§3.4.7) and which this package does not model as a
// *SimpleType, so the {final} half is vacuous there.
//
// Under a nil blocked the clause reduces to its {final} half, which
// st-props-correct clause 3 (CheckDerivation) already demands of every simple
// type: a no-set caller's answer changes only on a chain that clause rejects.
func stRestrictionUnblocked(r TypeResolver, d *SimpleType, blocked []DerivationMethod) (bool, error) {
	if restrictionBlocked(blocked) {
		return false, nil
	}
	if d.IsAnySimpleType() {
		return true, nil
	}
	base, err := d.Base(r)
	if err != nil {
		return false, err
	}
	return !finalContains(base.final, DerivationRestriction), nil
}

// restrictionBlocked reports whether restriction is in the blocking set: the
// set test cos-st-derived-ok clause 2.1's first half (stRestrictionUnblocked)
// and cos-equiv-derived-ok-rec clause 2.3's simple arm
// (Schema.derivationAdmitsSubstitution) share. Neither reads the {final} half
// here.
func restrictionBlocked(blocked []DerivationMethod) bool {
	return containsDerivationMethod(blocked, DerivationRestriction)
}

// finalContains reports whether the {final} set contains derivation method d.
func finalContains(final []DerivationMethod, d DerivationMethod) bool {
	for _, f := range final {
		if f == d {
			return true
		}
	}
	return false
}

// checkScaleFacets enforces the five precisionDecimal scale-facet Schema
// Component Constraints at construction (see the rule constants above). It reads
// the {facets} property directly through SimpleType.EffectiveFacets (the
// §3.16.6.4 overlay), so a facet inherited unchanged through several restriction
// levels is compared transitively with no manual ancestor walk. base — t's
// already-resolved {base type definition} — is nil only for xs:anySimpleType,
// which carries no facets, so the base-relative SCCs are vacuous there; the
// minScale ≤ maxScale consistency SCC is not restriction-specific and runs on
// every type's own effective {facets}.
func checkScaleFacets(r TypeResolver, t, base *SimpleType) error {
	if base != nil {
		baseEff, err := base.EffectiveFacets(r)
		if err != nil {
			return err
		}
		if err := checkScaleValueRestriction(t.loc, t, baseEff, FacetMaxScale, ruleMaxScaleValidRestriction); err != nil {
			return err
		}
		if err := checkScaleValueRestriction(t.loc, t, baseEff, FacetMinScale, ruleMinScaleValidRestriction); err != nil {
			return err
		}
		if err := checkScaleFixed(t.loc, t, baseEff, FacetMaxScale, ruleMaxScaleFixed); err != nil {
			return err
		}
		if err := checkScaleFixed(t.loc, t, baseEff, FacetMinScale, ruleMinScaleFixed); err != nil {
			return err
		}
	}
	return checkScaleConsistency(r, t)
}

// checkScaleValueRestriction charges maxScale-valid-restriction (§4.2.4) or
// minScale-valid-restriction (§4.3.4): a restriction's own scale facet {value}
// may not relax the base's effective same-kind {value}. maxScale may only move
// down (own > base is the violation), minScale only up (own < base). Both are
// vacuous when the base has no effective facet of this kind, or when t declares
// no own facet of this kind (an inherited-only facet equals the base's effective
// value and cannot cross it).
func checkScaleValueRestriction(loc xsderr.Loc, t *SimpleType, baseEff []EffectiveFacet, kind FacetKind, rule xsderr.Rule) error {
	baseF, ok := findEffectiveFacet(baseEff, kind)
	if !ok {
		return nil
	}
	ownF, ok := findFacet(t.ownFacets, kind)
	if !ok {
		return nil
	}
	ownV, err := scaleValue(ownF, loc, rule)
	if err != nil {
		return err
	}
	baseV, err := scaleValue(baseF, loc, rule)
	if err != nil {
		return err
	}
	if !scaleRelaxes(kind, ownV, baseV) {
		return nil
	}
	return xsderr.New(rule, loc,
		"simple type restriction's own %s {value} %s relaxes the {base type definition}'s effective %s {value} %s, which restriction may not do (%s)",
		kind, ownV, kind, baseV, rule)
}

// scaleRelaxes reports whether an own scale {value} widens (relaxes) the base's,
// which restriction forbids: for maxScale a larger value widens the space, for
// minScale a smaller value does.
func scaleRelaxes(kind FacetKind, own, base integerLiteral) bool {
	if kind == FacetMaxScale {
		return own.cmp(base) > 0
	}
	return own.cmp(base) < 0
}

// checkScaleFixed charges f-ms-fixed (§4.2.1) or f-mns-fixed (§4.3.1): if the
// base's effective scale facet of this kind is {fixed}, a restriction may not
// specify its own scale facet with ANY value other than the base's — this is
// distinct from checkScaleValueRestriction, which a further-narrowing value can
// satisfy while still overriding a {fixed} base facet. Vacuous when the base has
// no such effective facet, the base facet is not {fixed}, or t declares no own
// facet of this kind.
func checkScaleFixed(loc xsderr.Loc, t *SimpleType, baseEff []EffectiveFacet, kind FacetKind, rule xsderr.Rule) error {
	baseF, ok := findEffectiveFacet(baseEff, kind)
	if !ok {
		return nil
	}
	if fixed, _ := baseF.Fixed(); !fixed {
		return nil
	}
	ownF, ok := findFacet(t.ownFacets, kind)
	if !ok {
		return nil
	}
	ownV, err := scaleValue(ownF, loc, rule)
	if err != nil {
		return err
	}
	baseV, err := scaleValue(baseF, loc, rule)
	if err != nil {
		return err
	}
	if ownV.cmp(baseV) == 0 {
		return nil
	}
	return xsderr.New(rule, loc,
		"simple type restriction sets %s {value} %s but the {base type definition}'s effective %s is {fixed} at %s and may not be overridden (%s)",
		kind, ownV, kind, baseV, rule)
}

// checkScaleConsistency charges the "minScale less than or equal to maxScale"
// SCC (spec anchor id minScale-totalDigits, a copy-paste bug — see
// ruleMinScaleLEMaxScale): it is not restriction-specific, so it runs against
// t's OWN effective {facets} after overlay. It rejects when both facets are in
// force and minScale's {value} exceeds maxScale's. The spec's Note explicitly
// disclaims any cross-check against totalDigits.
func checkScaleConsistency(r TypeResolver, t *SimpleType) error {
	loc := t.loc
	eff, err := t.EffectiveFacets(r)
	if err != nil {
		return err
	}
	minF, hasMin := findEffectiveFacet(eff, FacetMinScale)
	maxF, hasMax := findEffectiveFacet(eff, FacetMaxScale)
	if !hasMin || !hasMax {
		return nil
	}
	// A malformed {value} on either facet is charged under that facet's own
	// valid-restriction rule (the rule a bad literal on it would otherwise hit).
	minV, err := scaleValue(minF, loc, ruleMinScaleValidRestriction)
	if err != nil {
		return err
	}
	maxV, err := scaleValue(maxF, loc, ruleMaxScaleValidRestriction)
	if err != nil {
		return err
	}
	if minV.cmp(maxV) <= 0 {
		return nil
	}
	return xsderr.New(ruleMinScaleLEMaxScale, loc,
		"simple type {facets} has minScale {value} %s greater than maxScale {value} %s, violating \"minScale less than or equal to maxScale\" (its spec anchor id %s is a copy-paste bug)",
		minV, maxV, ruleMinScaleLEMaxScale)
}

// checkFacetRestrictions enforces the §4.3 Constraining Facet Schema Component
// Constraints whose operands are plain counts or keyword tokens — the half of
// cos-st-restricts clause 1.3.2 / 2.2.2.5 / 3.2.2.5 that needs no value space,
// so it can live in this pure leaf. Like checkScaleFacets it reads the {facets}
// property through EffectiveFacets (the §3.16.6.4 overlay), so a facet inherited
// unchanged through several restriction levels is compared transitively with no
// manual ancestor walk, and it compares t's OWN facets against the base's
// EFFECTIVE ones: an inherited-only facet equals the base's effective value and
// cannot cross it. base — t's already-resolved {base type definition} — is nil
// only for xs:anySimpleType, which carries no facets, so the base-relative SCCs
// are vacuous there; the count-facet consistency SCCs are not
// restriction-specific and run on every type's own effective {facets}, and the
// same-step bound-pair SCCs on every type's own facets
// (checkSameStepBoundPairs).
func checkFacetRestrictions(r TypeResolver, t, base *SimpleType) error {
	if base != nil {
		loc := t.loc
		baseEff, err := base.EffectiveFacets(r)
		if err != nil {
			return err
		}
		if err := checkCountRestriction(loc, t, baseEff, FacetLength, ruleLengthValidRestriction); err != nil {
			return err
		}
		if err := checkCountRestriction(loc, t, baseEff, FacetMinLength, ruleMinLengthValidRestriction); err != nil {
			return err
		}
		if err := checkCountRestriction(loc, t, baseEff, FacetMaxLength, ruleMaxLengthValidRestriction); err != nil {
			return err
		}
		if err := checkCountRestriction(loc, t, baseEff, FacetTotalDigits, ruleTotalDigitsValidRestriction); err != nil {
			return err
		}
		if err := checkCountRestriction(loc, t, baseEff, FacetFractionDigits, ruleFractionDigitsValidRestriction); err != nil {
			return err
		}
		if err := checkWhiteSpaceRestriction(loc, t, baseEff); err != nil {
			return err
		}
		if err := checkTimezoneRestriction(loc, t, baseEff); err != nil {
			return err
		}
	}
	if err := checkSameStepBoundPairs(t); err != nil {
		return err
	}
	return checkFacetConsistency(r, t)
}

// checkSameStepBoundPairs charges "maxInclusive and maxExclusive" (§4.3.8.4,
// id="maxInclusive-maxExclusive") and "minInclusive and minExclusive" (§4.3.9.4,
// id="minInclusive-minExclusive"): "It is an ·error· for both ... to be specified
// in the same derivation step of a Simple Type Definition."
//
// The operand is t's OWN facets — the set S its <restriction> specifies, which
// §3.16.6.4 overlays onto the base's {facets} — never EffectiveFacets. A pair
// split across two steps, one bound inherited and the other specified here, is
// legal: the overlay keeps both kinds in {facets}, but neither step specified
// both. No {value} is read, so the check needs no value space and holds however
// far a backend maps the base.
//
// It runs before any applicability clause — checkVarietyApplicableFacets, later
// in CheckDerivation, and 1.3.1 in the SimpleTypeRestrictionChecker after it —
// so a pair on a type to which bounds are not applicable at all is charged under
// the pair's rule. The SCC's text carries no applicability qualifier, and the
// schema is rejected either way.
func checkSameStepBoundPairs(t *SimpleType) error {
	if err := checkSameStepBoundPair(t, FacetMaxInclusive, FacetMaxExclusive, ruleMaxInclusiveMaxExclusive); err != nil {
		return err
	}
	return checkSameStepBoundPair(t, FacetMinInclusive, FacetMinExclusive, ruleMinInclusiveMinExclusive)
}

// checkSameStepBoundPair rejects t under rule when its own facets specify both
// inclusive and exclusive — one side of checkSameStepBoundPairs.
func checkSameStepBoundPair(t *SimpleType, inclusive, exclusive FacetKind, rule xsderr.Rule) error {
	if _, ok := findFacet(t.ownFacets, inclusive); !ok {
		return nil
	}
	if _, ok := findFacet(t.ownFacets, exclusive); !ok {
		return nil
	}
	return xsderr.New(rule, t.loc,
		"simple type restriction specifies both %s and %s in the same derivation step (%s)",
		inclusive, exclusive, rule)
}

// checkCountRestriction charges one of the five count-valued "<facet> valid
// restriction" SCCs (§4.3.1.4, §4.3.2.4, §4.3.3.4, §4.3.11.4, §4.3.12.4): a
// restriction's own {value} for kind may not relax the base's effective
// same-kind {value}, where "relax" is per-kind (countRelaxes). Vacuous when the
// base has no effective facet of this kind, or when t declares no own facet of
// this kind.
func checkCountRestriction(loc xsderr.Loc, t *SimpleType, baseEff []EffectiveFacet, kind FacetKind, rule xsderr.Rule) error {
	baseF, ok := findEffectiveFacet(baseEff, kind)
	if !ok {
		return nil
	}
	ownF, ok := findFacet(t.ownFacets, kind)
	if !ok {
		return nil
	}
	ownV, err := countValue(ownF, loc, rule)
	if err != nil {
		return err
	}
	baseV, err := countValue(baseF, loc, rule)
	if err != nil {
		return err
	}
	if !countRelaxes(kind, ownV, baseV) {
		return nil
	}
	return xsderr.New(rule, loc,
		"simple type restriction's own %s {value} %s %s the {base type definition}'s effective %s {value} %s (%s)",
		kind, ownV, countRequirement(kind), kind, baseV, rule)
}

// countRelaxes reports whether an own count {value} violates its kind's valid
// restriction SCC against the base's. length is the odd one out: §4.3.1.4 makes
// it an EQUALITY, not a narrowing — "it is a consequence of length valid
// restriction that the value of the length facet cannot be changed, regardless
// of whether {fixed} is true or false" (§4.3.1) — so any difference is a
// violation, and length needs no separate {fixed}-inheritance check. minLength
// may only move up; maxLength, totalDigits and fractionDigits only down.
func countRelaxes(kind FacetKind, own, base integerLiteral) bool {
	c := own.cmp(base)
	if kind == FacetLength {
		return c != 0
	}
	if kind == FacetMinLength {
		return c < 0
	}
	return c > 0
}

// countRequirement renders the requirement countRelaxes encodes, for the
// rejection message.
func countRequirement(kind FacetKind) string {
	if kind == FacetLength {
		return "does not equal"
	}
	if kind == FacetMinLength {
		return "is less than"
	}
	return "is greater than"
}

// checkWhiteSpaceRestriction charges whiteSpace valid restriction (§4.3.6.4):
// it is an error if the base has a whiteSpace facet and the restriction's own
// {value} is LESS restrictive than the base's. Vacuous when either side has no
// facet in force, and — deliberately — when either {value} is outside the
// three-token domain: that malformed-{value} rejection belongs to the whiteSpace
// facet's own {value} constraint (§4.3.6.1), charged by the normalization stage
// in package value, not to this restriction SCC (STYLE E2: one rejection, one
// rule).
func checkWhiteSpaceRestriction(loc xsderr.Loc, t *SimpleType, baseEff []EffectiveFacet) error {
	baseF, ok := findEffectiveFacet(baseEff, FacetWhiteSpace)
	if !ok {
		return nil
	}
	ownF, ok := findFacet(t.ownFacets, FacetWhiteSpace)
	if !ok {
		return nil
	}
	ownRank, ok := whiteSpaceRank(ownF)
	if !ok {
		return nil
	}
	baseRank, ok := whiteSpaceRank(baseF)
	if !ok {
		return nil
	}
	if ownRank >= baseRank {
		return nil
	}
	ownV, _ := singleValue(ownF)
	baseV, _ := singleValue(baseF)
	return xsderr.New(ruleWhiteSpaceValidRestriction, loc,
		"simple type restriction sets whiteSpace {value} %s, which is less restrictive than the {base type definition}'s effective %s (%s)",
		ownV, baseV, ruleWhiteSpaceValidRestriction)
}

// whiteSpaceRank ranks a whiteSpace facet's {value} by restrictiveness, ok=false
// for a {value} outside the domain. The ordering is preserve < replace <
// collapse, read off §4.3.6.4's two numbered error conditions ("{value} is
// replace or preserve and the parent's is collapse"; "{value} is preserve and
// the parent's is replace"), which between them make exactly those three pairs
// errors. The Note beneath them lists the keywords "in order of increasing
// restrictiveness" as preserve, collapse, replace — that ordering contradicts
// the numbered conditions it is summarizing and is read here as an editorial
// slip; the normative numbered conditions win (PRINCIPLES 25).
func whiteSpaceRank(f Facet) (int, bool) {
	v, ok := singleValue(f)
	if !ok {
		return 0, false
	}
	switch v {
	case "preserve":
		return 0, true
	case "replace":
		return 1, true
	case "collapse":
		return 2, true
	}
	return 0, false
}

// checkTimezoneRestriction charges timezone valid restriction (§4.3.14.4): once
// the base's effective explicitTimezone {value} is anything other than optional
// (i.e. required or prohibited), a restriction's own explicitTimezone must
// repeat that same {value}. A base {value} of optional may be narrowed to either
// required or prohibited, which is the only derivation this facet permits. It is
// vacuous when either side has no facet in force, and — as in
// checkWhiteSpaceRestriction — when either {value} is outside the three-token
// domain (§4.3.14.1), which is charged by the explicitTimezone facet stage in
// package value.
func checkTimezoneRestriction(loc xsderr.Loc, t *SimpleType, baseEff []EffectiveFacet) error {
	baseF, ok := findEffectiveFacet(baseEff, FacetExplicitTimezone)
	if !ok {
		return nil
	}
	baseV, ok := singleValue(baseF)
	if !ok || baseV == "optional" || !timezoneToken(baseV) {
		return nil
	}
	ownF, ok := findFacet(t.ownFacets, FacetExplicitTimezone)
	if !ok {
		return nil
	}
	ownV, ok := singleValue(ownF)
	if !ok || ownV == baseV || !timezoneToken(ownV) {
		return nil
	}
	return xsderr.New(ruleTimezoneValidRestriction, loc,
		"simple type restriction sets explicitTimezone {value} %s but the {base type definition}'s effective {value} is %s, which is not optional and so must be repeated verbatim (%s)",
		ownV, baseV, ruleTimezoneValidRestriction)
}

// timezoneToken reports whether v is in an explicitTimezone {value}'s
// three-token domain, optional, required or prohibited (§4.3.14.1 f-tz-value).
func timezoneToken(v string) bool {
	switch v {
	case "optional", "required", "prohibited":
		return true
	}
	return false
}

// checkFacetConsistency charges the count-facet consistency SCCs. They are NOT
// restriction-specific — each constrains any single type's {facets} — so they
// run against t's OWN effective {facets} after overlay, the
// checkScaleConsistency shape.
//
// length-minLength-maxLength is the one SCC in the group that does not fit that
// shape whole: its clauses 1.2/2.2 ask which DERIVATION STEP specified what, so
// checkLengthDerivationHistory takes t as well as the overlaid facets. It is
// called here rather than beside the *-valid-restriction checks because it is
// one half of the same SCC as checkLengthCoexistence, and because it constrains
// t even when t declares no own facet of either kind.
func checkFacetConsistency(r TypeResolver, t *SimpleType) error {
	loc := t.loc
	eff, err := t.EffectiveFacets(r)
	if err != nil {
		return err
	}
	if err := checkLengthCoexistence(loc, eff); err != nil {
		return err
	}
	if err := checkLengthDerivationHistory(r, t, eff); err != nil {
		return err
	}
	if err := checkCountOrder(loc, eff, FacetMinLength, FacetMaxLength,
		ruleMinLengthValidRestriction, ruleMaxLengthValidRestriction, ruleMinLengthLEMaxLength); err != nil {
		return err
	}
	return checkCountOrder(loc, eff, FacetFractionDigits, FacetTotalDigits,
		ruleFractionDigitsValidRestriction, ruleTotalDigitsValidRestriction, ruleFractionDigitsLETotalDigits)
}

// checkCountOrder charges a "lower <= upper" consistency SCC over two count
// facets that are both in force: minLength <= maxLength (§4.3.2.4,
// id="minLength-less-than-equal-to-maxLength") and fractionDigits <= totalDigits
// (§4.3.12.4, id="fractionDigits-totalDigits"). lowerRule/upperRule are the
// rules a malformed {value} on the respective facet is charged under (the rule a
// bad literal on it would otherwise hit — checkScaleConsistency's convention).
func checkCountOrder(loc xsderr.Loc, eff []EffectiveFacet, lower, upper FacetKind, lowerRule, upperRule, rule xsderr.Rule) error {
	lowerF, hasLower := findEffectiveFacet(eff, lower)
	upperF, hasUpper := findEffectiveFacet(eff, upper)
	if !hasLower || !hasUpper {
		return nil
	}
	lowerV, err := countValue(lowerF, loc, lowerRule)
	if err != nil {
		return err
	}
	upperV, err := countValue(upperF, loc, upperRule)
	if err != nil {
		return err
	}
	if lowerV.cmp(upperV) <= 0 {
		return nil
	}
	return xsderr.New(rule, loc,
		"simple type {facets} has %s {value} %s greater than %s {value} %s (%s)",
		lower, lowerV, upper, upperV, rule)
}

// checkLengthCoexistence charges "length and minLength or maxLength" (§4.3.1.4,
// id="length-minLength-maxLength") clauses 1.1 and 2.1: when length is in
// {facets}, a coexisting minLength may not exceed it and a coexisting maxLength
// may not fall below it.
//
// The same SCC's clauses 1.2 and 2.2 are a derivation-HISTORY predicate over the
// whole base chain rather than a property of one type's {facets}, so they are a
// separate check on a separate input: checkLengthDerivationHistory, which needs
// the type itself and not only its effective facets.
func checkLengthCoexistence(loc xsderr.Loc, eff []EffectiveFacet) error {
	lengthF, ok := findEffectiveFacet(eff, FacetLength)
	if !ok {
		return nil
	}
	lengthV, err := countValue(lengthF, loc, ruleLengthValidRestriction)
	if err != nil {
		return err
	}
	if minF, has := findEffectiveFacet(eff, FacetMinLength); has {
		minV, err := countValue(minF, loc, ruleMinLengthValidRestriction)
		if err != nil {
			return err
		}
		if minV.cmp(lengthV) > 0 {
			return xsderr.New(ruleLengthMinLengthMaxLength, loc,
				"simple type {facets} has minLength {value} %s greater than length {value} %s (%s clause 1.1)",
				minV, lengthV, ruleLengthMinLengthMaxLength)
		}
	}
	maxF, has := findEffectiveFacet(eff, FacetMaxLength)
	if !has {
		return nil
	}
	maxV, err := countValue(maxF, loc, ruleMaxLengthValidRestriction)
	if err != nil {
		return err
	}
	if lengthV.cmp(maxV) <= 0 {
		return nil
	}
	return xsderr.New(ruleLengthMinLengthMaxLength, loc,
		"simple type {facets} has length {value} %s greater than maxLength {value} %s (%s clause 2.1)",
		lengthV, maxV, ruleLengthMinLengthMaxLength)
}

// checkLengthDerivationHistory charges "length and minLength or maxLength"
// (§4.3.1.4, id="length-minLength-maxLength") clauses 1.2 and 2.2, the
// derivation-history half of the SCC whose same-{facets} half (1.1/2.1)
// checkLengthCoexistence charges. When length is in {facets}, a coexisting
// minLength is an error unless "there is some type definition from which this
// one is derived by one or more ·restriction· steps in which minLength has the
// same {value} and length is not specified" (clause 1.2), and mirror-image for
// maxLength (clause 2.2). Its effect is that length and a minLength/maxLength
// may not be SPECIFIED TOGETHER at one derivation step, however consistent their
// two values are — which is exactly how the XSD 1.0 erratum that introduced
// these clauses (E2-35) summarizes itself, and how the W3C suite's errF001
// testGroup annotation quotes it: "length facet is now allowed with either
// minLength or maxLength if they are specified in different derivation steps".
//
// The steps quantified over are every hop of the {base type definition} chain,
// not only facet-based <restriction> hops: §2.4.3's ·restriction· is the
// value/lexical-space-subset relation, whose Note records that ·construction· by
// ·list· or ·union· produces restrictions of the base type too, and ·derived·
// (dt-derived) is defined purely as that chain. So the walk here is
// EffectiveFacets's own base-chain walk with no hop kind special-cased — an
// anonymous intermediate list is a step like any other, and list being one of
// the two varieties this SCC governs (cos-applicable-facets), a candidate match
// rather than an inert pass-through. Union steps are walked through but can
// never match: the length family is not applicable to a union, so no union
// carries the facet the clause asks for.
func checkLengthDerivationHistory(r TypeResolver, t *SimpleType, eff []EffectiveFacet) error {
	if _, ok := findEffectiveFacet(eff, FacetLength); !ok {
		return nil
	}
	if err := checkLengthFreeStep(r, t, eff, FacetMinLength, ruleMinLengthValidRestriction, "1.2"); err != nil {
		return err
	}
	return checkLengthFreeStep(r, t, eff, FacetMaxLength, ruleMaxLengthValidRestriction, "2.2")
}

// checkLengthFreeStep runs one side of checkLengthDerivationHistory: kind is
// FacetMinLength (clause 1.2) or FacetMaxLength (clause 2.2), valueRule is the
// rule a malformed {value} on that facet is charged under (countValue's
// convention), and clause names the clause in the rejection. It is vacuous when
// kind is not in force at all — the clause only constrains a minLength/maxLength
// that actually coexists with length.
//
// It searches for a step of t's derivation at which the facet already had the
// {value} it has now and length was NOT SPECIFIED, walking t's base chain
// most-derived first. Two readings of the clause's wording are load-bearing, and
// both are resolved in the ACCEPTING direction — see the GAP below:
//
//   - "has the same {value}" is read against the candidate's {facets}, the only
//     facet set a component has (§4.1.1, overlaid per §3.16.6.4) — its
//     EffectiveFacets. The walk therefore stops at the first candidate that
//     cannot be in the clause's same-{value} span: one where the kind is absent
//     (the overlay never removes a facet kind, so nothing above it has the kind
//     either), or one whose {value} differs (minLength may only grow and
//     maxLength only shrink across a step per their valid-restriction SCCs, so
//     the same-{value} candidates are a contiguous prefix of the walk). "minLength
//     exists somewhere above" is emphatically not the predicate.
//   - "length is not specified" is read against the candidate's OWN facets — the
//     directly-specified set S of §3.16.6.4, not the overlay. The clause says
//     "specified" here where it says "is a member of {facets}" everywhere else,
//     and it is the reading the erratum's own "in different derivation steps"
//     summary states: a step that inherits length without restating it did not
//     specify it.
//
// t itself is a candidate step for the same reason. No cycle guard is needed or
// wanted (STYLE D4): the chain's acyclicity is the proof CheckDerivation's doc
// states, established by Phase B before this runs.
//
// GAP(xsd): owned by #555. The two readings above make this check REJECT LESS
// than the strictest reading of clauses 1.2/2.2 — one that admits only STRICT
// ancestors as candidates and reads "length is not specified" against the
// overlay. Under that stricter reading a minLength/maxLength introduced at or
// below the step that introduced length is also an error, so a schema deriving
// st2 from st = string(length=5) by adding maxLength=5 would be rejected. It is
// not rejected here. That shape is the W3C suite's
// MS-Errata102006-07-15/errF001, which the suite declares VALID while flagging
// its own expectation status="queried" against W3C bug 4681 since 2007-06-21 —
// i.e. the one published fixture bearing on the divergence reads it the
// accepting way, and the erratum's prose agrees with it, so the stricter reading
// is not adopted without an oracle grounding that reaches this fixture.
//
// The withheld rejection is an UNDER-rejection for every consumer of this
// error, so no valid schema can be false-rejected by it. The error returned here
// reaches exactly one place — checkFacetConsistency -> checkFacetRestrictions ->
// CheckDerivation — and a non-nil CheckDerivation return is the only thing any
// caller ever sees of it. Its readers are Schema.checkSimpleTypeDerivations,
// which returns it as the finalized schema's rejection (and through it
// parser.Produce and parser.Parse); builtin.Seed's build closure and
// builtin.interposeListBase, which fail Seed with it; the Schema-less
// conformance datatypes lane, which turns it into an ok=false decline; and any
// library caller of the exported CheckDerivation. Every one of them reads a
// WITHHELD error as a schema ACCEPTED that a stricter processor rejects; not one
// of them treats a missing error as grounds to reject anything, so the gap
// cannot turn into a false reject at any of them.
func checkLengthFreeStep(r TypeResolver, t *SimpleType, eff []EffectiveFacet, kind FacetKind, valueRule xsderr.Rule, clause string) error {
	loc := t.loc
	inForce, ok := findEffectiveFacet(eff, kind)
	if !ok {
		return nil
	}
	want, err := countValue(inForce, loc, valueRule)
	if err != nil {
		return err
	}
	for s := t; s != nil; {
		stepEff, err := s.EffectiveFacets(r)
		if err != nil {
			return err
		}
		stepF, has := findEffectiveFacet(stepEff, kind)
		if !has {
			break
		}
		stepV, err := countValue(stepF, loc, valueRule)
		if err != nil {
			return err
		}
		if stepV.cmp(want) != 0 {
			break
		}
		if _, specified := findFacet(s.ownFacets, FacetLength); !specified {
			return nil
		}
		next, err := s.Base(r)
		if err != nil {
			return err
		}
		s = next
	}
	return xsderr.New(ruleLengthMinLengthMaxLength, loc,
		"simple type {facets} has length alongside %s {value} %s, but every derivation step at which %s held that {value} also specified length (%s clause %s)",
		kind, want, kind, ruleLengthMinLengthMaxLength, clause)
}

// checkVarietyApplicableFacets enforces cos-st-restricts clauses 2.2.2.4 (list)
// and 3.2.2.4 (union): "All facets in {facets} are applicable to D, as specified
// in [Applicable Facets]". For those two varieties cos-applicable-facets
// (§4.1.5) gives the applicable set as a FIXED LITERAL list keyed off {variety}
// alone — no per-primitive table lookup, which is meaningful only for the atomic
// case — so the whole clause is decidable in this pure leaf.
//
// Both clauses live in the RESTRICTED branch of their case split (B is a real
// list/union), and the two varieties are therefore scoped differently here:
//
//   - LIST runs unconditionally, including on a freshly-constructed list.
//     cos-applicable-facets constrains {facets} unconditionally. On a
//     freshly-constructed list the clause that covers {facets}, 2.2.1.2, is
//     itself CHARGED — in checkListGraph, by checkConstructedListFacets, whose
//     closed-set test is strictly stronger than this one. This site is not
//     redundant with it: CheckDerivation runs checkVarietyApplicableFacets before
//     checkListGraph, so a facet that is inapplicable to a list at all is
//     rejected here, reported as an applicability violation rather than as a
//     closed-set shape violation. On that path the rejection is likewise
//     CHARGED UNDER 2.2.1.2, the clause the spec's case split actually selects
//     (applicableClause): 2.2.1.2 admits only a fixed collapse whiteSpace
//     facet, so every facet this check rejects there — none of which is
//     whiteSpace — violates it too, and naming 2.2.2.4 would name a clause not
//     in force (STYLE E2).
//   - UNION skips a freshly-constructed one (B is xs:anySimpleType). There the
//     spec's own branch is 3.2.1, whose clause 3.2.1.2 checkUnionGraph charges —
//     and it is STRICTLY stronger, rejecting every facet rather than only the
//     inapplicable ones. Leaving 3.2.2.4 to fire first would name a clause the
//     spec's case split has not selected (STYLE E2) while changing no verdict.
//
// The ATOMIC case, clause 1.3.1, is NOT charged here — its applicable set comes
// from the generated per-primitive table, which lives above this leaf, so it is
// charged at finalize by the installed SimpleTypeRestrictionChecker
// (restrictionchecker.go) instead of at construction.
func checkVarietyApplicableFacets(r TypeResolver, t, base *SimpleType, variety Variety) error {
	switch variety.(type) {
	case List:
		return checkApplicableFacetSet(r, t, base, "list", listApplicableFacet)
	case Union:
		if base == anySimpleType {
			return nil
		}
		return checkApplicableFacetSet(r, t, base, "union", unionApplicableFacet)
	}
	return nil
}

// checkApplicableFacetSet rejects the FIRST facet of t's {facets}, in the
// document order EffectiveFacets yields (STYLE D2), that applicable rejects.
// variety is the label naming t's own {variety} in the message; base is t's
// already-resolved {base type definition}, which applicableClause needs.
func checkApplicableFacetSet(r TypeResolver, t, base *SimpleType, variety string, applicable func(FacetKind) bool) error {
	eff, err := t.EffectiveFacets(r)
	if err != nil {
		return err
	}
	for _, ef := range eff {
		if applicable(ef.facet.kind) {
			continue
		}
		return xsderr.New(ruleCosSTRestricts, t.loc,
			"simple type {facets} carries %s, which is not applicable to a %s simple type definition (cos-st-restricts clause %s via cos-applicable-facets §4.1.5)",
			ef.facet.kind, variety, applicableClause(variety, base))
	}
	return nil
}

// applicableClause names the cos-st-restricts sub-clause t's applicable-facet
// rejection is charged under, following the spec's own case split rather than
// the {variety} alone: a FRESHLY-CONSTRUCTED list (B is xs:anySimpleType) is in
// branch 2.2.1, not 2.2.2, so its facet clause is 2.2.1.2 ("{facets} contains
// only the whiteSpace facet component with {value} = collapse and {fixed} =
// true") — 2.2.2.4 is charged only for a list that RESTRICTS a real list. The
// union path only ever reaches here in its restricted branch
// (checkVarietyApplicableFacets returns early for a constructed union, whose
// clause 3.2.1.2 checkUnionGraph charges), so union is always 3.2.2.4.
//
// It takes the variety LABEL and the already-resolved base its one caller
// already holds, rather than re-deriving both from t: re-resolving would repeat
// a lookup for no new fact (STYLE D3), and it keeps this message helper
// infallible.
func applicableClause(variety string, base *SimpleType) string {
	if variety != "list" {
		return "3.2.2.4"
	}
	if base == anySimpleType {
		return "2.2.1.2"
	}
	return "2.2.2.4"
}

// listApplicableFacet reports whether kind is applicable to a list-variety
// simple type definition. The set is the verbatim cos-applicable-facets (§4.1.5)
// literal: "If {variety} is list, then the applicable facets are assertions,
// length, minLength, maxLength, pattern, enumeration, and whiteSpace."
func listApplicableFacet(kind FacetKind) bool {
	switch kind {
	case FacetAssertions, FacetLength, FacetMinLength, FacetMaxLength,
		FacetPattern, FacetEnumeration, FacetWhiteSpace:
		return true
	default:
		return false
	}
}

// unionApplicableFacet reports whether kind is applicable to a union-variety
// simple type definition. The set is the verbatim cos-applicable-facets (§4.1.5)
// literal: "If {variety} is union, then the applicable facets are pattern,
// enumeration, and assertions."
func unionApplicableFacet(kind FacetKind) bool {
	switch kind {
	case FacetPattern, FacetEnumeration, FacetAssertions:
		return true
	default:
		return false
	}
}

// singleValue returns a facet's sole lexical {value}, ok=false when the facet
// does not carry exactly one — the shape every single-valued facet kind
// requires.
func singleValue(f Facet) (string, bool) {
	if len(f.values) != 1 {
		return "", false
	}
	return f.values[0], true
}

// countValue reads a count facet's single xs:nonNegativeInteger {value} (length,
// minLength, maxLength, totalDigits, fractionDigits). xs:nonNegativeInteger is
// unbounded (Datatypes §3.4.20), so a literal past math.MaxInt is a valid
// {value} and is returned whole, never charged. Like scaleValue it treats a
// wrong value count, or a literal that is not an integer or is negative, as a
// real validity rejection charged under rule as an *xsderr.Error — that {value}
// is user-supplied schema lexical data reachable through the public
// NewFacet/NewSimpleType API — mirroring value/facets.go's facetCount, which
// reads the identical {value}s at instance-validation time.
func countValue(f Facet, loc xsderr.Loc, rule xsderr.Rule) (integerLiteral, error) {
	v, ok := singleValue(f)
	if !ok {
		return "", xsderr.New(rule, loc,
			"%s facet must carry exactly one value, has %d", f.kind, len(f.values))
	}
	n, ok := new(big.Int).SetString(v, 10)
	if !ok || n.Sign() < 0 {
		return "", xsderr.New(rule, loc,
			"%s facet value %q is not a nonNegativeInteger", f.kind, v)
	}
	return integerLiteral(v), nil
}

// findFacet returns the own Facet of the given kind and whether it is present.
func findFacet(facets []Facet, kind FacetKind) (Facet, bool) {
	for _, f := range facets {
		if f.kind == kind {
			return f, true
		}
	}
	return Facet{}, false
}

// findEffectiveFacet returns the in-force Facet of the given kind from an
// EffectiveFacets result and whether it is present.
func findEffectiveFacet(facets []EffectiveFacet, kind FacetKind) (Facet, bool) {
	for _, ef := range facets {
		if ef.facet.kind == kind {
			return ef.facet, true
		}
	}
	return Facet{}, false
}

// scaleValue reads a scale facet's single xs:integer {value} (which may be
// negative — no nonNegativeInteger constraint). xs:integer is unbounded in both
// directions (Datatypes §3.4.13), so a literal past math.MaxInt or math.MinInt is
// a valid {value} and is returned whole, never charged. That {value} is
// user-supplied schema lexical data reachable through the public
// NewFacet/NewSimpleType API, which accepts arbitrary lexical strings for scale
// kinds, so a wrong value count or non-integer literal is a real validity
// rejection charged as an *xsderr.Error, not a package logic error — mirroring
// value/facets.go's facetInt for the exact same maxScale/minScale {value} at
// instance-validation time.
func scaleValue(f Facet, loc xsderr.Loc, rule xsderr.Rule) (integerLiteral, error) {
	v, ok := singleValue(f)
	if !ok {
		return "", xsderr.New(rule, loc,
			"%s facet must carry exactly one value, has %d", f.kind, len(f.values))
	}
	if _, ok := new(big.Int).SetString(v, 10); !ok {
		return "", xsderr.New(rule, loc,
			"%s facet value %q is not an integer", f.kind, v)
	}
	return integerLiteral(v), nil
}

// integerLiteral is an integer facet {value} held as the literal the document
// wrote, which countValue or scaleValue has checked is an optional sign followed
// by decimal digits. The count and scale facets' value spaces are unbounded, so
// the literal is never narrowed to a host int: cmp orders two of them by value,
// exactly however large, and a rejection names the literal itself.
type integerLiteral string

// cmp orders l and m by value: negative, zero or positive as l is less than,
// equal to or greater than m. Two spellings of one value, such as "+007" and
// "7", compare equal.
func (l integerLiteral) cmp(m integerLiteral) int {
	return l.value().Cmp(m.value())
}

// value is the integer l spells. l is checked at construction, so SetString
// cannot fail here.
func (l integerLiteral) value() *big.Int {
	v, _ := new(big.Int).SetString(string(l), 10)
	return v
}

// unionMembershipHasList reports whether any type in u's transitive membership
// (Datatypes id="dt-transitivemembership") has {variety} = list; it is the
// negative form of clause 2.1's "no types whose {variety} is list among the
// union's transitive membership". u is expected to be a union; a non-union u
// yields false. It recurses through member unions with no visited set (STYLE
// D4), which rests on the acyclicity proof CheckDerivation's clause 3.3
// paragraph states — Phase B's checkUnionMembershipAcyclic for a finalized
// Schema, owned-only memberships for a Schema-less caller — the same footing the
// base-chain walks take from checkSimpleBaseAcyclic.
func unionMembershipHasList(r TypeResolver, u *SimpleType) (bool, error) {
	uVariety, err := u.Variety(r)
	if err != nil {
		return false, err
	}
	if _, ok := uVariety.(Union); !ok {
		return false, nil
	}
	members, err := u.Members(r)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		mVariety, err := m.Variety(r)
		if err != nil {
			return false, err
		}
		switch mVariety.(type) {
		case List:
			return true, nil
		case Union:
			has, err := unionMembershipHasList(r, m)
			if err != nil || has {
				return has, err
			}
		}
	}
	return false, nil
}
