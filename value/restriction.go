package value

import (
	"errors"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/regex"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file holds the SCHEMA-CONSTRUCTION half of the facet machinery: the
// §4.3 Schema Component Constraints whose operands live in a VALUE SPACE and so
// need a Backend, which is exactly the part of cos-st-restricts clause 1.3.2 /
// 2.2.2.5 / 3.2.2.5 that package xsd — a pure leaf that cannot import this
// package — has to leave undone (xsd/derivation.go's checkFacetRestrictions
// charges the count- and token-valued rules there, and the two bound-facet SCCs
// that read no {value}: maxInclusive-maxExclusive and minInclusive-minExclusive,
// which forbid specifying both bounds of one side at one derivation step).
//
// It is the construction-time complement of facets.go: the same four bound
// facets and the same enumeration facet, but comparing a RESTRICTION's facet
// {value}s ONCE, when the type is built, rather than comparing an instance's
// value against those facets on every validated literal. Two SCC families are
// charged here, and they differ in what the derived facet is compared AGAINST:
// the "valid restriction" family compares it against the {base type
// definition}'s facets ACROSS the restriction step, while the "opposite bound"
// family (checkBoundConsistency) compares a lower bound against an upper bound
// declared at the SAME step.
//
// cos-pattern-restriction (§4.3.4.5, "it is an error if there is any member of
// the {value} of the pattern facet on the {base type definition} which is not
// also a member of the {value}") is deliberately NOT checked here: it is a
// structural invariant of xsd.SimpleType, not a runtime condition.
// SimpleType.EffectiveFacets computes {facets} by walking the WHOLE base chain
// and overlaying each level's own facets, and xsd's overlayFacet gives
// FacetPattern keep-both semantics (§4.3.4.2 xr-pattern: patterns at different
// derivation steps are ANDed, so the base's pattern facet survives as its own
// entry beside the derived one) — the base's pattern facet is retained verbatim
// regardless of what a caller supplies as own facets, so every member of its
// {value} is trivially still a member of {facets}. There is no reachable
// violating state to reject.

// The construction-time Schema Component Constraints this file charges: the
// §4.3 "valid restriction" siblings of facets.go's instance-time cvc-* rules,
// then the four same-step "opposite bound" constraints of §4.3.7.4–§4.3.10.4.
// Each string is a live entry in xsderr's generated catalog.
const (
	// ruleEnumerationValidRestriction is enumeration valid restriction (§4.3.5.5,
	// id="enumeration-valid-restriction"): every member of a restriction's
	// enumeration facet {value} must be in the ·value space· of the {base type
	// definition}.
	ruleEnumerationValidRestriction xsderr.Rule = "enumeration-valid-restriction"
	// ruleMaxInclusiveValidRestriction is maxInclusive valid restriction
	// (§4.3.7.4, id="maxInclusive-valid-restriction").
	ruleMaxInclusiveValidRestriction xsderr.Rule = "maxInclusive-valid-restriction"
	// ruleMaxExclusiveValidRestriction is maxExclusive valid restriction
	// (§4.3.8.4, id="maxExclusive-valid-restriction").
	ruleMaxExclusiveValidRestriction xsderr.Rule = "maxExclusive-valid-restriction"
	// ruleMinExclusiveValidRestriction is minExclusive valid restriction
	// (§4.3.9.4, id="minExclusive-valid-restriction").
	ruleMinExclusiveValidRestriction xsderr.Rule = "minExclusive-valid-restriction"
	// ruleMinInclusiveValidRestriction is minInclusive valid restriction
	// (§4.3.10.4, id="minInclusive-valid-restriction").
	ruleMinInclusiveValidRestriction xsderr.Rule = "minInclusive-valid-restriction"
	// ruleMinInclusiveLEMaxInclusive is minInclusive <= maxInclusive (§4.3.7.4,
	// id="minInclusive-less-than-equal-to-maxInclusive").
	ruleMinInclusiveLEMaxInclusive xsderr.Rule = "minInclusive-less-than-equal-to-maxInclusive"
	// ruleMinExclusiveLEMaxExclusive is minExclusive <= maxExclusive (§4.3.8.4,
	// id="minExclusive-less-than-equal-to-maxExclusive").
	ruleMinExclusiveLEMaxExclusive xsderr.Rule = "minExclusive-less-than-equal-to-maxExclusive"
	// ruleMinExclusiveLTMaxInclusive is minExclusive < maxInclusive (§4.3.9.4,
	// id="minExclusive-less-than-maxInclusive").
	ruleMinExclusiveLTMaxInclusive xsderr.Rule = "minExclusive-less-than-maxInclusive"
	// ruleMinInclusiveLTMaxExclusive is minInclusive < maxExclusive (§4.3.10.4,
	// id="minInclusive-less-than-maxExclusive").
	ruleMinInclusiveLTMaxExclusive xsderr.Rule = "minInclusive-less-than-maxExclusive"
)

// CheckFacetRestriction charges the value-space Schema Component Constraints
// relating a Simple Type Definition's own Constraining Facets to its {base type
// definition} — the half of cos-st-restricts clause 1.3.2 / 2.2.2.5 / 3.2.2.5
// that needs a lexical→value mapping:
//
//   - maxInclusive / maxExclusive / minInclusive / minExclusive valid restriction
//     (§4.3.7.4–§4.3.10.4). Each is a FOUR-WAY cross-check: a derived bound is
//     compared against every bound facet in the base's {facets}, not only the
//     homonymous one, because a derived minInclusive can undercut the base's
//     minExclusive without ever touching the base's minInclusive.
//   - enumeration valid restriction (§4.3.5.5): every member of a derived
//     enumeration facet's {value} must be in the ·value space· of the {base type
//     definition}.
//
// It also charges the four same-step opposite-bound SCCs of the same sections —
// minInclusive <= maxInclusive, minExclusive <= maxExclusive, minExclusive <
// maxInclusive, minInclusive < maxExclusive (boundConsistencyViolates). Those
// relate two of t's OWN facets to each other rather than to the base, so they
// are the one charge here that would still have work to do if the base carried
// no facets at all.
//
// It is the value-aware second half of a two-part charge whose entry point is the
// installed xsd.SimpleTypeRestrictionChecker (builtin.NewRestrictionChecker):
// that implementation charges the atomic applicable-facet clause 1.3.1 first and
// then delegates here. A schema finalized with no such checker installed gets
// neither check.
//
// b supplies the value space. When b has NO governing mapping for the base type,
// every comparison below is skipped and CheckFacetRestriction returns nil: an
// unmapped type is a gap in the BACKEND, not an invalidity in the schema, and
// turning it into a rejection would false-reject every schema written against a
// datatype the caller chose not to back. The same gap is still reported — as a
// real cvc-datatype-valid error, not silently — the moment an instance is
// validated against such a type (ValidateLexical).
//
// t's OWN facets are the derived side of every comparison, deliberately: a facet
// t merely INHERITS is the very same facet component the base carries, so
// comparing it against the base's {facets} is vacuous, and it was already charged
// against the base's own base when the base was constructed. This mirrors
// xsd/derivation.go's checkScaleValueRestriction.
//
// r resolves t's {base type definition}, which a compiled schema may defer by
// name (xsd.SimpleTypeOrRef). Like [ValidateLexical]'s it is a PARAMETER stored
// nowhere — not on restrictionCheck, whose fields are the three resolved facts
// one pass needs and not the capability that produced them. r is also the
// current schema whose {notation declarations} make up NOTATION's value space
// (§3.3.19), read through a Notations method as *xsd.Schema has; against a
// resolver without one that value space cannot be judged, so a NOTATION-valued
// enumeration member is held only to NOTATION's lexical mapping, never rejected
// as undeclared (xsd.SimpleTypeRestrictionChecker: a fault the implementation
// cannot judge is never a rejection).
func CheckFacetRestriction(b Backend, r xsd.TypeResolver, t *xsd.SimpleType) error {
	base, err := t.Base(r)
	if err != nil {
		return err
	}
	if base == nil {
		// t IS xs:anySimpleType: no {base type definition} to restrict, and it
		// carries no facets of its own (§3.16.1).
		return nil
	}
	m, ok, err := governingMapping(b, r, base, assertionsUndecided{})
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	ws, err := whiteSpaceInForce(r, base)
	if err != nil {
		return err
	}
	rc := restrictionCheck{mapping: m, whiteSpace: ws, base: base, owner: t}
	if err := rc.checkBoundConsistency(); err != nil {
		return err
	}
	if err := rc.checkBoundRestrictions(r); err != nil {
		return err
	}
	return rc.checkEnumerationRestriction(b, r)
}

// restrictionCheck is the resolved context one CheckFacetRestriction pass needs:
// the type under construction, the base's governing mapping, and the base's
// in-force whiteSpace mode. Resolving all three once and carrying them together
// keeps every BOUND facet {value} in this pass parsed the same way, which is what
// makes the resulting values comparable at all. (The enumeration check needs no
// mapping of its own: membership in the base's value space is decided by running
// each member through the base type definition itself, checkEnumerationRestriction.)
//
// The {base type definition} IS a field, and that is a change of fact rather
// than a duplication of one. It used to be omitted because owner.Base() was a
// free pointer read, so carrying it would have been a second encoding (STYLE
// D3); the slot is now a SimpleTypeOrRef whose by-name arm needs a resolver and
// can fail, so the resolved component is a RESULT — of exactly the same kind as
// mapping and whiteSpace beside it, each resolved once and carried so that every
// comparison in one pass is made against the same three facts. Re-resolving per
// method would instead need the resolver itself on this struct, which is what
// the capability contract forbids.
type restrictionCheck struct {
	mapping Mapping
	// whiteSpace is the mode in force on base — the type whose value space every
	// facet {value} compared in this pass must be a member of — or the zero mode
	// when none is (a union {variety}, or a base carrying no usable whiteSpace
	// facet). It is resolved by whiteSpaceInForce and APPLIED by facetValue, the
	// same pair that parses facet {value}s at instance-pipeline construction;
	// this pass adds no normalization of its own.
	whiteSpace whiteSpace
	// base is owner's resolved {base type definition}, non-nil by construction:
	// CheckFacetRestriction returns before building this value when the base is
	// absent (owner IS xs:anySimpleType).
	base  *xsd.SimpleType
	owner *xsd.SimpleType
}

// checkBoundConsistency charges the four same-step opposite-bound SCCs of
// §4.3.7.4–§4.3.10.4: a lower bound facet and an upper bound facet declared "for
// the same datatype" must leave the value space non-empty.
//
// It walks owner's OWN facets only, and that is the whole check rather than a
// narrowing of it. A pair in which the upper bound is INHERITED is already
// charged, by the valid-restriction family beside this one, as the derived lower
// bound failing against the base's upper bound; a pair in which BOTH are
// inherited was charged when the base itself was constructed, against the base's
// own facets. What is left over — both bounds declared at this step, where the
// base has nothing to say — is exactly what this pass adds.
//
// Both operands are parsed through the BASE type's mapping, the same one
// checkBoundRestrictions uses, so the two values are members of one space and
// their Cmp is meaningful; a malformed {value} is charged under the offending
// facet's own valid-restriction rule, not under the pairing's, so which pass
// notices it first does not change what it is reported as (STYLE E2).
//
// The loops walk owner's own facets in document order, outer over lower bounds
// and inner over upper bounds, so which pairing is reported first is
// deterministic (STYLE D2).
func (rc restrictionCheck) checkBoundConsistency() error {
	own := rc.owner.OwnFacets()
	for _, low := range own {
		if !isLowerBoundKind(low.Kind()) {
			continue
		}
		lowV, ordered, err := rc.boundLimit(low, boundRestrictionRule(low.Kind()))
		if err != nil {
			return err
		}
		if !ordered {
			continue
		}
		for _, up := range own {
			if !isUpperBoundKind(up.Kind()) {
				continue
			}
			upV, ordered, err := rc.boundLimit(up, boundRestrictionRule(up.Kind()))
			if err != nil {
				return err
			}
			if !ordered {
				continue
			}
			rule, violates := boundConsistencyViolates(low.Kind(), up.Kind(), lowV.Cmp(upV))
			if !violates {
				continue
			}
			return xsderr.New(rule, rc.owner.Loc(),
				"simple type restriction's own %s {value} %q and %s {value} %q leave an empty value space, which %s forbids",
				low.Kind(), boundLexical(low), up.Kind(), boundLexical(up), rule)
		}
	}
	return nil
}

// boundConsistencyViolates names the same-step opposite-bound Schema Component
// Constraint governing a lower bound facet of kind lower paired with an upper
// bound facet of kind upper, and reports whether ord — the ·ordering· of the
// LOWER {value} relative to the UPPER {value} — violates it. The first result is
// the zero Rule exactly when the pair is not a lower/upper pairing at all, which
// the callers already exclude.
//
// The four pairings are four DISTINCT rules, they are NOT filed under matching
// section numbers, and they do not agree on whether an EQUAL pair is an error —
// the two same-kind pairings (§4.3.7.4, §4.3.8.4) test "greater than" and
// accept an equal pair, while the two cross pairings (§4.3.9.4, §4.3.10.4)
// test "greater than or equal to" and reject one. Verbatim:
//
//   - §4.3.7.4, minInclusive <= maxInclusive: "It is an ·error· for the value
//     specified for ·minInclusive· to be greater than the value specified for
//     ·maxInclusive· for the same datatype."
//   - §4.3.8.4, minExclusive <= maxExclusive: "It is an ·error· for the value
//     specified for ·minExclusive· to be greater than the value specified for
//     ·maxExclusive· for the same datatype."
//   - §4.3.9.4, minExclusive < maxInclusive: "It is an ·error· for the value
//     specified for ·minExclusive· to be greater than or equal to the value
//     specified for ·maxInclusive· for the same datatype."
//   - §4.3.10.4, minInclusive < maxExclusive: "It is an ·error· for the value
//     specified for ·minInclusive· to be greater than or equal to the value
//     specified for ·maxExclusive· for the same datatype."
//
// Incomparable never violates, for the reason boundRestrictionViolates gives:
// every clause is a "greater than" test, which an incomparable pair on a
// partially ordered primitive (float/double, duration) does not satisfy.
func boundConsistencyViolates(lower, upper xsd.FacetKind, ord Ordering) (xsderr.Rule, bool) {
	switch {
	case lower == xsd.FacetMinInclusive && upper == xsd.FacetMaxInclusive:
		return ruleMinInclusiveLEMaxInclusive, ord == Greater
	case lower == xsd.FacetMinExclusive && upper == xsd.FacetMaxExclusive:
		return ruleMinExclusiveLEMaxExclusive, ord == Greater
	case lower == xsd.FacetMinExclusive && upper == xsd.FacetMaxInclusive:
		return ruleMinExclusiveLTMaxInclusive, ord == Greater || ord == Equal
	case lower == xsd.FacetMinInclusive && upper == xsd.FacetMaxExclusive:
		return ruleMinInclusiveLTMaxExclusive, ord == Greater || ord == Equal
	default:
		return "", false
	}
}

// checkBoundRestrictions charges the four bound-facet valid-restriction SCCs
// (§4.3.7.4–§4.3.10.4). BOTH operands of every comparison are parsed by ONE
// mapping — the BASE type's governing mapping — so the two values are members of
// the same space and their Cmp is meaningful; parsing the derived side in the
// derived type's own (possibly narrower) representation and the base side in the
// base's would compare across representations, which the widest-space rule
// (st-restrict-facets §3.16.6.4) exists to prevent.
//
// Both loops walk their operands in document order — t's own facets as declared,
// the base's {facets} as EffectiveFacets yields them — so which violation is
// reported first is deterministic (STYLE D2).
func (rc restrictionCheck) checkBoundRestrictions(r xsd.TypeResolver) error {
	for _, own := range rc.owner.OwnFacets() {
		kind := own.Kind()
		if !isBoundKind(kind) {
			continue
		}
		rule := boundRestrictionRule(kind)
		ownV, ordered, err := rc.boundLimit(own, rule)
		if err != nil {
			return err
		}
		if !ordered {
			continue
		}
		if err := rc.checkBoundAgainstBase(r, own, rule, ownV); err != nil {
			return err
		}
	}
	return nil
}

// checkBoundAgainstBase cross-checks ONE derived bound facet {value} against
// EVERY bound facet in the base's {facets}, per the four numbered conditions of
// that facet's valid-restriction SCC (boundRestrictionViolates).
//
// A malformed BASE-side operand is charged under the base facet's OWN
// valid-restriction rule (boundRestrictionRule(baseF.Kind())), not under rule —
// which names the DERIVED facet's SCC. A bad {value} on the base's minExclusive
// is a minExclusive-valid-restriction problem wherever it is noticed; reporting
// it as, say, maxInclusive-valid-restriction would name a constraint that has
// nothing to say about it (STYLE E2).
func (rc restrictionCheck) checkBoundAgainstBase(r xsd.TypeResolver, own xsd.Facet, rule xsderr.Rule, ownV Ordered) error {
	baseEff, err := rc.base.EffectiveFacets(r)
	if err != nil {
		return err
	}
	for _, ef := range baseEff {
		baseF := ef.Facet()
		if !isBoundKind(baseF.Kind()) {
			continue
		}
		baseV, ordered, err := rc.boundLimit(baseF, boundRestrictionRule(baseF.Kind()))
		if err != nil {
			return err
		}
		if !ordered {
			continue
		}
		clause, violates := boundRestrictionViolates(own.Kind(), baseF.Kind(), ownV.Cmp(baseV))
		if !violates {
			continue
		}
		return xsderr.New(rule, rc.owner.Loc(),
			"simple type restriction's own %s {value} %q is not a valid restriction of the {base type definition}'s %s {value} %q, which %s clause %d requires",
			own.Kind(), boundLexical(own), baseF.Kind(), boundLexical(baseF), rule, clause)
	}
	return nil
}

// boundLimit parses a bound facet's single {value} through the pass's mapping
// and returns it as an Ordered limit. ordered is false — with a nil error — when the parsed
// value does not implement Ordered, in which case the caller SKIPS this facet
// rather than rejecting: a bound facet on a non-ordered value space is an
// APPLICABILITY violation (cos-applicable-facets §4.1.5), already charged
// upstream by the xsd.SimpleTypeRestrictionChecker that delegates here, and
// re-charging it under a bound rule would name the wrong constraint (STYLE E2).
//
// A wrong {value} count, or a lexical the base type's mapping cannot parse, IS
// rejected under rule: every numbered condition of the SCC presupposes that the
// facet's {value} is a member of the base type definition's value space, so a
// {value} that is not one cannot satisfy the constraint.
func (rc restrictionCheck) boundLimit(f xsd.Facet, rule xsderr.Rule) (limit Ordered, ordered bool, err error) {
	values := f.Values()
	if len(values) != 1 {
		return nil, false, xsderr.New(rule, rc.owner.Loc(),
			"the %s facet carries %d values rather than one, so %s cannot be decided for it", f.Kind(), len(values), rule)
	}
	v, err := facetValue(rc.mapping, rc.whiteSpace, values[0], nil)
	if err != nil {
		return nil, false, xsderr.Wrap(rule, rc.owner.Loc(), err)
	}
	ord, ok := v.(Ordered)
	if !ok {
		return nil, false, nil
	}
	return ord, true, nil
}

// boundRestrictionViolates reports whether a derived bound facet of kind derived
// violates its valid-restriction SCC against a base bound facet of kind base,
// given ord — the ·ordering· of the DERIVED {value} relative to the BASE {value}
// — and the number of the SCC's clause the (derived, base) pair falls under. The
// clause is a function of the pair alone and ord decides only whether it is
// violated; both come from one switch arm, so the message's clause and the
// verdict cannot disagree. A kind outside the four bounds answers clause 0.
//
// Incomparable never violates: every numbered condition is a "greater than" /
// "less than" test, and on a partially ordered primitive (float/double) an
// incomparable pair satisfies none of them. That is the same reading facets.go's
// boundFacet.violates applies at instance time, where Incomparable is handled by
// its own separate clause rather than folded into the ordering tests.
func boundRestrictionViolates(derived, base xsd.FacetKind, ord Ordering) (clause int, violates bool) {
	switch derived {
	case xsd.FacetMaxInclusive:
		return maxInclusiveRestrictionViolates(base, ord)
	case xsd.FacetMaxExclusive:
		return maxExclusiveRestrictionViolates(base, ord)
	case xsd.FacetMinExclusive:
		return minExclusiveRestrictionViolates(base, ord)
	case xsd.FacetMinInclusive:
		return minInclusiveRestrictionViolates(base, ord)
	default:
		// Unreachable: every caller filters on isBoundKind first. Reported as
		// "no violation" rather than a panic because this is a predicate on
		// user-supplied schema data, not a capability assertion.
		return 0, false
	}
}

// maxInclusiveRestrictionViolates is maxInclusive valid restriction (§4.3.7.4),
// clause by clause: 1 {value} greater than the base's maxInclusive; 2 {value}
// greater than or equal to the base's maxExclusive; 3 {value} less than the
// base's minInclusive; 4 {value} less than or equal to the base's minExclusive.
func maxInclusiveRestrictionViolates(base xsd.FacetKind, ord Ordering) (clause int, violates bool) {
	switch base {
	case xsd.FacetMaxInclusive:
		return 1, ord == Greater
	case xsd.FacetMaxExclusive:
		return 2, ord == Greater || ord == Equal
	case xsd.FacetMinInclusive:
		return 3, ord == Less
	case xsd.FacetMinExclusive:
		return 4, ord == Less || ord == Equal
	default:
		return 0, false
	}
}

// maxExclusiveRestrictionViolates is maxExclusive valid restriction (§4.3.8.4),
// clause by clause: 1 {value} greater than the base's maxExclusive; 2 {value}
// greater than the base's maxInclusive; 3 {value} less than or equal to the
// base's minInclusive; 4 {value} less than or equal to the base's minExclusive.
// Clause 3 is the asymmetry a shared "interval" abstraction would erase: an
// exclusive upper bound EQUAL to the base's inclusive lower bound leaves an
// empty space and is an error, whereas the inclusive/inclusive pairing at
// maxInclusive clause 3 is not.
func maxExclusiveRestrictionViolates(base xsd.FacetKind, ord Ordering) (clause int, violates bool) {
	switch base {
	case xsd.FacetMaxExclusive:
		return 1, ord == Greater
	case xsd.FacetMaxInclusive:
		return 2, ord == Greater
	case xsd.FacetMinInclusive:
		return 3, ord == Less || ord == Equal
	case xsd.FacetMinExclusive:
		return 4, ord == Less || ord == Equal
	default:
		return 0, false
	}
}

// minExclusiveRestrictionViolates is minExclusive valid restriction (§4.3.9.4),
// clause by clause: 1 {value} less than the base's minExclusive; 2 {value} less
// than the base's minInclusive; 3 {value} greater than or equal to the base's
// maxInclusive; 4 {value} greater than or equal to the base's maxExclusive.
func minExclusiveRestrictionViolates(base xsd.FacetKind, ord Ordering) (clause int, violates bool) {
	switch base {
	case xsd.FacetMinExclusive:
		return 1, ord == Less
	case xsd.FacetMinInclusive:
		return 2, ord == Less
	case xsd.FacetMaxInclusive:
		return 3, ord == Greater || ord == Equal
	case xsd.FacetMaxExclusive:
		return 4, ord == Greater || ord == Equal
	default:
		return 0, false
	}
}

// minInclusiveRestrictionViolates is minInclusive valid restriction (§4.3.10.4),
// clause by clause: 1 {value} less than the base's minInclusive; 2 {value}
// greater than the base's maxInclusive (the spec's "is greater the {value} of
// that maxInclusive" is missing a "than"); 3 {value} less than or equal to the
// base's minExclusive; 4 {value} greater than or equal to the base's
// maxExclusive.
func minInclusiveRestrictionViolates(base xsd.FacetKind, ord Ordering) (clause int, violates bool) {
	switch base {
	case xsd.FacetMinInclusive:
		return 1, ord == Less
	case xsd.FacetMaxInclusive:
		return 2, ord == Greater
	case xsd.FacetMinExclusive:
		return 3, ord == Less || ord == Equal
	case xsd.FacetMaxExclusive:
		return 4, ord == Greater || ord == Equal
	default:
		return 0, false
	}
}

// checkEnumerationRestriction charges enumeration valid restriction (§4.3.5.5):
// "it is an error if any member of {value} is not in the ·value space· of {base
// type definition}". MEMBERSHIP in that value space is the whole test, and it is
// strictly narrower than "the base's governing mapping can parse this lexical":
// the base type definition's own facets carve its value space out of its
// primitive's, so `200` is a perfectly well-formed integer lexical that is NOT in
// xs:byte's value space (bounded to [-128,127] by inherited maxInclusive /
// minInclusive), and `-5` is not in xs:positiveInteger's. Parsing alone accepts
// both. Each member is therefore run through the ordinary lexical→value pipeline
// against the BASE TYPE DEFINITION itself (ValidateLexical), which applies the
// base's whiteSpace, its pattern facets, its governing mapping and its value
// facets — the base's full facet stack, which is exactly what "value space of
// {base type definition}" denotes.
//
// The base — not, as newEnumFacet does for the instance-time check, the type that
// DECLARED the facet. The two differ exactly where this SCC bites: a facet
// declared on the derived type D resolves to D's own type, whose facets may be
// narrower than the base's, so a member outside the base's value space could
// still validate there.
//
// Each member carries its own namespace context — the bindings in scope where its
// <enumeration> was written (§3.3.18) — threaded through the same memberContext
// newEnumFacet uses, so a QName/NOTATION member resolves its prefix against the
// declaring schema's scope rather than against nothing.
//
// b is the same Backend CheckFacetRestriction was handed; it is a parameter
// rather than a restrictionCheck field because only this check needs it, and its
// caller has already established that b governs the base (an unmapped base
// returns early there), so ValidateLexical's no-mapping error is unreachable from
// here. ValidateLexical holds a NOTATION member to the notations r declares
// where r answers notationDeclarer (§3.3.19), so a member naming none is outside
// the base's value space and charged here; this is the same predicate as
// enumeration-required-notation (§3.3.19), charged under §4.3.5.5.
//
// A facet-pipeline PRECONDITION fault in the BASE type's own facets is SKIPPED, not
// charged (IsFacetPrecondition, ValidateLexical) — the same "skip, don't
// mis-attribute" answer boundLimit gives an unordered bound {value} above, and for
// the same reason (STYLE E2). §4.3.5.5 asks whether a member is in the base's value
// space; a base carrying a facet not applicable to it has no well-defined value
// space to be in, so the fault is an APPLICABILITY violation of the base — the
// xsd.SimpleTypeRestrictionChecker's to charge, against the base, under §4.1.5 —
// and re-charging it here as enumeration-valid-restriction against the DERIVED type
// would name a constraint with nothing to say about it and reject a schema whose
// enumeration may be perfectly valid.
//
// An assertions-facet DECLINE in the base is SKIPPED on the same terms
// (IsAssertionDeclined): this check holds no XPath engine (assertionsUndecided),
// so a member every other facet of the base accepts reaches the base's
// assertions undecided, and charging that would reject every enumeration on a
// base carrying an assertions facet. GAP(value): a member the base's
// assertions would reject is therefore never charged under §4.3.5.5. The
// withheld value is this function's error, whose one reader is
// builtin's checkSimpleTypeRestriction, the xsd.SimpleTypeRestrictionChecker
// xsd's finalize charges on an error PRESENT, so the decline can only cost a
// schema rejection and the direction is fail-open. (#1042)
func (rc restrictionCheck) checkEnumerationRestriction(b Backend, r xsd.TypeResolver) error {
	for _, own := range rc.owner.OwnFacets() {
		if own.Kind() != xsd.FacetEnumeration {
			continue
		}
		// Kind() is FacetEnumeration here, so EnumerationMembers always reports
		// ok=true; the second result is discarded deliberately.
		members, _ := own.EnumerationMembers()
		for _, em := range members {
			_, err := ValidateLexical(b, r, rc.base, em.Lexical(), newMemberContext(em), assertionsUndecided{})
			if IsFacetPrecondition(err) || IsAssertionDeclined(err) {
				continue
			}
			if err != nil {
				return xsderr.Wrap(ruleEnumerationValidRestriction, rc.owner.Loc(), err)
			}
		}
	}
	return nil
}

// notationDeclarer is the capability of a resolver that holds the current
// schema's {notation declarations} (§3.17.1): *xsd.Schema answers it, and it is
// the resolver the finalize pass hands CheckFacetRestriction and the validator
// hands ValidateLexical. Against a resolver that does not answer it, NOTATION's
// ·value space· cannot be judged, so declaredNotation rejects nothing.
type notationDeclarer interface {
	Notations() []xsd.Notation
}

// The build pins notationDeclarer to the resolver finalize and the validator
// pass: a renamed or re-typed Schema.Notations would otherwise stop the probe
// matching silently, and every NOTATION value would go unchecked.
var _ notationDeclarer = (*xsd.Schema)(nil)

// declaredNotation is the one decider of NOTATION's ·value space·, "the set of
// QNames of notations declared in the current schema" (Datatypes §3.3.19),
// which a leaf mapping holding no schema cannot decide: validateLexical asks it
// of every atomic literal its mapping and value facets accepted, so a literal
// whose {primitive type definition} is NOTATION is rejected where its prefix is
// unbound or its QName names none of r's notations — whether NOTATION governs
// the type itself, a list item or a union member, each of which is an atomic
// validateLexical of its own. That is the instance-time verdict under
// cvc-datatype-valid, and, for an enumeration member, enumeration valid
// restriction (§4.3.5.5, checkEnumerationRestriction). It answers nil against
// an r that is no notationDeclarer, and for a variety that is not atomic.
//
// It runs after the value facets, so a literal outside an atomic NOTATION
// type's enumeration is that facet's cvc-enumeration-valid verdict; every
// member such an enumeration admits names a declared notation, finalize having
// charged one that does not. A list or union type's own enumeration runs after
// its items or members, so an undeclared item or member is this verdict first.
// It runs before the assertions stage, so an undeclared literal is this
// verdict, never an assertion's verdict or decline.
//
// r.Notations is the whole assembled set, so a notation from an included,
// imported or overriding document counts wherever it sits in document order
// (sch-props-correct, Structures §3.17.6.1). Only a NOTATION literal reads it.
func declaredNotation(r xsd.TypeResolver, st *xsd.SimpleType, variety xsd.Variety, lexical string, ctx Context) error {
	s, ok := r.(notationDeclarer)
	if !ok {
		return nil
	}
	if _, atomic := variety.(xsd.Atomic); !atomic {
		return nil
	}
	primitive, err := st.Primitive(r)
	if err != nil {
		return typeFault(err)
	}
	if primitive == nil || primitive.Name() != notationName {
		return nil
	}
	name, ok := resolveNotationLexical(lexical, ctx)
	if !ok {
		return xsderr.New(ruleCvcDatatypeValid, xsderr.Loc{},
			"the NOTATION value %q has a prefix no in-scope namespace binding declares, so it resolves to no QName and is outside the ·value space· of NOTATION, which cvc-datatype-valid clause 2.1 requires it to be in",
			lexical)
	}
	if slices.ContainsFunc(s.Notations(), func(n xsd.Notation) bool { return n.Name() == name }) {
		return nil
	}
	return xsderr.New(ruleCvcDatatypeValid, xsderr.Loc{},
		"the NOTATION value %q resolves to the QName %s, which names no notation declaration of the schema, so it is outside the ·value space· of NOTATION, \"the set of QNames of notations declared in the current schema\", which cvc-datatype-valid clause 2.1 requires it to be in",
		lexical, name)
}

// resolveNotationLexical is the expanded name a NOTATION lexical denotes: its
// prefix, or the empty prefix of an unprefixed name, resolved against ctx
// (§3.3.18). It reports false where ctx binds no namespace to the prefix: a
// NOTATION mapping that does not itself reject an unbound prefix must not turn
// p:n into the no-namespace n. A nil ctx binds nothing, the empty prefix
// included ([Context]).
func resolveNotationLexical(lexical string, ctx Context) (xsd.QName, bool) {
	if ctx == nil {
		return xsd.QName{}, false
	}
	prefix, local, prefixed := strings.Cut(lexical, ":")
	if !prefixed {
		prefix, local = "", lexical
	}
	space, ok := ctx.LookupNamespace(prefix)
	if !ok {
		return xsd.QName{}, false
	}
	return xsd.QName{Space: space, Local: local}, true
}

// isBoundKind reports whether kind is one of the four bound Constraining Facets
// (§4.3.7–§4.3.10) — the kinds whose {value} is a member of the type's value
// space and is compared through the ·ordering· relation.
func isBoundKind(kind xsd.FacetKind) bool {
	return isLowerBoundKind(kind) || isUpperBoundKind(kind)
}

// isLowerBoundKind reports whether kind is one of the two LOWER bound
// Constraining Facets (§4.3.9, §4.3.10).
func isLowerBoundKind(kind xsd.FacetKind) bool {
	switch kind {
	case xsd.FacetMinInclusive, xsd.FacetMinExclusive:
		return true
	default:
		return false
	}
}

// isUpperBoundKind reports whether kind is one of the two UPPER bound
// Constraining Facets (§4.3.7, §4.3.8).
func isUpperBoundKind(kind xsd.FacetKind) bool {
	switch kind {
	case xsd.FacetMaxInclusive, xsd.FacetMaxExclusive:
		return true
	default:
		return false
	}
}

// boundRestrictionRule maps a bound facet kind to its construction-time
// valid-restriction rule ID (§4.3.7.4–§4.3.10.4) — the schema-construction
// sibling of boundRule's instance-time cvc-* IDs, and it panics on a non-bound
// kind for the same reason: every caller filters on isBoundKind first, so
// reaching the default is a package-internal bug, not schema data.
func boundRestrictionRule(k xsd.FacetKind) xsderr.Rule {
	switch k {
	case xsd.FacetMaxInclusive:
		return ruleMaxInclusiveValidRestriction
	case xsd.FacetMaxExclusive:
		return ruleMaxExclusiveValidRestriction
	case xsd.FacetMinInclusive:
		return ruleMinInclusiveValidRestriction
	case xsd.FacetMinExclusive:
		return ruleMinExclusiveValidRestriction
	default:
		panic("value: boundRestrictionRule: " + k.String() + " is not a bound facet")
	}
}

// CheckPatternSyntax charges src-pattern-value (§4.3.4.3) on the <pattern>
// facets t DECLARES: each value must be a regular expression as Datatypes
// Appendix G defines one. It is the EAGER counterpart of facets.go's
// newPatternFacet, which charges the same rule off the same translation but
// only when an instance literal is first validated against t — so a schema
// nothing validates against, which is every schema-only conformance fixture,
// never reached it and a malformed pattern passed silently. Reach this the way
// CheckFacetRestriction is reached, from the xsd.SimpleTypeRestrictionChecker
// installed at xsd.SchemaBuilder.FinalizeWith: that walk visits every simple
// type a schema contains, anonymous inline ones included.
//
// t.OwnFacets is the operand, not EffectiveFacets, for the reason
// CheckFacetRestriction takes the same side: an INHERITED pattern is the very
// facet component the base carries, and the same walk already charged it there.
// Passing the accumulated overlay instead would re-charge one author's mistake
// once per type derived from it, attributing it to each derived type's
// position in turn.
//
// A pattern this module recognizes but cannot compile is NOT charged — see
// regex.CheckSyntax, which owns that distinction. Neither is a translated
// pattern RE2 then rejects: that failure has no reproduction in the corpus and
// stays where newPatternFacet already charges it, rather than widening a
// construction-time rejection past what Appendix G's grammar decides.
//
// No xsd.TypeResolver and no Backend: a pattern's syntax is a property of the
// lexical value alone, so nothing here walks the base chain or touches a value
// space.
func CheckPatternSyntax(t *xsd.SimpleType) error {
	for _, f := range t.OwnFacets() {
		if f.Kind() != xsd.FacetPattern {
			continue
		}
		for _, p := range f.Values() {
			err := regex.CheckSyntax(p, regex.FlavorXSD, "")
			if err == nil {
				continue
			}
			return xsderr.New(ruleSrcPatternValue, t.Loc(),
				"pattern facet value %q is not a regular expression as Datatypes Appendix G defines one, which src-pattern-value requires: %s",
				p, patternDetail(err))
		}
	}
	return nil
}

// patternDetail renders what regex.CheckSyntax reported, without the "loc:
// [rule]" prefix *xsderr.Error.Error adds. The regex translator charges
// src-pattern-value itself but at the zero Loc — it parses a bare string and
// has no document to point at — so CheckPatternSyntax re-charges the same rule
// at the type's own position and quotes only the explanation.
func patternDetail(err error) string {
	var e *xsderr.Error
	if errors.As(err, &e) {
		return e.Msg
	}
	return err.Error()
}

// boundLexical renders a bound facet's lexical {value} for an error message.
// boundLimit has already rejected any facet that does not carry exactly one
// value, so the empty fallback is message rendering only, never a validity
// decision.
func boundLexical(f xsd.Facet) string {
	values := f.Values()
	if len(values) != 1 {
		return ""
	}
	return values[0]
}
