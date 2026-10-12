package xsd

import "github.com/kud360/goxsd8/xsderr"

// This file synthesizes the ONE component cos-ct-extends (§3.4.6.2) clause 1.5
// needs and no schema document expresses: the collapsed intermediate the clause's
// Note prescribes — "simply re-order the ·derivation· to put all the extension
// steps first, then collapse them into a single extension".
//
// The clause asks whether T can be ·derived· in two steps from the ancestor A
// whose {base type definition} is ·xs:anyType·: an extension to some
// intermediate, then a possibly vacuous restriction to T. Call that intermediate
// M. For a chain whose every step is an extension the re-ordering is the identity
// and M IS T, which is why the pure chain is decided by proof and never comes
// here (complexextension.go, #264). For a chain that MIXES the two methods the
// re-ordering moves the extension steps ahead of the restriction steps, and M has
// to be built: it is A with every extension step's OWN contribution applied, in
// chain order, and the restriction steps dropped.
//
// "The step's OWN contribution" is the whole difficulty. The finalize graph
// stores only the FOLDED value of each property — the §3.4.2.4 clause 3
// {attribute uses}, the §3.4.2.5 clause 2 {attribute wildcard}, the §3.4.2.3.3
// clause 4 {content type} — each already merged with its immediate base's. So
// each property is recovered from what that graph holds — by inverting its own
// fold, or by reading a value retained past it — and each recovery states its
// exactness or its approximation where it is written:
//
//   - {attribute uses}: EXACT, and by retention rather than inversion —
//     extensionStepAttributeUses (attributeusefold.go) reads back the
//     clause-1-and-2 value the fold kept (ownAttributeUses, complextype.go),
//     because clause 3.1's fold is not invertible.
//   - {content type}: EXACT for every source-derived component, by
//     recoverExtensionStepContent's structural inverse (extensioncontenttype.go),
//     which DECLINES rather than guesses on a shape the merge does not build.
//   - {attribute wildcard}: OVER-approximated, deliberately —
//     collapsedAttributeWildcard.
//   - {assertions} and {final}: A's and empty respectively —
//     newCollapsedExtension (complextype.go).
//
// M is a value that lives for the length of one constraint check. It is NEVER
// written into s.types or s.typeIndex, and no error charged to a user ever names
// it: checkExtensionTwoStepDerivable consumes the verdict and charges
// cos-ct-extends at T's own position (complexextension.go).

// collapsedProperties is the collapse in progress: the three properties that
// accumulate as each extension step is re-applied. It is a parameter object
// rather than three values threaded through the loop, and it is discarded once M
// is constructed — nothing derived from it outlives collapsedExtension.
type collapsedProperties struct {
	uses     []AttributeUse
	wildcard *Wildcard // nil is the ·absent· property, as everywhere in §3.4.2.5
	content  ContentType
}

// collapsedExtension synthesizes M for t: the type the §3.4.6.2 Note's
// re-ordering yields, an extension of the ancestor A whose {base type definition}
// is ·xs:anyType·, carrying every extension step's own contribution and none of
// the restriction steps'.
//
// The second result is false when the collapse is not statable — the chain leaves
// the complex-type graph before reaching such an ancestor (an unresolvable or
// simple base), or one step's own contribution is not recoverable. See
// checkExtensionTwoStepDerivable for that decline's direction and owner.
//
// The steps are applied bottom-up, nearest A first, so each one's merge sees the
// collapse so far as its base — which is what "collapse them into a single
// extension" means, and is why the §3.4.2.3.3 clause 4 merge is applied
// REPEATEDLY here, over an order no document expresses. T's own step is applied
// last: clause 1.5 fires only where T.{derivation method} is extension, so T is
// always the topmost extension step.
func (s *Schema) collapsedExtension(t ComplexType) (ComplexType, bool, error) {
	steps, a, ok := s.baseChainToAnyType(t)
	if !ok {
		return ComplexType{}, false, nil
	}
	acc := collapsedProperties{
		uses: a.attributeUses,
		// The FOLDED {attribute wildcard}, which for A is §3.4.2.5 clause 2's
		// output; attributeWildcardProperty is the accessor, and after Phase D's
		// fold the value it reads back is the folded one — which is precisely the
		// second of that accessor's two documented readings (#505).
		wildcard: attributeWildcardProperty(a),
		content:  a.ContentType(),
	}
	for i := len(steps) - 1; i >= 0; i-- {
		c := steps[i]
		if c.DerivationMethod() != DerivationExtension {
			continue // the Note's re-ordering IS this skip
		}
		next, ok, err := s.applyExtensionStep(t.Loc(), acc, c)
		if err != nil || !ok {
			return ComplexType{}, false, err
		}
		acc = next
	}
	m, err := newCollapsedExtension(t.Loc(), a, acc)
	if err != nil {
		return ComplexType{}, false, err
	}
	return m, true, nil
}

// applyExtensionStep re-applies one extension step's own contribution to the
// collapse so far, over the step's REAL immediate base b — which is what the
// three inversions read the contribution out against — while the merge that
// re-applies it takes the collapse as its base instead. That asymmetry IS the
// re-ordering: the step contributes what it contributed, to whatever now precedes
// it.
//
// It answers false, without an error, on the two recoveries that decline.
func (s *Schema) applyExtensionStep(loc xsderr.Loc, acc collapsedProperties, c ComplexType) (collapsedProperties, bool, error) {
	b, ok := s.baseComplexType(c)
	if !ok {
		return collapsedProperties{}, false, nil
	}
	own, ok := s.extensionStepAttributeUses(c, b)
	if !ok {
		return collapsedProperties{}, false, nil
	}
	wildcard, err := collapsedAttributeWildcard(loc, c, acc.wildcard)
	if err != nil {
		return collapsedProperties{}, false, err
	}
	content, ok, err := s.collapsedContentType(loc, c, b, acc.content)
	if err != nil || !ok {
		return collapsedProperties{}, false, err
	}
	return collapsedProperties{
		uses:     s.collapsedAttributeUses(own, acc.uses),
		wildcard: wildcard,
		content:  content,
	}, true, nil
}

// collapsedAttributeUses folds one extension step's own {attribute uses} into the
// collapse through §3.4.2.4 clause 3.1 (inheritAttributeUses, the one encoding of
// that fold), with ONE thing the re-ordering forces and the real chain never
// meets: an own use for a name the collapse ALREADY carries is dropped, and the
// inherited one stands.
//
// It is dropped because no legal intermediate can hold both. Clause 3.1 inherits
// every base use unconditionally, so an extension of the collapse-so-far that
// re-declared the name would give the intermediate two uses for it, which
// ct-props-correct clause 4 forbids outright — that component is unrepresentable,
// not merely rejected. The intermediate that IS legal is the one whose extension
// step simply does not re-declare the inherited name, and clause 1.5 asks only
// whether SOME two-step derivation is "in principle possible", so that is the
// intermediate to build.
//
// This shape is not exotic: it is what a chain A(@x) ←restriction prohibiting @x←
// R ←extension declaring @x← T folds to. Along the real chain the name is gone by
// the time T declares it, so T holds exactly one use and is perfectly legal
// (attributeusefold.go, clause 3.2.2). Under the re-ordering the prohibition is
// not replayed — the Note re-orders the extension steps, it does not re-order the
// restrictions — so A's use is still there when T's own arrives.
//
// Dropping is NOT the same as accepting. What the §3.4.6.2 Note actually forbids
// is adding something back "in an incompatible way (for example, with a
// conflicting type assignment or value constraint)", and that comparison is made
// where it belongs: T's own use is then measured against the INHERITED one by
// derivation-ok-restriction clause 3 (c-ran's ·subsumption·) and clause 4
// (c-vs-ctd-r's ·validly substitutable·, subject to {extension, list, union}) when
// T is checked against M. An identical re-declaration passes both; a conflicting
// type or value constraint fails one of them, and clause 1.5 charges. Rejecting on
// the collision itself would fail the compatible case, which is a valid schema.
//
// Clause 3.1's own dedup (inheritAttributeUses, #1082) never fires on this path
// and needs no guard here: the drop above is by NAME, so no member of acc that
// survives to be inherited can be identical to a kept own member — identity
// implies the shared name that would have dropped it.
func (s *Schema) collapsedAttributeUses(own, acc []AttributeUse) []AttributeUse {
	kept := make([]AttributeUse, 0, len(own))
	for _, u := range own {
		if hasAttributeUseNamed(acc, u.DeclarationName()) {
			continue
		}
		kept = append(kept, u)
	}
	return s.inheritAttributeUses(kept, acc, DerivationExtension, nil) // §3.4.2.4 clause 3.1
}

// collapsedAttributeWildcard folds one extension step's {attribute wildcard} into
// the collapse through §3.4.2.5 clause 2.2.2 — unionExtensionAttributeWildcard,
// this tree's one encoding of cos-aw-union (§3.10.6.3, attributewildcardfold.go)
// — so M's wildcard is the left fold of that union over A's folded value and each
// extension step's, in chain order. {process contents} comes from the last
// operand folded in, i.e. from T, exactly as it would in the real collapse:
// clause 2.2.2.3 takes it from the ·complete wildcard· side, and T is always the
// topmost extension step.
//
// GAP(xsd): the step's OWN <anyAttribute> is not recoverable, and what is folded
// in is its FOLDED {attribute wildcard} instead — owned by #586. cos-aw-union is
// not invertible and Phase D has overwritten each step's ·complete wildcard· with
// the union, so a step's own declaration is genuinely gone. Since a folded value
// is a cos-ns-subset superset of the own value it absorbed, M's {namespace
// constraint} is a SUPERSET of the true collapsed intermediate's. Reading the
// step's own value off T instead would be the opposite error and a false reject:
// for a chain topped by a restriction, §3.4.2.5 clause 2.1 makes T's folded value
// T's own ·complete wildcard· ALONE, a subset of the true M's.
//
// The direction, over the three readers of M.{attribute wildcard} — all of them
// reached through checkDerivationOKRestriction(t, M):
//
//   - checkAttributeRestrictionWildcard (attributerestriction.go) asks
//     wildcardSubset(T's constraint, M's). A wider M can only turn a rejection
//     into an acceptance: FAIL-OPEN. Past that gate it compares M's {process
//     contents} with T's, and M's is T's own, so that comparison never charges
//     here.
//   - Schema.attributeDefaultBinding, reached from checkRestrictionAttributes,
//     consults M.{attribute uses} FIRST and falls back to the wildcard only for a
//     name no use covers. Where the fallback is reached, more admitted names
//     means ok and no charge: FAIL-OPEN. Its {process contents} half is unaffected in any
//     direction — the value is T's own, the same one the true collapse would
//     carry, not a widened one, so wildcardKeywordBinding sees no substitute.
//   - checkAttributeRestrictionRequired (attributerestriction.go) reads
//     {attribute uses} only: UNAFFECTED.
func collapsedAttributeWildcard(loc xsderr.Loc, c ComplexType, base *Wildcard) (*Wildcard, error) {
	return unionExtensionAttributeWildcard(loc, attributeWildcardProperty(c), base)
}

// collapsedContentType re-applies one extension step's own ·effective content· to
// the collapse so far, per §3.4.2.3.3 clause 4.2 (extensioncontenttype.go).
//
// A step whose own {content type} is SIMPLE is not a clause 4.2 step at all: it
// is a <simpleContent><extension>, mapped by §3.4.2.2's tableau, whose {content
// type} is the step's own simple content outright and inherits no particle. Such
// a step replaces the collapse's content wholesale, which is what the real
// derivation did.
func (s *Schema) collapsedContentType(loc xsderr.Loc, c, b ComplexType, base ContentType) (ContentType, bool, error) {
	if sc, ok := c.ContentType().(SimpleContent); ok {
		return sc, true, nil
	}
	rec, ok, err := s.recoverExtensionStepContent(loc, c.ContentType(), b.ContentType())
	if err != nil || !ok {
		return nil, false, err
	}
	content, err := extensionContentTypeOver(loc, base, rec.effective, rec.explicitEmpty, rec.effectiveMixed, s.modelGroupNamed)
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
}
