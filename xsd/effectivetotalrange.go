package xsd

// This file decides the two Effective Total Range Schema Component Constraints —
// Effective Total Range (all and sequence) (Structures §3.8.6.5, cos-seq-range)
// and Effective Total Range (choice) (§3.8.6.6, cos-choice-range) — together with
// the ·emptiable· definition that reads them, Particle Emptiable (§3.9.6.3,
// cos-group-emptiable).
//
// None of the three is a rejection point of its own: an effective total range is
// a DERIVED pair and ·emptiable· is a predicate over it. Both are consumed by
// derivation-ok-restriction clauses 2.2.2.2 and 2.3.2.2 (complexderivation.go),
// which charge the failure with the rule the spec names there.
//
// NOT THE SAME NOTION as particleattribution.go's `emptiable`, and the two must
// never be collapsed into one helper: that one is "can this automaton fragment
// accept the empty sequence", built for Unique Particle Attribution, and it and
// this one genuinely DISAGREE on an empty <choice>. particleattribution.go's
// addChoice reports an empty <choice> as NOT emptiable ("accepts nothing at
// all"), while cos-choice-range's "(or 0 if there are no {particles})" makes its
// effective-total-range minimum 0 and hence the particle ·emptiable· under
// cos-group-emptiable clause 2. Both readings are right for their own rule.
//
// The walk below follows ModelGroupRef edges into their definitions with NO
// cycle guard. That is licensed, not overlooked: it runs only in Phase D
// (resolve.go), or through the exported ParticleEmptiable on a finalized
// Schema, so strictly after Phase B's checkModelGroupsAcyclic has rejected a
// circular <group ref> graph (PRINCIPLES 9).

// maxFiniteRange caps the saturating arithmetic below. An effective total range
// is a product of a particle's {min occurs}/{max occurs} with a sum over its
// group's members, so nesting a handful of maxOccurs="100000" groups overflows
// int. Saturating is sound because NO consumer of an effective total range
// distinguishes one large finite value from another: ParticleEmptiable asks only
// whether the minimum is 0, and the maximum rule asks only whether a value is
// unbounded or non-zero. Wrapping, by contrast, could turn a huge positive
// minimum into 0 and make a non-emptiable particle look ·emptiable· — the one
// failure mode that matters, since it would ACCEPT a restriction the spec
// rejects. Capping has precedent in this package (maxMandatoryCopies /
// maxOptionalCopies, particleattribution.go).
const maxFiniteRange = 1 << 30

// effectiveRange is the ·effective total range· of a particle: the (minimum,
// maximum) pair cos-seq-range (§3.8.6.5) and cos-choice-range (§3.8.6.6) define.
//
// It is deliberately NOT xsd.Occurs. Occurs is a PARTICLE'S {min occurs}/{max
// occurs} property pair (§3.9.1) whose constructors charge p-props-correct; an
// effective total range is a derived quantity that is no particle's property, so
// reusing Occurs would make a computed range assignable to a real {min
// occurs}/{max occurs} slot and force every step of the recursion below through
// an error-returning constructor for an arithmetically impossible failure. The
// unbounded fact keeps ONE encoding across both types, though: max carries
// occurs.go's unboundedMax sentinel (STYLE D3).
//
// There is no validating constructor because there is nothing to validate:
// min <= max is a theorem of the arithmetic (sums, minima and maxima of
// non-negative bounds all preserve it), not an invariant a caller could break.
type effectiveRange struct {
	min int
	max int
}

// isEmptiable reports whether the range's minimum is 0 — the "minimum part of
// the effective total range ... is 0" test of cos-group-emptiable clause 2.
func (r effectiveRange) isEmptiable() bool {
	return r.min == 0
}

// isUnbounded reports whether the maximum part is ***unbounded***.
func (r effectiveRange) isUnbounded() bool {
	return r.max == unboundedMax
}

// combineRange is the SINGLE combinator both §3.8.6.5 and §3.8.6.6 funnel
// through: given the enclosing particle's {min occurs}/{max occurs}, the group's
// {compositor}, and the effective total range each member particle contributes,
// it returns the group particle's effective total range.
//
// minimum: the product of P.{min occurs} and — over the members — the SUM for
// all/sequence (cos-seq-range) or the MINIMUM for choice (cos-choice-range), or
// 0 when the group has no {particles}. Transposing sum and minimum-of is the one
// mistake these two near-identical constraints invite, so the aggregation lives
// in exactly one place (aggregateRanges) keyed on the compositor.
//
// maximum: ***unbounded*** if any member's maximum is unbounded, OR if some
// member's maximum is NON-ZERO and P.{max occurs} is unbounded; otherwise the
// product of P.{max occurs} and — over the members — the SUM for all/sequence or
// the MAXIMUM for choice, or 0 when the group has no {particles}. The
// non-zero qualifier is why the unbounded case is decided HERE and cannot be got
// backwards by a caller: an unbounded P over a body whose every member has {max
// occurs} = 0 yields 0, not unbounded.
func combineRange(parent Occurs, compositor Compositor, members []effectiveRange) effectiveRange {
	memberMin, memberMax, anyUnbounded := aggregateRanges(compositor, members)
	r := effectiveRange{min: mulSat(parent.Min(), memberMin)}
	if anyUnbounded {
		r.max = unboundedMax
		return r
	}
	parentMax, bounded := parent.Max()
	if bounded {
		r.max = mulSat(parentMax, memberMax)
		return r
	}
	// P.{max occurs} = unbounded. Every member maximum is finite here, so the
	// aggregate is non-zero exactly when some member maximum is non-zero (a sum of
	// non-negative bounds, or a maximum over them, is 0 iff all of them are).
	if memberMax != 0 {
		r.max = unboundedMax
		return r
	}
	r.max = 0
	return r
}

// aggregateRanges reduces the member ranges to the (minimum, maximum) pair the
// enclosing particle's occurrence bounds multiply, plus whether any member's
// maximum is unbounded. A group with no {particles} aggregates to (0, 0) — the
// spec's parenthesised "(or 0 if there are no {particles})", which is what makes
// an empty <choice> or <sequence> ·emptiable·.
func aggregateRanges(compositor Compositor, members []effectiveRange) (int, int, bool) {
	if len(members) == 0 {
		return 0, 0, false
	}
	anyUnbounded := false
	for _, m := range members {
		if m.isUnbounded() {
			anyUnbounded = true
		}
	}
	switch compositor {
	case CompositorChoice:
		// cos-choice-range: minimum-of the minima, maximum-of the maxima.
		minPart, maxPart := members[0].min, 0
		for _, m := range members {
			if m.min < minPart {
				minPart = m.min
			}
			if !m.isUnbounded() && m.max > maxPart {
				maxPart = m.max
			}
		}
		return minPart, maxPart, anyUnbounded
	case CompositorAll, CompositorSequence:
		// cos-seq-range: sum of the minima, sum of the maxima.
		minPart, maxPart := 0, 0
		for _, m := range members {
			minPart = addSat(minPart, m.min)
			if !m.isUnbounded() {
				maxPart = addSat(maxPart, m.max)
			}
		}
		return minPart, maxPart, anyUnbounded
	default:
		panic("xsd: aggregateRanges: non-exhaustive Compositor switch")
	}
}

// addSat adds two non-negative bounds, saturating at maxFiniteRange rather than
// wrapping; see maxFiniteRange for why saturation is sound here.
func addSat(a, b int) int {
	if a > maxFiniteRange-b {
		return maxFiniteRange
	}
	return a + b
}

// mulSat multiplies two non-negative bounds, saturating at maxFiniteRange rather
// than wrapping; see maxFiniteRange for why saturation is sound here. A zero
// factor still yields 0, which is what keeps a maxOccurs="0" particle vacuous.
func mulSat(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	if a > maxFiniteRange/b {
		return maxFiniteRange
	}
	return a * b
}

// effectiveTotalRange is the ·effective total range· of particle p.
//
// When p.{term} resolves to a Model Group the pair is cos-seq-range (§3.8.6.5)
// or cos-choice-range (§3.8.6.6), per the group's {compositor}. When it is an
// element declaration or a wildcard the spec defines no effective total range at
// all — both constraints read such a particle's OWN {min occurs}/{max occurs}
// when aggregating — so that own pair is returned, which is exactly the value an
// enclosing group's computation needs.
//
// A <group ref> is followed into its definition's {model group} (§3.7.2) with no
// cycle guard; see this file's header for why that is licensed.
func (s *Schema) effectiveTotalRange(p Particle) effectiveRange {
	g, ok := s.resolveTermGroup(p.Term())
	if !ok {
		return particleOwnRange(p)
	}
	members := make([]effectiveRange, 0, len(g.particles))
	for _, member := range g.particles {
		members = append(members, s.effectiveTotalRange(member))
	}
	return combineRange(p.Occurs(), g.Compositor(), members)
}

// particleOwnRange renders a particle's own {min occurs}/{max occurs} as the
// range it contributes when its {term} is not a group.
func particleOwnRange(p Particle) effectiveRange {
	o := p.Occurs()
	if o.IsUnbounded() {
		return effectiveRange{min: o.Min(), max: unboundedMax}
	}
	max, _ := o.Max()
	return effectiveRange{min: o.Min(), max: max}
}

// resolveTermGroup returns the Model Group a particle's {term} denotes — written
// inline (ResolvedTerm) or reached through a <group ref> — and false when the
// {term} is an element declaration or a wildcard. A <group ref> that resolves to
// nothing yields false: Phase A already rejected a dangling one (src-resolve
// clause 1.5), so this is unreachable on a *Schema that exists, not a silent
// skip.
func (s *Schema) resolveTermGroup(t TermOrRef) (ModelGroup, bool) {
	switch t := t.(type) {
	case ResolvedTerm:
		g, ok := t.Term.(ModelGroup)
		return g, ok
	case ModelGroupRef:
		mgd, ok := s.modelGroupIndex[t.Name]
		if !ok {
			return ModelGroup{}, false
		}
		return mgd.ModelGroup(), true
	case ElementDeclarationRef:
		return ModelGroup{}, false
	default:
		panic("xsd: resolveTermGroup: non-exhaustive TermOrRef switch")
	}
}

// ParticleEmptiable reports whether p can match an empty sequence of elements:
// whether p is ·emptiable· (Particle Emptiable, §3.9.6.3 cos-group-emptiable).
// Clause 1: p's {min occurs} is 0. Clause 2: p's {term} is a model group,
// inline or the {model group} of the definition a <group ref> names on s
// (§3.7.2), and the minimum of p's effective total range (§3.8.6.5
// cos-seq-range for a sequence or all, §3.8.6.6 cos-choice-range for a
// choice, 0 for a group with no {particles}) is 0. s is needed because a
// <group ref> term resolves on s; for one that does not resolve on s, clause
// 2's antecedent fails, so the answer is whether p's {min occurs} is 0. It is
// exported for package codegen, which reads it to decide whether a model
// group term can match nothing and whether a sequence's other particles are
// emptiable.
func (s *Schema) ParticleEmptiable(p Particle) bool {
	if p.Occurs().Min() == 0 {
		return true // clause 1
	}
	if _, ok := s.resolveTermGroup(p.Term()); !ok {
		return false // clause 2's antecedent fails: the {term} is not a group
	}
	// Clause 2's "the effective total range of that group" is the range of the
	// PARTICLE whose {term} is that group — §3.8.6.5/6 define the pair for a
	// particle P, folding P.{min occurs} in as a factor — so clause 2 is read off
	// effectiveTotalRange(p) directly. A zero p.{min occurs} would already have
	// satisfied clause 1, so the two readings never disagree.
	return s.effectiveTotalRange(p).isEmptiable() // clause 2
}
