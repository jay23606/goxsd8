package xsd

// This file answers ·substitution group· membership (§3.3.6.4, key-eq) — "is the
// element declaration named member ·substitutable· for the one named head?" — for
// the finalize-phase consumers that ask it: cvc-wildcard clause 3's ·implicit·
// containment (wildcardadmit.go), cos-content-act-restrict's ·element particle·
// admission (contentrestricts.go), cos-nonambig's ·overlap· relation
// (particleattribution.go), and cos-element-consistent's ·implicitly contains·
// (elementconsistent.go). It was lifted out of wildcardadmit.go, whose subject is
// wildcards, once the second and third consumers appeared (STYLE T4/T5).
//
// It decides membership with ONE predicate, inSubstitutionGroupOf; the exported
// SubstitutionGroup enumerates a head's group with it, for package codegen and
// for the two finalize consumers that need the whole group rather than one
// name's membership (elementconsistent.go, contentrestricts.go). The predicate
// is EXACT: Substitution Group OK (Transitive) (§3.3.6.3,
// cos-equiv-derived-ok-rec) is decided in full, clause 1 (same declaration),
// clause 2.1 ({disallowed substitutions}), clause 2.2 (the {substitution group
// affiliations} chain) and clause 2.3 (the ·derivation· blocking test) alike.
// Consumers therefore need no approximation direction: the answer is the spec's
// answer whichever polarity the calling constraint reads it in — one that
// DISALLOWS a name on membership (cvc-wildcard clause 3) and one that FIRES a
// schema component constraint on it (cos-nonambig, cos-element-consistent) both
// get the same true relation.
//
// Two walks run here, over two different edge kinds, and each terminates on its
// own licence (PRINCIPLES 9, no runtime cycle checks):
//
//   - the {substitution group affiliations} walk (clause 2.2) terminates because
//     Finalize's Phase B ran checkSubstitutionGroupsAcyclic (e-props-correct
//     clause 5) and rejected a circular affiliation graph, so the graph is
//     acyclic by construction on any *Schema that exists;
//   - the {base type definition} walk (clause 2.3) terminates because Phase B
//     also ran checkComplexBaseAcyclic (ct-props-correct clause 3), plus an
//     explicit xs:anyType test for the one self-based type §3.4.7 permits —
//     exactly the licence derivedOKComplex's walk records (complexderivation.go).
//     Where that walk crosses into the simple-type chain it needs no check at
//     all: a *SimpleType takes its {base type definition} as an already-built
//     component, so a cycle is unconstructible (simpletype.go records
//     st-props-correct clause 2 as a no-op for that reason) and the chain ends
//     at xs:anySimpleType, whose base is nil.

// SubstitutionGroup returns every top-level element declaration that is
// substitutable for head, head included — an {abstract} member among them,
// though cvc-elt clause 2 keeps an abstract declaration itself out of an
// instance. That is the ·substitution group· of the top-level element
// declaration named head (§3.3.6.4 key-eq): every declaration in {element
// declarations} that is ·substitutable· for it by Substitution Group OK
// (Transitive) (§3.3.6.3 cos-equiv-derived-ok-rec), clauses 1 through 2.3
// decided in full; {abstract} plays no part. head itself is always a member
// (clause 1), so the result is a group of one, len 1, when no declaration
// affiliates with head, when head's {disallowed substitutions} contains
// substitution (clause 2.1), or when the ·derivation· of each affiliate's {type
// definition} is blocked (clause 2.3). The declarations are in the order
// Elements returns them, so head sits at its own place in that order, not
// first, and members from any namespace of the schema set s holds are
// included, not only head's. The slice is fresh and the declarations are
// shared with s and immutable. It is nil exactly when s declares no top-level
// element named head: a local declaration heads no substitution group. It is
// exported for package codegen, which types a field over an <element ref> to
// head as a sum over this group.
func (s *Schema) SubstitutionGroup(head QName) []ElementDeclaration {
	// The scan is over the document-order slice, never elementIndex (STYLE D2);
	// each candidate's membership is the one exact predicate below (STYLE T4).
	var group []ElementDeclaration
	for _, e := range s.elements {
		if s.inSubstitutionGroupOf(e.Name(), head) {
			group = append(group, e)
		}
	}
	return group
}

// inSubstitutionGroupOf reports whether the element declaration named member is
// in the ·substitution group· of the one named head (§3.3.6.4, key-eq: "An
// element declaration is in the ·substitution group· of HEAD if and only if it is
// ·substitutable· for HEAD"), i.e. whether the two satisfy Substitution Group OK
// (Transitive) (§3.3.6.3, cos-equiv-derived-ok-rec):
//
//   - clause 1: M and H are the same element declaration. Name equality IS
//     component identity for these two operands: both name entries of {element
//     declarations}, which sch-props-correct clause 2 keeps unique by expanded
//     name. Folding it in is what makes this the ·substitution group· relation
//     rather than its irreflexive part — HEAD is always in its own group;
//   - clause 2.1: head's {disallowed substitutions} must not contain
//     substitution. The rule tests HEAD's property only — intermediate
//     declarations on the affiliation chain are not consulted, and the spec says
//     "intermediate" only in clause 2.3, about type definitions;
//   - clause 2.2: there is a chain of {substitution group affiliations} from
//     member to head;
//   - clause 2.3: the ·derivation· of member's {type definition} from head's is
//     not blocked (derivationAdmitsSubstitution).
//
// {abstract} plays NO part: it appears nowhere in cos-equiv-derived-ok-rec, so an
// abstract declaration genuinely in the substitution group still counts.
//
// Membership is decided by walking the affiliation chain UPWARD from member (one
// index point lookup per hop), not by scanning {element declarations} for every
// member of head's group: the question here is membership of ONE name, and the
// upward walk answers exactly that. No map is ever ranged, and no iteration order
// reaches the result (STYLE D2).
func (s *Schema) inSubstitutionGroupOf(member, head QName) bool {
	if member == head {
		return true // clause 1
	}
	h, ok := s.Element(head)
	if !ok {
		return false // a local declaration heads no substitution group
	}
	for _, d := range h.disallowedSubstitutions {
		if d == DerivationSubstitution {
			return false // clause 2.1
		}
	}
	m, ok := s.Element(member)
	if !ok {
		return false // a local declaration is in no substitution group
	}
	if !s.affiliationChainReaches(m, head) {
		return false // clause 2.2
	}
	return s.derivationAdmitsSubstitution(m, h) // clause 2.3
}

// affiliationChainReaches reports whether a chain of {substitution group
// affiliations} runs from the element declaration m to head
// (cos-equiv-derived-ok-rec clause 2.2: "either M.{substitution group
// affiliations} contains H, or M.{substitution group affiliations} contains a
// declaration whose {substitution group affiliations} contains H, or . . .").
// Affiliations are followed in document order (STYLE D2); the walk terminates
// without a visited set because Finalize rejected a circular affiliation graph
// (e-props-correct clause 5).
//
// An affiliation naming no declaration in the schema is an ·absent· member, which
// resolve.go deliberately does not reject (§5.3 Missing Sub-components — see
// resolveElementDecl). Skipping it is what §5.3 requires: no chain runs through a
// component that is not there.
func (s *Schema) affiliationChainReaches(m ElementDeclaration, head QName) bool {
	for _, aff := range m.substitutionGroupAffiliations {
		if aff == head {
			return true
		}
		next, ok := s.Element(aff)
		if !ok {
			continue // an ·absent· member (§5.3): there is no component to walk through
		}
		if s.affiliationChainReaches(next, head) {
			return true
		}
	}
	return false
}

// derivationAdmitsSubstitution is cos-equiv-derived-ok-rec clause 2.3: "The set
// of all {derivation method}s involved in the ·derivation· of M.{type definition}
// from H.{type definition} does not intersect with the union of (1)
// H.{disallowed substitutions}, (2) H.{type definition}.{prohibited
// substitutions} (if H.{type definition} is complex, otherwise the empty set),
// and (3) the {prohibited substitutions} (respectively the empty set) of any
// intermediate declared {type definition}s in the ·derivation· of M.{type
// definition} from H.{type definition}."
//
// The ·derivation· (§2.2.1.1, key-derived: "If a type definition D can reach a
// type definition B by following its base type definition chain, then D is said
// to be derived from B") is walked from M.{type definition} up the {base type
// definition} chain to H.{type definition}. Each step contributes the {derivation
// method} of the type STEPPED FROM, which is the same shape cos-ct-derived-ok's
// walk has (§3.4.6.5, derivedOKComplex): every level except the terminal target
// is read. {derivation method} and {prohibited substitutions} are Complex Type
// Definition properties (§3.4.1) — a simple type on the chain has neither, yet a
// restriction step from one counts as restriction in the first set (RULING
// #1942, arm A; see the walk's simple arm), and the walk CONTINUES THROUGH it rather
// than stopping there: key-derived follows the {base type definition} chain
// whatever kind the current type is, and a simple type has a base too. So a
// complex type based on a simple one (the <simpleContent> shape) still reaches
// an H.{type definition} sitting further up the simple chain, and the
// {derivation method}s collected below it are genuinely involved in that
// ·derivation·.
//
// The two sets are both collected before either is read: an intermediate type's
// {prohibited substitutions} blocks a {derivation method} contributed FURTHER
// DOWN the chain, so no incremental per-step test would be equivalent.
//
// Clause 2.3 is a BLOCKING clause, and a ·derivation· that does not exist blocks
// nothing: when the chain ends without reaching H.{type definition} the set of
// involved {derivation method}s is empty and so is the intersection, and this
// predicate is deliberately SILENT — reading clause 2.3 as requiring the chain
// would be legislating a different rule. The same reading covers an absent or
// unresolvable {type definition} on either side, which declaredTypeRestricts
// (defaultbinding.go) skips identically: there is no component to walk, and a
// dangling name was already charged src-resolve by resolve.go's Phase A.
//
// Requiring the chain to exist at all belongs to e-props-correct clause 4
// (§3.3.6.1, c-vs-sg: "For each member M of E.{substitution group affiliations},
// E.{type definition} is ·validly substitutable· for M.{type definition},
// subject to the blocking keywords in M.{substitution group exclusions}"), and
// the silence above IS delegation to a check that exists: checkSubstitutionGroupTypes
// (substitutiongrouptypes.go) charges clause 4 at finalize, so no *Schema
// carrying an affiliation edge whose member type does not ·derive· from its
// head's reaches this predicate at all (#395).
//
// EVERY step is reached through ResolvedType, the package's one type-slot reader
// (STYLE T4) — the starting {type definition} and each {base type definition}
// after it — so an ANONYMOUS type participates in the walk rather than ending
// it. That matters in both directions: an anonymous inline type is M's own first
// step, and the anonymous src-expredef clause 1.1 original a redefining complex
// type owns is an intermediate step. Ending the walk on one would return TRUE, a
// fail-OPEN accept and not a conservative refusal (#505). No map is ranged
// (STYLE D2). An anonymous H.{type definition} can end the walk only by being
// M.{type definition} itself, since sameTypeDefinition reports two anonymous
// types as distinct on the licence §3.4.6.5's no-identity Note grants.
func (s *Schema) derivationAdmitsSubstitution(m, h ElementDeclaration) bool {
	headType, ok := s.ResolvedType(h.TypeDefinition())
	if !ok {
		return true
	}
	memberType, ok := s.ResolvedType(m.TypeDefinition())
	if !ok {
		return true
	}
	if sameTypeDefinition(memberType, headType) {
		return true // no derivation step, so no {derivation method} is involved
	}
	blocked := h.DisallowedSubstitutions() // union member (1), copied by the accessor
	if hc, ok := headType.(ComplexType); ok {
		blocked = unionDerivationMethods(blocked, hc.prohibitedSubstitutions) // union member (2)
	}
	var methods []DerivationMethod
	simpleRestricts := false
	cur := memberType
walk:
	for step := 0; ; step++ {
		switch c := cur.(type) {
		case ComplexType:
			if !containsDerivationMethod(methods, c.DerivationMethod()) {
				methods = append(methods, c.DerivationMethod())
			}
			if step > 0 {
				// Union member (3): the intermediate types are those STRICTLY
				// between M.{type definition} (step 0) and H.{type definition}
				// (where the walk stops), so M's own {prohibited substitutions} is
				// not in the union and H's entered it as union member (2).
				blocked = unionDerivationMethods(blocked, c.prohibitedSubstitutions)
			}
			if c.Name() == anyTypeName {
				return true // §3.4.7: xs:anyType is its own base, so the chain ends here
			}
			// The slot is followed through ResolvedType, BOTH arms: an anonymous
			// src-expredef clause 1.1 base is a real step of the ·derivation·,
			// and stopping on it would answer true — a fail-OPEN accept, not a
			// conservative refusal (#505).
			next, ok := s.ResolvedType(c.Base())
			if !ok {
				// An absent base ends the chain short of H.{type definition}; a
				// dangling one was already charged src-resolve by Phase A.
				return true
			}
			if sameTypeDefinition(next, headType) {
				break walk // the ·derivation· has reached H.{type definition}
			}
			cur = next
		case *SimpleType:
			// A simple type contributes to NEITHER set — {derivation method} and
			// {prohibited substitutions} are both Complex Type Definition
			// properties (§3.4.1) — but the ·derivation· runs THROUGH it: key-derived
			// follows {base type definition} whatever kind the current type is, and
			// a simple type's is the *SimpleType Base reports.
			//
			// A restriction step of it nonetheless counts as restriction in
			// clause 2.3's union (RULING #1942, arm A). The letter of 2.3 defines
			// {derivation method} for complex types only, so this reads intent:
			// the explicit-values prose (§3.3.1, "rule out element declarations
			// having types whose derivation from {type definition} involves any
			// extension steps, or restriction steps") and cos-st-derived-ok clause
			// 2.1, whose vocabulary names a simple step a restriction; the suite
			// agrees (elemT063.i, disallowedsubst00501m2 Negative). A list or union
			// step contributes nothing: the method is only extension|restriction.
			// The blocking-set test is restrictionBlocked, shared with
			// stRestrictionUnblocked; clause 2.3 does not read {final}.
			if !isListOrUnionStep(c) {
				simpleRestricts = true
			}
			base, err := c.Base(s)
			if err != nil || base == nil {
				// An absent base is xs:anySimpleType topping the chain short of
				// H.{type definition}. An unresolvable one is the same non-answer
				// here: Phase A already charged src-resolve for it, and this
				// predicate has no error to return, so it takes the accepting
				// branch rather than inventing a blocking verdict.
				return true
			}
			if sameTypeDefinition(base, headType) {
				break walk // the ·derivation· has reached H.{type definition}
			}
			cur = base
		default:
			panic("xsd: derivationAdmitsSubstitution: non-exhaustive TypeDefinition switch")
		}
	}
	if simpleRestricts && restrictionBlocked(blocked) {
		return false
	}
	for _, d := range methods {
		if containsDerivationMethod(blocked, d) {
			return false
		}
	}
	return true
}

// isListOrUnionStep reports whether t's own step from its {base type
// definition} is a ·list· or ·union· construction (§3.16.2.1) rather than a
// restriction.
func isListOrUnionStep(t *SimpleType) bool {
	switch t.derivation.(type) {
	case ListDerivation, UnionDerivation:
		return true
	}
	return false
}
