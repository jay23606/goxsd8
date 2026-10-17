package xsd

import "github.com/kud360/goxsd8/xsderr"

// ruleCosAllLimited is All Group Limited (Structures §3.8.6.2,
// id="cos-all-limited"): a model group whose {compositor} is all appears only as
// a model group definition's {model group} (clause 1.1), as the {term} of a
// Particle with {max occurs} = 1 which is the {particle} of a complex type's
// {content type} (clause 1.2), or as the {term} of a {min occurs} = {max occurs}
// = 1 Particle among another all group's {particles} (clause 1.3); and every
// particle of an all group whose {term} is a model group has an all group there
// (clause 2).
const ruleCosAllLimited xsderr.Rule = "cos-all-limited"

// This file decides cos-all-limited, both clauses, over the resolved component
// graph. It is a Schema Component Constraint (§2.3, §2.4) stated entirely in
// terms of {term}/{particles}/{compositor}, never over the <all>/<sequence>/
// <group ref> XML spelling — and §3.7.2 (xr.mgd3) makes the {term} of a <group
// ref> particle BE the referenced definition's {model group}, so the compositor
// a usage site imposes is invisible until references resolve. That is why the
// charge lives here and not in the producer (#469).
//
// SCOPE is every {term} slot in the assembled set, reached from three roots —
// the complex type definitions, the top-level element declarations (whose inline
// anonymous complex types no index holds), and the model group definitions. A
// <group ref> is a LEAF: its placement is decided on the referenced definition's
// {model group}, but that group's own {particles} are not descended here,
// because checkAllGroupsLimited walks every definition in its own right. Nothing
// else would be reached by descending, and descending would re-charge one
// component once per reference to it.
//
// Clause 1.1 is why a definition body is never charged: an all-bodied <group
// name="..."> is legal in itself, and the violation is always at a usage site.
//
// PROCESSOR-SYNTHESIZED CONTENT IS IN SCOPE, deliberately. §3.4.2.3.3 clause
// 4.2.3.3 wraps (·base particle·, ·effective content·) in a synthesized sequence
// (extensioncontenttype.go), so extending an all-content base with non-all
// content yields an all group inside a sequence that no author wrote. Neither
// §3.4.2.3.3, nor §3.8.6.2 itself, nor cos-ct-extends (§3.4.6.2, whose clause
// 1.4.3.2.2.2 positively PERMITS the shape through cos-particle-extend clause 2)
// carves out an exemption, and §3.8.6.2 is worded over the resulting component
// with no qualification by origin. So the shape is charged, which is what makes
// "extend a type whose content model is an <all>" an error rather than a
// silently accepted content model no instance can satisfy as the author meant.
//
// NOT A componentWalk PHASE, deliberately (#841). componentWalk
// (componentwalk.go) charges a by-name arm and never follows it; this rule
// needs a third treatment of a <group ref>: read the referenced definition's
// {compositor} through it and stop there (resolveTermGroup). §3.7.2 (xr.mgd3)
// makes that {model group} the reference's {term}, and componentWalk holds no
// schema to read it with. The rule is also stated over a particle's POSITION
// — its {min occurs}/{max occurs} (clauses 1.2 and 1.3) and its container's
// {compositor} (clause 1 against clause 2), with no container at all for a
// {content type}'s particle — and componentWalk's callbacks receive neither.
// A further Phase C/D constraint joins componentWalk when it enumerates what
// componentWalk enumerates and charges per component; one that reads through
// a by-name arm, or decides on a particle's position, takes a descent of its
// own with its reason at its head, as this file and complextypewalk.go do.

// checkAllGroupsLimited is the cos-all-limited step of Phase C. It runs FIRST in
// that phase: the shape clause 2 forbids — a sequence or choice group among an
// all group's {particles} — is one the UPA construction's all-group exit state
// over-approximates on (particleattribution.go), so charging the right rule
// before cos-nonambig sees it keeps that approximation unreachable (#468).
//
// It walks the compiled set in document order (STYLE D2) and returns the first
// violation. Following <group ref> edges needs no cycle guard: Phase B's
// checkModelGroupsAcyclic has already rejected a circular reference graph
// (PRINCIPLES 9), and the descent below follows only OWNED, by-value nesting.
func (s *Schema) checkAllGroupsLimited() error {
	for _, t := range s.types {
		c, ok := t.(ComplexType)
		if !ok {
			continue // a *SimpleType has no content model
		}
		if err := s.checkComplexTypeAllLimited(c); err != nil {
			return err
		}
	}
	for _, e := range s.elements {
		if err := s.checkTypeDefinitionAllLimited(e.TypeDefinition()); err != nil {
			return err
		}
	}
	for _, mgd := range s.modelGroups {
		// The definition's own {model group} is clause 1.1's legal home whatever
		// its {compositor}, so only the particles BELOW it are decided. It is
		// walked whether or not any <group ref> points at it: §3.8.6 binds every
		// Model Group component.
		if err := s.checkGroupAllLimited(mgd.ModelGroup(), mgd.Loc(), "model group definition "+mgd.Name().String()); err != nil {
			return err
		}
	}
	return nil
}

// checkComplexTypeAllLimited decides one complex type's content model, plus the
// OWNED arm of its {base type definition} — an anonymous inline base is in no
// index, so dropping that hop would leave its content model unchecked (the same
// inventory argument checkSimpleTypeDerivations records). A by-name base is not
// followed: it is a top-level type this walk reaches in its own right.
//
// Rejections are charged to the complex type's own Loc, per resolveReferences'
// referrer-Loc convention: a Particle and a Model Group retain no position, so
// the position of the nearest enclosing component that does names the
// declaration a reader must open.
func (s *Schema) checkComplexTypeAllLimited(c ComplexType) error {
	if err := s.checkTypeDefinitionAllLimited(c.Base()); err != nil {
		return err
	}
	ec, ok := c.ContentType().(ElementContent)
	if !ok {
		return nil // empty and simple {content type}s hold no particle
	}
	// typeDefinitionLabel renders an anonymous inline type as a phrase, so this
	// reads for both "the content model of {urn:x}CT" and "the content model of
	// an anonymous type definition".
	ctx := "the content model of " + typeDefinitionLabel(c)
	if err := s.checkContentParticlePlacement(ec.Particle, c.Loc(), ctx); err != nil {
		return err
	}
	return s.descendParticleAllLimited(ec.Particle, c.Loc(), ctx)
}

// checkTypeDefinitionAllLimited descends the InlineTypeDefinition arm of a
// {type definition}/{base type definition} slot, where an anonymous complex type
// lives. A TypeDefinitionRef names a top-level type this walk reaches through
// s.types, and a SubstitutionGroupHeadTypeRef names the head declaration that
// owns the anonymous type, itself an entry of s.elements.
func (s *Schema) checkTypeDefinitionAllLimited(ref TypeDefinitionOrRef) error {
	inline, ok := ref.(InlineTypeDefinition)
	if !ok {
		return nil
	}
	c, ok := inline.Definition.(ComplexType)
	if !ok {
		return nil // a *SimpleType has no content model
	}
	return s.checkComplexTypeAllLimited(c)
}

// checkContentParticlePlacement decides clause 1.2 for the {particle} of a
// {content type}: an all group is admitted there only when the particle's
// {max occurs} is 1, so an unbounded or larger maximum — reachable through a
// <group ref maxOccurs="2"> to an all-bodied definition, which the schema for
// schema documents does not restrict the way it restricts <all>'s own maxOccurs
// — is a violation.
func (s *Schema) checkContentParticlePlacement(p Particle, loc xsderr.Loc, ctx string) error {
	g, ok := s.resolveTermGroup(p.Term())
	if !ok || g.Compositor() != CompositorAll {
		return nil
	}
	if max, bounded := p.Occurs().Max(); bounded && max == 1 {
		return nil
	}
	return xsderr.New(ruleCosAllLimited, loc,
		"%s has an all model group as the {term} of its content particle, whose occurrence range is %s, but cos-all-limited clause 1.2 admits an all group there only with {max occurs} = 1", ctx, p.Occurs())
}

// checkGroupAllLimited decides every particle among one model group's
// {particles}, in document order (STYLE D2), and descends into each.
func (s *Schema) checkGroupAllLimited(g ModelGroup, loc xsderr.Loc, ctx string) error {
	for _, p := range g.Particles() {
		if err := s.checkMemberPlacement(p, g.Compositor(), loc, ctx); err != nil {
			return err
		}
		if err := s.descendParticleAllLimited(p, loc, ctx); err != nil {
			return err
		}
	}
	return nil
}

// checkMemberPlacement decides the three verdicts available at a particle among
// a model group's {particles}, container being that group's {compositor}:
//
//   - container is sequence or choice, and the {term} is an all group: no clause
//     1 sub-clause names this slot, so it is a clause 1 violation however the
//     particle occurs.
//   - container is all, and the {term} is a sequence or choice group: clause 2.
//   - container is all, and the {term} is an all group: clause 1.3, which admits
//     it only with {min occurs} = {max occurs} = 1.
//
// A {term} that is an element declaration or a wildcard is not a model group, so
// neither clause has anything to say about it.
func (s *Schema) checkMemberPlacement(p Particle, container Compositor, loc xsderr.Loc, ctx string) error {
	g, ok := s.resolveTermGroup(p.Term())
	if !ok {
		return nil
	}
	if container != CompositorAll {
		if g.Compositor() != CompositorAll {
			return nil
		}
		return xsderr.New(ruleCosAllLimited, loc,
			"%s nests an all model group inside a %s model group, but cos-all-limited clause 1 admits an all group only as a model group definition's {model group} (1.1), as the {term} of a complex type content particle with {max occurs} = 1 (1.2), or as the {term} of a {min occurs} = {max occurs} = 1 particle of another all group (1.3)", ctx, container)
	}
	if g.Compositor() != CompositorAll {
		return xsderr.New(ruleCosAllLimited, loc,
			"%s puts a %s model group among the {particles} of an all model group, but cos-all-limited clause 2 requires every such member {term} to have {compositor} all", ctx, g.Compositor())
	}
	max, bounded := p.Occurs().Max()
	if p.Occurs().Min() == 1 && bounded && max == 1 {
		return nil
	}
	return xsderr.New(ruleCosAllLimited, loc,
		"%s nests an all model group inside another all group through a particle whose occurrence range is %s, but cos-all-limited clause 1.3 admits it only with {min occurs} = {max occurs} = 1", ctx, p.Occurs())
}

// descendParticleAllLimited walks below one particle: an inline model group's
// own {particles}, and the inline anonymous type of an inline element
// declaration, whose content model is a component in its own right that no index
// reaches. An <element ref> and a <group ref> are leaves naming components this
// walk reaches from its own roots, so following either would re-charge one
// component once per reference and, for <element ref>, could revisit its
// declaration forever.
func (s *Schema) descendParticleAllLimited(p Particle, loc xsderr.Loc, ctx string) error {
	t, ok := p.Term().(ResolvedTerm)
	if !ok {
		return nil
	}
	switch inner := t.Term.(type) {
	case ElementDeclaration:
		return s.checkTypeDefinitionAllLimited(inner.TypeDefinition())
	case ModelGroup:
		return s.checkGroupAllLimited(inner, loc, ctx)
	case Wildcard:
		return nil // a wildcard is not a model group and holds no particles
	default:
		panic("xsd: descendParticleAllLimited: non-exhaustive Term switch")
	}
}
