package xsd

import "github.com/kud360/goxsd8/xsderr"

// This file renders two §3.4.6.4 definitions statically over the component
// model: ·default binding· (key-dft-binding) and ·subsumes· (loc-testSubP).
//
// Both are worded over INFORMATION ITEMS at assessment time. Their ATTRIBUTE
// half is nonetheless decidable per (Complex Type Definition, expanded name)
// without any instance, and that static rendering is the whole of what
// derivation-ok-restriction clause 3 (c-ran) needs: the clause quantifies over
// every element information item whose attributes are valid against T, which for
// attributes reduces to "for each expanded name T admits, compare the bindings".
//
// The ELEMENT half (key-dft-binding case 1, loc-testSubP clause 4) is
// cos-content-act-restrict's business (derivation-ok-restriction clause 2.4.2)
// and lives here too, but is reached from contentrestricts.go rather than from
// this file: that constraint walks a content model to learn WHICH declaration or
// wildcard an item binds to, while the two halves share the ·subsumes· relation
// below. The two entry points differ only in what they return — the attribute
// half charges an error per sub-clause, the element half answers a bool, because
// it is one conjunct of derivation-ok-restriction clause 2's disjunction and so
// is not yet a verdict where it is evaluated. The wildcard-bucket lattice
// (clauses 1-3) has ONE encoding, keywordSubsumes, which both reach (STYLE T4).

// defaultBinding is the ·default binding· a Content Type or Complex Type
// Definition gives an information item (Structures §3.4.6.4, key-dft-binding):
// "an Element Declaration, an Attribute Use, or one of the keywords strict, lax,
// or skip". The spec's six cases land in exactly those three shapes, so the set
// is closed and this is a sealed sum (STYLE T2) mirroring ContentType and
// Term — never a kind tag beside optional payloads, which would make "an
// Element Declaration that is also skip" representable.
type defaultBinding interface{ defaultBinding() }

// elementDeclarationBinding is key-dft-binding case 1: the item has a ·governing
// element declaration·, and the binding is that Element Declaration. It is
// constructed by contentrestricts.go's elementPositionBinding, for an item
// ·attributed· to an ·element particle· of a content model, and read by
// elementDeclarationSubsumes (loc-testSubP clause 4). attributeDefaultBinding
// never produces one: cases 2 and 3 are the attribute half.
type elementDeclarationBinding struct{ decl ElementDeclaration }

// attributeUseBinding is key-dft-binding case 2 (the item has a ·governing
// attribute declaration· and is ·attributed· to an Attribute Use, so the binding
// is that use) and case 3 (it is ·attributed· to an attribute wildcard, so the
// binding is a SYNTHESIZED Attribute Use whose {attribute declaration} is the
// ·governing attribute declaration·, whose {value constraint} is ·absent·, and
// whose {inheritable} is that declaration's).
type attributeUseBinding struct{ use AttributeUse }

// wildcardKeywordBinding is key-dft-binding cases 4, 5 and 6: the item is
// ·attributed· to a strict or lax wildcard with NO ·governing· declaration
// (cases 4 and 5), or to a skip wildcard (case 6, which needs no such
// qualifier), so the binding is the keyword itself. The keyword is the already
// typed ProcessContents closed set (closedsets.go), never a string, and it is
// the whole of what keywordSubsumes reads.
type wildcardKeywordBinding struct{ keyword ProcessContents }

func (elementDeclarationBinding) defaultBinding() {}
func (attributeUseBinding) defaultBinding()       {}
func (wildcardKeywordBinding) defaultBinding()    {}

// attributeDefaultBinding computes side's ·default binding· (key-dft-binding)
// for an attribute of expanded name n. side is one half of a c-ran clause 3
// comparison (attributerestriction.go) — the {attribute uses} and {attribute
// wildcard} of a Complex Type Definition, or of an Attribute Group Definition
// src-redefine clause 7.2.2 instructs be read as one:
//
//   - case 2: the member of side's {attribute uses} whose {attribute declaration}
//     carries n. For a complex type the property is the MATERIALISED one
//     (§3.4.2.4 clause 3, attributeusefold.go), so an INHERITED use counts
//     without any walk here. cvc-complex-type clause 2.1 makes such a use the
//     ·context-determined declaration·, so it wins over any wildcard — including
//     a wildcard on the same side, since case 2 is tested before cases 4/5/6. The
//     set is exact for both derivation methods: clause 3.2.2's prohibited names
//     are applied at the fold too, so a name a restriction prohibits is absent
//     here rather than reported as the ancestor's use.
//   - cases 4/5/6: otherwise, if side's {attribute wildcard} admits n, the
//     wildcard's {process contents} keyword. This one is read off that side
//     ALONE, and that is exact for both derivation methods: the property is the
//     MATERIALISED one (§3.4.2.5 clause 2, attributewildcardfold.go), so a
//     restriction carries its own ·complete wildcard· (clause 2.1) and an
//     extension carries the cos-aw-union of its own with its base's (clause 2.2)
//     without any walk here.
//
// ok is false when side admits no attribute of that name at all — no member of
// {attribute uses} and no admitting wildcard — in which case there is no binding
// to compare and the CALLER charges the failure. Case 1 (·governing element
// declaration·) is the element half and is not reachable here.
//
// GAP(xsd): case 3 — the item has a ·governing attribute declaration· and is
// ·attributed· to an attribute wildcard, so the binding is a SYNTHESIZED
// Attribute Use over that declaration — is not rendered, and the wildcard branch
// falls through to the keyword instead. Whether an attribute HAS a ·governing
// attribute declaration· is an assessment-episode fact (key-governing-ad clause
// 3 resolves it by expanded name at ·assessment· time, against the schema of a
// real episode rather than this document graph — cvc-resolve-instance,
// §3.17.6.3 — and clause 1 lets the processor stipulate one outright); a static
// schema check cannot fix it, and guessing "a top-level declaration of that name
// exists, so case 3 applies" makes an unrelated global declaration silently
// constrain a restriction's local attribute type. RULED permanent by #267 (STYLE
// P3b): no schema shape can conclude that case 3 APPLIES, and the real case-3
// binding is the instance validator's to render, with the assessed item and its
// ·resolution· in hand (validate/doc.go's M5 contract).
//
// Falling through to the keyword is FAIL-OPEN against both readers of this
// function's result, both of them in checkAttributeRestriction
// (attributerestriction.go). checkBindingSubsumes charges c-ran clause 3 only
// where the base binding does NOT ·subsume· the restriction's, and a keyword G
// as keywordSubsumes below renders it accepts every specific binding a case-3
// Attribute Use G would accept and more — every pairing keywordSubsumes refuses
// has a keyword S, which an Attribute Use G refuses too, through
// checkBindingSubsumes' catch-all — so the substitution can only turn a charge
// into an acceptance, never a false reject. The other reader, that caller's
// ok=false charge, is unaffected in either direction: name admission alone
// decides it, before any binding is built.
//
// The gap is NARROWER than the whole wildcard branch: it is every attribute
// wildcard whose {process contents} is strict or lax AND whose {namespace
// constraint}.{disallowed names} does NOT contain the keyword defined. Two
// wildcard shapes fall statically outside case 3, each by its own argument, and
// for them the keyword returned below is the EXACT key-dft-binding rather than
// an approximation of it — modulo clause 1's stipulation, which is an external
// input to an assessment episode that this schema-authoring-time constraint
// sets aside wherever it reads a binding.
//
//   - {process contents} skip. key-governing-ad clause 3 resolves by expanded
//     name only "provided the attribute is not ·skipped·", and key-skipped
//     (§3.10.4.1) makes an item ·attributed· to a skip wildcard ·skipped·.
//     Clause 2 is unavailable to any wildcard-attributed item at all — that
//     section's closing Note gives such an item no ·context-determined
//     declaration·, and states outright that a skip {process contents} leaves
//     it no ·governing· declaration. validate/cvcid.go's skippedAttribute is
//     this repo's one encoding of that reading, and the place to read it from.
//     key-dft-binding's own drafting agrees: cases 4 and 5 are each
//     qualified "and it does not have a ·governing ... declaration·" and case 6
//     is not, the exclusion being already guaranteed there.
//   - {disallowed names} containing defined (notQName="##defined", §3.10.2).
//     cvc-wildcard (§3.10.4.1) clause 2.2 makes "the expanded name does not
//     ·resolve· to an attribute declaration" a PRECONDITION of valid
//     attribution to that wildcard, so an item ·attributed· to it can have no
//     key-governing-ad clause-3 declaration either. AllowsAttributeWildcardName
//     (wildcardadmit.go) is what makes this exact rather than merely probable:
//     it decides clause 2.2 itself, so for a ##defined wildcard and a name the
//     schema declares at top level the return below is unreachable.
//
// Every subset returns the same wildcardKeywordBinding, so nothing here
// branches on which one applies (STYLE D3); what differs is only whether that
// value is exact or fail-open.
func (s *Schema) attributeDefaultBinding(side attributeRestrictionSide, n QName) (defaultBinding, bool) {
	if u, ok := findAttributeUse(side.uses, n); ok {
		return attributeUseBinding{use: u}, true // case 2
	}
	if !side.hasWildcard || !s.AllowsAttributeWildcardName(side.wildcard, n) {
		return nil, false
	}
	return wildcardKeywordBinding{keyword: side.wildcard.ProcessContents()}, true // cases 4/5/6
}

// ResolvedAttributeDeclaration resolves the Attribute Declaration behind an
// attribute use for both variants of the AttributeDeclarationOrRef sum: the
// sibling declaration a [LocalAttributeDeclaration] owns by value, or the
// top-level declaration an [AttributeDeclarationRef] names.
//
// u must be a use of s, because the Ref variant carries only the declaration's
// expanded name and is resolved by NAME through [Schema.Attribute] on s, not
// through a pointer to the declaration u was built against. A use of s never
// dangles — Phase A rejected a dangling Ref (src-resolve clause 1.2) — so ok is
// always true for one. A Ref from another *Schema either dangles, when s
// declares no attribute of that name, and reports ok false; or silently
// resolves to s's own declaration of that name rather than the one u was built
// against. The Local variant resolves the same from any receiver.
//
// It is exported for the instance validator, which needs the declaration behind
// a use it matched an attribute information item to — its {type definition} for
// cvc-attribute (§3.2.4.1), its {value constraint} for cvc-au (§3.5.4) (#714).
// A consumer wanting only the use's expanded name reads
// AttributeUse.DeclarationName instead, which needs no resolution and so cannot
// fail.
func (s *Schema) ResolvedAttributeDeclaration(u AttributeUse) (AttributeDeclaration, bool) {
	switch d := u.attributeDeclaration.(type) {
	case LocalAttributeDeclaration:
		return d.Declaration, true
	case AttributeDeclarationRef:
		return s.Attribute(d.Name)
	default:
		panic("xsd: ResolvedAttributeDeclaration: non-exhaustive AttributeDeclarationOrRef switch")
	}
}

// ownedAttributeDeclaration is ResolvedAttributeDeclaration narrowed to the
// declaration a use OWNS: ok is true only for the Local variant, whose sibling
// declaration belongs to no §3.17.1 symbol table and so has no other site that
// could charge it. The Ref variant names a GLOBAL declaration the schema's
// {attribute declarations} holds and charges in its own right, so it reports
// false rather than that declaration — a use is not its owner.
//
// It is a sibling of ResolvedAttributeDeclaration rather than a type assertion at the
// call site so that the switch over the sealed AttributeDeclarationOrRef sum is
// written once per concern and a new variant is a compile-or-panic here, not a
// silently wrong answer there (STYLE T4).
func (s *Schema) ownedAttributeDeclaration(u AttributeUse) (AttributeDeclaration, bool) {
	switch d := u.attributeDeclaration.(type) {
	case LocalAttributeDeclaration:
		return d.Declaration, true
	case AttributeDeclarationRef:
		return AttributeDeclaration{}, false
	default:
		panic("xsd: ownedAttributeDeclaration: non-exhaustive AttributeDeclarationOrRef switch")
	}
}

// EffectiveValueConstraint is the ·effective value constraint· of an attribute
// use (Structures §3.5.4, key-evc): U.{value constraint} if present, otherwise
// U.{attribute declaration}.{value constraint} if present, otherwise ·absent·.
//
// It is on *Schema rather than on AttributeUse because the Ref variant's
// declaration is reachable only through the schema's {attribute declarations}; a
// method on the use would need a schema back-pointer, or would silently answer for
// the Local variant alone. The receiver matches its sibling
// [Schema.ResolvedAttributeDeclaration], and so does the precondition: u must be
// a use of s, on the terms that method states.
//
// Two callers read it, and BOTH read the term by that name: finalize's
// checkAttributeValueConstraintSubsumes, where loc-testSubP (§3.4.6.4) clause 5.2
// invokes it, and the instance validator's cvc-complex-type (§3.4.4.2) clause 4,
// which validates the {lexical form} of a ·defaulted attribute·'s effective value
// constraint — a use qualifying as one by the ·defaulted attribute· definition's
// own clause 3, "U's ·effective value constraint· is not ·absent·" (#766).
//
// It is NOT what cvc-au (§3.5.4) or cvc-attribute (§3.2.4.1) clause 4 read. cvc-au
// tests U.{value constraint} and clause 4 tests D.{value constraint}; routing
// either through here would apply the declaration's fixed value where the use
// carries none, and charge a violation the spec does not.
func (s *Schema) EffectiveValueConstraint(u AttributeUse) (ValueConstraint, bool) {
	if vc, ok := u.ValueConstraint(); ok {
		return vc, true
	}
	d, ok := s.ResolvedAttributeDeclaration(u)
	if !ok {
		return ValueConstraint{}, false
	}
	return d.ValueConstraint()
}

// ResolvedInheritable is an attribute use's {inheritable} (Structures §3.5.1,
// au-inheritable). The use's own inheritable attribute wins when present. When
// it is absent the value is false for the Local variant,
// [LocalAttributeDeclaration] (§3.2.2.2 dcl.att.local), and the resolved
// declaration's {inheritable} for the Ref variant, [AttributeDeclarationRef]
// (§3.2.2.3 ref.att.local), which is why it is on *Schema: the same reason
// [Schema.EffectiveValueConstraint] is. u must be a use of s, on the terms
// [Schema.ResolvedAttributeDeclaration] states; a Ref that does not resolve
// through s answers false.
//
// Its readers are loc-testSubP (§3.4.6.4) clause 5.3 (checkAttributeUseSubsumes),
// §3.5.1 property identity (attributeUsesIdentical, complexextension.go), and the
// instance validator's key-p-inherited (§3.3.5.6) clause 3.1 (#1682).
func (s *Schema) ResolvedInheritable(u AttributeUse) bool {
	switch u.inheritable {
	case inheritTrue:
		return true
	case inheritFalse:
		return false
	case inheritAbsent:
		d, ok := s.ResolvedAttributeDeclaration(u)
		return ok && d.Inheritable()
	default:
		panic("xsd: ResolvedInheritable: non-exhaustive inheritableSpec switch")
	}
}

// checkBindingSubsumes charges c-ran clause 3 when general does not ·subsume·
// specific (Structures §3.4.6.4, loc-testSubP). general is the BASE side's
// binding (the spec's G) and specific the restriction's (S); n names the
// attribute both are for, and r carries the rule, position and two side labels
// the message is built from (attributerestriction.go).
//
//   - clause 1: G is skip — subsumes anything.
//   - clause 2: G is lax and S is not skip.
//   - clause 3: both are strict.
//   - clause 4: both are Element Declarations — unreachable from an attribute
//     binding, and decided by bindingSubsumes on the element side.
//   - clause 5: both are Attribute Uses (checkAttributeUseSubsumes).
//
// Anything else fails to subsume: G strict against an Attribute Use S, for
// instance, is a real violation — under B the attribute would have to ·resolve·
// to a declaration and be assessed strictly, which S's own use does not
// guarantee.
func (s *Schema) checkBindingSubsumes(n QName, r attributeRestriction, general, specific defaultBinding) error {
	if g, ok := general.(wildcardKeywordBinding); ok {
		return checkKeywordSubsumes("attribute "+n.String(), r, g, specific)
	}
	g, gIsUse := general.(attributeUseBinding)
	sp, sIsUse := specific.(attributeUseBinding)
	if gIsUse && sIsUse {
		return s.checkAttributeUseSubsumes(n, r, g.use, sp.use) // clause 5
	}
	return xsderr.New(r.rule, r.loc,
		"%s %s %s, but %s's ·default binding· for attribute %s does not ·subsume· the restriction's (%s, via loc-testSubP)", r.derived.label, r.verb, r.base.label, r.base.label, n, r.clause)
}

// checkKeywordSubsumes decides loc-testSubP clauses 1-3, where the base's
// binding G is one of the three keywords, and charges the ways they can fail.
// The predicate itself is keywordSubsumes, which the element half of the
// definition shares (STYLE T4); only the message is built here. Every refusal
// has a keyword S, one of the pairings keywordSubsumes' doc lists. subject names
// the attribute the two bindings are for: one expanded name from
// checkBindingSubsumes, the wildcards' witness (wildcardWitness) from
// checkAttributeRestrictionWildcard.
func checkKeywordSubsumes(subject string, r attributeRestriction, general wildcardKeywordBinding, specific defaultBinding) error {
	if keywordSubsumes(general, specific) {
		return nil
	}
	requirement := "loc-testSubP clause 2 requires the specific binding not to be skip"
	if general.keyword == ProcessStrict {
		requirement = "loc-testSubP clause 3 requires the specific binding to be strict too"
	}
	return xsderr.New(r.rule, r.loc,
		"%s %s %s, but %s binds %s to a %s wildcard while the restriction binds it to %s, and %s (%s)", r.derived.label, r.verb, r.base.label, r.base.label, subject, general.keyword, describeRefusedBinding(specific), requirement, r.clause)
}

// describeRefusedBinding names the specific binding keywordSubsumes refused, for
// checkKeywordSubsumes' message.
func describeRefusedBinding(specific defaultBinding) string {
	k, ok := specific.(wildcardKeywordBinding)
	if !ok {
		return "a non-keyword binding"
	}
	return "a " + k.keyword.String() + " wildcard"
}

// keywordSubsumes is loc-testSubP clauses 1-3, where the general binding G is
// one of the three keywords. It is the ONE encoding of the wildcard-bucket
// lattice (STYLE T4): both halves of ·subsumes· reach it — the attribute half
// through checkKeywordSubsumes (derivation-ok-restriction clause 3) and the
// element half through bindingSubsumes (clause 2.4.2's
// cos-content-act-restrict) — and neither re-derives it.
//
//   - clause 1: G is skip, which subsumes anything.
//   - clause 2: G is lax and S is not skip.
//   - clause 3: both G and S are strict. A strict G is refused against every
//     non-strict keyword S, skip or lax, and accepted against an Attribute Use
//     or Element Declaration S; see the GAP below for the reading taken there.
//
// This is the one list of the pairings refused, and other sites name it by
// reference: a lax G against a skip S, and a strict G against a skip or lax S.
// Every one has a keyword S, and each refusal is exact. A keyword is rendered
// only for an item ·attributed· to a wildcard (key-dft-binding cases 4-6). A
// wildcard that admits a namespace admits infinitely many expanded names while
// a schema declares finitely many, so some item it admits has no ·governing·
// declaration in any assessment episode, and that item's binding under either
// wildcard IS the wildcard's keyword. ctr-child-type-subsumption quantifies
// "for all elements E in ES" with no episode parameter, so that one item
// decides the clause (#2546). The argument needs an S wildcard that admits a
// namespace: contentModelRestricts takes no transition on one that admits
// none, and the attribute half passes a keyword S only from
// checkAttributeRestrictionWildcard, past an admitsSomeNamespace test.
func keywordSubsumes(general wildcardKeywordBinding, specific defaultBinding) bool {
	switch general.keyword {
	case ProcessSkip:
		return true // clause 1
	case ProcessLax:
		k, ok := specific.(wildcardKeywordBinding)
		return !ok || k.keyword != ProcessSkip // clause 2
	case ProcessStrict:
		// Clause 3 against a keyword S, exact as the doc above argues. W3C suite
		// wildZ008 is the skip pairing; MS-Particles particlesOb001 and
		// MS-Errata10 errC008 are the lax one.
		if k, ok := specific.(wildcardKeywordBinding); ok {
			return k.keyword == ProcessStrict // clause 3
		}
		// GAP(xsd): loc-testSubP clause 3 says a strict G ·subsumes· only another
		// strict S, so a restriction that replaces a base's strict wildcard with a
		// named {attribute use} — or, on the element side, with a named element
		// particle — reads as a violation. That reading is not statically sound
		// and is not what conforming processors do: the keyword is reached only
		// through key-dft-binding cases 4/5, whose "does not have a ·governing
		// element declaration· or a ·governing attribute declaration·" qualifier
		// is an assessment-episode fact this check cannot settle (see
		// attributeDefaultBinding's and elementPositionBinding's GAPs), and XSD
		// 1.0's derivation-ok-restriction decided this branch by namespace
		// allowance alone. Rejecting would decline the canonical valid pattern
		// "base carries a ##any wildcard, restriction names specific attributes
		// or elements" — W3C suite MS-ComplexType ctG007 and ctO003 declare
		// exactly that VALID. RULED permanent by #345 (STYLE P3b), over the
		// assessment-dependent extent only: specific an Attribute Use or an
		// Element Declaration. Each names finitely many expanded names, so no
		// item need exist that a case-1/2/3 binding cannot govern, and a real
		// one might apply in place of G's keyword for every item. A keyword S is
		// outside the extent and was decided above (#2546). Only those two reach
		// the return below. There accepting is FAIL-OPEN against both readers of
		// false: checkKeywordSubsumes charges derivation-ok-restriction clause 3
		// on it, and bindingSubsumes hands it through someBindingSubsumes to
		// contentModelRestricts, which charges cos-content-act-restrict clause 2;
		// neither charges on true.
		return true
	default:
		panic("xsd: keywordSubsumes: non-exhaustive ProcessContents switch")
	}
}

// bindingSubsumes is the bool-valued rendering of ·subsumes· (loc-testSubP) the
// ELEMENT half needs: cos-content-act-restrict clause 2
// (ctr-child-type-subsumption) is one conjunct of a DISJUNCTION —
// derivation-ok-restriction clause 2 — so a failure inside it is a verdict, not
// yet an error, and contentTypeRestricts (complexderivation.go) answers a bool
// all the way up to the one site that charges the rule.
//
//   - G a keyword: clauses 1-3, keywordSubsumes.
//   - both Element Declarations: clause 4, elementDeclarationSubsumes.
//   - anything else fails to subsume. An Element Declaration G against a keyword
//     S is the pairing loc-testSubP deliberately leaves uncovered — clause 3
//     requires BOTH sides strict — so a wildcard-matched item in the restriction
//     is never subsumed by an explicitly named element declaration in the base.
//     Attribute Uses cannot reach here: key-dft-binding cases 2 and 3 are
//     attribute-only, and the element sequences this predicate serves produce
//     only cases 1 and 4-6.
func (s *Schema) bindingSubsumes(general, specific defaultBinding) bool {
	if g, ok := general.(wildcardKeywordBinding); ok {
		return keywordSubsumes(g, specific) // clauses 1-3
	}
	g, gIsElement := general.(elementDeclarationBinding)
	sp, sIsElement := specific.(elementDeclarationBinding)
	if gIsElement && sIsElement {
		return s.elementDeclarationSubsumes(g.decl, sp.decl) // clause 4
	}
	return false
}

// elementDeclarationSubsumes is loc-testSubP clause 4, where both bindings are
// Element Declarations. All six sub-clauses must hold; they are tested in spec
// order so the verdict does not depend on evaluation order.
//
//   - 4.1: G.{nillable} = true or S.{nillable} = false.
//   - 4.2: G has no {value constraint}, or it is not fixed, or S has a fixed
//     {value constraint} with an equal or identical value.
//   - 4.3: S.{identity-constraint definitions} ⊇ G.{identity-constraint definitions}.
//   - 4.4: S disallows a superset of the substitutions G does.
//   - 4.5 (c-vs-ct): S's declared {type definition} is ·validly substitutable as
//     a restriction· for G's (key-val-sub-type-restricts: ·validly
//     substitutable· subject to the blocking keywords {extension, list, union},
//     which is exactly restrictionBlockingKeywords).
//   - 4.6 (c-tt-equiv): the two {type table}s are both ·absent· or both present
//     and ·equivalent· (key-equiv-tt, typeTablesEquivalent).
func (s *Schema) elementDeclarationSubsumes(general, specific ElementDeclaration) bool {
	if !general.Nillable() && specific.Nillable() {
		return false // clause 4.1
	}
	if !s.fixedValueConstraintSubsumes(general, specific) {
		return false // clause 4.2
	}
	if !identityConstraintsSuperset(specific, general) {
		return false // clause 4.3
	}
	if !disallowedSubstitutionsSuperset(specific, general) {
		return false // clause 4.4
	}
	if !s.declaredTypeRestricts(specific, general) {
		return false // clause 4.5
	}
	return typeTablesAgree(general, specific) // clause 4.6
}

// fixedValueConstraintSubsumes is loc-testSubP clause 4.2. An absent or
// non-fixed G {value constraint} discharges it outright; a fixed G against an
// absent or default S is an exact rejection, since no reading of "S has a fixed
// {value constraint}" can hold.
//
// The remaining outcome — both fixed — is 4.2's "with an equal or identical value",
// a VALUE-space test: "1" and "01" are the same xs:integer value, so the
// {lexical form}s ValueConstraint carries cannot decide it (valueconstraint.go).
// The schema's installed ValueSpace (valuespace.go) decides it instead, in each
// declaration's elementValueType; an undecided verdict accepts, so the comparison
// can only NARROW what this clause admits. Two ·special· types are decided over
// their mapping union (the ValueSpace contract).
//
// GAP(xsd): what remains fail-open here is what the ValueSpace declines to
// decide, and every one of those accepts:
//
//   - a type no backend mapping governs, other than the ·special· pair. RULED
//     permanent by #1379 (STYLE P3b): which types a backend maps is backend
//     coverage, and charging loc-testSubP for a type the processor cannot read
//     would blame the schema for a gap in the processor — #774's ruling at
//     validate's matchedAttribute, transferred.
//   - a {lexical form} the governing mapping cannot map. RULED permanent by
//     #1379 (STYLE P3b): such a literal is not Datatype Valid, which
//     checkSimpleDefault charges under a-props-correct, au-props-correct or
//     e-props-correct clause 2 (cos-valid-simple-default clause 1); where
//     ValidDefault stays undecided too, it is undecided for the type, not for
//     this comparison. "Not a value" is never "not the same value".
//   - a QName or NOTATION lexical whose prefix has no binding in the context its
//     ValueConstraint captured: package value's own GAP(value) marker owns it,
//     and #667 the routing that retires it; such a literal also fails
//     cos-valid-simple-default clause 1. A resolvable prefix IS compared.
//   - two types resolving to DIFFERENT governing mappings, a list or union type,
//     and a ·special· type against an ordinary one (#2087).
//   - an absent or unresolvable {type definition}. RULED permanent by #1379
//     (STYLE P3b): Phase A charges src-resolve for a dangling name, and an
//     absent one names no value space at all.
//   - a MIXED complex {type definition}, whose {value} cos-valid-default clause 2
//     reads with no simple type to name a value space (#2087). An element-only
//     or empty one admits no {value constraint} at all, which e-props-correct
//     clause 2 charges (cos-valid-default clause 2.1).
func (s *Schema) fixedValueConstraintSubsumes(general, specific ElementDeclaration) bool {
	gvc, present := general.ValueConstraint()
	if !present || gvc.Kind() != ValueFixed {
		return true
	}
	svc, present := specific.ValueConstraint()
	if !present || svc.Kind() != ValueFixed {
		return false
	}
	gt, ok := s.elementValueType(general)
	if !ok {
		return true
	}
	st, ok := s.elementValueType(specific)
	if !ok {
		return true
	}
	same, decided := s.valueSpace.EqualOrIdentical(s, st, svc, gt, gvc)
	return same || !decided
}

// elementValueType is the Simple Type Definition naming the value space of e's
// {value constraint}.{value}: e's {type definition} itself when simple, or its
// {content type}.{simple type definition} when it is a complex type with simple
// content — the same pick cos-valid-default clause 1 makes
// (checkComplexDefaultValid). ok is false for an absent or unresolvable {type
// definition} and for a complex one whose {content type} is not simple.
func (s *Schema) elementValueType(e ElementDeclaration) (*SimpleType, bool) {
	t, ok := s.ResolvedType(e.TypeDefinition())
	if !ok {
		return nil, false
	}
	if st, isSimple := t.(*SimpleType); isSimple {
		return st, true
	}
	c, isComplex := t.(ComplexType)
	if !isComplex {
		return nil, false
	}
	sc, isSimpleContent := c.ContentType().(SimpleContent)
	return sc.SimpleType, isSimpleContent
}

// identityConstraintsSuperset is loc-testSubP clause 4.3: every member of
// general's {identity-constraint definitions} must also be a member of
// specific's. Component identity is the expanded {name}, the same reading
// sameTypeDefinition (complexderivation.go) takes for type definitions —
// sch-props-correct clause 2 keeps {identity-constraint definitions} unique by
// expanded name across the schema, so a name is a component key.
//
// Both sets are walked in document order; no map is consulted (STYLE D2).
func identityConstraintsSuperset(specific, general ElementDeclaration) bool {
	for _, g := range general.identityConstraints {
		if !hasIdentityConstraintNamed(specific, g.Name()) {
			return false
		}
	}
	return true
}

// hasIdentityConstraintNamed reports whether e's {identity-constraint
// definitions} contains one with the expanded name.
func hasIdentityConstraintNamed(e ElementDeclaration, name QName) bool {
	for _, c := range e.identityConstraints {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// disallowedSubstitutionsSuperset is loc-testSubP clause 4.4, "S disallows a
// superset of the substitutions that G does": every member of general's
// {disallowed substitutions} must also be a member of specific's.
func disallowedSubstitutionsSuperset(specific, general ElementDeclaration) bool {
	for _, m := range general.disallowedSubstitutions {
		if !containsDerivationMethod(specific.disallowedSubstitutions, m) {
			return false
		}
	}
	return true
}

// declaredTypeRestricts is loc-testSubP clause 4.5 (c-vs-ct), delegating to
// ValidlySubstitutable (key-val-sub-type) under the {extension, list, union}
// blocking keywords that ·validly substitutable as a restriction·
// (key-val-sub-type-restricts) names — the same set derivation-ok-restriction
// clause 4 works under, and the same slice, so the two clauses cannot drift.
//
// An absent or unresolvable {type definition} on either side is SKIPPED rather
// than rejected, exactly as checkLocallyDeclaredElementTypes skips it: there is
// no component to compare, so the clause is not competent to charge a failure.
// Skipping is fail-open, never a false reject. An ANONYMOUS type is no longer
// among the skipped cases: ResolvedType hands back the inline component itself, so the
// comparison is made rather than waved through.
//
// An unresolvable simple-type {base type definition} reached INSIDE the derivation walk
// joins that same skipped class, for the same reason and with the same polarity. This is
// the ONE consumer of ValidlySubstitutable in this package that cannot propagate the error
// — it sits inside loc-testSubP's bool chain, which runs all the way down into
// contentrestricts.go's automaton because cos-content-act-restrict clause 2 is one conjunct
// of a DISJUNCTION and so has no error to return until the single site that charges the
// rule — and it already folds exactly this class of fault into "accept". A schema reaching
// here has survived Phase A, which charges src-resolve for every unresolvable base a Schema
// reaches, so the case is unreachable rather than merely benign.
func (s *Schema) declaredTypeRestricts(specific, general ElementDeclaration) bool {
	sub, ok := s.ResolvedType(specific.TypeDefinition())
	if !ok {
		return true
	}
	super, ok := s.ResolvedType(general.TypeDefinition())
	if !ok {
		return true
	}
	substitutable, err := s.ValidlySubstitutable(sub, super, restrictionBlockingKeywords)
	return err != nil || substitutable
}

// typeTablesAgree is loc-testSubP clause 4.6 (c-tt-equiv): the two {type table}s
// are both ·absent·, or both present and ·equivalent· per key-equiv-tt. The
// equivalence itself is elementconsistent.go's typeTablesEquivalent, the one
// canonical §3.8.6.3 implementation (STYLE T4).
func typeTablesAgree(general, specific ElementDeclaration) bool {
	gt, gPresent := general.TypeTable()
	st, sPresent := specific.TypeTable()
	if gPresent != sPresent {
		return false
	}
	if !gPresent {
		return true
	}
	return typeTablesEquivalent(st, gt)
}

// checkAttributeUseSubsumes decides loc-testSubP clause 5, where both bindings
// are Attribute Uses: 5.1 type derivation, 5.2 effective value constraint, 5.3
// {inheritable} equality. The sub-clauses are checked in spec order so the first
// reported failure is deterministic (STYLE D1).
func (s *Schema) checkAttributeUseSubsumes(n QName, r attributeRestriction, general, specific AttributeUse) error {
	if err := s.checkAttributeTypeDerivedOK(n, r, general, specific); err != nil {
		return err
	}
	if err := s.checkAttributeValueConstraintSubsumes(n, r, general, specific); err != nil {
		return err
	}
	g, sp := s.ResolvedInheritable(general), s.ResolvedInheritable(specific)
	if g != sp {
		return xsderr.New(r.rule, r.loc,
			"%s %s %s, but attribute %s has {inheritable} = %t there and %t in the base, and loc-testSubP clause 5.3 requires them to be equal (%s)", r.derived.label, r.verb, r.base.label, n, sp, g, r.clause)
	}
	return nil
}

// checkAttributeTypeDerivedOK is loc-testSubP clause 5.1: S.{attribute
// declaration}.{type definition} must be validly ·derived· from G's, as defined
// in Type Derivation OK (Simple) (§3.16.6.3, cos-st-derived-ok).
//
// The blocking-keyword set the enclosing clause 3 works under is empty here —
// clause 5.1 names cos-st-derived-ok with no set — so derivedOKSimple
// (derivation.go) is called with a nil blocked.
//
// An absent {type definition} on either side is SKIPPED rather than rejected:
// it names no type to derive from. Skipping is fail-open, never a false reject.
// A dangling type= and one naming a complex type never reach here: Phase A
// charges both src-resolve (resolveTypeDefinitionSlot, resolveAttributeDecl).
// Both sides are resolved through attributeUseType, the one encoding of "the
// simple type governing this use" clause 5.2.2 also reads (STYLE T4).
//
// An unresolvable {base type definition} INSIDE either chain is a different
// thing and is returned as the src-resolve error rather than skipped: this frame
// charges an error already, so there is no bool to fold it into (see
// validlyDerived). It is unreachable for a schema that survived Phase A.
func (s *Schema) checkAttributeTypeDerivedOK(n QName, r attributeRestriction, general, specific AttributeUse) error {
	gt, ok := s.attributeUseType(general)
	if !ok {
		return nil
	}
	st, ok := s.attributeUseType(specific)
	if !ok {
		return nil
	}
	derived, err := derivedOKSimple(s, st, gt, nil)
	if err != nil {
		return err
	}
	if derived {
		return nil
	}
	return xsderr.New(r.rule, r.loc,
		"%s %s %s but types attribute %s as %s, which is not validly derived from the base's %s (%s, via loc-testSubP clause 5.1 and cos-st-derived-ok §3.16.6.3)", r.derived.label, r.verb, r.base.label, n, typeDefinitionLabel(st), typeDefinitionLabel(gt), r.clause)
}

// checkAttributeValueConstraintSubsumes is loc-testSubP clause 5.2: with GVC and
// SVC the two ·effective value constraints· (key-evc), one or more of 5.2.1 (GVC
// ·absent· or {variety} default) and 5.2.2 (SVC.{variety} = fixed and SVC.{value}
// equal or identical to GVC.{value}) must hold.
//
// The first two outcomes are exact. 5.2.1 is read directly. When GVC is fixed
// and SVC is absent or default, 5.2.2 cannot hold under any reading, so the
// rejection is exact. When both are fixed, 5.2.2's "equal or identical" is a
// VALUE-space test — "1" and "01" are the same xs:integer value with different
// lexical forms — which the {lexical form}s ValueConstraint carries
// (valueconstraint.go) cannot decide; the schema's installed ValueSpace
// (valuespace.go) decides it, in each side's own attribute {type definition},
// and an undecided verdict accepts, so the comparison can only NARROW what this
// clause admits. Two ·special· types — what every typeless <attribute> gets
// (§3.2.2.2) — are decided over their mapping union (the ValueSpace contract).
//
// GAP(xsd): what remains fail-open is what the ValueSpace declines to decide,
// and every one of those accepts. Four declines are fixedValueConstraintSubsumes'
// clause 4.2 twin's, on the terms stated there: a type no backend mapping
// governs other than the ·special· pair and a {lexical form} the governing
// mapping cannot map are each RULED permanent by #1379 (STYLE P3b); an unbound
// QName or NOTATION prefix is package value's GAP(value) marker's, retired by
// #667; and an absent or unresolvable {type definition} is RULED permanent by
// #1379 (STYLE P3b), src-resolve charging the dangling name. The rest are not
// permanent and belong to #2087: two types resolving to DIFFERENT governing
// mappings, a list or union type, and a ·special· type against an ordinary
// one — all of which clause 5.1 permits, since S's type need only be DERIVED
// from G's. A complex {type definition} is not among them: Phase A charges a
// by-name one src-resolve (resolveAttributeDecl), and NewAttributeDeclaration
// an inline one a-props-correct clause 1.
func (s *Schema) checkAttributeValueConstraintSubsumes(n QName, r attributeRestriction, general, specific AttributeUse) error {
	gvc, present := s.EffectiveValueConstraint(general)
	if !present || gvc.Kind() != ValueFixed {
		return nil // clause 5.2.1
	}
	svc, present := s.EffectiveValueConstraint(specific)
	if !present || svc.Kind() != ValueFixed {
		return xsderr.New(r.rule, r.loc,
			"%s %s %s, but the base fixes attribute %s to %q while the restriction leaves it unfixed, and loc-testSubP clause 5.2 requires a fixed ·effective value constraint· with the same value (%s)", r.derived.label, r.verb, r.base.label, n, gvc.LexicalForm(), r.clause)
	}
	if s.attributeValueConstraintsAgree(general, specific, gvc, svc) {
		return nil // clause 5.2.2
	}
	return xsderr.New(r.rule, r.loc,
		"%s %s %s and fixes attribute %s to %q, but the base fixes it to %q, and loc-testSubP clause 5.2.2 requires the two {value}s to be equal or identical (%s)", r.derived.label, r.verb, r.base.label, n, svc.LexicalForm(), gvc.LexicalForm(), r.clause)
}

// attributeValueConstraintsAgree decides loc-testSubP clause 5.2.2's "SVC.{value}
// is equal or identical to GVC.{value}" for two fixed ·effective value
// constraints·, asking the installed ValueSpace in each side's own attribute
// {type definition}. It reports true — accept — for every case the ValueSpace
// does not decide and for every side whose {type definition} names no simple
// type; see checkAttributeValueConstraintSubsumes' GAP for the residual.
func (s *Schema) attributeValueConstraintsAgree(general, specific AttributeUse, gvc, svc ValueConstraint) bool {
	gt, ok := s.attributeUseType(general)
	if !ok {
		return true
	}
	st, ok := s.attributeUseType(specific)
	if !ok {
		return true
	}
	same, decided := s.valueSpace.EqualOrIdentical(s, st, svc, gt, gvc)
	return same || !decided
}

// attributeUseType is the Simple Type Definition governing an attribute use's
// values: its {attribute declaration}.{type definition}, resolved through the
// same two helpers every other consumer here uses (STYLE T4). ok is false for a
// dangling Ref and for an absent {type definition} — the cases the
// value-constraint clauses treat as "not decidable", never as a violation. An
// unresolvable or complex {type definition} is false too, but no finalized
// schema holds one: Phase A charges both src-resolve (resolveAttributeDecl).
func (s *Schema) attributeUseType(u AttributeUse) (*SimpleType, bool) {
	d, ok := s.ResolvedAttributeDeclaration(u)
	if !ok {
		return nil, false
	}
	return s.ResolvedSimpleType(d.TypeDefinition())
}

// ResolvedSimpleType narrows [Schema.ResolvedType] to a Simple Type Definition. ok
// is false for an absent slot, an unresolvable name, and a {type definition} that
// is a complex type — the three cases every caller treats as "not decidable by
// this clause", never as a violation.
//
// It is exported for the instance validator, which reaches a Simple Type
// Definition through two different slots and must not write the *SimpleType
// assertion at either: an attribute declaration's {type definition} for
// cvc-attribute (§3.2.4.1) clause 3, and the same slot again for
// cvc-complex-type (§3.4.4.2) clause 4's ·defaulted attribute· (#766). Both hand
// the result to value.ValidateLexical, which takes a *SimpleType and nothing
// wider.
func (s *Schema) ResolvedSimpleType(ref TypeDefinitionOrRef) (*SimpleType, bool) {
	t, ok := s.ResolvedType(ref)
	if !ok {
		return nil, false
	}
	st, ok := t.(*SimpleType)
	return st, ok
}
